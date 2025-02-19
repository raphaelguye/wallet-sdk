/*
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package openid4vp

import (
	"time"

	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"
)

// Opts contains all optional arguments that can be passed into the NewInteraction function.
type Opts struct {
	documentLoader                   api.LDDocumentLoader
	activityLogger                   api.ActivityLogger
	metricsLogger                    api.MetricsLogger
	additionalHeaders                api.Headers
	disableHTTPClientTLSVerification bool
	disableOpenTelemetry             bool
	httpTimeout                      *time.Duration
}

// NewOpts returns a new Opts object.
func NewOpts() *Opts {
	return &Opts{}
}

// SetDocumentLoader sets a document loader to use.
func (o *Opts) SetDocumentLoader(documentLoader api.LDDocumentLoader) *Opts {
	o.documentLoader = documentLoader

	return o
}

// SetActivityLogger sets an activity logger to use.
func (o *Opts) SetActivityLogger(activityLogger api.ActivityLogger) *Opts {
	o.activityLogger = activityLogger

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

// AddHeaders adds the given HTTP headers to all REST calls made to the verifier during the OpenID4VP flow.
func (o *Opts) AddHeaders(headers *api.Headers) *Opts {
	headersAsArray := headers.GetAll()

	for i := range headersAsArray {
		o.additionalHeaders.Add(&headersAsArray[i])
	}

	return o
}

// AddHeader adds the given HTTP header to all REST calls made to the issuer during the OpenID4CI flow.
func (o *Opts) AddHeader(header *api.Header) *Opts {
	o.additionalHeaders.Add(header)

	return o
}

// DisableHTTPClientTLSVerify disables tls verification, should be used only for test purposes.
func (o *Opts) DisableHTTPClientTLSVerify() *Opts {
	o.disableHTTPClientTLSVerification = true

	return o
}

// DisableOpenTelemetry disables sending of open telemetry header.
func (o *Opts) DisableOpenTelemetry() *Opts {
	o.disableOpenTelemetry = true

	return o
}
