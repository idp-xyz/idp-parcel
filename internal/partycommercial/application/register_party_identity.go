package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PartyRegistryOutcome 是身份/关系登记用例的结果代数：`已登记`与`已停用`是本次落库；
// `重复`按同键同内容重放返回原结果；`内容冲突`要操作者对着册面修正、绝不覆盖
// （ADR-0031）；`未受理`是输入被领域门或引用检查拒绝（含修订错位与悬空参与方），
// 一个字节没写；`未找到`只出现在停用——要停用的身份从未登记。
type PartyRegistryOutcome uint8

const (
	PartyRegistryOutcomeInvalid PartyRegistryOutcome = iota
	PartyIdentityRegistered
	PartyIdentityDeactivated
	PartyIdentityAlreadyRegistered
	PartyIdentityContentConflict
	PartyIdentityNotAccepted
	PartyIdentityNotFound
)

func (outcome PartyRegistryOutcome) String() string {
	switch outcome {
	case PartyIdentityRegistered:
		return "REGISTERED"
	case PartyIdentityDeactivated:
		return "DEACTIVATED"
	case PartyIdentityAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case PartyIdentityContentConflict:
		return "CONTENT_CONFLICT"
	case PartyIdentityNotAccepted:
		return "NOT_ACCEPTED"
	case PartyIdentityNotFound:
		return "NOT_FOUND"
	default:
		return ""
	}
}

// PartyRegistryResult 携带落点与`未受理`的原因。原因随结果交回而不是折进 error：
// 输入被拒不是技术失败，事务保持可用，批内后项照常推进（AT-PC-011 同款纪律）。
type PartyRegistryResult struct {
	outcome PartyRegistryOutcome
	cause   error
}

func (result PartyRegistryResult) Outcome() PartyRegistryOutcome {
	return result.outcome
}

// Cause 只在`未受理`时非空：修订错位、悬空参与方与领域门拒绝的恢复动作不同，
// 原因必须随结果交回，不靠格分辨。
func (result PartyRegistryResult) Cause() error {
	return result.cause
}

func notAccepted(cause error) PartyRegistryResult {
	return PartyRegistryResult{outcome: PartyIdentityNotAccepted, cause: cause}
}

// RegisterBusinessPartyCommand 登记业务参与方身份的一笔修订。
type RegisterBusinessPartyCommand struct {
	Party         domain.BusinessParty
	Revision      int
	Basis         domain.IdentityBasisReference
	EffectiveFrom time.Time
}

// RegisterLegalEntityCommand 登记责任法人身份的一笔修订。Party 是引用：法人必须钉在
// 已登记且届时已生效的参与方身份上，参与方内容（名称）在参与方册上，不随法人重复登记。
type RegisterLegalEntityCommand struct {
	Tenant        domain.TenantID
	Entity        domain.LegalEntityReference
	Party         domain.PartyID
	Revision      int
	Basis         domain.IdentityBasisReference
	EffectiveFrom time.Time
}

// RegisterCustomerAccountCommand 登记货主客户账户的一笔修订，判据同法人登记。
type RegisterCustomerAccountCommand struct {
	Tenant        domain.TenantID
	Account       domain.CustomerAccountID
	CustomerParty domain.PartyID
	Revision      int
	Basis         domain.IdentityBasisReference
	EffectiveFrom time.Time
}

// RelationshipApproval 是关系登记随带的批准事实。登记一段从纸面搬进来的已批准关系
// 时随登记一并给出；缺席则登记为候选关系，批准另行形成新修订。
type RelationshipApproval struct {
	Reference  domain.ApprovalReference
	ApprovedAt time.Time
}

// RegisterPartyRelationshipCommand 登记一段参与方关系的一笔修订。正文沿
// domain.PartyRelationshipSpec，不在应用层抄第二份字段清单。
type RegisterPartyRelationshipCommand struct {
	Tenant   domain.TenantID
	ID       domain.RelationshipID
	Revision int
	Spec     domain.PartyRelationshipSpec
	Approval *RelationshipApproval
}

