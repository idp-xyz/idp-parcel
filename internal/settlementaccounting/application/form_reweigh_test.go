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

// supersessionDouble 记下两种替代：带后继的（saved）与没有后继的（withoutSuccessor，新评价合计为零）。Superseded 只认记下过的。
type supersessionDouble struct {
	saved            [][2]string
	withoutSuccessor []string
}

func (double *supersessionDouble) SaveSupersessionWithoutSuccessor(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
	_ time.Time,
) error {
	double.withoutSuccessor = append(double.withoutSuccessor, charge.String())
	return nil
}

func (double *supersessionDouble) SaveSupersession(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
	supersededBy domain.CustomerChargeID,
	_ time.Time,
) error {
	double.saved = append(double.saved, [2]string{charge.String(), supersededBy.String()})
	return nil
}

func (double *supersessionDouble) Superseded(_ context.Context, _ domain.TenantID, charge domain.CustomerChargeID) (bool, error) {
	for _, withoutSuccessor := range double.withoutSuccessor {
		if withoutSuccessor == charge.String() {
			return true, nil
		}
	}
	for _, pair := range double.saved {
		if pair[0] == charge.String() {
			return true, nil
		}
	}
	return false, nil
}

type reweighAdoptionDouble struct{ adopted bool }

// crossCurrencyEstimatedCharge 形成一笔跨币种的预估费用：原币 USD 1700、结算币 CNY 12000，带换算依据。
func crossCurrencyEstimatedCharge(t *testing.T, id domain.CustomerChargeID) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID: id, FeeItem: mustFeeItem("SYN-FEE-FREIGHT"),
		Evaluation:         mustAccountValue(t, domain.NewSellEvaluationReference, "SYN-EVAL-SELL-0"),
		OriginalCurrency:   mustAccountValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      1700,
		SettlementCurrency: mustAccountValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    12000,
		Conversion:         mustAccountValue(t, domain.NewConversionStepReference, "SYN-FX-0"),
		Stage:              domain.ChargeEstimated,
		FormedAt:           estimatedChargeAt.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("跨币种旧费用：%v", err)
	}
	return charge
}

func (double reweighAdoptionDouble) ReweighAdjustmentAdopted(
	context.Context, domain.TenantID, domain.SettlementAccountID,
) (bool, error) {
	return double.adopted, nil
}

func TestAnUnconfirmedPredecessorIsSupersededByTheNewEstimate(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeEstimated, 12000, false)
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	charge, has := result.Charge()
	if !has || charge.String() != "SELLCC-SELLREQ-SYN-2" {
		t.Fatalf("新费用 = %q %v", charge, has)
	}
	if len(fixture.supersessions.saved) != 1 || fixture.supersessions.saved[0] != [2]string{"SELLCC-SELLREQ-SYN-1", "SELLCC-SELLREQ-SYN-2"} {
		t.Fatalf("替代 = %v", fixture.supersessions.saved)
	}
	if fixture.adjustments.saves != 0 {
		t.Fatalf("未确认的前一代却记了 %d 笔调整", fixture.adjustments.saves)
	}
	if len(fixture.locks.parcels) != 1 || fixture.locks.parcels[0] != "SYN-PARCEL-1" {
		t.Fatalf("包裹确认锁 = %v，want 读链之前取一次", fixture.locks.parcels)
	}
}

func TestAConfirmedPredecessorStopsUntilReweighAdjustmentIsAdopted(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeConfirmed, 15000, false)
	result, err := fixture.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != domain.SellRequestPendingJudgment {
		t.Fatalf("结果 = %s / %s err = %v，want 待判断", result.Outcome(), result.State(), err)
	}
	if got, stopped := result.StopCause(); !stopped || got != domain.StopReweighAdjustmentNotAdopted {
		t.Fatalf("停因 = %s %v", got, stopped)
	}
	stored := fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-2"]
	if stored.State != domain.SellRequestPendingJudgment || stored.StopCause != domain.StopReweighAdjustmentNotAdopted || stored.FormedCharge.String() != "" {
		t.Fatalf("下一代 = %s / %s / %q", stored.State, stored.StopCause, stored.FormedCharge)
	}
	if len(fixture.supersessions.saved) != 0 || fixture.adjustments.saves != 0 {
		t.Fatalf("没采用却替代 %v 或调整 %d", fixture.supersessions.saved, fixture.adjustments.saves)
	}
}

