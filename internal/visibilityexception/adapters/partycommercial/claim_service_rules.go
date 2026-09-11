// Package partycommercial 是 visibility-exception 对 party-commercial 的消费侧适配器
// （ADR-0025：只有本包可以同时导入两个上下文；适配器只翻译不判断，翻译必须是全函数）。
//
// 首发只翻译一条缝：索赔资格规则里的首次索赔期限与最低材料两维，从 PC 的客户服务规则正文
// （ADR-0104）读回来。VE 的期限硬句要五样一体——适用规则版本、起算事件、业务时区或日历、截止
// 时间、适用范围——其中前三样与范围是**规则**，PC 交；截止时间是 VE 拿起算事实与规则**派生**
// 出来的事实，PC 不交（PC 类型注释「截止时刻本身不在这里算」）。派生要一个起算事实源与一份按
// 引用取日历的能力，两样今天 VE 都没有（票 ve-claims-read-seams/03「裁决」），所以本包只填规则
// 半边，`Deadline`、`SupplementDeadline` 与没有任何来源的 `Notice` 留零值——编排侧既有守卫把
// 「登了规则而截止算不出」判成核不了、停在指名到维的未决，索赔项一字不动。这里若拿本方时钟或
// 日历凑一个截止，「租户还没登记」就会变成一次有依据的超期拒赔。
//
// 两维按哪一版规则读，由目标包裹所属委托**接受时固定的商业解析回指**决定（ADR-0136 决定一 / 二）：
// 缝跨两个提供方——目标包裹 → 回指归 parcel-shipment（按租户 + 包裹身份答），回指 → 闭包 → 已采用
// 的客户服务规则版本归 party-commercial（按租户 + 回指答）。本包不自己解析商业依据、不登键、不持
// 合同维、不在任何一处拿系统时间或默认范围顶一个键：选用时点就是那次接受的商业选择锚点，已固定在
// 闭包里，接受后规则发了新版本也不改口（PC CONTEXT「委托被接受时固定其适用的…」）。实例半边落在
// parcel-shipment 既有的解析键登记面——那一行的必需依据要含客户服务规则与客户合同两类（ADR-0136
// 决定三 / 四），VE 侧没有键登记面。形照 transport-fulfillment 的交付条件适配器（同为两个提供方的
// 消费侧适配器）。
package partycommercial

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容——提供方阶段契约被打破（PS 答在场
	// 却没有回指、PC 交回别的租户或别的回指的闭包、闭包在客户服务规则那一格采用了别的类别冒名）或 PC 引用
	// 译不进 VE 的构造门。它是编程或配置错误，不是业务答案；哨兵与本仓其余跨上下文适配器同名同义。
	ErrUntranslatableAnswer = errors.New("visibility exception partycommercial adapter: untranslatable answer")
	// ErrCommercialClosureAbsent 表示回指所指的闭包在 party-commercial 不在场。对一份已接受委托的回指，这是
	// 提供方缺数据，不是「没登规则」（ADR-0136 决定二；判据同 ADR-0133 决定二「闭包不在场 → error」）——折成
	// 未登记会把租户支去登一份与缺口无关的东西。
	ErrCommercialClosureAbsent = errors.New("visibility exception partycommercial adapter: no commercial closure is held for the acceptance-time resolution reference")
	// ErrCustomerContractNotAdopted 表示闭包在场却未采用客户合同版本。PC CONTEXT「委托被接受时固定其适用的…
	// 客户合同版本」说它恒在，缺了是接受流的装配缺陷（ADR-0136 决定三；判据同 ADR-0133 决定二、ADR-0062
	// 决定三）。
	ErrCustomerContractNotAdopted = errors.New("visibility exception partycommercial adapter: the commercial closure did not adopt a customer contract version")
)

// CommercialResolutionReferenceSource 是本适配器向 parcel-shipment 取商业解析回指的窄口（ADR-0136 决定四）。
// 签名照 PS ports.CommercialResolutionReferenceView.LoadCommercialResolutionReference；真实装配交给 PS 的
// adapters/postgres.ShipmentRequests（它兼实现那个读口）。这里只声明读的这一个方法、只 import PS 的 domain，
// 不 import 它的 application / ports——判据同 TF 交付条件适配器的同名窄口。接口留在本包不进 VE ports：签名
// 引用提供方类型（ADR-0025）。
type CommercialResolutionReferenceSource interface {
	LoadCommercialResolutionReference(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.CommercialResolutionID, bool, error)
}

