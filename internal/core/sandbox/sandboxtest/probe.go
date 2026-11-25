// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandboxtest

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

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
		"report": report,
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
// namespace; and it tries to change the routes, which it must not be allowed to do.
func report([]string) error {
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
