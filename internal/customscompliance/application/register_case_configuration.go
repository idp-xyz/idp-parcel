package application

import (
	"context"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 案件配置面五本登记册的登记用例。W13 那堵墙的另一半：仓储写口解决「写得进去」，
// 这一层解决「谁能说清写进去的是不是同一份」。
//
// **本层存在的理由就是冲突判定。** 写口一律 `ON CONFLICT DO NOTHING`，同键已在册只
// 交回`已登记`，库不判内容一不一样——那个判断放在这里：读回既有登记，与本次请求逐
// 字段比，同则`已存在`（重放），异则`内容冲突`（拒绝，绝不顶替）。把比对放在编排而
// 不是 SQL 里，「重放同一份」与「换了内容」才在用例结果上分得开；压进一条 UPSERT 就
// 只剩「写成功了」一个信号，而这两件的续办动作完全相反。
//
// 事务不由本层开：与本上下文其余用例一致，环境事务由进程级入口给出，登记因此能与
// 它所属的那次业务动作同笔落地或同笔回滚。

// CaseConfigurationOutcome 是案件配置登记请求的应用处理结果。
type CaseConfigurationOutcome uint8

const (
	CaseConfigurationOutcomeInvalid CaseConfigurationOutcome = iota
	ConfigurationRegistered
	ConfigurationExisting
	ConfigurationContentConflict
	ConfigurationRevoked
	ConfigurationAlreadyRevoked
	ConfigurationNotRegistered
	ConfigurationNotAccepted
	ConfigurationUndecided
)

func (outcome CaseConfigurationOutcome) String() string {
	switch outcome {
	case ConfigurationRegistered:
		return "REGISTERED"
	case ConfigurationExisting:
		return "EXISTING"
	case ConfigurationContentConflict:
		return "CONTENT_CONFLICT"
	case ConfigurationRevoked:
		return "REVOKED"
	case ConfigurationAlreadyRevoked:
		return "ALREADY_REVOKED"
	case ConfigurationNotRegistered:
		return "NOT_REGISTERED"
	case ConfigurationNotAccepted:
		return "NOT_ACCEPTED"
	case ConfigurationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterCaseConfigurationDeps 每类配置都要写口与读口两半：读口不是可选的便利，
// 冲突判定就靠它——只有写口时「已在册」永远说不出是重放还是改内容。
type RegisterCaseConfigurationDeps struct {
	Readiness     ports.ReadinessRegistry
	ReadinessView ports.ReadinessView

	Authorities   ports.SubmissionAuthorityRegistry
	AuthorityView ports.SubmissionAuthorityView

	Rules    ports.InterpretationRuleRegistry
	RuleView ports.InterpretationRuleView

	Obligations    ports.ObligationInventoryRegistry
	ObligationView ports.ObligationInventoryView

	Gates    ports.GateConditionRegistry
	GateView ports.GateConditionView
}

type RegisterCaseConfigurationHandler struct {
	deps RegisterCaseConfigurationDeps
}

func NewRegisterCaseConfigurationHandler(deps RegisterCaseConfigurationDeps) *RegisterCaseConfigurationHandler {
	return &RegisterCaseConfigurationHandler{deps: deps}
}

// RegisterReadinessCommand 携带一次就绪判断登记。
type RegisterReadinessCommand struct {
	TenantID domain.TenantID
	Unit     domain.DeclarationUnitID
	Basis    domain.ReadinessBasisReference
	JudgedAt time.Time
}

// RevokeReadinessCommand 携带一次`不再就绪`。原判断保留，这里只推进状态。
type RevokeReadinessCommand struct {
	TenantID domain.TenantID
	Unit     domain.DeclarationUnitID
	Cause    string
	At       time.Time
}

// RegisterReadiness 登记申报就绪判断。已在册时读回既有判断比对：同依据同形成时间是
// 重放，异则冲突——换依据要先撤销再重登，不能靠覆盖抹掉原依据。
func (handler *RegisterCaseConfigurationHandler) RegisterReadiness(
	ctx context.Context,
	command RegisterReadinessCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) {
		return ConfigurationNotAccepted, nil
	}
	judgment, err := domain.JudgeReady(command.Unit, command.Basis, command.JudgedAt)
	if err != nil {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Readiness.RegisterReadiness(ctx, command.TenantID, judgment)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.ReadinessView.LoadReadiness(ctx, command.TenantID, command.Unit)
	if err != nil || !found {
		return ConfigurationUndecided, nil
	}
	if existing.Basis() != judgment.Basis() || !existing.JudgedAt().Equal(judgment.JudgedAt()) {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

// RevokeReadiness 记录`不再就绪`。撤销的合法性由领域判（已撤销的不能再撤、时间不得
// 早于形成时间），本层只负责取回在册那一份交给它。
func (handler *RegisterCaseConfigurationHandler) RevokeReadiness(
	ctx context.Context,
	command RevokeReadinessCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) {
		return ConfigurationNotAccepted, nil
	}
	existing, found, err := handler.deps.ReadinessView.LoadReadiness(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found {
		return ConfigurationNotRegistered, nil
	}
	if !existing.Effective() {
		return ConfigurationAlreadyRevoked, nil
	}

	revoked, err := existing.Revoke(command.Cause, command.At)
	if err != nil {
		return ConfigurationNotAccepted, nil
	}
	if err := handler.deps.Readiness.RevokeReadiness(ctx, command.TenantID, revoked); err != nil {
		return ConfigurationUndecided, nil
	}
	return ConfigurationRevoked, nil
}

// GrantSubmissionAuthorityCommand 携带一次提交授权登记。
type GrantSubmissionAuthorityCommand struct {
	TenantID  domain.TenantID
	Unit      domain.DeclarationUnitID
	Authority domain.SubmissionAuthorityReference
	GrantedAt time.Time
}

// RevokeSubmissionAuthorityCommand 携带一次授权失效。
type RevokeSubmissionAuthorityCommand struct {
	TenantID domain.TenantID
	Unit     domain.DeclarationUnitID
	Cause    string
	At       time.Time
}

// GrantSubmissionAuthority 登记提交授权。与就绪同形而分走两条轨（CONTEXT 244：提交
// 授权与就绪判断分别形成和失效），因此不共用类型也不共用写口。
func (handler *RegisterCaseConfigurationHandler) GrantSubmissionAuthority(
	ctx context.Context,
	command GrantSubmissionAuthorityCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) {
		return ConfigurationNotAccepted, nil
	}
	authorization, err := domain.GrantSubmissionAuthority(command.Unit, command.Authority, command.GrantedAt)
	if err != nil {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Authorities.GrantSubmissionAuthority(ctx, command.TenantID, authorization)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.AuthorityView.LoadSubmissionAuthority(ctx, command.TenantID, command.Unit)
	if err != nil || !found {
		return ConfigurationUndecided, nil
	}
	if existing.Authority() != authorization.Authority() ||
		!existing.GrantedAt().Equal(authorization.GrantedAt()) {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

func (handler *RegisterCaseConfigurationHandler) RevokeSubmissionAuthority(
	ctx context.Context,
	command RevokeSubmissionAuthorityCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) {
		return ConfigurationNotAccepted, nil
	}
	existing, found, err := handler.deps.AuthorityView.LoadSubmissionAuthority(ctx, command.TenantID, command.Unit)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found {
		return ConfigurationNotRegistered, nil
	}
	if !existing.Effective() {
		return ConfigurationAlreadyRevoked, nil
	}

	revoked, err := existing.Revoke(command.Cause, command.At)
	if err != nil {
		return ConfigurationNotAccepted, nil
	}
	if err := handler.deps.Authorities.RevokeSubmissionAuthority(ctx, command.TenantID, revoked); err != nil {
		return ConfigurationUndecided, nil
	}
	return ConfigurationRevoked, nil
}

// RegisterInterpretationRuleCommand 携带一次解释规则版本登记：辖区与法定生效起点
// 在键上（ADR-0070 问一甲），终点不是输入——它在后继版本登记时落定（换版）。
type RegisterInterpretationRuleCommand struct {
	TenantID     domain.TenantID
	Layer        domain.ResultLayer
	Jurisdiction domain.RegulatoryJurisdictionReference
	Rule         domain.InterpretationRuleReference
	AppliesFrom  time.Time
}

// RegisterInterpretationRule 登记该结果层某辖区自某法定起点生效的解释规则版本。
//
// 写口把撞键与撞重叠都折成`已登记`，这里按**请求的生效起点**读回在册版本比对（同义务
// 项按区间起点盘点的理由）：同规则是重放；异规则是冲突——既有 ExternalResult 上
// 「实际采用的规则」不接受被顶替，换版走登记一个更晚起点的新版本，不走覆盖。写口说
// 已在册、按起点却读不回版本，只可能是撞上了起点不同的既有区间（错序或追改历史），
// 区间也是登记内容的一部分，仍是冲突不是重放。
func (handler *RegisterCaseConfigurationHandler) RegisterInterpretationRule(
	ctx context.Context,
	command RegisterInterpretationRuleCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) || command.Layer.String() == "" ||
		strings.TrimSpace(command.Jurisdiction.String()) == "" ||
		command.Rule.String() == "" || command.AppliesFrom.IsZero() {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Rules.RegisterInterpretationRule(
		ctx, command.TenantID, command.Layer, command.Jurisdiction, command.Rule, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.RuleView.LoadInterpretationRule(
		ctx, command.TenantID, command.Layer, command.Jurisdiction, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found || existing != command.Rule {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

// RegisterObligationCatalogCommand 携带一次关闭义务目录登记。
type RegisterObligationCatalogCommand struct {
	TenantID     domain.TenantID
	CaseRef      domain.CustomsCaseID
	RegisteredAt time.Time
}

// RegisterObligationItemCommand 携带一项关闭义务及其适用区间。
type RegisterObligationItemCommand struct {
	TenantID     domain.TenantID
	CaseRef      domain.CustomsCaseID
	Registration ports.ObligationRegistration
}

// RegisterObligationCatalog 登记关闭义务目录。目录行只表达「这本册子已登记」这一件
// 事，没有可比内容，因此重复登记只会是`已存在`，不可能是内容冲突。
func (handler *RegisterCaseConfigurationHandler) RegisterObligationCatalog(
	ctx context.Context,
	command RegisterObligationCatalogCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) || strings.TrimSpace(command.CaseRef.String()) == "" {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Obligations.RegisterObligationCatalog(
		ctx, command.TenantID, command.CaseRef, command.RegisteredAt)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}
	return ConfigurationExisting, nil
}

// RegisterObligationItem 登记一项关闭义务。已在册时按**该项适用区间的起点**盘点读回
// ——按别的截点盘会把一项在册但此刻不适用的义务读成「不在册」，于是冲突判成新登记。
func (handler *RegisterCaseConfigurationHandler) RegisterObligationItem(
	ctx context.Context,
	command RegisterObligationItemCommand,
) (CaseConfigurationOutcome, error) {
	registration := command.Registration
	if blankTenant(command.TenantID) || strings.TrimSpace(command.CaseRef.String()) == "" ||
		registration.AppliesFrom.IsZero() || registration.Item.Obligation == "" {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Obligations.RegisterObligationItem(ctx, command.TenantID, command.CaseRef, registration)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	items, configured, err := handler.deps.ObligationView.LoadObligationItems(
		ctx, command.TenantID, command.CaseRef, registration.AppliesFrom)
	if err != nil || !configured {
		return ConfigurationUndecided, nil
	}
	for _, item := range items {
		if item.Obligation != registration.Item.Obligation {
			continue
		}
		if item != registration.Item {
			return ConfigurationContentConflict, nil
		}
		return ConfigurationExisting, nil
	}
	// 写口说已在册，按起点却盘不出这一项：只可能是在册那一份带着不同的适用区间。
	// 区间也是登记内容的一部分，这仍是冲突，不是重放。
	return ConfigurationContentConflict, nil
}

// RegisterGateCatalogCommand 携带一次门禁前置条件目录登记。
type RegisterGateCatalogCommand struct {
	TenantID     domain.TenantID
	Scope        domain.DecisionScopeReference
	Action       domain.GuardedAction
	Boundary     domain.CustomsProcedureReference
	RegisteredAt time.Time
}

// RegisterGateFindingCommand 携带一项前置条件判断。
type RegisterGateFindingCommand struct {
	TenantID domain.TenantID
	Scope    domain.DecisionScopeReference
	Action   domain.GuardedAction
	Boundary domain.CustomsProcedureReference
	Finding  domain.PreconditionFinding
}

// RegisterGateCatalog 登记门禁前置条件目录。同义务目录：只表达在场，无可比内容。
// 登了目录不登任何前置条件是一个**有意义且必须登得出来**的状态——领域折成`不适用`。
func (handler *RegisterCaseConfigurationHandler) RegisterGateCatalog(
	ctx context.Context,
	command RegisterGateCatalogCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) || command.Action.String() == "" ||
		command.Scope.String() == "" || command.Boundary.String() == "" {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Gates.RegisterGateCatalog(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary, command.RegisteredAt)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}
	return ConfigurationExisting, nil
}

// RegisterGateFinding 登记一项前置条件判断。同键异判断是冲突：门禁判断绑定动作与
// 边界（CONTEXT 硬句 216），改判断要走复核而不是把原判断顶掉。
func (handler *RegisterCaseConfigurationHandler) RegisterGateFinding(
	ctx context.Context,
	command RegisterGateFindingCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) || command.Action.String() == "" ||
		command.Scope.String() == "" || command.Boundary.String() == "" ||
		command.Finding.Precondition.String() == "" {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Gates.RegisterGateFinding(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary, command.Finding)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	findings, configured, err := handler.deps.GateView.LoadPreconditionFindings(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil || !configured {
		return ConfigurationUndecided, nil
	}
	for _, finding := range findings {
		if finding.Precondition != command.Finding.Precondition {
			continue
		}
		if finding.State != command.Finding.State {
			return ConfigurationContentConflict, nil
		}
		return ConfigurationExisting, nil
	}
	return ConfigurationUndecided, nil
}

func blankTenant(tenant domain.TenantID) bool {
	return strings.TrimSpace(tenant.String()) == ""
}
