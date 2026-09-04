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
package partycommercial

import (
	"context"
	"errors"
	"fmt"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容——键配错、提供方阶段契约
	// 被打破或 PC 引用译不进 VE 的构造门。它是编程或配置错误，不是业务答案；哨兵与本仓其余跨上下文
	// 适配器同名同义。
	ErrUntranslatableAnswer = errors.New("visibility exception partycommercial adapter: untranslatable answer")
	// ErrCustomerServiceRuleUnresolved 表示 PC 闭包没有唯一解出客户服务规则版本，且成因不是「没人登记过」：
	// `适用冲突`要商业依据所有方修正区间重叠，`解析未决`要重试权威或等实例参数落地，`输入未受理`
	// 是键立不起来。VE 端口只有「未登记」与 error 两个格，这三种的恢复动作都不是去登记，所以走 error
	// 让人来看——折成未登记会把租户支去补一份其实已经存在的规则。
	ErrCustomerServiceRuleUnresolved = errors.New("visibility exception partycommercial adapter: customer service rule resolution did not settle")
)

// RuleResolutionKeySource 把一次资格规则查询折成 PC 的闭包解析键。
//
// 查询只带租户与客户账户；闭包键还要责任法人候选、商业范围、目的与锚点（时刻 + 锚点策略版本），
// 那些全是实例半边——没有租户时谁也说不出这个客户的索赔该在哪个商业范围、按哪个时点选规则版本
// （判据同 parcel-shipment 侧的 ResolutionKeySource）。接口留在本包不进 ports：它的签名引用提供方
// 的类型（ADR-0025）。nil 或第二个返回值为 false 都是「显式未配置」，两维据以答未登记；本包不代拟
// 任何一项，尤其不拿系统当前时间顶锚点。
//
// 键上的必需依据由登记方给，但必须含 CustomerServiceRuleObject——不含就永远选不出规则版本，那是
// 配置缺陷不是缺席，适配器以 ErrUntranslatableAnswer 拒绝而不折成未登记。要不要一并要客户合同让
// 闭包核指名引用，归登记方。
type RuleResolutionKeySource interface {
	FormRuleResolutionKey(ctx context.Context, query veports.EligibilityQuery) (pcdomain.ClosureResolutionKey, bool, error)
}

// ClaimServiceRulesDeps 收拢四个协作方。Rules 是 VE 自己的册（合同责任范围 + 申请人授权目录，
// adapters/postgres 的两个形状任一），本适配器在它的答案上叠两维；Resolve 与 Contents 是提供方
// 以用例与端口提供的两半（ADR-0027 之后适配器直接调用，不再转手）；Keys 见 RuleResolutionKeySource。
type ClaimServiceRulesDeps struct {
	Rules    veports.EligibilityRuleView
	Resolve  *pcapplication.ResolveCommercialBasisHandler
	Contents pcports.CustomerServiceRuleContentView
	Keys     RuleResolutionKeySource
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
	rules    veports.EligibilityRuleView
	resolve  *pcapplication.ResolveCommercialBasisHandler
	contents pcports.CustomerServiceRuleContentView
	keys     RuleResolutionKeySource
}

