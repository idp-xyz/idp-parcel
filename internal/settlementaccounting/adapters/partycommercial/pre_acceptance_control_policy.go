// Package partycommercial 是 settlement-accounting 对 party-commercial 的消费侧适配器
// （ADR-0025：只有本包可以同时导入两个上下文；适配器只翻译不判断，翻译必须是全函数）。
//
// 它是 ADR-0054 当年预留而一直空着的那个落点：那份记录给 SA 的控制策略读口补齐了「未配置」
// 格，却明写「本记录不解决提供方表面」。提供方表面在票 03（PC 声明发布写入方）之后已经就位，
// 缺的只是这一段翻译。
package partycommercial

import (
	"context"
	"errors"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容——编程错误或提供方
// 阶段契约被打破，不是业务答案。哨兵与本仓其余跨上下文适配器同名同义。
var ErrUntranslatableAnswer = errors.New("settlement accounting partycommercial adapter: untranslatable answer")

// PreAcceptanceControlPolicy 实现 saports.PreAcceptanceControlPolicyView：凭调用方回指的
// 商业解析取已固定的闭包，从闭包里取出已采用的客户合同版本按它读控制声明（要不要），再取出
// 已采用的接受前财务控制策略版本按它点读正文（要执行哪些控制项）。
//
// 为什么要经闭包绕这一道：SA 手里的键是资金维（法人/账户/币种），PC 的声明按客户合同版本
// 键入、正文按策略版本键入，都与资金维不同维。由本适配器从作用域反查合同，就是在这里发明
// 账户映射的第二处定义——而账户映射本身是实例半边，本仓迄今没有那份数据。回指是 ADR-0027 /
// ADR-0062 钦定的「消费方只回指标识」形状，闭包已经由提供方固定过，取回来的合同与策略就是
// 当初解析采用的那两份。
//
// 三格的分派（ADR-0054；`要求`那一格的内容自 ADR-0122 起换了来源）：
//
//   - 声明未登记 / 闭包未采用控制策略 / 策略正文未登记 / 正文在本费用范围下无控制项 →
//     found=false。都是 `PAR-COM-15` 这一格：恢复动作是租户去补合同声明、解析键或策略正文，
//     不是重试。
//   - 声明在场且正文在场 → found=true，`不适用`带上合同给的依据；`要求`带上正文里本费用范围
//     的控制项与共同通过条件、采用的控制策略版本，以及闭包里已采用结算政策的方式与引用
//     （后两者按 CONTEXT 要保存在冻结与暴露上，不再决定走哪条控制路——从结算方式推控制方式
//     正是 pn-02-w03 禁的那条推导）。
//   - 回指译不动、闭包读不回或查无、闭包不是唯一已解析、正文读不回或坏（有父无子）→ error。
//     这些都不是「没人登记过」：答成 found=false 会把租户支去补一份其实已经存在的声明，而
//     真正坏的是那条回指或那份正文。
type PreAcceptanceControlPolicy struct {
	closures     pcports.CommercialResolutionView
	declarations pcports.PreAcceptanceControlDeclarationView
	contents     pcports.PreAcceptanceFinancialControlPolicyContentView
}

// NewPreAcceptanceControlPolicy 装配三个只读半边。三者都不许为 nil：本适配器一旦被装上，
// 就是要真去问商业侧的；缺一半而静默答「未配置」会让一次装配疏漏与租户没登记长得一样。
func NewPreAcceptanceControlPolicy(
	closures pcports.CommercialResolutionView,
	declarations pcports.PreAcceptanceControlDeclarationView,
	contents pcports.PreAcceptanceFinancialControlPolicyContentView,
) (*PreAcceptanceControlPolicy, error) {
	if closures == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: commercial resolution view is nil")
	}
	if declarations == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: control declaration view is nil")
	}
	if contents == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: control policy content view is nil")
	}
	return &PreAcceptanceControlPolicy{closures: closures, declarations: declarations, contents: contents}, nil
}

var _ saports.PreAcceptanceControlPolicyView = (*PreAcceptanceControlPolicy)(nil)

