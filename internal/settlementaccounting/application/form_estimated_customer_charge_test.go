package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0190 决定五的形成编排外层（票 sell-customer-charge/04）：SELL·客户费用评价已记录之后，按评价回指读回 SELL
// 请求，全部费用行经映射册映到同一个费用项目才形成，费用标识由请求派生，请求册记`已形成`与费用标识；映不全、映到
// 多个、金额未声明或币种对不上记`待判断`（AT-SA-174、AT-SA-177），不可计价与冲突照记，失败停在`已提交`；不是 SELL
// 请求的评价零写入；重放不多形成。依赖全是替身，金额断言只看它是否原样出自评价。夹具全是合成串。

var estimatedChargeAt = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

const (
	sellRequestID    = "SELLREQ-SYN-1"
	sellEvaluationID = "SYN-EVAL-SELL-1"
	derivedChargeID  = "SELLCC-SELLREQ-SYN-1"
)

// sellRequestRegisterDouble 同时是请求读口与改记口：RecordConclusion 把状态写回读口那一份，重放因此走得到「早有结论」。
type sellRequestRegisterDouble struct {
	records     map[string]ports.SellEvaluationRequestRecord
	viewErr     error
	recordErr   error
	forceEffect ports.SellEvaluationRequestConclusionEffect
	concluded   []domain.SellEvaluationRequestConclusion
}

func (double *sellRequestRegisterDouble) FindByID(
	_ context.Context,
	key ports.SellEvaluationRequestKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	if double.viewErr != nil {
		return ports.SellEvaluationRequestRecord{}, false, double.viewErr
	}
	record, found := double.records[key.TenantID.String()+"/"+key.Request.String()]
	return record, found, nil
}

func (double *sellRequestRegisterDouble) FindCurrentByParcel(
	context.Context, domain.TenantID, domain.ParcelReference,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return ports.SellEvaluationRequestRecord{}, false, errors.New("not used by the formation")
}

func (double *sellRequestRegisterDouble) RecordConclusion(
	_ context.Context,
	key ports.SellEvaluationRequestKey,
	conclusion domain.SellEvaluationRequestConclusion,
) (ports.SellEvaluationRequestConclusionEffect, error) {
	if double.recordErr != nil {
		return ports.SellEvaluationRequestConclusionEffectInvalid, double.recordErr
	}
	if double.forceEffect != ports.SellEvaluationRequestConclusionEffectInvalid {
		return double.forceEffect, nil
	}
	double.concluded = append(double.concluded, conclusion)
	index := key.TenantID.String() + "/" + key.Request.String()
	record := double.records[index]
	record.State = conclusion.State()
	record.FormedCharge, _ = conclusion.FormedCharge()
	double.records[index] = record
	return ports.SellEvaluationRequestConcluded, nil
}

type feeItemMappingViewDouble struct {
	items map[string]string
	err   error
	asked []string
}

func (double *feeItemMappingViewDouble) FindFeeItemMapping(
	_ context.Context,
	_ domain.TenantID,
	code domain.PricingChargeCode,
) (domain.FeeItemMapping, bool, error) {
	double.asked = append(double.asked, code.String())
	if double.err != nil {
		return domain.FeeItemMapping{}, false, double.err
	}
	item, found := double.items[code.String()]
	if !found {
		return domain.FeeItemMapping{}, false, nil
	}
	mapping, err := domain.NewFeeItemMapping(code, mustFeeItem(item), mustBasis("SYN-CLAUSE-"+code.String()))
	return mapping, err == nil, err
}

// formationEvaluationDouble 是形成编排本体按评价引用再读的那一份——与命令上的采用快照同一份。
type formationEvaluationDouble struct {
	adoption ports.SellEvaluationAdoption
}

func (double *formationEvaluationDouble) LoadSellEvaluation(
	context.Context, domain.TenantID, domain.SellEvaluationReference,
) (ports.SellEvaluationAdoption, bool, error) {
	return double.adoption, true, nil
}

// estimatedChargeStoreDouble 照费用库主键（租户、费用标识）挡第二笔：同一标识再存答`已存在`。
type estimatedChargeStoreDouble struct {
	saved map[string]domain.CustomerCharge
	err   error
}

func (double *estimatedChargeStoreDouble) SaveEstimated(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	if double.err != nil {
		return ports.ChargeSaveOutcomeInvalid, double.err
	}
	if _, exists := double.saved[charge.ID().String()]; exists {
		return ports.ChargeAlreadyConfirmed, nil
	}
	double.saved[charge.ID().String()] = charge
	return ports.ChargeSaved, nil
}