func TestAnAdoptedReweighRecordsTheDeltaOnTheConfirmedCharge(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeConfirmed, 15000, true)
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	charge, has := result.Charge()
	if !has || charge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("结论费用 = %q %v，want 原来那笔已确认费用", charge, has)
	}
	if len(fixture.charges.saved) != 0 {
		t.Fatalf("已确认之后又建了预估 %v", fixture.charges.saved)
	}
	if fixture.adjustments.saves != 1 {
		t.Fatalf("调整笔数 = %d，want 1", fixture.adjustments.saves)
	}
}

// crossCurrencyConfirmedPredecessor 把链上前一笔费用换成跨币种的已确认费用，见 crossCurrencyEstimatedCharge。
func crossCurrencyConfirmedPredecessor(t *testing.T) *reweighFormation {
	t.Helper()
	var prior *chargeStoreDouble
	fixture := newReweighFormationWith(t, domain.ChargeConfirmed, 12000, true, func(deps *application.FormEstimatedCustomerChargeDeps) {
		prior = deps.PriorCharges.(*chargeStoreDouble)
	})
	oldID := mustAccountValue(t, domain.NewCustomerChargeID, "SELLCC-SELLREQ-SYN-1")
	confirmed, err := crossCurrencyEstimatedCharge(t, oldID).Confirm(
		registeredFacts(t),
		mustAccountValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-1"),
		estimatedChargeAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("确认跨币种旧费用：%v", err)
	}
	prior.charges[oldID.String()] = confirmed
	return fixture
}

// Covers: 前一笔已确认费用跨币种，差额按结算币判，不管原币那一格长什么样，这一代都有结论——已形成、指向同一笔费用，不上抛错误
// 让它一直重投：原币种与费用不同或原币差额记不成时按结算币记（原币种取结算币种、两额相等、无换算依据）；结算币差额为零
// 不记；原币可比且同号时照旧连原币差额一起记，换算依据取新评价的。
func TestAConfirmedCrossCurrencyPredecessorIsSettledOnTheSettlementCurrencyDifference(t *testing.T) {
	cases := []struct {
		name                         string
		originalCurrency             string
		originalMinor, settlementNew int64
		wantAdjustments              int
		wantOriginalCurrency         string
		wantOriginalMinor            int64
		wantConversion               string
	}{
		{"原币种与费用不同", "EUR", 2000, 15000, 1, "CNY", 3000, ""},
		{"结算币差额为零而原币差额不为零", "USD", 1800, 12000, 0, "", 0, ""},
		{"原币差额为零", "USD", 1700, 15000, 1, "CNY", 3000, ""},
		{"两个差额异号", "USD", 1800, 11000, 1, "CNY", 1000, ""},
		{"原币可比且同号", "USD", 2100, 15000, 1, "USD", 400, "SYN-FX-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := crossCurrencyConfirmedPredecessor(t)
			fixture.adoption.OriginalCurrency = mustAccountValue(t, domain.NewCurrencyCode, tc.originalCurrency)
			fixture.adoption.OriginalMinor = tc.originalMinor
			fixture.adoption.SettlementMinor = tc.settlementNew
			fixture.adoption.Conversion = mustAccountValue(t, domain.NewConversionStepReference, "SYN-FX-1")
			result, err := fixture.handle(t)
			if err != nil || result.State() != domain.SellRequestFormed {
				t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
			}
			if charge, has := result.Charge(); !has || charge.String() != "SELLCC-SELLREQ-SYN-1" {
				t.Fatalf("结论费用 = %q %v，want 原来那笔已确认费用", charge, has)
			}
			if len(fixture.charges.saved) != 0 || len(fixture.supersessions.saved) != 0 || fixture.adjustments.saves != tc.wantAdjustments {
				t.Fatalf("预估 %v、替代 %v、调整 %d 笔，want 调整 %d 笔", fixture.charges.saved, fixture.supersessions.saved,
					fixture.adjustments.saves, tc.wantAdjustments)
			}
			if tc.wantAdjustments == 0 {
				return
			}
			recorded, found := fixture.adjustments.records["REWEIGH-"+sellEvaluationID]
			if !found {
				t.Fatalf("册上没有 REWEIGH-%s", sellEvaluationID)
			}
			currency, minor := recorded.Adjustment.OriginalAmount()
			conversion, _ := recorded.Adjustment.Conversion()
			if currency.String() != tc.wantOriginalCurrency || minor != tc.wantOriginalMinor || conversion.String() != tc.wantConversion {
				t.Fatalf("调整原币 = %s %d 换算依据 %q，want %s %d %q",
					currency, minor, conversion, tc.wantOriginalCurrency, tc.wantOriginalMinor, tc.wantConversion)
			}
		})
	}
}

