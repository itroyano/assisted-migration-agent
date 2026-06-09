package rightsizing

import (
	"net/url"
	"strings"
	"testing"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

func generateTestCert(t *testing.T) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestBuildSoapClient_NoCACert(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")
	sc, err := buildSoapClient(u, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected non-nil soap client")
	}
}

func TestBuildSoapClient_ValidCACert(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")
	caPEM := generateTestCert(t)

	sc, err := buildSoapClient(u, false, caPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport := sc.DefaultTransport()
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated")
	}
}

func TestBuildSoapClient_InvalidCACert(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")
	_, err := buildSoapClient(u, false, []byte("not a cert"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestBuildSoapClient_ErrorDoesNotLeakCertContent(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")
	sentinel := "SENTINELMARKER99"
	badPEM := []byte("-----BEGIN CERTIFICATE-----\n" + sentinel + "\n-----END CERTIFICATE-----\n")

	_, err := buildSoapClient(u, false, badPEM)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Errorf("error must not contain cert content; got: %v", err)
	}
}
