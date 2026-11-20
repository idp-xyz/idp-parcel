package parcelpricing_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// evaluationsByIDDouble 是评价登记册按一批标识读的替身：记下被问的次数与标识，交回预置的评价或故障。
type evaluationsByIDDouble struct {
	evaluations []ppdomain.PricingEvaluation
	err         error
	asked       []string
	calls       int
}

func (double *evaluationsByIDDouble) FindByIDs(
	_ context.Context, _ ppdomain.TenantID, ids []ppdomain.EvaluationID,
) ([]ppdomain.PricingEvaluation, error) {
	double.calls++
	for _, id := range ids {
		double.asked = append(double.asked, id.String())
	}
	return double.evaluations, double.err
}

// halfKilogramCard 造一张按 0.5 千克向上进位的 SELL 卡。实重 2.1 的计价重量是 2.5：两个数不同，读口把哪一格放错了
// 位置测得出来。
func halfKilogramCard(t *testing.T) ppdomain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-SELL-CARD-HALF-KG", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-SELL-TABLE-HALF-KG", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-SELL-WEIGHT-HALF-KG", "v1"),
		Scope:                 "scope-1",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "100",
		WeightRounding:      ppdomain.RoundingCeiling,
		WeightStepKilograms: "0.5",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
}

// measuredInput 造一份带着一条测量事实的快照：实重、可缺席的尺寸，事实的种类说它出自实测还是申报。
func measuredInput(
	t *testing.T,
	kilogramsRaw string,
	sides *ppdomain.Dimensions,
	kind ppdomain.ArtifactKind,
	version string,
) ppdomain.PricingInputSnapshot {
	t.Helper()
	subject := must[ppdomain.EvaluationSubject](t)(ppdomain.NewAcceptedPackageSubject(
		must[ppdomain.PackageID](t)(ppdomain.NewPackageID("pkg-1"))))
	fact := must[ppdomain.VersionedFactReference](t)(ppdomain.NewVersionedFactReference(
		must[ppdomain.VersionReference](t)(ppdomain.NewVersionReferenceIdentity(kind, "pkg-1", version))))
	return must[ppdomain.PricingInputSnapshot](t)(ppdomain.NewPricingInputSnapshot(
		must[ppdomain.TenantID](t)(ppdomain.NewTenantID("tenant-1")),
		must[ppdomain.PricingScopeID](t)(ppdomain.NewPricingScopeID("scope-1")),
		subject, "Z1", kilograms(t, kilogramsRaw), sides, basisAt, fact))
}

func loadOrigins(t *testing.T, double *evaluationsByIDDouble, ids ...string) map[string]adapter.PricedWeightAndOrigin {
	t.Helper()
	origins := must[*adapter.PricedWeightsAndOrigins](t)(adapter.NewPricedWeightsAndOrigins(double))
	return must[map[string]adapter.PricedWeightAndOrigin](t)(origins.PricedWeightsAndOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")),
		adapter.MustTranslateEvaluationRefs(ids), nil))
}

func loadBoth(
	t *testing.T,
	double *evaluationsByIDDouble,
	strict adapter.MustTranslateEvaluationRefs,
	lenient adapter.AbsentIfUntranslatableEvaluationRefs,
) (map[string]adapter.PricedWeightAndOrigin, error) {
	t.Helper()
	origins := must[*adapter.PricedWeightsAndOrigins](t)(adapter.NewPricedWeightsAndOrigins(double))
	return origins.PricedWeightsAndOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")), strict, lenient)
}

