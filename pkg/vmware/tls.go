package vmware

import (
	"crypto/x509"
	"errors"
	"net/url"

	"github.com/vmware/govmomi/vim25/soap"
)

// NewSoapClient creates a govmomi SOAP client with TLS configured from the
// provided parameters. Pass insecure=true only when the caller has explicitly
// opted out of certificate verification (SkipTLS=true on Credentials).
// If caCert is non-empty it must be a PEM-encoded certificate bundle; it is
// loaded into the client's root CA pool. caCert content is never logged.
func NewSoapClient(u *url.URL, insecure bool, caCert []byte) (*soap.Client, error) {
	sc := soap.NewClient(u, insecure)
	if len(caCert) == 0 {
		return sc, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("invalid CA bundle: no valid PEM certificates found")
	}
	// soap.NewClient always initialises TLSClientConfig; SetRootCAs() is not used
	// because it loads from file paths, not in-memory PEM bytes.
	transport := sc.DefaultTransport()
	transport.TLSClientConfig.RootCAs = pool
	return sc, nil
}
