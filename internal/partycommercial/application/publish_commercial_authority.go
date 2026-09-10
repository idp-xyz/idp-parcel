package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PublishCommercialAuthorityOutcome 是发布用例的结果代数（UC-PC-001 结果语义的机制
// 半边）。`已发布已生效`与`已计划生效`分格，因为后者不得用于生产解析（AppliesAt 只认
// `已生效`）；`重复`按 AT-PC-002 返回原结果不造第二版本；`冲突`要商业责任方修正、绝不
// 覆盖（ADR-0031）；`发布未决`（AT-PC-005/010）等待批准角色或被引对象，不默认发布也
// 不默认拒绝；`未受理`（ADR-0126 Decision 二）是这一份输入内部不自洽——声明的内容摘要与
// 服务端按正文算出的不相等——恢复动作是改批文，与`冲突`（换版本号）是两格，一个字节不写。
type PublishCommercialAuthorityOutcome uint8

const (
	PublishCommercialAuthorityOutcomeInvalid PublishCommercialAuthorityOutcome = iota
	CommercialVersionPublishedEffective
	CommercialVersionPlannedEffective
	CommercialPublicationReplayed
	CommercialPublicationConflicted
	CommercialPublicationPending
	CommercialPublicationNotAccepted
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
	case CommercialPublicationNotAccepted:
		return "NOT_ACCEPTED"
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

// CommercialDeclarations 收拢一次发布随行的声明正文。字段全部可缺：
// 声明属实例半边，缺席就是没有声明，本用例不代拟。归属由领域构造门把守——把收寄资格
// 挂在合同上会在构造时被拒，不会静默丢弃（ADR-0042/0058）。
//
// 声明只能随发布登记：正文随发布固定（内容摘要盖住它），事后补声明等于改一份已固定
// 的正文，那要发新版本。
type CommercialDeclarations struct {
	AsOfPolicies         []domain.AsOfPolicy
	AcceptanceContent    *AcceptanceContentDeclaration
	PendingRoutingBasis  *domain.PendingRoutingBasisReference
	PreAcceptanceControl *PreAcceptanceControlInstruction
	ContractContent      *ContractContentDeclaration
	IntakeQualification  *IntakeQualificationDeclaration
	FinalRules           []domain.FinalizationDeclaration
	// FinalRuleValidity 是终局规则声明父行上那一格面单有效期（ADR-0119）。它不是另一条通道：与 FinalRules
	// 折进同一份 FinalRuleContent、走同一 FinalRuleChannel——有效期没有独立的拥有对象，单独给出（FinalRules
	// 为空）整项拒。nil 就是「没有这一格」，不失效。
	FinalRuleValidity       *domain.LabelValidityDeclaration
	CancellationAuthority   []domain.CancellationAuthorityDeclaration
	RulePackageBody         *RulePackageBodyDeclaration
	SettlementPolicyBody    *SettlementPolicyBodyDeclaration
	CreditPolicyBody        *CreditPolicyBodyDeclaration
	SupplierAgreementBody   *SupplierAgreementBodyDeclaration
	PricePolicyBody         *PricePolicyBodyDeclaration
	CustomerServiceRuleBody *CustomerServiceRuleBodyDeclaration
	// PreAcceptanceFinancialControlPolicyBody 与 PreAcceptanceControl 是两层不同的声明：后者挂在客户
	// 合同版本上答「要不要」（0007），前者挂在策略版本上答「控制怎么做」（0024，ADR-0115）。
	PreAcceptanceFinancialControlPolicyBody *PreAcceptanceFinancialControlPolicyBodyDeclaration
	// ContractDelegations 挂在客户合同版本上（ADR-0116 Decision 二）：委派方把某一授权动作在某一范围内
	// 的实际决定权交给持某一等级的运营角色。它与 ContractContent 是同一份合同的两种话——那一节答
	// 「采用哪个规则包、哪些范围怎么控」，本节答「谁能替谁决定」；同键两条与空清单由领域构造门拒。
	ContractDelegations []domain.ContractDelegationDeclaration
	// SourceDataAmendment 挂在接单规则包版本上（ADR-0120）：接受后客户原始资料按（资料组 × 阶段 × 意图）能不能改，
	// 外加一格「封闭」说缺格怎么读。是指针不是切片：「封闭 + 零格」是一份合法声明，len 表达不了在不在场。
	// 未封闭却零格、某格立不住、同格两行由领域构造门拒。
	SourceDataAmendment *SourceDataAmendmentDeclaration
	// DeliveryConditions 挂在服务产品版本（产品层）或客户合同版本（合同层）上（ADR-0133 决定四）：允许的交付方式集合、
	// 收件范围规则引用、交付证明规则引用。层由发布的版本类别定，不另给开关；合同层必须指名所收紧的产品版本、产品层
	// 必须不带，两条与零方式、同方式两行一样由领域构造门拒。是指针不是切片：整节缺席就是这一版没有交付条件声明。
	DeliveryConditions *DeliveryConditionDeclaration
}

func (declarations CommercialDeclarations) empty() bool {
	return len(declarations.AsOfPolicies) == 0 &&
		declarations.AcceptanceContent == nil &&
		declarations.PendingRoutingBasis == nil &&
		declarations.PreAcceptanceControl == nil &&
		declarations.ContractContent == nil &&
		declarations.IntakeQualification == nil &&
		len(declarations.FinalRules) == 0 &&
		declarations.FinalRuleValidity == nil &&
		len(declarations.CancellationAuthority) == 0 &&
		declarations.RulePackageBody == nil &&
		declarations.SettlementPolicyBody == nil &&
		declarations.CreditPolicyBody == nil &&
		declarations.SupplierAgreementBody == nil &&
		declarations.PricePolicyBody == nil &&
		declarations.CustomerServiceRuleBody == nil &&
		declarations.PreAcceptanceFinancialControlPolicyBody == nil &&
		len(declarations.ContractDelegations) == 0 &&
		declarations.SourceDataAmendment == nil &&
		declarations.DeliveryConditions == nil
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

// SourceDataAmendmentDeclaration 是接单规则包的资料修订允许声明输入（PAR-COM-13，ADR-0120）：Closed 说缺格
// 怎么读（false 未声明 / true 不允许），Rules 是逐格的允许 / 不允许。Closed 没有默认——它是这一节正文的一部分，
// 由调用方显式给出；Rules 在 Closed=true 时可为空清单（这一版什么都不许改），Closed=false 时至少一格由领域把守。
type SourceDataAmendmentDeclaration struct {
	Closed bool
	Rules  []domain.SourceDataAmendmentRule
}

// DeliveryConditionDeclaration 是交付条件声明输入（票 party-commercial-context-gaps/11，ADR-0133 决定四）。Terms 是
// 三格正文；Tightens 只在合同层：所收紧的服务产品版本（对象标识 + 版本号），nil 就是产品层。方式与规则引用都是开放
// 引用，用例不解读也不给默认——不内置「本人签收」，不内置任何一条规则。合同层的方式是否真在那一版产品层之内，由
// 持久化写口读回产品层后核（ports.PublicationRegistry.SaveDeliveryConditions 的注释），这里没有产品层可对。
type DeliveryConditionDeclaration struct {
	Tightens *domain.TightenedProductVersion
	Terms    domain.DeliveryConditionTerms
}

// RulePackageBodyDeclaration 是接单规则包版本的正文输入（open-decisions D-3）：五维
// 适用性与按分类归档的规则引用。
type RulePackageBodyDeclaration struct {
	Applicability domain.RulePackageApplicability
	Rules         []domain.AssembledRule
}

// SettlementPolicyBodyDeclaration 是结算政策版本的正文输入（ADR-0044）：一种结算方式
// 与它覆盖的六维适用范围。
//
// 六维整体由 domain.NewSettlementApplicability 构造，本类型不逐维摊平：摊平之后应用层
// 就得自己判「六维齐不齐」，而那条判据只能有一处——少一维的适用范围写得进库，读回来却
// 命不中任何查询，看起来像「这个范围没有结算政策」。
type SettlementPolicyBodyDeclaration struct {
	Method        domain.SettlementMethod
	Applicability domain.SettlementApplicability
}

// CreditPolicyBodyDeclaration 是信用政策版本的正文输入（票 party-commercial-context-gaps/03）：
// 责任法人、权限等级、费用类型、额度与区间。
//
// Limit 直接收领域的 CreditLimit 而不是摊成「金额、比例、哪一格」三个字段：金额或比例恰一在场
// 那条判据只能有一处，摊平之后应用层就得自己判「两格齐不齐」，而判错的形状恰恰是那种没有任何
// 东西会报的——两格都填时挑一格读、都空时读成零额度。
type CreditPolicyBodyDeclaration struct {
	LegalEntity domain.LegalEntityReference
	Level       domain.AuthorityLevel
	ChargeType  domain.ChargeTypeReference
	Limit       domain.CreditLimit
	Effective   domain.EffectiveInterval
}

// SupplierAgreementBodyDeclaration 是供应商商业协议版本的正文输入（同票）：供应商、责任法人、
// 协议自己的适用范围、采购定价方案与区间。方向不在输入上——领域把它钉死为 BUY。
type SupplierAgreementBodyDeclaration struct {
	Supplier     domain.PartyID
	LegalEntity  domain.LegalEntityReference
	Scope        domain.CommercialScopeReference
	PurchasePlan domain.PricingPlanReference
	Effective    domain.EffectiveInterval
}

// PricePolicyBodyDeclaration 是价格规则版本的正文输入（票 party-commercial-context-gaps/06）：
// 方向、定价方案绑定、政策自己的适用范围与区间，以及可缺的计价口径。
//
// PlanDirection 与 Conversion 按 ADR-0057 是**发布当时** parcel-pricing 的答复与当时声明的转换，
// 由调用方交出、本用例不推断——受控 CLI 的批文照价卡目录抄，在线口将来要向 parcel-pricing 现问
// （票 06「planDirection 从哪来」）。
//
// Caliber 嵌在正文里而不是另一条平行通道：口径只能随正文同一次发布登记（0022 的外键把它钉在
// 正文行上并连带方向一致），嵌套让「只给口径不给正文」在结构上就写不出来。
type PricePolicyBodyDeclaration struct {
	Direction     domain.PriceDirection
	PricingPlan   domain.PricingPlanReference
	PlanDirection domain.PriceDirection
	Conversion    domain.PlanBindingConversion
	Scope         domain.CommercialScopeReference
	Effective     domain.EffectiveInterval
	Caliber       *PricePolicyCaliberDeclaration
}

// PricePolicyCaliberDeclaration 是价格政策声明的计价口径输入。Tax 与 Volumetric 必需（零值由
// 领域构造门拒），Fx 可缺——不涉及外币的政策没有汇率口径，nil 就是「没声明」而不是零口径。
type PricePolicyCaliberDeclaration struct {
	Tax        domain.TaxCaliber
	Volumetric domain.VolumetricCaliber
	Fx         *domain.FxCaliber
}

// CustomerServiceRuleBodyDeclaration 是客户服务规则版本的正文输入（票 party-commercial-context-gaps/05，
// ADR-0104）：挂在哪个商业对象上、责任方、范围，与首发两项——索赔期限、最低材料。
//
// Applicability 直接收领域的两格封闭而不是摊成「产品、合同、哪一格」：产品或合同恰一在场那条判据
// 只能有一处（判据同 CreditPolicyBodyDeclaration 收 CreditLimit）。两项清单可各自为空，但合起来至少
// 一行由 NewCustomerServiceRuleVersion 把守——「对首发两项都无客户差异」不是一版规则，是不登记。
type CustomerServiceRuleBodyDeclaration struct {
	Applicability domain.CustomerServiceRuleApplicability
	Responsible   domain.PartyID
	Scope         domain.CommercialScopeReference
	Deadlines     []domain.ClaimDeadlineRule
	Materials     []domain.MinimumMaterialsRule
}

// PreAcceptanceFinancialControlPolicyBodyDeclaration 是接受前财务控制策略版本的正文输入（票
// party-commercial-context-gaps/07，ADR-0115）：共同通过条件与要执行的控制项。
//
// Items 直接收领域的控制项而不是摊成「种类、范围、顺序、处置、责任」五列：每项的形状只校一次，在
// NewPreAcceptanceControlItem；至少一项、顺序唯一、（种类 × 范围）唯一由 NewPreAcceptanceFinancialControlPolicy
// 把守——零项不是「显式无控制」，那一句由客户合同声明，本通道说不了它。
type PreAcceptanceFinancialControlPolicyBodyDeclaration struct {
	JointPass domain.JointPassCondition
	Items     []domain.PreAcceptanceControlItem
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
	SettlementPolicyBodyChannel
	CreditPolicyBodyChannel
	SupplierAgreementBodyChannel
	PricePolicyBodyChannel
	PricePolicyCaliberChannel
	CustomerServiceRuleBodyChannel
	PreAcceptanceFinancialControlPolicyBodyChannel
	ContractDelegationChannel
	SourceDataAmendmentChannel
	DeliveryConditionChannel
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
	case SettlementPolicyBodyChannel:
		return "SETTLEMENT_POLICY_BODY"
	case CreditPolicyBodyChannel:
		return "CREDIT_POLICY_BODY"
	case SupplierAgreementBodyChannel:
		return "SUPPLIER_AGREEMENT_BODY"
	case PricePolicyBodyChannel:
		return "PRICE_POLICY_BODY"
	case PricePolicyCaliberChannel:
		return "PRICE_POLICY_CALIBER"
	case CustomerServiceRuleBodyChannel:
		return "CUSTOMER_SERVICE_RULE_BODY"
	case PreAcceptanceFinancialControlPolicyBodyChannel:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY"
	case ContractDelegationChannel:
		return "CONTRACT_DELEGATION"
	case SourceDataAmendmentChannel:
		return "SOURCE_DATA_AMENDMENT"
	case DeliveryConditionChannel:
		return "DELIVERY_CONDITION"
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
	// refusalCause 与两个摘要串只在`未受理`时在场（ADR-0126 Decision 二）：调用方要的不是「不等」
	// 这一个字，是两个串——抄算出的那一个就是恢复动作。
	refusalCause   error
	declaredDigest string
	computedDigest string
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

// RefusalCause 只在`未受理`时非空：声明的摘要与算出的不等、声明的串带本构建不认识的规范化版本、
// 或正文折不成规范化文档，三种成因恢复动作不同，随结果交回。
func (result PublishCommercialAuthorityResult) RefusalCause() error {
	return result.refusalCause
}

// DigestReconciliation 交回对账门比过的两个串：声明的与算出的。只在`未受理`且确实比过时 ok——
// 正文折不成文档时没有算出的串，ok 为假，成因在 RefusalCause。
func (result PublishCommercialAuthorityResult) DigestReconciliation() (declared, computed string, ok bool) {
	return result.declaredDigest, result.computedDigest, result.computedDigest != ""
}

type PublishCommercialAuthorityHandler struct {
	registry ports.PublicationRegistry
	clock    ports.Clock
	handoff  ports.OperatorRegistrationCompletedHandoff
}

// NewPublishCommercialAuthorityHandler 收交接口为必需依赖而不是可选项：ADR-0094 决定四要求
// `等待运营登记`那一格与它的续办触发同笔落地，一个没接交接口的发布编排会让时点策略静静落库、
// 停等的委托永远没有信来推——装配漏接必须在编译期就红，不能等到真租户上才发现。
func NewPublishCommercialAuthorityHandler(
	registry ports.PublicationRegistry,
	clock ports.Clock,
	handoff ports.OperatorRegistrationCompletedHandoff,
) *PublishCommercialAuthorityHandler {
	return &PublishCommercialAuthorityHandler{registry: registry, clock: clock, handoff: handoff}
}

// Handle 执行一次单对象发布（UC-PC-001 步骤 5–7 的机制半边）。批次不是聚合
// （AT-PC-011）：调用方逐项调用本方法、逐项各起事务，先落库的对象自然被后项装载的
// 整册看见，项与项之间没有共同命运。
func (handler *PublishCommercialAuthorityHandler) Handle(
	ctx context.Context,
	command PublishCommercialAuthorityCommand,
) (PublishCommercialAuthorityResult, error) {
	// 对账门先于一切（ADR-0126 Decision 二）：已接进服务端规范化的册，声明的摘要必须与按正文算出的
	// 逐字节相等，不等即`未受理`、一个字节不写。放在草稿构造之前，是因为这一格说的是输入自己不自洽，
	// 与册上有什么无关——读整册是为了判重放与冲突，输入都立不住时那一次读没有意义。
	if refused, refusal := reconcileDeclaredDigest(command); refused {
		return refusal, nil
	}

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

	if err := handler.announceOperatorRegistrations(ctx, version, now, result.declarations); err != nil {
		return PublishCommercialAuthorityResult{}, fmt.Errorf("publish commercial authority: %w", err)
	}
	return result, nil
}

// announceOperatorRegistrations 对本次发布里属于运营登记参数的声明通道，各交一份「参数已登记」
// 意图（ADR-0094 决定四；票 first-tenant-runway/07 D4）。这一族今天只有时点策略通道——
// 授权规则那一半没有生产编排，见 ports.AuthorityGrantRegistered 的注释。
//
// 判据是落点不是通道在不在场：`已保存`与`已登记`都交（重放重发同一份，ADR-0043，认领由适配器按
// 同一个 ID 去重）；`内容冲突`不交——冲突那一行没写进册，下游重驱只会再次撞见未配置。交接失败
// 上抛而不是记进报告：信封与登记同一事务，入不了队就整项回滚，不留「登记了却没人知道」的半份。
func (handler *PublishCommercialAuthorityHandler) announceOperatorRegistrations(
	ctx context.Context,
	version domain.CommercialVersion,
	registeredAt time.Time,
	reports []DeclarationReport,
) error {
	for _, report := range reports {
		if report.Channel != AsOfPolicyChannel {
			continue
		}
		if report.Outcome != ports.DeclarationSaved && report.Outcome != ports.DeclarationAlreadyRegistered {
			continue
		}
		if err := handler.handoff.HandOffOperatorRegistrationCompleted(ctx, ports.OperatorRegistrationCompletedIntent{
			Kind:         ports.AsOfPolicyRegistered,
			Registration: version,
			RegisteredAt: registeredAt,
		}); err != nil {
			return fmt.Errorf("announce %s registration: %w", ports.AsOfPolicyRegistered, err)
		}
	}
	return nil
}

// reconcileDeclaredDigest 是受控批文那一半的对账门（ADR-0126 Decision 二）。只对已接进规范化的册
// 开门；已接的册正文缺席时没有可比对象，按今天的样子放行声明的串（那种版本没有正文可读，摘要盖住的
// 是空；预览与表单路径永远带正文）。正文折不成文档（零值、册与类别不符）与两串不等同归`未受理`，成因
// 各自随结果交回——它们都是输入自己的问题，与册上的任何一版无关。
func reconcileDeclaredDigest(command PublishCommercialAuthorityCommand) (bool, PublishCommercialAuthorityResult) {
	if !domain.IsRegisterCanonicalized(command.Spec.Kind) {
		return false, PublishCommercialAuthorityResult{}
	}
	content, present := publicationContentOf(command.Spec.Kind, command.Declarations)
	if !present {
		return false, PublishCommercialAuthorityResult{}
	}
	canonical, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		return true, PublishCommercialAuthorityResult{
			outcome:        CommercialPublicationNotAccepted,
			refusalCause:   fmt.Errorf("canonicalize declared content: %w", err),
			declaredDigest: command.Spec.ContentDigest.String(),
		}
	}
	if err := domain.ReconcileDeclaredDigest(command.Spec.ContentDigest, canonical); err != nil {
		return true, PublishCommercialAuthorityResult{
			outcome:        CommercialPublicationNotAccepted,
			refusalCause:   err,
			declaredDigest: command.Spec.ContentDigest.String(),
			computedDigest: canonical.Digest().String(),
		}
	}
	return false, PublishCommercialAuthorityResult{}
}

// publicationContentOf 把命令里属于该册的声明正文折成领域的正文输入面。首例是信用政策一格，其余各册
// 由各自的子票在此加一分支；declarationsOfContent 是它的反向，两处同笔改。第二个返回值答「正文在不在场」
// ——不在场是合法的（壳可以单独发布），交给调用方决定要不要对账。
func publicationContentOf(
	kind domain.CommercialObjectKind,
	declarations CommercialDeclarations,
) (domain.PublicationContent, bool) {
	content := domain.PublicationContent{Kind: kind}
	switch kind {
	case domain.CreditPolicyObject:
		if declarations.CreditPolicyBody == nil {
			return content, false
		}
		body := declarations.CreditPolicyBody
		content.CreditPolicy = &domain.CreditPolicyBody{
			LegalEntity: body.LegalEntity,
			Level:       body.Level,
			ChargeType:  body.ChargeType,
			Limit:       body.Limit,
			Effective:   body.Effective,
		}
		return content, true
	case domain.SupplierAgreementObject:
		if declarations.SupplierAgreementBody == nil {
			return content, false
		}
		body := declarations.SupplierAgreementBody
		content.SupplierAgreement = &domain.SupplierAgreementBody{
			Supplier:     body.Supplier,
			LegalEntity:  body.LegalEntity,
			Scope:        body.Scope,
			PurchasePlan: body.PurchasePlan,
			Effective:    body.Effective,
		}
		return content, true
	case domain.CustomerContractObject:
		// 正文在不在场看 0012 那一层（contractContent）：只带合同级声明不带正文的项没有可比对象，照今天登记声明的
		// 串——批文里两键各自可缺是既有语义（ADR-0126 边界：不改受控批文既有字段语义）。
		if declarations.ContractContent == nil {
			return content, false
		}
		content.CustomerContract = &domain.CustomerContractBody{
			RulePackage: declarations.ContractContent.RulePackage,
			Bindings:    declarations.ContractContent.Bindings,
		}
		if declarations.PreAcceptanceControl != nil {
			content.CustomerContract.Control = &domain.PreAcceptanceControlBody{
				Requirement: declarations.PreAcceptanceControl.Requirement,
				Basis:       declarations.PreAcceptanceControl.Basis,
			}
		}
		return content, true
	case domain.AuthorizationRuleObject:
		// 本册的正文就是取消授权目录（票 admin-write-faces/17）：零行是「壳单独发布」——没有可比对象，照今天登记声明的串。
		if len(declarations.CancellationAuthority) == 0 {
			return content, false
		}
		content.AuthorizationRule = &domain.AuthorizationRuleBody{
			CancellationAuthority: append([]domain.CancellationAuthorityDeclaration(nil), declarations.CancellationAuthority...),
		}
		return content, true
	case domain.SettlementPolicyObject:
		if declarations.SettlementPolicyBody == nil {
			return content, false
		}
		// 六维整体过去，不逐维摊平——「六维齐不齐」只在 NewSettlementApplicability 一处判（声明类型处的注释）。
		content.SettlementPolicy = &domain.SettlementPolicyBody{
			Method:        declarations.SettlementPolicyBody.Method,
			Applicability: declarations.SettlementPolicyBody.Applicability,
		}
		return content, true
	case domain.PriceRuleObject:
		if declarations.PricePolicyBody == nil {
			return content, false
		}
		body := declarations.PricePolicyBody
		content.PricePolicy = &domain.PricePolicyBody{
			Direction:     body.Direction,
			PricingPlan:   body.PricingPlan,
			PlanDirection: body.PlanDirection,
			Conversion:    body.Conversion,
			Scope:         body.Scope,
			Effective:     body.Effective,
		}
		// 口径是同一份正文的一部分（PricePolicyBodyDeclaration.Caliber 嵌在正文里），随正文一起进摘要；缺席不折进文档。
		if body.Caliber != nil {
			content.PricePolicy.Caliber = &domain.PricePolicyCaliberBody{
				Tax:        body.Caliber.Tax,
				Volumetric: body.Caliber.Volumetric,
				Fx:         body.Caliber.Fx,
			}
		}
		return content, true
	case domain.AcceptanceRulePackageObject:
		// 正文在不在场看 0014 那一层（rulePackageBody）：只带声明（时点锚、资料修订……）不带正文的项没有可比对象，照今天
		// 登记声明的串——批文里各键各自可缺是既有语义（ADR-0126 边界，判据同客户合同那一支）。正文在场时全部声明节一并
		// 折进同一个摘要（票 admin-write-faces/12「正文与全部声明在同一份载荷里、同一个摘要下」）。
		if declarations.RulePackageBody == nil {
			return content, false
		}
		content.AcceptanceRulePackage = acceptanceRulePackageBodyOf(declarations)
		return content, true
	case domain.PreAcceptanceFinancialControlPolicyObject:
		// 正文在不在场看 0024 那一层（preAcceptanceFinancialControlPolicyBody）——它与合同级的 PreAcceptanceControl 是两层
		// 不同的声明（ADR-0115），后者挂在合同版本上，不在本册的正文里。
		if declarations.PreAcceptanceFinancialControlPolicyBody == nil {
			return content, false
		}
		content.PreAcceptanceFinancialControlPolicy = &domain.PreAcceptanceFinancialControlPolicyBody{
			JointPass: declarations.PreAcceptanceFinancialControlPolicyBody.JointPass,
			Items:     declarations.PreAcceptanceFinancialControlPolicyBody.Items,
		}
		return content, true
	case domain.CustomerServiceRuleObject:
		// 正文在不在场看 0023 那一层（customerServiceRuleBody）。适用对象整格过去、不摊成产品 / 合同两键——恰一在场只在
		// CustomerServiceRuleApplicability 一处判（声明类型处的注释）；壳与正文的适用一致（ADR-0104 Decision 四）不在这里核，
		// 那是 declarationWrites 写入前的那一道，对账门只管声明的串与算出的串等不等。
		if declarations.CustomerServiceRuleBody == nil {
			return content, false
		}
		body := declarations.CustomerServiceRuleBody
		content.CustomerServiceRule = &domain.CustomerServiceRuleBody{
			Applicability: body.Applicability,
			Responsible:   body.Responsible,
			Scope:         body.Scope,
			Deadlines:     body.Deadlines,
			Materials:     body.Materials,
		}
		return content, true
	default:
		return content, false
	}
}

// acceptanceRulePackageBodyOf 把命令里归接单规则包册的各键折成本册的正文输入面；declarationsOfContent 是它的反向，
// 两处的键一一对应、同笔改。待路由许可不在其中——它挂在服务产品版本上（DeclarePendingRoutingPermission）。
func acceptanceRulePackageBodyOf(declarations CommercialDeclarations) *domain.AcceptanceRulePackageBody {
	body := &domain.AcceptanceRulePackageBody{
		Applicability:     declarations.RulePackageBody.Applicability,
		Rules:             declarations.RulePackageBody.Rules,
		AsOfPolicies:      declarations.AsOfPolicies,
		FinalRules:        declarations.FinalRules,
		FinalRuleValidity: declarations.FinalRuleValidity,
	}
	if declarations.AcceptanceContent != nil {
		body.AcceptanceContent = &domain.AcceptanceContentBody{
			ApplicableGroups: declarations.AcceptanceContent.ApplicableGroups,
			ManualReview:     declarations.AcceptanceContent.ManualReview,
		}
	}
	if declarations.IntakeQualification != nil {
		body.IntakeQualification = &domain.IntakeQualificationBody{
			Sources:        declarations.IntakeQualification.Sources,
			Qualifications: declarations.IntakeQualification.Qualifications,
		}
	}
	if declarations.SourceDataAmendment != nil {
		body.SourceDataAmendment = &domain.SourceDataAmendmentBody{
			Closed: declarations.SourceDataAmendment.Closed,
			Rules:  declarations.SourceDataAmendment.Rules,
		}
	}
	return body
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

	if declarations.FinalRuleValidity != nil && len(declarations.FinalRules) == 0 {
		// 有效期是终局规则声明上的一格，没有独立的拥有对象：只给有效期不给终局规则行，登进去也没有
		// 任何读口读得到它，这里整项拒（ADR-0119 Decision 五）。
		return nil, fmt.Errorf("final rule content: a label validity declaration needs the final rule declarations it belongs to")
	}
	if len(declarations.FinalRules) > 0 {
		var content domain.FinalRuleContent
		var err error
		if declarations.FinalRuleValidity != nil {
			content, err = domain.NewFinalRuleContentWithValidity(version, declarations.FinalRules, *declarations.FinalRuleValidity)
		} else {
			content, err = domain.NewFinalRuleContent(version, declarations.FinalRules)
		}
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

	if declarations.SettlementPolicyBody != nil {
		policy, err := domain.NewSettlementPolicy(
			version, declarations.SettlementPolicyBody.Method, declarations.SettlementPolicyBody.Applicability)
		if err != nil {
			return nil, fmt.Errorf("settlement policy body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: SettlementPolicyBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SaveSettlementPolicy(ctx, policy)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfSettlementPolicy(outcome)
			},
		})
	}

	if declarations.CreditPolicyBody != nil {
		body := declarations.CreditPolicyBody
		policy, err := domain.NewCreditPolicy(
			version, body.LegalEntity, body.Level, body.ChargeType, body.Limit, body.Effective)
		if err != nil {
			return nil, fmt.Errorf("credit policy body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: CreditPolicyBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SaveCreditPolicy(ctx, policy)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfCreditPolicy(outcome)
			},
		})
	}

	if declarations.SupplierAgreementBody != nil {
		body := declarations.SupplierAgreementBody
		agreement, err := domain.NewSupplierAgreement(
			version, body.Supplier, body.LegalEntity, body.Scope, body.PurchasePlan, body.Effective)
		if err != nil {
			return nil, fmt.Errorf("supplier agreement body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: SupplierAgreementBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SaveSupplierAgreement(ctx, agreement)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfSupplierAgreement(outcome)
			},
		})
	}

	if declarations.PricePolicyBody != nil {
		body := declarations.PricePolicyBody
		policy, err := domain.NewCommercialPricePolicy(
			version, body.Direction, body.PricingPlan, body.PlanDirection, body.Conversion, body.Scope, body.Effective)
		if err != nil {
			return nil, fmt.Errorf("price policy body: %w", err)
		}
		planDirection, conversion := body.PlanDirection, body.Conversion
		writes = append(writes, declarationWrite{
			channel: PricePolicyBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SavePricePolicy(ctx, policy, planDirection, conversion)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfPricePolicy(outcome)
			},
		})

		if body.Caliber != nil {
			caliber, err := pricePolicyCaliberOf(version, *body.Caliber)
			if err != nil {
				return nil, fmt.Errorf("price policy caliber: %w", err)
			}
			// 口径与正文是同一份声明的两半：方向对不上整项拒绝，正文也不写。
			if err := caliber.ConsistentWithDirection(body.Direction); err != nil {
				return nil, fmt.Errorf("price policy caliber: %w", err)
			}
			// 紧跟正文之后写：0022 的外键要求正文行先在。
			writes = append(writes, declarationWrite{
				channel: PricePolicyCaliberChannel,
				save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
					outcome, err := registry.SavePricePolicyCaliber(ctx, caliber)
					if err != nil {
						return ports.DeclarationSaveOutcomeInvalid, err
					}
					return declarationOutcomeOfPricePolicyCaliber(outcome)
				},
			})
		}
	}

	if declarations.CustomerServiceRuleBody != nil {
		body := declarations.CustomerServiceRuleBody
		// ADR-0104 Decision 四：壳上指名了正文所挂那一类（产品 / 合同）的引用时两处必须相等，
		// 不等整项拒绝，不静默选一处——读侧同一道核对在点读时还会再走一遍。
		if err := domain.ConsistentCustomerServiceRuleApplicability(version, body.Applicability); err != nil {
			return nil, err
		}
		rule, err := domain.NewCustomerServiceRuleVersion(
			version, body.Applicability, body.Responsible, body.Scope, body.Deadlines, body.Materials)
		if err != nil {
			return nil, fmt.Errorf("customer service rule body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: CustomerServiceRuleBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SaveCustomerServiceRule(ctx, rule)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfCustomerServiceRule(outcome)
			},
		})
	}

	if declarations.PreAcceptanceFinancialControlPolicyBody != nil {
		body := declarations.PreAcceptanceFinancialControlPolicyBody
		policy, err := domain.NewPreAcceptanceFinancialControlPolicy(version, body.JointPass, body.Items)
		if err != nil {
			return nil, fmt.Errorf("pre-acceptance financial control policy body: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: PreAcceptanceFinancialControlPolicyBodyChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				outcome, err := registry.SavePreAcceptanceFinancialControlPolicy(ctx, policy)
				if err != nil {
					return ports.DeclarationSaveOutcomeInvalid, err
				}
				return declarationOutcomeOfPreAcceptanceFinancialControlPolicy(outcome)
			},
		})
	}

	if len(declarations.ContractDelegations) > 0 {
		content, err := domain.NewContractDelegationContent(version, declarations.ContractDelegations)
		if err != nil {
			return nil, fmt.Errorf("contract delegations: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: ContractDelegationChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveContractDelegations(ctx, content)
			},
		})
	}

	if declarations.SourceDataAmendment != nil {
		// 归属由领域构造门把守：挂在合同或授权规则上会被拒（ADR-0120 Decision 一），不静默丢弃。
		content, err := domain.NewSourceDataAmendmentAllowanceContent(
			version, declarations.SourceDataAmendment.Closed, declarations.SourceDataAmendment.Rules)
		if err != nil {
			return nil, fmt.Errorf("source data amendment allowance: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: SourceDataAmendmentChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveSourceDataAmendmentAllowance(ctx, content)
			},
		})
	}

	if declarations.DeliveryConditions != nil {
		content, err := deliveryConditionsOf(version, *declarations.DeliveryConditions)
		if err != nil {
			return nil, fmt.Errorf("delivery conditions: %w", err)
		}
		writes = append(writes, declarationWrite{
			channel: DeliveryConditionChannel,
			save: func(ctx context.Context, registry ports.PublicationRegistry) (ports.DeclarationSaveOutcome, error) {
				return registry.SaveDeliveryConditions(ctx, content)
			},
		})
	}

	return writes, nil
}

