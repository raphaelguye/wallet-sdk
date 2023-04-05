/*
Copyright Avast Software. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"errors"

	"github.com/hyperledger/aries-framework-go/pkg/doc/verifiable"
)

const claimsTypesFieldName = "type"

// VerifiableCredential represents a Verifiable Credential per the VC Data Model spec:
// https://www.w3.org/TR/vc-data-model/.
// It wraps the VC type from aries-framework-go and provides gomobile-compatible methods.
type VerifiableCredential struct {
	VC *verifiable.Credential // Will be skipped in the gomobile bindings due to using an incompatible type
}

// NewVerifiableCredential creates a new VerifiableCredential.
// This function is only used internally in wallet-sdk-gomobile and is not available in the bindings due to it using
// unsupported types.
// To create a VC from a serialized format via the bindings, see the ParseVC method.
// This function will be skipped in the gomobile bindings due to using an incompatible type.
func NewVerifiableCredential(vc *verifiable.Credential) *VerifiableCredential {
	return &VerifiableCredential{
		VC: vc,
	}
}

// ID returns this VC's ID.
func (v *VerifiableCredential) ID() string {
	return v.VC.ID
}

// Name returns this VC's name.
// If this VC doesn't provide a name, or the name isn't a string, then an empty string is returned.
func (v *VerifiableCredential) Name() string {
	rawName, found := v.VC.CustomFields["name"]
	if !found {
		return ""
	}

	nameAsString, ok := rawName.(string)
	if !ok {
		return ""
	}

	return nameAsString
}

// IssuerID returns the ID of this VC's issuer.
// While the ID is typically going to be a DID, the Verifiable Credential spec does not mandate this.
func (v *VerifiableCredential) IssuerID() string {
	return v.VC.Issuer.ID
}

// Types returns the types of this VC. At a minimum, one of the types will be "VerifiableCredential".
// There may be additional more specific credential types as well.
func (v *VerifiableCredential) Types() *StringArray {
	return &StringArray{strings: v.VC.Types}
}

// ClaimTypes returns the types specified in the claims of this VC.
// It first checks the selective disclosure claims for the types - if found, they are returned.
// Otherwise, the non-selective disclosure claims are checked and returned if found.
// When checking the non-selective disclosure claims, only the first subject is checked.
// If not found in either place, then nil is returned.
func (v *VerifiableCredential) ClaimTypes() *StringArray {
	types := v.claimTypesFromSelectiveDisclosures()
	if types != nil {
		return types
	}

	types = v.claimTypesFromCredentialSubject()
	if types != nil {
		return types
	}

	return nil
}

func (v *VerifiableCredential) claimTypesFromSelectiveDisclosures() *StringArray {
	if len(v.VC.SDJWTDisclosures) > 0 {
		for _, disclosure := range v.VC.SDJWTDisclosures {
			if disclosure.Name == claimsTypesFieldName {
				return rawTypesToStringArray(disclosure.Value)
			}
		}
	}

	return nil
}

func (v *VerifiableCredential) claimTypesFromCredentialSubject() *StringArray {
	credentialSubjects, ok := v.VC.Subject.([]verifiable.Subject)
	if !ok {
		return nil
	}

	if len(credentialSubjects) == 0 {
		return nil
	}

	rawTypes, exists := credentialSubjects[0].CustomFields[claimsTypesFieldName]
	if !exists {
		return nil
	}

	return rawTypesToStringArray(rawTypes)
}

func rawTypesToStringArray(rawTypes interface{}) *StringArray {
	typesAsInterfaceArray, ok := rawTypes.([]interface{}) // This will be the type if the VC was parsed (unmarshalled)
	if ok {
		types := make([]string, len(typesAsInterfaceArray))
		for i := 0; i < len(typesAsInterfaceArray); i++ {
			types[i], ok = typesAsInterfaceArray[i].(string)
			if !ok {
				return nil
			}
		}

		return &StringArray{strings: types}
	}

	typesAsStringArray, ok := rawTypes.([]string)
	if ok {
		return &StringArray{strings: typesAsStringArray}
	}

	typeAsString, ok := rawTypes.(string)
	if ok {
		return &StringArray{strings: []string{typeAsString}}
	}

	return nil
}

// IssuanceDate returns this VC's issuance date as a Unix timestamp.
func (v *VerifiableCredential) IssuanceDate() (int64, error) {
	if v.VC.Issued == nil {
		return -1, errors.New("issuance date missing (invalid VC)")
	}

	return v.VC.Issued.Unix(), nil
}

// HasExpirationDate returns whether this VC has an expiration date.
func (v *VerifiableCredential) HasExpirationDate() bool {
	return v.VC.Expired != nil
}

// ExpirationDate returns this VC's expiration date as a Unix timestamp.
// HasExpirationDate should be called first to ensure this VC has an expiration date before calling this method.
// This method returns an error if the VC has no expiration date.
func (v *VerifiableCredential) ExpirationDate() (int64, error) {
	if v.VC.Expired == nil {
		return -1, errors.New("VC has no expiration date")
	}

	return v.VC.Expired.Unix(), nil
}

// Serialize returns a JSON representation of this VC.
func (v *VerifiableCredential) Serialize() (string, error) {
	marshalledVC, err := v.VC.MarshalJSON()
	if err != nil {
		return "", err
	}

	return string(marshalledVC), nil
}

// VerifiableCredentialsArray represents an array of VerifiableCredentials.
// Since arrays and slices are not compatible with gomobile, this type acts as a wrapper around a Go array of VCs.
type VerifiableCredentialsArray struct {
	credentials []*VerifiableCredential
}

// NewVerifiableCredentialsArray creates new NewVerifiableCredentialsArray.
func NewVerifiableCredentialsArray() *VerifiableCredentialsArray {
	return &VerifiableCredentialsArray{}
}

// Add adds new VC to underlying array.
func (a *VerifiableCredentialsArray) Add(cred *VerifiableCredential) {
	a.credentials = append(a.credentials, cred)
}

// Length returns length of underlying array.
func (a *VerifiableCredentialsArray) Length() int {
	return len(a.credentials)
}

// AtIndex returns element from underlying array by index.
func (a *VerifiableCredentialsArray) AtIndex(index int) *VerifiableCredential {
	return a.credentials[index]
}