func mustFeeItem(raw string) domain.FeeItemReference {
	item, _ := domain.NewFeeItemReference(raw)
	return item
}

func mustBasis(raw string) domain.FeeItemMappingBasisReference {
	basis, _ := domain.NewFeeItemMappingBasisReference(raw)
	return basis
}

type estimatedChargeFixture struct {
	tenant   domain.TenantID
	requests *sellRequestRegisterDouble
	mappings *feeItemMappingViewDouble
	charges  *estimatedChargeStoreDouble
	adoption ports.SellEvaluationAdoption
	handler  *application.FormEstimatedCustomerChargeHandler
}

// newEstimatedChargeFixture 登一份`已提交`的 CNY 请求与一份已完成的 CNY 评价（12000 分，两条费用行：BASE_FREIGHT 与
// REMOTE_AREA_SURCHARGE），映射册按 items 登。
func newEstimatedChargeFixture(t *testing.T, items map[string]string) *estimatedChargeFixture {
	t.Helper()
	tenant := mustAccountValue(t, domain.NewTenantID, "SYN-T1")
	cny := mustAccountValue(t, domain.NewCurrencyCode, "CNY")
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 mustAccountValue(t, domain.NewSellEvaluationRequestID, sellRequestID),
		Parcel:             mustAccountValue(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Account:            mustAccountValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		SettlementCurrency: cny,
		PricingScope:       mustAccountValue(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
		Moment:             domain.BillingMomentNetworkIntakeAdopted,
		Source:             mustAccountValue(t, domain.NewSourceFactReference, "SYN-INTAKE-1"),
		PricingBasisAt:     estimatedChargeAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("SELL 请求：%v", err)
	}
	fixture := &estimatedChargeFixture{
		tenant: tenant,
		requests: &sellRequestRegisterDouble{records: map[string]ports.SellEvaluationRequestRecord{
			tenant.String() + "/" + sellRequestID: {
				Key:        ports.SellEvaluationRequestKey{TenantID: tenant, Request: request.ID()},
				Request:    request,
				RecordedAt: estimatedChargeAt.Add(-time.Hour),
				State:      domain.SellRequestSubmitted,
			},
		}},
		mappings: &feeItemMappingViewDouble{items: items},
		charges:  &estimatedChargeStoreDouble{saved: map[string]domain.CustomerCharge{}},
		adoption: ports.SellEvaluationAdoption{
			Evaluation: mustAccountValue(t, domain.NewSellEvaluationReference, sellEvaluationID),
			Outcome:    ports.SellEvaluationCompleted,
			Request:    request.ID(),
			ChargeCodes: []domain.PricingChargeCode{
				mustAccountValue(t, domain.NewPricingChargeCode, "BASE_FREIGHT"),
				mustAccountValue(t, domain.NewPricingChargeCode, "REMOTE_AREA_SURCHARGE"),
			},
			OriginalCurrency:   cny,
			OriginalMinor:      12000,
			SettlementCurrency: cny,
			SettlementMinor:    12000,
		},
	}
	return fixture
}

// build 按 fixture 当下的采用快照装编排：形成编排本体读到的评价与命令上的同一份。
func (fixture *estimatedChargeFixture) build(t *testing.T) {
	t.Helper()
	charges, err := application.NewFormSellCustomerChargeHandler(&formationEvaluationDouble{adoption: fixture.adoption}, fixture.charges)
	if err != nil {
		t.Fatalf("形成编排本体：%v", err)
	}
	handler, err := application.NewFormEstimatedCustomerChargeHandler(application.FormEstimatedCustomerChargeDeps{
		Requests:    fixture.requests,
		Conclusions: fixture.requests,
		Mappings:    fixture.mappings,
		Charges:     charges,
		Clock:       fixedClock{at: estimatedChargeAt},
	})
	if err != nil {
		t.Fatalf("形成编排外层：%v", err)
	}
	fixture.handler = handler
}

func (fixture *estimatedChargeFixture) handle(t *testing.T) (application.EstimatedCustomerChargeResult, error) {
	t.Helper()
	if fixture.handler == nil {
		fixture.build(t)
	}
	return fixture.handler.Handle(t.Context(), application.FormEstimatedCustomerChargeCommand{
		TenantID: fixture.tenant, Evaluation: fixture.adoption,
	})
}

func (fixture *estimatedChargeFixture) storedState(t *testing.T) ports.SellEvaluationRequestRecord {
	t.Helper()
	return fixture.requests.records[fixture.tenant.String()+"/"+sellRequestID]
}

// assertPendingWithoutCharge 断言请求记了`待判断`、费用库零写入。
func (fixture *estimatedChargeFixture) assertPendingWithoutCharge(t *testing.T, result application.EstimatedCustomerChargeResult, err error) {
	t.Helper()
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != domain.SellRequestPendingJudgment {
		t.Fatalf("结果 = %s / %s err = %v，want 记待判断", result.Outcome(), result.State(), err)
	}
	if _, has := result.Charge(); has {
		t.Fatal("待判断带了费用标识")
	}
	if len(fixture.charges.saved) != 0 {
		t.Fatalf("待判断却建了 %d 笔费用（占位费用）", len(fixture.charges.saved))
	}
	if stored := fixture.storedState(t); stored.State != domain.SellRequestPendingJudgment || stored.FormedCharge.String() != "" {
		t.Fatalf("请求册 = %s / %q，want PENDING_JUDGMENT 不带费用", stored.State, stored.FormedCharge)
	}
}

// Covers: 全部费用行映到同一个费用项目 → 形成一笔预估客户费用：标识由请求派生，费用项目取映射，金额整组出自评价；
// 请求册同一笔记`已形成`与那笔费用标识。重复的代码只问一次映射册。
func TestEveryChargeLineMappingToOneFeeItemFormsTheEstimatedCustomerCharge(t *testing.T) {
	fixture := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	fixture.adoption.ChargeCodes = append(fixture.adoption.ChargeCodes, fixture.adoption.ChargeCodes[0])

	result, err := fixture.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s / %s err = %v，want 已形成", result.Outcome(), result.State(), err)
	}
	if charge, has := result.Charge(); !has || charge.String() != derivedChargeID {
		t.Fatalf("结果费用 = %q %v，want %s", charge, has, derivedChargeID)
	}
	charge, saved := fixture.charges.saved[derivedChargeID]
	if !saved || len(fixture.charges.saved) != 1 {
		t.Fatalf("费用库 = %v，want 恰一笔 %s", fixture.charges.saved, derivedChargeID)
	}
	if charge.FeeItem().String() != "SYN-FEE-FREIGHT" || charge.Evaluation().String() != sellEvaluationID || charge.Stage() != domain.ChargeEstimated {
		t.Fatalf("费用 = 项目 %s 评价 %s 阶段 %s", charge.FeeItem(), charge.Evaluation(), charge.Stage())
	}
	if currency, minor := charge.SettlementAmount(); currency.String() != "CNY" || minor != 12000 {
		t.Fatalf("结算金额 = %s %d，want 评价上的 CNY 12000", currency, minor)
	}
	if !charge.FormedAt().Equal(estimatedChargeAt) {
		t.Fatalf("形成时刻 = %v，want 时钟给的 %v", charge.FormedAt(), estimatedChargeAt)
	}
	if stored := fixture.storedState(t); stored.State != domain.SellRequestFormed || stored.FormedCharge.String() != derivedChargeID {
		t.Fatalf("请求册 = %s / %q，want FORMED / %s", stored.State, stored.FormedCharge, derivedChargeID)
	}
	if len(fixture.mappings.asked) != 2 {
		t.Fatalf("映射册被问了 %v，want 两个不同代码各一次", fixture.mappings.asked)
	}
}

