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

// AuthorityViewRevision proves whether the commercial view a scope was resolved
// under is still the same one. It never replaces the immutable object version:
// a new revision means "re-check compatibility", not "the adopted version
// changed".
type AuthorityViewRevision struct{ requiredValue }

// ContinuationReference lets a caller pick a stalled decision back up. A stale
// or pending answer must remain continuable, because neither is a business
// refusal the caller may act on.
type ContinuationReference struct{ requiredValue }

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

// ResolutionPurpose is what the caller needs a basis for. It is part of the key
// because the same scope answers differently depending on the question: an
// acceptance-control basis and a pricing basis are not interchangeable.
type ResolutionPurpose uint8

const (
	ResolutionPurposeInvalid ResolutionPurpose = iota
	AcceptanceControlPurpose
	PricingPurpose
)

func (purpose ResolutionPurpose) valid() bool {
	return purpose >= AcceptanceControlPurpose && purpose <= PricingPurpose
}

func (purpose ResolutionPurpose) String() string {
	switch purpose {
	case AcceptanceControlPurpose:
		return "ACCEPTANCE_CONTROL"
	case PricingPurpose:
		return "PRICING"
	default:
		return ""
	}
}

// PriceDirection separates what is sold, what is bought and what moves between
// the operator's own legal entities. The three never share a resolution or a
// cache entry, so a SELL request can never be answered by a BUY result.
type PriceDirection uint8

const (
	PriceDirectionInvalid PriceDirection = iota
	BuyDirection
	SellDirection
	InternalDirection
)

func (direction PriceDirection) valid() bool {
	return direction >= BuyDirection && direction <= InternalDirection
}

func (direction PriceDirection) String() string {
	switch direction {
	case BuyDirection:
		return "BUY"
	case SellDirection:
		return "SELL"
	case InternalDirection:
		return "INTERNAL"
	default:
		return ""
	}
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
	Purpose              ResolutionPurpose
	// PriceDirection applies to pricing only. It must be absent for any other
	// purpose: two keys differing solely in a dimension that means nothing for
	// their purpose would otherwise resolve to different identities.
	PriceDirection PriceDirection
	Anchor         SelectionAnchor
}

func (key ResolutionKey) minimumIdentityEstablished() bool {
	if !key.TenantID.valid() ||
		!key.CustomerAccountID.valid() ||
		!key.LegalEntityCandidate.valid() ||
		!key.Scope.valid() ||
		!key.RequiredBasis.valid() ||
		!key.Purpose.valid() {
		return false
	}
	if key.Purpose == PricingPurpose {
		return key.PriceDirection.valid()
	}
	return !key.PriceDirection.valid()
}

