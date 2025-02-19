/*
Copyright Avast Software. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package openid4vp implements the OpenID4VP presentation flow.
package openid4vp

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/aries-framework-go/component/kmscrypto/doc/jose"
	diddoc "github.com/hyperledger/aries-framework-go/component/models/did"
	"github.com/hyperledger/aries-framework-go/component/models/jwt"
	"github.com/hyperledger/aries-framework-go/component/models/presexch"
	"github.com/hyperledger/aries-framework-go/component/models/verifiable"
	vdrspi "github.com/hyperledger/aries-framework-go/spi/vdr"
	"github.com/piprate/json-gold/ld"

	"github.com/trustbloc/wallet-sdk/pkg/api"
	"github.com/trustbloc/wallet-sdk/pkg/common"
	"github.com/trustbloc/wallet-sdk/pkg/internal/httprequest"
	"github.com/trustbloc/wallet-sdk/pkg/models"
	"github.com/trustbloc/wallet-sdk/pkg/walleterror"
)

const (
	requestURIPrefix = "openid-vc://?request_uri="
	tokenLiveTimeSec = 600

	activityLogOperation = "oidc-presentation"

	getQueryEventText           = "Get query"
	fetchRequestObjectEventText = "Fetch request object via an HTTP GET request to %s"

	presentCredentialEventText      = "Present credential" //nolint:gosec // false positive
	sendAuthorizedResponseEventText = "Send authorized response via an HTTP POST request to %s"
)

type jwtSignatureVerifier interface {
	Verify(joseHeaders jose.Headers, payload, signingInput, signature []byte) error
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Interaction is used to help with OpenID4VP operations.
type Interaction struct {
	authorizationRequest string
	signatureVerifier    jwtSignatureVerifier
	httpClient           httpClient
	activityLogger       api.ActivityLogger
	metricsLogger        api.MetricsLogger
	didResolver          api.DIDResolver
	crypto               api.Crypto
	documentLoader       ld.DocumentLoader

	requestObject *requestObject
}

type authorizedResponse struct {
	IDTokenJWS string
	VPTokenJWS string
	State      string
}

// New creates new openid4vp instance.
// If no ActivityLogger is provided (via an option), then no activity logging will take place.
func New(
	authorizationRequest string,
	signatureVerifier jwtSignatureVerifier,
	didResolver api.DIDResolver,
	crypto api.Crypto,
	documentLoader ld.DocumentLoader,
	opts ...Opt,
) *Interaction {
	client, activityLogger, metricsLogger := processOpts(opts)

	return &Interaction{
		authorizationRequest: authorizationRequest,
		signatureVerifier:    signatureVerifier,
		httpClient:           client,
		activityLogger:       activityLogger,
		metricsLogger:        metricsLogger,
		didResolver:          didResolver,
		crypto:               crypto,
		documentLoader:       documentLoader,
	}
}

// GetQuery creates query based on authorization request data.
func (o *Interaction) GetQuery() (*presexch.PresentationDefinition, error) {
	timeStartGetQuery := time.Now()

	rawRequestObject, err := o.fetchRequestObject()
	if err != nil {
		return nil, walleterror.NewExecutionError(
			module,
			RequestObjectFetchFailedCode,
			RequestObjectFetchFailedError,
			fmt.Errorf("fetch request object: %w", err))
	}

	requestObject, err := verifyAuthorizationRequestAndDecodeClaims(rawRequestObject, o.signatureVerifier)
	if err != nil {
		return nil, walleterror.NewExecutionError(
			module,
			VerifyAuthorizationRequestFailedCode,
			VerifyAuthorizationRequestFailedError,
			fmt.Errorf("verify authorization request: %w", err))
	}

	o.requestObject = requestObject

	return requestObject.Claims.VPToken.PresentationDefinition,
		o.metricsLogger.Log(&api.MetricsEvent{
			Event:    getQueryEventText,
			Duration: time.Since(timeStartGetQuery),
		})
}

// VerifierDisplayData returns display information about verifier.
func (o *Interaction) VerifierDisplayData() (*VerifierDisplayData, error) {
	if o.requestObject == nil {
		return nil, walleterror.NewExecutionError(
			module,
			NotInitializedProperlyErrorCode,
			NotInitializedProperlyError,
			fmt.Errorf("call GetQuery first"))
	}

	return &VerifierDisplayData{
		DID:     o.requestObject.ClientID,
		Name:    o.requestObject.Registration.ClientName,
		Purpose: o.requestObject.Registration.ClientPurpose,
		LogoURI: o.requestObject.Registration.ClientLogoURI,
	}, nil
}

// PresentCredential presents credentials to redirect uri from request object.
func (o *Interaction) PresentCredential(credentials []*verifiable.Credential) error {
	timeStartPresentCredential := time.Now()

	if o.requestObject == nil {
		return walleterror.NewExecutionError(
			module,
			NotInitializedProperlyErrorCode,
			NotInitializedProperlyError,
			fmt.Errorf("call GetQuery first"))
	}

	response, err := createAuthorizedResponse(credentials, o.requestObject, o.didResolver, o.crypto, o.documentLoader)
	if err != nil {
		return walleterror.NewExecutionError(
			module,
			CreateAuthorizedResponseFailedCode,
			CreateAuthorizedResponseFailedError,
			fmt.Errorf("create authorized response failed: %w", err))
	}

	data := url.Values{}
	data.Set("id_token", response.IDTokenJWS)
	data.Set("vp_token", response.VPTokenJWS)
	data.Set("state", response.State)

	err = o.sendAuthorizedResponse(data.Encode())
	if err != nil {
		return err
	}

	err = o.metricsLogger.Log(&api.MetricsEvent{
		Event:    presentCredentialEventText,
		Duration: time.Since(timeStartPresentCredential),
	})
	if err != nil {
		return err
	}

	return o.activityLogger.Log(&api.Activity{
		ID:   uuid.New(),
		Type: api.LogTypeCredentialActivity,
		Time: time.Now(),
		Data: api.Data{
			Client:    o.requestObject.Registration.ClientName,
			Operation: activityLogOperation,
			Status:    api.ActivityLogStatusSuccess,
		},
	})
}

func (o *Interaction) fetchRequestObject() (string, error) {
	if !strings.HasPrefix(o.authorizationRequest, requestURIPrefix) {
		return o.authorizationRequest, nil
	}

	endpointURL := strings.TrimPrefix(o.authorizationRequest, requestURIPrefix)

	respBytes, err := httprequest.New(o.httpClient, o.metricsLogger).Do(http.MethodGet, endpointURL, "", nil,
		fmt.Sprintf(fetchRequestObjectEventText, endpointURL), getQueryEventText)
	if err != nil {
		return "", err
	}

	return string(respBytes), nil
}

func (o *Interaction) sendAuthorizedResponse(responseBody string) error {
	_, err := httprequest.New(o.httpClient, o.metricsLogger).Do(http.MethodPost,
		o.requestObject.RedirectURI, "application/x-www-form-urlencoded",
		bytes.NewBuffer([]byte(responseBody)),
		fmt.Sprintf(sendAuthorizedResponseEventText, o.requestObject.RedirectURI),
		presentCredentialEventText)
	if err != nil {
		return walleterror.NewExecutionError(
			module,
			SendAuthorizedResponseFailedCode,
			SendAuthorizedResponseFailedError,
			fmt.Errorf("send authorized response failed: %w", err))
	}

	return nil
}

func verifyAuthorizationRequestAndDecodeClaims(
	rawRequestObject string,
	signatureVerifier jwtSignatureVerifier,
) (*requestObject, error) {
	requestObject := &requestObject{}

	err := verifyTokenSignature(rawRequestObject, requestObject, signatureVerifier)
	if err != nil {
		return nil, err
	}

	return requestObject, nil
}

func verifyTokenSignature(rawJwt string, claims interface{}, verifier jose.SignatureVerifier) error {
	jsonWebToken, _, err := jwt.Parse(rawJwt, jwt.WithSignatureVerifier(verifier))
	if err != nil {
		return fmt.Errorf("parse JWT: %w", err)
	}

	err = jsonWebToken.DecodeClaims(claims)
	if err != nil {
		return fmt.Errorf("decode claims: %w", err)
	}

	return nil
}

func createAuthorizedResponse(
	credentials []*verifiable.Credential,
	requestObject *requestObject,
	didResolver api.DIDResolver,
	crypto api.Crypto,
	documentLoader ld.DocumentLoader,
) (*authorizedResponse, error) {
	switch len(credentials) {
	case 0:
		return nil, fmt.Errorf("expected at least one credential to present to verifier")
	case 1:
		return createAuthorizedResponseOneCred(credentials[0], requestObject, didResolver, crypto, documentLoader)
	default:
		return createAuthorizedResponseMultiCred(credentials, requestObject, didResolver, crypto, documentLoader)
	}
}

func createAuthorizedResponseOneCred( //nolint:funlen
	credential *verifiable.Credential,
	requestObject *requestObject,
	didResolver api.DIDResolver,
	crypto api.Crypto,
	documentLoader ld.DocumentLoader,
) (*authorizedResponse, error) {
	var (
		err        error
		vpTokenJWS string
		did        string
		signer     api.JWTSigner
	)

	var presentation *verifiable.Presentation

	pd := requestObject.Claims.VPToken.PresentationDefinition

	// TODO: https://github.com/trustbloc/wallet-sdk/issues/165 remove this code after to re enable Schema check.
	for i := range pd.InputDescriptors {
		pd.InputDescriptors[i].Schema = nil
	}

	presentation, err = pd.CreateVP(
		[]*verifiable.Credential{credential},
		documentLoader,
		verifiable.WithDisabledProofCheck(),
		verifiable.WithJSONLDDocumentLoader(documentLoader),
		verifiable.WithPublicKeyFetcher(verifiable.NewVDRKeyResolver(wrapResolver(didResolver)).PublicKeyFetcher()),
	)
	if err != nil {
		return nil, err
	}

	did, err = verifiable.SubjectID(credential.Subject)
	if err != nil || did == "" {
		return nil, fmt.Errorf("presentation VC does not have a subject ID")
	}

	signer, err = getHolderSigner(did, didResolver, crypto)
	if err != nil {
		return nil, err
	}

	presentationSubmission := presentation.CustomFields["presentation_submission"]

	presentation.CustomFields["presentation_submission"] = nil

	idTokenJWS, err := createIDToken(requestObject, presentationSubmission, did, signer)
	if err != nil {
		return nil, err
	}

	vpTok := vpTokenClaims{
		VP:    presentation,
		Nonce: requestObject.Nonce,
		Exp:   time.Now().Unix() + tokenLiveTimeSec,
		Iss:   did,
		Aud:   requestObject.ClientID,
		Nbf:   time.Now().Unix(),
		Iat:   time.Now().Unix(),
		Jti:   uuid.NewString(),
	}

	vpTokenJWS, err = signToken(vpTok, signer)
	if err != nil {
		return nil, fmt.Errorf("sign vp_token: %w", err)
	}

	return &authorizedResponse{IDTokenJWS: idTokenJWS, VPTokenJWS: vpTokenJWS, State: requestObject.State}, nil
}

func createAuthorizedResponseMultiCred( //nolint:funlen
	credentials []*verifiable.Credential,
	requestObject *requestObject,
	didResolver api.DIDResolver,
	crypto api.Crypto,
	documentLoader ld.DocumentLoader,
) (*authorizedResponse, error) {
	pd := requestObject.Claims.VPToken.PresentationDefinition

	// TODO: https://github.com/trustbloc/wallet-sdk/issues/165 remove this code after to re enable Schema check.
	for i := range pd.InputDescriptors {
		pd.InputDescriptors[i].Schema = nil
	}

	presentations, submission, err := pd.CreateVPArray(
		credentials,
		documentLoader,
		verifiable.WithDisabledProofCheck(),
		verifiable.WithJSONLDDocumentLoader(documentLoader),
	)
	if err != nil {
		return nil, err
	}

	var vpTokens []string

	signers := map[string]api.JWTSigner{}

	for _, presentation := range presentations {
		holderDID, e := getSubjectID(presentation.Credentials()[0])
		if e != nil {
			return nil, e
		}

		signer, e := getHolderSigner(holderDID, didResolver, crypto)
		if e != nil {
			return nil, e
		}

		signers[holderDID] = signer

		vpTok := vpTokenClaims{
			VP:    presentation,
			Nonce: requestObject.Nonce,
			Exp:   time.Now().Unix() + tokenLiveTimeSec,
			Iss:   holderDID,
			Aud:   requestObject.ClientID,
			Nbf:   time.Now().Unix(),
			Iat:   time.Now().Unix(),
			Jti:   uuid.NewString(),
		}

		vpTokJWS, e := signToken(vpTok, signer)
		if e != nil {
			return nil, fmt.Errorf("sign vp_token: %w", e)
		}

		vpTokens = append(vpTokens, vpTokJWS)
	}

	vpTokenListJSON, err := json.Marshal(vpTokens)
	if err != nil {
		return nil, err
	}

	idTokenSigningDID, err := pickRandomElement(mapKeys(signers))
	if err != nil {
		return nil, err
	}

	idTokenJWS, err := createIDToken(requestObject, submission, idTokenSigningDID, signers[idTokenSigningDID])
	if err != nil {
		return nil, err
	}

	return &authorizedResponse{
		IDTokenJWS: idTokenJWS,
		VPTokenJWS: string(vpTokenListJSON),
		State:      requestObject.State,
	}, nil
}

func createIDToken(
	req *requestObject,
	submission interface{},
	signingDID string,
	signer api.JWTSigner,
) (string, error) {
	idToken := &idTokenClaims{
		VPToken: idTokenVPToken{
			PresentationSubmission: submission,
		},
		Nonce: req.Nonce,
		Exp:   time.Now().Unix() + tokenLiveTimeSec,
		Iss:   "https://self-issued.me/v2/openid-vc",
		Sub:   signingDID,
		Aud:   req.ClientID,
		Nbf:   time.Now().Unix(),
		Iat:   time.Now().Unix(),
		Jti:   uuid.NewString(),
	}

	idTokenJWS, err := signToken(idToken, signer)
	if err != nil {
		return "", fmt.Errorf("sign id_token: %w", err)
	}

	return idTokenJWS, nil
}

func signToken(claims interface{}, signer api.JWTSigner) (string, error) {
	token, err := jwt.NewSigned(claims, nil, signer)
	if err != nil {
		return "", fmt.Errorf("sign token failed: %w", err)
	}

	tokenBytes, err := token.Serialize(false)
	if err != nil {
		return "", fmt.Errorf("serialize token failed: %w", err)
	}

	return tokenBytes, nil
}

func getHolderSigner(holderDID string, didResolver api.DIDResolver, crypto api.Crypto) (api.JWTSigner, error) {
	docRes, err := didResolver.Resolve(holderDID)
	if err != nil {
		return nil, fmt.Errorf("resolve holder DID for signing: %w", err)
	}

	verificationMethods := docRes.DIDDocument.VerificationMethods(diddoc.AssertionMethod)

	if len(verificationMethods[diddoc.AssertionMethod]) == 0 {
		return nil, fmt.Errorf("holder DID has no assertion method for signing")
	}

	signingVM := verificationMethods[diddoc.AssertionMethod][0].VerificationMethod

	return common.NewJWSSigner(models.VerificationMethodFromDoc(&signingVM), crypto)
}

func getSubjectID(vc interface{}) (string, error) {
	var (
		err    error
		subjID string
	)

	switch cred := vc.(type) {
	case *verifiable.Credential:
		subjID, err = verifiable.SubjectID(cred.Subject)
	case map[string]interface{}:
		subjID, err = verifiable.SubjectID(cred["credentialSubject"])
	}

	if err != nil || subjID == "" {
		return "", fmt.Errorf("VC does not have a subject ID")
	}

	return subjID, nil
}

func mapKeys(in map[string]api.JWTSigner) []string {
	var keys []string

	for s := range in {
		keys = append(keys, s)
	}

	return keys
}

func pickRandomElement(list []string) (string, error) {
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(list))))
	if err != nil {
		return "", err
	}

	return list[idx.Int64()], nil
}

type resolverAdapter struct {
	didResolver api.DIDResolver
}

func (r *resolverAdapter) Resolve(did string, opts ...vdrspi.DIDMethodOption) (*diddoc.DocResolution, error) {
	return r.didResolver.Resolve(did)
}

func wrapResolver(didResolver api.DIDResolver) *resolverAdapter {
	return &resolverAdapter{didResolver: didResolver}
}