// ClaimServiceRulesDeps 收拢四个协作方。Rules 是 VE 自己的册（合同责任范围 + 申请人授权目录，
// adapters/postgres 的两个形状任一），本适配器在它的答案上叠两维；References 是 parcel-shipment 的回指读口，
// Closures 是 party-commercial 的闭包读口，Contents 是 party-commercial 的正文点读口（ADR-0104 决定四）——
// 提供方那几只全是只读半边，本包不持任何写口（判据同 PC ports.CommercialResolutionView 头注）。
type ClaimServiceRulesDeps struct {
	Rules      veports.EligibilityRuleView
	References CommercialResolutionReferenceSource
	Closures   pcports.CommercialResolutionView
	Contents   pcports.CustomerServiceRuleContentView
}

// ClaimServiceRules 实现 veports.EligibilityRuleView：先让 VE 自己的册作答，声明在场时再把首次
// 索赔期限与最低材料两维换成从 PC 客户服务规则正文翻译来的答案。
//
// 装饰而不是重写：另三维（合同责任范围承不承担该类型、申请人授权目录、声明版本）是 VE 自己的
// 判断参数（ADR-0104 Decision 一），本包一个字不碰；「声明不在场」（第二个返回值为 false）也原样
// 交回——连「这个类型在不在保」都无从谈起时，两维没有可挂的地方。
//
// 两维各自的 Registered 按「PC 那一项有没有行」答（票 03「裁决」）：一版规则可以只登期限不登
// 材料，版本壳在场不等于两维都登记；缺哪一项去 PC 补哪一项。
type ClaimServiceRules struct {
	rules      veports.EligibilityRuleView
	references CommercialResolutionReferenceSource
	closures   pcports.CommercialResolutionView
	contents   pcports.CustomerServiceRuleContentView
}

// NewClaimServiceRules 装配。四个协作方一个都不许缺（ADR-0079 决定八：nil 半边在装配期拒）：本适配器
// 一旦装上就是要真去问两个提供方的，缺一半而静默答未登记会让一次装配疏漏与租户没登记长得一样；
// 「答不出」在读口上会长成 error，日后有人会把它读成提供方坏了。
func NewClaimServiceRules(deps ClaimServiceRulesDeps) (*ClaimServiceRules, error) {
	if deps.Rules == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: own eligibility rule view is nil")
	}
	if deps.References == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: commercial resolution reference source is nil")
	}
	if deps.Closures == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: commercial resolution view is nil")
	}
	if deps.Contents == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: customer service rule content view is nil")
	}
	return &ClaimServiceRules{
		rules:      deps.Rules,
		references: deps.References,
		closures:   deps.Closures,
		contents:   deps.Contents,
	}, nil
}

var _ veports.EligibilityRuleView = (*ClaimServiceRules)(nil)

// RulesForClaim 取适用于一项索赔的资格规则。第二个返回值与 error 的语义随端口：false 只有
// 「合同的索赔资格声明不在场」一个意思，依赖调不通作为错误返回。
func (adapter *ClaimServiceRules) RulesForClaim(
	ctx context.Context,
	query veports.EligibilityQuery,
) (veports.EligibilityRules, bool, error) {
	rules, declared, err := adapter.rules.RulesForClaim(ctx, query)
	if err != nil || !declared {
		return rules, declared, err
	}
	deadline, materials, err := adapter.serviceRuleDimensions(ctx, query)
	if err != nil {
		return veports.EligibilityRules{}, false, err
	}
	rules.FilingDeadline = deadline
	rules.Materials = materials
	return rules, true, nil
}

// serviceRuleDimensions 分三段（ADR-0136 决定二）：目标包裹 → 回指（parcel-shipment）→ 闭包与已采用的客户
// 服务规则版本（party-commercial）→ 按选中的版本点读正文并翻译。结果代数按恢复动作分格（ADR-0029），逐格
// 写在各段上；这一层只剩第三段那一格：规则版本在场而正文未登（found=false）→ 两维未登记，恢复动作是去 PC
// 登正文（票 03 原格）。
func (adapter *ClaimServiceRules) serviceRuleDimensions(
	ctx context.Context,
	query veports.EligibilityQuery,
) (veports.FilingDeadlineRule, veports.MinimumMaterialsRule, error) {
	var unregistered veports.FilingDeadlineRule
	var noMaterials veports.MinimumMaterialsRule

	tenant, resolution, present, err := adapter.acceptanceTimeResolution(ctx, query)
	if err != nil || !present {
		return unregistered, noMaterials, err
	}
	version, adopted, err := adapter.adoptedRuleVersion(ctx, tenant, resolution)
	if err != nil || !adopted {
		return unregistered, noMaterials, err
	}

	rule, found, err := adapter.contents.LoadCustomerServiceRule(ctx, tenant, version)
	if err != nil {
		return unregistered, noMaterials, fmt.Errorf("load customer service rule: %w", err)
	}
	if !found {
		return unregistered, noMaterials, nil
	}
	return translateRule(rule, query)
}