func (key ResolutionKey) fingerprint() string {
	return strings.Join([]string{
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.LegalEntityCandidate.String(),
		key.Scope.String(),
		key.RequiredBasis.String(),
		key.Purpose.String(),
		key.PriceDirection.String(),
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

// ResolutionReason is the stable cause behind an outcome that stopped short of
// a usable basis. It is a closed set rather than free text so that pending and
// stale results stay countable by cause instead of by message; values appear
// together with the rule that produces them.
//
// A unique resolution has no reason, which is why the zero value means absent
// rather than unknown.
type ResolutionReason uint8

const (
	ResolutionReasonNone ResolutionReason = iota
	AnchorPolicyNotConfigured
	AuthorityUnreadable
	CurrentResolutionChanged
)

func (reason ResolutionReason) String() string {
	switch reason {
	case AnchorPolicyNotConfigured:
		return "ANCHOR_POLICY_NOT_CONFIGURED"
	case AuthorityUnreadable:
		return "AUTHORITY_UNREADABLE"
	case CurrentResolutionChanged:
		return "CURRENT_RESOLUTION_CHANGED"
	default:
		return ""
	}
}

type Resolution struct {
	outcome        ResolutionOutcome
	resolutionID   ResolutionID
	key            ResolutionKey
	anchor         SelectionAnchor
	adopted        CommercialVersion
	hasAdopted     bool
	viewRevision   AuthorityViewRevision
	reason         ResolutionReason
	continuation   ContinuationReference
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

func (resolution Resolution) ViewRevision() (AuthorityViewRevision, bool) {
	if !resolution.viewRevision.valid() {
		return AuthorityViewRevision{}, false
	}
	return resolution.viewRevision, true
}

// Reason names why the resolution stopped short of a usable basis. It pairs
// with ContinuationReference: the reason says what to fix, the reference says
// which attempt to resume.
func (resolution Resolution) Reason() ResolutionReason {
	return resolution.reason
}

func (resolution Resolution) ContinuationReference() ContinuationReference {
	return resolution.continuation
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
		return pending(key, ResolutionID{}, AnchorPolicyNotConfigured, SelectionAnchor{})
	}
	if registry == nil {
		return pending(key, ResolutionID{}, AuthorityUnreadable, key.Anchor)
	}

	candidates := registry.applicable(key)
	result := Resolution{
		key:            key,
		anchor:         key.Anchor,
		viewRevision:   registry.ViewRevision(key.Scope),
		candidateCount: len(candidates),
	}
	switch len(candidates) {
	case 0:
		result.outcome = NoApplicableBasis
	case 1:
		result.outcome = UniquelyResolved
		result.adopted = candidates[0]
		result.hasAdopted = true
		result.resolutionID = resolutionIdentity(key, result.viewRevision, candidates[0])
	default:
		result.outcome = ApplicabilityConflict
	}
	return result
}

// ValidateBeforeDecision re-runs phase one before the caller commits a decision.
// It re-resolves the original query rather than inspecting the adopted object
// alone: a rival candidate added to the same scope leaves that object untouched
// while making the resolution ambiguous, and only re-resolution sees it.
//
// A prior result that never resolved uniquely is returned unchanged — there is
// no adopted basis whose continued validity could be in question.
func ValidateBeforeDecision(registry *CommercialRegistry, prior Resolution) Resolution {
	if prior.outcome != UniquelyResolved {
		return prior
	}
	// Without a readable authority the prior result can be neither confirmed nor
	// declared stale, so it stays pending and continuable.
	if registry == nil {
		stalled := prior
		stalled.outcome = ResolutionPending
		stalled.adopted = CommercialVersion{}
		stalled.hasAdopted = false
		stalled.reason = AuthorityUnreadable
		stalled.continuation = continuationFor(prior.key, prior.resolutionID, AuthorityUnreadable)
		return stalled
	}

	current := ResolveCommercialBasis(registry, prior.key)
	if current.outcome == UniquelyResolved && current.resolutionID == prior.resolutionID {
		return prior
	}

	stale := Resolution{
		outcome:        ResolutionStale,
		resolutionID:   prior.resolutionID,
		key:            prior.key,
		anchor:         prior.anchor,
		viewRevision:   current.viewRevision,
		candidateCount: current.candidateCount,
		reason:         CurrentResolutionChanged,
		continuation:   continuationFor(prior.key, prior.resolutionID, CurrentResolutionChanged),
	}
	return stale
}

// pending builds the one shape every undecided answer takes. Routing all of them
// through here is what stops a result's usefulness from depending on which path
// produced it: before this existed, a pending from pre-decision validation was
// continuable while a pending from the first resolution was not.
func pending(
	key ResolutionKey,
	priorID ResolutionID,
	reason ResolutionReason,
	anchor SelectionAnchor,
) Resolution {
	return Resolution{
		outcome:      ResolutionPending,
		key:          key,
		anchor:       anchor,
		reason:       reason,
		continuation: continuationFor(key, priorID, reason),
	}
}

// continuationFor derives the reference a caller resumes a stalled decision
// with. It is derived from the query, the prior identity and the cause, so the
// same input stalled for the same cause is always offered the same reference —
// which is what lets the caller query the original attempt instead of guessing.
func continuationFor(key ResolutionKey, priorID ResolutionID, reason ResolutionReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		key.fingerprint(),
		priorID.String(),
	}, "\x00")))
	return ContinuationReference{requiredValue{value: "CONT-" + hex.EncodeToString(digest[:8])}}
}

// resolutionIdentity is derived from the key, the authority view revision and
// the adopted version, so the same input against the same view answers with the
// same identity while any change to the view produces a different one. That is
// what lets pre-decision validation detect staleness by comparison alone. It is
// not a new commercial version and creates nothing.
func resolutionIdentity(key ResolutionKey, view AuthorityViewRevision, adopted CommercialVersion) ResolutionID {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.fingerprint(),
		view.String(),
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
