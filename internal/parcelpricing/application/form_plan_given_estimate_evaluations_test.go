package application_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证「对调用方给的价卡形成试算评价」的入口（票 product-strategy-boundary/24；ADR-0152 决定二、三、五）：卡由调用方给，
// 入口不选卡；逐件按卡的目录绑定造输入、与正式评价共用补齐读数、纯函数形成评价；证据层级由调用方声明；不入册、不交付。

// planGivenFuelSeries 取演示售价卡 SYN-PLAN-CN-SG-01 绑的那条燃油序列的标识：票面「现象」点名的正是这一格。
const planGivenFuelSeries = "SYN-SERIES-FUEL-01"

// planGivenFormedAt 是补齐读数那一刻的时钟，刻意与计价基准时点不同：形成时刻定序列版本、基准时点定期次（ADR-0099 决定四）。
var planGivenFormedAt = time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)

// fuelBoundPlan 是 minimalPlan 加一条燃油附加费：按基数百分比计收，百分比取绑定的燃油序列、乘卡上系数 1，基数是基础运费。
// 1 千克一件即基础运费 10 美元、燃油费 10 美元的当期百分比。
func fuelBoundPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	fuel := mustValue(t, domain.NewChargeCode, "FUEL_SURCHARGE")
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThan, mustWeight(t, "0"))
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	calculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, mustValue(t, domain.ParseDecimal, "1"), "dep-fuel")
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule("fuel", fuel, "fuel surcharge", domain.ChargeEffectAdd, trigger, calculation)
	if err == nil {
		rule, err = rule.Standalone()
	}
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	dependency, err := domain.NewListedChargeDependency("dep-fuel", fuel, []domain.ChargeCode{base.BaseChargeCode()}, nil)
	if err != nil {
		t.Fatalf("dependency: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, planGivenFuelSeries)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{rule}, []domain.ChargeDependency{dependency}, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func fuelSeriesVersion(t *testing.T, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, planGivenFuelSeries, version, "sha256:syn-fuel-"+version)
	if err != nil {
		t.Fatalf("fuel reference: %v", err)
	}
	return reference
}

// fuelSeriesReading 造一期已解析的燃油费率：登记一版自年初起不封口的单期序列，再按计价基准时点解析。
func fuelSeriesReading(t *testing.T, version, rate string) domain.ResolvedSeriesReading {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
		mustValue(t, domain.ParseDecimal, rate), "SYN-EVIDENCE/fuel")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        fuelSeriesVersion(t, version),
		SourceIdentifier: "SYN-CARRIER/fuel",
		Registrant:       "SYN-OPS",
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	reading, found := registration.ResolveAt(estimateBasisAt)
	if !found {
		t.Fatal("fixture period does not cover the pricing basis time")
	}
	return reading
}

// fuelCompletion 是燃油序列那一对补齐口：在用版本 v1，其中一期覆盖计价基准时点、费率 20（百分比）。
func fuelCompletion(t *testing.T) (*inForceDouble, *seriesReadingDouble) {
	t.Helper()
	return &inForceDouble{reference: fuelSeriesVersion(t, "v1"), outcome: ports.SeriesVersionInForce},
		&seriesReadingDouble{readings: map[string]domain.ResolvedSeriesReading{"v1": fuelSeriesReading(t, "v1", "20")}}
}

// inForceUnreachableAfterFirst 头一问照内层替身答，此后读不回：一次调用里头一件补齐得了，后面几件补不了。
type inForceUnreachableAfterFirst struct{ *inForceDouble }

func (double inForceUnreachableAfterFirst) ResolveInForce(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceSeriesKind,
	seriesID string,
	at time.Time,
) (domain.VersionReference, ports.InForceOutcome, error) {
	if double.calls > 0 {
		double.err = errors.New("register unreachable")
	}
	return double.inForceDouble.ResolveInForce(ctx, tenant, kind, seriesID, at)
}

// planGivenDeclaration 是一件 1 千克、给了分区 Z1、没给尺寸、路线与结算币种的声明。
func planGivenDeclaration(t *testing.T, subject string) application.PlanGivenEstimateDeclaration {
	t.Helper()
	return application.PlanGivenEstimateDeclaration{
		Subject: subject,
		BasisAt: estimateBasisAt,
		Weight:  mustWeight(t, "1"),
		Zone:    "Z1",
	}
}