// acceptanceTimeResolution 是第一段：把索赔项的目标范围引用原样作声明包裹身份问 parcel-shipment，交回译成
// PC 词的租户与回指。本包不猜目标范围引用是不是包裹（ADR-0136 决定二；判据同 TF 交付条件适配器对载运对象
// 的处置）：PS 答「没有」就是没有——目标不属任何已接受委托的成员集合，含目标是明确服务责任范围、集运单元、
// 不可见对象——两维据以答未登记，与本票之前「键来源未配置」那一行同一可观察行为（ADR-0136 越权风险点 1
// 把「明确服务范围的索赔项按什么选规则」留给 VE owner，本包不替它开第二条路）。PS 答 error 原样上抛；答
// 在场却没有回指是读口违约，不是任一格业务答案。
//
// 租户与包裹身份在两个上下文里是同一串字面（约定同 TF 交付条件适配器）；回指按 ADR-0027 的回指入口重建成
// PC 的解析标识，不拆不拼（两段式串只许 PC 一处拼，ADR-0080 决定七）。
func (adapter *ClaimServiceRules) acceptanceTimeResolution(
	ctx context.Context,
	query veports.EligibilityQuery,
) (pcdomain.TenantID, pcdomain.ResolutionID, bool, error) {
	var noTenant pcdomain.TenantID
	var noResolution pcdomain.ResolutionID

	psTenant, err := psdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return noTenant, noResolution, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(query.Target.String())
	if err != nil {
		return noTenant, noResolution, false, fmt.Errorf("%w: claim target: %v", ErrUntranslatableAnswer, err)
	}
	reference, present, err := adapter.references.LoadCommercialResolutionReference(ctx, psTenant, parcel)
	if err != nil {
		return noTenant, noResolution, false, fmt.Errorf("load commercial resolution reference: %w", err)
	}
	if !present {
		return noTenant, noResolution, false, nil
	}
	if reference.String() == "" {
		return noTenant, noResolution, false, fmt.Errorf(
			"%w: parcel shipment answered present with an empty resolution reference", ErrUntranslatableAnswer)
	}

	tenant, err := pcdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return noTenant, noResolution, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolution, err := pcdomain.NewResolutionID(reference.String())
	if err != nil {
		return noTenant, noResolution, false, fmt.Errorf("%w: resolution reference: %v", ErrUntranslatableAnswer, err)
	}
	return tenant, resolution, true, nil
}

