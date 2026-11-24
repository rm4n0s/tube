// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package nstest runs a function of a test inside a new user and network namespace, where it
// is uid 0 and has its own empty network: the test binary starts itself again in the namespace
// and runs only that function. It needs no root; a machine that does not allow user namespaces
// skips the test.
package nstest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// funcVar is the variable that names the function that the test binary runs.
const funcVar = "TUBE_NSTEST_FUNC"

// probe is the function that only shows that a namespace can be made.
const probe = "nstest-probe"

// timeout is how long a function may run.
const timeout = 30 * time.Second

// Main is the TestMain of a package whose tests run functions inside a namespace. When the
// process was started by Run, it calls the function and exits; else it runs the tests.
func Main(m *testing.M, funcs map[string]func() error) {
	if name, ok := os.LookupEnv(funcVar); ok {
		os.Exit(runFunc(name, funcs))
	}
	os.Exit(m.Run())
}

// runFunc runs the function called name and returns the exit status of the process.
func runFunc(name string, funcs map[string]func() error) int {
	if name == probe {
		return 0
	}
	f, ok := funcs[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "nstest: no function called %q\n", name)
		return 2
	}
	if err := f(); err != nil {
		fmt.Fprintf(os.Stderr, "nstest: %s: %v\n", name, err)
		return 1
	}
	return 0
}

// command is the test binary started in a new user and network namespace, mapped to uid 0.
func command(ctx context.Context, name string, env []string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(append(os.Environ(), funcVar+"="+name), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}

// Require skips the test when this machine does not let the user make such a namespace.
func Require(t testing.TB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd, err := command(ctx, probe, nil)
	if err == nil {
		err = cmd.Run()
	}
	if err != nil {
		t.Skipf("this machine does not allow user namespaces: %v", err)
	}
}

// Run runs the function called name, in a new process of the test binary inside a new user
// and network namespace where it is uid 0, with the given variables ("NAME=value") added to
// its environment, and returns what it wrote to standard output. It fails the test, with the
// process's output, when the function returns an error or does not end in 30 seconds.
func Run(t testing.TB, name string, env ...string) string {
	t.Helper()
	Require(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd, err := command(ctx, name, env)
	if err != nil {
		t.Fatalf("nstest: %v", err)
		return ""
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("nstest: the function %s failed: %v\n--- standard output:\n%s--- standard error:\n%s", name, err, stdout.String(), stderr.String())
		return ""
	}
	return stdout.String()
}
