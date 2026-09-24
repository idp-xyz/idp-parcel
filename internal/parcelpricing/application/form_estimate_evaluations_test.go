package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证 UC-PP-001「按假设包裹形成试算评价」的编排（ADR-0152）：取全部适用价卡、逐卡按目录绑定选分区给法造试算
// 输入、与正式评价共用补齐读数、纯函数形成评价；不入册、不交付、不择优。

type applicablePriceCardsDouble struct {
	plans []domain.PricingPlanVersion
	err   error
	calls int
}

func (double *applicablePriceCardsDouble) LoadApplicable(
	_ context.Context,
	_ domain.TenantID,
	_ domain.PricingDirection,
	_ domain.PricingScopeID,
	_ time.Time,
) ([]domain.PricingPlanVersion, error) {
	double.calls++
	if double.err != nil {
		return nil, double.err
	}
	return double.plans, nil
}

var estimateBasisAt = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)

func estimateCommand(t *testing.T) application.FormEstimateEvaluationsCommand {
	t.Helper()
	weight, err := domain.NewWeightFromString("1", domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	return application.FormEstimateEvaluationsCommand{
		Tenant:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Scope:     mustValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction: domain.PricingDirectionSell,
		BasisAt:   estimateBasisAt,
		Weight:    weight,
		Zone:      "Z1",
	}
}

func newEstimateHandler(t *testing.T, cards *applicablePriceCardsDouble) *application.FormEstimateEvaluationsHandler {
	t.Helper()
	return newEstimateHandlerWith(t, cards, &priceCardResolverDouble{})
}

func newEstimateHandlerWith(
	t *testing.T,
	cards *applicablePriceCardsDouble,
	resolver *priceCardResolverDouble,
) *application.FormEstimateEvaluationsHandler {
	t.Helper()
	handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
		PriceCards: cards,
		InForce:    resolver,
		Clock:      fixedClock{at: estimateBasisAt},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

// UC-PP-001 验收示例 1：单卡完成——评价对象是试算对象、证据层级按定义是 S、状态完成、版本清单指向那张卡。
func TestEstimateOnASingleCardFormsOneEvaluatedCandidate(t *testing.T) {
	plan := minimalPlan(t)
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{plan}})

	result, err := handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
		t.Fatalf("result = %+v, 想要已形成且一格", result)
	}
	candidate := result.Candidates[0]
	if candidate.Answer != application.EstimateCandidateEvaluated || candidate.Plan != plan.Reference() {
		t.Fatalf("candidate = %+v, 想要这张卡已评价", candidate)
	}
	evaluation := candidate.Evaluation
	if evaluation.Status() != domain.EvaluationCompleted || evaluation.Evidence() != domain.EvidenceSynthetic {
		t.Fatalf("status = %s evidence = %s, 想要 COMPLETED 与 S", evaluation.Status(), evaluation.Evidence())
	}
	subject, present := evaluation.Input().Subject()
	if !present || subject.Kind() != domain.SubjectEstimate {
		t.Fatalf("subject = %+v, 想要试算对象", subject)
	}
	if evaluation.PlanReference() != plan.Reference() {
		t.Fatalf("plan reference = %+v, 想要 %+v", evaluation.PlanReference(), plan.Reference())
	}
}

// UC-PP-001 验收示例 4：没有卡——价卡未配置，不交回任何评价。
func TestEstimateWithoutApplicableCardsIsNotConfigured(t *testing.T) {
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{})

	result, err := handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimatePriceCardNotConfigured || len(result.Candidates) != 0 {
		t.Fatalf("result = %+v, 想要价卡未配置且无候选", result)
	}
}

// UC-PP-001 验收示例 3：缺邮编路线——绑了分区目录的卡输入不全、点名邮编路线；没绑的卡照算。分区给法逐卡定，
// 不拿另一种给法顶替（ADR-0152 决定五）。
func TestEstimateNamesTheMissingInputPerCardAndEvaluatesTheRest(t *testing.T) {
	zoned := minimalPlan(t)
	bound := planWithIdentity(t, catalogueBoundPlan(t), "plan-2")
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{zoned, bound}})

	result, err := handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 2 {
		t.Fatalf("result = %+v, 想要已形成且两格", result)
	}
	if got := result.Candidates[0]; got.Answer != application.EstimateCandidateEvaluated {
		t.Fatalf("未绑目录的卡 = %+v, 想要已评价", got)
	}
	got := result.Candidates[1]
	if got.Answer != application.EstimateCandidateInputIncomplete || len(got.Missing) != 1 || got.Missing[0] != application.EstimateMissingPostalRoute {
		t.Fatalf("绑了分区目录的卡 = %+v, 想要输入不全、点名邮编路线", got)
	}
}

