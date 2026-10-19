package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 SELL 消费门（票 sell-customer-charge/04；ADR-0169 决定一、ADR-0190 决定五）：真算的 PP 评价经真 SELL 读口与真形成
// 编排走一遍，SA 那几本册是内存替身。门只管译引用、分方向与折结果：BUY 评价与不回指 SELL 请求的评价入账、零写入；
// 能形成的形成并在请求册上记`已形成`；金额未声明记`待判断`；评价读不回、依赖答不出是唯一那枚未决哨兵；两侧说法矛盾
// 原样上抛、不是未决。夹具全为 SYN 卡（S 级）。

const gateTenant = "tenant-1"

var gateFormedAt = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

type gateClock struct{}

func (gateClock) Now() time.Time { return gateFormedAt }

// gateRequests 同时是 SELL 请求读口与改记口，按（租户/请求）存。
type gateRequests struct {
	records     map[string]ports.SellEvaluationRequestRecord
	viewErr     error
	forceEffect ports.SellEvaluationRequestConclusionEffect
}

func (requests *gateRequests) FindByID(_ context.Context, key ports.SellEvaluationRequestKey) (ports.SellEvaluationRequestRecord, bool, error) {
	if requests.viewErr != nil {
		return ports.SellEvaluationRequestRecord{}, false, requests.viewErr
	}
	record, found := requests.records[key.TenantID.String()+"/"+key.Request.String()]
	return record, found, nil
}

func (requests *gateRequests) FindCurrentByParcel(context.Context, domain.TenantID, domain.ParcelReference) (ports.SellEvaluationRequestRecord, bool, error) {
	return ports.SellEvaluationRequestRecord{}, false, errors.New("not used by the gate")
}

func (requests *gateRequests) RecordConclusion(
	_ context.Context, key ports.SellEvaluationRequestKey, conclusion domain.SellEvaluationRequestConclusion,
) (ports.SellEvaluationRequestConclusionEffect, error) {
	if requests.forceEffect != ports.SellEvaluationRequestConclusionEffectInvalid {
		return requests.forceEffect, nil
	}
	index := key.TenantID.String() + "/" + key.Request.String()
	record := requests.records[index]
	record.State = conclusion.State()
	record.FormedCharge, _ = conclusion.FormedCharge()
	requests.records[index] = record
	return ports.SellEvaluationRequestConcluded, nil
}

type gateMappings map[string]string

func (mappings gateMappings) FindFeeItemMapping(_ context.Context, _ domain.TenantID, code domain.PricingChargeCode) (domain.FeeItemMapping, bool, error) {
	item, found := mappings[code.String()]
	if !found {
		return domain.FeeItemMapping{}, false, nil
	}
	feeItem, _ := domain.NewFeeItemReference(item)
	basis, _ := domain.NewFeeItemMappingBasisReference("SYN-CLAUSE-1")
	mapping, err := domain.NewFeeItemMapping(code, feeItem, basis)
	return mapping, err == nil, err
}

type gateCharges struct {
	saved map[string]domain.CustomerCharge
}

func (charges *gateCharges) SaveEstimated(_ context.Context, _ domain.TenantID, charge domain.CustomerCharge) (ports.ChargeSaveOutcome, error) {
	if _, exists := charges.saved[charge.ID().String()]; exists {
		return ports.ChargeAlreadyConfirmed, nil
	}
	charges.saved[charge.ID().String()] = charge
	return ports.ChargeSaved, nil
}

type gateFixture struct {
	gate     *adapter.FormOnSellEvaluationRecordedAdapter
	requests *gateRequests
	charges  *gateCharges
}