// planGivenSides 是一件 20 × 15 × 10 厘米的包裹。
func planGivenSides(t *testing.T) domain.Dimensions {
	t.Helper()
	sides, err := domain.NewDimensions(mustValue(t, domain.ParseDecimal, "20"), mustValue(t, domain.ParseDecimal, "15"),
		mustValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	return sides
}

// fuelDeclaration 是燃油那几条用例的声明，比 planGivenDeclaration 多给尺寸：卡上有附加费时纯函数要从输入派生包裹特征，缺尺寸
// 就落「特征不可用」的待判断，燃油取值补没补上也就看不出来了。
func fuelDeclaration(t *testing.T, subject string) application.PlanGivenEstimateDeclaration {
	t.Helper()
	declaration := planGivenDeclaration(t, subject)
	sides := planGivenSides(t)
	declaration.Dimensions = &sides
	return declaration
}

func planGivenCommand(
	t *testing.T,
	plan domain.PricingPlanVersion,
	declarations ...application.PlanGivenEstimateDeclaration,
) application.FormPlanGivenEstimateEvaluationsCommand {
	t.Helper()
	return application.FormPlanGivenEstimateEvaluationsCommand{
		Tenant:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Plan:         plan,
		Evidence:     domain.EvidenceSynthetic,
		Declarations: declarations,
	}
}

func newPlanGivenHandler(
	t *testing.T,
	deps application.FormPlanGivenEstimateEvaluationsDeps,
) *application.FormPlanGivenEstimateEvaluationsHandler {
	t.Helper()
	if deps.Clock == nil {
		deps.Clock = fixedClock{at: planGivenFormedAt}
	}
	handler, err := application.NewFormPlanGivenEstimateEvaluationsHandler(deps)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

// 判据一前半：绑燃油序列的卡，序列在用且基准时点有期次——经共用补齐读数取到取值，评价完成，合计含燃油项。纯函数直调时这一格
// 落 PENDING（缺序列取值），本入口要补的正是这一格。
func TestPlanGivenEstimateOnAFuelBoundCardCompletesThroughTheSharedReadingCompletion(t *testing.T) {
	inForce, register := fuelCompletion(t)
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{SeriesInForce: inForce, SeriesVersions: register})

	result, err := handler.Handle(context.Background(), planGivenCommand(t, fuelBoundPlan(t), fuelDeclaration(t, "SYN-DECL-01/1")))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
		t.Fatalf("result = %+v, 想要已形成且一件", result)
	}
	estimate := result.Estimates[0]
	if estimate.Answer != application.EstimateCandidateEvaluated || estimate.Subject != "SYN-DECL-01/1" {
		t.Fatalf("estimate = %+v, 想要这一件已评价", estimate)
	}
	evaluation := estimate.Evaluation
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s issues = %+v, 想要燃油取值补齐后完成", evaluation.Status(), evaluation.Issues())
	}
	total, present := evaluation.Total()
	if !present || total.Amount().String() != "12" || total.Currency().String() != "USD" {
		t.Fatalf("total = %s %s present = %t, 想要 12 USD（基础运费 10 + 燃油 10 × 20%%）", total.Amount(), total.Currency(), present)
	}
	fuel := false
	for _, line := range evaluation.ChargeLines() {
		if line.Code().String() == "FUEL_SURCHARGE" && line.Amount().Amount().String() == "2" {
			fuel = true
		}
	}
	if !fuel {
		t.Fatalf("charge lines = %+v, 想要一条 2 USD 的燃油项", evaluation.ChargeLines())
	}
	if inForce.calls != 1 || !inForce.lastAt.Equal(planGivenFormedAt) || inForce.lastID != planGivenFuelSeries {
		t.Fatalf("in-force calls = %d at = %s id = %s, 想要按形成时刻问一次那条序列", inForce.calls, inForce.lastAt, inForce.lastID)
	}
	if !register.lastAsOf.Equal(estimateBasisAt) {
		t.Fatalf("period resolved at %s, 想要按计价基准时点", register.lastAsOf)
	}
	frozen := false
	for _, reference := range evaluation.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceSeries && reference.ID() == planGivenFuelSeries && reference.Version() == "v1" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("manifest = %+v, 想要燃油序列 v1 冻结进版本清单", evaluation.Manifest().References())
	}
}

