package api

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestRequestUsesHTTPSFromTransportOrReverseProxy(t *testing.T) {
	plain := httptest.NewRequest("GET", "http://buildworld.test/", nil)
	if requestUsesHTTPS(plain) {
		t.Fatal("plain HTTP request was treated as HTTPS")
	}

	directTLS := httptest.NewRequest("GET", "https://buildworld.test/", nil)
	directTLS.TLS = &tls.ConnectionState{}
	if !requestUsesHTTPS(directTLS) {
		t.Fatal("TLS request was not treated as HTTPS")
	}

	proxied := httptest.NewRequest("GET", "http://buildworld.test/", nil)
	proxied.Header.Set("X-Forwarded-Proto", "https, http")
	if !requestUsesHTTPS(proxied) {
		t.Fatal("HTTPS reverse-proxy request was not treated as HTTPS")
	}
}