// PartyIdentityKind 点名停用命令指向哪本身份册。封闭三值：关系不在内——关系的
// 终止走撤销/到期/替代三格（domain.PartyRelationship），不叫停用。
type PartyIdentityKind uint8

const (
	PartyIdentityKindInvalid PartyIdentityKind = iota
	BusinessPartyIdentity
	LegalEntityIdentity
	CustomerAccountIdentity
)

func (kind PartyIdentityKind) String() string {
	switch kind {
	case BusinessPartyIdentity:
		return "BUSINESS_PARTY"
	case LegalEntityIdentity:
		return "LEGAL_ENTITY"
	case CustomerAccountIdentity:
		return "CUSTOMER_ACCOUNT"
	default:
		return ""
	}
}

// DeactivatePartyIdentityCommand 停用一个身份。Revision 是停用落点的修订号
// （= 册上最新修订 + 1）：操作者声明自己看到的册面，错位说明册面已被并发推进或
// 意图已陈旧，拒绝而不是替操作者猜。
type DeactivatePartyIdentityCommand struct {
	Tenant   domain.TenantID
	Kind     PartyIdentityKind
	ID       string
	Revision int
	Basis    domain.IdentityBasisReference
	At       time.Time
}

// RegisterPartyIdentityHandler 是参与方身份与关系登记的写侧编排：领域门 → 引用与
// 修订连续性检查 → 登记册落库。调用方逐命令各起事务（批不是聚合，AT-PC-011）。
type RegisterPartyIdentityHandler struct {
	registry ports.PartyIdentityRegistry
}

func NewRegisterPartyIdentityHandler(registry ports.PartyIdentityRegistry) *RegisterPartyIdentityHandler {
	return &RegisterPartyIdentityHandler{registry: registry}
}

// RegisterBusinessParty 登记业务参与方身份修订。
func (handler *RegisterPartyIdentityHandler) RegisterBusinessParty(
	ctx context.Context,
	command RegisterBusinessPartyCommand,
) (PartyRegistryResult, error) {
	lifecycle, err := domain.NewIdentityLifecycle(command.EffectiveFrom)
	if err != nil {
		return notAccepted(err), nil
	}
	registration, err := domain.NewBusinessPartyRegistration(
		command.Party, command.Revision, command.Basis, lifecycle,
	)
	if err != nil {
		return notAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatestBusinessParty(
		ctx, command.Party.Tenant(), command.Party.ID(),
	)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register business party: %w", err)
	}
	var latestRevision int
	var deactivated bool
	if found {
		latestRevision = latest.Revision()
		_, _, deactivated = latest.Lifecycle().Deactivation()
	}
	if result, rejected := checkRevisionSlot(command.Revision, found, latestRevision, deactivated); rejected {
		return result, nil
	}

	outcome, err := handler.registry.SaveBusinessParty(ctx, registration)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register business party: %w", err)
	}
	return registrationResult(outcome), nil
}