// adoptedRuleVersion 是第二段：按（租户，回指）取接受时固定的闭包，再取它已采用的客户服务规则版本。逐格
// 分派不留兜底（ADR-0025），每格的恢复动作不同（ADR-0029）：
//
//   - 闭包不在场 → ErrCommercialClosureAbsent：对一份已接受委托的回指是提供方缺数据，不折成未登记。
//   - 闭包在场却未采用客户合同版本 → ErrCustomerContractNotAdopted（ADR-0136 决定三）。
//   - 闭包在场、合同也采用了，却未采用客户服务规则版本 → 两维未登记。这是「没登」的一种，但缺的不是规则
//     正文：租户没在 parcel-shipment 的解析键登记面把客户服务规则列进必需依据，接受时闭包就没有这一成员；
//     恢复动作是**去 PS 解析键登记面把它列进必需依据**，与第三段「正文未登」同格不同因，所以分开写。已接受
//     的委托不会因此追溯取得规则版本——接受时没固定的，就是那次接受没有采用的依据，本包不替它补
//     （PC CONTEXT「不能以缺失代替判断」同一立场）。
//   - 闭包属别的租户或别的回指、结局不是唯一已解析、客户服务规则那一格采用的版本不是这一类或不属这一租户
//     → ErrUntranslatableAnswer：提供方阶段契约被打破，不是任一格业务答案。租户是身份不是过滤器
//     （ADR-0003），这里不纠正、不沉默，报出去。
//
// 读一份已固定的闭包没有`适用冲突` / `解析未决` / `输入未受理`可答（ADR-0136 决定二）：那几格属第一阶段
// 解析，本包不再走它。
func (adapter *ClaimServiceRules) adoptedRuleVersion(
	ctx context.Context,
	tenant pcdomain.TenantID,
	resolution pcdomain.ResolutionID,
) (pcdomain.CommercialVersion, bool, error) {
	var none pcdomain.CommercialVersion

	closure, found, err := adapter.closures.LoadResolution(ctx, tenant, resolution)
	if err != nil {
		return none, false, fmt.Errorf("load commercial resolution: %w", err)
	}
	if !found {
		return none, false, fmt.Errorf("%w: tenant %q resolution %q", ErrCommercialClosureAbsent, tenant, resolution)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		return none, false, fmt.Errorf(
			"%w: the held closure for %q has outcome %q", ErrUntranslatableAnswer, resolution, closure.Outcome())
	}
	if closure.ResolutionKey().TenantID != tenant || closure.ResolutionID() != resolution {
		return none, false, fmt.Errorf(
			"%w: party commercial answered closure %q of tenant %q for tenant %q resolution %q",
			ErrUntranslatableAnswer, closure.ResolutionID(), closure.ResolutionKey().TenantID, tenant, resolution)
	}
	if _, ok := closure.AdoptedFor(pcdomain.CustomerContractObject); !ok {
		return none, false, fmt.Errorf("%w: resolution %q", ErrCustomerContractNotAdopted, resolution)
	}
	adopted, ok := closure.AdoptedFor(pcdomain.CustomerServiceRuleObject)
	if !ok {
		return none, false, nil
	}
	version := adopted.Version()
	if version.Kind() != pcdomain.CustomerServiceRuleObject || version.Tenant() != tenant {
		return none, false, fmt.Errorf(
			"%w: the closure adopted %q %q of tenant %q in the customer service rule slot",
			ErrUntranslatableAnswer, version.Kind(), version.ObjectID(), version.Tenant())
	}
	return version, true, nil
}

// translateRule 把一版正文翻成 VE 的两条规则。两维各按自己那一行的在场答 Registered；RuleVersion
// 冻三段版本引用（租户 / 对象 / 版本号，ADR-0104 Decision 五），不冻正文。Scope 取索赔自己固定
// 的目标范围。截止时刻、补充截止与通知依据留零值，理由见包注释。
func translateRule(
	rule pcdomain.CustomerServiceRuleVersion,
	query veports.EligibilityQuery,
) (veports.FilingDeadlineRule, veports.MinimumMaterialsRule, error) {
	reference := ruleVersionReference(rule.Version())

	var deadline veports.FilingDeadlineRule
	if row, registered := rule.ClaimDeadline(pcdomain.FirstClaimDeadline); registered {
		deadline = veports.FilingDeadlineRule{
			Registered:  true,
			RuleVersion: reference,
			StartEvent:  row.StartEvent().String(),
			Calendar:    row.Calendar().String(),
			Scope:       query.Target.String(),
		}
	}

	var materials veports.MinimumMaterialsRule
	claimKind, err := pcdomain.NewClaimKindReference(query.Kind.String())
	if err != nil {
		return veports.FilingDeadlineRule{}, veports.MinimumMaterialsRule{}, fmt.Errorf(
			"%w: claim kind reference: %v", ErrUntranslatableAnswer, err)
	}
	if row, registered := rule.MinimumMaterialsFor(claimKind); registered {
		required, err := requiredMaterials(row)
		if err != nil {
			return veports.FilingDeadlineRule{}, veports.MinimumMaterialsRule{}, err
		}
		materials = veports.MinimumMaterialsRule{
			Registered:  true,
			RuleVersion: reference,
			Required:    required,
		}
	}
	return deadline, materials, nil
}

// requiredMaterials 逐条把 PC 的材料条目引用译进 VE 的构造门。PC 只校非空，VE 也只校非空，译不进
// 只可能是构造门此后收紧了——那要人来看，不能悄悄少一条：差集因此永远缺那一件。
func requiredMaterials(row pcdomain.MinimumMaterialsRule) ([]vedomain.MaterialRequirementReference, error) {
	source := row.Materials()
	required := make([]vedomain.MaterialRequirementReference, 0, len(source))
	for _, material := range source {
		translated, err := vedomain.NewMaterialRequirementReference(material.String())
		if err != nil {
			return nil, fmt.Errorf("%w: material requirement reference: %v", ErrUntranslatableAnswer, err)
		}
		required = append(required, translated)
	}
	return required, nil
}

func ruleVersionReference(version pcdomain.CommercialVersion) string {
	return version.Tenant().String() + "/" + version.ObjectID().String() + "/" + version.Version().String()
}
