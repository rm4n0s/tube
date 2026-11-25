// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandboxtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/netlink"
	"golang.org/x/sys/unix"
)

// probeVar is the variable that names the probe the test binary runs.
const probeVar = "TUBE_SANDBOX_PROBE"

// HandleProbe runs the probe that the variable TUBE_SANDBOX_PROBE names, and exits; it returns
// when the variable is not set. A TestMain calls it right after the keeper check. The
// arguments of a probe are the application's arguments, after argv[0].
func HandleProbe() {
	name, ok := os.LookupEnv(probeVar)
	if !ok {
		return
	}
	probes := map[string]func(args []string) error{
		"report":     report,
		"lookup":     lookup,
		"files":      files,
		"stubborn":   stubborn,
		"sigint":     sigint,
		"ping-steps": pingSteps,
		"linger":     linger,
	}
	probe, ok := probes[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "sandboxtest: no probe called %q\n", name)
		os.Exit(2)
	}
	if err := probe(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sandboxtest: the probe %s: %v\n", name, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// ProbeEnv is the variable that makes the test binary run the probe: "TUBE_SANDBOX_PROBE=<name>".
func ProbeEnv(name string) string { return probeVar + "=" + name }

// Report is what the probe report prints, as one JSON object.
type Report struct {
	UID    int               `json:"uid"`
	GID    int               `json:"gid"`
	Cwd    string            `json:"cwd"`
	Env    map[string]string `json:"env"`
	Links  []netlink.Link    `json:"links"`
	Addrs  []string          `json:"addrs"`
	Routes []ReportRoute     `json:"routes"`
	// AddRouteError is the text of the errno that adding a default route got, or "".
	AddRouteError string `json:"add_route_error"`
}

// ReportRoute is a route of a Report.
type ReportRoute struct {
	Family int    `json:"family"` // unix.AF_INET or unix.AF_INET6
	Dst    string `json:"dst"`
	Out    string `json:"out"` // the name of the link
}

// report prints what the application sees: who it is, where, and the network of the
// namespace; and it tries to change the routes, which it must not be allowed to do. When the
// variable GoFileVar is set, it first waits for that file.
func report([]string) error {
	if path := os.Getenv(GoFileVar); path != "" {
		waitForFile(path)
	}
	nl, err := netlink.Open()
	if err != nil {
		return err
	}
	defer func() { _ = nl.Close() }() // the probe is ending
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	r := Report{UID: os.Getuid(), GID: os.Getgid(), Cwd: cwd, Env: map[string]string{}}
	for _, name := range []string{"TUBE", "TUBE_PROFILE", "MOZ_NO_REMOTE"} {
		r.Env[name] = os.Getenv(name)
	}
	if r.Links, err = nl.Links(); err != nil {
		return err
	}
	names := map[int]string{}
	dev := 0
	for _, l := range r.Links {
		names[l.Index] = l.Name
		if l.Name == "tube0" {
			dev = l.Index
		}
	}
	if dev != 0 {
		addrs, err := nl.Addrs(dev)
		if err != nil {
			return err
		}
		for _, a := range addrs {
			r.Addrs = append(r.Addrs, a.String())
		}
		err = nl.AddDefaultRoute(dev, unix.AF_INET)
		var errno syscall.Errno
		if errors.As(err, &errno) {
			r.AddRouteError = errno.Error()
		} else if err != nil {
			r.AddRouteError = err.Error()
		}
	}
	routes, err := nl.Routes()
	if err != nil {
		return err
	}
	for _, rt := range routes {
		r.Routes = append(r.Routes, ReportRoute{Family: rt.Family, Dst: rt.Dst.String(), Out: names[rt.OutIndex]})
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}

// lookup resolves the name of its first argument with Go's resolver and prints the addresses,
// sorted and one on each line, or "error: " and the error.
func lookup(args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "sandboxtest: lookup takes one name, not %q\n", args)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := (&net.Resolver{PreferGo: true}).LookupHost(ctx, args[0])
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return nil
	}
	slices.Sort(addrs)
	fmt.Println(strings.Join(addrs, "\n"))
	return nil
}

// CoveredDirs are the folders that the probe files lists, the ones the keeper covers.
var CoveredDirs = []string{"/run/systemd/resolve", "/run/nscd", "/var/run/nscd", "/run/avahi-daemon"}