// Covers: 费用册上的计价重量是评价进位之后、据以查表的那一个，不是实重；来源一格另答评价用的实重与尺寸，以及这两样
// 出自实测还是申报。尺寸三边由长到短。申报没报尺寸时尺寸缺席；评价没有测量事实时来源整格缺席，计价重量照答。
func TestThePricedWeightAndOriginAnswersThePricedWeightAndWhatItCameFrom(t *testing.T) {
	card := halfKilogramCard(t)
	remeasured := must[ppdomain.Dimensions](t)(ppdomain.NewDimensions(
		decimal(t, "25"), decimal(t, "30"), decimal(t, "20"), ppdomain.LengthUnitCentimeter))
	double := &evaluationsByIDDouble{evaluations: []ppdomain.PricingEvaluation{
		evaluate(t, "eval-measured", card, measuredInput(t, "2.10", &remeasured, ppdomain.ArtifactActualMeasurement, "measure-2")),
		evaluate(t, "eval-declared", card, measuredInput(t, "1.20", nil, ppdomain.ArtifactDeclaredMeasurement, "declaration")),
		evaluate(t, "eval-bare", card, packageInput(t, "tenant-1", "Z1")),
	}}
	for _, evaluation := range double.evaluations {
		if evaluation.Status() != ppdomain.EvaluationCompleted {
			t.Fatalf("夹具 %s 状态 = %s %#v", evaluation.ID(), evaluation.Status(), evaluation.Issues())
		}
	}

	origins := loadOrigins(t, double, "eval-measured", "", "eval-declared", "eval-measured", "eval-bare", "eval-unknown")

	want := map[string]adapter.PricedWeightAndOrigin{
		"eval-measured": {
			Kind: "actual-measurement", PricedWeight: "2.5", PricedWeightUnit: "KG",
			ActualWeight: "2.1", ActualWeightUnit: "KG",
			DimensionSides: []string{"30", "25", "20"}, DimensionUnit: "CM",
			MeasurementSource: "measure-2",
		},
		"eval-declared": {
			Kind: "declared-measurement", PricedWeight: "1.5", PricedWeightUnit: "KG",
			ActualWeight: "1.2", ActualWeightUnit: "KG",
		},
		"eval-bare": {PricedWeight: "5", PricedWeightUnit: "KG"},
	}
	if !reflect.DeepEqual(origins, want) {
		t.Fatalf("来源 = %+v\nwant %+v", origins, want)
	}
	if asked := []string{"eval-measured", "eval-declared", "eval-bare", "eval-unknown"}; !reflect.DeepEqual(double.asked, asked) || double.calls != 1 {
		t.Fatalf("问登记册 %d 次、标识 = %v，want 一次且去掉空的与重复的 %v", double.calls, double.asked, asked)
	}
}

// Covers: 登记册读不回时上抛，不答成「这些费用都没有计价重量」。
func TestAnUnreadableEvaluationRegisterIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	down := errors.New("evaluation register unreachable")
	origins := must[*adapter.PricedWeightsAndOrigins](t)(adapter.NewPricedWeightsAndOrigins(&evaluationsByIDDouble{err: down}))
	answered, err := origins.PricedWeightsAndOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")),
		adapter.MustTranslateEvaluationRefs{"eval-measured"}, nil)
	if !errors.Is(err, down) || answered != nil {
		t.Fatalf("answered = %v err = %v，want 上抛", answered, err)
	}
}

// Covers: 费用行上的评价引用只来自计价评价登记册里找得到的评价（形成费用的唯一路径先按评价标识读回评价才记下它），所以引用在计价
// 一侧造不出评价标识，是数据坏了，不是「这笔费用没有计价重量」。读口报错并点名那个引用，不再跳过它答一份少一笔的结果；不去问登记册，
// 前面读得到的几笔也不交回。
func TestAChargeEvaluationReferenceThePricingSideCannotBuildIsAnErrorNamingIt(t *testing.T) {
	double := &evaluationsByIDDouble{}
	origins := must[*adapter.PricedWeightsAndOrigins](t)(adapter.NewPricedWeightsAndOrigins(double))

	answered, err := origins.PricedWeightsAndOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")),
		adapter.MustTranslateEvaluationRefs{"eval-measured", " eval-padded ", "eval-declared"}, nil)

	if !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("err = %v，want 以 ErrUntranslatableReference 报错", err)
	}
	if !strings.Contains(err.Error(), `" eval-padded "`) {
		t.Fatalf("err = %q，want 点名坏的那个引用", err)
	}
	if answered != nil || len(double.asked) != 0 {
		t.Fatalf("answered = %v asked = %v，want 不交回任何一笔也不去问登记册", answered, double.asked)
	}
}

