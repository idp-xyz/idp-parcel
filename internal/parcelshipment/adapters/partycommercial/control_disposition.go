package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ControlDispositionAdapter 实现 psports.ControlDispositionView：凭本轮已采用的商业解析取已固定的闭包，
// 从闭包里取出已采用的结算政策（它的适用范围就是本次委托的费用范围）与已采用的接受前财务控制策略版本，
// 按后者点读正文，只取本费用范围下的行，逐种类译成本上下文的采用引用（ADR-0132 决定三）。
//
// 它走的路与 settlement-accounting 执行控制时读正文的那只适配器同源——同一份闭包、同一版正文、同一个
// 费用范围——所以受限项的种类能按行对上：SA 执行的是这几行，PS 读到的处置也是这几行。不在这里问合同的
// 控制声明：走到这里的前提是 SA 已经交回了含受限项的结果，合同要不要控制早由那一步答过。
//
// 三格的分派（形照 SA 那只适配器）：
//
//   - 闭包没采用结算政策或控制策略、正文未登记、本费用范围下一项都没写 → found=false。都是租户要补的
//     配置，恢复动作不是重试。
//   - 正文在场且范围下有行 → found=true，逐种类交回处置 × 责任引用。范围下缺哪一种类由领域按受限项判。
//   - 回指译不动、闭包读不回或查无、闭包不是唯一已解析、正文读不回或坏、词汇集外 → error。这些都不是
//     「没人登记过」：答成 found=false 会把租户支去补一份其实已经存在的正文，而真正坏的是那条回指。
type ControlDispositionAdapter struct {
	closures pcports.CommercialResolutionView
	contents pcports.PreAcceptanceFinancialControlPolicyContentView
}

// NewControlDispositionAdapter 装配两个只读半边。两者都不许为 nil：本适配器一旦被装上，就是要真去问商业侧
// 的；缺一半而静默答「未配置」会让一次装配疏漏与租户没登记长得一样。
func NewControlDispositionAdapter(
	closures pcports.CommercialResolutionView,
	contents pcports.PreAcceptanceFinancialControlPolicyContentView,
) (*ControlDispositionAdapter, error) {
	if closures == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: commercial resolution view is nil")
	}
	if contents == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: control policy content view is nil")
	}
	return &ControlDispositionAdapter{closures: closures, contents: contents}, nil
}

var _ psports.ControlDispositionView = (*ControlDispositionAdapter)(nil)

// LoadControlDispositions 分三段：回指换闭包 → 闭包取结算政策与控制策略版本 → 版本读正文按范围取行。
func (adapter *ControlDispositionAdapter) LoadControlDispositions(
	ctx context.Context,
	query psports.ControlDispositionQuery,
) (map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition, bool, error) {
	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return nil, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(query.Resolution.String())
	if err != nil {
		return nil, false, fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}

	closure, loaded, err := adapter.closures.LoadResolution(ctx, tenant, resolutionID)
	if err != nil {
		return nil, false, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		// 查无是坏回指，不是「未登记」：这次解析是本轮刚采用并记下的，闭包却不在，说明回指对不上任何
		// 一次已固定的解析。
		return nil, false, fmt.Errorf("%w: commercial closure %q was not found", ErrUntranslatableAnswer, resolutionID)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		// 已固定的闭包必然是唯一已解析。走到这里是提供方阶段契约被打破，不能折成「未登记」。
		return nil, false, fmt.Errorf("%w: fixed closure is %q, not uniquely resolved",
			ErrUntranslatableAnswer, closure.Outcome())
	}

	settlementBasis, adopted := closure.AdoptedFor(pcdomain.SettlementPolicyObject)
	if !adopted {
		return nil, false, nil
	}
	settlement, present := settlementBasis.SettlementPolicy()
	if !present {
		// 采用了结算依据对象却不带政策正文：与「压根没有结算依据」同归缺席，理由同 SA 那只适配器——
		// 那一格由调用链上更早读这份闭包的 PS 适配器承重，两处对同一份坏数据各报一次不同的话没有意义。
		return nil, false, nil
	}
	controlBasis, adopted := closure.AdoptedFor(pcdomain.PreAcceptanceFinancialControlPolicyObject)
	if !adopted {
		return nil, false, nil
	}

	content, registered, err := adapter.contents.LoadPreAcceptanceFinancialControlPolicy(ctx, tenant, controlBasis.Version())
	if err != nil {
		// 有父无子在提供方那口已经是 error（ADR-0115 决定五），这里不再折成未登记。
		return nil, false, fmt.Errorf("load control policy content: %w", err)
	}
	if !registered {
		return nil, false, nil
	}
	scoped := content.ItemsFor(settlement.Applicability().ChargeScope())
	if len(scoped) == 0 {
		return nil, false, nil
	}

	dispositions := make(map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition, len(scoped))
	for _, item := range scoped {
		kind, err := controlItemKind(item.Kind())
		if err != nil {
			return nil, false, err
		}
		disposition, err := controlFailureDisposition(item.FailureDisposition())
		if err != nil {
			return nil, false, err
		}
		responsibility, err := psdomain.NewControlResponsibilityReference(item.Responsibility().String())
		if err != nil {
			return nil, false, fmt.Errorf("%w: control responsibility reference: %v", ErrUntranslatableAnswer, err)
		}
		adoptedDisposition, err := psdomain.NewAdoptedControlDisposition(disposition, responsibility)
		if err != nil {
			return nil, false, fmt.Errorf("%w: adopted control disposition: %v", ErrUntranslatableAnswer, err)
		}
		if _, duplicated := dispositions[kind]; duplicated {
			// 正文那一侧同一版本同一范围下一种控制至多一行（ADR-0115 决定二的行级约束）；读到两行是坏数据，
			// 挑任一行都是替租户选去向。
			return nil, false, fmt.Errorf("%w: control kind %s appears twice on the charge scope",
				ErrUntranslatableAnswer, kind)
		}
		dispositions[kind] = adoptedDisposition
	}
	return dispositions, true, nil
}

// controlItemKind 是两侧封闭集之间的全函数。default 报错不吸收（ADR-0025）：提供方放宽 CHECK 加了第三种控制
// 而这里没接分支时必须炸出来，静默跳过等于让那一种控制的受限项永远对不上处置。
func controlItemKind(kind pcdomain.PreAcceptanceControlKind) (psdomain.ControlItemKind, error) {
	switch kind {
	case pcdomain.PrepaidFreezeControl:
		return psdomain.PrepaidFreezeControlItem, nil
	case pcdomain.CreditCheckControl:
		return psdomain.CreditCheckControlItem, nil
	default:
		return psdomain.ControlItemKindInvalid, fmt.Errorf(
			"%w: control kind %d", ErrUntranslatableAnswer, uint8(kind))
	}
}

// controlFailureDisposition 同上。两侧今天都是两值；提供方放宽出第三种去向时这里先炸，而不是让一个本上下文
// 还不认识的去向静默落成 REJECT 或 AUTHORIZED_DISPOSITION——那正是替租户决定委托的去向。
func controlFailureDisposition(
	disposition pcdomain.ControlFailureDisposition,
) (psdomain.ControlFailureDisposition, error) {
	switch disposition {
	case pcdomain.RejectOnControlFailure:
		return psdomain.RejectOnControlFailure, nil
	case pcdomain.AuthorizedDispositionOnControlFailure:
		return psdomain.AuthorizedDispositionOnControlFailure, nil
	default:
		return psdomain.ControlFailureDispositionInvalid, fmt.Errorf(
			"%w: control failure disposition %d", ErrUntranslatableAnswer, uint8(disposition))
	}
}