// Covers: 前一代停下、没有费用时，新一代仍认得出链上更早那笔已确认费用，照差额记调整，不给同一件包裹再建一笔全额。
// 没采用而停下、补登采用后续办出来的那一代走的就是这条路。
func TestAConfirmedChargeBehindAStoppedGenerationIsSettledByDelta(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeConfirmed, 15000, true)
	fixture.stopSecondAndSubmitThird(t, domain.StopReweighAdjustmentNotAdopted)
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	if charge, has := result.Charge(); !has || charge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("结论费用 = %q %v，want 链上那笔已确认费用", charge, has)
	}
	if len(fixture.charges.saved) != 0 {
		t.Fatalf("隔着一代停下的请求，又建了预估 %v", fixture.charges.saved)
	}
	if fixture.adjustments.saves != 1 || len(fixture.supersessions.saved) != 0 {
		t.Fatalf("调整 %d 笔，替代 %v；want 一笔调整、不记替代", fixture.adjustments.saves, fixture.supersessions.saved)
	}
}

// Covers: 前一代停下时，新预估替代的是链上更早那笔仍未确认的费用。仅有的那条实测被作废、退回没报尺寸的申报，取较大者的
// 卡因此停下，之后再称重量方，走的就是这条路。
func TestAnEstimateBehindAStoppedGenerationIsSuperseded(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeEstimated, 15000, true)
	fixture.stopSecondAndSubmitThird(t, domain.StopEvaluationInputIncomplete)
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	if charge, has := result.Charge(); !has || charge.String() != "SELLCC-SELLREQ-SYN-3" {
		t.Fatalf("新费用 = %q %v", charge, has)
	}
	if len(fixture.supersessions.saved) != 1 || fixture.supersessions.saved[0] != [2]string{"SELLCC-SELLREQ-SYN-1", "SELLCC-SELLREQ-SYN-3"} {
		t.Fatalf("替代 = %v，want 更早那笔预估被新预估替代", fixture.supersessions.saved)
	}
	if fixture.adjustments.saves != 0 {
		t.Fatalf("未确认的费用上记了 %d 笔调整", fixture.adjustments.saves)
	}
}

// Covers: 新评价合计为零、前一笔费用还没确认：复核结果整体取代预估，那笔费用照样被替代、不再确认，只是没有后继；这一代
// 记负向结束（零额），不建新费用、不记调整。
func TestAZeroTotalSupersedesTheUnconfirmedPredecessorWithoutASuccessor(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeEstimated, 0, false)
	result, err := fixture.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != domain.SellRequestZeroTotal {
		t.Fatalf("结果 = %s / %s err = %v，want 记负向结束（零额）", result.Outcome(), result.State(), err)
	}
	if _, has := result.Charge(); has {
		t.Fatal("零额带了费用标识")
	}
	if len(fixture.supersessions.withoutSuccessor) != 1 || fixture.supersessions.withoutSuccessor[0] != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("没有后继的替代 = %v，want 前一笔预估被替代", fixture.supersessions.withoutSuccessor)
	}
	if len(fixture.supersessions.saved) != 0 || len(fixture.charges.saved) != 0 || fixture.adjustments.saves != 0 {
		t.Fatalf("零额之外还写了：带后继的替代 %v，预估 %v，调整 %d 笔",
			fixture.supersessions.saved, fixture.charges.saved, fixture.adjustments.saves)
	}
	if stored := fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-2"]; stored.State != domain.SellRequestZeroTotal {
		t.Fatalf("这一代的状态 = %s，want 负向结束（零额）", stored.State)
	}
	if len(fixture.chargeLocks.locked) != 1 || fixture.chargeLocks.locked[0] != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("费用行锁 = %v，want 记替代之前锁住前一笔", fixture.chargeLocks.locked)
	}
}

