package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidBusinessParty          = errors.New("party commercial: invalid business party")
	ErrInvalidPartyRelationship      = errors.New("party commercial: invalid party relationship")
	ErrInvalidRelationshipTransition = errors.New("party commercial: invalid party relationship transition")
	ErrInvalidCustomerAccount        = errors.New("party commercial: invalid customer account")
	// ErrCrossTenantCustomerAccount 是账户试图绑定他租参与方（ADR-0041 / AT-PC-014 并列 F）。
	ErrCrossTenantCustomerAccount = errors.New("party commercial: customer account binds a party from another tenant")
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

// RelationshipBasisReference 指向一段关系成立所依据的东西，以及终止它所依据的东西。
// 没有记录依据的关系事后无从辩护，而没有依据的撤销与数据丢失无从区分。
type RelationshipBasisReference struct{ requiredValue }

func NewRelationshipBasisReference(value string) (RelationshipBasisReference, error) {
	required, err := newRequiredValue("relationship basis reference", value)
	return RelationshipBasisReference{required}, err
}

// BusinessParty 只是一个稳定身份，别无其他。它有意不携带角色、类型或分类：同一
// 参与方在一段关系里是货主、在另一段里是供应商，在这里给它分类等于把一次交易的
// 角色变成永久属性。身份就是 ID；同名的两个参与方仍然是两个参与方。
//
// TenantID 是隔离边界（ADR-0041），不是角色分类。
type BusinessParty struct {
	tenant TenantID
	id     PartyID
	name   PartyName
}

func NewBusinessParty(tenant TenantID, id PartyID, name PartyName) (BusinessParty, error) {
	if !tenant.valid() || !id.valid() || !name.valid() {
		return BusinessParty{}, ErrInvalidBusinessParty
	}
	return BusinessParty{tenant: tenant, id: id, name: name}, nil
}

func (party BusinessParty) Tenant() TenantID {
	return party.tenant
}

func (party BusinessParty) ID() PartyID {
	return party.id
}

func (party BusinessParty) Name() PartyName {
	return party.name
}

// PartyRole 是一段关系中某个参与方对另一方所持的角色。取值随实际建模的关系增长。
type PartyRole uint8

const (
	PartyRoleInvalid PartyRole = iota
	CustomerRole
	SupplierRole
	CarrierAgentRole
	ResellerRole
	AccountHolderRole
	// AuthorizedDeliveringPartyRole：持有方是交付方，相对方是客户账户所属参与方，范围见 AuthorizedDeliveryScope
	// （PC CONTEXT「参与方关系」，ADR-0212 决定一）。
	AuthorizedDeliveringPartyRole
)

func (role PartyRole) valid() bool {
	return role >= CustomerRole && role <= AuthorizedDeliveringPartyRole
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
	case AuthorizedDeliveringPartyRole:
		return "AUTHORIZED_DELIVERING_PARTY"
	default:
		return ""
	}
}

// RelationshipStatus 遵循本上下文的参与方关系生命周期。关系是时态事实：终止一段
// 关系只是自其生效边界起不再支持新的商业决定，绝不删除它——既有合同和快照仍然引用
// 它们形成当时为真的事实。
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