// NewClaimServiceRules 装配。Keys 允许为 nil（显式未配置），其余三个不许：本适配器一旦装上就是
// 要真去问商业侧的，缺一半而静默答未登记会让一次装配疏漏与租户没登记长得一样。
func NewClaimServiceRules(deps ClaimServiceRulesDeps) (*ClaimServiceRules, error) {
	if deps.Rules == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: own eligibility rule view is nil")
	}
	if deps.Resolve == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: commercial resolution handler is nil")
	}
	if deps.Contents == nil {
		return nil, fmt.Errorf("visibility exception partycommercial adapter: customer service rule content view is nil")
	}
	return &ClaimServiceRules{
		rules:    deps.Rules,
		resolve:  deps.Resolve,
		contents: deps.Contents,
		keys:     deps.Keys,
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

// serviceRuleDimensions 分三段：查询折成键 → 经 PC 既有闭包选中规则版本壳（ADR-0104 Decision 四，
// 不另开口）→ 按选中的版本点读正文并翻译。三处缺席（键形不成、范围里没有生效版本、壳在正文没登）
// 都答两维未登记：恢复动作都是去登记，只是登的东西不同。
func (adapter *ClaimServiceRules) serviceRuleDimensions(
	ctx context.Context,
	query veports.EligibilityQuery,
) (veports.FilingDeadlineRule, veports.MinimumMaterialsRule, error) {
	var unregistered veports.FilingDeadlineRule
	var noMaterials veports.MinimumMaterialsRule
	if adapter.keys == nil {
		return unregistered, noMaterials, nil
	}
	key, formed, err := adapter.keys.FormRuleResolutionKey(ctx, query)
	if err != nil {
		return unregistered, noMaterials, fmt.Errorf("form rule resolution key: %w", err)
	}
	if !formed {
		return unregistered, noMaterials, nil
	}
	if key.TenantID.String() != query.Tenant.String() {
		// 键来源替别的租户成了键：按它去解会拿另一户的规则给这一户用。租户是身份不是过滤器
		// （ADR-0003），这里不纠正、不沉默，报出去。
		return unregistered, noMaterials, fmt.Errorf(
			"%w: key source formed a key for tenant %q while the query carries %q",
			ErrUntranslatableAnswer, key.TenantID, query.Tenant)
	}
	if !requiresCustomerServiceRule(key) {
		return unregistered, noMaterials, fmt.Errorf(
			"%w: resolution key does not require a customer service rule", ErrUntranslatableAnswer)
	}

	answer, err := adapter.resolve.Handle(ctx, pcapplication.ResolveCommercialBasisCommand{Key: key})
	if err != nil {
		return unregistered, noMaterials, fmt.Errorf("resolve customer service rule: %w", err)
	}
	version, selected, err := adoptedRuleVersion(answer)
	if err != nil || !selected {
		return unregistered, noMaterials, err
	}

	rule, found, err := adapter.contents.LoadCustomerServiceRule(ctx, key.TenantID, version)
	if err != nil {
		return unregistered, noMaterials, fmt.Errorf("load customer service rule: %w", err)
	}
	if !found {
		return unregistered, noMaterials, nil
	}
	return translateRule(rule, query)
}

// adoptedRuleVersion 把第一阶段闭包折成「选中了哪一版 / 没有可选的 / 解不出来」三格，逐格分派不留
// 兜底（ADR-0025）：提供方日后新增的一格必须在这里炸出来，而不是静默落进某一支。
func adoptedRuleVersion(answer pcapplication.ResolveCommercialBasisResult) (pcdomain.CommercialVersion, bool, error) {
	closure := answer.Closure()
	switch closure.Outcome() {
	case pcdomain.UniquelyResolved:
		if answer.Fixed() == pcports.ResolutionContentConflict {
			// 同标识异内容：库里那份与本次解出的不是一回事，而 ADR-0031 禁止静默覆盖，谁也说不准
			// 该按哪份的规则版本去读正文。
			return pcdomain.CommercialVersion{}, false, fmt.Errorf(
				"%w: fixing the resolution answered %q", ErrCustomerServiceRuleUnresolved, answer.Fixed())
		}
		adopted, ok := closure.AdoptedFor(pcdomain.CustomerServiceRuleObject)
		if !ok {
			// 键已核过含客户服务规则，唯一闭包却没采用它：提供方阶段契约被打破。
			return pcdomain.CommercialVersion{}, false, fmt.Errorf(
				"%w: uniquely resolved closure adopted no customer service rule", ErrUntranslatableAnswer)
		}
		return adopted.Version(), true, nil
	case pcdomain.NoApplicableBasis:
		// 权威说了这个范围里没有生效的客户服务规则版本——那是「没人登记过」，去发布一版。
		return pcdomain.CommercialVersion{}, false, nil
	case pcdomain.ApplicabilityConflict, pcdomain.ResolutionPending, pcdomain.InputNotAccepted:
		return pcdomain.CommercialVersion{}, false, fmt.Errorf(
			"%w: closure outcome %q reason %q", ErrCustomerServiceRuleUnresolved, closure.Outcome(), closure.Reason())
	case pcdomain.ResolutionStale, pcdomain.BasisNotResolved:
		// 第一阶段从不产这两个：两者都以「已有原解析」为前提。
		return pcdomain.CommercialVersion{}, false, fmt.Errorf(
			"%w: first-phase closure outcome %q", ErrUntranslatableAnswer, closure.Outcome())
	default:
		return pcdomain.CommercialVersion{}, false, fmt.Errorf(
			"%w: first-phase closure outcome %d", ErrUntranslatableAnswer, closure.Outcome())
	}
}

func requiresCustomerServiceRule(key pcdomain.ClosureResolutionKey) bool {
	for _, kind := range key.RequiredBases {
		if kind == pcdomain.CustomerServiceRuleObject {
			return true
		}
	}
	return false
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
