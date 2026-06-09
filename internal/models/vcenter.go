package models

import "errors"

// Credentials holds vCenter connection credentials and TLS options.
type Credentials struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	// CACert is a PEM-encoded CA certificate bundle used to verify the vCenter
	// TLS certificate. Mutually exclusive with SkipTLS.
	CACert []byte `json:"cacert,omitempty"`
	// SkipTLS disables TLS certificate verification. Mutually exclusive with CACert.
	// Default false — verified TLS is the secure default.
	SkipTLS bool `json:"skipTls"`
}

// Validate returns an error if the TLS fields are in a conflicting state.
// Call this at every service entry point before making any vCenter connection.
func (c Credentials) Validate() error {
	if c.SkipTLS && len(c.CACert) > 0 {
		return errors.New("skipTls and cacert are mutually exclusive: set skipTls=true for unverified TLS or provide a cacert for verified TLS, not both")
	}
	return nil
}
