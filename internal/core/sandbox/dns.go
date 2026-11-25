// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"golang.org/x/sys/unix"
)

// The Tags of the keeper's DNS files.
const (
	TagKeeperCoverFailed       = "KeeperCoverFailed"
	TagKeeperResolvLinkFailed  = "KeeperResolvLinkFailed"
	TagKeeperResolvMountFailed = "KeeperResolvMountFailed"
	TagKeeperResolvWriteFailed = "KeeperResolvWriteFailed"
	TagKeeperNoTempDir         = "KeeperNoTempDir"
)

// The limits of the C library's resolver: three servers, six search domains.
const (
	maxNameservers   = 3
	maxSearchDomains = 6
	maxResolvLinks   = 8
)

// The variables through which Start tells the keeper its folder and, for tests, the file that
// stands for /etc/resolv.conf. Start removes them from the environment it is given, and the
// keeper from the application's.
const (
	keeperVarPrefix = "TUBE_KEEPER_"
	keeperTmpVar    = keeperVarPrefix + "TMP"
	keeperResolvVar = keeperVarPrefix + "RESOLV"
)

// resolvConfPath is the file of the C library's resolver.
const resolvConfPath = "/etc/resolv.conf"

// coveredDirs are the folders through which the C library reaches a resolver of the machine
// (docs/SPEC.md §8.4). Each one that exists is covered with an empty tmpfs.
var coveredDirs = []string{"/run/systemd/resolve", "/run/nscd", "/var/run/nscd", "/run/avahi-daemon"}

// The warnings of a configure about the names.
const (
	warnNoDNSServer  = "the VPN gave no DNS server: names will not be found; set DNS servers for the profile"
	warnNoResolvConf = "this machine has no resolv.conf: names will not be found"
)

// ResolvConf is the content of resolv.conf for a run: a comment line that says Tube wrote it,
// a nameserver line for each of the first 3 servers, a search line when there are search
// domains (the first 6), and "options edns0". With no server it has a comment that says that
// the VPN gave none, and no nameserver line. A server with a zone and a search domain that is
// not a valid host name are dropped: this is the last guard before the file.
func ResolvConf(dns []netip.Addr, search []string) []byte {
	var b bytes.Buffer
	b.WriteString("# Written by Tube for the application that runs in the VPN.\n")
	servers := 0
	for _, a := range dns {
		if !a.IsValid() || a.Zone() != "" || servers == maxNameservers {
			continue
		}
		b.WriteString("nameserver " + a.Unmap().String() + "\n")
		servers++
	}
	if servers == 0 {
		b.WriteString("# The VPN gave no DNS server: names will not be found.\n")
	}
	var domains []string
	for _, d := range search {
		if sanitize.HostName(d) && len(domains) < maxSearchDomains {
			domains = append(domains, d)
		}
	}
	if len(domains) > 0 {
		b.WriteString("search " + strings.Join(domains, " ") + "\n")
	}
	b.WriteString("options edns0\n")
	return b.Bytes()
}

// dnsFiles are the files of the names in the keeper's mount namespace. The first apply finds
// where resolv.conf leads and covers the resolvers of the machine; every apply writes the file.
type dnsFiles struct {
	// resolvPath is /etc/resolv.conf, or the file of a test.
	resolvPath string
	// tmpDir is the keeper's folder, for a file that is bind-mounted; "" when there is none.
	tmpDir string

	ready bool
	// target is where resolvPath leads, with its folder's links resolved.
	target string
	// covered are the folders that have been covered, links resolved.
	covered []string
	// bound is the file that is bind-mounted over target, once it is.
	bound string
}

// newDNSFiles are the DNS files of the keeper, from its environment.
func newDNSFiles() *dnsFiles {
	d := &dnsFiles{resolvPath: resolvConfPath, tmpDir: os.Getenv(keeperTmpVar)}
	if path := os.Getenv(keeperResolvVar); path != "" {
		d.resolvPath = path
	}
	return d
}

