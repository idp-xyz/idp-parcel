package partycommercial

import (
	"context"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AdoptedSellPriceArrangement 实现 saports.AdoptedSellPriceArrangementView：凭回指取已固定的闭包，取出它采用的
// 那一版客户合同，再按那一版点读销售价格约定（ADR-0380 决定二）。约定不抄进闭包快照——点读固定下来的版本，
// 合同之后发的新版不会改已固定解析的口；票 16 进 main 之后、本口落地之前固定的解析也不会被读成「没有约定」。
//
// 与 AdoptedSettlementTerms 并列：同一份回指、同一份闭包、各取各的成员。
//
// 三种答复都走成功路径，不拿空列表顶第三种：
//
//   - 这一版有有序来源 → 来源按声明顺序，名字照原串交、不去空白。
//   - 这一版显式不计销售价 → 带依据。
//   - 这一版没有销售价格约定（无父行）→ NotDeclared。
//
// 回指译不动、闭包读不回或查无、闭包不是唯一已解析、没采用客户合同、约定正文坏 → error。都不是三种答复之一。
type AdoptedSellPriceArrangement struct {
	closures     pcports.CommercialResolutionView
	arrangements pcports.ContractSellPriceArrangementView
}

// NewAdoptedSellPriceArrangement 装配闭包读口与约定点读口。两者都不许为 nil：缺一口而静默答「没有约定」会让
// 一次装配疏漏与旧规范化版本长得一样。
func NewAdoptedSellPriceArrangement(
	closures pcports.CommercialResolutionView,
	arrangements pcports.ContractSellPriceArrangementView,
) (*AdoptedSellPriceArrangement, error) {
	if closures == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: commercial resolution view is nil")
	}
	if arrangements == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: sell price arrangement view is nil")
	}
	return &AdoptedSellPriceArrangement{closures: closures, arrangements: arrangements}, nil
}

var _ saports.AdoptedSellPriceArrangementView = (*AdoptedSellPriceArrangement)(nil)

func (adapter *AdoptedSellPriceArrangement) LoadAdoptedSellPriceArrangement(
	ctx context.Context,
	tenant sadomain.TenantID,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.AdoptedSellPriceArrangement, error) {
	none := sadomain.AdoptedSellPriceArrangement{}
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return none, fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}

	closure, loaded, err := adapter.closures.LoadResolution(ctx, commercialTenant, resolutionID)
	if err != nil {
		return none, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		return none, fmt.Errorf("%w: commercial closure %q was not found", ErrUntranslatableAnswer, resolutionID)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		return none, fmt.Errorf("%w: fixed closure is %q, not uniquely resolved", ErrUntranslatableAnswer, closure.Outcome())
	}

	adopted, present := closure.AdoptedFor(pcdomain.CustomerContractObject)
	if !present {
		return none, fmt.Errorf("%w: closure %q adopted no customer contract", ErrUntranslatableAnswer, resolutionID)
	}

	arrangement, found, err := adapter.arrangements.LoadContractSellPriceArrangement(ctx, commercialTenant, adopted.Version())
	if err != nil {
		return none, fmt.Errorf("load contract sell price arrangement: %w", err)
	}
	if !found {
		return sadomain.NewNotDeclaredSellPriceArrangement(), nil
	}
	return sellPriceArrangementFrom(arrangement)
}

// sellPriceArrangementFrom 是两侧约定之间的全函数。来源名照原串交：计价范围引用要逐字对上，这里再规整一次，
// 两边的构造门就会各认各的（票 16 留给本票的第 3 条）。两格都不是的形状领域构造门造不出，走到这里是提供方
// 阶段契约被打破。
func sellPriceArrangementFrom(
	arrangement pcdomain.ContractSellPriceArrangement,
) (sadomain.AdoptedSellPriceArrangement, error) {
	if sources, ordered := arrangement.Sources(); ordered {
		translated := make([]sadomain.EstimatePricingScope, 0, len(sources))
		for _, source := range sources {
			scope, err := sadomain.NewEstimatePricingScope(source.String())
			if err != nil {
				return sadomain.AdoptedSellPriceArrangement{}, fmt.Errorf(
					"%w: pricing scope %q: %v", ErrUntranslatableAnswer, source.String(), err)
			}
			translated = append(translated, scope)
		}
		adopted, err := sadomain.NewOrderedSellPriceSources(translated)
		if err != nil {
			return sadomain.AdoptedSellPriceArrangement{}, fmt.Errorf("%w: %v", ErrUntranslatableAnswer, err)
		}
		return adopted, nil
	}
	if basis, notPriced := arrangement.NoSellPriceBasis(); notPriced {
		translated, err := sadomain.NewNoSellPriceBasis(basis.String())
		if err != nil {
			return sadomain.AdoptedSellPriceArrangement{}, fmt.Errorf(
				"%w: no sell price basis: %v", ErrUntranslatableAnswer, err)
		}
		adopted, err := sadomain.NewExplicitlyNotPricedSellPrice(translated)
		if err != nil {
			return sadomain.AdoptedSellPriceArrangement{}, fmt.Errorf("%w: %v", ErrUntranslatableAnswer, err)
		}
		return adopted, nil
	}
	return sadomain.AdoptedSellPriceArrangement{}, fmt.Errorf(
		"%w: sell price arrangement is neither sources nor an explicit no-sell-price basis", ErrUntranslatableAnswer)
}
