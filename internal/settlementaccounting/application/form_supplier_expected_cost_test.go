package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	costOccurredAt = time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	costRecordedAt = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
)

// buyEvaluationViewDouble 是 BUY 评价入向缝的 SA 侧替身：按评价引用答一份采用快照。
type buyEvaluationViewDouble struct {
	adoptions map[string]ports.BuyEvaluationAdoption
	err       error
}

func newBuyEvaluationView() *buyEvaluationViewDouble {
	return &buyEvaluationViewDouble{adoptions: map[string]ports.BuyEvaluationAdoption{}}
}

func (double *buyEvaluationViewDouble) LoadBuyEvaluation(
	_ context.Context,
	_ domain.TenantID,
	evaluation domain.BuyEvaluationReference,
) (ports.BuyEvaluationAdoption, bool, error) {
	if double.err != nil {
		return ports.BuyEvaluationAdoption{}, false, double.err
	}
	adoption, found := double.adoptions[evaluation.String()]
	return adoption, found, nil
}

// expectedCostRegistryDouble 同时演读口与登记面（生产里也是同一个适配器）：主键按版本，
// 首版唯一按（发生项+费用项目+规则版本）三维。
type expectedCostRegistryDouble struct {
	costs   map[string]domain.SupplierExpectedCost
	loadErr error
	saveErr error
	saves   int
}

func newExpectedCostRegistry() *expectedCostRegistryDouble {
	return &expectedCostRegistryDouble{costs: map[string]domain.SupplierExpectedCost{}}
}

func (double *expectedCostRegistryDouble) LoadExpectedCost(
	_ context.Context,
	_ domain.TenantID,
	version domain.SupplierCostVersionID,
) (domain.SupplierExpectedCost, bool, error) {
	if double.loadErr != nil {
		return domain.SupplierExpectedCost{}, false, double.loadErr
	}
	cost, found := double.costs[version.String()]
	return cost, found, nil
}

func (double *expectedCostRegistryDouble) LoadFirstVersion(
	_ context.Context,
	_ domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
	feeItem domain.FeeItemReference,
	ruleVersion domain.PurchaseRuleVersionReference,
) (domain.SupplierExpectedCost, bool, error) {
	if double.loadErr != nil {
		return domain.SupplierExpectedCost{}, false, double.loadErr
	}
	for _, cost := range double.costs {
		if _, corrected := cost.PriorVersion(); corrected {
			continue
		}
		if cost.Occurrence().ID() == occurrence && cost.FeeItem() == feeItem && cost.RuleVersion() == ruleVersion {
			return cost, true, nil
		}
	}
	return domain.SupplierExpectedCost{}, false, nil
}

func (double *expectedCostRegistryDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	cost domain.SupplierExpectedCost,
	_ time.Time,
) (ports.ExpectedCostSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.ExpectedCostSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.costs[cost.Version().String()]; exists {
		return ports.ExpectedCostAlreadyRecorded, nil
	}
	for _, existing := range double.costs {
		if _, corrected := existing.PriorVersion(); corrected {
			continue
		}
		if existing.Occurrence().ID() == cost.Occurrence().ID() && existing.FeeItem() == cost.FeeItem() &&
			existing.RuleVersion() == cost.RuleVersion() {
			return ports.ExpectedCostAlreadyRecorded, nil
		}
	}
	double.costs[cost.Version().String()] = cost
	return ports.ExpectedCostSaved, nil
}

type costClock struct{ at time.Time }

func (clock costClock) Now() time.Time { return clock.at }

type expectedCostFixture struct {
	evaluations *buyEvaluationViewDouble
	registry    *expectedCostRegistryDouble
	handler     *application.FormSupplierExpectedCostHandler
}

func newExpectedCostFixture(t *testing.T) *expectedCostFixture {
	t.Helper()
	fixture := &expectedCostFixture{
		evaluations: newBuyEvaluationView(),
		registry:    newExpectedCostRegistry(),
	}
	fixture.handler = application.NewFormSupplierExpectedCostHandler(application.FormSupplierExpectedCostDeps{
		Evaluations: fixture.evaluations,
		Costs:       fixture.registry,
		Registry:    fixture.registry,
		Clock:       costClock{at: costRecordedAt},
	})
	fixture.evaluations.adoptions["buy-eval-1"] = completedAdoption(t, "buy-eval-1")
	pending := completedAdoption(t, "buy-eval-pending")
	pending.Outcome = ports.BuyEvaluationPending
	fixture.evaluations.adoptions["buy-eval-pending"] = pending
	unratable := completedAdoption(t, "buy-eval-unratable")
	unratable.Outcome = ports.BuyEvaluationUnratable
	fixture.evaluations.adoptions["buy-eval-unratable"] = unratable
	conflict := completedAdoption(t, "buy-eval-conflict")
	conflict.Outcome = ports.BuyEvaluationConflict
	fixture.evaluations.adoptions["buy-eval-conflict"] = conflict
	notFormed := completedAdoption(t, "buy-eval-not-formed")
	notFormed.Outcome = ports.BuyEvaluationNotFormed
	fixture.evaluations.adoptions["buy-eval-not-formed"] = notFormed
	unconverted := completedAdoption(t, "buy-eval-unconverted")
	unconverted.Conversion = domain.ConversionStepReference{}
	fixture.evaluations.adoptions["buy-eval-unconverted"] = unconverted
	return fixture
}

