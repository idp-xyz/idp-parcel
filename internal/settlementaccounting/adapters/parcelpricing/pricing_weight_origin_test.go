package parcelpricing_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// evaluationsByIDDouble 是评价登记册按一批标识读的替身：记下被问的标识，交回预置的评价或故障。
type evaluationsByIDDouble struct {
	evaluations []ppdomain.PricingEvaluation
	err         error
	asked       []string
}

func (double *evaluationsByIDDouble) FindByIDs(
	_ context.Context, _ ppdomain.TenantID, ids []ppdomain.EvaluationID,
) ([]ppdomain.PricingEvaluation, error) {
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

func loadOrigins(t *testing.T, double *evaluationsByIDDouble, ids ...string) map[string]adapter.PricingWeightOrigin {
	t.Helper()
	origins := must[*adapter.PricingWeightOrigins](t)(adapter.NewPricingWeightOrigins(double))
	return must[map[string]adapter.PricingWeightOrigin](t)(origins.PricingWeightOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")), ids))
}

// Covers: 费用册上的计价重量是评价进位之后、据以查表的那一个，不是实重；来源一格另答评价用的实重与尺寸，以及这两样
// 出自实测还是申报。尺寸三边由长到短。申报没报尺寸时尺寸缺席；评价没有测量事实时来源整格缺席，计价重量照答。
func TestThePricingWeightOriginAnswersThePricedWeightAndWhatItCameFrom(t *testing.T) {
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

	want := map[string]adapter.PricingWeightOrigin{
		"eval-measured": {
			Kind: "actual-measurement", PricedWeight: "2.5", PricedWeightUnit: "KG",
			ActualWeight: "2.1", ActualWeightUnit: "KG",
			DimensionSides: []string{"30", "25", "20"}, DimensionUnit: "CM",
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
	if asked := []string{"eval-measured", "eval-declared", "eval-bare", "eval-unknown"}; !reflect.DeepEqual(double.asked, asked) {
		t.Fatalf("问登记册的标识 = %v，want 去掉空的与重复的 %v", double.asked, asked)
	}
}

// Covers: 登记册读不回时上抛，不答成「这些费用都没有计价重量」。
func TestAnUnreadableEvaluationRegisterIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	down := errors.New("evaluation register unreachable")
	origins := must[*adapter.PricingWeightOrigins](t)(adapter.NewPricingWeightOrigins(&evaluationsByIDDouble{err: down}))
	answered, err := origins.PricingWeightOrigins(
		context.Background(), must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1")), []string{"eval-measured"})
	if !errors.Is(err, down) || answered != nil {
		t.Fatalf("answered = %v err = %v，want 上抛", answered, err)
	}
}

func TestThePricingWeightOriginsRefuseAMissingRegister(t *testing.T) {
	if _, err := adapter.NewPricingWeightOrigins(nil); err == nil {
		t.Fatal("没有评价登记册也构造成了")
	}
}
