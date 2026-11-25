// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/dnstest"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// addrs parses addresses for a test.
func addrs(texts ...string) []netip.Addr {
	var out []netip.Addr
	for _, text := range texts {
		out = append(out, netip.MustParseAddr(text))
	}
	return out
}

func TestResolvConf(t *testing.T) {
	const head = "# Written by Tube for the application that runs in the VPN.\n"
	const none = "# The VPN gave no DNS server: names will not be found.\n"
	const tail = "options edns0\n"
	tests := []struct {
		name   string
		dns    []netip.Addr
		search []string
		want   string
	}{
		{"one server", addrs("10.9.0.1"), nil, head + "nameserver 10.9.0.1\n" + tail},
		{"five servers", addrs("10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"), nil,
			head + "nameserver 10.0.0.1\nnameserver 10.0.0.2\nnameserver 10.0.0.3\n" + tail},
		{"an IPv6 server", addrs("fd00::53"), nil, head + "nameserver fd00::53\n" + tail},
		{"a server with a zone", addrs("fe80::1%eth0", "10.9.0.1"), nil, head + "nameserver 10.9.0.1\n" + tail},
		{"no server", nil, nil, head + none + tail},
		{"two search domains", addrs("10.9.0.1"), []string{"tube.test", "example.org"},
			head + "nameserver 10.9.0.1\nsearch tube.test example.org\n" + tail},
		{"eight search domains", addrs("10.9.0.1"), []string{"a1.test", "a2.test", "a3.test", "a4.test", "a5.test", "a6.test", "a7.test", "a8.test"},
			head + "nameserver 10.9.0.1\nsearch a1.test a2.test a3.test a4.test a5.test a6.test\n" + tail},
		{"a search domain with a new line", addrs("10.9.0.1"), []string{"evil.test\nnameserver 192.0.2.1", "tube.test"},
			head + "nameserver 10.9.0.1\nsearch tube.test\n" + tail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(ResolvConf(tt.dns, tt.search)); got != tt.want {
				t.Errorf("ResolvConf =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestFollowLinks(t *testing.T) {
	// The answers have their folders' links resolved: so must the expected ones.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	link := func(target, name string) string {
		path := filepath.Join(dir, name)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	file := write("file")
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	link("real", "linked")
	tests := []struct {
		name, path, want string
	}{
		{"a file", file, file},
		{"an absolute link", link(file, "abs"), file},
		{"a relative link to a link", link("abs", "rel"), file},
		{"a link to nothing", link("real/missing", "dangling"), filepath.Join(dir, "real", "missing")},
		{"nothing", filepath.Join(dir, "nothing"), filepath.Join(dir, "nothing")},
		{"a folder that is a link", filepath.Join(dir, "linked", "x"), filepath.Join(dir, "real", "x")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := followLinks(tt.path)
			if err != nil || got != tt.want {
				t.Errorf("followLinks = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
	t.Run("a loop", func(t *testing.T) {
		link("loop-b", "loop-a")
		path := link("loop-a", "loop-b")
		_, err := followLinks(path)
		meta := errtest.Route(t, err, "followLinks.KeeperResolvLinkFailed")
		if meta["reason"] != "more than 8 symbolic links" {
			t.Errorf("metadata = %v", meta)
		}
	})
}

func TestDNSFilesFailures(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("a folder that cannot be covered", func(t *testing.T) {
		// The test process has no mount namespace of its own: the kernel refuses the mount.
		d := &dnsFiles{}
		errtest.Route(t, d.cover(dir), "dnsFiles.cover.KeeperCoverFailed")
	})
	t.Run("a folder that does not exist is not covered", func(t *testing.T) {
		d := &dnsFiles{}
		if err := d.cover(filepath.Join(dir, "missing")); err != nil || len(d.covered) != 0 {
			t.Errorf("cover = %v, covered %v", err, d.covered)
		}
	})
	t.Run("a file that cannot be bind-mounted", func(t *testing.T) {
		d := &dnsFiles{tmpDir: t.TempDir(), target: file}
		errtest.Route(t, d.bind([]byte("x")), "dnsFiles.bind.KeeperResolvMountFailed")
	})
	t.Run("no folder for the file", func(t *testing.T) {
		d := &dnsFiles{target: file}
		errtest.Route(t, d.bind([]byte("x")), "dnsFiles.bind.KeeperNoTempDir")
	})
	t.Run("a file that cannot be written", func(t *testing.T) {
		errtest.Route(t, writeResolv(filepath.Join(file, "resolv.conf"), []byte("x"), true), "writeResolv.KeeperResolvWriteFailed")
	})
}

// machineNameservers are the servers of the machine's resolv.conf. The tests read that file
// because the promise is about it.
func machineNameservers(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && fields[0] == "nameserver" {
			out = append(out, fields[1])
		}
	}
	return out
}

// files runs the probe files of r and reads what it printed.
func (r running) files(t *testing.T) sandboxtest.Files {
	t.Helper()
	if st := r.run(t); st.Status() != 0 {
		t.Fatalf("the probe ended with %+v:\n%s", st, r.errors())
	}
	var f sandboxtest.Files
	if err := json.Unmarshal([]byte(r.output(t)), &f); err != nil {
		t.Fatalf("what the probe printed cannot be read: %v\n%s", err, r.output(t))
	}
	return f
}

// withDNS is network4 with DNS servers and search domains.
func withDNS(dns []netip.Addr, search ...string) vpn.Network {
	n := network4
	n.DNSServers, n.SearchDomains = dns, search
	return n
}

func TestResolvConfInside(t *testing.T) {
	r := startProbe(t, "files")
	if warnings := r.configure(t, withDNS(addrs("10.9.0.1"), "tube.test")); len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	f := r.files(t)
	conf := f.Resolv[resolvConfPath]
	if !strings.Contains(conf, "\nnameserver 10.9.0.1\n") || !strings.Contains(conf, "\nsearch tube.test\n") {
		t.Errorf("resolv.conf inside is:\n%s", conf)
	}
	for _, server := range machineNameservers(t) {
		if server != "10.9.0.1" && strings.Contains(conf, server) {
			t.Errorf("resolv.conf inside has the machine's server %s:\n%s", server, conf)
		}
	}
	// A covered folder holds at most the file that Tube wrote: where resolv.conf leads.
	target, err := followLinks(resolvConfPath)
	if err != nil {
		t.Fatal(err)
	}
	for dir, names := range f.Dirs {
		for _, name := range names {
			if !strings.HasSuffix(target, string(filepath.Separator)+name) {
				t.Errorf("the covered folder %s has %q, which Tube did not put there", dir, name)
			}
		}
	}
}

// lookupNames are the names that the peer of the lookup tests knows.
var lookupNames = map[string]netip.Addr{"tube-test.example": netip.MustParseAddr("192.0.2.7")}

// isQueryTo says whether a packet is a DNS query over UDP to the server, for the name when it
// is not "".
func isQueryTo(server netip.Addr, name string) func([]byte) bool {
	return func(p []byte) bool {
		_, dst, payload, ok := packettest.ParseUDP4(p)
		if !ok || dst != netip.AddrPortFrom(server, 53) {
			return false
		}
		got, _, _, ok := dnstest.Question(payload)
		return ok && (name == "" || got == name)
	}
}

func TestLookupGoesThroughTheDevice(t *testing.T) {
	r := startProbe(t, "lookup", "tube-test.example")
	r.peer.AnswerDNS(lookupNames)
	r.configure(t, withDNS(addrs("10.9.0.1")))
	st := r.run(t)
	if out := strings.TrimSpace(r.output(t)); st.Status() != 0 || out != "192.0.2.7" {
		t.Errorf("the lookup printed %q and ended with %+v:\n%s", out, st, r.errors())
	}
	r.peer.WaitFor(t, time.Second, isQueryTo(netip.MustParseAddr("10.9.0.1"), "tube-test.example"))
}

func TestGetent(t *testing.T) {
	getent := lookPath(t, "getent")
	r := startSandbox(t, getent, []string{"getent", "hosts", "tube-test.example"})
	r.peer.AnswerDNS(lookupNames)
	r.configure(t, withDNS(addrs("10.9.0.1")))
	st := r.run(t)
	if out := r.output(t); st.Status() != 0 || !strings.Contains(out, "192.0.2.7") {
		t.Errorf("getent printed %q and ended with %+v:\n%s", out, st, r.errors())
	}
	r.peer.WaitFor(t, time.Second, isQueryTo(netip.MustParseAddr("10.9.0.1"), "tube-test.example"))
}

func TestTheProfilesDNSWins(t *testing.T) {
	r := startProbe(t, "lookup", "tube-test.example")
	r.peer.AnswerDNS(lookupNames)
	r.configure(t, withDNS(addrs("10.9.0.1")), netip.MustParseAddr("10.9.0.9"))
	st := r.run(t)
	if out := strings.TrimSpace(r.output(t)); st.Status() != 0 || out != "192.0.2.7" {
		t.Errorf("the lookup printed %q and ended with %+v", out, st)
	}
	r.peer.WaitFor(t, time.Second, isQueryTo(netip.MustParseAddr("10.9.0.9"), ""))
	if slices.ContainsFunc(r.peer.Packets(), isQueryTo(netip.MustParseAddr("10.9.0.1"), "")) {
		t.Error("a query went to the server that the VPN pushed, not to the profile's")
	}
}

func TestNoDNSServer(t *testing.T) {
	r := startProbe(t, "lookup", "tube-test.example")
	r.peer.AnswerDNS(lookupNames)
	warnings := r.configure(t, network4)
	if !slices.Contains(warnings, warnNoDNSServer) {
		t.Errorf("warnings = %q", warnings)
	}
	r.run(t)
	if out := r.output(t); !strings.HasPrefix(out, "error: ") {
		t.Errorf("the lookup printed %q", out)
	}
	for _, p := range r.peer.Packets() {
		if _, _, _, ok := packettest.ParseUDP4(p); ok {
			t.Errorf("a UDP packet left the namespace: % x", p)
		}
	}
}

func TestConfigureTwiceRewritesTheFile(t *testing.T) {
	r := startProbe(t, "files")
	r.configure(t, withDNS(addrs("10.9.0.1")))
	r.configure(t, withDNS(addrs("10.9.0.8")))
	conf := r.files(t).Resolv[resolvConfPath]
	if !strings.Contains(conf, "nameserver 10.9.0.8\n") || strings.Contains(conf, "10.9.0.1") {
		t.Errorf("resolv.conf inside is:\n%s", conf)
	}
}

func TestTheMachinesResolvConfIsUntouched(t *testing.T) {
	before, err := os.ReadFile(resolvConfPath)
	if err != nil {
		t.Skipf("this machine has no resolv.conf: %v", err)
	}
	r := startProbe(t, "files")
	r.configure(t, withDNS(addrs("10.9.0.1")))
	r.files(t)
	if err := r.s.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(resolvConfPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("the machine's resolv.conf changed:\n%s\n(%v)", after, err)
	}
}

// A machine whose resolv.conf is a plain file, outside the covered folders: the test's file
// stands for it.
func TestResolvConfMountedOverAFile(t *testing.T) {
	machine := filepath.Join(t.TempDir(), "resolv.conf")
	const original = "nameserver 192.0.2.53\n"
	if err := os.WriteFile(machine, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	r := startProbeWith(t, func(o *Options) { o.resolvConf = machine }, "files", machine)
	r.configure(t, withDNS(addrs("10.9.0.1")))
	r.configure(t, withDNS(addrs("10.9.0.8")))
	conf := r.files(t).Resolv[machine]
	if !strings.Contains(conf, "nameserver 10.9.0.8\n") || strings.Contains(conf, "192.0.2.53") || strings.Contains(conf, "10.9.0.1") {
		t.Errorf("resolv.conf inside is:\n%s", conf)
	}
	if data, err := os.ReadFile(machine); err != nil || string(data) != original {
		t.Errorf("the file outside changed: %q, %v", data, err)
	}
}

func TestNoResolvConf(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing", "resolv.conf")
	r := startProbeWith(t, func(o *Options) { o.resolvConf = missing }, "files")
	if warnings := r.configure(t, withDNS(addrs("10.9.0.1"))); !slices.Contains(warnings, warnNoResolvConf) {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestNoTempDir(t *testing.T) {
	machine := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(machine, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := startProbeWith(t, func(o *Options) { o.resolvConf, o.TempDir = machine, "" }, "files")
	_, err := r.s.Configure(t.Context(), withDNS(addrs("10.9.0.1")), nil)
	meta := errtest.Route(t, err, "Sandbox.configure.SandboxConfigureFailed")
	keeperRoute(t, meta, "keeper.configure->dnsFiles.apply->dnsFiles.bind.KeeperNoTempDir")
}

func TestTheKeepersVariablesStayInside(t *testing.T) {
	env := keeperEnv(Options{Env: []string{"A=1", keeperTmpVar + "=/elsewhere", keeperResolvVar + "=/etc/passwd"}, TempDir: "/tmp/k"})
	if want := []string{"A=1", keeperTmpVar + "=/tmp/k"}; !slices.Equal(env, want) {
		t.Errorf("the keeper's environment is %q, want %q", env, want)
	}
	r := startSandbox(t, lookPath(t, "env"), []string{"env"})
	r.configure(t, network4)
	r.run(t)
	if out := r.output(t); strings.Contains(out, keeperVarPrefix) {
		t.Errorf("the application has the keeper's variables:\n%s", out)
	}
}
