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
	Version = "v0.1.0-rc.2"

	// ModuleSum is the module checksum reported by `go mod download -json` for
	// the candidate. A different checksum means a different artifact and fails
	// the contract.
	ModuleSum = "h1:W9T1KsbXwuVs3lHTNwSUcEBY0Ids00hUH7I9KuBtCEo="

	// GoModSum is the checksum of the candidate `go.mod`.
	GoModSum = "h1:RFR6ylLNIIA7e4PPGVCzojYiH6DB8eHd2s5vI9XcUgY="

	// OriginRef is the immutable Git reference the candidate must come from.
	OriginRef = "refs/tags/v0.1.0-rc.2"

	// OriginCommit is the framework commit the candidate tag points at.
	OriginCommit = "56322dc25373b872ab5c49a74c0d544d7c088184"
)

// Governance records the honest single maintainer state of this proof. Parcel
// never fabricates a second reviewer.
const (
	GovernanceMode  = "SOLO_BOOTSTRAP"
	ApprovalNotice  = "NO INDEPENDENT HUMAN APPROVAL"
	BypassableNotic = "OWNER-BYPASSABLE"
)

// ContractSuiteVersion is the contract matrix version the framework
// coordination job pins. Both consumers must report the same value, otherwise
// one side could prove a stale matrix, so this is not a Parcel-local label.
const ContractSuiteVersion = "r05-v1"
