package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证初始路由库与复核库的行为：计划/无路可走整行往返后经
// 领域构造门复验、同键撞写答`已有记录`且事务保持可用（ADR-0031）、复核四走向的在场
// 件矩阵由库内 CHECK 钉住、租户隔离、无事务拒、回滚无痕。

var (
	routeJudgedAt  = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	reassessedAtTS = time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC)
)

func TestAFormedPlanRoundTripsWholly(t *testing.T) {
	routes, _, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	plan := formedPlan(t, key, "RPV-0001")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: plan, HasPlan: true,
	})

	found, present, err := routes.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("按键读回：%v", err)
	}
	if !present || !found.HasPlan || found.HasNoRoute {
		t.Fatalf("读回形状不对：present=%v hasPlan=%v hasNoRoute=%v",
			present, found.HasPlan, found.HasNoRoute)
	}
	got := found.Plan
	if got.Version().String() != "RPV-0001" ||
		got.SelectedCandidate().String() != "candidate-1" ||
		got.Strategy().String() != "strategy/v1" ||
		got.ViewRevision().String() != "netview-42" ||
		!got.JudgedAt().Equal(routeJudgedAt) ||
		!got.EffectiveFrom().Equal(routeJudgedAt.Add(time.Hour)) {
		t.Fatalf("计划头部往返变形：%+v", got)
	}
	candidates := got.Candidates()
	if len(candidates) != 2 ||
		candidates[1].Outcome() != domain.CandidateEliminated ||
		candidates[1].Reason().String() != "customs-restriction" {
		t.Fatal("候选依据（含淘汰理由）没有随计划往返")
	}
	legs := got.Legs()
	if len(legs) != 2 ||
		legs[0].Responsible().String() != "carrier-1" ||
		!legs[1].Opaque() ||
		legs[1].Window().Basis().String() != "calendar/v1" ||
		!legs[0].Window().Earliest().Equal(routeJudgedAt.Add(2*time.Hour)) {
		t.Fatalf("段链往返变形：%+v", legs)
	}
	nodes := got.Nodes()
	if len(nodes) != 3 || nodes[0].String() != "hub-a" || nodes[2].String() != "zone-c" {
		t.Fatalf("节点序列 = %v", nodes)
	}
	if found.RecordedAt.IsZero() {
		t.Fatal("读回缺 RecordedAt——消费方要拿它当接收时间，不得用信封时间顶替")
	}
}

func TestANoRouteJudgmentRoundTripsWholly(t *testing.T) {
	routes, _, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, NoRoute: noRouteJudgment(t, key), HasNoRoute: true,
	})

	found, present, err := routes.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("按键读回：%v", err)
	}
	if !present || !found.HasNoRoute || found.HasPlan {
		t.Fatal("无路可走判断读回形状不对")
	}
	candidates := found.NoRoute.Candidates()
	if len(candidates) != 2 ||
		candidates[0].Reason().String() != "area-excluded" ||
		candidates[1].Reason().String() != "path-closed" {
		t.Fatal("逐候选淘汰依据没有随判断往返（AT-NR-005 的可查半边）")
	}
	if found.NoRoute.Strategy().String() != "strategy/v1" ||
		!found.NoRoute.JudgedAt().Equal(routeJudgedAt) {
		t.Fatal("判断头部往返变形")
	}
}

// TestSavingTheSameKeyTwiceKeepsTheFirstResult 证同键写入代数：第二份结果（哪怕内容
// 相反）答`已有记录`不覆盖，撞键后同一事务立刻可读——编排要在同事务读回赢家作答。
func TestSavingTheSameKeyTwiceKeepsTheFirstResult(t *testing.T) {
	routes, _, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := routes.Save(txCtx, ports.InitialRouteRecord{
			Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
		})
		if err != nil {
			return err
		}
		if outcome != ports.InitialRouteSaved {
			t.Fatalf("first save outcome = %d", outcome)
		}

		outcome, err = routes.Save(txCtx, ports.InitialRouteRecord{
			Key: key, NoRoute: noRouteJudgment(t, key), HasNoRoute: true,
		})
		if err != nil {
			return err
		}
		if outcome != ports.InitialRouteAlreadyRecorded {
			t.Fatalf("second save outcome = %d, want ALREADY_RECORDED", outcome)
		}

		winner, present, err := routes.FindByKey(txCtx, key)
		if err != nil || !present {
			t.Fatalf("撞键后同事务读回失败：present=%v err=%v", present, err)
		}
		if !winner.HasPlan {
			t.Fatal("迟到的无路可走覆盖了先到的计划")
		}
		return nil
	})
}

