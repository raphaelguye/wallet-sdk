/*
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package display

import (
	"time"

	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"
)

// Opts contains all optional arguments that can be passed into the Resolve function.
type Opts struct {
	preferredLocale                  string
	metricsLogger                    api.MetricsLogger
	additionalHeaders                api.Headers
	httpTimeout                      *time.Duration
	disableHTTPClientTLSVerification bool
}

// NewOpts returns a new Opts object.
func NewOpts() *Opts {
	return &Opts{}
}

// SetPreferredLocale sets the preferred locale to use while resolving VC display data.
// If the preferred locale is not available (or no preferred locale is specified), then the first locale specified by
// the issuer's metadata will be used during resolution.
// The actual locales used for various pieces of display information are available in the Data object.
func (o *Opts) SetPreferredLocale(preferredLocale string) *Opts {
	o.preferredLocale = preferredLocale

	return o
}

// SetMetricsLogger sets a metrics logger to use.
func (o *Opts) SetMetricsLogger(metricsLogger api.MetricsLogger) *Opts {
	o.metricsLogger = metricsLogger

	return o
}

// SetHTTPTimeoutNanoseconds sets the timeout (in nanoseconds) for HTTP calls.
// Passing in 0 will disable timeouts.
func (o *Opts) SetHTTPTimeoutNanoseconds(timeout int64) *Opts {
	timeoutDuration := time.Duration(timeout)
	o.httpTimeout = &timeoutDuration

	return o
}

// AddHeaders adds the given HTTP headers to all REST calls made to the issuer during display resolution.
func (o *Opts) AddHeaders(headers *api.Headers) *Opts {
	headersAsArray := headers.GetAll()

	for i := range headersAsArray {
		o.additionalHeaders.Add(&headersAsArray[i])
	}

	return o
}

// DisableHTTPClientTLSVerify disables TLS verification. Should be used for testing purposes only.
func (o *Opts) DisableHTTPClientTLSVerify() *Opts {
	o.disableHTTPClientTLSVerification = true

	return o
}