// 判据一后半：序列没登记、或登记了没复核——不编造取值，评价落 PENDING，解释里带补齐说明。说明与正式评价同一份：同一组登记册
// 状态、同一形成时刻下，正式评价编排交回的那条说明逐字出现在试算评价的解释里（ADR-0152 决定三：补齐读数只有一处实现）。
func TestPlanGivenEstimateLeavesAnUnresolvedFuelSeriesPendingWithTheFormalEvaluationsNote(t *testing.T) {
	for name, outcome := range map[string]ports.InForceOutcome{
		"没登记": ports.SeriesHasNoRegisteredVersion,
		"没复核": ports.SeriesHasNoApprovedVersion,
	} {
		t.Run(name, func(t *testing.T) {
			inForce, register := fuelCompletion(t)
			inForce.outcome = outcome
			handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{SeriesInForce: inForce, SeriesVersions: register})

			result, err := handler.Handle(context.Background(), planGivenCommand(t, fuelBoundPlan(t), fuelDeclaration(t, "SYN-DECL-01/1")))
			if err != nil || result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
				t.Fatalf("result = %+v err = %v, 想要已形成且一件", result, err)
			}
			evaluation := result.Estimates[0].Evaluation
			if issues := evaluation.Issues(); evaluation.Status() != domain.EvaluationPending ||
				len(issues) != 1 || issues[0].Code() != "REFERENCE_SERIES_UNRESOLVED" {
				t.Fatalf("status = %s issues = %+v, 想要因缺燃油取值而 PENDING", evaluation.Status(), issues)
			}

			formal := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
				Store:          &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
				Downstream:     &evaluationDownstreamDouble{},
				Clock:          fixedClock{at: planGivenFormedAt},
				InForce:        inForce,
				SeriesVersions: register,
			})
			subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
			if err != nil {
				t.Fatalf("subject: %v", err)
			}
			sides := planGivenSides(t)
			input, err := domain.NewPricingInputSnapshot(mustValue(t, domain.NewTenantID, "tenant-1"),
				mustValue(t, domain.NewPricingScopeID, "scope-1"), subject, "Z1", mustWeight(t, "1"), &sides, estimateBasisAt)
			if err != nil {
				t.Fatalf("input: %v", err)
			}
			request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-formal-fuel"), fuelBoundPlan(t), input, domain.EvidenceSynthetic)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			recorded, err := formal.Handle(context.Background(), application.EvaluatePricingCommand{Request: request})
			if err != nil || recorded.Outcome() != application.EvaluationRecorded {
				t.Fatalf("formal outcome = %s err = %v", recorded.Outcome(), err)
			}
			notes := recorded.SeriesResolutionNotes()
			if len(notes) != 1 {
				t.Fatalf("formal notes = %#v, 想要正式评价留一条说明", notes)
			}
			if !slices.Contains(evaluation.Explanation(), notes[0]) {
				t.Fatalf("explanation = %#v, 想要逐字含正式评价的说明 %q", evaluation.Explanation(), notes[0])
			}
		})
	}
}

