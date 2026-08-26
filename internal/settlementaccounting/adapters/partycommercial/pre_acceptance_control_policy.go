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
// 商业解析取已固定的闭包，从闭包里取出已采用的客户合同版本，再按那份合同读控制声明。
//
// 为什么要经闭包绕这一道：SA 手里的键是资金维（法人/账户/币种），PC 的声明按客户合同版本
// 键入，两者不同维。由本适配器从作用域反查合同，就是在这里发明账户映射的第二处定义——
// 而账户映射本身是实例半边，本仓迄今没有那份数据。回指是 ADR-0027 / ADR-0062 钦定的
// 「消费方只回指标识」形状，闭包已经由提供方固定过，取回来的合同就是当初解析采用的那一份。
//
// 三格的分派（ADR-0054）：
//
//   - 声明未登记 → found=false。那是 `PAR-COM-15` 这一格，恢复动作是租户去补合同正文。
//   - 声明在场 → found=true，`不适用`带上合同给的依据，`要求`带上闭包里已采用结算政策的
//     方式与引用（ADR-0044 的分工：声明只答「要不要」，方式与政策由结算政策解析回答）。
//   - 回指译不动、闭包读不回或查无、闭包不是唯一已解析 → error。这些都不是「没人登记过」：
//     答成 found=false 会把租户支去补一份其实已经存在的声明，而真正坏的是那条回指。
type PreAcceptanceControlPolicy struct {
	closures     pcports.CommercialResolutionView
	declarations pcports.PreAcceptanceControlDeclarationView
}

// NewPreAcceptanceControlPolicy 装配两个只读半边。两者都不许为 nil：本适配器一旦被装上，
// 就是要真去问商业侧的；缺一半而静默答「未配置」会让一次装配疏漏与租户没登记长得一样。
func NewPreAcceptanceControlPolicy(
	closures pcports.CommercialResolutionView,
	declarations pcports.PreAcceptanceControlDeclarationView,
) (*PreAcceptanceControlPolicy, error) {
	if closures == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: commercial resolution view is nil")
	}
	if declarations == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: control declaration view is nil")
	}
	return &PreAcceptanceControlPolicy{closures: closures, declarations: declarations}, nil
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
		// 这一格、也只有这一格，是 ADR-0054 说的「未登记」：合同在、声明没写。
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}
	return policyFrom(declaration, closure)
}

// policyFrom 把合同声明与闭包里的结算政策合成 SA 的控制策略答复。声明只答「要不要」，
// 方式与采用政策一概取自闭包（ADR-0044 / ADR-0054 后果节的分工）——由本适配器从声明反推
// 方式，就会从账期倒推出无需信用校验，那是 `pn-02-w03` 明禁的那条推导。
func policyFrom(
	declaration pcdomain.PreAcceptanceControlDeclaration,
	closure pcdomain.CommercialClosure,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	switch declaration.Requirement() {
	case pcdomain.PreAcceptanceControlNotApplicable:
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

	case pcdomain.PreAcceptanceControlRequired:
		settlement, adopted := adoptedSettlementPolicy(closure)
		if !adopted {
			// 合同说要控制，闭包却没解析出结算政策：说不出按哪种方式控制。答 found=false
			// 而不是 error，是因为恢复方向对得上——这仍是「等商业侧把件补齐」，重试无用。
			// 生产路径上走不到这里（资金作用域正是从这份结算政策派生的），但本适配器不能
			// 假定自己只有那一个调用方。
			return sadomain.PreAcceptanceControlPolicy{}, false, nil
		}
		method, err := settlementMethod(settlement.Method())
		if err != nil {
			return sadomain.PreAcceptanceControlPolicy{}, false, err
		}
		reference, err := sadomain.NewAdoptedPolicyReference(
			settlement.Version().ObjectID().String() + "/" + settlement.Version().Version().String())
		if err != nil {
			return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
				"%w: adopted policy reference: %v", ErrUntranslatableAnswer, err)
		}
		policy, err := sadomain.NewRequiredControlPolicy(method, reference)
		if err != nil {
			return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
				"%w: required control policy: %v", ErrUntranslatableAnswer, err)
		}
		return policy, true, nil

	default:
		// 未声明的声明进不了 DeclarePreAcceptanceControl 的构造期，取到它只能是坏数据。
		return sadomain.PreAcceptanceControlPolicy{}, false, fmt.Errorf(
			"%w: control requirement %d", ErrUntranslatableAnswer, uint8(declaration.Requirement()))
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
