package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidLegalEntity = errors.New("party commercial: invalid responsible legal entity")
	// ErrCrossTenantLegalEntityParty 是法人试图绑定他租参与方身份（ADR-0003 的隔离边界，
	// 判据与 ErrCrossTenantCustomerAccount 同一条）。
	ErrCrossTenantLegalEntityParty = errors.New("party commercial: legal entity binds a party from another tenant")
	ErrInvalidIdentityRegistration = errors.New("party commercial: invalid party identity registration")
	ErrInvalidIdentityTransition   = errors.New("party commercial: invalid party identity transition")
)

// IdentityBasisReference 指向一次身份登记或停用所依据的东西。没有依据的登记事后无从
// 辩护，没有依据的停用与数据丢失无从区分——判据与 RelationshipBasisReference 同一条，
// 但身份不是关系，两个引用不共用一个类型。
type IdentityBasisReference struct{ requiredValue }

func NewIdentityBasisReference(value string) (IdentityBasisReference, error) {
	required, err := newRequiredValue("identity basis reference", value)
	return IdentityBasisReference{required}, err
}

// PartyIdentityStatus 遵循参与方身份生命周期（CONTEXT「参与方身份」节）：
// 已登记（生效时点未到）→ 已生效 → 已停用。停用只挡新决定，不删除身份。
type PartyIdentityStatus uint8

const (
	PartyIdentityStatusInvalid PartyIdentityStatus = iota
	IdentityRegistered
	IdentityEffective
	IdentityDeactivated
)

func (status PartyIdentityStatus) String() string {
	switch status {
	case IdentityRegistered:
		return "REGISTERED"
	case IdentityEffective:
		return "EFFECTIVE"
	case IdentityDeactivated:
		return "DEACTIVATED"
	default:
		return ""
	}
}

// IdentityLifecycle 是三类参与方身份共用的生命周期事实：生效自何时、是否已被停用。
// 状态由时点导出而不是另存一格——「已登记」与「已生效」的边界是生效时点，「已停用」
// 的边界是停用时点；存一列状态等于存一份会过期的推导结果。
type IdentityLifecycle struct {
	effectiveFrom     time.Time
	deactivatedAt     time.Time
	deactivationBasis IdentityBasisReference
}

func NewIdentityLifecycle(effectiveFrom time.Time) (IdentityLifecycle, error) {
	if effectiveFrom.IsZero() {
		return IdentityLifecycle{}, ErrInvalidIdentityRegistration
	}
	return IdentityLifecycle{effectiveFrom: effectiveFrom.UTC()}, nil
}

// Deactivate 以显式依据与时点停用一个身份。允许停用生效时点未到的登记——那是撤下
// 一笔尚未生效的登记；不允许的是二次停用与无依据停用。
func (lifecycle IdentityLifecycle) Deactivate(
	basis IdentityBasisReference,
	at time.Time,
) (IdentityLifecycle, error) {
	if !lifecycle.deactivatedAt.IsZero() || !basis.valid() || at.IsZero() {
		return IdentityLifecycle{}, ErrInvalidIdentityTransition
	}
	lifecycle.deactivatedAt = at.UTC()
	lifecycle.deactivationBasis = basis
	return lifecycle, nil
}

// StatusAt 回答该身份在明确时点处于生命周期哪一格。停用判断在先：一笔生效前被撤下
// 的登记自撤下时点起就是`已停用`，不会先「生效」一下。
func (lifecycle IdentityLifecycle) StatusAt(at time.Time) PartyIdentityStatus {
	if lifecycle.effectiveFrom.IsZero() || at.IsZero() {
		return PartyIdentityStatusInvalid
	}
	if !lifecycle.deactivatedAt.IsZero() && !at.Before(lifecycle.deactivatedAt) {
		return IdentityDeactivated
	}
	if !at.Before(lifecycle.effectiveFrom) {
		return IdentityEffective
	}
	return IdentityRegistered
}

func (lifecycle IdentityLifecycle) EffectiveFrom() time.Time {
	return lifecycle.effectiveFrom
}

func (lifecycle IdentityLifecycle) Deactivation() (IdentityBasisReference, time.Time, bool) {
	if lifecycle.deactivatedAt.IsZero() {
		return IdentityBasisReference{}, time.Time{}, false
	}
	return lifecycle.deactivationBasis, lifecycle.deactivatedAt, true
}

func (lifecycle IdentityLifecycle) valid() bool {
	return !lifecycle.effectiveFrom.IsZero()
}

