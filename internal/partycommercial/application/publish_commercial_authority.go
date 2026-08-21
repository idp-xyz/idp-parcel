package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PublishCommercialAuthorityOutcome 是发布用例的结果代数（UC-PC-001 结果语义的机制
// 半边）。`已发布已生效`与`已计划生效`分格，因为后者不得用于生产解析（AppliesAt 只认
// `已生效`）；`重复`按 AT-PC-002 返回原结果不造第二版本；`冲突`要商业责任方修正、绝不
// 覆盖（ADR-0031）；`发布未决`（AT-PC-005/010）等待批准角色或被引对象，不默认发布也
// 不默认拒绝。
type PublishCommercialAuthorityOutcome uint8

const (
	PublishCommercialAuthorityOutcomeInvalid PublishCommercialAuthorityOutcome = iota
	CommercialVersionPublishedEffective
	CommercialVersionPlannedEffective
	CommercialPublicationReplayed
	CommercialPublicationConflicted
	CommercialPublicationPending
)

func (outcome PublishCommercialAuthorityOutcome) String() string {
	switch outcome {
	case CommercialVersionPublishedEffective:
		return "PUBLISHED_EFFECTIVE"
	case CommercialVersionPlannedEffective:
		return "PLANNED_EFFECTIVE"
	case CommercialPublicationReplayed:
		return "REPLAYED"
	case CommercialPublicationConflicted:
		return "CONTENT_CONFLICT"
	case CommercialPublicationPending:
		return "PENDING"
	default:
		return ""
	}
}

// PublishCommercialAuthorityCommand 携带一次受控发布：草稿规格、批准责任、角色确认，
// 以及随本版本一并登记的声明正文。规格直接用领域的 CommercialVersionSpec，不在应用层
// 抄第二份字段清单——版本由哪些维度构成是领域的事（先例：ResolveCommercialBasisCommand
// 携带领域解析键）。
type PublishCommercialAuthorityCommand struct {
	Spec         domain.CommercialVersionSpec
	Approval     domain.ApprovalBasis
	RoleStanding domain.ApprovalRoleStanding
	Declarations CommercialDeclarations
}

// CommercialDeclarations 收拢一次发布随行的声明正文（票 03 的六族）。字段全部可缺：
// 声明属实例半边，缺席就是没有声明，本用例不代拟。归属由领域构造门把守——把收寄资格
// 挂在合同上会在构造时被拒，不会静默丢弃（ADR-0042/0058）。
//
// 声明只能随发布登记：正文随发布固定（内容摘要盖住它），事后补声明等于改一份已固定
// 的正文，那要发新版本。
type CommercialDeclarations struct {
	AsOfPolicies          []domain.AsOfPolicy
	AcceptanceContent     *AcceptanceContentDeclaration
	PendingRoutingBasis   *domain.PendingRoutingBasisReference
	PreAcceptanceControl  *PreAcceptanceControlInstruction
	ContractContent       *ContractContentDeclaration
	IntakeQualification   *IntakeQualificationDeclaration
	FinalRules            []domain.FinalizationDeclaration
	CancellationAuthority []domain.CancellationAuthorityDeclaration
	RulePackageBody       *RulePackageBodyDeclaration
}

func (declarations CommercialDeclarations) empty() bool {
	return len(declarations.AsOfPolicies) == 0 &&
		declarations.AcceptanceContent == nil &&
		declarations.PendingRoutingBasis == nil &&
		declarations.PreAcceptanceControl == nil &&
		declarations.ContractContent == nil &&
		declarations.IntakeQualification == nil &&
		len(declarations.FinalRules) == 0 &&
		len(declarations.CancellationAuthority) == 0 &&
		declarations.RulePackageBody == nil
}

// AcceptanceContentDeclaration 是接单规则包的接受内容声明输入（ADR-0042）。
type AcceptanceContentDeclaration struct {
	ApplicableGroups []domain.AcceptanceCheckGroupType
	ManualReview     domain.ManualReviewDirective
}

// PreAcceptanceControlInstruction 是客户合同版本的接受前控制声明输入（PAR-COM-15）。
// Basis 只在`不适用`时给出；`要求控制`必须留零值，两头都带的声明由领域构造门拒绝。
type PreAcceptanceControlInstruction struct {
	Requirement domain.PreAcceptanceControlRequirement
	Basis       domain.ControlNotApplicableBasis
}

