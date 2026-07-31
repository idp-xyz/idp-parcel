// Package bentocontract is the Parcel consumer contract against one immutable
// `idp-bento-go` candidate. It exists only to prove the framework contract from
// Parcel's own security boundary; no production package may import it.
package bentocontract

// Candidate identifies the exact framework candidate this contract run proves.
// The values are compared against the module graph at test time, so a proof can
// never claim a version the build did not actually resolve.
const (
	// ModulePath is the official module path. The contract must resolve the
	// candidate through it, never through a local directory, a GitHub import
	// path or a source copy.
	ModulePath = "go.idp.xyz/idp-bento-go"

	// Version is the immutable candidate under test.
	Version = "v0.1.0-rc.1"

	// ModuleSum is the module checksum reported by `go mod download -json` for
	// the candidate. A different checksum means a different artifact and fails
	// the contract.
	ModuleSum = "h1:lx/snQkwUQW5pWPOgNI4AsZrSxxi0YYqHfOemsgEuLQ="

	// GoModSum is the checksum of the candidate `go.mod`.
	GoModSum = "h1:RFR6ylLNIIA7e4PPGVCzojYiH6DB8eHd2s5vI9XcUgY="

	// OriginRef is the immutable Git reference the candidate must come from.
	OriginRef = "refs/tags/v0.1.0-rc.1"

	// OriginCommit is the framework commit the candidate tag points at.
	OriginCommit = "af68525c1d08fccae13eccd01f209465959673d6"
)

// Governance records the honest single maintainer state of this proof. Parcel
// never fabricates a second reviewer.
const (
	GovernanceMode  = "SOLO_BOOTSTRAP"
	ApprovalNotice  = "NO INDEPENDENT HUMAN APPROVAL"
	BypassableNotic = "OWNER-BYPASSABLE"
)

// ContractSuiteVersion is the version of this contract suite. A framework
// candidate proof cites both the candidate and the suite that produced it.
const ContractSuiteVersion = "parcel-bento-contract-1"
