package models_test

import (
	"strings"
	"testing"

	"github.com/kubev2v/assisted-migration-agent/internal/models"
)

func TestCredentials_Validate(t *testing.T) {
	validPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----\n")

	tests := []struct {
		name       string
		creds      models.Credentials
		wantErr    bool
		errMustNot string // substring that must NOT appear in the error (cert safety)
	}{
		{
			name:    "no TLS fields: valid",
			creds:   models.Credentials{URL: "https://vc/sdk", Username: "u", Password: "p"},
			wantErr: false,
		},
		{
			name:    "only SkipTLS true: valid",
			creds:   models.Credentials{SkipTLS: true},
			wantErr: false,
		},
		{
			name:    "only CACert provided: valid",
			creds:   models.Credentials{CACert: validPEM},
			wantErr: false,
		},
		{
			name:       "both SkipTLS and CACert: invalid",
			creds:      models.Credentials{SkipTLS: true, CACert: validPEM},
			wantErr:    true,
			errMustNot: "BEGIN CERTIFICATE", // cert content must not leak into error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.creds.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && tc.errMustNot != "" {
				if strings.Contains(err.Error(), tc.errMustNot) {
					t.Errorf("error message must not contain %q, got: %v", tc.errMustNot, err)
				}
			}
		})
	}
}

func TestCredentials_Validate_ErrorDoesNotLeakCertContent(t *testing.T) {
	// Use a cert with a distinctive marker string unlikely to appear in error prose.
	sentinel := "SENTINELMARKER99"
	fakePEM := []byte("-----BEGIN CERTIFICATE-----\n" + sentinel + "\n-----END CERTIFICATE-----\n")

	creds := models.Credentials{
		SkipTLS: true,
		CACert:  fakePEM,
	}
	err := creds.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Errorf("error message must not contain cert content; got: %v", err)
	}
}