// Covers: AT-SA-174——任一费用行没有映射，记`待判断`，不建占位费用、不拿价格代码当费用项目。
func TestAChargeLineWithoutAMappingLeavesTheRequestPendingJudgment(t *testing.T) {
	fixture := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
	result, err := fixture.handle(t)
	fixture.assertPendingWithoutCharge(t, result, err)
}

// Covers: AT-SA-174——全部费用行映到多于一个费用项目，记`待判断`，不挑其中一个。
func TestChargeLinesMappingToTwoFeeItemsLeaveTheRequestPendingJudgment(t *testing.T) {
	fixture := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-REMOTE",
	})
	result, err := fixture.handle(t)
	fixture.assertPendingWithoutCharge(t, result, err)
}

// Covers: 映射册上这一租户一行都没登（租户取值缺席）同样是`待判断`；评价没有费用行也映不到一个项目。
func TestAnUnregisteredMappingOrNoChargeLinesLeaveTheRequestPendingJudgment(t *testing.T) {
	fixture := newEstimatedChargeFixture(t, nil)
	result, err := fixture.handle(t)
	fixture.assertPendingWithoutCharge(t, result, err)

	empty := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
	empty.adoption.ChargeCodes = nil
	result, err = empty.handle(t)
	empty.assertPendingWithoutCharge(t, result, err)
}

