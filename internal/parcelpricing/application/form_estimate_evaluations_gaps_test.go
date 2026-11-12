package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证输入完备性门上的两个新缺项码（ADR-0381 决定二）：ORIGIN_POSTAL——卡绑的在用分区目录按始发邮编前缀
// 分表、路线给了而始发没报；DIMENSIONS——卡的评价要读包裹尺寸而尺寸没报。两个都是「客户补得上、在输入门上点名」
// 的缺项；「报了但解不出」照旧走评价落待判断那一路，不归缺项。

// prefixZoneChart 造一版按始发邮编前缀分表的分区目录：目的前缀 902 → Z1，始发覆盖给定前缀集。
func prefixZoneChart(t *testing.T, version string, prefixes ...string) domain.ReferenceCatalogueRegistration {
	t.Helper()
	period, err := domain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	entry, err := domain.NewCatalogueEntry("902", "Z1")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	origin, err := domain.NewPostalPrefixCatalogueOrigin(prefixes)
	if err != nil {
		t.Fatalf("origin: %v", err)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.CatalogueKindZone,
		Reference:        catalogueVersion(t, version),
		SourceIdentifier: "SYN-CARRIER/zone-chart-by-origin",
		Registrant:       "SYN-OPS",
		Origin:           origin,
		PrefixLength:     3,
		Entries:          []domain.CatalogueEntry{entry},
		Period:           period,
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

// originGateHandler 装一个只有一张绑分区目录的卡的试算入口，目录那一对按给定替身接。
func originGateHandler(
	t *testing.T,
	inForce *catalogueInForceDouble,
	register *catalogueReadingDouble,
) *application.FormEstimateEvaluationsHandler {
	t.Helper()
	handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
		PriceCards:       &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{catalogueBoundPlan(t)}},
		InForce:          &priceCardResolverDouble{},
		Clock:            fixedClock{at: estimateBasisAt},
		CatalogueInForce: inForce,
		Catalogues:       register,
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

// routedCommand 是一份带路线的试算声明：始发按给定，目的 90210（目录里 902 → Z1）。
func routedCommand(t *testing.T, origin string) application.FormEstimateEvaluationsCommand {
	t.Helper()
	route, err := domain.NewPostalRoute(origin, "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	command := estimateCommand(t)
	command.Zone = ""
	command.Route = &route
	return command
}

// Covers: ADR-0381 决定二——ORIGIN_POSTAL 只在「在用分区目录按始发邮编前缀分表、路线给了而始发没报」时点名。
// 独立成表的目录不要始发；没有在用版本时该名哪一格说不准，照旧走解不出那一路；报了始发而前缀不覆盖是
// 「报了但解不出」，同样照旧——缺项码只说「没报」，不替目录覆盖面作答。
func TestEstimateNamesOriginPostalOnlyForAPrefixScopedCatalogue(t *testing.T) {
	cases := map[string]struct {
		inForce     *catalogueInForceDouble
		chart       domain.ReferenceCatalogueRegistration
		origin      string
		wantAnswer  application.EstimateCandidateAnswer
		wantMissing []string
		wantStatus  domain.EvaluationStatus
	}{
		"前缀分表且始发没报点名": {
			chart:       prefixZoneChart(t, "v2", "940"),
			origin:      "",
			wantAnswer:  application.EstimateCandidateInputIncomplete,
			wantMissing: []string{application.EstimateMissingOriginPostal},
		},
		"独立成表不要始发照常完成": {
			chart:      zoneChartVersion(t, "v2"),
			origin:     "",
			wantAnswer: application.EstimateCandidateEvaluated,
			wantStatus: domain.EvaluationCompleted,
		},
		"无在用版本不点名照旧解不出": {
			inForce:    &catalogueInForceDouble{outcome: ports.CatalogueHasNoRegisteredVersion},
			chart:      prefixZoneChart(t, "v2", "940"),
			origin:     "",
			wantAnswer: application.EstimateCandidateEvaluated,
			wantStatus: domain.EvaluationPending,
		},
		"报了始发而前缀不覆盖照旧解不出": {
			chart:      prefixZoneChart(t, "v2", "940"),
			origin:     "200000",
			wantAnswer: application.EstimateCandidateEvaluated,
			wantStatus: domain.EvaluationPending,
		},
		"报了始发且前缀覆盖照常完成": {
			chart:      prefixZoneChart(t, "v2", "940"),
			origin:     "940112",
			wantAnswer: application.EstimateCandidateEvaluated,
			wantStatus: domain.EvaluationCompleted,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inForce := tc.inForce
			if inForce == nil {
				inForce = &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
			}
			register := &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": tc.chart}}

			result, err := originGateHandler(t, inForce, register).Handle(context.Background(), routedCommand(t, tc.origin))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
				t.Fatalf("result = %+v, 想要已形成且一格", result)
			}
			candidate := result.Candidates[0]
			if candidate.Answer != tc.wantAnswer || !slices.Equal(candidate.Missing, tc.wantMissing) {
				t.Fatalf("candidate = %+v, 想要 %s 且缺项 %v", candidate, tc.wantAnswer, tc.wantMissing)
			}
			if tc.wantAnswer != application.EstimateCandidateEvaluated {
				return
			}
			evaluation := candidate.Evaluation
			if evaluation.Status() != tc.wantStatus {
				t.Fatalf("status = %s issues = %+v, 想要 %s", evaluation.Status(), evaluation.Issues(), tc.wantStatus)
			}
			if tc.wantStatus == domain.EvaluationPending && evaluation.Issues()[0].Code() != "ZONE_UNRESOLVED" {
				t.Fatalf("issues = %+v, 想要照旧落 ZONE_UNRESOLVED", evaluation.Issues())
			}
		})
	}
}

// Covers: 「要不要点名缺始发邮编」读不回与补齐读数读不回同一格——整份答未决、停在补齐读数那一口，不点下去
// 也不放过去（ADR-0381 决定二的依赖故障半边）。在用解析与整版读回两口各试一次。
func TestEstimateIsUndecidedWhenTheOriginScopeCannotBeRead(t *testing.T) {
	for name, arrange := range map[string]func(*catalogueInForceDouble, *catalogueReadingDouble){
		"在用解析读不回": func(inForce *catalogueInForceDouble, _ *catalogueReadingDouble) {
			inForce.err = errors.New("register unreachable")
		},
		"整版读回读不回": func(_ *catalogueInForceDouble, register *catalogueReadingDouble) {
			register.err = errors.New("register unreachable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			inForce := &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
			register := &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": prefixZoneChart(t, "v2", "940")}}
			arrange(inForce, register)

			result, err := originGateHandler(t, inForce, register).Handle(context.Background(), routedCommand(t, ""))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.EstimateUndecided || result.Reason != application.EstimateReadingCompletionUnavailable {
				t.Fatalf("result = %+v, 想要未决、停在补齐读数", result)
			}
		})
	}
}

// maxWeightEstimatePlan 是 minimalPlan 换实重与体积重取较大者的重量策略：体积系数 5000（立方厘米折千克）、
// 体积重按 0.1 千克向上进位。这张卡的评价非算体积重不可，缺尺寸该在输入门上点名。
func maxWeightEstimatePlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling,
		mustWeight(t, "0.1"))
	if err != nil {
		t.Fatalf("factor rounding: %v", err)
	}
	factor, err := domain.NewVolumetricFactor(mustValue(t, domain.ParseDecimal, "5000"), domain.LengthUnitCentimeter, factorRounding)
	if err != nil {
		t.Fatalf("factor: %v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactWeightPolicy, "weight-max", "v1", "sha256:syn-weight-max")
	if err != nil {
		t.Fatalf("weight reference: %v", err)
	}
	policy, err := domain.NewPricingWeightPolicy(reference, domain.PricingWeightMax, base.WeightPolicy().Rounding(), &factor)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), policy, nil, domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// Covers: ADR-0381 决定二——重量策略取较大者的卡缺尺寸即点名 DIMENSIONS；尺寸报了照常完成；只读实重的卡
// 不要尺寸。判据本身（RequiresDeclaredDimensions 的两处）在领域层另证，这里证编排把它接在输入门上。
func TestEstimateNamesDimensionsForACardThatReadsThem(t *testing.T) {
	t.Run("取较大者的卡缺尺寸点名", func(t *testing.T) {
		handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{maxWeightEstimatePlan(t)}})
		result, err := handler.Handle(context.Background(), estimateCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
			t.Fatalf("result = %+v, 想要已形成且一格", result)
		}
		got := result.Candidates[0]
		if got.Answer != application.EstimateCandidateInputIncomplete ||
			!slices.Equal(got.Missing, []string{application.EstimateMissingDimensions}) {
			t.Fatalf("candidate = %+v, 想要输入不全、点名尺寸", got)
		}
	})

	t.Run("尺寸报了照常完成", func(t *testing.T) {
		handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{maxWeightEstimatePlan(t)}})
		sides, err := domain.NewDimensions(mustValue(t, domain.ParseDecimal, "20"), mustValue(t, domain.ParseDecimal, "15"),
			mustValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
		if err != nil {
			t.Fatalf("dimensions: %v", err)
		}
		command := estimateCommand(t)
		command.Dimensions = &sides

		result, err := handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		got := result.Candidates[0]
		if got.Answer != application.EstimateCandidateEvaluated || got.Evaluation.Status() != domain.EvaluationCompleted {
			t.Fatalf("candidate = %+v issues = %+v, 想要照常完成", got, got.Evaluation.Issues())
		}
	})

	t.Run("只读实重的卡不要尺寸", func(t *testing.T) {
		handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}})
		result, err := handler.Handle(context.Background(), estimateCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if got := result.Candidates[0]; got.Answer != application.EstimateCandidateEvaluated {
			t.Fatalf("candidate = %+v, 想要照常评价", got)
		}
	})
}