// newGateFixture 在请求册上登一份`已提交`的 CNY 请求 SELLREQ-SYN-1，评价库装 evaluations，映射册按 mappings 登。
func newGateFixture(t *testing.T, mappings gateMappings, evaluations ...ppdomain.PricingEvaluation) *gateFixture {
	t.Helper()
	tenant := must[domain.TenantID](t)(domain.NewTenantID(gateTenant))
	request := must[domain.SellEvaluationRequest](t)(domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 must[domain.SellEvaluationRequestID](t)(domain.NewSellEvaluationRequestID("SELLREQ-SYN-1")),
		Parcel:             must[domain.ParcelReference](t)(domain.NewParcelReference("pkg-1")),
		Account:            must[domain.SettlementAccountID](t)(domain.NewSettlementAccountID("SYN-ACCT-1")),
		SettlementCurrency: must[domain.CurrencyCode](t)(domain.NewCurrencyCode("CNY")),
		PricingScope:       must[domain.EstimatePricingScope](t)(domain.NewEstimatePricingScope("scope-1")),
		Moment:             domain.BillingMomentNetworkIntakeAdopted,
		Source:             must[domain.SourceFactReference](t)(domain.NewSourceFactReference("SYN-INTAKE-1")),
		PricingBasisAt:     basisAt,
	}))
	requests := &gateRequests{records: map[string]ports.SellEvaluationRequestRecord{
		gateTenant + "/SELLREQ-SYN-1": {
			Key:     ports.SellEvaluationRequestKey{TenantID: tenant, Request: request.ID()},
			Request: request, RecordedAt: basisAt, State: domain.SellRequestSubmitted,
		},
	}}
	charges := &gateCharges{saved: map[string]domain.CustomerCharge{}}
	view := newSellAdapter(t, evaluations...)
	formSell := must[*saapplication.FormSellCustomerChargeHandler](t)(saapplication.NewFormSellCustomerChargeHandler(view, charges))
	formation := must[*saapplication.FormEstimatedCustomerChargeHandler](t)(saapplication.NewFormEstimatedCustomerChargeHandler(
		saapplication.FormEstimatedCustomerChargeDeps{
			Requests: requests, Conclusions: requests, Mappings: mappings, Charges: formSell, Clock: gateClock{},
		}))
	gate := must[*adapter.FormOnSellEvaluationRecordedAdapter](t)(adapter.NewFormOnSellEvaluationRecordedAdapter(view, formation))
	return &gateFixture{gate: gate, requests: requests, charges: charges}
}

func (fixture *gateFixture) handle(t *testing.T, evaluation string) error {
	t.Helper()
	return fixture.gate.HandleRecordedSellEvaluation(t.Context(), sainbox.RecordedSellEvaluation{
		TenantID: gateTenant, EvaluationID: evaluation,
	})
}

func (fixture *gateFixture) request() ports.SellEvaluationRequestRecord {
	return fixture.requests.records[gateTenant+"/SELLREQ-SYN-1"]
}

var freightOnly = gateMappings{"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT"}

// Covers: 回指 SELL 请求、全部费用行映到同一个费用项目的评价经门形成一笔预估客户费用，标识由请求派生，金额出自
// 评价（105 元 → 10500 分）；请求记`已形成`。同一封再投一次入账、仍只有一笔。
func TestTheSellGateFormsTheEstimatedCustomerChargeFromAMappedEvaluation(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-1", "SELLREQ-SYN-1",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")
	fixture := newGateFixture(t, freightOnly, evaluation)

	if err := fixture.handle(t, "eval-sell-1"); err != nil {
		t.Fatalf("门：%v", err)
	}
	charge, saved := fixture.charges.saved["SELLCC-SELLREQ-SYN-1"]
	if !saved || len(fixture.charges.saved) != 1 {
		t.Fatalf("费用库 = %v，want 恰一笔 SELLCC-SELLREQ-SYN-1", fixture.charges.saved)
	}
	if currency, minor := charge.SettlementAmount(); currency.String() != "CNY" || minor != 10500 || charge.FeeItem().String() != "SYN-FEE-FREIGHT" {
		t.Fatalf("费用 = %s %d 项目 %s", currency, minor, charge.FeeItem())
	}
	if request := fixture.request(); request.State != domain.SellRequestFormed || request.FormedCharge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("请求册 = %s / %q", request.State, request.FormedCharge)
	}
	if err := fixture.handle(t, "eval-sell-1"); err != nil || len(fixture.charges.saved) != 1 {
		t.Fatalf("重放：err = %v，费用 %d 笔", err, len(fixture.charges.saved))
	}
}

