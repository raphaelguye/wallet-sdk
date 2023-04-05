/*
Copyright Avast Software. All Rights Reserved.
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/activitylogger/mem"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/credential"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/did"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/localkms"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/openid4vp"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/otel"
	"github.com/trustbloc/wallet-sdk/internal/testutil"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/helpers"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/metricslogger"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/setup/oidc4vp"
	"github.com/trustbloc/wallet-sdk/test/integration/pkg/testenv"
)

type claimData = map[string]interface{}

func TestOpenID4VPFullFlow(t *testing.T) {
	driverLicenseClaims := claimData{
		"birthdate":            "1990-01-01",
		"document_number":      "123-456-789",
		"driving_privileges":   "G2",
		"expiry_date":          "2025-05-26",
		"family_name":          "Smith",
		"given_name":           "John",
		"issue_date":           "2020-05-27",
		"issuing_authority":    "Ministry of Transport Ontario",
		"issuing_country":      "Canada",
		"resident_address":     "4726 Pine Street",
		"resident_city":        "Toronto",
		"resident_postal_code": "A1B 2C3",
		"resident_province":    "Ontario",
	}

	verifiableEmployeeClaims := claimData{
		"displayName":       "John Doe",
		"givenName":         "John",
		"jobTitle":          "Software Developer",
		"surname":           "Doe",
		"preferredLanguage": "English",
		"mail":              "john.doe@foo.bar",
		"photo":             "data-URL-encoded image",
	}

	universityDegreeClaims := map[string]interface{}{
		"familyName":   "John Doe",
		"givenName":    "John",
		"degree":       "MIT",
		"degreeSchool": "MIT school",
	}

	type test struct {
		issuerProfileIDs  []string
		claimData         []claimData
		walletDIDMethod   string
		verifierProfileID string
		signingKeyType    string
	}

	tests := []test{
		{
			issuerProfileIDs:  []string{"university_degree_issuer"},
			claimData:         []claimData{universityDegreeClaims},
			walletDIDMethod:   "ion",
			verifierProfileID: "v_ldp_university_degree",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer"},
			claimData:         []claimData{verifiableEmployeeClaims},
			walletDIDMethod:   "ion",
			verifierProfileID: "v_myprofile_jwt_verified_employee",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer"},
			claimData:         []claimData{verifiableEmployeeClaims},
			walletDIDMethod:   "key",
			verifierProfileID: "v_myprofile_jwt_verified_employee",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer"},
			claimData:         []claimData{verifiableEmployeeClaims},
			walletDIDMethod:   "jwk",
			verifierProfileID: "v_myprofile_jwt_verified_employee",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer_jwtsd"},
			claimData:         []claimData{verifiableEmployeeClaims},
			walletDIDMethod:   "jwk",
			verifierProfileID: "v_myprofile_sdjwt",
			signingKeyType:    localkms.KeyTypeP384,
		},
		{
			issuerProfileIDs:  []string{"drivers_license_issuer"},
			claimData:         []claimData{driverLicenseClaims},
			walletDIDMethod:   "ion",
			verifierProfileID: "v_myprofile_jwt_drivers_license",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer", "drivers_license_issuer"},
			claimData:         []claimData{verifiableEmployeeClaims, driverLicenseClaims},
			walletDIDMethod:   "ion",
			verifierProfileID: "v_myprofile_jwt_verified_employee",
		},
		{
			issuerProfileIDs:  []string{"bank_issuer", "bank_issuer"},
			claimData:         []claimData{verifiableEmployeeClaims, verifiableEmployeeClaims},
			walletDIDMethod:   "ion",
			verifierProfileID: "v_myprofile_jwt_verified_employee",
		},
	}

	var traceIDs []string

	for i, tc := range tests {
		fmt.Printf("running test %d: issuerProfileIDs=%s verifierProfileID=%s "+
			"walletDIDMethod=%s\n", i,
			tc.issuerProfileIDs, tc.verifierProfileID, tc.walletDIDMethod)

		testHelper := helpers.NewVPTestHelper(t, tc.walletDIDMethod, tc.signingKeyType)

		issuedCredentials := testHelper.IssueCredentials(t, vcsAPIDirectURL, tc.issuerProfileIDs, tc.claimData)
		println("Issued", issuedCredentials.Length(), "credentials")
		for k := 0; k < issuedCredentials.Length(); k++ {
			cred, _ := issuedCredentials.AtIndex(k).Serialize()
			println("Issued VC[", k, "]: ", cred)
		}

		setup := oidc4vp.NewSetup(testenv.NewHttpRequest())

		err := setup.AuthorizeVerifierBypassAuth("test_org", vcsAPIDirectURL)
		require.NoError(t, err)

		initiateURL, err := setup.InitiateInteraction(tc.verifierProfileID)
		require.NoError(t, err)

		opts := did.NewResolverOpts()
		opts.SetResolverServerURI(didResolverURL)

		didResolver, err := did.NewResolver(opts)
		require.NoError(t, err)

		activityLogger := mem.NewActivityLogger()

		docLoader := &documentLoaderReverseWrapper{DocumentLoader: testutil.DocumentLoader(t)}

		metricsLogger := metricslogger.NewMetricsLogger()

		trace, err := otel.NewTrace()
		require.NoError(t, err)
		println("traceID:", trace.TraceID())
		traceIDs = append(traceIDs, trace.TraceID())

		interactionRequiredArgs := openid4vp.NewArgs(initiateURL, testHelper.KMS.GetCrypto(), didResolver)

		interactionOptionalArgs := openid4vp.NewOpts()
		interactionOptionalArgs.SetDocumentLoader(docLoader)
		interactionOptionalArgs.SetActivityLogger(activityLogger)
		interactionOptionalArgs.SetMetricsLogger(metricsLogger)
		interactionOptionalArgs.AddHeader(trace.TraceHeader())

		interaction := openid4vp.NewInteraction(interactionRequiredArgs, interactionOptionalArgs)

		query, err := interaction.GetQuery()
		require.NoError(t, err)
		println("query", string(query))

		inquirerOpts := credential.NewInquirerOpts()
		inquirerOpts.SetDocumentLoader(docLoader)
		inquirer := credential.NewInquirer(inquirerOpts)
		require.NoError(t, err)

		requirements, err := inquirer.GetSubmissionRequirements(query, credential.NewCredentialsArgFromVCArray(issuedCredentials))
		require.NoError(t, err)
		require.GreaterOrEqual(t, requirements.Len(), 1)
		require.GreaterOrEqual(t, requirements.AtIndex(0).DescriptorLen(), 1)

		requirementDescriptor := requirements.AtIndex(0).DescriptorAtIndex(0)
		require.GreaterOrEqual(t, requirementDescriptor.MatchedVCs.Length(), 1)

		selectedCreds := api.NewVerifiableCredentialsArray()
		selectedCreds.Add(requirementDescriptor.MatchedVCs.AtIndex(0))

		verifiablePres, err := inquirer.Query(query, credential.NewCredentialsArgFromVCArray(selectedCreds))
		require.NoError(t, err)

		matchedCreds, err := verifiablePres.Credentials()
		require.NoError(t, err)

		require.Equal(t, 1, matchedCreds.Length())

		serializedIssuedVC, err := issuedCredentials.AtIndex(0).Serialize()
		require.NoError(t, err)

		serializedMatchedVC, err := matchedCreds.AtIndex(0).Serialize()
		require.NoError(t, err)
		println(serializedMatchedVC)

		require.Equal(t, serializedIssuedVC, serializedMatchedVC)

		err = interaction.PresentCredential(selectedCreds)
		require.NoError(t, err)

		testHelper.CheckActivityLogAfterOpenID4VPFlow(t, activityLogger, tc.verifierProfileID)
		testHelper.CheckMetricsLoggerAfterOpenID4VPFlow(t, metricsLogger)

		fmt.Printf("done test %d\n", i)
	}

	time.Sleep(5 * time.Second)
	for _, traceID := range traceIDs {
		_, err := testenv.NewHttpRequest().Send(http.MethodGet,
			queryTraceURL+traceID,
			"",
			nil,
			nil,
			nil,
		)
		require.NoError(t, err)
	}
}
