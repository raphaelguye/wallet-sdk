/*
Copyright Gen Digital Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package credential

import "github.com/trustbloc/wallet-sdk/cmd/wallet-sdk-gomobile/api"

// CredentialsArg represents the different ways that credentials can be passed in to the Query method.
// At most one out of VCs and CredentialReader should be used for a given call to Resolve. If both are specified,
// then VCs will take precedence.
type CredentialsArg struct {
	// VCs is an array of Verifiable CredentialsArg. If specified, this takes precedence over the CredentialReader
	// used in the constructor (NewResolver).
	vcs *api.VerifiableCredentialsArray
	// CredentialReader allows for access to a VC storage mechanism.
	reader api.CredentialReader
}

// NewCredentialsArgFromVCArray creates CredentialsArg from VCs.
func NewCredentialsArgFromVCArray(vcArr *api.VerifiableCredentialsArray) *CredentialsArg {
	return &CredentialsArg{
		vcs: vcArr,
	}
}

// NewCredentialsArgFromReader creates CredentialsArg from CredentialReader.
func NewCredentialsArgFromReader(credentialReader api.CredentialReader) *CredentialsArg {
	return &CredentialsArg{
		reader: credentialReader,
	}
}
