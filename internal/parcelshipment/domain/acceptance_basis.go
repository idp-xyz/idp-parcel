package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCommercialBasisSnapshot = errors.New("parcel shipment: invalid commercial basis snapshot")
	ErrInvalidDeclaredAsOf            = errors.New("parcel shipment: invalid declared as-of")
	ErrInvalidReachabilityJudgment    = errors.New("parcel shipment: invalid reachability judgment")
)

// The types here are parcel-shipment's own references to facts other contexts
// own. party-commercial and network-routing keep their models; this context
// records the references and snapshots it adopted, which is what lets the two
// evolve without either editing the other's objects.

type CommercialResolutionID struct{ requiredValue }

func NewCommercialResolutionID(value string) (CommercialResolutionID, error) {
	required, err := newRequiredValue("commercial resolution ID", value)
	return CommercialResolutionID{required}, err
}

type RulePackageReference struct{ requiredValue }

func NewRulePackageReference(value string) (RulePackageReference, error) {
	required, err := newRequiredValue("rule package reference", value)
	return RulePackageReference{required}, err
}

type CommercialViewRevision struct{ requiredValue }

func NewCommercialViewRevision(value string) (CommercialViewRevision, error) {
	required, err := newRequiredValue("commercial view revision", value)
	return CommercialViewRevision{required}, err
}

type AsOfPolicyVersion struct{ requiredValue }

func NewAsOfPolicyVersion(value string) (AsOfPolicyVersion, error) {
	required, err := newRequiredValue("as-of policy version", value)
	return AsOfPolicyVersion{required}, err
}

type ReachabilityJudgmentID struct{ requiredValue }

func NewReachabilityJudgmentID(value string) (ReachabilityJudgmentID, error) {
	required, err := newRequiredValue("reachability judgment ID", value)
	return ReachabilityJudgmentID{required}, err
}

// JudgmentKind names a downstream judgment that the adopted rule package
// declares an as-of policy for. Values appear together with the orchestration
// that consumes them; pre-acceptance financial control is absent because that
// step is not orchestrated yet.
type JudgmentKind uint8

const (
	JudgmentKindInvalid JudgmentKind = iota
	ReachabilityJudgmentKind
)

func (kind JudgmentKind) valid() bool {
	return kind == ReachabilityJudgmentKind
}

func (kind JudgmentKind) String() string {
	switch kind {
	case ReachabilityJudgmentKind:
		return "REACHABILITY"
	default:
		return ""
	}
}

// DeclaredAsOf is one per-judgment anchor the adopted rule package declared.
// parcel-shipment forms the value; it never invents one, so a judgment with no
// declaration simply cannot proceed.
type DeclaredAsOf struct {
	kind          JudgmentKind
	at            time.Time
	policyVersion AsOfPolicyVersion
}

func NewDeclaredAsOf(kind JudgmentKind, at time.Time, policyVersion AsOfPolicyVersion) (DeclaredAsOf, error) {
	if !kind.valid() || at.IsZero() || !policyVersion.valid() {
		return DeclaredAsOf{}, ErrInvalidDeclaredAsOf
	}
	return DeclaredAsOf{kind: kind, at: at.UTC(), policyVersion: policyVersion}, nil
}

func (declared DeclaredAsOf) Kind() JudgmentKind {
	return declared.kind
}

func (declared DeclaredAsOf) At() time.Time {
	return declared.at
}

func (declared DeclaredAsOf) PolicyVersion() AsOfPolicyVersion {
	return declared.policyVersion
}

// JudgmentAsOf is the anchor actually sent to an authority provider, which must
// verify and echo it. It is the same shape as the declaration because forming a
// value must not add anything the policy did not authorise.
type JudgmentAsOf = DeclaredAsOf

// CommercialBasisSnapshot is what parcel-shipment keeps of a unique commercial
// resolution: the identity, the adopted rule package, the authority view
// revision it held under, and the anchors that package declared. It holds no
// commercial version content, because that content is party-commercial's.
type CommercialBasisSnapshot struct {
	resolutionID CommercialResolutionID
	rulePackage  RulePackageReference
	viewRevision CommercialViewRevision
	declaredAsOf []DeclaredAsOf
}