// 判据二：绑分区目录的卡——给了邮编路线的那一件由共用补齐读数按路线从目录解出分区；只给分区、不给路线的那一件单独答输入不全、
// 点名邮编路线，不拿声明的分区顶替（ADR-0152 决定五）。逐件作答，按声明顺序交回；缺项那一件一口登记册都不问。
func TestPlanGivenEstimateOnAZoneBoundCardAnswersEachDeclarationByItsOwnRoute(t *testing.T) {
	catalogueInForce := &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
	catalogues := &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": zoneChartVersion(t, "v2")}}
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{CatalogueInForce: catalogueInForce, Catalogues: catalogues})
	route, err := domain.NewPostalRoute("200000", "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	routed := planGivenDeclaration(t, "SYN-DECL-01/1")
	routed.Zone = ""
	routed.Route = &route
	zoneOnly := planGivenDeclaration(t, "SYN-DECL-01/2")

	result, err := handler.Handle(context.Background(), planGivenCommand(t, catalogueBoundPlan(t), routed, zoneOnly))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 2 {
		t.Fatalf("result = %+v, 想要已形成且两件", result)
	}
	first := result.Estimates[0]
	if first.Subject != "SYN-DECL-01/1" || first.Answer != application.EstimateCandidateEvaluated ||
		first.Evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("给了路线的那一件 = %+v issues = %+v, 想要分区经目录解析后完成", first, first.Evaluation.Issues())
	}
	if catalogueInForce.calls != 1 || catalogues.lastRoute != route {
		t.Fatalf("in-force calls = %d route = %+v, 想要只为给了路线的那一件按声明的路线解一次", catalogueInForce.calls, catalogues.lastRoute)
	}
	second := result.Estimates[1]
	if second.Subject != "SYN-DECL-01/2" || second.Answer != application.EstimateCandidateInputIncomplete ||
		!slices.Equal(second.Missing, []string{application.EstimateMissingPostalRoute}) {
		t.Fatalf("只给分区的那一件 = %+v, 想要输入不全、点名邮编路线", second)
	}
}

// 反向：没绑分区目录的卡读声明的分区；那一件没给分区就点名分区，不给默认分区。
func TestPlanGivenEstimateOnAnUnboundCardNamesAMissingZone(t *testing.T) {
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})
	unzoned := planGivenDeclaration(t, "SYN-DECL-01/1")
	unzoned.Zone = ""

	result, err := handler.Handle(context.Background(), planGivenCommand(t, minimalPlan(t), unzoned))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
		t.Fatalf("result = %+v, 想要已形成且一件", result)
	}
	if got := result.Estimates[0]; got.Answer != application.EstimateCandidateInputIncomplete ||
		!slices.Equal(got.Missing, []string{application.EstimateMissingZone}) {
		t.Fatalf("estimate = %+v, 想要输入不全、点名分区", got)
	}
}

// 判据三前半：标识由调用方给的试算对象引用与声明确定性派生——同一声明同一张卡调两次，得同一标识与同一结果；评价对象就是调用方
// 给的那个引用。换引用、换一格声明、换一张卡，标识各自不同。
func TestPlanGivenEstimateIdentityIsDeterministicInTheDeclarationAndTheCard(t *testing.T) {
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})
	form := func(plan domain.PricingPlanVersion, declaration application.PlanGivenEstimateDeclaration) domain.PricingEvaluation {
		t.Helper()
		result, err := handler.Handle(context.Background(), planGivenCommand(t, plan, declaration))
		if err != nil || result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 ||
			result.Estimates[0].Answer != application.EstimateCandidateEvaluated {
			t.Fatalf("result = %+v err = %v, 想要已形成且这一件已评价", result, err)
		}
		return result.Estimates[0].Evaluation
	}

	declaration := planGivenDeclaration(t, "SYN-DECL-01/1")
	first, second := form(minimalPlan(t), declaration), form(minimalPlan(t), declaration)
	if first.ID() != second.ID() || first.SemanticDigest() != second.SemanticDigest() {
		t.Fatalf("ids %s / %s digests %s / %s, 想要同声明同卡同标识同摘要", first.ID(), second.ID(), first.SemanticDigest(), second.SemanticDigest())
	}
	subject, present := first.Input().Subject()
	if !present || subject.Kind() != domain.SubjectEstimate || subject.Reference() != "SYN-DECL-01/1" {
		t.Fatalf("subject = %+v, 想要调用方给的那个试算对象引用", subject)
	}

	heavier := declaration
	heavier.Weight = mustWeight(t, "2")
	for name, evaluation := range map[string]domain.PricingEvaluation{
		"换引用":  form(minimalPlan(t), planGivenDeclaration(t, "SYN-DECL-01/2")),
		"换实重":  form(minimalPlan(t), heavier),
		"换一张卡": form(planWithIdentity(t, minimalPlan(t), "plan-2"), declaration),
	} {
		if evaluation.ID() == first.ID() {
			t.Fatalf("%s仍是同一标识 %s", name, first.ID())
		}
	}
}