// ContractContentDeclaration 是客户合同版本的正文输入：规则包引用与按费用范围的
// 财务控制约定。Bindings 可为空——「合同已登记但没对任何范围作约定」是合法的显式空。
type ContractContentDeclaration struct {
	RulePackage domain.CommercialObjectID
	Bindings    []domain.FinancialControlBinding
}

// IntakeQualificationDeclaration 是接单规则包的收寄资格声明输入（PAR-COM-16）。
// Qualifications 可为空清单（真没有硬资格也要显式声明），Sources 至少一格由领域把守。
type IntakeQualificationDeclaration struct {
	Sources        []domain.DeclaredIntakeSource
	Qualifications []domain.RuleReference
}

// RulePackageBodyDeclaration 是接单规则包版本的正文输入（open-decisions D-3）：五维
// 适用性与按分类归档的规则引用。
type RulePackageBodyDeclaration struct {
	Applicability domain.RulePackageApplicability
	Rules         []domain.AssembledRule
}

// DeclarationChannel 点名一次发布里的一个声明通道，供报告与进程口展示落点。
type DeclarationChannel uint8

const (
	DeclarationChannelInvalid DeclarationChannel = iota
	AsOfPolicyChannel
	AcceptanceContentChannel
	PendingRoutingChannel
	PreAcceptanceControlChannel
	ContractContentChannel
	IntakeQualificationChannel
	FinalRuleChannel
	CancellationAuthorityChannel
	RulePackageBodyChannel
)

func (channel DeclarationChannel) String() string {
	switch channel {
	case AsOfPolicyChannel:
		return "AS_OF_POLICY"
	case AcceptanceContentChannel:
		return "ACCEPTANCE_CONTENT"
	case PendingRoutingChannel:
		return "PENDING_ROUTING"
	case PreAcceptanceControlChannel:
		return "PRE_ACCEPTANCE_CONTROL"
	case ContractContentChannel:
		return "CONTRACT_CONTENT"
	case IntakeQualificationChannel:
		return "INTAKE_QUALIFICATION"
	case FinalRuleChannel:
		return "FINAL_RULE"
	case CancellationAuthorityChannel:
		return "CANCELLATION_AUTHORITY"
	case RulePackageBodyChannel:
		return "RULE_PACKAGE_BODY"
	default:
		return ""
	}
}

// DeclarationReport 是一个声明通道的写入落点。`内容冲突`留在报告里而不是 error：
// 事务保持可用（ADR-0031），由商业责任方对着报告修正。
type DeclarationReport struct {
	Channel DeclarationChannel
	Outcome ports.DeclarationSaveOutcome
}

type PublishCommercialAuthorityResult struct {
	outcome      PublishCommercialAuthorityOutcome
	version      domain.CommercialVersion
	hasVersion   bool
	pendingCause error
	declarations []DeclarationReport
}

func (result PublishCommercialAuthorityResult) Outcome() PublishCommercialAuthorityOutcome {
	return result.outcome
}

// Version 交回本次处理后的版本终态（已生效或已计划生效）。未决与冲突没有版本可交：
// 未决的草稿与来源由调用方保留（ADR-0035），冲突的权威版本在册上，不在本次结果里。
func (result PublishCommercialAuthorityResult) Version() (domain.CommercialVersion, bool) {
	return result.version, result.hasVersion
}

// PendingCause 只在`发布未决`时非空：角色未确认与被引对象未发布的恢复动作不同
// （等角色确认 / 等被引对象发布），原因必须随结果交回，不靠格分辨。
func (result PublishCommercialAuthorityResult) PendingCause() error {
	return result.pendingCause
}

// Declarations 交回各声明通道的写入落点（副本），顺序与通道枚举一致。
func (result PublishCommercialAuthorityResult) Declarations() []DeclarationReport {
	return append([]DeclarationReport(nil), result.declarations...)
}

type PublishCommercialAuthorityHandler struct {
	registry ports.PublicationRegistry
	clock    ports.Clock
}

func NewPublishCommercialAuthorityHandler(
	registry ports.PublicationRegistry,
	clock ports.Clock,
) *PublishCommercialAuthorityHandler {
	return &PublishCommercialAuthorityHandler{registry: registry, clock: clock}
}