// 反向：没绑目录的卡要调用方给的分区，声明没给即点名分区，不给默认分区。
func TestEstimateWithoutZoneNamesZoneForAnUnboundCard(t *testing.T) {
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}})
	command := estimateCommand(t)
	command.Zone = ""

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
		t.Fatalf("result = %+v, 想要已形成且一格", result)
	}
	got := result.Candidates[0]
	if got.Answer != application.EstimateCandidateInputIncomplete || len(got.Missing) != 1 || got.Missing[0] != application.EstimateMissingZone {
		t.Fatalf("candidate = %+v, 想要输入不全、点名分区", got)
	}
}

// planWithIdentity 把一张卡换一个方案身份重立：多卡用例要两张身份不同的卡。
func planWithIdentity(t *testing.T, base domain.PricingPlanVersion, id string) domain.PricingPlanVersion {
	t.Helper()
	reference, err := domain.NewVersionReference(domain.ArtifactPricingPlan, id, "v1", "sha256:syn-"+id)
	if err != nil {
		t.Fatalf("plan reference: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		reference, base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), base.Rules(), base.Structures(),
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// UC-PP-001 验收示例 5：同一方案两版重叠——装载把它作为登记册数据错误交回，试算按恢复动作译成适用冲突，候选按在用
// 解析口交人裁的本义取回，不评价任何一版（ADR-0152 决定四）。
func TestEstimateOverOverlappingVersionsIsAnApplicabilityConflict(t *testing.T) {
	first := minimalPlan(t).Reference()
	second, err := domain.NewVersionReference(domain.ArtifactPricingPlan, "plan-1", "v2", "sha256:syn-plan-v2")
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	cards := &applicablePriceCardsDouble{err: fmt.Errorf("%w：plan-1", ports.ErrAmbiguousPriceCard)}
	resolver := &priceCardResolverDouble{resolution: ports.PriceCardInForceResolution{
		Outcome:    ports.PriceCardApplicabilityConflict,
		Candidates: []domain.VersionReference{first, second},
	}}

	result, err := newEstimateHandlerWith(t, cards, resolver).Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimatePriceCardApplicabilityConflict || len(result.Candidates) != 0 {
		t.Fatalf("result = %+v, 想要适用冲突且不评价", result)
	}
	if len(result.Conflict) != 2 || result.Conflict[0] != first || result.Conflict[1] != second {
		t.Fatalf("conflict = %+v, 想要两版候选", result.Conflict)
	}
}

// 未决两口分开点名：候选读不回停在取候选那一口；装载读不回停在装载那一口。
func TestEstimateUndecidedNamesWhereItStopped(t *testing.T) {
	ambiguous := &applicablePriceCardsDouble{err: fmt.Errorf("%w：plan-1", ports.ErrAmbiguousPriceCard)}
	result, err := newEstimateHandlerWith(t, ambiguous, &priceCardResolverDouble{err: errors.New("resolver down")}).
		Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateUndecided || result.Reason != application.EstimateConflictCandidatesUnavailable {
		t.Fatalf("result = %+v, 想要未决、停在取候选", result)
	}

	down := &applicablePriceCardsDouble{err: errors.New("catalogue down")}
	result, err = newEstimateHandler(t, down).Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateUndecided || result.Reason != application.EstimatePriceCardLoadUnavailable {
		t.Fatalf("result = %+v, 想要未决、停在装载", result)
	}
}

// UC-PP-001 结果语义「未受理」：声明不成形即拒，一口价卡都不问——落成「未配置」会让人去登记一张本不该存在的卡。
func TestEstimateRefusesAMalformedDeclarationBeforeLoadingCards(t *testing.T) {
	for name, mutate := range map[string]func(*application.FormEstimateEvaluationsCommand){
		"缺范围":     func(command *application.FormEstimateEvaluationsCommand) { command.Scope = domain.PricingScopeID{} },
		"缺租户":     func(command *application.FormEstimateEvaluationsCommand) { command.Tenant = domain.TenantID{} },
		"基准时点为零":  func(command *application.FormEstimateEvaluationsCommand) { command.BasisAt = time.Time{} },
		"方向不认识":   func(command *application.FormEstimateEvaluationsCommand) { command.Direction = "" },
		"没给实重":    func(command *application.FormEstimateEvaluationsCommand) { command.Weight = domain.Weight{} },
		"分区两侧带空白": func(command *application.FormEstimateEvaluationsCommand) { command.Zone = " Z1" },
	} {
		t.Run(name, func(t *testing.T) {
			cards := &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}}
			command := estimateCommand(t)
			mutate(&command)

			result, err := newEstimateHandler(t, cards).Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.EstimateNotAccepted || cards.calls != 0 {
				t.Fatalf("result = %+v calls = %d, 想要未受理且一口价卡都不问", result, cards.calls)
			}
		})
	}
}

// ADR-0152 决定三：绑了分区目录的卡由与正式评价同一件补齐读数按邮编路线解析分区——试算因此与正式评价在同一时点、
// 同一版本清单下给同一结果。发起方另给的分区对这张卡不起作用。
func TestEstimateResolvesTheZoneThroughTheSharedReadingCompletion(t *testing.T) {
	inForce := &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
	register := &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": zoneChartVersion(t, "v2")}}
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
	route, err := domain.NewPostalRoute("200000", "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	command := estimateCommand(t)
	command.Zone = "Z9"
	command.Route = &route

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
		t.Fatalf("result = %+v, 想要已形成且一格", result)
	}
	evaluation := result.Candidates[0].Evaluation
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s issues = %+v, 想要分区经目录解析后完成", evaluation.Status(), evaluation.Issues())
	}
	if inForce.calls != 1 || register.lastRoute != route {
		t.Fatalf("in-force calls = %d route = %+v, 想要经共用件按声明的邮编路线解一次", inForce.calls, register.lastRoute)
	}
}