// ResponsibleLegalEntity 把一个责任法人钉在租户与其业务参与方身份上（ADR-0003 三级
// 边界的第二级）。CONTEXT：责任法人同时具有业务参与方身份，但不能从集团层级、经营
// 组织或实际操作人员自动推断——所以它是一笔显式登记，不是一个推导视图。名称等身份
// 内容在参与方上，这里不抄第二份。
type ResponsibleLegalEntity struct {
	tenant TenantID
	id     LegalEntityReference
	party  PartyID
}

// NewResponsibleLegalEntity 要求法人与其参与方身份同租户。跨租户绑定拒绝，不建立法人。
func NewResponsibleLegalEntity(
	tenant TenantID,
	id LegalEntityReference,
	partyIdentity BusinessParty,
) (ResponsibleLegalEntity, error) {
	if !tenant.valid() || !id.valid() || !partyIdentity.id.valid() {
		return ResponsibleLegalEntity{}, ErrInvalidLegalEntity
	}
	if !partyIdentity.tenant.valid() || partyIdentity.tenant != tenant {
		return ResponsibleLegalEntity{}, ErrCrossTenantLegalEntityParty
	}
	return ResponsibleLegalEntity{tenant: tenant, id: id, party: partyIdentity.id}, nil
}

// RehydrateResponsibleLegalEntity 从登记册快照重建法人身份。快照里只有参与方引用
// （名称在参与方册上，法人册不抄第二份），因此这里收 PartyID 而不是 BusinessParty；
// 跨租守卫在写入时已把过门，重建不再复核一份不在场的参与方壳。
func RehydrateResponsibleLegalEntity(
	tenant TenantID,
	id LegalEntityReference,
	party PartyID,
) (ResponsibleLegalEntity, error) {
	if !tenant.valid() || !id.valid() || !party.valid() {
		return ResponsibleLegalEntity{}, ErrInvalidLegalEntity
	}
	return ResponsibleLegalEntity{tenant: tenant, id: id, party: party}, nil
}

func (entity ResponsibleLegalEntity) Tenant() TenantID {
	return entity.tenant
}

func (entity ResponsibleLegalEntity) ID() LegalEntityReference {
	return entity.id
}

func (entity ResponsibleLegalEntity) Party() PartyID {
	return entity.party
}

// BusinessPartyRegistration 是业务参与方身份在登记册上的一笔修订：身份内容 + 登记
// 依据 + 生命周期事实。修订从 1 起连续递增，内容更正与停用都形成新修订，不原地改写
// （CONTEXT「身份登记版本化不可覆盖」）。
type BusinessPartyRegistration struct {
	party     BusinessParty
	revision  int
	basis     IdentityBasisReference
	lifecycle IdentityLifecycle
}

func NewBusinessPartyRegistration(
	party BusinessParty,
	revision int,
	basis IdentityBasisReference,
	lifecycle IdentityLifecycle,
) (BusinessPartyRegistration, error) {
	if !party.id.valid() || !party.tenant.valid() || !party.name.valid() ||
		revision < 1 || !basis.valid() || !lifecycle.valid() {
		return BusinessPartyRegistration{}, ErrInvalidIdentityRegistration
	}
	return BusinessPartyRegistration{
		party:     party,
		revision:  revision,
		basis:     basis,
		lifecycle: lifecycle,
	}, nil
}

// Deactivate 交回下一笔修订：同一身份、修订号加一、生命周期进入已停用。
func (registration BusinessPartyRegistration) Deactivate(
	basis IdentityBasisReference,
	at time.Time,
) (BusinessPartyRegistration, error) {
	deactivated, err := registration.lifecycle.Deactivate(basis, at)
	if err != nil {
		return BusinessPartyRegistration{}, err
	}
	registration.revision++
	registration.lifecycle = deactivated
	return registration, nil
}

func (registration BusinessPartyRegistration) Party() BusinessParty {
	return registration.party
}

func (registration BusinessPartyRegistration) Revision() int {
	return registration.revision
}

func (registration BusinessPartyRegistration) Basis() IdentityBasisReference {
	return registration.basis
}

func (registration BusinessPartyRegistration) Lifecycle() IdentityLifecycle {
	return registration.lifecycle
}

// LegalEntityRegistration 是责任法人身份在登记册上的一笔修订，结构判据同
// BusinessPartyRegistration；另带身份层（ADR-0145 决定一）与身份更正依据（决定二）。
//
// 身份层是可缺的：本格落地之前登记的历史修订没有它，读回照样成立。新登记与新修订必须带，
// 那道门在写入用例里，不在这里——这里要同时装得下历史修订。
type LegalEntityRegistration struct {
	entity        ResponsibleLegalEntity
	revision      int
	basis         IdentityBasisReference
	lifecycle     IdentityLifecycle
	identity      LegalEntityIdentityLayer
	hasIdentity   bool
	correction    IdentityBasisReference
	hasCorrection bool
}

