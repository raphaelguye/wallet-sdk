/*
Copyright Avast Software. All Rights Reserved.
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	diddoc "github.com/hyperledger/aries-framework-go/pkg/doc/did"
	"github.com/hyperledger/aries-framework-go/pkg/doc/util"
	"github.com/hyperledger/aries-framework-go/pkg/doc/verifiable"
	"github.com/hyperledger/aries-framework-go/pkg/framework/aries/api/vdr"
	"github.com/piprate/json-gold/ld"
	"github.com/stretchr/testify/require"

	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/credential"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/did"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/localkms"
	"github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/metricslogger/stderr"
	"github.com/trustbloc/wallet-sdk/internal/testutil"
	sdkapi "github.com/trustbloc/wallet-sdk/pkg/api"
	"github.com/trustbloc/wallet-sdk/pkg/did/resolver"
)

func TestCredentialAPI(t *testing.T) {
	kms, e := localkms.NewKMS(localkms.NewMemKMSStore())
	require.NoError(t, e)

	crypto := kms.GetCrypto()

	credStore := credential.NewInMemoryDB()

	didResolver, e := did.NewResolver(nil)
	require.NoError(t, e)

	signer, e := credential.NewSigner(credStore, didResolver, crypto)
	require.NoError(t, e)

	c, e := did.NewCreator(kms)
	require.NoError(t, e)

	sdkResolver, e := resolver.NewDIDResolver("")
	require.NoError(t, e)

	ldLoader := testutil.DocumentLoader(t)

	verifier := jwtvcVerifier{
		ldLoader:         ldLoader,
		publicKeyFetcher: verifiable.NewVDRKeyResolver(&didResolverWrapper{didResolver: sdkResolver}).PublicKeyFetcher(),
	}

	testCases := []struct {
		name          string
		didMethod     string
		getCredByName bool
	}{
		{
			name:          "did:ion signing DID",
			didMethod:     "ion",
			getCredByName: false,
		},
		{
			name:          "did:ion signing DID, with stored credential",
			didMethod:     "ion",
			getCredByName: true,
		},
		{
			name:          "did:key signing DID",
			didMethod:     "key",
			getCredByName: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			createDIDOptionalArgs := did.NewCreateOpts()
			createDIDOptionalArgs.SetMetricsLogger(stderr.NewMetricsLogger())

			didDoc, err := c.Create(tc.didMethod, createDIDOptionalArgs)
			require.NoError(t, err)

			docID, err := didDoc.ID()
			require.NoError(t, err)

			templateCredential := &verifiable.Credential{
				ID:      "cred-ID",
				Types:   []string{verifiable.VCType},
				Context: []string{verifiable.ContextURI},
				Subject: verifiable.Subject{
					ID: "foo",
				},
				Issuer: verifiable.Issuer{
					ID: docID,
				},
				Issued: util.NewTime(time.Now()),
			}

			err = credStore.Add(api.NewVerifiableCredential(templateCredential))
			require.NoError(t, err)

			var cred *api.VerifiableCredential
			var credID string

			if tc.getCredByName {
				credID = templateCredential.ID
			} else {
				cred = api.NewVerifiableCredential(templateCredential)
			}

			issuedCred, err := signer.Issue(cred, credID, docID)
			require.NoError(t, err)

			require.NoError(t, verifier.verify(issuedCred))
		})
	}
}

type jwtvcVerifier struct {
	ldLoader         ld.DocumentLoader
	publicKeyFetcher verifiable.PublicKeyFetcher
}

func (j *jwtvcVerifier) verify(cred []byte) error {
	_, err := verifiable.ParseCredential(
		cred,
		verifiable.WithJSONLDDocumentLoader(j.ldLoader),
		verifiable.WithPublicKeyFetcher(j.publicKeyFetcher),
	)

	return err
}

type didResolverWrapper struct {
	didResolver sdkapi.DIDResolver
}

func (d *didResolverWrapper) Resolve(did string, _ ...vdr.DIDMethodOption) (*diddoc.DocResolution, error) {
	return d.didResolver.Resolve(did)
}

type documentLoaderReverseWrapper struct {
	DocumentLoader ld.DocumentLoader
}

func (l *documentLoaderReverseWrapper) LoadDocument(url string) (*api.LDDocument, error) {
	doc, err := l.DocumentLoader.LoadDocument(url)
	if err != nil {
		return nil, err
	}

	documentBytes, err := json.Marshal(doc.Document)
	if err != nil {
		return nil, fmt.Errorf("fail to unmarshal ld document bytes: %w", err)
	}

	wrappedDoc := &api.LDDocument{
		DocumentURL: doc.DocumentURL,
		Document:    string(documentBytes),
		ContextURL:  doc.ContextURL,
	}

	return wrappedDoc, nil
}