func TestRouteScopesAreInvisibleToEachOther(t *testing.T) {
	routes, _, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})

	if _, present, err := routes.FindByKey(ctx, routeKey(t, "tenant-b", "parcel-1")); err != nil || present {
		t.Errorf("他租户按同名键读到了本租户的结果：present=%v err=%v", present, err)
	}
}

// TestNoRouteCannotWearAnEmptyPlan 证「无路由不得用空计划表达」两面都钉住：适配器拒
// 两者都带/都缺的记录，数据库 CHECK 拒绕过适配器直插的坏行。
func TestNoRouteCannotWearAnEmptyPlan(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := routes.Save(txCtx, ports.InitialRouteRecord{Key: key})
		return err
	}); err == nil {
		t.Fatal("两者都缺的记录被落库了")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.initial_route
			(tenant_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, plan_version, plan, no_route)
		 VALUES ('tenant-1', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-9', 'LAST_MILE_DELIVERY',
		         'NO_CURRENT_ROUTE', NULL, '{}', '{}')`); err == nil {
		t.Fatal("一行同时带计划与无路可走进了判断库")
	}
}

func TestAReroutedReassessmentRoundTripsWithItsDecision(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-1")
	record := reroutedRecord(t, key, correlation)
	saveReassessment(t, transactor, ctx, reassessments, correlation, record)

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if found.Conclusion != ports.ReassessmentRerouted ||
		found.ReviewedPlan.String() != "RPV-0001" ||
		found.LapseBasis.String() != "closure-7" ||
		found.CandidateState != ports.CandidatesAvailable ||
		found.RerouteState != domain.AutomaticRerouteAllowed {
		t.Fatalf("复核头部往返变形：%+v", found)
	}
	if !found.HasNewPlan || found.NewPlan.Version().String() != "RPV-0002" {
		t.Fatal("新计划没有随记录往返")
	}
	if !found.HasDecision ||
		found.Decision.Mode() != domain.AutomaticReroute ||
		found.Decision.OriginalPlan().String() != "RPV-0001" ||
		found.Decision.EffectiveFromNode().String() != "hub-a" ||
		!found.Decision.DecidedAt().Equal(reassessedAtTS) {
		t.Fatalf("改路决定往返变形：%+v", found.Decision)
	}
	if found.HasSuggestion {
		t.Fatal("自动改路的记录长出了建议")
	}
	if !found.ReassessedAt.Equal(reassessedAtTS) {
		t.Fatalf("reassessedAt = %v", found.ReassessedAt)
	}
}

func TestALapsedReassessmentCarriesItsSuggestion(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-2")
	suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         key,
		Trigger:     scalar(t, domain.NewRerouteTriggerReference, "trigger-2"),
		Candidates:  planCandidates(t),
		Blockers:    []string{"NOT_AT_CONTROLLED_NODE"},
		SuggestedAt: reassessedAtTS,
	})
	if err != nil {
		t.Fatalf("构造建议：%v", err)
	}
	record := ports.ReassessmentRecord{
		Correlation:     correlation,
		Key:             key,
		Conclusion:      ports.ReassessmentPlanLapsed,
		ReviewedPlan:    scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		LapseBasis:      scalar(t, domain.NewApplicabilityBasisReference, "closure-7"),
		CandidateState:  ports.CandidatesAvailable,
		RerouteState:    domain.SuggestionOnly,
		RerouteBlockers: []string{"NOT_AT_CONTROLLED_NODE"},
		Suggestion:      suggestion,
		HasSuggestion:   true,
		ReassessedAt:    reassessedAtTS,
	}
	saveReassessment(t, transactor, ctx, reassessments, correlation, record)

	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation)
	if err != nil || !present {
		t.Fatalf("按关联读回：present=%v err=%v", present, err)
	}
	if found.RerouteState != domain.SuggestionOnly ||
		len(found.RerouteBlockers) != 1 || found.RerouteBlockers[0] != "NOT_AT_CONTROLLED_NODE" {
		t.Fatal("改路判定与阻塞清单往返变形")
	}
	if !found.HasSuggestion ||
		found.Suggestion.Trigger().String() != "trigger-2" ||
		len(found.Suggestion.Blockers()) != 1 ||
		len(found.Suggestion.Candidates()) != 2 {
		t.Fatal("建议（候选+为什么没自动）没有随记录往返")
	}
	if found.HasNewPlan || found.HasDecision {
		t.Fatal("仅建议的记录长出了新计划或决定")
	}
}

// TestReassessmentReplayKeepsTheFirstConclusion 证复核库的写入代数：同关联第二份结论
// 答`已有记录`，读回的仍是第一份——同一触发和输入版本不重复决定。
func TestReassessmentReplayKeepsTheFirstConclusion(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-3")
	first := ports.ReassessmentRecord{
		Correlation:  correlation,
		Key:          key,
		Conclusion:   ports.ReassessmentStillApplicable,
		ReviewedPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		ReassessedAt: reassessedAtTS,
	}
	saveReassessment(t, transactor, ctx, reassessments, correlation, first)

	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := reassessments.Save(txCtx, correlation, reroutedRecord(t, key, correlation))
		if err != nil {
			return err
		}
		if outcome != ports.ReassessmentAlreadyRecorded {
			t.Fatalf("replay outcome = %d, want ALREADY_RECORDED", outcome)
		}
		winner, present, err := reassessments.FindByCorrelation(txCtx, key.TenantID, correlation)
		if err != nil || !present {
			t.Fatalf("撞键后同事务读回失败：present=%v err=%v", present, err)
		}
		if winner.Conclusion != ports.ReassessmentStillApplicable {
			t.Fatal("迟到的结论覆盖了先到者")
		}
		return nil
	})
}

func TestReassessmentScopesAreInvisibleToEachOther(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-4")
	saveReassessment(t, transactor, ctx, reassessments, correlation, ports.ReassessmentRecord{
		Correlation:  correlation,
		Key:          key,
		Conclusion:   ports.ReassessmentStillApplicable,
		ReviewedPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		ReassessedAt: reassessedAtTS,
	})

	other := scalar(t, domain.NewTenantID, "tenant-b")
	if _, present, err := reassessments.FindByCorrelation(ctx, other, correlation); err != nil || present {
		t.Errorf("他租户按同名关联读到了本租户的复核：present=%v err=%v", present, err)
	}
}

// TestConclusionShapeIsPinnedInTheDatabase 证在场件矩阵的库面：绕过适配器直插的坏行
// 被 CHECK 拒。后三支探针钉的是 SQL 三值逻辑的缝——可空列上的等号 / IN /
// jsonb_array_length 在 NULL 上给 NULL，没有显式 IS NOT NULL（或 IS NOT DISTINCT
// FROM）时整条约束会按 NULL 放行。
func TestConclusionShapeIsPinnedInTheDatabase(t *testing.T) {
	_, _, _, pool := newRouteStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, reassessed_at)
		 VALUES ('tenant-1', 'trigger-9', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
		         'STILL_APPLICABLE', 'RPV-0001', 'closure-7', now())`); err == nil {
		t.Fatal("一行「仍适用却带失效依据」进了复核库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state, reassessed_at)
		 VALUES ('tenant-1', 'trigger-10', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
		         'PLAN_LAPSED', 'RPV-0001', 'closure-7', NULL, now())`); err == nil {
		t.Fatal("一行「已失效却没有候选评估状态」按 NULL 溜进了复核库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state,
			 reroute_state, new_plan, decision, reassessed_at)
		 VALUES ('tenant-1', 'trigger-11', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
		         'REROUTED', 'RPV-0001', 'closure-7', 'CANDIDATES_AVAILABLE',
		         NULL, '{}', '{}', now())`); err == nil {
		t.Fatal("一行「已改路却没有改路判定」按 NULL 溜进了复核库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state,
			 reroute_state, reroute_blockers, suggestion, reassessed_at)
		 VALUES ('tenant-1', 'trigger-12', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
		         'PLAN_LAPSED', 'RPV-0001', 'closure-7', 'CANDIDATES_AVAILABLE',
		         'SUGGESTION_ONLY', NULL, '{}', now())`); err == nil {
		t.Fatal("一行「仅建议却没有阻塞清单」按 NULL 溜进了复核库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state,
			 reroute_state, reroute_blockers, suggestion, reassessed_at)
		 VALUES ('tenant-1', 'trigger-13', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY',
		         'PLAN_LAPSED', 'RPV-0001', 'closure-7', 'CANDIDATES_AVAILABLE',
		         'SUGGESTION_ONLY', '{}', '{}', now())`); err == nil {
		t.Fatal("一行「阻塞清单是对象不是数组」按 jsonb 类型缝溜进了复核库")
	}
}