// Covers: 链上没有费用（前一代停下、没有费用）时新评价合计为零：没有可替代的，不记替代，只记零额。
func TestAZeroTotalWithNoEarlierChargeRecordsNoSupersession(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeEstimated, 0, false)
	index := fixture.tenant.String() + "/" + sellRequestID
	first := fixture.requests.records[index]
	first.State, first.StopCause, first.FormedCharge = domain.SellRequestPendingJudgment, domain.StopEvaluationInputIncomplete, domain.CustomerChargeID{}
	fixture.requests.records[index] = first
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestZeroTotal {
		t.Fatalf("结果 = %s err = %v，want 负向结束（零额）", result.State(), err)
	}
	if len(fixture.supersessions.withoutSuccessor) != 0 || len(fixture.supersessions.saved) != 0 {
		t.Fatalf("链上没有费用却记了替代：%v %v", fixture.supersessions.withoutSuccessor, fixture.supersessions.saved)
	}
}

// Covers: 前一笔已确认时新评价合计为零，行为不变：不记替代、不记零额，走差额——差额就是把净额全数贷记；这一代已形成，
// 指向原来那笔已确认费用。
func TestAZeroTotalOnAConfirmedPredecessorCreditsTheWholeNetThroughTheDifference(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeConfirmed, 0, true)
	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	if charge, has := result.Charge(); !has || charge.String() != "SELLCC-SELLREQ-SYN-1" {
		t.Fatalf("结论费用 = %q %v，want 原来那笔已确认费用", charge, has)
	}
	if len(fixture.supersessions.withoutSuccessor) != 0 || len(fixture.supersessions.saved) != 0 {
		t.Fatalf("已确认的费用被记了替代：%v %v", fixture.supersessions.withoutSuccessor, fixture.supersessions.saved)
	}
	credit, found := fixture.adjustments.records["REWEIGH-"+sellEvaluationID]
	if _, minor := credit.Adjustment.SettlementAmount(); !found || credit.Adjustment.Direction() != domain.AdjustmentCredit || minor != 12000 {
		t.Fatalf("调整 = %v %s %d，want 贷项 12000（净额全数贷记）", found, credit.Adjustment.Direction(), minor)
	}
}

// Covers: 被零额替代的那笔预估再去确认，答 CHARGE_SUPERSEDED，不落确认：确认编排读到的替代关系里没有后继也算数。
func TestAnEstimateSupersededByAZeroTotalIsNotConfirmedAfterwards(t *testing.T) {
	var prior *chargeStoreDouble
	fixture := newReweighFormationWith(t, domain.ChargeEstimated, 0, false, func(deps *application.FormEstimatedCustomerChargeDeps) {
		prior = deps.PriorCharges.(*chargeStoreDouble)
	})
	if result, err := fixture.handle(t); err != nil || result.State() != domain.SellRequestZeroTotal {
		t.Fatalf("零额 = %s err = %v", result.State(), err)
	}

	confirmation := newConfirmFixture(t)
	deps := confirmation.deps(t)
	deps.Charges, deps.Supersessions = prior, fixture.supersessions
	// 确认时刻不能早于这笔预估的形成时刻，否则领域的确认门先拒，盖住替代关系这一道。
	deps.Clock = confirmClock{at: estimatedChargeAt}
	handler, err := application.NewConfirmChargeHandler(deps)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(t.Context(), application.ConfirmChargeCommand{TenantID: fixture.tenant, ChargeID: "SELLCC-SELLREQ-SYN-1"})
	if err != nil || result.Outcome() != application.ChargeSuperseded {
		t.Fatalf("确认 = %s err = %v，want CHARGE_SUPERSEDED", result.Outcome(), err)
	}
	if prior.saves != 0 || prior.charges["SELLCC-SELLREQ-SYN-1"].Stage() == domain.ChargeConfirmed {
		t.Fatalf("被零额替代的预估落了确认：写入 %d 次", prior.saves)
	}
	if len(confirmation.handoff.intents) != 0 || len(confirmation.occupation.charges) != 0 {
		t.Fatalf("被替代的费用发了确认意图 %d 份、收口占用 %d 次", len(confirmation.handoff.intents), len(confirmation.occupation.charges))
	}
}

