// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package vpntest

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

// readyLine is what the server's log says when it is ready for clients.
const readyLine = "Initialization Sequence Completed"

// startAttempts is how many ports StartServer tries when another program took one.
const startAttempts = 3

// devCounter makes the device names of the servers of one test process different.
var devCounter atomic.Int32

// ServerOptions say how the server of a test is set up.
type ServerOptions struct {
	// Proto is "udp" (the default) or "tcp".
	Proto string
	// DNS are pushed to the clients as "dhcp-option DNS".
	DNS []netip.Addr
	// SearchDomain is pushed as "dhcp-option DOMAIN" when it is not empty.
	SearchDomain string
	// Username and Password, when Password is not empty, are what the server asks for.
	Username string
	Password string
	// PushIPv6 also gives the clients an IPv6 address, of fd00:8::/64.
	PushIPv6 bool
}

// Server is a running OpenVPN server on the loopback of the namespace of scripts/vpn-ns.sh.
type Server struct {
	t       testing.TB
	dir     string
	args    []string
	profile string

	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	launches int
}

// StartServer starts openvpn and returns when it is ready. It skips the test when openvpn is
// not installed or the test does not run under scripts/vpn-ns.sh. The server is stopped when
// the test ends.
func StartServer(t testing.TB, opts ServerOptions) *Server {
	t.Helper()
	if _, err := exec.LookPath("openvpn"); err != nil {
		t.Skip("vpntest: openvpn is not installed")
	}
	if os.Getenv("TUBE_NS") != "1" {
		t.Skip("vpntest: not inside the namespace of scripts/vpn-ns.sh")
	}
	proto := opts.Proto
	if proto == "" {
		proto = "udp"
	}
	if proto != "udp" && proto != "tcp" {
		t.Fatalf("vpntest: Proto is %q, want udp or tcp", opts.Proto)
	}
	dir := t.TempDir()
	pki := profilestest.NewPKI(t)
	files := map[string][]byte{"ca.crt": pki.CA, "server.crt": pki.ServerCert, "server.key": pki.ServerKey}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("vpntest: %v", err)
		}
	}
	extra := []string{"--keepalive", "1", "4"}
	for _, addr := range opts.DNS {
		extra = append(extra, "--push", "dhcp-option DNS "+addr.String())
	}
	if opts.SearchDomain != "" {
		extra = append(extra, "--push", "dhcp-option DOMAIN "+opts.SearchDomain)
	}
	if opts.PushIPv6 {
		extra = append(extra, "--server-ipv6", "fd00:8::/64")
	}
	if opts.Password != "" {
		extra = append(extra, authArgs(t, dir, opts.Username, opts.Password)...)
	}

	s := &Server{t: t, dir: dir}
	t.Cleanup(s.Stop)
	for attempt := 1; ; attempt++ {
		port := freePort(t, proto)
		s.args = append([]string{
			"--mode", "server",
			"--tls-server",
			"--dev", fmt.Sprintf("tt%dx%d", os.Getpid(), devCounter.Add(1)%1000),
			"--dev-type", "tun",
			"--topology", "subnet",
			"--server", "10.8.0.0", "255.255.255.0",
			"--proto", proto,
			"--local", "127.0.0.1",
			"--port", strconv.Itoa(port),
			"--ca", filepath.Join(dir, "ca.crt"),
			"--cert", filepath.Join(dir, "server.crt"),
			"--key", filepath.Join(dir, "server.key"),
			"--dh", "none",
			"--verb", "4",
			"--disable-dco",
		}, extra...)
		failure := s.launch()
		if failure == "" {
			profileDir := filepath.Join(dir, "client")
			s.profile = profilestest.WriteProfile(t, profileDir, "test.ovpn", pki, profilestest.Options{
				Remote: "127.0.0.1 " + strconv.Itoa(port), Proto: proto, Password: opts.Password != "",
			})
			return s
		}
		if !strings.Contains(failure, "Address already in use") || attempt == startAttempts {
			t.Fatalf("vpntest: openvpn did not start (attempt %d):\n%s", attempt, failure)
		}
	}
}

