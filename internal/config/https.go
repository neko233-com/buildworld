package config

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type TLSConfig struct {
	Enabled   bool   `json:"enabled"`
	CertFile  string `json:"cert_file"`
	KeyFile   string `json:"key_file"`
	Domain    string `json:"domain,omitempty"`
	Email     string `json:"email,omitempty"`
	AutoRenew bool   `json:"auto_renew"`
}

type ProxysssConfig struct {
	Enabled bool   `json:"enabled"`
	Domain  string `json:"domain"`
	Email   string `json:"email"`
}

type HTTPSManager struct {
	mu          sync.RWMutex
	tlsConfig   *TLSConfig
	proxyConfig *ProxysssConfig
	workspace   string
	certDir     string
}

func NewHTTPSManager(workspace string) *HTTPSManager {
	return &HTTPSManager{
		tlsConfig: &TLSConfig{
			Enabled:   false,
			AutoRenew: true,
		},
		proxyConfig: &ProxysssConfig{
			Enabled: false,
		},
		workspace: workspace,
		certDir:   filepath.Join(workspace, "certs"),
	}
}

func (m *HTTPSManager) Setup(domain, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	domain = strings.TrimSpace(domain)
	email = strings.TrimSpace(email)

	// Create cert directory
	if err := os.MkdirAll(m.certDir, 0755); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}

	// Local hosts and incomplete ACME requests must use a self-signed
	// certificate even when an unrelated proxysss binary is on PATH.
	if email != "" && domain != "" && !strings.EqualFold(domain, "localhost") && m.isProxysssAvailable() {
		return m.setupProxysss(domain, email)
	}

	// Fallback to self-signed certificate
	return m.setupSelfSigned(domain)
}

func (m *HTTPSManager) isProxysssAvailable() bool {
	_, err := exec.LookPath("proxysss")
	return err == nil
}

func (m *HTTPSManager) setupProxysss(domain, email string) error {
	// Use proxysss to get Let's Encrypt certificate
	cmd := exec.Command("proxysss", "certonly", "--webroot",
		"--webroot-path", m.certDir,
		"-d", domain,
		"--email", email,
		"--agree-tos",
		"--non-interactive",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("proxysss certonly failed: %w, output: %s", err, output)
	}

	// Update config
	m.proxyConfig.Enabled = true
	m.proxyConfig.Domain = domain
	m.proxyConfig.Email = email

	m.tlsConfig.Enabled = true
	m.tlsConfig.Domain = domain
	m.tlsConfig.Email = email
	m.tlsConfig.CertFile = filepath.Join(m.certDir, domain, "fullchain.pem")
	m.tlsConfig.KeyFile = filepath.Join(m.certDir, domain, "privkey.pem")

	return nil
}

func (m *HTTPSManager) setupSelfSigned(domain string) error {
	// Generate self-signed certificate
	certFile := filepath.Join(m.certDir, "server.crt")
	keyFile := filepath.Join(m.certDir, "server.key")

	// Generate key
	cmd := exec.Command("openssl", "genrsa", "-out", keyFile, "2048")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generate key: %w, output: %s", err, output)
	}

	// Generate certificate
	cmd = exec.Command("openssl", "req", "-new", "-x509",
		"-key", keyFile,
		"-out", certFile,
		"-days", "365",
		"-subj", fmt.Sprintf("/CN=%s", domain),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generate cert: %w, output: %s", err, output)
	}

	m.tlsConfig.Enabled = true
	m.tlsConfig.Domain = domain
	m.tlsConfig.CertFile = certFile
	m.tlsConfig.KeyFile = keyFile

	return nil
}

func (m *HTTPSManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.tlsConfig.Enabled {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(m.tlsConfig.CertFile, m.tlsConfig.KeyFile)
	if err != nil {
		log.Printf("Failed to load TLS certificate: %v", err)
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

func (m *HTTPSManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsConfig.Enabled
}

func (m *HTTPSManager) GetDomain() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsConfig.Domain
}

func (m *HTTPSManager) RenewCertificates() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.proxyConfig.Enabled {
		return nil
	}

	// Use proxysss to renew certificates
	cmd := exec.Command("proxysss", "renew", "--non-interactive")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certificate renewal failed: %w, output: %s", err, output)
	}

	return nil
}

func (m *HTTPSManager) StartRenewalTicker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if err := m.RenewCertificates(); err != nil {
				log.Printf("Certificate renewal failed: %v", err)
			}
		}
	}()
}

// LoadTLSConfig loads TLS configuration from certificate files
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// GenerateSelfSignedCert generates a self-signed certificate for development
func GenerateSelfSignedCert(domain, outputDir string) (certFile, keyFile string, err error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}

	certFile = filepath.Join(outputDir, "server.crt")
	keyFile = filepath.Join(outputDir, "server.key")

	// Generate key
	cmd := exec.Command("openssl", "genrsa", "-out", keyFile, "2048")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("generate key: %w, output: %s", err, output)
	}

	// Generate certificate
	cmd = exec.Command("openssl", "req", "-new", "-x509",
		"-key", keyFile,
		"-out", certFile,
		"-days", "365",
		"-subj", fmt.Sprintf("/CN=%s", domain),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("generate cert: %w, output: %s", err, output)
	}

	return certFile, keyFile, nil
}