// RegisterLegalEntity 登记责任法人身份修订。参与方引用必须已登记且在法人生效时点
// 已生效——法人钉在悬空或未生效的参与方上，册面会在名称栏说不出话。
func (handler *RegisterPartyIdentityHandler) RegisterLegalEntity(
	ctx context.Context,
	command RegisterLegalEntityCommand,
) (PartyRegistryResult, error) {
	lifecycle, err := domain.NewIdentityLifecycle(command.EffectiveFrom)
	if err != nil {
		return notAccepted(err), nil
	}

	partyRegistration, found, err := handler.registry.LoadLatestBusinessParty(
		ctx, command.Tenant, command.Party,
	)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register legal entity: %w", err)
	}
	if !found {
		return notAccepted(fmt.Errorf("参与方 %s 未登记，法人不钉悬空身份", command.Party)), nil
	}
	if status := partyRegistration.Lifecycle().StatusAt(command.EffectiveFrom); status != domain.IdentityEffective {
		return notAccepted(fmt.Errorf(
			"参与方 %s 在法人生效时点状态为 %s，不支持新的法人登记", command.Party, status,
		)), nil
	}

	entity, err := domain.NewResponsibleLegalEntity(
		command.Tenant, command.Entity, partyRegistration.Party(),
	)
	if err != nil {
		return notAccepted(err), nil
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, command.Revision, command.Basis, lifecycle,
	)
	if err != nil {
		return notAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatestLegalEntity(ctx, command.Tenant, command.Entity)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register legal entity: %w", err)
	}
	var latestRevision int
	var deactivated bool
	if found {
		latestRevision = latest.Revision()
		_, _, deactivated = latest.Lifecycle().Deactivation()
	}
	if result, rejected := checkRevisionSlot(command.Revision, found, latestRevision, deactivated); rejected {
		return result, nil
	}

	outcome, err := handler.registry.SaveLegalEntity(ctx, registration)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register legal entity: %w", err)
	}
	return registrationResult(outcome), nil
}

// RegisterCustomerAccount 登记货主客户账户修订。跨租户绑定由 domain.NewCustomerAccount
// 拒绝（ADR-0041），这里不复述那条门。
func (handler *RegisterPartyIdentityHandler) RegisterCustomerAccount(
	ctx context.Context,
	command RegisterCustomerAccountCommand,
) (PartyRegistryResult, error) {
	lifecycle, err := domain.NewIdentityLifecycle(command.EffectiveFrom)
	if err != nil {
		return notAccepted(err), nil
	}

	partyRegistration, found, err := handler.registry.LoadLatestBusinessParty(
		ctx, command.Tenant, command.CustomerParty,
	)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register customer account: %w", err)
	}
	if !found {
		return notAccepted(fmt.Errorf("客户参与方 %s 未登记，账户不钉悬空身份", command.CustomerParty)), nil
	}
	if status := partyRegistration.Lifecycle().StatusAt(command.EffectiveFrom); status != domain.IdentityEffective {
		return notAccepted(fmt.Errorf(
			"客户参与方 %s 在账户生效时点状态为 %s，不支持新的账户登记", command.CustomerParty, status,
		)), nil
	}

	account, err := domain.NewCustomerAccount(
		command.Tenant, command.Account, partyRegistration.Party(),
	)
	if err != nil {
		return notAccepted(err), nil
	}
	registration, err := domain.NewCustomerAccountRegistration(
		account, command.Revision, command.Basis, lifecycle,
	)
	if err != nil {
		return notAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatestCustomerAccount(ctx, command.Tenant, command.Account)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register customer account: %w", err)
	}
	var latestRevision int
	var deactivated bool
	if found {
		latestRevision = latest.Revision()
		_, _, deactivated = latest.Lifecycle().Deactivation()
	}
	if result, rejected := checkRevisionSlot(command.Revision, found, latestRevision, deactivated); rejected {
		return result, nil
	}

	outcome, err := handler.registry.SaveCustomerAccount(ctx, registration)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register customer account: %w", err)
	}
	return registrationResult(outcome), nil
}

