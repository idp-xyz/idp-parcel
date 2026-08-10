package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrInvalidSelectionAnchor = errors.New("party commercial: invalid commercial selection anchor")

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

type CustomerAccountID struct{ requiredValue }

func NewCustomerAccountID(value string) (CustomerAccountID, error) {
	required, err := newRequiredValue("customer account ID", value)
	return CustomerAccountID{required}, err
}

type LegalEntityReference struct{ requiredValue }

func NewLegalEntityReference(value string) (LegalEntityReference, error) {
	required, err := newRequiredValue("legal entity reference", value)
	return LegalEntityReference{required}, err
}

type AnchorPolicyVersion struct{ requiredValue }

func NewAnchorPolicyVersion(value string) (AnchorPolicyVersion, error) {
	required, err := newRequiredValue("anchor policy version", value)
	return AnchorPolicyVersion{required}, err
}

type ResolutionID struct{ requiredValue }

// SelectionAnchor is the business instant phase one selects against. It cannot
// be built without the versioned policy that produced it, which is what stops
// the source time, the customer's requested time, the wall clock or a candidate
// rule package from quietly becoming the anchor.
type SelectionAnchor struct {
	at            time.Time
	policyVersion AnchorPolicyVersion
}

func NewSelectionAnchor(at time.Time, policyVersion AnchorPolicyVersion) (SelectionAnchor, error) {
	if at.IsZero() || !policyVersion.valid() {
		return SelectionAnchor{}, ErrInvalidSelectionAnchor
	}
	return SelectionAnchor{at: at.UTC(), policyVersion: policyVersion}, nil
}

func (anchor SelectionAnchor) At() time.Time {
	return anchor.at
}

func (anchor SelectionAnchor) PolicyVersion() AnchorPolicyVersion {
	return anchor.policyVersion
}

func (anchor SelectionAnchor) valid() bool {
	return !anchor.at.IsZero() && anchor.policyVersion.valid()
}

// ResolutionKey is the full set of dimensions one required basis is selected
// by. Dropping any of them would let one customer's resolution answer another's,
// or let one scope borrow a basis resolved for a different one.
type ResolutionKey struct {
	TenantID             TenantID
	CustomerAccountID    CustomerAccountID
	LegalEntityCandidate LegalEntityReference
	Scope                CommercialScopeReference
	RequiredBasis        CommercialObjectKind
	Anchor               SelectionAnchor
}

func (key ResolutionKey) minimumIdentityEstablished() bool {
	return key.TenantID.valid() &&
		key.CustomerAccountID.valid() &&
		key.LegalEntityCandidate.valid() &&
		key.Scope.valid() &&
		key.RequiredBasis.valid()
}

func (key ResolutionKey) fingerprint() string {
	return strings.Join([]string{
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.LegalEntityCandidate.String(),
		key.Scope.String(),
		key.RequiredBasis.String(),
		key.Anchor.PolicyVersion().String(),
		key.Anchor.At().Format(time.RFC3339Nano),
	}, "\x00")
}

// ResolutionOutcome is the closed set of answers phase one may give. They are
// deliberately separate: "no applicable basis" is an authority statement, while
// "pending" means the authority could not be read at all, and collapsing them
// would let a dependency failure read as a customer having no contract.
type ResolutionOutcome uint8

const (
	ResolutionOutcomeInvalid ResolutionOutcome = iota
	UniquelyResolved
	NoApplicableBasis
	ApplicabilityConflict
	ResolutionPending
	InputNotAccepted
	ResolutionStale
)

func (outcome ResolutionOutcome) String() string {
	switch outcome {
	case UniquelyResolved:
		return "UNIQUELY_RESOLVED"
	case NoApplicableBasis:
		return "NO_APPLICABLE_BASIS"
	case ApplicabilityConflict:
		return "APPLICABILITY_CONFLICT"
	case ResolutionPending:
		return "RESOLUTION_PENDING"
	case InputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ResolutionStale:
		return "STALE"
	default:
		return ""
	}
}

type Resolution struct {
	outcome        ResolutionOutcome
	resolutionID   ResolutionID
	anchor         SelectionAnchor
	adopted        CommercialVersion
	hasAdopted     bool
	candidateCount int
}

func (resolution Resolution) Outcome() ResolutionOutcome {
	return resolution.outcome
}

func (resolution Resolution) ResolutionID() ResolutionID {
	return resolution.resolutionID
}

func (resolution Resolution) Anchor() SelectionAnchor {
	return resolution.anchor
}

func (resolution Resolution) AdoptedVersion() (CommercialVersion, bool) {
	return resolution.adopted, resolution.hasAdopted
}

// CandidateCount reports how many applicable versions the authority held. It
// stays zero for inputs that were never accepted, because counting would
// already disclose whether objects exist in a scope the caller cannot name.
func (resolution Resolution) CandidateCount() int {
	return resolution.candidateCount
}

// ResolveCommercialBasis performs phase one: select the single applicable
// version of one required basis. It forms no acceptance, price, control or
// downstream asOf; phase two is the caller's, driven by the rule package this
// resolution adopts.
func ResolveCommercialBasis(registry *CommercialRegistry, key ResolutionKey) Resolution {
	if !key.minimumIdentityEstablished() {
		return Resolution{outcome: InputNotAccepted}
	}
	// A missing anchor policy is pending rather than a failure: the policy is an
	// instance parameter that may simply not be configured yet, and the one
	// thing that must never happen is substituting a default instant for it.
	if !key.Anchor.valid() {
		return Resolution{outcome: ResolutionPending}
	}
	if registry == nil {
		return Resolution{outcome: ResolutionPending, anchor: key.Anchor}
	}

	candidates := registry.applicable(key)
	result := Resolution{
		anchor:         key.Anchor,
		candidateCount: len(candidates),
	}
	switch len(candidates) {
	case 0:
		result.outcome = NoApplicableBasis
	case 1:
		result.outcome = UniquelyResolved
		result.adopted = candidates[0]
		result.hasAdopted = true
		result.resolutionID = resolutionIdentity(key, candidates[0])
	default:
		result.outcome = ApplicabilityConflict
	}
	return result
}

// resolutionIdentity is derived from the key and the adopted version so that
// the same input against the same authority view answers with the same
// identity. It is not a new commercial version and creates nothing.
func resolutionIdentity(key ResolutionKey, adopted CommercialVersion) ResolutionID {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.fingerprint(),
		adopted.objectID.String(),
		adopted.version.String(),
		adopted.contentDigest.String(),
	}, "\x00")))
	return ResolutionID{requiredValue{value: "RES-" + hex.EncodeToString(digest[:8])}}
}

// applicable narrows the registry to versions that may still be selected for a
// new decision: the requested basis kind, the requested scope, and effective at
// the anchor. Ended versions drop out here rather than being filtered later.
func (registry *CommercialRegistry) applicable(key ResolutionKey) []CommercialVersion {
	matches := make([]CommercialVersion, 0, 2)
	for _, version := range registry.versions {
		if version.kind != key.RequiredBasis || version.scope != key.Scope {
			continue
		}
		if !version.AppliesAt(key.Anchor.At()) {
			continue
		}
		matches = append(matches, version)
	}
	return matches
}