func NewCommercialBasisSnapshot(
	resolutionID CommercialResolutionID,
	rulePackage RulePackageReference,
	viewRevision CommercialViewRevision,
	declaredAsOf []DeclaredAsOf,
) (CommercialBasisSnapshot, error) {
	if !resolutionID.valid() || !rulePackage.valid() || !viewRevision.valid() {
		return CommercialBasisSnapshot{}, ErrInvalidCommercialBasisSnapshot
	}
	seen := make(map[JudgmentKind]struct{}, len(declaredAsOf))
	for _, declared := range declaredAsOf {
		if !declared.kind.valid() || declared.at.IsZero() || !declared.policyVersion.valid() {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		if _, exists := seen[declared.kind]; exists {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		seen[declared.kind] = struct{}{}
	}
	return CommercialBasisSnapshot{
		resolutionID: resolutionID,
		rulePackage:  rulePackage,
		viewRevision: viewRevision,
		declaredAsOf: append([]DeclaredAsOf(nil), declaredAsOf...),
	}, nil
}

func (snapshot CommercialBasisSnapshot) ResolutionID() CommercialResolutionID {
	return snapshot.resolutionID
}

func (snapshot CommercialBasisSnapshot) RulePackage() RulePackageReference {
	return snapshot.rulePackage
}

func (snapshot CommercialBasisSnapshot) ViewRevision() CommercialViewRevision {
	return snapshot.viewRevision
}

// AsOfFor returns the anchor the rule package declared for one judgment. A
// missing declaration is reported as absent rather than defaulted, because
// substituting any instant here is exactly the global-time shortcut the use case
// forbids.
func (snapshot CommercialBasisSnapshot) AsOfFor(kind JudgmentKind) (JudgmentAsOf, bool) {
	for _, declared := range snapshot.declaredAsOf {
		if declared.kind == kind {
			return declared, true
		}
	}
	return DeclaredAsOf{}, false
}

func (snapshot CommercialBasisSnapshot) valid() bool {
	return snapshot.resolutionID.valid() && snapshot.rulePackage.valid() && snapshot.viewRevision.valid()
}

// ReachabilityValue mirrors network-routing's three-valued finding as an adopted
// reference. parcel-shipment never produces one: it records what the owning
// context judged, and none of the three values is an acceptance decision.
type ReachabilityValue uint8

const (
	ReachabilityValueInvalid ReachabilityValue = iota
	ReachabilityReachable
	ReachabilityUnreachable
	ReachabilityInsufficientEvidence
)

func (value ReachabilityValue) valid() bool {
	return value >= ReachabilityReachable && value <= ReachabilityInsufficientEvidence
}

func (value ReachabilityValue) String() string {
	switch value {
	case ReachabilityReachable:
		return "REACHABLE"
	case ReachabilityUnreachable:
		return "UNREACHABLE"
	case ReachabilityInsufficientEvidence:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return ""
	}
}

type ReachabilityJudgment struct {
	judgmentID ReachabilityJudgmentID
	parcelID   DeclaredParcelID
	value      ReachabilityValue
	asOf       JudgmentAsOf
}

func NewReachabilityJudgment(
	judgmentID ReachabilityJudgmentID,
	parcelID DeclaredParcelID,
	value ReachabilityValue,
	asOf JudgmentAsOf,
) (ReachabilityJudgment, error) {
	if !judgmentID.valid() || !parcelID.valid() || !value.valid() ||
		asOf.at.IsZero() || !asOf.policyVersion.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	return ReachabilityJudgment{judgmentID: judgmentID, parcelID: parcelID, value: value, asOf: asOf}, nil
}

func (judgment ReachabilityJudgment) JudgmentID() ReachabilityJudgmentID {
	return judgment.judgmentID
}

func (judgment ReachabilityJudgment) DeclaredParcelID() DeclaredParcelID {
	return judgment.parcelID
}

func (judgment ReachabilityJudgment) Value() ReachabilityValue {
	return judgment.value
}

func (judgment ReachabilityJudgment) AsOf() JudgmentAsOf {
	return judgment.asOf
}

func (judgment ReachabilityJudgment) valid() bool {
	return judgment.judgmentID.valid() && judgment.parcelID.valid() && judgment.value.valid()
}