// PartyRelationshipSpec 携带本上下文要求一段关系必须保存的全部内容：双方、角色、
// 方向、适用范围、依据和有效区间。方向由「哪一方对哪一方持有该角色」表达，
// 而不是另设一个标志位。
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
	// 授权交付判断要读出范围里的客户账户与节点，读不出的范围登进册里只会在判断时答否，而那时没人知道是写法错了。
	if spec.Role == AuthorizedDeliveringPartyRole {
		if _, err := ParseAuthorizedDeliveryScope(spec.Scope); err != nil {
			return PartyRelationship{}, fmt.Errorf("%w: %w", ErrInvalidPartyRelationship, err)
		}
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

// Approve 把一个完整的候选关系推入`已生效`。在此之前该关系不支持任何决定：
// 候选关系是提议，不是商业事实。
func (relationship PartyRelationship) Approve(approval ApprovalReference, approvedAt time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipCandidate || !approval.valid() || approvedAt.IsZero() {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	relationship.status = RelationshipEffective
	relationship.approval = approval
	relationship.approvedAt = approvedAt.UTC()
	return relationship, nil
}

// Expire 记下一段关系已到期：有效区间要有终点，且到期时刻不早于终点——终点未到的关系没有可到期的东西，开区间的
// 关系只能撤销或替代。到期同样要依据：登记到期的人凭什么认定它到了期（期满通知、合同终止条款），事后要辩护的是这一笔。
func (relationship PartyRelationship) Expire(basis RelationshipBasisReference, at time.Time) (PartyRelationship, error) {
	endsAt, bounded := relationship.effective.EndsAt()
	if relationship.status != RelationshipEffective || !bounded || !basis.valid() || at.IsZero() || at.Before(endsAt) {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	return relationship.end(RelationshipExpired, at, basis), nil
}

// Revoke 以显式决定终止一段关系，并记录该决定依据什么，因此撤销永远不会与
// 「记录缺失」混为一谈。
func (relationship PartyRelationship) Revoke(basis RelationshipBasisReference, at time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipEffective || !basis.valid() || at.IsZero() {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	return relationship.end(RelationshipRevoked, at, basis), nil
}

// SupersededBy 以一个接替的持有方终止本关系。替代只换持有方：接替关系的相对方、角色与范围
// 照本关系（SuccessorSpec），相对方、角色或范围也要变的，是撤销本关系、另登一段新关系。
// 接替方不能是原持有方（那不是接替，是同一方续登），也不能是相对方（接替之后它要对自己
// 持有这个角色）。
func (relationship PartyRelationship) SupersededBy(successor PartyID, at time.Time) (PartyRelationship, error) {
	if relationship.status != RelationshipEffective || !successor.valid() || at.IsZero() ||
		successor == relationship.holder || successor == relationship.counterparty {
		return PartyRelationship{}, ErrInvalidRelationshipTransition
	}
	ended := relationship.end(RelationshipSuperseded, at, RelationshipBasisReference{})
	ended.successor = successor
	return ended, nil
}

// SuccessorSpec 交回替代之后的那一段接替关系：持有方换成接替方，相对方、角色与适用范围不变，接住本关系在替代时刻之后
// 余下的有效区间——起点取替代时刻与本关系起点中较晚的那个，终点同本关系（开区间仍是开区间）；依据是这次替代自己的依据。
//
// 替代只换持有方。相对方、角色或范围也要变的，是撤销本关系、另登一段新关系：接替关系若可以随意改内容，「替代」就成了
// 一次不经撤销的改写，册上读不出旧关系的哪一部分由谁接了过去。余下的区间为空（替代时刻不早于本关系终点）时没有可接替的
// 东西，拒。
//
// 本方法不判状态：重放一次替代时本关系已是`已替代`，仍要从它推出同一段接替关系来核对。状态门在 SupersededBy。
func (relationship PartyRelationship) SuccessorSpec(
	successor PartyID,
	basis RelationshipBasisReference,
	at time.Time,
) (PartyRelationshipSpec, error) {
	if !successor.valid() || successor == relationship.holder || successor == relationship.counterparty ||
		!basis.valid() || at.IsZero() {
		return PartyRelationshipSpec{}, ErrInvalidRelationshipTransition
	}
	startsAt := at.UTC()
	if startsAt.Before(relationship.effective.StartsAt()) {
		startsAt = relationship.effective.StartsAt()
	}
	endsAt, _ := relationship.effective.EndsAt()
	interval, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return PartyRelationshipSpec{}, fmt.Errorf("%w: 替代时刻之后本关系没有余下的有效区间：%w", ErrInvalidRelationshipTransition, err)
	}
	return PartyRelationshipSpec{
		Holder:       successor,
		Counterparty: relationship.counterparty,
		Role:         relationship.role,
		Scope:        relationship.scope,
		Basis:        basis,
		Effective:    interval,
	}, nil
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

// AppliesAt 回答本关系能否支撑一个新的商业决定。只有落在有效区间内的`已生效`关系
// 可以；已终止的关系仍可读出，作为其他快照引用的历史。
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

// AuthorizedDeliveryScope 交回授权交付方关系的适用范围；别的角色的范围是不透明引用，答 false。
func (relationship PartyRelationship) AuthorizedDeliveryScope() (AuthorizedDeliveryScope, bool) {
	if relationship.role != AuthorizedDeliveringPartyRole {
		return AuthorizedDeliveryScope{}, false
	}
	scope, err := ParseAuthorizedDeliveryScope(relationship.scope)
	if err != nil {
		return AuthorizedDeliveryScope{}, false
	}
	return scope, true
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

// Approval 交回批准事实。只有经 Approve 出过候选格的关系才有它——候选关系答 false。
func (relationship PartyRelationship) Approval() (ApprovalReference, time.Time, bool) {
	if !relationship.approval.valid() {
		return ApprovalReference{}, time.Time{}, false
	}
	return relationship.approval, relationship.approvedAt, true
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

// RelationshipID 是一段参与方关系在登记册上的稳定标识。关系本体（PartyRelationship）
// 刻意不带标识——双方+角色+范围+区间是它的内容；登记册需要一个可回指的键，键在
// 登记信封上。
type RelationshipID struct{ requiredValue }

func NewRelationshipID(value string) (RelationshipID, error) {
	required, err := newRequiredValue("relationship ID", value)
	return RelationshipID{required}, err
}

// PartyRelationshipRegistration 给一段参与方关系一个登记册身份：租户 + 关系标识 +
// 修订。关系正文沿既有 PartyRelationship 模型，不另起第二套生命周期；内容、角色或
// 范围变化形成新修订（CONTEXT「不原地改写此前有效事实」），修订从 1 起连续递增。
type PartyRelationshipRegistration struct {
	tenant       TenantID
	id           RelationshipID
	revision     int
	relationship PartyRelationship
}

func NewPartyRelationshipRegistration(
	tenant TenantID,
	id RelationshipID,
	revision int,
	relationship PartyRelationship,
) (PartyRelationshipRegistration, error) {
	// 状态为零值即关系没经真构造门建成；登记册不收裸结构。
	if !tenant.valid() || !id.valid() || revision < 1 ||
		relationship.Status() == RelationshipStatusInvalid {
		return PartyRelationshipRegistration{}, ErrInvalidPartyRelationship
	}
	return PartyRelationshipRegistration{
		tenant:       tenant,
		id:           id,
		revision:     revision,
		relationship: relationship,
	}, nil
}

func (registration PartyRelationshipRegistration) Tenant() TenantID {
	return registration.tenant
}

func (registration PartyRelationshipRegistration) ID() RelationshipID {
	return registration.id
}

func (registration PartyRelationshipRegistration) Revision() int {
	return registration.revision
}

func (registration PartyRelationshipRegistration) Relationship() PartyRelationship {
	return registration.relationship
}

// CustomerAccount 是一个货主客户的业务隔离边界。它必须指明所属租户与客户参与方：
// 账户、参与方、法人和合同标识各自不同，任何一个都不能替代另一个（ADR-0041）。
type CustomerAccount struct {
	tenant        TenantID
	id            CustomerAccountID
	customerParty PartyID
}

// NewCustomerAccount 要求账户与客户参与方同租户。跨租户绑定拒绝，不建立账户。
func NewCustomerAccount(tenant TenantID, id CustomerAccountID, customerParty BusinessParty) (CustomerAccount, error) {
	if !tenant.valid() || !id.valid() || !customerParty.id.valid() {
		return CustomerAccount{}, ErrInvalidCustomerAccount
	}
	if !customerParty.tenant.valid() || customerParty.tenant != tenant {
		return CustomerAccount{}, ErrCrossTenantCustomerAccount
	}
	return CustomerAccount{tenant: tenant, id: id, customerParty: customerParty.id}, nil
}

// RehydrateCustomerAccount 从登记册快照重建账户。快照里只有客户参与方引用（名称在
// 参与方册上），因此收 PartyID；跨租守卫在写入时已把过门，判据同
// RehydrateResponsibleLegalEntity。
func RehydrateCustomerAccount(
	tenant TenantID,
	id CustomerAccountID,
	customerParty PartyID,
) (CustomerAccount, error) {
	if !tenant.valid() || !id.valid() || !customerParty.valid() {
		return CustomerAccount{}, ErrInvalidCustomerAccount
	}
	return CustomerAccount{tenant: tenant, id: id, customerParty: customerParty}, nil
}

func (account CustomerAccount) Tenant() TenantID {
	return account.tenant
}

func (account CustomerAccount) ID() CustomerAccountID {
	return account.id
}

func (account CustomerAccount) CustomerParty() PartyID {
	return account.customerParty
}