// LoadControlPolicy 分三段：回指换闭包 → 闭包取合同 → 合同读声明。
//
// scope 收下但不参与提问：它是资金维，商业侧的声明不按它键入。留在签名上是因为端口的其余
// 实现（含未配置桩）按它作答，且它日后可能参与一致性核对；此处若拿它去过滤，就成了「按租户
// 过滤一张没有租户的表」的同型错误——用一个装不下答案的键去找答案，恰好答对时也不是因为
// 它对。
func (adapter *PreAcceptanceControlPolicy) LoadControlPolicy(
	ctx context.Context,
	tenant sadomain.TenantID,
	_ sadomain.SettlementScope,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false,
			fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false,
			fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}

	closure, loaded, err := adapter.closures.LoadResolution(ctx, commercialTenant, resolutionID)
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		// 查无是坏回指，不是「未登记」：调用方刚刚才由这次解析派生出作用域，闭包却不在，
		// 说明它给的标识对不上任何一次已固定的解析。
		return sadomain.PreAcceptanceControlPolicy{}, false,
			fmt.Errorf("%w: commercial closure %q was not found", ErrUntranslatableAnswer, resolutionID)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		// 已固定的闭包必然是唯一已解析（全有或全无）。走到这里是提供方阶段契约被打破，
		// 不能折成「未登记」——那会把一份坏闭包伪装成租户还没登记。
		return sadomain.PreAcceptanceControlPolicy{}, false,
			fmt.Errorf("%w: fixed closure is %q, not uniquely resolved",
				ErrUntranslatableAnswer, closure.Outcome())
	}

	contract, adopted := closure.AdoptedFor(pcdomain.CustomerContractObject)
	if !adopted {
		// 「这个范围要不要接受前财务控制」这一问以合同为前提。闭包里没有合同依据时答
		// found=false，会把租户支去给一份不在这次解析里的合同补声明。
		return sadomain.PreAcceptanceControlPolicy{}, false,
			fmt.Errorf("%w: closure %q adopted no customer contract to ask about",
				ErrUntranslatableAnswer, resolutionID)
	}

	declaration, declared, err := adapter.declarations.LoadPreAcceptanceControl(
		ctx, commercialTenant, contract.Version())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf("load control declaration: %w", err)
	}
	if !declared {
		// 这一格是 ADR-0054 说的「未登记」：合同在、声明没写。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}

	switch declaration.Requirement() {
	case pcdomain.PreAcceptanceControlNotApplicable:
		return noControlPolicyFrom(declaration)
	case pcdomain.PreAcceptanceControlRequired:
		return adapter.requiredPolicyFrom(ctx, commercialTenant, closure)
	default:
		// 未声明的声明进不了 DeclarePreAcceptanceControl 的构造期，取到它只能是坏数据。
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: control requirement %d", ErrUntranslatableAnswer, uint8(declaration.Requirement()))
	}
}

// noControlPolicyFrom 把合同的`不适用`声明译成 SA 的无控制答复，依据必须一路带过来。
func noControlPolicyFrom(
	declaration pcdomain.PreAcceptanceControlDeclaration,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	basis, present := declaration.NotApplicableBasis()
	if !present {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: a not-applicable declaration carries no basis", ErrUntranslatableAnswer)
	}
	reference, err := sadomain.NewControlBasisReference(basis.String())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: control basis reference: %v", ErrUntranslatableAnswer, err)
	}
	policy, err := sadomain.NewNoControlPolicy(reference)
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: no-control policy: %v", ErrUntranslatableAnswer, err)
	}
	return policy, true, nil
}

// requiredPolicyFrom 把合同的`要求`声明合成 SA 的控制策略答复：控制项来自闭包里已采用的接受前
// 财务控制策略版本的正文（ADR-0115 / ADR-0122），只取本次委托费用范围下的那几项；结算方式与
// 采用的结算政策仍取自同一份闭包（CONTEXT 要冻结与暴露保存它们），但不再据它推控制方式——
// 从账期倒推出无需信用校验正是 `pn-02-w03` 明禁的那条推导，本适配器过去恰恰在做它。
//
// 费用范围从已采用结算政策的适用范围上取，不另收一格：资金作用域本就是从这份结算政策派生
// 的，它适用的费用范围就是这次解析对上的那一个；再让调用方送一格进来，两处就可以不一致。
func (adapter *PreAcceptanceControlPolicy) requiredPolicyFrom(
	ctx context.Context,
	tenant pcdomain.TenantID,
	closure pcdomain.CommercialClosure,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	settlement, adopted := adoptedSettlementPolicy(closure)
	if !adopted {
		// 合同说要控制，闭包却没解析出结算政策：结果上保存不下方式与政策，也取不到费用范围。
		// 答 found=false 而不是 error，是因为恢复方向对得上——这仍是「等商业侧把件补齐」，
		// 重试无用。生产路径上走不到这里（资金作用域正是从这份结算政策派生的），但本适配器
		// 不能假定自己只有那一个调用方。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}
	controlBasis, adopted := closure.AdoptedFor(pcdomain.PreAcceptanceFinancialControlPolicyObject)
	if !adopted {
		// 闭包没采用接受前财务控制策略：要么租户登记的解析键没要求这一项，要么合同没绑。都是
		// 租户配置未齐的那一格（ADR-0054 的`未配置`），不是本适配器能替它选一份的事。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}

	content, registered, err := adapter.contents.LoadPreAcceptanceFinancialControlPolicy(
		ctx, tenant, controlBasis.Version())
	if err != nil {
		// 有父无子在提供方那口已经是 error（ADR-0115 决定五），这里不再折成未登记。
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf("load control policy content: %w", err)
	}
	if !registered {
		// 版本壳发布了、正文没登记：`PAR-COM-15` 的控制项半边还没到，恢复动作是补正文。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}
	scoped := content.ItemsFor(settlement.Applicability().ChargeScope())
	if len(scoped) == 0 {
		// 正文有、但对这个费用范围一项都没说：合同绑定把范围指到了这份策略，策略却没为它写
		// 控制项——仍是租户要补的配置，不是「无控制」（那要合同带依据声明，ADR-0115 决定一）。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}

	items := make([]sadomain.ControlItem, 0, len(scoped))
	for _, scopedItem := range scoped {
		kind, err := controlKind(scopedItem.Kind())
		if err != nil {
			return sadomain.PreAcceptanceControlPolicy{}, false, err
		}
		if scopedItem.EvaluationOrder() <= 0 {
			return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
				"%w: control item order %d", ErrUntranslatableAnswer, scopedItem.EvaluationOrder())
		}
		item, err := sadomain.NewControlItem(kind, uint32(scopedItem.EvaluationOrder()))
		if err != nil {
			return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
				"%w: control item: %v", ErrUntranslatableAnswer, err)
		}
		items = append(items, item)
	}
	jointPass, err := jointPassCondition(content.JointPassCondition())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, err
	}
	controlPolicy, err := sadomain.NewControlPolicyReference(versionReference(content.Version()))
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: control policy reference: %v", ErrUntranslatableAnswer, err)
	}
	method, err := settlementMethod(settlement.Method())
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, err
	}
	settlementPolicy, err := sadomain.NewAdoptedPolicyReference(versionReference(settlement.Version()))
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: adopted policy reference: %v", ErrUntranslatableAnswer, err)
	}
	policy, err := sadomain.NewRequiredControlPolicy(items, jointPass, controlPolicy, method, settlementPolicy)
	if err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: required control policy: %v", ErrUntranslatableAnswer, err)
	}
	return policy, true, nil
}