// RegisterRelationship 登记一段参与方关系修订。双方都必须是已登记且在关系生效起点
// 已生效的参与方身份；带批准事实的登记走真转换（候选→Approve），不直构已生效结构。
func (handler *RegisterPartyIdentityHandler) RegisterRelationship(
	ctx context.Context,
	command RegisterPartyRelationshipCommand,
) (PartyRegistryResult, error) {
	relationship, err := domain.NewCandidateRelationship(command.Spec)
	if err != nil {
		return notAccepted(err), nil
	}
	if command.Approval != nil {
		relationship, err = relationship.Approve(command.Approval.Reference, command.Approval.ApprovedAt)
		if err != nil {
			return notAccepted(err), nil
		}
	}

	startsAt := command.Spec.Effective.StartsAt()
	for _, side := range []struct {
		label string
		party domain.PartyID
	}{
		{label: "持有方", party: command.Spec.Holder},
		{label: "相对方", party: command.Spec.Counterparty},
	} {
		registration, found, err := handler.registry.LoadLatestBusinessParty(ctx, command.Tenant, side.party)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("register party relationship: %w", err)
		}
		if !found {
			return notAccepted(fmt.Errorf("%s %s 未登记，关系不钉悬空身份", side.label, side.party)), nil
		}
		if status := registration.Lifecycle().StatusAt(startsAt); status != domain.IdentityEffective {
			return notAccepted(fmt.Errorf(
				"%s %s 在关系生效起点状态为 %s，不支持新的关系登记", side.label, side.party, status,
			)), nil
		}
	}

	registration, err := domain.NewPartyRelationshipRegistration(
		command.Tenant, command.ID, command.Revision, relationship,
	)
	if err != nil {
		return notAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatestRelationship(ctx, command.Tenant, command.ID)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register party relationship: %w", err)
	}
	var latestRevision int
	if found {
		latestRevision = latest.Revision()
	}
	if result, rejected := checkRevisionSlot(command.Revision, found, latestRevision, false); rejected {
		return result, nil
	}

	outcome, err := handler.registry.SaveRelationship(ctx, registration)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("register party relationship: %w", err)
	}
	return registrationResult(outcome), nil
}

// Deactivate 停用一个身份：取最新修订，经领域转换生成下一笔修订落库。
func (handler *RegisterPartyIdentityHandler) Deactivate(
	ctx context.Context,
	command DeactivatePartyIdentityCommand,
) (PartyRegistryResult, error) {
	switch command.Kind {
	case BusinessPartyIdentity:
		party, err := domain.NewPartyID(command.ID)
		if err != nil {
			return notAccepted(err), nil
		}
		latest, found, err := handler.registry.LoadLatestBusinessParty(ctx, command.Tenant, party)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate business party: %w", err)
		}
		if !found {
			return PartyRegistryResult{outcome: PartyIdentityNotFound}, nil
		}
		if result, done := deactivationReplay(
			command, latest.Revision(), latest.Lifecycle(),
		); done {
			return result, nil
		}
		next, err := latest.Deactivate(command.Basis, command.At)
		if err != nil {
			return notAccepted(err), nil
		}
		if next.Revision() != command.Revision {
			return notAccepted(revisionMismatch(command.Revision, next.Revision())), nil
		}
		outcome, err := handler.registry.SaveBusinessParty(ctx, next)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate business party: %w", err)
		}
		return deactivationResult(outcome), nil

	case LegalEntityIdentity:
		entity, err := domain.NewLegalEntityReference(command.ID)
		if err != nil {
			return notAccepted(err), nil
		}
		latest, found, err := handler.registry.LoadLatestLegalEntity(ctx, command.Tenant, entity)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate legal entity: %w", err)
		}
		if !found {
			return PartyRegistryResult{outcome: PartyIdentityNotFound}, nil
		}
		if result, done := deactivationReplay(
			command, latest.Revision(), latest.Lifecycle(),
		); done {
			return result, nil
		}
		next, err := latest.Deactivate(command.Basis, command.At)
		if err != nil {
			return notAccepted(err), nil
		}
		if next.Revision() != command.Revision {
			return notAccepted(revisionMismatch(command.Revision, next.Revision())), nil
		}
		outcome, err := handler.registry.SaveLegalEntity(ctx, next)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate legal entity: %w", err)
		}
		return deactivationResult(outcome), nil

	case CustomerAccountIdentity:
		account, err := domain.NewCustomerAccountID(command.ID)
		if err != nil {
			return notAccepted(err), nil
		}
		latest, found, err := handler.registry.LoadLatestCustomerAccount(ctx, command.Tenant, account)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate customer account: %w", err)
		}
		if !found {
			return PartyRegistryResult{outcome: PartyIdentityNotFound}, nil
		}
		if result, done := deactivationReplay(
			command, latest.Revision(), latest.Lifecycle(),
		); done {
			return result, nil
		}
		next, err := latest.Deactivate(command.Basis, command.At)
		if err != nil {
			return notAccepted(err), nil
		}
		if next.Revision() != command.Revision {
			return notAccepted(revisionMismatch(command.Revision, next.Revision())), nil
		}
		outcome, err := handler.registry.SaveCustomerAccount(ctx, next)
		if err != nil {
			return PartyRegistryResult{}, fmt.Errorf("deactivate customer account: %w", err)
		}
		return deactivationResult(outcome), nil

	default:
		return notAccepted(errors.New("未知身份种类")), nil
	}
}