// Covers: 零额那一代之后再有一代算出非零金额、形成新预估：链上最近一笔费用仍是被零额替代的那笔预估（零额那一代没有费用，
// 跳过），新预估补成这条替代关系的后继。
func TestTheEstimateAfterAZeroTotalGenerationBecomesTheSuccessorOfTheSupersededCharge(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeEstimated, 15000, false)
	index := fixture.tenant.String() + "/SELLREQ-SYN-2"
	second := fixture.requests.records[index]
	second.State = domain.SellRequestZeroTotal
	fixture.requests.records[index] = second
	fixture.supersessions.withoutSuccessor = []string{"SELLCC-SELLREQ-SYN-1"}
	third := fixture.resumeAs(t, second.Request, "SELLREQ-SYN-3")
	fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-3"] = ports.SellEvaluationRequestRecord{
		Key:     ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: third.ID()},
		Request: third,
		State:   domain.SellRequestSubmitted,
	}
	fixture.adoption.Request = third.ID()

	result, err := fixture.handle(t)
	if err != nil || result.State() != domain.SellRequestFormed {
		t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
	}
	if len(fixture.supersessions.saved) != 1 || fixture.supersessions.saved[0] != [2]string{"SELLCC-SELLREQ-SYN-1", "SELLCC-SELLREQ-SYN-3"} {
		t.Fatalf("替代 = %v，want 新预估补成前一笔预估的后继", fixture.supersessions.saved)
	}
}

// Covers: 旧一代的评价晚于新一代的到。更晚的一代已经照它的评价办过钱，旧一代不再建费用、不记调整、不记替代，
// 只照记那一代的结论。
func TestAGenerationOvertakenByALaterOneMovesNoMoney(t *testing.T) {
	for name, stage := range map[string]domain.ChargeStage{"已确认": domain.ChargeConfirmed, "预估": domain.ChargeEstimated} {
		t.Run(name, func(t *testing.T) {
			fixture := newReweighFormation(t, stage, 15000, true)
			later := fixture.formThirdAhead(t, stage)
			result, err := fixture.handle(t)
			if err != nil || result.State() != domain.SellRequestFormed {
				t.Fatalf("结果 = %s err = %v，want 照记更晚一代的已形成", result.State(), err)
			}
			if charge, has := result.Charge(); !has || charge != later {
				t.Fatalf("结论费用 = %q %v，want %q", charge, has, later)
			}
			if len(fixture.charges.saved) != 0 || fixture.adjustments.saves != 0 || len(fixture.supersessions.saved) != 0 {
				t.Fatalf("被越过的一代动了钱：预估 %v，调整 %d 笔，替代 %v",
					fixture.charges.saved, fixture.adjustments.saves, fixture.supersessions.saved)
			}
		})
	}
}

// Covers: 前一代不在册上是两侧说法矛盾。当成链上没有费用往下走，会再建一笔全额，所以上抛、零写入。
func TestAMissingEarlierGenerationIsAnErrorNotAFirstCharge(t *testing.T) {
	fixture := newReweighFormation(t, domain.ChargeConfirmed, 15000, true)
	delete(fixture.requests.records, fixture.tenant.String()+"/"+sellRequestID)
	if _, err := fixture.handle(t); !errors.Is(err, application.ErrSellEvaluationRequestMissing) {
		t.Fatalf("err = %v，want 前一代不在册", err)
	}
	if len(fixture.charges.saved) != 0 || fixture.adjustments.saves != 0 {
		t.Fatalf("前一代不在册却写了：预估 %v，调整 %d 笔", fixture.charges.saved, fixture.adjustments.saves)
	}
}