// Covers: 从宽的一组是调整上的评价引用。造不出标识的那一个缺席，其余照常交回，登记册里没有的评价也只是缺席。
// 同一条译得出的引用两组都有时只问登记册一次。出自恰好一条实际测量的评价带出那次测量的来源身份。
func TestUntranslatableAdjustmentReferencesAreAbsentWhileTheRestAreReturnedInOneRead(t *testing.T) {
	card := halfKilogramCard(t)
	remeasured := must[ppdomain.Dimensions](t)(ppdomain.NewDimensions(
		decimal(t, "25"), decimal(t, "30"), decimal(t, "20"), ppdomain.LengthUnitCentimeter))
	double := &evaluationsByIDDouble{evaluations: []ppdomain.PricingEvaluation{
		evaluate(t, "eval-measured", card, measuredInput(t, "2.10", &remeasured, ppdomain.ArtifactActualMeasurement, "measure-2")),
		evaluate(t, "eval-declared", card, measuredInput(t, "1.20", nil, ppdomain.ArtifactDeclaredMeasurement, "declaration")),
	}}

	answered, err := loadBoth(t, double,
		adapter.MustTranslateEvaluationRefs{"eval-measured"},
		adapter.AbsentIfUntranslatableEvaluationRefs{"eval-measured", " eval-padded ", "eval-declared", "eval-unknown"},
	)
	if err != nil {
		t.Fatalf("从宽的一组造不出标识不该报错：%v", err)
	}
	if _, present := answered[" eval-padded "]; present {
		t.Fatal("造不出标识的调整评价不该有重量与来源")
	}
	if _, present := answered["eval-unknown"]; present {
		t.Fatal("登记册里没有的评价不该有重量与来源")
	}
	measured, declared := answered["eval-measured"], answered["eval-declared"]
	if measured.PricedWeight != "2.5" || measured.MeasurementSource != "measure-2" || measured.Kind != "actual-measurement" {
		t.Fatalf("从严的那笔 = %+v，want 计价重量 2.5、实测来源身份 measure-2", measured)
	}
	if declared.PricedWeight != "1.5" || declared.Kind != "declared-measurement" || declared.MeasurementSource != "" {
		t.Fatalf("申报那笔 = %+v，want 计价重量 1.5、来源身份缺席", declared)
	}
	if double.calls != 1 || !reflect.DeepEqual(double.asked, []string{"eval-measured", "eval-declared", "eval-unknown"}) {
		t.Fatalf("问登记册 %d 次、标识 = %v，want 一次且不重复已在从严组里的", double.calls, double.asked)
	}
}

// Covers: 同一条造不出标识的引用既在从严组又在从宽组时，从严赢：报 ErrUntranslatableReference 并点名，整批不交回，不去问登记册。
func TestAnUntranslatableReferenceInBothGroupsFailsTheStrictGroup(t *testing.T) {
	double := &evaluationsByIDDouble{}
	answered, err := loadBoth(t, double,
		adapter.MustTranslateEvaluationRefs{"eval-measured", " eval-padded "},
		adapter.AbsentIfUntranslatableEvaluationRefs{" eval-padded ", "eval-declared"},
	)
	if !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("err = %v，want 以 ErrUntranslatableReference 报错", err)
	}
	if !strings.Contains(err.Error(), `" eval-padded "`) {
		t.Fatalf("err = %q，want 点名坏的那个引用", err)
	}
	if answered != nil || double.calls != 0 {
		t.Fatalf("answered = %v calls = %d，want 不交回也不去问登记册", answered, double.calls)
	}
}

func TestThePricedWeightsAndOriginsRefuseAMissingRegister(t *testing.T) {
	if _, err := adapter.NewPricedWeightsAndOrigins(nil); err == nil {
		t.Fatal("没有评价登记册也构造成了")
	}
}