// checkRevisionSlot 是登记修订连续性门：从未登记的身份只收修订 1；已登记的身份收
// 到已有修订（交给登记册按内容答重放/冲突）或紧邻的下一号；跳号说明操作者看到的
// 册面已陈旧。已停用身份不再收新修订——身份是稳定的，停用不是中场休息；确要重新
// 合作，登记新身份或先立重新启用的机制（那是难逆转取舍，走 ADR）。
func checkRevisionSlot(
	revision int,
	found bool,
	latestRevision int,
	latestDeactivated bool,
) (PartyRegistryResult, bool) {
	if !found {
		if revision != 1 {
			return notAccepted(fmt.Errorf("首笔登记修订必须是 1，收到 %d", revision)), true
		}
		return PartyRegistryResult{}, false
	}
	if revision > latestRevision {
		if latestDeactivated {
			return notAccepted(errors.New("身份已停用，不再收新修订")), true
		}
		if revision != latestRevision+1 {
			return notAccepted(fmt.Errorf(
				"修订必须连续：册上最新为 %d，收到 %d", latestRevision, revision,
			)), true
		}
	}
	return PartyRegistryResult{}, false
}

// deactivationReplay 处理停用命令撞上已停用册面的两格：同修订同内容是重放，同修订
// 异内容是冲突。未停用的册面交回 done=false，走正常转换。
func deactivationReplay(
	command DeactivatePartyIdentityCommand,
	latestRevision int,
	lifecycle domain.IdentityLifecycle,
) (PartyRegistryResult, bool) {
	basis, at, has := lifecycle.Deactivation()
	if !has {
		return PartyRegistryResult{}, false
	}
	if command.Revision == latestRevision && basis == command.Basis && at.Equal(command.At.UTC()) {
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}, true
	}
	if command.Revision == latestRevision {
		return PartyRegistryResult{outcome: PartyIdentityContentConflict}, true
	}
	return notAccepted(errors.New("身份已停用，停用不重复落笔")), true
}

func revisionMismatch(commanded, actual int) error {
	return fmt.Errorf("停用落点修订错位：命令声明 %d，册面推进到 %d——册面已被并发推进或意图已陈旧", commanded, actual)
}

func registrationResult(outcome ports.PartyRegistrySaveOutcome) PartyRegistryResult {
	switch outcome {
	case ports.PartyRegistrySaved:
		return PartyRegistryResult{outcome: PartyIdentityRegistered}
	case ports.PartyRegistryAlreadyRegistered:
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}
	case ports.PartyRegistryContentConflict:
		return PartyRegistryResult{outcome: PartyIdentityContentConflict}
	default:
		return PartyRegistryResult{}
	}
}

func deactivationResult(outcome ports.PartyRegistrySaveOutcome) PartyRegistryResult {
	switch outcome {
	case ports.PartyRegistrySaved:
		return PartyRegistryResult{outcome: PartyIdentityDeactivated}
	case ports.PartyRegistryAlreadyRegistered:
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}
	case ports.PartyRegistryContentConflict:
		return PartyRegistryResult{outcome: PartyIdentityContentConflict}
	default:
		return PartyRegistryResult{}
	}
}