// Covers: 卡没声明金额取整（租户取值缺席，ADR-0107）与评价结算币种对不上请求快照（AT-SA-177）都记`待判断`，不重投。
func TestAnUndeclaredAmountPrecisionOrAForeignSettlementCurrencyLeavesThePendingJudgment(t *testing.T) {
	undeclared := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	undeclared.adoption.Outcome = ports.SellEvaluationAmountUndeclared
	result, err := undeclared.handle(t)
	undeclared.assertPendingWithoutCharge(t, result, err)

	foreign := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	usd := mustAccountValue(t, domain.NewCurrencyCode, "USD")
	foreign.adoption.SettlementCurrency, foreign.adoption.OriginalCurrency = usd, usd
	result, err = foreign.handle(t)
	foreign.assertPendingWithoutCharge(t, result, err)
}

// Covers: ADR-0190 决定五——评价待判断、不可计价、冲突各照记一格，入账不重投，不形成任何费用（AT-SA-175）；
// 评价失败时请求停在`已提交`、不改记（请求册不设「未形成」那一格）。
func TestPendingUnratableAndConflictEvaluationsAreRecordedAndAFailedOneLeavesTheRequestSubmitted(t *testing.T) {
	for outcome, want := range map[ports.SellEvaluationOutcome]domain.SellEvaluationRequestState{
		ports.SellEvaluationPending:   domain.SellRequestPendingJudgment,
		ports.SellEvaluationUnratable: domain.SellRequestUnratable,
		ports.SellEvaluationConflict:  domain.SellRequestConflict,
	} {
		fixture := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
		fixture.adoption.Outcome = outcome
		fixture.adoption.ChargeCodes = nil
		result, err := fixture.handle(t)
		if err != nil || result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != want {
			t.Fatalf("%s：结果 = %s / %s err = %v，want 记 %s", outcome, result.Outcome(), result.State(), err, want)
		}
		if stored := fixture.storedState(t); stored.State != want || len(fixture.charges.saved) != 0 || len(fixture.mappings.asked) != 0 {
			t.Fatalf("%s：请求册 = %s，费用 %d 笔，问映射 %v", outcome, stored.State, len(fixture.charges.saved), fixture.mappings.asked)
		}
	}

	failed := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
	failed.adoption.Outcome = ports.SellEvaluationNotFormed
	failed.adoption.ChargeCodes = nil
	result, err := failed.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeEvaluationNotFormed {
		t.Fatalf("失败评价：结果 = %s err = %v，want 停在已提交", result.Outcome(), err)
	}
	if stored := failed.storedState(t); stored.State != domain.SellRequestSubmitted || len(failed.requests.concluded) != 0 {
		t.Fatalf("失败评价改记了请求：%s，记了 %d 次", stored.State, len(failed.requests.concluded))
	}
}