// ADR-0152 决定五：同声明同卡得同一标识与同一语义摘要；换一格声明即换标识。
func TestEstimateIdentityIsDeterministicInTheDeclaration(t *testing.T) {
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}})
	first, err := handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	a, b := first.Candidates[0].Evaluation, second.Candidates[0].Evaluation
	if a.ID() != b.ID() || a.SemanticDigest() != b.SemanticDigest() {
		t.Fatalf("ids %s / %s digests %s / %s, 想要同声明同标识同摘要", a.ID(), b.ID(), a.SemanticDigest(), b.SemanticDigest())
	}

	heavier := estimateCommand(t)
	heavier.Weight, err = domain.NewWeightFromString("2", domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	third, err := handler.Handle(context.Background(), heavier)
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if third.Candidates[0].Evaluation.ID() == a.ID() {
		t.Fatalf("换了实重仍是同一标识 %s", a.ID())
	}
}

// UC-PP-001 验收示例 6 的结构保证（ADR-0152 决定二）：依赖里没有评价库与交付口，装配点装不进去，结算因此看不见试算。
func TestEstimateDependenciesCarryNoEvaluationStoreOrHandoff(t *testing.T) {
	store := reflect.TypeOf((*ports.EvaluationStore)(nil)).Elem()
	handoff := reflect.TypeOf((*ports.EvaluationHandoff)(nil)).Elem()
	deps := reflect.TypeOf(application.FormEstimateEvaluationsDeps{})
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

// 构造期拒法：缺必备依赖、补齐读数只装一半，都在构造时报出来，而不是等第一次试算。
func TestEstimateHandlerRefusesIncompleteWiring(t *testing.T) {
	complete := application.FormEstimateEvaluationsDeps{
		PriceCards: &applicablePriceCardsDouble{},
		InForce:    &priceCardResolverDouble{},
		Clock:      fixedClock{at: estimateBasisAt},
	}
	for name, deps := range map[string]application.FormEstimateEvaluationsDeps{
		"缺价卡装载":  {InForce: complete.InForce, Clock: complete.Clock},
		"缺在用解析":  {PriceCards: complete.PriceCards, Clock: complete.Clock},
		"缺时钟":    {PriceCards: complete.PriceCards, InForce: complete.InForce},
		"目录只装一半": {PriceCards: complete.PriceCards, InForce: complete.InForce, Clock: complete.Clock, CatalogueInForce: &catalogueInForceDouble{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := application.NewFormEstimateEvaluationsHandler(deps); err == nil {
				t.Fatalf("想要构造期拒")
			}
		})
	}
	if _, err := application.NewFormEstimateEvaluationsHandler(complete); err != nil {
		t.Fatalf("齐全装配被拒：%v", err)
	}
	var half error
	_, half = application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
		PriceCards: complete.PriceCards, InForce: complete.InForce, Clock: complete.Clock, CatalogueInForce: &catalogueInForceDouble{},
	})
	if !errors.Is(half, application.ErrCatalogueResolutionHalfWired) {
		t.Fatalf("err = %v, 想要 ErrCatalogueResolutionHalfWired", half)
	}
}