// Covers: 给定价卡的入口（受理前估价走的那条）与 UC-PP-001 共用同一道输入完备性门：同一张卡、同一种缺项，
// 逐件点名 ORIGIN_POSTAL 与 DIMENSIONS。
func TestPlanGivenEstimateNamesTheNewGapsPerDeclaration(t *testing.T) {
	t.Run("前缀分表且始发没报", func(t *testing.T) {
		inForce := &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
		register := &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": prefixZoneChart(t, "v2", "940")}}
		handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{CatalogueInForce: inForce, Catalogues: register})
		route, err := domain.NewPostalRoute("", "90210")
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		declaration := planGivenDeclaration(t, "SYN-DECL-01/1")
		declaration.Zone = ""
		declaration.Route = &route

		result, err := handler.Handle(context.Background(), planGivenCommand(t, catalogueBoundPlan(t), declaration))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
			t.Fatalf("result = %+v, 想要已形成且一件", result)
		}
		if got := result.Estimates[0]; got.Answer != application.EstimateCandidateInputIncomplete ||
			!slices.Equal(got.Missing, []string{application.EstimateMissingOriginPostal}) {
			t.Fatalf("estimate = %+v, 想要输入不全、点名始发邮编", got)
		}
	})

	t.Run("取较大者的卡缺尺寸", func(t *testing.T) {
		handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})

		result, err := handler.Handle(context.Background(), planGivenCommand(t, maxWeightEstimatePlan(t), planGivenDeclaration(t, "SYN-DECL-01/1")))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
			t.Fatalf("result = %+v, 想要已形成且一件", result)
		}
		if got := result.Estimates[0]; got.Answer != application.EstimateCandidateInputIncomplete ||
			!slices.Equal(got.Missing, []string{application.EstimateMissingDimensions}) {
			t.Fatalf("estimate = %+v, 想要输入不全、点名尺寸", got)
		}
	})
}