// Covers: BUY 评价与不回指 SELL 请求的 SELL 评价都入账、零写入：前者归 BUY 那一路，后者不是为包裹计费时点形成的。
func TestTheSellGateLetsBuyAndUnreferencedEvaluationsPass(t *testing.T) {
	buy := sellEvaluation(t, "eval-buy-1", "EVREQ-SYN-1",
		sellCard(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true), "Z1")
	unreferenced := sellEvaluation(t, "eval-sell-2", "",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")
	fixture := newGateFixture(t, freightOnly, buy, unreferenced)

	for _, id := range []string{"eval-buy-1", "eval-sell-2"} {
		if err := fixture.handle(t, id); err != nil {
			t.Fatalf("%s 被当成了错误：%v", id, err)
		}
	}
	if len(fixture.charges.saved) != 0 || fixture.request().State != domain.SellRequestSubmitted {
		t.Fatalf("零写入被破：费用 %d 笔、请求 %s", len(fixture.charges.saved), fixture.request().State)
	}
}

// Covers: 卡没声明金额取整的评价入账、不重投，请求记`待判断`，不建费用（ADR-0190 决定五；租户取值缺席）。
func TestTheSellGateRecordsPendingJudgmentForAnUndeclaredAmountPrecision(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-3", "SELLREQ-SYN-1",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, false), "Z1")
	fixture := newGateFixture(t, freightOnly, evaluation)

	if err := fixture.handle(t, "eval-sell-3"); err != nil {
		t.Fatalf("金额未声明应入账：%v", err)
	}
	if request := fixture.request(); request.State != domain.SellRequestPendingJudgment || len(fixture.charges.saved) != 0 {
		t.Fatalf("请求 = %s，费用 %d 笔，want 待判断、零费用", request.State, len(fixture.charges.saved))
	}
}

// Covers: 评价读不回与请求册答不出是唯一那枚未决哨兵；请求册上另有结论原样上抛、不是未决；空白引用是引用坏了。
func TestTheSellGateSeparatesUndecidedFromContradictions(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-1", "SELLREQ-SYN-1",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")

	missing := newGateFixture(t, freightOnly)
	if err := missing.handle(t, "eval-sell-1"); !errors.Is(err, adapter.ErrSellChargeFormationUndecided) {
		t.Fatalf("评价读不回 err = %v", err)
	}
	down := newGateFixture(t, freightOnly, evaluation)
	down.requests.viewErr = errors.New("db down")
	if err := down.handle(t, "eval-sell-1"); !errors.Is(err, adapter.ErrSellChargeFormationUndecided) {
		t.Fatalf("请求册答不出 err = %v", err)
	}
	otherwise := newGateFixture(t, freightOnly, evaluation)
	otherwise.requests.forceEffect = ports.SellEvaluationRequestConcludedOtherwise
	err := otherwise.handle(t, "eval-sell-1")
	if !errors.Is(err, saapplication.ErrSellEvaluationRequestConcludedOtherwise) || errors.Is(err, adapter.ErrSellChargeFormationUndecided) {
		t.Fatalf("另有结论 err = %v，want 原样上抛、不是未决", err)
	}
	if err := otherwise.gate.HandleRecordedSellEvaluation(t.Context(), sainbox.RecordedSellEvaluation{TenantID: " ", EvaluationID: "eval-sell-1"}); !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("空白租户 err = %v", err)
	}
}

func TestTheSellGateRefusesNilDependencies(t *testing.T) {
	view := newSellAdapter(t)
	if _, err := adapter.NewFormOnSellEvaluationRecordedAdapter(nil, nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
	if _, err := adapter.NewFormOnSellEvaluationRecordedAdapter(view, nil); err == nil {
		t.Fatal("nil 形成编排被收下了")
	}
}