// deliveryConditionsOf 按发布的版本类别选层：客户合同版本走合同层的门（必须指名所收紧的产品版本），其余一律走产品层
// 的门——产品层的门只认已生效的服务产品版本，挂在接单规则包上会在那里被拒（ADR-0133 决定四），不静默丢弃。产品层
// 带着 Tightens 是类别错误：只有合同能收紧产品。
func deliveryConditionsOf(
	version domain.CommercialVersion,
	declaration DeliveryConditionDeclaration,
) (domain.DeliveryConditionContent, error) {
	if version.Kind() == domain.CustomerContractObject {
		if declaration.Tightens == nil {
			return domain.DeliveryConditionContent{}, fmt.Errorf(
				"%w: a customer contract's delivery conditions must name the service product version they tighten",
				domain.ErrDeliveryConditionNotConfigured)
		}
		return domain.DeclareContractDeliveryConditions(version, *declaration.Tightens, declaration.Terms)
	}
	if declaration.Tightens != nil {
		return domain.DeliveryConditionContent{}, fmt.Errorf(
			"%w: only a customer contract version tightens a service product's delivery conditions",
			domain.ErrDeliveryConditionOwner)
	}
	return domain.DeclareProductDeliveryConditions(version, declaration.Terms)
}

// declarationOutcomeOfPreAcceptanceFinancialControlPolicy 把策略正文册的落点折成声明通道的落点，判据同
// declarationOutcomeOfSettlementPolicy：折的是「落在哪一格」，逐值折不做数值转换。
func declarationOutcomeOfPreAcceptanceFinancialControlPolicy(
	outcome ports.PreAcceptanceFinancialControlPolicySaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.PreAcceptanceFinancialControlPolicySaved:
		return ports.DeclarationSaved, nil
	case ports.PreAcceptanceFinancialControlPolicyAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.PreAcceptanceFinancialControlPolicyContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("pre-acceptance financial control policy body: 集合外的策略正文落点 %q", outcome)
	}
}

