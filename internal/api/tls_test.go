package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

// writeSelfSignedCert generates an ephemeral self-signed certificate and
// writes the PEM files to a temp directory, returning their paths.
func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vpc-proof-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func TestServerBootsInTLSMode(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)
	port := freeAPIPort(t)

	server, _, _ := newTestServer(t, func(cfg *config.Config) {
		cfg.Server.Addr = "127.0.0.1"
		cfg.Server.Port = port
		cfg.Server.TLSCertFile = certFile
		cfg.Server.TLSKeyFile = keyFile
	})

	if server.TLSCertFile() != certFile {
		t.Errorf("TLSCertFile() = %q, want %q", server.TLSCertFile(), certFile)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	waitAPIPort(t, port)

	// #nosec G402 -- test-only client that accepts the ephemeral certificate.
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/healthz", port))
	if err != nil {
		t.Fatalf("https request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz over TLS = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("ListenAndServe returned unexpected error: %v", err)
	}
}

func TestServerPlainHTTPWithoutTLS(t *testing.T) {
	port := freeAPIPort(t)
	server, _, _ := newTestServer(t, func(cfg *config.Config) {
		cfg.Server.Addr = "127.0.0.1"
		cfg.Server.Port = port
	})
	if server.TLSCertFile() != "" {
		t.Error("TLSCertFile should be empty when TLS is not configured")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	waitAPIPort(t, port)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz over plain HTTP = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	<-errCh
}
