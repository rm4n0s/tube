// SPDX-License-Identifier: GPL-2.0-only

// Package profilestest makes OpenVPN client profiles and the keys they need, for tests. The keys
// are made in the test's own temporary folder each time; nothing secret is committed.
package profilestest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ServerName is the DNS name of the server certificate.
const ServerName = "tube-test-server"

// PKI is a certificate authority with one server and one client certificate, made for a test.
// Every field is PEM.
type PKI struct {
	CA, ServerCert, ServerKey, ClientCert, ClientKey []byte
}

// NewPKI makes a certificate authority, a server certificate and a client certificate, with
// ECDSA P-256 keys, valid from an hour ago for a day.
func NewPKI(t testing.TB) *PKI {
	t.Helper()
	now := time.Now()
	caKey := newKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          newSerial(t),
		Subject:               pkix.Name{CommonName: "tube test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("profilestest: the CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("profilestest: the CA: %v", err)
	}
	leaf := func(name string, usage x509.ExtKeyUsage) ([]byte, []byte) {
		key := newKey(t)
		template := &x509.Certificate{
			SerialNumber:          newSerial(t),
			Subject:               pkix.Name{CommonName: name},
			DNSNames:              []string{name},
			NotBefore:             now.Add(-time.Hour),
			NotAfter:              now.Add(24 * time.Hour),
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatalf("profilestest: the certificate of %s: %v", name, err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("profilestest: the key of %s: %v", name, err)
		}
		return pemBlock("CERTIFICATE", der), pemBlock("PRIVATE KEY", keyDER)
	}
	p := &PKI{CA: pemBlock("CERTIFICATE", caDER)}
	p.ServerCert, p.ServerKey = leaf(ServerName, x509.ExtKeyUsageServerAuth)
	p.ClientCert, p.ClientKey = leaf("tube-test-client", x509.ExtKeyUsageClientAuth)
	return p
}

// newKey makes an ECDSA P-256 key.
func newKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("profilestest: a key: %v", err)
	}
	return key
}

// newSerial makes a random serial number of a certificate.
func newSerial(t testing.TB) *big.Int {
	t.Helper()
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 63))
	if err != nil {
		t.Fatalf("profilestest: a serial number: %v", err)
	}
	return n
}

// pemBlock encodes DER as one PEM block.
func pemBlock(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

// Options say what the profile of a test has.
type Options struct {
	// Remote is the argument of the remote option: "127.0.0.1 1194" by default.
	Remote string
	// Proto is "udp" (the default) or "tcp".
	Proto string
	// Password adds auth-user-pass, without a file: the profile asks for a user name and a
	// password.
	Password bool
	// Separate writes the ca, the cert and the key as the files ca.crt, client.crt and
	// client.key next to the profile, and names them; else they are inside the profile.
	Separate bool
	// Extra are more lines, written at the end.
	Extra []string
}

// WriteProfile writes a client profile called name into dir (with the files next to it when
// opts.Separate) and returns its path. The files have mode 0600.
func WriteProfile(t testing.TB, dir, name string, pki *PKI, opts Options) string {
	t.Helper()
	remote := opts.Remote
	if remote == "" {
		remote = "127.0.0.1 1194"
	}
	proto := opts.Proto
	if proto == "" {
		proto = "udp"
	}
	lines := []string{
		"client",
		"dev tun",
		"proto " + proto,
		"remote " + remote,
		"nobind",
		"remote-cert-tls server",
	}
	if opts.Separate {
		write(t, filepath.Join(dir, "ca.crt"), pki.CA)
		write(t, filepath.Join(dir, "client.crt"), pki.ClientCert)
		write(t, filepath.Join(dir, "client.key"), pki.ClientKey)
		lines = append(lines, "ca ca.crt", "cert client.crt", "key client.key")
	} else {
		lines = append(lines,
			"<ca>", strings.TrimSpace(string(pki.CA)), "</ca>",
			"<cert>", strings.TrimSpace(string(pki.ClientCert)), "</cert>",
			"<key>", strings.TrimSpace(string(pki.ClientKey)), "</key>")
	}
	if opts.Password {
		lines = append(lines, "auth-user-pass")
	}
	lines = append(lines, opts.Extra...)
	path := filepath.Join(dir, name)
	write(t, path, []byte(strings.Join(lines, "\n")+"\n"))
	return path
}

// write writes a file with mode 0600, making its folder.
func write(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("profilestest: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("profilestest: %v", err)
	}
}
