/*
Copyright Avast Software. All Rights Reserved.
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/activitylogger/mem"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/did"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/localkms"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/metricslogger/stderr"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/openid4ci"
	goapi "github.com/trustbloc/wallet-sdk/pkg/api"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/metricslogger"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/setup/oidc4ci"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/testenv"
)

type VPTestHelper struct {
	KMS    *localkms.KMS
	DIDDoc *api.DIDDocResolution
}

func NewVPTestHelper(t *testing.T, didMethod string, keyType string) *VPTestHelper {
	kms, err := localkms.NewKMS(localkms.NewMemKMSStore())
	require.NoError(t, err)

	// create DID
	c, err := did.NewCreator(kms)
	require.NoError(t, err)

	metricsLogger := stderr.NewMetricsLogger()

	opts := did.NewCreateOpts()
	opts.SetKeyType(keyType)
	opts.SetMetricsLogger(metricsLogger)

	didDoc, err := c.Create(didMethod, opts)
	require.NoError(t, err)

	return &VPTestHelper{
		KMS:    kms,
		DIDDoc: didDoc,
	}
}

func (h *VPTestHelper) IssueCredentials(t *testing.T, vcsAPIDirectURL string, issuerProfileIDs []string,
	claimData []map[string]interface{},
) *api.VerifiableCredentialsArray {
	oidc4ciSetup, err := oidc4ci.NewSetup(testenv.NewHttpRequest())
	require.NoError(t, err)

	err = oidc4ciSetup.AuthorizeIssuerBypassAuth("test_org", vcsAPIDirectURL)
	require.NoError(t, err)

	credentials := api.NewVerifiableCredentialsArray()

	for i := 0; i < len(issuerProfileIDs); i++ {
		offerCredentialURL, err := oidc4ciSetup.InitiatePreAuthorizedIssuance(issuerProfileIDs[i], claimData[i])
		require.NoError(t, err)

		opts := did.NewResolverOpts()
		opts.SetResolverServerURI("http://did-resolver.trustbloc.local:8072/1.0/identifiers")

		didResolver, err := did.NewResolver(opts)
		require.NoError(t, err)

		requiredArgs := openid4ci.NewArgs(offerCredentialURL, "ClientID", h.KMS.GetCrypto(),
			didResolver)

		interactionOptionalArgs := openid4ci.NewOpts()
		interactionOptionalArgs.SetMetricsLogger(stderr.NewMetricsLogger())

		interaction, err := openid4ci.NewInteraction(requiredArgs, interactionOptionalArgs)
		require.NoError(t, err)

		authorizeResult, err := interaction.Authorize()
		require.NoError(t, err)
		require.False(t, authorizeResult.UserPINRequired)

		vm, err := h.DIDDoc.AssertionMethod()
		require.NoError(t, err)

		result, err := interaction.RequestCredential(vm)
		require.NoError(t, err)
		require.NotEmpty(t, result)

		for i := 0; i < result.Length(); i++ {
			vc := result.AtIndex(i)

			serializedVC, err := vc.Serialize()
			require.NoError(t, err)

			println(serializedVC)
			credentials.Add(result.AtIndex(i))
		}

	}

	return credentials
}

func (h *VPTestHelper) CheckActivityLogAfterOpenID4VPFlow(t *testing.T, activityLogger *mem.ActivityLogger, verifierProfileID string) {
	numberOfActivitiesLogged := activityLogger.Length()
	require.Equal(t, 1, numberOfActivitiesLogged)

	activity := activityLogger.AtIndex(0)

	require.NotEmpty(t, activity.ID())
	require.Equal(t, goapi.LogTypeCredentialActivity, activity.Type())
	require.NotEmpty(t, activity.UnixTimestamp())
	require.Equal(t, verifierProfileID, activity.Client())
	require.Equal(t, "oidc-presentation", activity.Operation())
	require.Equal(t, goapi.ActivityLogStatusSuccess, activity.Status())
	require.Equal(t, 0, activity.Params().AllKeyValuePairs().Length())
}

func (h *VPTestHelper) CheckMetricsLoggerAfterOpenID4VPFlow(t *testing.T, metricsLogger *metricslogger.MetricsLogger) {
	require.Len(t, metricsLogger.Events, 4)

	h.checkFetchRequestObjectMetricsEvent(t, metricsLogger.Events[0])
	h.checkGetQueryMetricsEvent(t, metricsLogger.Events[1])
	h.checkSendAuthorizedResponseMetricsEvent(t, metricsLogger.Events[2])
	h.checkPresentCredentialMetricsEvent(t, metricsLogger.Events[3])
}

func (h *VPTestHelper) checkFetchRequestObjectMetricsEvent(t *testing.T, metricsEvent *api.MetricsEvent) {
	require.Contains(t, metricsEvent.Event(), "Fetch request object via an HTTP GET request to "+
		"http://localhost:8075/request-object/")
	require.Equal(t, "Get query", metricsEvent.ParentEvent())
	require.Positive(t, metricsEvent.DurationNanoseconds())
}

func (h *VPTestHelper) checkGetQueryMetricsEvent(t *testing.T, metricsEvent *api.MetricsEvent) {
	require.Equal(t, "Get query", metricsEvent.Event())
	require.Empty(t, metricsEvent.ParentEvent())
	require.Positive(t, metricsEvent.DurationNanoseconds())
}

func (h *VPTestHelper) checkSendAuthorizedResponseMetricsEvent(t *testing.T, metricsEvent *api.MetricsEvent) {
	require.Equal(t, "Send authorized response via an HTTP POST request to http://localhost:8075/oidc/present",
		metricsEvent.Event())
	require.Equal(t, "Present credential", metricsEvent.ParentEvent())
	require.Positive(t, metricsEvent.DurationNanoseconds())
}

func (h *VPTestHelper) checkPresentCredentialMetricsEvent(t *testing.T, metricsEvent *api.MetricsEvent) {
	require.Equal(t, "Present credential", metricsEvent.Event())
	require.Empty(t, metricsEvent.ParentEvent())
	require.Positive(t, metricsEvent.DurationNanoseconds())
}