func NewLegalEntityRegistration(
	entity ResponsibleLegalEntity,
	revision int,
	basis IdentityBasisReference,
	lifecycle IdentityLifecycle,
) (LegalEntityRegistration, error) {
	if !entity.id.valid() || !entity.tenant.valid() || !entity.party.valid() ||
		revision < 1 || !basis.valid() || !lifecycle.valid() {
		return LegalEntityRegistration{}, ErrInvalidIdentityRegistration
	}
	return LegalEntityRegistration{
		entity:    entity,
		revision:  revision,
		basis:     basis,
		lifecycle: lifecycle,
	}, nil
}

// WithIdentityLayer 交回带着身份层的同一笔修订；correction 非空即这笔修订是身份更正，携带其依据。
// 身份更正依据只能随身份层出现——没有身份层的修订无从更正身份层。
func (registration LegalEntityRegistration) WithIdentityLayer(
	identity LegalEntityIdentityLayer,
	correction *IdentityBasisReference,
) (LegalEntityRegistration, error) {
	if !identity.valid() {
		return LegalEntityRegistration{}, ErrInvalidLegalEntityIdentityLayer
	}
	registration.identity = identity
	registration.hasIdentity = true
	registration.correction = IdentityBasisReference{}
	registration.hasCorrection = false
	if correction != nil {
		if !correction.valid() {
			return LegalEntityRegistration{}, ErrInvalidIdentityRegistration
		}
		registration.correction = *correction
		registration.hasCorrection = true
	}
	return registration, nil
}

// Deactivate 交回下一笔修订。身份层原样沿用；身份更正依据不沿用——停用修订不更正任何号。
func (registration LegalEntityRegistration) Deactivate(
	basis IdentityBasisReference,
	at time.Time,
) (LegalEntityRegistration, error) {
	deactivated, err := registration.lifecycle.Deactivate(basis, at)
	if err != nil {
		return LegalEntityRegistration{}, err
	}
	registration.revision++
	registration.lifecycle = deactivated
	registration.correction = IdentityBasisReference{}
	registration.hasCorrection = false
	return registration, nil
}

// IdentityLayer 交回身份层；本格落地之前登记的历史修订答 false。
func (registration LegalEntityRegistration) IdentityLayer() (LegalEntityIdentityLayer, bool) {
	return registration.identity, registration.hasIdentity
}

// IdentityCorrectionBasis 交回身份更正依据；不是身份更正的修订答 false。
func (registration LegalEntityRegistration) IdentityCorrectionBasis() (IdentityBasisReference, bool) {
	return registration.correction, registration.hasCorrection
}

func (registration LegalEntityRegistration) Entity() ResponsibleLegalEntity {
	return registration.entity
}

func (registration LegalEntityRegistration) Revision() int {
	return registration.revision
}

func (registration LegalEntityRegistration) Basis() IdentityBasisReference {
	return registration.basis
}

func (registration LegalEntityRegistration) Lifecycle() IdentityLifecycle {
	return registration.lifecycle
}

// CustomerAccountRegistration 是货主客户账户在登记册上的一笔修订（ADR-0003 三级边界
// 的第三级），结构判据同 BusinessPartyRegistration。
type CustomerAccountRegistration struct {
	account   CustomerAccount
	revision  int
	basis     IdentityBasisReference
	lifecycle IdentityLifecycle
}

func NewCustomerAccountRegistration(
	account CustomerAccount,
	revision int,
	basis IdentityBasisReference,
	lifecycle IdentityLifecycle,
) (CustomerAccountRegistration, error) {
	if !account.id.valid() || !account.tenant.valid() || !account.customerParty.valid() ||
		revision < 1 || !basis.valid() || !lifecycle.valid() {
		return CustomerAccountRegistration{}, ErrInvalidIdentityRegistration
	}
	return CustomerAccountRegistration{
		account:   account,
		revision:  revision,
		basis:     basis,
		lifecycle: lifecycle,
	}, nil
}

func (registration CustomerAccountRegistration) Deactivate(
	basis IdentityBasisReference,
	at time.Time,
) (CustomerAccountRegistration, error) {
	deactivated, err := registration.lifecycle.Deactivate(basis, at)
	if err != nil {
		return CustomerAccountRegistration{}, err
	}
	registration.revision++
	registration.lifecycle = deactivated
	return registration, nil
}

func (registration CustomerAccountRegistration) Account() CustomerAccount {
	return registration.account
}

func (registration CustomerAccountRegistration) Revision() int {
	return registration.revision
}

func (registration CustomerAccountRegistration) Basis() IdentityBasisReference {
	return registration.basis
}

func (registration CustomerAccountRegistration) Lifecycle() IdentityLifecycle {
	return registration.lifecycle
}
