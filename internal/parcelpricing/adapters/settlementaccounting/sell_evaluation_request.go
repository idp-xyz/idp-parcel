package settlementaccounting

import (
	"context"
	"fmt"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellEvaluationRequestSource 是 SA SELL 评价请求册的只读半边在本适配器眼里的读口（ADR-0190 决定一）。SA 的 postgres
// `SellEvaluationRequests` 直接满足它。只声明按标识读那一口：计价只按信封所指那一份回读，拿不到 Save，也用不着
// 按包裹读。
type SellEvaluationRequestSource interface {
	FindByID(ctx context.Context, key saports.SellEvaluationRequestKey) (saports.SellEvaluationRequestRecord, bool, error)
}

// TranslateSellEvaluationRequest 把 SA 的一份 SELL 评价请求记录译成本上下文的形成命令（ADR-0190 决定四）。
//
//   - 租户与回指取登记册的键：回指是 SA 铸造的 SELL 请求标识的字面。评价上的回指不分 BUY、SELL——两种请求标识都是
//     铸造的，同租户下撞不上；评价自己带方向与目的，读回指的一方据此知道该去哪本册找。
//   - 方向与目的固定译成 SELL + CUSTOMER_CHARGE：SELL 请求身份上没有目的一格，它只为预估客户费用而立。选卡因此与
//     受理前试算同一口径（ADR-0179 决定三）。
//   - 计价范围按字面译成 PricingScopeID：SA 那一格本就回指计价的范围（受理前估价同一口径）。
//   - 计价基准时点取请求上那一格：内置的两种包裹计费时点各自规定了它（ADR-0190 决定二），这里不重算；RecordedAt 是
//     请求何时登记，不是被计价的那一刻。
//   - 来源引用只带包裹：快照按包裹建。来源事实引用（收寄采用键或终局键）是 SA 留痕的那一格，计价不拿它键任何事实，
//     不带。
//   - 结算币种取请求上快照的那一格，译不成计价的币种就拒——不拿卡币种顶，那会让预估客户费用落在另一个币种上。
//   - 证据层级由装配方声明、不给默认（BUY 那一路同一取舍）。
func TranslateSellEvaluationRequest(
	record saports.SellEvaluationRequestRecord,
	evidence ppdomain.EvidenceKind,
) (ppapplication.FormEvaluationFromRequestCommand, error) {
	none := ppapplication.FormEvaluationFromRequestCommand{}
	if !evidence.Declared() {
		return none, fmt.Errorf("%w: evidence kind %q is not one of S / R / P", ErrUntranslatableAnswer, evidence)
	}
	tenant, err := ppdomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	reference, err := ppdomain.NewEvaluationRequestReference(record.Key.Request.String())
	if err != nil {
		return none, fmt.Errorf("%w: request reference: %v", ErrUntranslatableAnswer, err)
	}
	request := record.Request
	scope, err := ppdomain.NewPricingScopeID(request.PricingScope().String())
	if err != nil {
		return none, fmt.Errorf("%w: pricing scope: %v", ErrUntranslatableAnswer, err)
	}
	settlement, err := ppdomain.NewCurrency(request.SettlementCurrency().String())
	if err != nil {
		return none, fmt.Errorf("%w: settlement currency %q: %v", ErrUntranslatableAnswer, request.SettlementCurrency().String(), err)
	}
	return ppapplication.FormEvaluationFromRequestCommand{
		Tenant:     tenant,
		Request:    reference,
		Scope:      scope,
		Direction:  ppdomain.PricingDirectionSell,
		Purpose:    ppdomain.PricingPurposeCustomerCharge,
		BasisAt:    request.PricingBasisAt(),
		Sources:    ppports.EligibleSourceReferences{Parcel: request.Parcel().String()},
		Settlement: settlement,
		Evidence:   evidence,
	}, nil
}