// Handle 执行一次单对象发布（UC-PC-001 步骤 5–7 的机制半边）。批次不是聚合
// （AT-PC-011）：调用方逐项调用本方法、逐项各起事务，先落库的对象自然被后项装载的
// 整册看见，项与项之间没有共同命运。
func (handler *PublishCommercialAuthorityHandler) Handle(
	ctx context.Context,
	command PublishCommercialAuthorityCommand,
) (PublishCommercialAuthorityResult, error) {
	draft, err := domain.NewCommercialDraft(command.Spec)
	if err != nil {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
	}

	// 读回该范围整册再折叠：Register 拿它挡同键异内容的覆盖，指名引用的发布存续也由
	// 它回答。读不回就不写——发布是写权威的动作，看不见既有权威时继续写等于闭眼登记；
	// 这与解析用例把读失败折成空视图相反，那边空视图表达`权威不可读`并停在未决，这边
	// 照原样上抛等重试。
	registry, err := handler.registry.LoadForScope(ctx, command.Spec.TenantID, command.Spec.Scope)
	if err != nil {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
	}

	now := handler.clock.Now()
	results := registry.PublishBatch([]domain.PublicationBatchItem{{
		Draft:        draft,
		Basis:        command.Approval,
		RoleStanding: command.RoleStanding,
		PublishedAt:  now,
	}}, nil)
	item := results[0]
	if publishErr := item.Err(); publishErr != nil {
		switch {
		case errors.Is(publishErr, domain.ErrApprovalRoleNotConfirmed),
			errors.Is(publishErr, domain.ErrNamedReferenceNotPublished),
			errors.Is(publishErr, domain.ErrIncompleteCommercialPublication):
			// AT-PC-010 / AT-PC-005：三格都是`发布未决`。这里不写任何东西，草稿与
			// 导入来源由调用方原样保留（ADR-0035/0036）。
			return PublishCommercialAuthorityResult{
				outcome:      CommercialPublicationPending,
				pendingCause: publishErr,
			}, nil
		case errors.Is(publishErr, domain.ErrCommercialVersionConflict):
			return PublishCommercialAuthorityResult{outcome: CommercialPublicationConflicted}, nil
		default:
			return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", publishErr)
		}
	}
	version, published := item.Published()
	if !published {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: 折叠成功却没有已发布版本")
	}

	// 到界即取效。只增仓储没有事后翻状态的口：停在`已发布`的行永远进不了解析，生效
	// 边界已开却不取效等于登记一个永远选不中的版本。边界未开的保持`已计划生效`
	// （UC-PC-001 结果语义），不得提前用于生产解析——那一格由消费侧 AppliesAt 结构性
	// 保证，这里如实入册。
	if !now.Before(version.Effective().StartsAt()) {
		version, err = version.TakeEffect(now)
		if err != nil {
			return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: take effect: %w", err)
		}
	}

	// 声明先于任何写入构造：领域构造门（归属类别、缺件、冲突）与壳-正文一致性在这里
	// 全部裁完，裁不过整项一行不写。构造要求拥有版本已生效，因此挂在`已计划生效`
	// 版本上的声明整项拒绝——只增仓储没有「日后取效时补声明」的口，收下它等于登记
	// 一份永远读不出的正文；届期改为到界发布，声明随那次发布一并登记。
	writes, err := declarationWrites(version, command.Declarations)
	if err != nil {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
	}

	saveOutcome, err := handler.registry.SaveVersion(ctx, version)
	if err != nil {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
	}
	result := PublishCommercialAuthorityResult{version: version, hasVersion: true}
	switch saveOutcome {
	case ports.PublicationSaved:
		result.outcome = CommercialVersionPublishedEffective
		if version.Status() != domain.CommercialVersionEffective {
			result.outcome = CommercialVersionPlannedEffective
		}
	case ports.PublicationAlreadyRegistered:
		// 折叠层看到的整册与持久化面各自判重放，以持久化面为准：装载与写入之间别人
		// 先落了同一份时，折叠答`新登记`而库答`已登记`，本次仍是重复（AT-PC-002）。
		// 重复的发布照样跑声明通道：首次发布若在声明写入前中断，重放正是补齐的路。
		result.outcome = CommercialPublicationReplayed
	case ports.PublicationContentConflict:
		return PublishCommercialAuthorityResult{outcome: CommercialPublicationConflicted}, nil
	default:
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: unexpected save outcome %q", saveOutcome)
	}

	for _, write := range writes {
		outcome, err := write.save(ctx, handler.registry)
		if err != nil {
			// 技术失败上抛，让调用方的事务整项回滚：版本与声明同一事务落库，不留
			// 半份发布（UC-PC-001 步骤 6「原子保存单一对象版本与发布意图」）。
			return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
		}
		switch outcome {
		case ports.DeclarationSaved, ports.DeclarationAlreadyRegistered, ports.DeclarationContentConflict:
			result.declarations = append(result.declarations, DeclarationReport{
				Channel: write.channel,
				Outcome: outcome,
			})
		default:
			return PublishCommercialAuthorityResult{}, fmt.Errorf(
				"publish commercial authority: unexpected declaration outcome %q on %s", outcome, write.channel)
		}
	}
	return result, nil
}

