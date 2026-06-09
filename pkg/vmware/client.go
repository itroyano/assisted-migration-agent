package vmware

import (
	"context"
	"fmt"
	"net/url"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	"github.com/kubev2v/assisted-migration-agent/internal/models"
)

// NewVsphereClient creates and authenticates a govmomi client using the provided
// credentials. TLS behaviour is governed by creds.SkipTLS and creds.CACert.
// Callers must invoke creds.Validate() before calling this function.
func NewVsphereClient(ctx context.Context, creds *models.Credentials) (*govmomi.Client, error) {
	if creds == nil {
		return nil, fmt.Errorf("credentials must not be nil")
	}
	u, err := soap.ParseURL(creds.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	soapClient, err := NewSoapClient(u, creds.SkipTLS, creds.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create vim25 client: %w", err)
	}

	client := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	if err := client.Login(ctx, u.User); err != nil {
		return nil, fmt.Errorf("failed to login to vCenter: %w", err)
	}

	return client, nil
}
