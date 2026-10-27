// Package settlementaccounting 是 collection-remittance 读 settlement-accounting 的消费侧适配器
// （`internal/<consumer>/adapters/<provider>/`，与 customs-compliance 读它同一位置纪律）：跨上下文翻译只落在这里，
// `application` 不 import 提供方。本包只读提供方的只读口，不写它的库。
package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	crdomain "go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	crports "go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatable 表示两侧词汇表对不上：提供方交回的事实在本上下文构造不出（金额非正、币种或版本为空）。那是读口
// 坏了或两侧词汇分歧，不是业务答案，所以是 error（未决），不折成 found=false。
var ErrUntranslatable = errors.New("collection remittance settlementaccounting adapter: adopted funds fact does not translate")

// AdoptedFundsView 实现 crports.AdoptedFundsView：把本上下文的（租户、已采用事实引用）译成提供方的键，问结算的只读口，
// 再把链头译成本上下文要读的两维——版本与带币种的金额。
//
// 只译不判：结算答「没有这条事实」或「有，但没被整条认作代收本金」，对本上下文是同一件事（先去结算采用并认作），
// 都译成 found=false。认作与否由结算在它的事实锁下判，本上下文不另存一份。
type AdoptedFundsView struct {
	view saports.CollectedPrincipalFundsView
}

func NewAdoptedFundsView(view saports.CollectedPrincipalFundsView) (*AdoptedFundsView, error) {
	if view == nil {
		return nil, fmt.Errorf("collection remittance settlementaccounting adapter: collected principal funds view is nil")
	}
	return &AdoptedFundsView{view: view}, nil
}

var _ crports.AdoptedFundsView = (*AdoptedFundsView)(nil)

// LockAdoptedFunds 先对提供方的事实身份行取共享锁再读（提供方的口约定），交回链头与认作状态的翻译。
func (adapter *AdoptedFundsView) LockAdoptedFunds(
	ctx context.Context,
	tenant crdomain.TenantID,
	reference crdomain.AdoptedFundsReference,
) (crdomain.AdoptedFunds, bool, error) {
	saTenant, err := sadomain.NewTenantID(tenant.String())
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatable, err)
	}
	saFact, err := sadomain.NewFundsFactReference(reference.String())
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: fact: %v", ErrUntranslatable, err)
	}

	loaded, found, err := adapter.view.LockCollectedPrincipalFact(ctx, saTenant, saFact)
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("lock collected principal fact: %w", err)
	}
	if !found || !loaded.SetAside {
		return crdomain.AdoptedFunds{}, false, nil
	}

	currency, amountMinor := loaded.Head.Amount()
	code, err := crdomain.NewCurrencyCode(currency.String())
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: currency: %v", ErrUntranslatable, err)
	}
	amount, err := crdomain.NewMoney(code, amountMinor)
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: amount: %v", ErrUntranslatable, err)
	}
	version, err := crdomain.NewAdoptedFundsVersion(loaded.Head.Version().String())
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: version: %v", ErrUntranslatable, err)
	}
	funds, err := crdomain.NewAdoptedFunds(version, amount)
	if err != nil {
		return crdomain.AdoptedFunds{}, false, fmt.Errorf("%w: %v", ErrUntranslatable, err)
	}
	return funds, true, nil
}
