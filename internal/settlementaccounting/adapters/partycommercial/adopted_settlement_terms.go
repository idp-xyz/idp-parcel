package partycommercial

import (
	"context"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AdoptedSettlementTerms 实现 saports.AdoptedSettlementTermsView：凭回指取已固定的闭包，从闭包里取出采用的结算政策，
// 把它落在结算账户键上的四维译过来。它是「包裹 → 结算账户」那条链的中段（ADR-0190 决定三），与 CreditBasis、
// PreAcceptanceControlPolicy 并列——同一份回指、同一份闭包、各取各的成员，三格含义各不相同，不合成一只。
//
// 法人、相对方、币种取政策的适用范围，政策引用取政策对象标识不带版本：账户册的键就是这么登的（SettlementPolicyReference
// 头注），受理前财务控制在小包托运那一侧拼账户键时取的也是这四样（那一侧的 RegisteredAccountDirectory）。两边取法不同，
// 同一件包裹在受理前冻结与计费时就会落到两个账户上。
//
// 三格：
//
//   - 闭包没采用结算政策 → found=false：租户登记的解析键没要这一项，等租户补，不是重试的事。
//   - 采用了且带政策正文 → found=true，四维原样。
//   - 回指译不动、闭包读不回或查无、闭包不是唯一已解析、采用了结算政策对象却没带正文、某一维译不动 → error。都不是
//     「没人登记过」，理由与 CreditBasis 头注同一条。
type AdoptedSettlementTerms struct {
	closures pcports.CommercialResolutionView
}

// NewAdoptedSettlementTerms 装配闭包读口。不许为 nil：缺读口而静默答「没采用」会让一次装配疏漏与租户没登记长得一样。
func NewAdoptedSettlementTerms(closures pcports.CommercialResolutionView) (*AdoptedSettlementTerms, error) {
	if closures == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: commercial resolution view is nil")
	}
	return &AdoptedSettlementTerms{closures: closures}, nil
}

var _ saports.AdoptedSettlementTermsView = (*AdoptedSettlementTerms)(nil)

func (adapter *AdoptedSettlementTerms) LoadAdoptedSettlementTerms(
	ctx context.Context,
	tenant sadomain.TenantID,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.AdoptedSettlementTerms, bool, error) {
	none := sadomain.AdoptedSettlementTerms{}
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}

	closure, loaded, err := adapter.closures.LoadResolution(ctx, commercialTenant, resolutionID)
	if err != nil {
		return none, false, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		return none, false, fmt.Errorf("%w: commercial closure %q was not found", ErrUntranslatableAnswer, resolutionID)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		return none, false, fmt.Errorf("%w: fixed closure is %q, not uniquely resolved", ErrUntranslatableAnswer, closure.Outcome())
	}

	basis, adopted := closure.AdoptedFor(pcdomain.SettlementPolicyObject)
	if !adopted {
		return none, false, nil
	}
	policy, carried := basis.SettlementPolicy()
	if !carried {
		// 采用了结算政策对象却没带正文：提供方阶段契约被打破（成功路径必经政策采用），不是一种缺席。
		return none, false, fmt.Errorf("%w: closure %q adopted a settlement policy without its content",
			ErrUntranslatableAnswer, resolutionID)
	}
	terms, err := settlementTermsFrom(policy)
	if err != nil {
		return none, false, err
	}
	return terms, true, nil
}

func settlementTermsFrom(policy pcdomain.SettlementPolicy) (sadomain.AdoptedSettlementTerms, error) {
	applicability := policy.Applicability()
	legalEntity, err := sadomain.NewLegalEntityReference(applicability.LegalEntity().String())
	if err != nil {
		return sadomain.AdoptedSettlementTerms{}, fmt.Errorf("%w: legal entity: %v", ErrUntranslatableAnswer, err)
	}
	counterparty, err := sadomain.NewSettlementCounterpartyReference(applicability.Counterparty().String())
	if err != nil {
		return sadomain.AdoptedSettlementTerms{}, fmt.Errorf("%w: counterparty: %v", ErrUntranslatableAnswer, err)
	}
	currency, err := sadomain.NewCurrencyCode(applicability.Currency().String())
	if err != nil {
		return sadomain.AdoptedSettlementTerms{}, fmt.Errorf("%w: currency: %v", ErrUntranslatableAnswer, err)
	}
	reference, err := sadomain.NewSettlementPolicyReference(policy.Version().ObjectID().String())
	if err != nil {
		return sadomain.AdoptedSettlementTerms{}, fmt.Errorf("%w: settlement policy: %v", ErrUntranslatableAnswer, err)
	}
	terms, err := sadomain.NewAdoptedSettlementTerms(legalEntity, counterparty, currency, reference)
	if err != nil {
		return sadomain.AdoptedSettlementTerms{}, fmt.Errorf("%w: adopted settlement terms: %v", ErrUntranslatableAnswer, err)
	}
	return terms, nil
}