// Covers: 不是为 SELL 请求形成的评价——没有回指，或回指的标识不在 SELL 请求册上（例如一份 BUY 请求标识）——零写入。
func TestAnEvaluationThatDoesNotReferToASellRequestIsSkipped(t *testing.T) {
	unreferenced := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
	unreferenced.adoption.Request = domain.SellEvaluationRequestID{}
	result, err := unreferenced.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeNoSellRequest {
		t.Fatalf("没有回指：结果 = %s err = %v", result.Outcome(), err)
	}

	buy := newEstimatedChargeFixture(t, map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT"})
	buy.adoption.Request = mustAccountValue(t, domain.NewSellEvaluationRequestID, "EVREQ-SYN-BUY-1")
	result, err = buy.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeNoSellRequest {
		t.Fatalf("回指 BUY 请求：结果 = %s err = %v", result.Outcome(), err)
	}
	for name, fixture := range map[string]*estimatedChargeFixture{"没有回指": unreferenced, "回指 BUY 请求": buy} {
		if len(fixture.charges.saved) != 0 || len(fixture.requests.concluded) != 0 || len(fixture.mappings.asked) != 0 {
			t.Fatalf("%s：费用 %d 笔、结论 %d 次、问映射 %v，want 零写入", name,
				len(fixture.charges.saved), len(fixture.requests.concluded), fixture.mappings.asked)
		}
	}
}

// Covers: 重放不多形成——同一份评价再来一次，请求已是`已形成`，原样答早有结论，不再碰映射册与费用库；费用已在库上
// （上一拍形成了费用却没记上结论）时照派生标识答`已存在`，结论照记`已形成`、仍只有一笔。
func TestAReplayDoesNotFormTwice(t *testing.T) {
	fixture := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	if result, err := fixture.handle(t); err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("首拍：%s / %s err = %v", result.Outcome(), result.State(), err)
	}
	asked := len(fixture.mappings.asked)
	result, err := fixture.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeAlreadyConcluded || result.State() != domain.SellRequestFormed {
		t.Fatalf("重放：结果 = %s / %s err = %v，want 早有结论 FORMED", result.Outcome(), result.State(), err)
	}
	if charge, has := result.Charge(); !has || charge.String() != derivedChargeID {
		t.Fatalf("重放交回的费用 = %q %v", charge, has)
	}
	if len(fixture.charges.saved) != 1 || len(fixture.requests.concluded) != 1 || len(fixture.mappings.asked) != asked {
		t.Fatalf("重放又写了：费用 %d 笔、结论 %d 次、问映射 %d 次", len(fixture.charges.saved), len(fixture.requests.concluded), len(fixture.mappings.asked))
	}

	halfway := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	halfway.charges.saved[derivedChargeID] = domain.CustomerCharge{}
	result, err = halfway.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed || len(halfway.charges.saved) != 1 {
		t.Fatalf("费用已在库上：结果 = %s / %s err = %v，费用 %d 笔", result.Outcome(), result.State(), err, len(halfway.charges.saved))
	}
}

// Covers: 依赖答不出时是未决、指名停在哪一个依赖上；请求册上另有结论是错误、不覆盖；封闭集外的评价结果是错误。
func TestUnavailableDependenciesAreUndecidedAndContradictionsAreErrors(t *testing.T) {
	mapped := map[string]string{"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT"}
	broken := errors.New("db down")
	for name, tc := range map[string]struct {
		breakIt func(*estimatedChargeFixture)
		reason  application.EstimatedCustomerChargeUndecidedReason
	}{
		"请求读口": {func(f *estimatedChargeFixture) { f.requests.viewErr = broken }, application.SellEvaluationRequestViewUnavailable},
		"映射读口": {func(f *estimatedChargeFixture) { f.mappings.err = broken }, application.FeeItemMappingViewUnavailable},
		"费用库":  {func(f *estimatedChargeFixture) { f.charges.err = broken }, application.SellChargeFormationUndecided},
		"请求改记": {func(f *estimatedChargeFixture) { f.requests.recordErr = broken }, application.SellEvaluationRequestRegisterUnavailable},
	} {
		fixture := newEstimatedChargeFixture(t, mapped)
		tc.breakIt(fixture)
		result, err := fixture.handle(t)
		if err != nil || result.Outcome() != application.EstimatedCustomerChargeUndecided || result.UndecidedReason() != tc.reason {
			t.Fatalf("%s：结果 = %s / %s err = %v，want 未决 %s", name, result.Outcome(), result.UndecidedReason(), err, tc.reason)
		}
	}

	otherwise := newEstimatedChargeFixture(t, mapped)
	otherwise.requests.forceEffect = ports.SellEvaluationRequestConcludedOtherwise
	if _, err := otherwise.handle(t); !errors.Is(err, application.ErrSellEvaluationRequestConcludedOtherwise) {
		t.Fatalf("另有结论 err = %v", err)
	}
	unknown := newEstimatedChargeFixture(t, mapped)
	unknown.adoption.Outcome = ports.SellEvaluationOutcomeInvalid
	if _, err := unknown.handle(t); !errors.Is(err, application.ErrUnexpectedSellEvaluationOutcome) {
		t.Fatalf("封闭集外的结果 err = %v", err)
	}
}

func TestTheEstimatedCustomerChargeHandlerRefusesMissingDependencies(t *testing.T) {
	if _, err := application.NewFormEstimatedCustomerChargeHandler(application.FormEstimatedCustomerChargeDeps{}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺依赖 err = %v", err)
	}
}
