package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAuthorityGrant       = errors.New("party commercial: invalid authority grant")
	ErrInvalidAuthorizationRequest = errors.New("party commercial: invalid authorization request")
	ErrNotAuthorized               = errors.New("party commercial: no effective grant authorizes this request")
)

// AuthorityLevel is a commercial permission level, not a personnel grade. It is
// a versioned business grant: nothing about someone's title, org position or
// technical account confers it, and no party role substitutes for it.
type AuthorityLevel struct{ requiredValue }

func NewAuthorityLevel(value string) (AuthorityLevel, error) {
	required, err := newRequiredValue("authority level", value)
	return AuthorityLevel{required}, err
}

type StructuredReason struct{ requiredValue }

func NewStructuredReason(value string) (StructuredReason, error) {
	required, err := newRequiredValue("structured reason", value)
	return StructuredReason{required}, err
}

type EvidenceReference struct{ requiredValue }

func NewEvidenceReference(value string) (EvidenceReference, error) {
	required, err := newRequiredValue("evidence reference", value)
	return EvidenceReference{required}, err
}

// AuthorizedAction is what a grant permits. Manual review and active rejection
// are separate actions because a grant to review does not carry a grant to
// refuse the business outright.
type AuthorizedAction uint8

const (
	AuthorizedActionInvalid AuthorizedAction = iota
	ManualReviewAction
	ActiveRejectionAction
)

func (action AuthorizedAction) valid() bool {
	return action >= ManualReviewAction && action <= ActiveRejectionAction
}

func (action AuthorizedAction) String() string {
	switch action {
	case ManualReviewAction:
		return "MANUAL_REVIEW"
	case ActiveRejectionAction:
		return "ACTIVE_REJECTION"
	default:
		return ""
	}
}

// AuthorityGrant is one versioned permission: which action, at which commercial
// authority level, for which legal entity and scope, over which interval.
type AuthorityGrant struct {
	version     CommercialVersion
	action      AuthorizedAction
	legalEntity LegalEntityReference
	level       AuthorityLevel
	scope       CommercialScopeReference
	effective   EffectiveInterval
}

func NewAuthorityGrant(
	version CommercialVersion,
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (AuthorityGrant, error) {
	if version.kind != AuthorizationRuleObject ||
		version.status != CommercialVersionEffective ||
		!action.valid() || !legalEntity.valid() || !level.valid() ||
		!scope.valid() || !effective.valid() {
		return AuthorityGrant{}, ErrInvalidAuthorityGrant
	}
	return AuthorityGrant{
		version:     version,
		action:      action,
		legalEntity: legalEntity,
		level:       level,
		scope:       scope,
		effective:   effective,
	}, nil
}

func (grant AuthorityGrant) Version() CommercialVersion {
	return grant.version
}

func (grant AuthorityGrant) Action() AuthorizedAction {
	return grant.action
}

func (grant AuthorityGrant) Level() AuthorityLevel {
	return grant.level
}

func (grant AuthorityGrant) Scope() CommercialScopeReference {
	return grant.scope
}

func (grant AuthorityGrant) permits(request AuthorizationRequest) bool {
	return grant.action == request.action &&
		grant.legalEntity == request.legalEntity &&
		grant.level == request.level &&
		grant.scope == request.scope &&
		grant.effective.Contains(request.at)
}

// AuthorizationRequest asks to perform one action. It deliberately carries no
// party role: a carrier agent or channel provider holding a relationship role
// gains nothing here, because authority comes only from a versioned grant.
type AuthorizationRequest struct {
	action      AuthorizedAction
	legalEntity LegalEntityReference
	level       AuthorityLevel
	scope       CommercialScopeReference
	reason      StructuredReason
	evidence    EvidenceReference
	at          time.Time
}

// NewAuthorizationRequest requires a structured reason and evidence for every
// action. An active rejection without them could not be defended afterwards, and
// free-text justification would make refusals uncountable by cause.
func NewAuthorizationRequest(
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	reason StructuredReason,
	evidence EvidenceReference,
	at time.Time,
) (AuthorizationRequest, error) {
	if !action.valid() || !legalEntity.valid() || !level.valid() || !scope.valid() ||
		!reason.valid() || !evidence.valid() || at.IsZero() {
		return AuthorizationRequest{}, ErrInvalidAuthorizationRequest
	}
	return AuthorizationRequest{
		action:      action,
		legalEntity: legalEntity,
		level:       level,
		scope:       scope,
		reason:      reason,
		evidence:    evidence,
		at:          at.UTC(),
	}, nil
}

// Authorization records that a specific grant permitted a specific request,
// together with the reason and evidence the requester gave.
type Authorization struct {
	action   AuthorizedAction
	grant    AuthorityGrant
	reason   StructuredReason
	evidence EvidenceReference
	at       time.Time
}

func (authorization Authorization) Action() AuthorizedAction {
	return authorization.action
}

func (authorization Authorization) GrantVersion() CommercialVersion {
	return authorization.grant.version
}

func (authorization Authorization) Reason() StructuredReason {
	return authorization.reason
}

func (authorization Authorization) Evidence() EvidenceReference {
	return authorization.evidence
}

func (authorization Authorization) At() time.Time {
	return authorization.at
}

// Authorize looks for an effective grant permitting the request. Absence of a
// grant is a refusal, never a pass: authority is granted explicitly or not at
// all.
func Authorize(grants []AuthorityGrant, request AuthorizationRequest) (Authorization, error) {
	for _, grant := range grants {
		if grant.permits(request) {
			return Authorization{
				action:   request.action,
				grant:    grant,
				reason:   request.reason,
				evidence: request.evidence,
				at:       request.at,
			}, nil
		}
	}
	return Authorization{}, ErrNotAuthorized
}

// ManualReviewRequirementFor answers whether a scope demands human review.
// Silence means not required: the context states manual review is not a default
// step, so an absent declaration must never be read as demanding one.
func ManualReviewRequirementFor(
	grants []AuthorityGrant,
	scope CommercialScopeReference,
	at time.Time,
) bool {
	for _, grant := range grants {
		if grant.action == ManualReviewAction &&
			grant.scope == scope &&
			grant.effective.Contains(at) {
			return true
		}
	}
	return false
}