// completedAdoption 造一份 BUY 价卡为美元、合同结算币为人民币、评价内已完成换算的采用快照
// （AT-SA-176 的形状）。金额只是夹具锚点，不是任何价卡的取值。
func completedAdoption(t *testing.T, evaluation string) ports.BuyEvaluationAdoption {
	t.Helper()
	return ports.BuyEvaluationAdoption{
		Evaluation:         billValue(t, domain.NewBuyEvaluationReference, evaluation),
		Outcome:            ports.BuyEvaluationCompleted,
		RuleVersion:        billValue(t, domain.NewPurchaseRuleVersionReference, "buy-card/v3"),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      4200,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    30000,
		Conversion:         billValue(t, domain.NewConversionStepReference, "fx-series/2026-08/step-1"),
	}
}

func expectedCostCommand(t *testing.T, version, evaluation string) application.FormSupplierExpectedCostCommand {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		billValue(t, domain.NewChargeOccurrenceID, "occurrence-7"),
		billValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		billValue(t, domain.NewOccurrenceVersion, "occurrence-7/v1"),
		costOccurredAt,
	)
	if err != nil {
		t.Fatalf("new occurrence: %v", err)
	}
	return application.FormSupplierExpectedCostCommand{
		TenantID:   billValue(t, domain.NewTenantID, "tenant-1"),
		Version:    billValue(t, domain.NewSupplierCostVersionID, version),
		Occurrence: occurrence,
		FeeItem:    billValue(t, domain.NewFeeItemReference, "fee-linehaul"),
		Agreement:  billValue(t, domain.NewSupplierAgreementReference, "agreement-9"),
		Evaluation: billValue(t, domain.NewBuyEvaluationReference, evaluation),
	}
}

// Covers: `AT-SA-161`「外部订舱发生项及采购价格规则有效 → 形成可解释的订舱预期成本，不把
// 订舱确认直接写成账单主张或审核应付」与 `AT-SA-176`「BUY 价卡为美元、合同结算币为人民币，
// 评价已在内部完成换算 → 采用评价内的原币金额、汇率序列版本和换算步骤，不重算」的编排面
// （UC-SA-002 步 5 的 BUY 方向）：首版按版本幂等、同一发生项/费用项目/规则版本只形成一次
// 首版（第二个版本身份交回先到的首版——纠错走 AppendCorrection，不在形成口）。
func TestACompletedBuyEvaluationFormsTheFirstExpectedCostVersion(t *testing.T) {
	fixture := newExpectedCostFixture(t)
	result, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", "buy-eval-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ExpectedCostFormed {
		t.Fatalf("outcome = %q, want EXPECTED_COST_FORMED", result.Outcome())
	}
	cost, present := result.Cost()
	if !present {
		t.Fatal("no cost returned")
	}
	if cost.Version().String() != "cost-7/v1" || cost.Occurrence().ID().String() != "occurrence-7" ||
		cost.FeeItem().String() != "fee-linehaul" || cost.Agreement().String() != "agreement-9" ||
		cost.Evaluation().String() != "buy-eval-1" || cost.RuleVersion().String() != "buy-card/v3" {
		t.Fatalf("预期成本没有把发生项、协议、评价与规则版本整组带上：%+v", cost)
	}
	if currency, minor := cost.OriginalAmount(); currency.String() != "USD" || minor != 4200 {
		t.Fatalf("original = %s %d", currency, minor)
	}
	if currency, minor := cost.SettlementAmount(); currency.String() != "CNY" || minor != 30000 {
		t.Fatalf("settlement = %s %d（结算币金额取自评价内换算，不重算）", currency, minor)
	}
	if conversion, crossCurrency := cost.Conversion(); !crossCurrency || conversion.String() != "fx-series/2026-08/step-1" {
		t.Fatal("换算步骤没有随评价采用进来")
	}
	if _, corrected := cost.PriorVersion(); corrected {
		t.Fatal("首版带了回指")
	}
	if fixture.registry.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.registry.saves)
	}

	t.Run("a replay returns the original version without a second save", func(t *testing.T) {
		replay, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", "buy-eval-1"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.ExpectedCostExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_EXPECTED_COST", replay.Outcome())
		}
		if fixture.registry.saves != 1 {
			t.Fatal("重放重复提交了预期成本")
		}
	})

	t.Run("a different content under the same version identity is a conflict", func(t *testing.T) {
		flipped := expectedCostCommand(t, "cost-7/v1", "buy-eval-1")
		flipped.FeeItem = billValue(t, domain.NewFeeItemReference, "fee-fuel")
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict handle: %v", err)
		}
		if result.Outcome() != application.ExpectedCostConflict {
			t.Fatalf("outcome = %q, want EXPECTED_COST_CONFLICT（AT-SA-051 同一身份不同内容）", result.Outcome())
		}
	})

	t.Run("a second version identity for the same occurrence, fee item and rule is already formed", func(t *testing.T) {
		again, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v9", "buy-eval-1"))
		if err != nil {
			t.Fatalf("second identity: %v", err)
		}
		if again.Outcome() != application.ExpectedCostAlreadyFormed {
			t.Fatalf("outcome = %q, want EXPECTED_COST_ALREADY_FORMED（同一发生项、费用项目和规则版本不得重复形成首版）", again.Outcome())
		}
		cost, _ := again.Cost()
		if cost.Version().String() != "cost-7/v1" {
			t.Fatalf("交回的不是先到的首版：%s", cost.Version())
		}
	})

	t.Run("an unknown evaluation is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-8/v1", "buy-eval-9"))
		if err != nil {
			t.Fatalf("unknown evaluation: %v", err)
		}
		if result.Outcome() != application.ExpectedCostNotAccepted {
			t.Fatalf("outcome = %q（指名了不存在的评价是提交矛盾）", result.Outcome())
		}
	})
}

