// Package domain holds the party-commercial model: party identity, commercial
// relationships and the versioned commercial definitions other contexts resolve
// against. It owns no executable rate card, shipment request or settlement fact.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue                      = errors.New("party commercial: blank value")
	ErrInvalidCommercialVersion        = errors.New("party commercial: invalid commercial version")
	ErrInvalidEffectiveInterval        = errors.New("party commercial: invalid effective interval")
	ErrIncompleteCommercialPublication = errors.New("party commercial: incomplete commercial publication")
	ErrCommercialContentIsFixed        = errors.New("party commercial: published commercial content is fixed")
	ErrInvalidCommercialTransition     = errors.New("party commercial: invalid commercial version transition")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type CommercialObjectID struct{ requiredValue }

func NewCommercialObjectID(value string) (CommercialObjectID, error) {
	required, err := newRequiredValue("commercial object ID", value)
	return CommercialObjectID{required}, err
}

type CommercialVersionLabel struct{ requiredValue }

func NewCommercialVersionLabel(value string) (CommercialVersionLabel, error) {
	required, err := newRequiredValue("commercial version label", value)
	return CommercialVersionLabel{required}, err
}

type CommercialScopeReference struct{ requiredValue }

func NewCommercialScopeReference(value string) (CommercialScopeReference, error) {
	required, err := newRequiredValue("commercial scope reference", value)
	return CommercialScopeReference{required}, err
}

type CommercialContentDigest struct{ requiredValue }

func NewCommercialContentDigest(value string) (CommercialContentDigest, error) {
	required, err := newRequiredValue("commercial content digest", value)
	return CommercialContentDigest{required}, err
}

type ApprovalReference struct{ requiredValue }

func NewApprovalReference(value string) (ApprovalReference, error) {
	required, err := newRequiredValue("approval reference", value)
	return ApprovalReference{required}, err
}

type CommercialSourceReference struct{ requiredValue }

func NewCommercialSourceReference(value string) (CommercialSourceReference, error) {
	required, err := newRequiredValue("commercial source reference", value)
	return CommercialSourceReference{required}, err
}

type RetirementReference struct{ requiredValue }

func NewRetirementReference(value string) (RetirementReference, error) {
	required, err := newRequiredValue("retirement reference", value)
	return RetirementReference{required}, err
}

// CommercialObjectKind is the closed set of objects that follow the shared
// commercial-version invariants. Customer accounts and legal entities are
// deliberately absent: they are party identities with their own lifecycle, not
// commercial versions.
type CommercialObjectKind uint8

const (
	CommercialObjectKindInvalid CommercialObjectKind = iota
	ServiceProductObject
	CustomerContractObject
	SupplierAgreementObject
	AcceptanceRulePackageObject
	PreAcceptanceFinancialControlPolicyObject
	PriceRuleObject
	SettlementPolicyObject
	CreditPolicyObject
	AuthorizationRuleObject
)

func (kind CommercialObjectKind) valid() bool {
	return kind >= ServiceProductObject && kind <= AuthorizationRuleObject
}

func (kind CommercialObjectKind) String() string {
	switch kind {
	case ServiceProductObject:
		return "SERVICE_PRODUCT"
	case CustomerContractObject:
		return "CUSTOMER_CONTRACT"
	case SupplierAgreementObject:
		return "SUPPLIER_AGREEMENT"
	case AcceptanceRulePackageObject:
		return "ACCEPTANCE_RULE_PACKAGE"
	case PreAcceptanceFinancialControlPolicyObject:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY"
	case PriceRuleObject:
		return "PRICE_RULE"
	case SettlementPolicyObject:
		return "SETTLEMENT_POLICY"
	case CreditPolicyObject:
		return "CREDIT_POLICY"
	case AuthorizationRuleObject:
		return "AUTHORIZATION_RULE"
	default:
		return ""
	}
}

// CommercialVersionStatus follows the context lifecycle. Approval is a
// completeness condition of publication rather than a status of its own, so
// there is no APPROVED between draft and published.
type CommercialVersionStatus uint8

const (
	CommercialVersionStatusInvalid CommercialVersionStatus = iota
	CommercialVersionDraft
	CommercialVersionPublished
	CommercialVersionEffective
	CommercialVersionExpired
	CommercialVersionRetired
	CommercialVersionSuperseded
)