func TestRouteWritesRefuseToRunOutsideATransaction(t *testing.T) {
	routes, reassessments, _, _ := newRouteStores(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	if _, err := routes.Save(ctx, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存路由应返回 ErrTransactionRequired，实得：%v", err)
	}

	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-5")
	if _, err := reassessments.Save(ctx, correlation, ports.ReassessmentRecord{
		Correlation:  correlation,
		Key:          key,
		Conclusion:   ports.ReassessmentStillApplicable,
		ReviewedPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		ReassessedAt: reassessedAtTS,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存复核应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestRouteRollbackLeavesNothingBehind(t *testing.T) {
	routes, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	key := routeKey(t, "tenant-1", "parcel-1")
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-6")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := routes.Save(txCtx, ports.InitialRouteRecord{
			Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
		}); err != nil {
			return err
		}
		if _, err := reassessments.Save(txCtx, correlation, ports.ReassessmentRecord{
			Correlation:  correlation,
			Key:          key,
			Conclusion:   ports.ReassessmentStillApplicable,
			ReviewedPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
			ReassessedAt: reassessedAtTS,
		}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, present, err := routes.FindByKey(ctx, key); err != nil || present {
		t.Errorf("回滚后路由结果仍在：present=%v err=%v", present, err)
	}
	if _, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, correlation); err != nil || present {
		t.Errorf("回滚后复核记录仍在：present=%v err=%v", present, err)
	}
}

// ---- 夹具 ----

func newRouteStores(t *testing.T) (*adapter.InitialRoutes, *adapter.RouteReassessments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	routes, err := adapter.NewInitialRoutes(db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	reassessments, err := adapter.NewRouteReassessments(db)
	if err != nil {
		t.Fatalf("构造复核库：%v", err)
	}
	return routes, reassessments, db.Transactor(), pool
}

func saveRoute(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	routes *adapter.InitialRoutes,
	record ports.InitialRouteRecord,
) {
	t.Helper()
	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := routes.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.InitialRouteSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func saveReassessment(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	reassessments *adapter.RouteReassessments,
	correlation domain.RequestCorrelationID,
	record ports.ReassessmentRecord,
) {
	t.Helper()
	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := reassessments.Save(txCtx, correlation, record)
		if err != nil {
			return err
		}
		if outcome != ports.ReassessmentSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func routeKey(t *testing.T, tenant, parcel string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           scalar(t, domain.NewTenantID, tenant),
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "baseline-v1"),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, parcel),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, "LAST_MILE_DELIVERY"),
	}
}

// planCandidates 造一格合格加一格带理由的淘汰：往返要保住的正是「保留所有候选依据」。
func planCandidates(t *testing.T) []domain.RouteCandidate {
	t.Helper()
	qualified, err := domain.NewRouteCandidate(
		scalar(t, domain.NewCandidateID, "candidate-1"),
		domain.CandidateQualified,
		domain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("构造合格候选：%v", err)
	}
	eliminated, err := domain.NewRouteCandidate(
		scalar(t, domain.NewCandidateID, "candidate-2"),
		domain.CandidateEliminated,
		scalar(t, domain.NewCandidateReason, "customs-restriction"),
	)
	if err != nil {
		t.Fatalf("构造淘汰候选：%v", err)
	}
	return []domain.RouteCandidate{qualified, eliminated}
}

// formedPlan 造一份两段链计划：第二段外部不透明——往返要保住的段链内容一件不少。
func formedPlan(t *testing.T, key domain.InitialRouteJudgmentKey, version string) domain.InitialRoutePlan {
	t.Helper()

	window := func(offset time.Duration) domain.PlannedTimeWindow {
		formed, err := domain.NewPlannedTimeWindow(
			routeJudgedAt.Add(offset), routeJudgedAt.Add(offset+4*time.Hour),
			scalar(t, domain.NewWindowBasisReference, "calendar/v1"))
		if err != nil {
			t.Fatalf("构造窗口：%v", err)
		}
		return formed
	}
	leg := func(from, to, responsible string, offset time.Duration, opaque bool) domain.PlannedLeg {
		formed, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
			From:        scalar(t, domain.NewPlanNodeReference, from),
			To:          scalar(t, domain.NewPlanNodeReference, to),
			Responsible: scalar(t, domain.NewResponsiblePartyReference, responsible),
			Window:      window(offset),
			Opaque:      opaque,
		})
		if err != nil {
			t.Fatalf("构造段：%v", err)
		}
		return formed
	}
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:        key,
		Version:    scalar(t, domain.NewRoutePlanVersionID, version),
		Selected:   scalar(t, domain.NewCandidateID, "candidate-1"),
		Candidates: planCandidates(t),
		Legs: []domain.PlannedLeg{
			leg("hub-a", "hub-b", "carrier-1", 2*time.Hour, false),
			leg("hub-b", "zone-c", "partner-2", 8*time.Hour, true),
		},
		Strategy:      scalar(t, domain.NewRouteStrategyReference, "strategy/v1"),
		ViewRevision:  scalar(t, domain.NewNetworkViewRevision, "netview-42"),
		JudgedAt:      routeJudgedAt,
		EffectiveFrom: routeJudgedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("形成计划：%v", err)
	}
	return plan
}