// declarationOutcomeOfCustomerServiceRule 把客户服务规则册的落点折成声明通道的落点，判据同
// declarationOutcomeOfSettlementPolicy：折的是「落在哪一格」，逐值折不做数值转换。
func declarationOutcomeOfCustomerServiceRule(
	outcome ports.CustomerServiceRuleSaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.CustomerServiceRuleSaved:
		return ports.DeclarationSaved, nil
	case ports.CustomerServiceRuleAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.CustomerServiceRuleContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("customer service rule body: 集合外的客户服务规则落点 %q", outcome)
	}
}

// pricePolicyCaliberOf 按汇率格在不在场选构造门：nil 是合法缺席，走不带 Fx 的那条；零值 Fx 只会
// 从带指针的那条进来并被领域拒，缺席与缺件因此在这里就分开。
func pricePolicyCaliberOf(
	version domain.CommercialVersion,
	declaration PricePolicyCaliberDeclaration,
) (domain.PricePolicyCaliber, error) {
	if declaration.Fx == nil {
		return domain.NewPricePolicyCaliber(version, declaration.Tax, declaration.Volumetric)
	}
	return domain.NewPricePolicyCaliberWithFx(version, declaration.Tax, declaration.Volumetric, *declaration.Fx)
}

// declarationOutcomeOfPricePolicy 与 declarationOutcomeOfPricePolicyCaliber 把两册各自的落点折成
// 声明通道的落点，判据同 declarationOutcomeOfSettlementPolicy。
func declarationOutcomeOfPricePolicy(
	outcome ports.PricePolicySaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.PricePolicySaved:
		return ports.DeclarationSaved, nil
	case ports.PricePolicyAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.PricePolicyContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("price policy body: 集合外的价格政策落点 %q", outcome)
	}
}