func (status CommercialVersionStatus) String() string {
	switch status {
	case CommercialVersionDraft:
		return "DRAFT"
	case CommercialVersionPublished:
		return "PUBLISHED"
	case CommercialVersionEffective:
		return "EFFECTIVE"
	case CommercialVersionExpired:
		return "EXPIRED"
	case CommercialVersionRetired:
		return "RETIRED"
	case CommercialVersionSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

type EffectiveInterval struct {
	startsAt time.Time
	endsAt   time.Time
}

// NewEffectiveInterval accepts an open end: a version may apply until it is
// expired, retired or superseded rather than at a date fixed on publication.
func NewEffectiveInterval(startsAt, endsAt time.Time) (EffectiveInterval, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return EffectiveInterval{}, ErrInvalidEffectiveInterval
	}
	return EffectiveInterval{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

func (interval EffectiveInterval) StartsAt() time.Time {
	return interval.startsAt
}

func (interval EffectiveInterval) EndsAt() (time.Time, bool) {
	if interval.endsAt.IsZero() {
		return time.Time{}, false
	}
	return interval.endsAt, true
}

func (interval EffectiveInterval) Contains(at time.Time) bool {
	if interval.startsAt.IsZero() || at.Before(interval.startsAt) {
		return false
	}
	return interval.endsAt.IsZero() || at.Before(interval.endsAt)
}

func (interval EffectiveInterval) valid() bool {
	return !interval.startsAt.IsZero() &&
		(interval.endsAt.IsZero() || interval.endsAt.After(interval.startsAt))
}

// ApprovalBasis is the approval and provenance a publication must carry. It is
// a single value object because a reference without its source, or either
// without a time, cannot support the publication it is meant to justify.
type ApprovalBasis struct {
	reference  ApprovalReference
	source     CommercialSourceReference
	approvedAt time.Time
}

func NewApprovalBasis(
	reference ApprovalReference,
	source CommercialSourceReference,
	approvedAt time.Time,
) (ApprovalBasis, error) {
	if !reference.valid() || !source.valid() || approvedAt.IsZero() {
		return ApprovalBasis{}, ErrIncompleteCommercialPublication
	}
	return ApprovalBasis{reference: reference, source: source, approvedAt: approvedAt.UTC()}, nil
}

func (basis ApprovalBasis) Reference() ApprovalReference {
	return basis.reference
}

func (basis ApprovalBasis) Source() CommercialSourceReference {
	return basis.source
}

func (basis ApprovalBasis) ApprovedAt() time.Time {
	return basis.approvedAt
}

func (basis ApprovalBasis) valid() bool {
	return basis.reference.valid() && basis.source.valid() && !basis.approvedAt.IsZero()
}

type CommercialVersionSpec struct {
	Kind          CommercialObjectKind
	ObjectID      CommercialObjectID
	Version       CommercialVersionLabel
	Scope         CommercialScopeReference
	ContentDigest CommercialContentDigest
	Effective     EffectiveInterval
}

// CommercialVersion is one controlled release of a commercial definition. It is
// a value: every transition returns a new version and leaves the receiver as it
// was, which is what makes "published content is never edited in place"
// structural rather than a rule someone has to remember.
type CommercialVersion struct {
	kind          CommercialObjectKind
	objectID      CommercialObjectID
	version       CommercialVersionLabel
	scope         CommercialScopeReference
	contentDigest CommercialContentDigest
	effective     EffectiveInterval
	status        CommercialVersionStatus
	approval      ApprovalBasis
	publishedAt   time.Time
	effectiveAt   time.Time
	closedAt      time.Time
	retirementRef RetirementReference
	successor     CommercialVersionLabel
}

func NewCommercialDraft(spec CommercialVersionSpec) (CommercialVersion, error) {
	if !spec.Kind.valid() ||
		!spec.ObjectID.valid() ||
		!spec.Version.valid() ||
		!spec.Scope.valid() ||
		!spec.ContentDigest.valid() ||
		!spec.Effective.valid() {
		return CommercialVersion{}, ErrInvalidCommercialVersion
	}
	return CommercialVersion{
		kind:          spec.Kind,
		objectID:      spec.ObjectID,
		version:       spec.Version,
		scope:         spec.Scope,
		contentDigest: spec.ContentDigest,
		effective:     spec.Effective,
		status:        CommercialVersionDraft,
	}, nil
}

// Revise changes draft content. A published version refuses: its content is
// fixed and a change must form a further version instead.
func (version CommercialVersion) Revise(digest CommercialContentDigest) (CommercialVersion, error) {
	if version.status != CommercialVersionDraft {
		return CommercialVersion{}, ErrCommercialContentIsFixed
	}
	if !digest.valid() {
		return CommercialVersion{}, ErrInvalidCommercialVersion
	}
	version.contentDigest = digest
	return version, nil
}

// Publish fixes the content. Approval and provenance must be complete and the
// publication cannot precede the approval that justifies it.
func (version CommercialVersion) Publish(basis ApprovalBasis, publishedAt time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionDraft {
		return CommercialVersion{}, ErrCommercialContentIsFixed
	}
	if !basis.valid() || publishedAt.IsZero() || publishedAt.Before(basis.ApprovedAt()) {
		return CommercialVersion{}, ErrIncompleteCommercialPublication
	}
	version.status = CommercialVersionPublished
	version.approval = basis
	version.publishedAt = publishedAt.UTC()
	return version, nil
}

// TakeEffect moves a published version into use once its own boundary is
// reached. Publication is not effectiveness: a version published ahead of its
// interval must not answer resolution before that interval opens.
func (version CommercialVersion) TakeEffect(at time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionPublished || at.IsZero() || at.Before(version.effective.StartsAt()) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	version.status = CommercialVersionEffective
	version.effectiveAt = at.UTC()
	return version, nil
}

// Expire ends a version at its own declared boundary. A version with an open
// end has no such boundary; expiring it would invent one, so it must be retired
// or superseded instead.
func (version CommercialVersion) Expire(at time.Time) (CommercialVersion, error) {
	endsAt, bounded := version.effective.EndsAt()
	if version.status != CommercialVersionEffective || !bounded || at.IsZero() || at.Before(endsAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	return version.close(CommercialVersionExpired, at), nil
}

// Retire ends a version by explicit decision rather than by its interval, so it
// records the reference that decision came from.
func (version CommercialVersion) Retire(reference RetirementReference, at time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionEffective || !reference.valid() || at.IsZero() || at.Before(version.effectiveAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	closed := version.close(CommercialVersionRetired, at)
	closed.retirementRef = reference
	return closed, nil
}

// SupersededBy ends a version in favour of a named successor. The successor
// must be another version of the same object: superseding across objects or
// kinds would destroy the predecessor/successor relation the context requires.
func (version CommercialVersion) SupersededBy(successor CommercialVersion, at time.Time) (CommercialVersion, error) {
	sameObject := successor.kind == version.kind && successor.objectID == version.objectID
	if version.status != CommercialVersionEffective ||
		!sameObject ||
		successor.version == version.version ||
		successor.status == CommercialVersionDraft ||
		at.IsZero() ||
		at.Before(version.effectiveAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	closed := version.close(CommercialVersionSuperseded, at)
	closed.successor = successor.version
	return closed, nil
}

// close ends participation in new resolution while leaving the fixed content,
// approval basis and publication time untouched, because existing shipments,
// transactions and settlements keep referring to them.
func (version CommercialVersion) close(status CommercialVersionStatus, at time.Time) CommercialVersion {
	version.status = status
	version.closedAt = at.UTC()
	return version
}

// AppliesAt answers whether this version may still be selected for a new
// resolution. Only an effective version inside its interval can; an ended one
// remains readable as history but never applies again.
func (version CommercialVersion) AppliesAt(at time.Time) bool {
	return version.status == CommercialVersionEffective && version.effective.Contains(at)
}

func (version CommercialVersion) Kind() CommercialObjectKind {
	return version.kind
}

func (version CommercialVersion) ObjectID() CommercialObjectID {
	return version.objectID
}

func (version CommercialVersion) Version() CommercialVersionLabel {
	return version.version
}

func (version CommercialVersion) Scope() CommercialScopeReference {
	return version.scope
}

func (version CommercialVersion) ContentDigest() CommercialContentDigest {
	return version.contentDigest
}

func (version CommercialVersion) Effective() EffectiveInterval {
	return version.effective
}

func (version CommercialVersion) Status() CommercialVersionStatus {
	return version.status
}

func (version CommercialVersion) ApprovalBasis() (ApprovalBasis, bool) {
	if !version.approval.valid() {
		return ApprovalBasis{}, false
	}
	return version.approval, true
}

func (version CommercialVersion) PublishedAt() (time.Time, bool) {
	if version.publishedAt.IsZero() {
		return time.Time{}, false
	}
	return version.publishedAt, true
}

func (version CommercialVersion) EffectiveAt() (time.Time, bool) {
	if version.effectiveAt.IsZero() {
		return time.Time{}, false
	}
	return version.effectiveAt, true
}

func (version CommercialVersion) ClosedAt() (time.Time, bool) {
	if version.closedAt.IsZero() {
		return time.Time{}, false
	}
	return version.closedAt, true
}

func (version CommercialVersion) RetirementReference() (RetirementReference, bool) {
	if !version.retirementRef.valid() {
		return RetirementReference{}, false
	}
	return version.retirementRef, true
}

func (version CommercialVersion) Successor() (CommercialVersionLabel, bool) {
	if !version.successor.valid() {
		return CommercialVersionLabel{}, false
	}
	return version.successor, true
}
