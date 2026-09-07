package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type evaluationStoreDouble struct {
	byID  map[string]domain.PricingEvaluation
	saved int
	err   error
}

func (double *evaluationStoreDouble) FindByID(
	_ context.Context,
	id domain.EvaluationID,
) (domain.PricingEvaluation, bool, error) {
	if double.err != nil {
		return domain.PricingEvaluation{}, false, double.err
	}
	evaluation, found := double.byID[id.String()]
	return evaluation, found, nil
}

func (double *evaluationStoreDouble) Save(
	_ context.Context,
	evaluation domain.PricingEvaluation,
) (ports.EvaluationSaveOutcome, error) {
	if double.err != nil {
		return ports.EvaluationSaveOutcomeInvalid, double.err
	}
	if _, exists := double.byID[evaluation.ID().String()]; exists {
		return ports.EvaluationAlreadyRecorded, nil
	}
	double.byID[evaluation.ID().String()] = evaluation
	double.saved++
	return ports.EvaluationSaved, nil
}

type evaluationDownstreamDouble struct {
	intents []ports.EvaluationHandoffIntent
	err     error
}

func (double *evaluationDownstreamDouble) HandOffEvaluation(
	_ context.Context,
	intent ports.EvaluationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type evaluateFixture struct {
	handler    *application.EvaluatePricingHandler
	store      *evaluationStoreDouble
	downstream *evaluationDownstreamDouble
}

func newEvaluateFixture(t *testing.T) *evaluateFixture {
	t.Helper()
	fixture := &evaluateFixture{
		store:      &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		downstream: &evaluationDownstreamDouble{},
	}
	fixture.handler = application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)},
	})
	return fixture
}

// minimalPlan 造一张最小可评价的合成价卡：单分区重量段费率表 + 实重策略。
func minimalPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return minimalPlanPriced(t, "10")
}

// minimalPlanPriced 是 minimalPlan 的可变金额版本：版本引用一字不改、只换费率金额，造的是「同版本
// 引用背后内容变了」那一格——回放用例要它。
func minimalPlanPriced(t *testing.T, rate string) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	amount, err := domain.NewMoneyFromString(rate, currency)
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	zeroWeight, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "0"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("zero weight: %v", err)
	}
	capWeight, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "10"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("cap weight: %v", err)
	}
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-1"), "Z1", zeroWeight, capWeight, amount)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	period, err := domain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	tableRef, err := domain.NewVersionReference(domain.ArtifactRateTable, "table-1", "v1", "sha256:syn-table")
	if err != nil {
		t.Fatalf("table reference: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		tableRef, domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	stepWeight, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "0.5"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("step weight: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, stepWeight)
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightRef, err := domain.NewVersionReference(domain.ArtifactWeightPolicy, "weight-1", "v1", "sha256:syn-weight")
	if err != nil {
		t.Fatalf("weight reference: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(weightRef, domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	planRef, err := domain.NewVersionReference(domain.ArtifactPricingPlan, "plan-1", "v1", "sha256:syn-plan")
	if err != nil {
		t.Fatalf("plan reference: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		planRef,
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		period,
		table,
		weightPolicy,
		nil,
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func minimalInput(t *testing.T, zone string) domain.PricingInputSnapshot {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	actual, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "5"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("actual weight: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject,
		zone,
		actual,
		nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("input snapshot: %v", err)
	}
	return input
}

func evaluationCommand(t *testing.T, id, zone string) application.EvaluatePricingCommand {
	t.Helper()
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		minimalPlan(t),
		minimalInput(t, zone),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("evaluation request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

// Covers: PP CONTEXT「价格评价」词条经编排端到端——评价入册且意图交付下游（费用采用
// 在 settlement-accounting，本上下文只交结果）；同标识同语义重放返原评价不重算（saves
// 恒一）；同标识异语义是冒名冲突不顶替（版本清单/输入不同的请求装同一个标识）。
func TestEvaluationIsRecordedOnceAndImpostorsConflict(t *testing.T) {
	fixture := newEvaluateFixture(t)

	first, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-1", "Z1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q", first.Outcome())
	}
	evaluation, _ := first.Evaluation()
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %q", evaluation.Status())
	}

	replay, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-1", "Z1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.EvaluationExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 重复请求重算又入册了", fixture.store.saved)
	}

	impostor, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-1", "Z9"))
	if err != nil {
		t.Fatalf("impostor handle: %v", err)
	}
	if impostor.Outcome() != application.EvaluationConflict {
		t.Fatalf("impostor = %q; 同标识装不同输入必须是冲突", impostor.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 冲突顶替了原评价", fixture.store.saved)
	}
}

// Covers: 编排纪律——失败评价同样入册与交付（无法命中费率的评价带着解释入册，不是
// 丢弃品）；意图投递失败评价不翻留续办、重放重发同一份；库读不回未决。
func TestFailedEvaluationsStillRecordAndIntentsRetry(t *testing.T) {
	fixture := newEvaluateFixture(t)

	missedZone, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-miss", "Z9"))
	if err != nil {
		t.Fatalf("missed zone handle: %v", err)
	}
	if missedZone.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q; 失败评价也是版本化结果", missedZone.Outcome())
	}
	missed, _ := missedZone.Evaluation()
	if missed.Status() == domain.EvaluationCompleted {
		t.Fatal("Z9 命不中单分区费率表还答完成")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d; 失败评价没有交付", len(fixture.downstream.intents))
	}

	fixture.downstream.err = errors.New("downstream unreachable")
	held, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-2", "Z1"))
	if err != nil {
		t.Fatalf("held handle: %v", err)
	}
	if held.Outcome() != application.EvaluationRecorded || held.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q; 投递失败评价不翻但要留续办", held.Outcome(), held.HandoffReference())
	}

	fixture.downstream.err = nil
	resent, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-2", "Z1"))
	if err != nil {
		t.Fatalf("resend handle: %v", err)
	}
	if resent.Outcome() != application.EvaluationExistingResult || resent.HandoffReference() != "" {
		t.Fatalf("outcome = %q ref = %q", resent.Outcome(), resent.HandoffReference())
	}
	if len(fixture.downstream.intents) != 2 || fixture.store.saved != 2 {
		t.Fatalf("intents = %d saved = %d", len(fixture.downstream.intents), fixture.store.saved)
	}

	fixture.store.err = errors.New("store unreachable")
	stalled, err := fixture.handler.Handle(context.Background(), evaluationCommand(t, "eval-3", "Z1"))
	if err != nil {
		t.Fatalf("stalled handle: %v", err)
	}
	if stalled.Outcome() != application.EvaluationUndecided {
		t.Fatalf("outcome = %q", stalled.Outcome())
	}
}