// authArgs are the arguments of a server that checks a user name and a password with a script
// of the test, which compares the two lines that openvpn writes with the expected ones.
func authArgs(t testing.TB, dir, username, password string) []string {
	t.Helper()
	expected := filepath.Join(dir, "expected")
	if err := os.WriteFile(expected, []byte(username+"\n"+password+"\n"), 0o600); err != nil {
		t.Fatalf("vpntest: %v", err)
	}
	script := filepath.Join(dir, "verify.sh")
	text := "#!/bin/sh\n" +
		"{ IFS= read -r u; IFS= read -r p; } < \"$1\"\n" +
		"{ IFS= read -r eu; IFS= read -r ep; } < '" + expected + "'\n" +
		"[ \"$u\" = \"$eu\" ] && [ \"$p\" = \"$ep\" ]\n"
	if err := os.WriteFile(script, []byte(text), 0o700); err != nil {
		t.Fatalf("vpntest: %v", err)
	}
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatalf("vpntest: %v", err)
	}
	// The script is run by /bin/sh, so it does not need a folder that allows executing.
	return []string{"--script-security", "2", "--auth-user-pass-verify", "/bin/sh " + script, "via-file", "--tmp-dir", tmp}
}

// freePort asks the system for a free port of the loopback. Another program may take it before
// the server binds it: StartServer then tries again.
func freePort(t testing.TB, proto string) int {
	t.Helper()
	if proto == "tcp" {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("vpntest: a free TCP port (is the loopback up?): %v", err)
		}
		defer closeOrLog(t, l)
		return l.Addr().(*net.TCPAddr).Port
	}
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("vpntest: a free UDP port (is the loopback up?): %v", err)
	}
	defer closeOrLog(t, c)
	return c.LocalAddr().(*net.UDPAddr).Port
}

// closeOrLog closes c and logs a failure.
func closeOrLog(t testing.TB, c interface{ Close() error }) {
	if err := c.Close(); err != nil {
		t.Logf("vpntest: close: %v", err)
	}
}

// logPath is the log file of the launch n.
func (s *Server) logPath(n int) string {
	return filepath.Join(s.dir, "server-"+strconv.Itoa(n)+".log")
}

// launch starts openvpn with the server's arguments and waits until it is ready. It returns ""
// on success, and on failure the reason with the log, after stopping the process.
func (s *Server) launch() string {
	s.mu.Lock()
	s.launches++
	n := s.launches
	cmd := exec.Command("openvpn", append(s.args, "--log", s.logPath(n))...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return "cannot run openvpn: " + err.Error()
	}
	done := make(chan struct{})
	s.cmd, s.done = cmd, done
	s.mu.Unlock()
	go func() {
		// The status is not needed: Stop only waits for the end.
		_ = cmd.Wait()
		close(done)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// The log does not exist until openvpn opens it.
		if data, err := os.ReadFile(s.logPath(n)); err == nil && strings.Contains(string(data), readyLine) {
			return ""
		}
		select {
		case <-done:
			return "openvpn exited before it was ready:\n" + s.Log()
		case <-time.After(25 * time.Millisecond):
		}
	}
	s.Stop()
	return "openvpn was not ready after 10 seconds:\n" + s.Log()
}

// ProfilePath is a client profile for this server, in a folder of the test.
func (s *Server) ProfilePath() string { return s.profile }

// TunnelAddr is the server's own address inside the tunnel.
func (s *Server) TunnelAddr() netip.Addr { return netip.MustParseAddr("10.8.0.1") }

// TunnelAddr6 is the server's own IPv6 address inside the tunnel, or the zero Addr when it
// gives no IPv6.
func (s *Server) TunnelAddr6() netip.Addr {
	if !strings.Contains(strings.Join(s.args, " "), "--server-ipv6") {
		return netip.Addr{}
	}
	return netip.MustParseAddr("fd00:8::1")
}

// Running says whether the server's process runs.
func (s *Server) Running() bool {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// Stop ends the server and returns when its process has been waited for: SIGTERM to its
// process group, and SIGKILL after three seconds. It may be called more than once.
func (s *Server) Stop() {
	s.mu.Lock()
	cmd, done := s.cmd, s.done
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	select {
	case <-done:
		return
	default:
	}
	// The group may be gone already; the wait below is what matters.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}

// Start starts the server again, after Stop, on the same port and with the same keys.
func (s *Server) Start() {
	s.t.Helper()
	if failure := s.launch(); failure != "" {
		s.t.Fatalf("vpntest: openvpn did not start again:\n%s", failure)
	}
}

// Log is the server's log, of every launch, for the messages of a failing test.
func (s *Server) Log() string {
	s.mu.Lock()
	n := s.launches
	s.mu.Unlock()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		data, err := os.ReadFile(s.logPath(i))
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "--- launch %d\n%s", i, data)
	}
	return b.String()
}
