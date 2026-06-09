package collector

import (
	"testing"

	"github.com/kubev2v/assisted-migration-agent/internal/models"
)

func TestCreateSecret_DefaultsToVerifiedTLS(t *testing.T) {
	creds := &models.Credentials{
		Username: "admin",
		Password: "secret",
	}
	s := createSecret(creds)

	got := string(s.Data["insecureSkipVerify"])
	if got != "false" {
		t.Errorf("insecureSkipVerify: want %q, got %q", "false", got)
	}
	if _, ok := s.Data["ca.crt"]; ok {
		t.Error("ca.crt must not be present when CACert is empty")
	}
}

func TestCreateSecret_SkipTLS(t *testing.T) {
	creds := &models.Credentials{
		Username: "admin",
		Password: "secret",
		SkipTLS:  true,
	}
	s := createSecret(creds)

	got := string(s.Data["insecureSkipVerify"])
	if got != "true" {
		t.Errorf("insecureSkipVerify: want %q, got %q", "true", got)
	}
}

func TestCreateSecret_WithCACert(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	creds := &models.Credentials{
		Username: "admin",
		Password: "secret",
		CACert:   caPEM,
	}
	s := createSecret(creds)

	got := string(s.Data["insecureSkipVerify"])
	if got != "false" {
		t.Errorf("insecureSkipVerify: want %q, got %q", "false", got)
	}

	gotCACert := s.Data["ca.crt"]
	if string(gotCACert) != string(caPEM) {
		t.Errorf("ca.crt: want %q, got %q", caPEM, gotCACert)
	}
}

func TestCreateSecret_CACertNotInInsecureKey(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----\n")
	creds := &models.Credentials{CACert: caPEM}
	s := createSecret(creds)

	// The cert must only be in "ca.crt", not leaked into other fields.
	for k, v := range s.Data {
		if k == "ca.crt" {
			continue
		}
		if string(v) == string(caPEM) {
			t.Errorf("cert content leaked into secret key %q", k)
		}
	}
}