// 判据三后半（ADR-0152 决定二的手法）：依赖里没有评价库写口与交付口，装配点装不进去——结构上不入册、不交付。
func TestPlanGivenEstimateDependenciesCarryNoEvaluationStoreOrHandoff(t *testing.T) {
	store := reflect.TypeOf((*ports.EvaluationStore)(nil)).Elem()
	handoff := reflect.TypeOf((*ports.EvaluationHandoff)(nil)).Elem()
	deps := reflect.TypeOf(application.FormPlanGivenEstimateEvaluationsDeps{})
	for index := 0; index < deps.NumField(); index++ {
		field := deps.Field(index)
		if field.Type == store || field.Type == handoff {
			t.Fatalf("依赖 %s 的类型是 %s——试算不得有评价库写口或交付口", field.Name, field.Type)
		}
		if field.Type.Kind() == reflect.Interface && (field.Type.Implements(store) || field.Type.Implements(handoff)) {
			t.Fatalf("依赖 %s 的接口 %s 带着评价库或交付的方法集", field.Name, field.Type)
		}
	}
}

// 证据层级由调用方声明、原样落在评价上：受理前估价的输入是客户申报，不是假设包裹，UC-PP-001 那一格「一律 S」不扩用到这里。
func TestPlanGivenEstimateCarriesTheEvidenceTheCallerDeclared(t *testing.T) {
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})
	for _, evidence := range []domain.EvidenceKind{domain.EvidenceSynthetic, domain.EvidenceReplay, domain.EvidenceProduction} {
		command := planGivenCommand(t, minimalPlan(t), planGivenDeclaration(t, "SYN-DECL-01/1"))
		command.Evidence = evidence

		result, err := handler.Handle(context.Background(), command)
		if err != nil || result.Outcome != application.PlanGivenEstimatesFormed || len(result.Estimates) != 1 {
			t.Fatalf("%s: result = %+v err = %v", evidence, result, err)
		}
		if got := result.Estimates[0].Evaluation.Evidence(); got != evidence {
			t.Fatalf("evidence = %s, 想要调用方声明的 %s", got, evidence)
		}
	}
}

// 未受理：命令不成形即拒，一口补齐读数都不问。证据层级没声明也算不成形——不给默认；同一试算对象引用在一次调用里出现两次也算——
// 试算对象只在该次试算内有身份，两件同名就分不清哪份评价是哪一件的。
func TestPlanGivenEstimateRefusesAMalformedCommandBeforeAskingAnyRegister(t *testing.T) {
	wellFormed := func(t *testing.T) (application.FormPlanGivenEstimateEvaluationsCommand, *application.FormPlanGivenEstimateEvaluationsHandler, *inForceDouble) {
		t.Helper()
		inForce, register := fuelCompletion(t)
		handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{SeriesInForce: inForce, SeriesVersions: register})
		command := planGivenCommand(t, fuelBoundPlan(t), fuelDeclaration(t, "SYN-DECL-01/1"), fuelDeclaration(t, "SYN-DECL-01/2"))
		return command, handler, inForce
	}
	command, handler, inForce := wellFormed(t)
	if result, err := handler.Handle(context.Background(), command); err != nil ||
		result.Outcome != application.PlanGivenEstimatesFormed || inForce.calls == 0 {
		t.Fatalf("未改动的命令 = %+v err = %v, 想要它本身受理——否则下面各格拒得再整齐也证明不了什么", result, err)
	}

	for name, mutate := range map[string]func(*application.FormPlanGivenEstimateEvaluationsCommand){
		"缺租户": func(command *application.FormPlanGivenEstimateEvaluationsCommand) { command.Tenant = domain.TenantID{} },
		"缺价卡": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Plan = domain.PricingPlanVersion{}
		},
		"没声明证据层级":    func(command *application.FormPlanGivenEstimateEvaluationsCommand) { command.Evidence = "" },
		"证据层级不在封闭集里": func(command *application.FormPlanGivenEstimateEvaluationsCommand) { command.Evidence = "X" },
		"一件声明都没有":    func(command *application.FormPlanGivenEstimateEvaluationsCommand) { command.Declarations = nil },
		"引用为空": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[0].Subject = ""
		},
		"引用两侧带空白": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[0].Subject = " SYN-DECL-01/1"
		},
		"基准时点为零": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[0].BasisAt = time.Time{}
		},
		"没给实重": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[0].Weight = domain.Weight{}
		},
		"分区两侧带空白": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[0].Zone = " Z1"
		},
		"引用重复": func(command *application.FormPlanGivenEstimateEvaluationsCommand) {
			command.Declarations[1].Subject = command.Declarations[0].Subject
		},
	} {
		t.Run(name, func(t *testing.T) {
			command, handler, inForce := wellFormed(t)
			mutate(&command)

			result, err := handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.PlanGivenEstimatesNotAccepted || len(result.Estimates) != 0 || inForce.calls != 0 {
				t.Fatalf("result = %+v in-force calls = %d, 想要未受理且一口都不问", result, inForce.calls)
			}
		})
	}
}

