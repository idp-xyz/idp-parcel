package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidBusinessParty          = errors.New("party commercial: invalid business party")
	ErrInvalidPartyRelationship      = errors.New("party commercial: invalid party relationship")
	ErrInvalidRelationshipTransition = errors.New("party commercial: invalid party relationship transition")
	ErrInvalidCustomerAccount        = errors.New("party commercial: invalid customer account")
)

type PartyID struct{ requiredValue }

func NewPartyID(value string) (PartyID, error) {
	required, err := newRequiredValue("party ID", value)
	return PartyID{required}, err
}

type PartyName struct{ requiredValue }

func NewPartyName(value string) (PartyName, error) {
	required, err := newRequiredValue("party name", value)
	return PartyName{required}, err
}

// RelationshipBasisReference points at what a relationship rests on, and at what
// ends it. A relationship without a recorded basis could not later be defended,
// and a revocation without one could not be distinguished from data loss.
type RelationshipBasisReference struct{ requiredValue }

func NewRelationshipBasisReference(value string) (RelationshipBasisReference, error) {
	required, err := newRequiredValue("relationship basis reference", value)
	return RelationshipBasisReference{required}, err
}

// BusinessParty is a stable identity and nothing more. It deliberately carries
// no role, type or category: a party is a shipper in one relationship and a
// supplier in another, so classifying it here would make one transaction's role
// permanent. Identity is the ID; two parties sharing a name remain two parties.
type BusinessParty struct {
	id   PartyID
	name PartyName
}

func NewBusinessParty(id PartyID, name PartyName) (BusinessParty, error) {
	if !id.valid() || !name.valid() {
		return BusinessParty{}, ErrInvalidBusinessParty
	}
	return BusinessParty{id: id, name: name}, nil
}

func (party BusinessParty) ID() PartyID {
	return party.id
}

func (party BusinessParty) Name() PartyName {
	return party.name
}

// PartyRole is the role one party holds toward another inside a relationship.
// Values grow with the relationships actually modelled.
type PartyRole uint8

const (
	PartyRoleInvalid PartyRole = iota
	CustomerRole
	SupplierRole
	CarrierAgentRole
	ResellerRole
	AccountHolderRole
)

func (role PartyRole) valid() bool {
	return role >= CustomerRole && role <= AccountHolderRole
}

func (role PartyRole) String() string {
	switch role {
	case CustomerRole:
		return "CUSTOMER"
	case SupplierRole:
		return "SUPPLIER"
	case CarrierAgentRole:
		return "CARRIER_AGENT"
	case ResellerRole:
		return "RESELLER"
	case AccountHolderRole:
		return "ACCOUNT_HOLDER"
	default:
		return ""
	}
}

// RelationshipStatus follows the context lifecycle. A relationship is a temporal
// fact: ending one stops it supporting new decisions from its boundary onward
// and never deletes it, because existing contracts and snapshots still refer to
// what was true when they were formed.
type RelationshipStatus uint8

const (
	RelationshipStatusInvalid RelationshipStatus = iota
	RelationshipCandidate
	RelationshipEffective
	RelationshipExpired
	RelationshipRevoked
	RelationshipSuperseded
)