// apply makes resolv.conf, as the application sees it, say the servers and the search domains.
func (d *dnsFiles) apply(dns []netip.Addr, search []string) (warnings []string, err error) {
	if !d.ready {
		// Where resolv.conf leads must be known before anything is covered.
		if d.target, err = followLinks(d.resolvPath); err != nil {
			return nil, err
		}
		for _, dir := range coveredDirs {
			if err := d.cover(dir); err != nil {
				return nil, err
			}
		}
		d.ready = true
	}
	content := ResolvConf(dns, search)
	if !slices.ContainsFunc(dns, func(a netip.Addr) bool { return a.IsValid() && a.Zone() == "" }) {
		warnings = append(warnings, warnNoDNSServer)
	}
	switch {
	case d.inCovered():
		err = writeResolv(d.target, content, true)
	case d.bound != "":
		// Rewritten in place: the mount shows this file (trap 16).
		err = writeResolv(d.bound, content, false)
	case exists(d.target):
		err = d.bind(content)
	default:
		warnings = append(warnings, warnNoResolvConf)
	}
	return warnings, err
}

// exists says whether there is a file at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// followLinks follows the symbolic links of path by hand, at most 8 times, so that the answer
// is a path even when the last target does not exist; the folder of the answer has its links
// resolved, when it exists.
func followLinks(path string) (string, error) {
	target, reason := followLinksText(path)
	if reason != "" {
		return "", errors.New(TagKeeperResolvLinkFailed, "where resolv.conf leads cannot be known", "path", path, "reason", reason)
	}
	return target, nil
}

// followLinksText is followLinks, which says why it failed instead of failing.
func followLinksText(path string) (target, reason string) {
	for range maxResolvLinks {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", err.Error()
		}
		if info.Mode()&os.ModeSymlink == 0 {
			break
		}
		next, err := os.Readlink(path)
		if err != nil {
			return "", err.Error()
		}
		if !filepath.IsAbs(next) {
			next = filepath.Join(filepath.Dir(path), next)
		}
		path = filepath.Clean(next)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", "more than 8 symbolic links"
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(dir, filepath.Base(path))
	}
	return path, ""
}

// cover covers the folder with an empty tmpfs, when it exists and is not covered already.
func (d *dnsFiles) cover(dir string) error {
	real, err := filepath.EvalSymlinks(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil && slices.Contains(d.covered, real) {
		return nil
	}
	if err == nil {
		err = unix.Mount("tmpfs", real, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "size=64k,mode=0755")
	}
	if err != nil {
		return errors.NewErr(TagKeeperCoverFailed, err).SetMetadata("dir", dir)
	}
	d.covered = append(d.covered, real)
	return nil
}

// inCovered says whether resolv.conf leads into a covered folder.
func (d *dnsFiles) inCovered() bool {
	return slices.ContainsFunc(d.covered, func(dir string) bool {
		return strings.HasPrefix(d.target, dir+string(filepath.Separator))
	})
}

// bind writes the content into the keeper's folder and mounts that file over resolv.conf.
func (d *dnsFiles) bind(content []byte) error {
	if d.tmpDir == "" {
		return errors.New(TagKeeperNoTempDir, "the keeper has no folder for resolv.conf")
	}
	file := filepath.Join(d.tmpDir, "resolv.conf")
	if err := writeResolv(file, content, false); err != nil {
		return err
	}
	if err := unix.Mount(file, d.target, "", unix.MS_BIND, ""); err != nil {
		return errors.NewErr(TagKeeperResolvMountFailed, err).SetMetadata("target", d.target)
	}
	d.bound = file
	return nil
}

// writeResolv writes the content into the file, in place (an existing file keeps its inode), with
// mode 0644; with mkdir, it makes the file's folder first.
func writeResolv(path string, content []byte, mkdir bool) error {
	var err error
	if mkdir {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err == nil {
		err = os.WriteFile(path, content, 0o644)
	}
	if err != nil {
		return errors.NewErr(TagKeeperResolvWriteFailed, err).SetMetadata("path", path)
	}
	return nil
}
