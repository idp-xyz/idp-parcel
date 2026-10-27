package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件对真实 PostgreSQL 证复核给候选出价的包裹事实依据的留痕（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经
// 取得的物理事实，到站复核使用当前有效物理实测」）：复核记录上一格、逐段成本出处里一格；没出过价的记录读回为空，出处缺这一格
// 读回为客户声明（依据出现之前落库的出处，包裹事实一律取客户声明）。

// Covers: 复核记录往返带依据——退回声明的、并用的、定不下的各一格；改善依据里被替代那一侧的出处也带依据往返。
func TestTheCandidateFactsBasisRoundTripsOnTheReassessmentRecord(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	for _, basis := range []domain.ParcelFactsBasis{
		domain.ParcelFactsPhysical,
		domain.ParcelFactsPhysicalWithDeclaration,
		domain.ParcelFactsDeclarationFallback,
		domain.ParcelFactsPhysicalUnresolved,
		domain.ParcelFactsNone,
	} {
		correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-"+basis.String())
		record := reroutedRecord(t, key, correlation)
		record.CandidateFactsBasis = basis
		saveReassessment(t, transactor, ctx, reassessments, correlation, record)

		found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
		if err != nil || !present {
			t.Fatalf("%s 按关联读回：present=%v err=%v", basis, present, err)
		}
		if found.CandidateFactsBasis != basis {
			t.Fatalf("依据往返 = %s，want %s", found.CandidateFactsBasis, basis)
		}
	}

	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-improve-basis")
	saveReassessment(t, transactor, ctx, reassessments, correlation, improvementReroutedRecord(t, key, correlation))
	found, _, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil {
		t.Fatal(err)
	}
	if cited := found.ImprovementBasis.SupersededCostCitations(); len(cited) != 1 || cited[0].FactsBasis() != domain.ParcelFactsDeclarationFallback {
		t.Fatalf("被替代那一侧的出处依据往返 = %+v，want DECLARATION_FALLBACK", cited)
	}
}

// Covers: 没向取数侧要过成本的复核不记依据——读回为空，不被填成某一格。
func TestARecordThatNeverPricedCandidatesReadsBackWithoutABasis(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-never-priced")
	saveReassessment(t, transactor, ctx, reassessments, correlation, reroutedRecord(t, key, correlation))

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if found.CandidateFactsBasis != domain.ParcelFactsBasisInvalid {
		t.Fatalf("依据 = %s，want 空", found.CandidateFactsBasis)
	}
	var stored *string
	if err := pool.QueryRow(ctx,
		`SELECT candidate_facts_basis FROM network_routing.route_reassessment WHERE correlation_id = $1`,
		correlation.String()).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("库里 = %v、err = %v，want NULL", stored, err)
	}
}

// Covers: 出处缺依据那一格读回为客户声明——依据出现之前落库的出处，包裹事实一律取客户声明，读回不另造「未知」。
func TestACitationStoredBeforeTheBasisExistedReadsBackAsTheDeclaration(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-legacy-citation")
	saveReassessment(t, transactor, ctx, reassessments, correlation, improvementReroutedRecord(t, key, correlation))
	if _, err := pool.Exec(ctx,
		`UPDATE network_routing.route_reassessment
		    SET improvement_basis = jsonb_set(improvement_basis, '{superseded_cost_citations}',
		        (SELECT jsonb_agg(citation - 'facts_basis')
		           FROM jsonb_array_elements(improvement_basis -> 'superseded_cost_citations') AS citation))
		  WHERE correlation_id = $1`, correlation.String()); err != nil {
		t.Fatalf("抹掉依据那一格：%v", err)
	}

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if cited := found.ImprovementBasis.SupersededCostCitations(); len(cited) != 1 || cited[0].FactsBasis() != domain.ParcelFactsDeclaration {
		t.Fatalf("缺依据的出处读回 = %+v，want 客户声明", cited)
	}
}

// Covers: 库面钉住复核记录的依据是封闭集，且不含 DECLARATION——那是预路由取客户声明的依据，复核在任何判断点上都不按它出价；
// 认不出的文本也拒。正例同形状只换依据那一格，证明拒的是这一格。
func TestTheCandidateFactsBasisColumnIsAClosedSetWithoutThePreRouteBasis(t *testing.T) {
	_, _, _, pool := newRouteStores(t)
	ctx := t.Context()

	insert := func(correlation string, basis *string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO network_routing.route_reassessment
				(tenant_id, correlation_id, customer_account_id, shipment_request_id,
				 acceptance_baseline, declared_parcel_id, service_purpose,
				 conclusion, reviewed_plan, lapse_basis, candidate_state, candidate_facts_basis,
				 trigger_occurred_at, reassessed_at)
			 VALUES ('tenant-1', $1, 'customer-a', 'request-1',
			         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
			         'PLAN_LAPSED', 'RPV-0001', 'closure-7', 'NO_QUALIFIED_CANDIDATES', $2,
			         now(), now())`, correlation, basis)
		return err
	}
	text := func(value string) *string { return &value }

	for name, basis := range map[string]*string{
		"退回声明": text("DECLARATION_FALLBACK"), "实测": text("PHYSICAL"), "并用": text("PHYSICAL_WITH_DECLARATION"),
		"定不下": text("PHYSICAL_UNRESOLVED"), "没有事实": text("NONE"), "没出过价": nil,
	} {
		if err := insert("ok-"+name, basis); err != nil {
			t.Fatalf("%s 应落得了库：%v", name, err)
		}
	}
	for name, basis := range map[string]string{"预路由的依据": "DECLARATION", "认不出": "LAST_WEIGHED", "空串": ""} {
		if err := insert("bad-"+name, text(basis)); err == nil {
			t.Fatalf("%s 进了复核库", name)
		}
	}
}