// versionReference 是两侧共用的版本引用写法「对象/版本」，结算政策与控制策略同一形。
func versionReference(version pcdomain.CommercialVersion) string {
	return version.ObjectID().String() + "/" + version.Version().String()
}

// controlKind 是两侧封闭集之间的全函数。default 报错不吸收（ADR-0025）：提供方放宽 CHECK 加了
// 第三种控制而这里没接分支时必须炸出来，静默跳过等于把一项租户明确要求的控制当没有。
func controlKind(kind pcdomain.PreAcceptanceControlKind) (sadomain.ControlKind, error) {
	switch kind {
	case pcdomain.PrepaidFreezeControl:
		return sadomain.PrepaidFreezeControl, nil
	case pcdomain.CreditCheckControl:
		return sadomain.CreditCheckControl, nil
	default:
		return sadomain.ControlKindInvalid, fmt.Errorf(
			"%w: control kind %d", ErrUntranslatableAnswer, uint8(kind))
	}
}

// jointPassCondition 同上。首发两侧都只有「全部通过」一值；提供方放宽出第二种组合子时这里先炸，
// 而不是让一个本上下文还不会算的组合子静默落成全部通过（ADR-0115 决定三点名的那条红线）。
func jointPassCondition(condition pcdomain.JointPassCondition) (sadomain.JointPassCondition, error) {
	switch condition {
	case pcdomain.AllControlsPass:
		return sadomain.AllControlsPass, nil
	default:
		return sadomain.JointPassConditionInvalid, fmt.Errorf(
			"%w: joint pass condition %d", ErrUntranslatableAnswer, uint8(condition))
	}
}

// adoptedSettlementPolicy 取闭包里已采用的结算政策。采用了结算依据对象却不带政策正文是
// 提供方阶段契约被打破，但那一格由调用链上更早的 PS 适配器承重（它同样读这份闭包）；
// 这里与「压根没有结算依据」同归缺席，避免两处对同一份坏数据各报一次不同的话。
func adoptedSettlementPolicy(closure pcdomain.CommercialClosure) (pcdomain.SettlementPolicy, bool) {
	basis, adopted := closure.AdoptedFor(pcdomain.SettlementPolicyObject)
	if !adopted {
		return pcdomain.SettlementPolicy{}, false
	}
	return basis.SettlementPolicy()
}

// settlementMethod 是两侧封闭二值之间的全函数。default 报错不吸收（ADR-0025）：新增取值
// 没接分支时必须炸出来，静默落成预付会让一个约定了账期的客户被冻资金。
func settlementMethod(method pcdomain.SettlementMethod) (sadomain.SettlementMethod, error) {
	switch method {
	case pcdomain.PrepaidMethod:
		return sadomain.PrepaidSettlement, nil
	case pcdomain.TermsMethod:
		return sadomain.TermsSettlement, nil
	default:
		return sadomain.SettlementMethodInvalid, fmt.Errorf(
			"%w: settlement method %d", ErrUntranslatableAnswer, uint8(method))
	}
}