func (status RelationshipStatus) String() string {
	switch status {
	case RelationshipCandidate:
		return "CANDIDATE"
	case RelationshipEffective:
		return "EFFECTIVE"
	case RelationshipExpired:
		return "EXPIRED"
	case RelationshipRevoked:
		return "REVOKED"
	case RelationshipSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

// PartyRelationshipSpec carries everything the context requires a relationship
// to record: both parties, the role, its direction, the applicable scope, the
// basis and the effective interval. Direction is expressed by which party holds
// the role toward which, rather than by a separate flag.
type PartyRelationshipSpec struct {
	Holder       PartyID
	Counterparty PartyID
	Role         PartyRole
	Scope        CommercialScopeReference
	Basis        RelationshipBasisReference
	Effective    EffectiveInterval
}

type PartyRelationship struct {
	holder       PartyID
	counterparty PartyID
	role         PartyRole
	scope        CommercialScopeReference
	basis        RelationshipBasisReference
	effective    EffectiveInterval
	status       RelationshipStatus
	approval     ApprovalReference
	approvedAt   time.Time
	endedAt      time.Time
	endBasis     RelationshipBasisReference
	successor    PartyID
}

func NewCandidateRelationship(spec PartyRelationshipSpec) (PartyRelationship, error) {
	if !spec.Holder.valid() || !spec.Counterparty.valid() || spec.Holder == spec.Counterparty ||
		!spec.Role.valid() || !spec.Scope.valid() || !spec.Basis.valid() || !spec.Effective.valid() {
		return PartyRelationship{}, ErrInvalidPartyRelationship
	}
	return PartyRelationship{
		holder:       spec.Holder,
		counterparty: spec.Counterparty,
		role:         spec.Role,
		scope:        spec.Scope,
		basis:        spec.Basis,
		effective:    spec.Effective,
		status:       RelationshipCandidate,
	}, nil
}

// Approve moves a complete candidate into effect. Until then the relationship
// supports no decision: a candidate is a proposal, not a commercial fact.
func (relationship PartyRelationship) Approve(approval ApprovalReference, approvedAt time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipCandidate || !approval.valid() || approvedAt.IsZero() {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	relationship.status = RelationshipEffective
	relationship.approval = approval
	relationship.approvedAt = approvedAt.UTC()
	return relationship, nil
}

func (relationship PartyRelationship) Expire(at time.Time) (PartyRelationship, error) {
	endsAt, bounded := relationship.effective.EndsAt()
	if relationship.status != RelationshipEffective || !bounded || at.IsZero() || at.Before(endsAt) {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	return relationship.end(RelationshipExpired, at, RelationshipBasisReference{}), nil
}

// Revoke ends a relationship by explicit decision and records what that decision
// rested on, so a revocation is never indistinguishable from a missing record.
func (relationship PartyRelationship) Revoke(basis RelationshipBasisReference, at time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipEffective || !basis.valid() || at.IsZero() {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	return relationship.end(RelationshipRevoked, at, basis), nil
}

// SupersededBy ends a relationship in favour of a replacement holder. Content,
// role or scope changing forms a new relationship rather than editing this one.
func (relationship PartyRelationship) SupersededBy(successor PartyID, at time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipEffective || !successor.valid() || at.IsZero() {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	ended := relationship.end(RelationshipSuperseded, at, RelationshipBasisReference{})
	ended.successor = successor
	return ended, nil
}

func (relationship PartyRelationship) end(
	status RelationshipStatus,
	at time.Time,
	basis RelationshipBasisReference,
) PartyRelationship {
	relationship.status = status
	relationship.endedAt = at.UTC()
	relationship.endBasis = basis
	return relationship
}

// AppliesAt answers whether this relationship may support a new commercial
// decision. Only an effective one inside its interval can; an ended one stays
// readable as the history other snapshots reference.
func (relationship PartyRelationship) AppliesAt(at time.Time) bool {
	return relationship.status == RelationshipEffective && relationship.effective.Contains(at)
}

func (relationship PartyRelationship) Holder() PartyID {
	return relationship.holder
}

func (relationship PartyRelationship) Counterparty() PartyID {
	return relationship.counterparty
}

func (relationship PartyRelationship) Role() PartyRole {
	return relationship.role
}

func (relationship PartyRelationship) Scope() CommercialScopeReference {
	return relationship.scope
}

func (relationship PartyRelationship) Basis() RelationshipBasisReference {
	return relationship.basis
}

func (relationship PartyRelationship) Effective() EffectiveInterval {
	return relationship.effective
}

func (relationship PartyRelationship) Status() RelationshipStatus {
	return relationship.status
}

func (relationship PartyRelationship) EndedAt() (time.Time, bool) {
	if relationship.endedAt.IsZero() {
		return time.Time{}, false
	}
	return relationship.endedAt, true
}

func (relationship PartyRelationship) EndBasis() (RelationshipBasisReference, bool) {
	if !relationship.endBasis.valid() {
		return RelationshipBasisReference{}, false
	}
	return relationship.endBasis, true
}

func (relationship PartyRelationship) Successor() (PartyID, bool) {
	if !relationship.successor.valid() {
		return PartyID{}, false
	}
	return relationship.successor, true
}

// CustomerAccount is the isolation boundary for one shipper customer. It must
// name the customer party it belongs to: account, party, legal entity and
// contract identifiers are distinct and none may stand in for another.
type CustomerAccount struct {
	id            CustomerAccountID
	customerParty PartyID
}

func NewCustomerAccount(id CustomerAccountID, customerParty PartyID) (CustomerAccount, error) {
	if !id.valid() || !customerParty.valid() {
		return CustomerAccount{}, ErrInvalidCustomerAccount
	}
	return CustomerAccount{id: id, customerParty: customerParty}, nil
}

func (account CustomerAccount) ID() CustomerAccountID {
	return account.id
}

func (account CustomerAccount) CustomerParty() PartyID {
	return account.customerParty
}