func declarationOutcomeOfPricePolicyCaliber(
	outcome ports.PricePolicyCaliberSaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.PricePolicyCaliberSaved:
		return ports.DeclarationSaved, nil
	case ports.PricePolicyCaliberAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.PricePolicyCaliberContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("price policy caliber: 集合外的口径落点 %q", outcome)
	}
}

// declarationOutcomeOfCreditPolicy 与 declarationOutcomeOfSupplierAgreement 把两册各自的落点
// 折成声明通道的落点，判据与 declarationOutcomeOfSettlementPolicy 同一条：折的是「落在哪一格」，
// 逐值折不做数值转换，任一族多出一格时这里响亮失败。
func declarationOutcomeOfCreditPolicy(
	outcome ports.CreditPolicySaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.CreditPolicySaved:
		return ports.DeclarationSaved, nil
	case ports.CreditPolicyAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.CreditPolicyContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("credit policy body: 集合外的信用政策落点 %q", outcome)
	}
}

func declarationOutcomeOfSupplierAgreement(
	outcome ports.SupplierAgreementSaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.SupplierAgreementSaved:
		return ports.DeclarationSaved, nil
	case ports.SupplierAgreementAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.SupplierAgreementContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("supplier agreement body: 集合外的供应商协议落点 %q", outcome)
	}
}

// declarationOutcomeOfSettlementPolicy 把结算政策册的落点折成声明通道的落点。
//
// 折的是「落在哪一格」，不是「两族正文是同一种东西」。端口上的两族 Save 因此不合并：
// 政策册按版本四元组存一行方式与六维范围，声明表按拥有版本挂一份正文，两者的行形状与
// 冲突判据各自不同。三格同构也不是这两族碰巧一样——ADR-0031 给本上下文所有登记面定的
// 就是同一条纪律：重放与内容冲突都不是 error，都绝不覆盖，事务保持可用。
//
// 逐值折而不做数值转换：将来任一族多出一格时，这里会响亮失败，而不是把新格静静读成
// 旧格里的某一个。
func declarationOutcomeOfSettlementPolicy(
	outcome ports.SettlementPolicySaveOutcome,
) (ports.DeclarationSaveOutcome, error) {
	switch outcome {
	case ports.SettlementPolicySaved:
		return ports.DeclarationSaved, nil
	case ports.SettlementPolicyAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.SettlementPolicyContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid,
			fmt.Errorf("settlement policy body: 集合外的结算政策落点 %q", outcome)
	}
}