// Covers: `AT-SA-175`「parcel-pricing 返回不可计价 → 不形成任何费用行（含零金额），不转为待
// 判断反复重试」、`AT-SA-177`「原币与合同结算币不同但评价未携带换算步骤 → 待判断，不自行补
// 算」、`AT-SA-050`「规则或币种版本缺失 → 待判断」与 UC-SA-002 的冲突结束——提供方的四种
// 非完成结果逐格译到 SA 自己的结果格，不互相替代、不折成零金额（PP CONTEXT「四者不得互相
// 替代」）；依赖故障归未决并指名等谁；未决原因集封闭。
func TestNonCompletedEvaluationsDoNotBecomeCosts(t *testing.T) {
	grades := map[string]struct {
		evaluation string
		outcome    application.ExpectedCostOutcome
		reason     application.ExpectedCostUndecidedReason
	}{
		"pending":     {"buy-eval-pending", application.ExpectedCostUndecided, application.EvaluationPending},
		"unratable":   {"buy-eval-unratable", application.ExpectedCostUnratable, application.ExpectedCostUndecidedReasonNone},
		"conflict":    {"buy-eval-conflict", application.EvaluationConflict, application.ExpectedCostUndecidedReasonNone},
		"not formed":  {"buy-eval-not-formed", application.ExpectedCostUndecided, application.EvaluationNotFormed},
		"unconverted": {"buy-eval-unconverted", application.ExpectedCostUndecided, application.ConversionStepMissing},
		"completed":   {"buy-eval-1", application.ExpectedCostFormed, application.ExpectedCostUndecidedReasonNone},
	}
	for name, grade := range grades {
		t.Run(name, func(t *testing.T) {
			fixture := newExpectedCostFixture(t)
			result, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", grade.evaluation))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != grade.outcome || result.UndecidedReason() != grade.reason {
				t.Fatalf("outcome = %q reason = %q, want %q/%q", result.Outcome(), result.UndecidedReason(), grade.outcome, grade.reason)
			}
			formed := grade.outcome == application.ExpectedCostFormed
			if (fixture.registry.saves == 1) != formed || (len(fixture.registry.costs) == 1) != formed {
				t.Fatalf("saves = %d costs = %d（非完成结果不得落成任何成本行，含零金额）", fixture.registry.saves, len(fixture.registry.costs))
			}
		})
	}

	t.Run("dependency failures are undecided with their reasons", func(t *testing.T) {
		fixture := newExpectedCostFixture(t)
		fixture.evaluations.err = errors.New("view down")
		result, err := fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", "buy-eval-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ExpectedCostUndecided || result.UndecidedReason() != application.EvaluationViewUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}

		fixture = newExpectedCostFixture(t)
		fixture.registry.saveErr = errors.New("registry down")
		result, err = fixture.handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", "buy-eval-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ExpectedCostUndecided || result.UndecidedReason() != application.ExpectedCostRegistryUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.ExpectedCostUndecidedReason{
			application.EvaluationViewUnavailable, application.EvaluationPending, application.EvaluationNotFormed,
			application.ConversionStepMissing, application.ExpectedCostRegistryUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.ExpectedCostUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第六个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: SA CONTEXT「本上下文只能消费评价……不能发布、重写或自行替代价卡」的结构面：BUY 评价
// 入向缝在 SA 这一侧只有读法，依赖上没有任何写评价的方法。
func TestTheBuyEvaluationSeamOnlyReads(t *testing.T) {
	field, found := reflect.TypeOf(application.FormSupplierExpectedCostDeps{}).FieldByName("Evaluations")
	if !found {
		t.Fatal("编排没有 BUY 评价读口")
	}
	if field.Type.NumMethod() != 1 || field.Type.Method(0).Name != "LoadBuyEvaluation" {
		t.Fatalf("BUY 评价口的方法集不是只有 LoadBuyEvaluation：%d 个方法", field.Type.NumMethod())
	}
}