// Files is what the probe files prints, as one JSON object.
type Files struct {
	// Resolv is the content of each file the probe was given (/etc/resolv.conf when it was
	// given none), or "error: " and why it cannot be read.
	Resolv map[string]string `json:"resolv"`
	// Dirs are the names in each of CoveredDirs that exists.
	Dirs map[string][]string `json:"dirs"`
}

// files prints what the application sees of the files of the names.
func files(args []string) error {
	if len(args) == 0 {
		args = []string{"/etc/resolv.conf"}
	}
	f := Files{Resolv: map[string]string{}, Dirs: map[string][]string{}}
	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Resolv[path] = "error: " + err.Error()
			continue
		}
		f.Resolv[path] = string(data)
	}
	for _, dir := range CoveredDirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		f.Dirs[dir] = names
	}
	return json.NewEncoder(os.Stdout).Encode(f)
}

// PIDFileVar names the file into which the probe stubborn writes its pid.
const PIDFileVar = "TUBE_PROBE_PID_FILE"

// stubborn catches SIGTERM and does nothing with it, writes its pid into the file that
// PIDFileVar names, and sleeps for a minute: only SIGKILL ends it in time.
func stubborn([]string) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		for range sigs {
		}
	}()
	if err := os.WriteFile(os.Getenv(PIDFileVar), []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		return err
	}
	time.Sleep(time.Minute)
	return nil
}

// sigint prints whether SIGINT is ignored in the application: "true" or "false".
func sigint([]string) error {
	fmt.Println(signal.Ignored(syscall.SIGINT))
	return nil
}

// The variables that name the files through which a probe and its test talk.
const (
	// ReadyFileVar names the file that the probe writes when it has done its first part.
	ReadyFileVar = "TUBE_PROBE_READY_FILE"
	// GoFileVar names the file that the test makes when the probe may go on.
	GoFileVar = "TUBE_PROBE_GO_FILE"
	// OutFileVar names the file into which the probe writes its result.
	OutFileVar = "TUBE_PROBE_OUT_FILE"
)

// pingOnce says whether one ping to addr is answered within a second.
func pingOnce(addr string) bool {
	return exec.Command("ping", "-c", "1", "-W", "1", addr).Run() == nil
}

// result is "ok" or "failed".
func result(ok bool) string {
	if ok {
		return "ok"
	}
	return "failed"
}

// pingSteps pings each address of its arguments in turn, once. Before the ping number i (from
// 1) but the first, it waits (a minute at most) for the file of GoFileVar with ".<i>" added;
// after the ping number i it writes the file of ReadyFileVar with ".<i>" added. At the end it
// prints "1=<ok|failed> 2=<ok|failed> ...".
func pingSteps(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sandboxtest: ping-steps takes addresses")
		os.Exit(2)
	}
	var results []string
	for i, addr := range args {
		step := fmt.Sprint(i + 1)
		if i > 0 {
			waitForFile(os.Getenv(GoFileVar) + "." + step)
		}
		results = append(results, step+"="+result(pingOnce(addr)))
		if err := os.WriteFile(os.Getenv(ReadyFileVar)+"."+step, []byte("ready"), 0o600); err != nil {
			return err
		}
	}
	fmt.Println(strings.Join(results, " "))
	return nil
}

// waitForFile waits, a minute at most, until the file exists.
func waitForFile(path string) {
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
}

// linger catches SIGTERM and does nothing with it, writes "ready <pid>" into the file of
// ReadyFileVar, then looks every 50 ms whether tube0 still exists; when it is gone it writes the
// names of the links that are left into the file of OutFileVar, one on each line, and exits.
// It gives up after a minute.
func linger([]string) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		for range sigs {
		}
	}()
	if err := os.WriteFile(os.Getenv(ReadyFileVar), []byte(fmt.Sprintf("ready %d", os.Getpid())), 0o600); err != nil {
		return err
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := net.InterfaceByName("tube0"); err == nil {
			continue
		}
		ifaces, err := net.Interfaces()
		if err != nil {
			return err
		}
		var names []string
		for _, i := range ifaces {
			names = append(names, i.Name)
		}
		return os.WriteFile(os.Getenv(OutFileVar), []byte(strings.Join(names, "\n")+"\n"), 0o600)
	}
	return nil
}