// Covers: 处理链上已有费用的那一组依赖只装一部分，构造时即拒。
func TestThePartlyWiredEarlierChargeDependenciesAreRefused(t *testing.T) {
	base := newEstimatedChargeFixture(t, nil)
	charges, err := application.NewFormSellCustomerChargeHandler(&formationEvaluationDouble{adoption: base.adoption}, base.charges)
	if err != nil {
		t.Fatalf("形成本体：%v", err)
	}
	_, err = application.NewFormEstimatedCustomerChargeHandler(application.FormEstimatedCustomerChargeDeps{
		Requests: base.requests, Conclusions: base.requests, Mappings: base.mappings,
		Charges: charges, FollowUp: base.followUp, Clock: fixedClock{at: estimatedChargeAt},
		PriorCharges: newChargeStore(), Supersessions: &supersessionDouble{},
	})
	if !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v，want 缺依赖", err)
	}
}

type reweighFormation struct {
	*estimatedChargeFixture
	supersessions *supersessionDouble
	adjustments   *chargeAdjustmentStoreDouble
	locks         *serializerDouble
	chargeLocks   *chargeLockDouble
}

// stopSecondAndSubmitThird 把第二代改成停下（没有费用），再起第三代作为这一次要形成的那一代。
func (fixture *reweighFormation) stopSecondAndSubmitThird(t *testing.T, cause domain.EvaluationStopCause) {
	t.Helper()
	index := fixture.tenant.String() + "/SELLREQ-SYN-2"
	second := fixture.requests.records[index]
	second.State = domain.SellRequestPendingJudgment
	second.StopCause = cause
	fixture.requests.records[index] = second
	third := fixture.resumeAs(t, second.Request, "SELLREQ-SYN-3")
	fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-3"] = ports.SellEvaluationRequestRecord{
		Key:     ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: third.ID()},
		Request: third,
		State:   domain.SellRequestSubmitted,
	}
	fixture.adoption.Request = third.ID()
}

// formThirdAhead 让第三代先于第二代记下已形成，交回它记下的费用：原费用已确认时是那笔已确认费用，未确认时是第三代
// 自己的预估。第二代仍是已提交，是这一次要形成的那一代。
func (fixture *reweighFormation) formThirdAhead(t *testing.T, stage domain.ChargeStage) domain.CustomerChargeID {
	t.Helper()
	second := fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-2"]
	third := fixture.resumeAs(t, second.Request, "SELLREQ-SYN-3")
	formed := fixture.requests.records[fixture.tenant.String()+"/"+sellRequestID].FormedCharge
	if stage != domain.ChargeConfirmed {
		formed = third.EstimatedCustomerChargeID()
	}
	fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-3"] = ports.SellEvaluationRequestRecord{
		Key:          ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: third.ID()},
		Request:      third,
		State:        domain.SellRequestFormed,
		FormedCharge: formed,
	}
	return formed
}

func (fixture *reweighFormation) resumeAs(t *testing.T, from domain.SellEvaluationRequest, id string) domain.SellEvaluationRequest {
	t.Helper()
	basis := mustAccountValue(t, domain.NewMeasurementChangeResumeBasis, "measure-"+id)
	next, err := from.Resume(mustAccountValue(t, domain.NewSellEvaluationRequestID, id), basis)
	if err != nil {
		t.Fatalf("下一代 %s：%v", id, err)
	}
	return next
}

func newReweighFormation(t *testing.T, stage domain.ChargeStage, nextMinor int64, adopted bool) *reweighFormation {
	t.Helper()
	return newReweighFormationWith(t, stage, nextMinor, adopted, nil)
}

