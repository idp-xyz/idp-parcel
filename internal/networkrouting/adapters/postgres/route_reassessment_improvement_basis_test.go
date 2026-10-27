package postgres_test

import (
	"fmt"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 证仍适用线上改善改路那一行的落库形状（票 docs-consistency/code/53）：`已改路`的依据是失效依据
// 或改善依据，恰有其一；改善依据整份往返，读回时重走领域构造门。

// improvementReroutedRecord 造一份仍适用线上的`已改路`记录：被复核计划仍可执行，没有失效依据，改善依据点名两份成本、阈值
// 与被替代那一侧的一段出处。
func improvementReroutedRecord(
	t *testing.T,
	key domain.InitialRouteJudgmentKey,
	correlation domain.RequestCorrelationID,
) ports.ReassessmentRecord {
	t.Helper()
	record := reroutedRecord(t, key, correlation)
	record.LapseBasis = domain.ApplicabilityBasisReference{}
	citation, err := domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
		Ordinal: 1, EvaluationID: "EST-RPV-0001-1", PlanReference: "PLAN-COST/v1",
		PolicyReference: "POLICY-1", HasPolicy: true, ComparisonAmount: "42.00", ComparisonCurrency: "CNY", FactsBasis: domain.ParcelFactsDeclarationFallback,
	})
	if err != nil {
		t.Fatalf("段出处：%v", err)
	}
	basis, err := domain.NewCostImprovementBasis(domain.CostImprovementBasisSpec{
		SupersededCostMinor: 4200, SuccessorCostMinor: 3100, Currency: "CNY", ThresholdMinor: 1000,
		SupersededCostCitations: []domain.PlannedLegCostCitation{citation},
	})
	if err != nil {
		t.Fatalf("改善依据：%v", err)
	}
	record.ImprovementBasis = basis
	record.HasImprovementBasis = true
	return record
}

// Covers: AT-NR-085（复核册半边）——冻结未越过、改善超过阈值时那一行`已改路`落得了库：不带失效依据，带改善依据、新计划与
// 自动改路决定，过逐结论 CHECK；读回时改善依据各格与出处不变形。
func TestAnImprovementRerouteRoundTripsWithItsImprovementBasis(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-improve")
	saveReassessment(t, transactor, ctx, reassessments, correlation, improvementReroutedRecord(t, key, correlation))

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if found.Conclusion != ports.ReassessmentRerouted || found.ReviewedPlan.String() != "RPV-0001" ||
		found.LapseBasis.String() != "" || found.RerouteState != domain.AutomaticRerouteAllowed ||
		!found.HasNewPlan || !found.HasDecision {
		t.Fatalf("改善改路往返变形：%+v", found)
	}
	if !found.HasImprovementBasis {
		t.Fatal("改善依据没有随记录往返")
	}
	basis := found.ImprovementBasis
	if basis.SupersededCostMinor() != 4200 || basis.SuccessorCostMinor() != 3100 ||
		basis.Currency() != "CNY" || basis.ThresholdMinor() != 1000 {
		t.Fatalf("改善依据往返变形：%+v", basis)
	}
	cited := basis.SupersededCostCitations()
	if len(cited) != 1 || cited[0].Ordinal() != 1 || cited[0].EvaluationID() != "EST-RPV-0001-1" ||
		cited[0].PlanReference() != "PLAN-COST/v1" || cited[0].ComparisonAmount() != "42.00" ||
		cited[0].ComparisonCurrency() != "CNY" {
		t.Fatalf("被替代那一侧的出处往返变形：%+v", cited)
	}
	if policy, stated := cited[0].PolicyReference(); !stated || policy != "POLICY-1" {
		t.Fatalf("出处的政策引用往返变形：%q %v", policy, stated)
	}
}

// Covers: 失效线改路照旧只带失效依据，读回时没有改善依据。
func TestALapseRerouteCarriesNoImprovementBasis(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-lapse")
	saveReassessment(t, transactor, ctx, reassessments, correlation, reroutedRecord(t, key, correlation))

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if found.LapseBasis.String() != "closure-7" || found.HasImprovementBasis {
		t.Fatalf("失效线改路 = 失效依据 %q、改善依据 %v, want 只有失效依据", found.LapseBasis, found.HasImprovementBasis)
	}
}

// Covers: 库面钉住`已改路`的依据恰有其一——两样都带、两样都缺都拒；改善依据只出现在`已改路`上，且须是对象。正例同形状
// 只换依据那一格，证明拒的是依据那一格，不是别的列。
func TestAReroutedRowCarriesExactlyOneReason(t *testing.T) {
	_, _, _, pool := newRouteStores(t)
	ctx := t.Context()
	const improvement = `'{"superseded_cost_minor":4200,"successor_cost_minor":3100,"currency":"CNY","threshold_minor":1000}'`

	insert := func(correlation, conclusion, lapseBasis, candidateState, rerouteState, newPlan, decision, improvementBasis string) error {
		_, err := pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO network_routing.route_reassessment
				(tenant_id, correlation_id, customer_account_id, shipment_request_id,
				 acceptance_baseline, declared_parcel_id, service_purpose,
				 conclusion, reviewed_plan, lapse_basis, candidate_state,
				 reroute_state, new_plan, decision, improvement_basis, trigger_occurred_at, reassessed_at)
			 VALUES ('tenant-1', '%s', 'customer-a', 'request-1',
			         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
			         '%s', 'RPV-0001', %s, %s,
			         %s, %s, %s, %s, now(), now())`,
			correlation, conclusion, lapseBasis, candidateState, rerouteState, newPlan, decision, improvementBasis))
		return err
	}

	if err := insert("improve-only", "REROUTED", "NULL", "'CANDIDATES_AVAILABLE'",
		"'AUTOMATIC_ALLOWED'", "'{}'", "'{}'", improvement); err != nil {
		t.Fatalf("只带改善依据的`已改路`应落得了库：%v", err)
	}
	if err := insert("lapse-only", "REROUTED", "'closure-7'", "'CANDIDATES_AVAILABLE'",
		"'AUTOMATIC_ALLOWED'", "'{}'", "'{}'", "NULL"); err != nil {
		t.Fatalf("只带失效依据的`已改路`应落得了库：%v", err)
	}
	if err := insert("both", "REROUTED", "'closure-7'", "'CANDIDATES_AVAILABLE'",
		"'AUTOMATIC_ALLOWED'", "'{}'", "'{}'", improvement); err == nil {
		t.Fatal("一行「已改路却同时带失效依据与改善依据」进了复核库")
	}
	if err := insert("neither", "REROUTED", "NULL", "'CANDIDATES_AVAILABLE'",
		"'AUTOMATIC_ALLOWED'", "'{}'", "'{}'", "NULL"); err == nil {
		t.Fatal("一行「已改路却说不出依据」进了复核库")
	}
	if err := insert("array", "REROUTED", "NULL", "'CANDIDATES_AVAILABLE'",
		"'AUTOMATIC_ALLOWED'", "'{}'", "'{}'", "'[]'"); err == nil {
		t.Fatal("一行「改善依据是数组不是对象」按 jsonb 类型缝溜进了复核库")
	}
	if err := insert("lapsed-with-improvement", "PLAN_LAPSED", "'closure-7'", "'NO_QUALIFIED_CANDIDATES'",
		"NULL", "NULL", "NULL", improvement); err == nil {
		t.Fatal("一行「已失效却带改善依据」进了复核库")
	}
}
