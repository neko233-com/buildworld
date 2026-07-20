package config

import (
	"os"
	"os/exec"
	"testing"
)

func TestHTTPSManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "https-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewHTTPSManager(tmpDir)

	if m.IsEnabled() {
		t.Error("HTTPS should be disabled by default")
	}

	if m.GetDomain() != "" {
		t.Error("Domain should be empty by default")
	}
}

func TestHTTPSManagerSelfSigned(t *testing.T) {
	// Skip if openssl not available
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available, skipping test")
	}

	tmpDir, err := os.MkdirTemp("", "https-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewHTTPSManager(tmpDir)

	// Setup self-signed certificate
	err = m.Setup("localhost", "")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if !m.IsEnabled() {
		t.Error("HTTPS should be enabled after setup")
	}

	if m.GetDomain() != "localhost" {
		t.Errorf("Domain = %s, want localhost", m.GetDomain())
	}

	// Verify TLS config can be created
	tlsConfig := m.GetTLSConfig()
	if tlsConfig == nil {
		t.Error("GetTLSConfig() returned nil")
	}
}

func TestHTTPSManagerGenerateSelfSigned(t *testing.T) {
	// Skip if openssl not available
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available, skipping test")
	}

	tmpDir, err := os.MkdirTemp("", "https-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	certFile, keyFile, err := GenerateSelfSignedCert("test.local", tmpDir)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Error("Certificate file not created")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Error("Key file not created")
	}
}

func TestHTTPSManagerRenewal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "https-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewHTTPSManager(tmpDir)

	// Renewal should not fail when not configured
	err = m.RenewCertificates()
	if err != nil {
		t.Errorf("RenewCertificates() error = %v", err)
	}
}

func TestLoadTLSConfig(t *testing.T) {
	// Skip if openssl not available
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available, skipping test")
	}

	tmpDir, err := os.MkdirTemp("", "https-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate test certificate
	certFile, keyFile, err := GenerateSelfSignedCert("test.local", tmpDir)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	// Load TLS config
	tlsConfig, err := LoadTLSConfig(certFile, keyFile)
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	if tlsConfig == nil {
		t.Error("LoadTLSConfig() returned nil")
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("Certificates length = %d, want 1", len(tlsConfig.Certificates))
	}
}