// newReweighFormationWith 在装配形成编排之前让 rewire 改一遍整组依赖：要在包裹锁、费用行锁与读费用之间记先后的用例，
// 把这几只依赖换成记先后的包装。
func newReweighFormationWith(
	t *testing.T,
	stage domain.ChargeStage,
	nextMinor int64,
	adopted bool,
	rewire func(*application.FormEstimatedCustomerChargeDeps),
) *reweighFormation {
	t.Helper()
	base := newEstimatedChargeFixture(t, map[string]string{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT",
	})
	first := base.requests.records[base.tenant.String()+"/"+sellRequestID].Request
	basis := mustAccountValue(t, domain.NewMeasurementChangeResumeBasis, "measure-1")
	next, err := first.Resume(mustAccountValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2"), basis)
	if err != nil {
		t.Fatalf("下一代：%v", err)
	}
	oldID := first.EstimatedCustomerChargeID()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID: oldID, FeeItem: mustFeeItem("SYN-FEE-FREIGHT"),
		Evaluation:         mustAccountValue(t, domain.NewSellEvaluationReference, "SYN-EVAL-SELL-0"),
		OriginalCurrency:   base.adoption.OriginalCurrency,
		OriginalMinor:      12000,
		SettlementCurrency: base.adoption.SettlementCurrency,
		SettlementMinor:    12000,
		Stage:              domain.ChargeEstimated,
		FormedAt:           estimatedChargeAt.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("旧费用：%v", err)
	}
	if stage == domain.ChargeConfirmed {
		charge, err = charge.Confirm(
			registeredFacts(t),
			mustAccountValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-1"),
			estimatedChargeAt.Add(-time.Hour),
		)
		if err != nil {
			t.Fatalf("确认旧费用：%v", err)
		}
	}
	prior := newChargeStore()
	prior.charges[oldID.String()] = charge
	base.requests.records[base.tenant.String()+"/"+sellRequestID] = ports.SellEvaluationRequestRecord{
		Key:          ports.SellEvaluationRequestKey{TenantID: base.tenant, Request: first.ID()},
		Request:      first,
		State:        domain.SellRequestFormed,
		FormedCharge: oldID,
	}
	base.requests.records[base.tenant.String()+"/"+next.ID().String()] = ports.SellEvaluationRequestRecord{
		Key:     ports.SellEvaluationRequestKey{TenantID: base.tenant, Request: next.ID()},
		Request: next,
		State:   domain.SellRequestSubmitted,
	}
	base.adoption.Request = next.ID()
	base.adoption.OriginalMinor = nextMinor
	base.adoption.SettlementMinor = nextMinor
	supersessions := &supersessionDouble{}
	adjustments := newChargeAdjustmentStore()
	locks := &serializerDouble{}
	chargeLocks := &chargeLockDouble{}
	reweigh := application.NewRecordChargeAdjustmentHandler(application.RecordChargeAdjustmentDeps{
		Charges: prior, Adjustments: adjustments, Clock: fixedClock{at: estimatedChargeAt},
	})
	charges, err := application.NewFormSellCustomerChargeHandler(&formationEvaluationDouble{adoption: base.adoption}, base.charges)
	if err != nil {
		t.Fatalf("形成本体：%v", err)
	}
	deps := application.FormEstimatedCustomerChargeDeps{
		Requests: base.requests, Conclusions: base.requests, Mappings: base.mappings,
		Charges: charges, FollowUp: base.followUp, Clock: fixedClock{at: estimatedChargeAt},
		PriorCharges: prior, Supersessions: supersessions,
		ReweighAdoptions: reweighAdoptionDouble{adopted: adopted}, Reweigh: reweigh,
		Successors: base.requests, Serializer: locks, ChargeLock: chargeLocks,
	}
	if rewire != nil {
		rewire(&deps)
	}
	handler, err := application.NewFormEstimatedCustomerChargeHandler(deps)
	if err != nil {
		t.Fatalf("形成外层：%v", err)
	}
	base.handler = handler
	return &reweighFormation{
		estimatedChargeFixture: base, supersessions: supersessions, adjustments: adjustments, locks: locks, chargeLocks: chargeLocks,
	}
}