func noRouteJudgment(t *testing.T, key domain.InitialRouteJudgmentKey) domain.NoCurrentRouteJudgment {
	t.Helper()
	eliminated := func(id, reason string) domain.RouteCandidate {
		candidate, err := domain.NewRouteCandidate(
			scalar(t, domain.NewCandidateID, id),
			domain.CandidateEliminated,
			scalar(t, domain.NewCandidateReason, reason),
		)
		if err != nil {
			t.Fatalf("构造淘汰候选：%v", err)
		}
		return candidate
	}
	judgment, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key: key,
		Candidates: []domain.RouteCandidate{
			eliminated("candidate-1", "area-excluded"),
			eliminated("candidate-2", "path-closed"),
		},
		Strategy:     scalar(t, domain.NewRouteStrategyReference, "strategy/v1"),
		ViewRevision: scalar(t, domain.NewNetworkViewRevision, "netview-42"),
		JudgedAt:     routeJudgedAt,
	})
	if err != nil {
		t.Fatalf("形成无路可走判断：%v", err)
	}
	return judgment
}

// reroutedRecord 造一份`已改路`记录：失效三件+判定+新计划+自动决定，复核库最富的一行。
func reroutedRecord(
	t *testing.T,
	key domain.InitialRouteJudgmentKey,
	correlation domain.RequestCorrelationID,
) ports.ReassessmentRecord {
	t.Helper()

	newPlan := formedPlan(t, key, "RPV-0002")
	decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
		Authority:    domain.AutomaticRerouteAllowed,
		Mode:         domain.AutomaticReroute,
		Trigger:      scalar(t, domain.NewRerouteTriggerReference, correlation.String()),
		OriginalPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		NewPlan:      newPlan,
		DecidedAt:    reassessedAtTS,
	})
	if err != nil {
		t.Fatalf("形成改路决定：%v", err)
	}
	return ports.ReassessmentRecord{
		Correlation:    correlation,
		Key:            key,
		Conclusion:     ports.ReassessmentRerouted,
		ReviewedPlan:   scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		LapseBasis:     scalar(t, domain.NewApplicabilityBasisReference, "closure-7"),
		CandidateState: ports.CandidatesAvailable,
		RerouteState:   domain.AutomaticRerouteAllowed,
		NewPlan:        newPlan,
		HasNewPlan:     true,
		Decision:       decision,
		HasDecision:    true,
		ReassessedAt:   reassessedAtTS,
	}
}