type declarationWrite struct {
	channel DeclarationChannel
	save    func(context.Context, ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error)
}

// declarationWrites 把命令里的声明输入逐通道折成（构造好的领域对象 + 写入调用）。
// 全部构造先于全部写入：任何一条裁不过，整项在触碰持久化面之前就停。
func declarationWrites(
	version domain.CommercialVersion,
	declarations CommercialDeclarations,
) ([]declarationWrite, error) {
	var writes []declarationWrite

	if len(declarations.AsOfPolicies) > 0 {
		declared, err := domain.DeclareAsOfPolicies(version, declarations.AsOfPolicies)
		if err != nil {
			return nil, fmt.Errorf("as-of policies: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: AsOfPolicyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveAsOfPolicies(ctx, declared)
			},
		})
	}

	if declarations.AcceptanceContent != nil {
		content, err := domain.DeclareAcceptanceRuleContent(
			version, declarations.AcceptanceContent.ApplicableGroups, declarations.AcceptanceContent.ManualReview)
		if err != nil {
			return nil, fmt.Errorf("acceptance rule content: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: AcceptanceContentChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveAcceptanceRuleContent(ctx, content)
			},
		})
	}

	if declarations.PendingRoutingBasis != nil {
		permission, err := domain.DeclarePendingRoutingPermission(version, *declarations.PendingRoutingBasis)
		if err != nil {
			return nil, fmt.Errorf("pending routing permission: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: PendingRoutingChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SavePendingRoutingPermission(ctx, permission)
			},
		})
	}

	if declarations.PreAcceptanceControl != nil {
		declared, err := domain.DeclarePreAcceptanceControl(
			version, declarations.PreAcceptanceControl.Requirement, declarations.PreAcceptanceControl.Basis)
		if err != nil {
			return nil, fmt.Errorf("pre-acceptance control: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: PreAcceptanceControlChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SavePreAcceptanceControl(ctx, declared)
			},
		})
	}

	if declarations.ContractContent != nil {
		// open-decisions F-3：版本壳指名的规则包与正文件引用都在场时必须相等，
		// 不等整项拒绝，不静默选一处——读侧同一道核对在装载时还会再走一遍。
		if err := domain.ConsistentAcceptanceRulePackage(version, declarations.ContractContent.RulePackage); err != nil {
			return nil, err
		}
		content, err := domain.NewCustomerContract(
			version, declarations.ContractContent.RulePackage, declarations.ContractContent.Bindings)
		if err != nil {
			return nil, fmt.Errorf("customer contract content: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: ContractContentChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveCustomerContractContent(ctx, content)
			},
		})
	}

	if declarations.IntakeQualification != nil {
		content, err := domain.NewIntakeQualificationContent(
			version, declarations.IntakeQualification.Sources, declarations.IntakeQualification.Qualifications)
		if err != nil {
			return nil, fmt.Errorf("intake qualification: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: IntakeQualificationChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveIntakeQualification(ctx, content)
			},
		})
	}

	if len(declarations.FinalRules) > 0 {
		content, err := domain.NewFinalRuleContent(version, declarations.FinalRules)
		if err != nil {
			return nil, fmt.Errorf("final rule content: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: FinalRuleChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveFinalRule(ctx, content)
			},
		})
	}

	if len(declarations.CancellationAuthority) > 0 {
		content, err := domain.NewCancellationAuthorityContent(version, declarations.CancellationAuthority)
		if err != nil {
			return nil, fmt.Errorf("cancellation authority: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: CancellationAuthorityChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveCancellationAuthority(ctx, content)
			},
		})
	}

	if declarations.RulePackageBody != nil {
		pack, err := domain.NewAcceptanceRulePackage(
			version, declarations.RulePackageBody.Applicability, declarations.RulePackageBody.Rules)
		if err != nil {
			return nil, fmt.Errorf("acceptance rule package body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: RulePackageBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveAcceptanceRulePackage(ctx, pack)
			},
		})
	}

	return writes, nil
}
