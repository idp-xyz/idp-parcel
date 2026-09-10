// Package settlementaccounting 是 customs-compliance 读 settlement-accounting 的消费侧适配器
// （`internal/<consumer>/adapters/<provider>/`，与 PS 读 PC 同一位置纪律）：跨上下文翻译只落在这里，
// `application` 不 import 提供方（票 sa-cc/03 做法 2）。本包只读提供方的只读口，不写它的库。
package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatableReference 表示信封带来的引用在提供方词汇里构造不出来（空白租户 / 事实 / 版本）。
// 那是引用坏了，不是等谁——但消费者译码已经拒过三维缺席，这里只可能是两侧词汇表出了分歧。
var ErrUntranslatableReference = errors.New(
	"customs compliance settlementaccounting adapter: funds fact reference does not translate")

// SettlementAdoptedFundsFactSource 实现 ccports.AdoptedFundsFactSource：把本上下文的（租户、事实引用、
// 版本）译成提供方的键，问 SA 的只读视图，再把事实本体译成本上下文最少要读的几维。
//
// 只译不判：付款人在提供方显式缺席就译成空——要不要收是 ReceiveFundsFact 的事；金额与币种只是
// 转述给入向登记，权威留在提供方（票面红线）。
type SettlementAdoptedFundsFactSource struct {
	view saports.AdoptedFundsFactView
}

func NewSettlementAdoptedFundsFactSource(view saports.AdoptedFundsFactView) (*SettlementAdoptedFundsFactSource, error) {
	if view == nil {
		return nil, fmt.Errorf("customs compliance settlementaccounting adapter: adopted funds fact view is nil")
	}
	return &SettlementAdoptedFundsFactSource{view: view}, nil
}

var _ ccports.AdoptedFundsFactSource = (*SettlementAdoptedFundsFactSource)(nil)

// LoadAdoptedFundsFact 取信封所指的那一版。提供方答「没有」原样交回 false——那一版还没落或存的是
// 另一版，两者都不该拿当前版顶替（票 lc/24 的教训）。
func (source *SettlementAdoptedFundsFactSource) LoadAdoptedFundsFact(
	ctx context.Context,
	tenant ccdomain.TenantID,
	fact ccdomain.ExternalFundsFactReference,
	version string,
) (ccports.AdoptedFundsFact, bool, error) {
	saTenant, err := sadomain.NewTenantID(tenant.String())
	if err != nil {
		return ccports.AdoptedFundsFact{}, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	saFact, err := sadomain.NewFundsFactReference(fact.String())
	if err != nil {
		return ccports.AdoptedFundsFact{}, false, fmt.Errorf("%w: fact: %v", ErrUntranslatableReference, err)
	}
	saVersion, err := sadomain.NewFundsFactVersion(version)
	if err != nil {
		return ccports.AdoptedFundsFact{}, false, fmt.Errorf("%w: version: %v", ErrUntranslatableReference, err)
	}

	adopted, found, err := source.view.LoadAdoptedFundsFact(ctx, saTenant, saFact, saVersion)
	if err != nil {
		return ccports.AdoptedFundsFact{}, false, fmt.Errorf("load adopted funds fact: %w", err)
	}
	if !found {
		return ccports.AdoptedFundsFact{}, false, nil
	}

	currency, amount := adopted.Amount()
	translated := ccports.AdoptedFundsFact{
		Source:      adopted.Source().String(),
		Currency:    currency.String(),
		AmountMinor: amount,
		OccurredAt:  adopted.OccurredAt(),
	}
	if payer, provided := adopted.Payer(); provided {
		translated.Payer = payer.String()
	}
	return translated, true, nil
}