// 未决：补齐读数的依赖读不回，形成与否未知——整份答未决，一件都不交回，也不把一次故障记成一次待判断。
func TestPlanGivenEstimateIsUndecidedWhenTheReadingCompletionCannotRead(t *testing.T) {
	inForce, register := fuelCompletion(t)
	inForce.err = errors.New("register unreachable")
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{SeriesInForce: inForce, SeriesVersions: register})

	result, err := handler.Handle(context.Background(), planGivenCommand(t, fuelBoundPlan(t), fuelDeclaration(t, "SYN-DECL-01/1")))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PlanGivenEstimatesUndecided || len(result.Estimates) != 0 {
		t.Fatalf("result = %+v, 想要未决且一件都不交回", result)
	}
}

// 判断项「整份答 `UNDECIDED`，不交回其余几件」：头一件补齐读数读得回、评价已形成，第二件读不回——整份答未决，已形成的那一件
// 也不交回。TestPlanGivenEstimateIsUndecidedWhenTheReadingCompletionCannotRead 只有一件声明，钉不住这一格：没有其余几件可交。
func TestPlanGivenEstimateIsUndecidedAsAWholeWhenALaterDeclarationCannotRead(t *testing.T) {
	inForce, register := fuelCompletion(t)
	handler := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{
		SeriesInForce: inForceUnreachableAfterFirst{inForce}, SeriesVersions: register,
	})

	result, err := handler.Handle(context.Background(), planGivenCommand(t, fuelBoundPlan(t),
		fuelDeclaration(t, "SYN-DECL-01/1"), fuelDeclaration(t, "SYN-DECL-01/2")))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if inForce.calls != 2 {
		t.Fatalf("in-force calls = %d, 想要两件各问一次：头一件读得回，第二件读不回", inForce.calls)
	}
	if result.Outcome != application.PlanGivenEstimatesUndecided || len(result.Estimates) != 0 {
		t.Fatalf("result = %+v, 想要整份未决，已形成的头一件也不交回", result)
	}
}

// 构造期拒法：缺时钟、补齐读数某一对只装一半，都在构造时报出来；两对都不装是合法装配，与正式评价同形。
func TestPlanGivenEstimateHandlerRefusesIncompleteWiring(t *testing.T) {
	clock := fixedClock{at: planGivenFormedAt}
	if _, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺时钟：err = %v, 想要 ErrNilDependency", err)
	}
	if _, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{
		Clock: clock, SeriesInForce: &inForceDouble{},
	}); !errors.Is(err, application.ErrSeriesResolutionHalfWired) {
		t.Fatalf("序列只装一半：err = %v, 想要 ErrSeriesResolutionHalfWired", err)
	}
	if _, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{
		Clock: clock, Catalogues: &catalogueReadingDouble{},
	}); !errors.Is(err, application.ErrCatalogueResolutionHalfWired) {
		t.Fatalf("目录只装一半：err = %v, 想要 ErrCatalogueResolutionHalfWired", err)
	}
	if _, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{Clock: clock}); err != nil {
		t.Fatalf("只装时钟被拒：%v", err)
	}
}
