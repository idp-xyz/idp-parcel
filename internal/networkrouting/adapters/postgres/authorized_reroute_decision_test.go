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

// 本文件对真实 PostgreSQL 16 证授权改路决定册（ADR-0284 决定五，迁移 0029）：一份建议至多一份决定、撞键答已有记录；决定
// 指回复核册那一行；整行往返后经领域构造门复验；决定形成的计划进路由历史（FormedPlans）与段窗口读口；过时一问只看同判断键上
// 不早于建议的另一次复核；建议读面按决定册与复核联查出三格状态；无事务拒写。

// decisionSuggestionAt 是建议所在那次复核的触发业务时间。
var decisionSuggestionAt = reassessedAtTS.Add(-time.Hour)

const decisionOperator = "OPERATOR/https://issuer.example#ops-7"

type decisionStores struct {
	reassessments *adapter.RouteReassessments
	decisions     *adapter.AuthorizedRerouteDecisions
	catalogue     *adapter.RerouteSuggestionCatalogue
	windows       *adapter.PlannedLegWindows
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newDecisionStores(t *testing.T) decisionStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	stores := decisionStores{transactor: db.Transactor(), pool: pool}
	if stores.reassessments, err = adapter.NewRouteReassessments(db); err != nil {
		t.Fatalf("构造复核库：%v", err)
	}
	if stores.decisions, err = adapter.NewAuthorizedRerouteDecisions(db); err != nil {
		t.Fatalf("构造决定册：%v", err)
	}
	if stores.catalogue, err = adapter.NewRerouteSuggestionCatalogue(db); err != nil {
		t.Fatalf("构造建议读面：%v", err)
	}
	if stores.windows, err = adapter.NewPlannedLegWindows(db); err != nil {
		t.Fatalf("构造段窗口读口：%v", err)
	}
	return stores
}

// saveSuggestion 落一行失效线上带建议的复核：候选 1 合格、候选 2 带理由淘汰。
func saveSuggestion(t *testing.T, stores decisionStores, key domain.InitialRouteJudgmentKey, correlation string, at time.Time) domain.RequestCorrelationID {
	t.Helper()
	id := scalar(t, domain.NewRequestCorrelationID, correlation)
	suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         key,
		Trigger:     scalar(t, domain.NewRerouteTriggerReference, correlation),
		Candidates:  planCandidates(t),
		Blockers:    []string{"IMPROVEMENT_BELOW_THRESHOLD"},
		SuggestedAt: reassessedAtTS,
	})
	if err != nil {
		t.Fatalf("构造建议：%v", err)
	}
	saveReassessment(t, stores.transactor, t.Context(), stores.reassessments, id, ports.ReassessmentRecord{
		Correlation:       id,
		Key:               key,
		Conclusion:        ports.ReassessmentPlanLapsed,
		ReviewedPlan:      scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		LapseBasis:        scalar(t, domain.NewApplicabilityBasisReference, "closure-7"),
		CandidateState:    ports.CandidatesAvailable,
		RerouteState:      domain.SuggestionOnly,
		RerouteBlockers:   []string{"IMPROVEMENT_BELOW_THRESHOLD"},
		Suggestion:        suggestion,
		HasSuggestion:     true,
		ReassessedAt:      reassessedAtTS,
		TriggerOccurredAt: at,
	})
	return id
}

// saveOtherReassessment 在同一判断键上落另一次复核（原先无路由、无合格候选），触发业务时间由调用方给。
func saveOtherReassessment(t *testing.T, stores decisionStores, key domain.InitialRouteJudgmentKey, correlation string, at time.Time) {
	t.Helper()
	id := scalar(t, domain.NewRequestCorrelationID, correlation)
	saveReassessment(t, stores.transactor, t.Context(), stores.reassessments, id, ports.ReassessmentRecord{
		Correlation:       id,
		Key:               key,
		Conclusion:        ports.ReassessmentPlanLapsed,
		CandidateState:    ports.NoQualifiedCandidates,
		ReassessedAt:      reassessedAtTS.Add(time.Hour),
		TriggerOccurredAt: at,
	})
}

func authorizedDecision(t *testing.T, key domain.InitialRouteJudgmentKey, suggestion domain.RequestCorrelationID, version string) ports.AuthorizedRerouteDecision {
	t.Helper()
	plan := formedPlan(t, key, version)
	decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
		Authority:    domain.SuggestionOnly,
		Mode:         domain.AuthorizedRoleReroute,
		DecidedBy:    scalar(t, domain.NewAuthorizedRoleReference, decisionOperator),
		Trigger:      scalar(t, domain.NewRerouteTriggerReference, suggestion.String()),
		OriginalPlan: scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		NewPlan:      plan,
		DecidedAt:    reassessedAtTS.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成授权改路决定：%v", err)
	}
	return ports.AuthorizedRerouteDecision{
		Suggestion: suggestion,
		Key:        key,
		Candidate:  plan.SelectedCandidate(),
		Basis:      scalar(t, domain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-7"),
		Decision:   decision,
	}
}

func saveDecision(t *testing.T, stores decisionStores, decision ports.AuthorizedRerouteDecision) ports.AuthorizedRerouteDecisionSaveOutcome {
	t.Helper()
	var outcome ports.AuthorizedRerouteDecisionSaveOutcome
	within(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
		saved, err := stores.decisions.Save(txCtx, decision)
		outcome = saved
		return err
	})
	return outcome
}

// Covers: AT-NR-095、AT-NR-096（ADR-0284 决定五）——一份决定整行往返：决定方式、决定方、触发、原计划、新计划、依据与时刻经构造门重建原样；同一份建议
// 再落一份答已有记录、先到的那一份不被顶替；他租户读不到。决定形成的计划进路由历史，段引用读得到它的窗口。
func TestAnAuthorizedRerouteDecisionRoundTripsAndJoinsTheRoutingHistory(t *testing.T) {
	stores := newDecisionStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	suggestion := saveSuggestion(t, stores, key, "trigger-2", decisionSuggestionAt)

	decision := authorizedDecision(t, key, suggestion, "RPV-0002")
	if outcome := saveDecision(t, stores, decision); outcome != ports.AuthorizedRerouteDecisionSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
	found, present, err := stores.decisions.FindBySuggestion(ctx, key.TenantID, suggestion)
	if err != nil || !present {
		t.Fatalf("读回：present=%v err=%v", present, err)
	}
	by, _ := found.Decision.DecidedBy()
	if found.Decision.Mode() != domain.AuthorizedRoleReroute || by.String() != decisionOperator ||
		found.Decision.Trigger().String() != "trigger-2" || found.Decision.OriginalPlan().String() != "RPV-0001" ||
		found.Decision.NewPlan().Version().String() != "RPV-0002" || found.Candidate.String() != "candidate-1" ||
		found.Basis.String() != "SYN-APPROVAL-7" || found.Key != key ||
		!found.Decision.DecidedAt().Equal(decision.Decision.DecidedAt()) {
		t.Fatalf("往返变形：%+v", found)
	}
	if len(found.Decision.NewPlan().Legs()) != 2 {
		t.Fatalf("新计划段链 = %d 段, want 2", len(found.Decision.NewPlan().Legs()))
	}

	again := authorizedDecision(t, key, suggestion, "RPV-0003")
	if outcome := saveDecision(t, stores, again); outcome != ports.AuthorizedRerouteDecisionAlreadyRecorded {
		t.Fatalf("同一份建议再落一份：outcome = %d, want 已有记录", outcome)
	}
	if kept, _, _ := stores.decisions.FindBySuggestion(ctx, key.TenantID, suggestion); kept.Decision.NewPlan().Version().String() != "RPV-0002" {
		t.Fatalf("先到的决定被顶替成 %q", kept.Decision.NewPlan().Version())
	}
	if _, present, err := stores.decisions.FindBySuggestion(ctx, scalar(t, domain.NewTenantID, "tenant-2"), suggestion); err != nil || present {
		t.Fatalf("他租户读到了决定：present=%v err=%v", present, err)
	}

	formed, err := stores.reassessments.FormedPlans(ctx, key)
	if err != nil {
		t.Fatalf("FormedPlans：%v", err)
	}
	if len(formed) != 1 || formed[0].Version().String() != "RPV-0002" {
		t.Fatalf("路由历史里的新计划 = %v, want 决定形成的 RPV-0002", formed)
	}
	reference, err := domain.NewPlannedLegReference(scalar(t, domain.NewRoutePlanVersionID, "RPV-0002"), 1)
	if err != nil {
		t.Fatalf("段引用：%v", err)
	}
	if _, found, err := stores.windows.LoadPlannedLegWindow(ctx, key.TenantID, reference); err != nil || !found {
		t.Fatalf("段窗口读口没找到决定形成的计划：found=%v err=%v", found, err)
	}
}

// Covers: ADR-0284 决定五——决定指回复核册那一行：没有那次复核的决定落不进；行形状与建议、新计划对不上时写口当场拒。
func TestTheDecisionRegisterRefusesADecisionThatDoesNotFitItsSuggestion(t *testing.T) {
	stores := newDecisionStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	suggestion := saveSuggestion(t, stores, key, "trigger-2", decisionSuggestionAt)

	orphan := authorizedDecision(t, key, scalar(t, domain.NewRequestCorrelationID, "trigger-unknown"), "RPV-0002")
	if err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := stores.decisions.Save(txCtx, orphan)
		return err
	}); err == nil {
		t.Fatal("没有复核的决定落进去了")
	}

	mismatched := authorizedDecision(t, key, suggestion, "RPV-0002")
	mismatched.Candidate = scalar(t, domain.NewCandidateID, "candidate-2")
	if err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := stores.decisions.Save(txCtx, mismatched)
		return err
	}); err == nil {
		t.Fatal("所择候选与新计划选中的不是同一条，写口却收了")
	}
	if _, present, _ := stores.decisions.FindBySuggestion(ctx, key.TenantID, suggestion); present {
		t.Fatal("被拒的决定留下了行")
	}
}

// Covers: AT-NR-096（ADR-0284 决定二「过时」）——同一判断键上另有一次触发业务时间不早于建议的复核才算：更早的不算，同一时刻与更晚的算，
// 别的判断键的不算。
func TestASuggestionIsOvertakenOnlyByAnotherReassessmentNotEarlier(t *testing.T) {
	stores := newDecisionStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	suggestion := saveSuggestion(t, stores, key, "trigger-2", decisionSuggestionAt)
	overtaken := func() bool {
		t.Helper()
		var answer bool
		within(t, stores.transactor, ctx, func(txCtx context.Context) error {
			var err error
			answer, err = stores.decisions.SuggestionOvertaken(txCtx, key, suggestion, decisionSuggestionAt)
			return err
		})
		return answer
	}
	if overtaken() {
		t.Fatal("只有建议自己，读成了过时")
	}
	saveOtherReassessment(t, stores, key, "trigger-earlier", decisionSuggestionAt.Add(-time.Minute))
	saveOtherReassessment(t, stores, routeKey(t, "tenant-1", "parcel-2"), "trigger-other-parcel", decisionSuggestionAt.Add(time.Hour))
	if overtaken() {
		t.Fatal("更早的复核或别的包裹的复核让建议过时了")
	}
	saveOtherReassessment(t, stores, key, "trigger-same-time", decisionSuggestionAt)
	if !overtaken() {
		t.Fatal("同一时刻另有复核，建议没读成过时")
	}
}

// Covers: AT-NR-099（ADR-0284 决定六）——建议读面按三格列状态：待决定、已决定（带决定）、已过时；候选与阻塞照建议原样转写。
func TestTheSuggestionCatalogueListsPendingDecidedAndSupersededSuggestions(t *testing.T) {
	stores := newDecisionStores(t)
	ctx := t.Context()
	pending := routeKey(t, "tenant-1", "parcel-pending")
	saveSuggestion(t, stores, pending, "trigger-pending", decisionSuggestionAt)
	decided := routeKey(t, "tenant-1", "parcel-decided")
	decidedSuggestion := saveSuggestion(t, stores, decided, "trigger-decided", decisionSuggestionAt)
	saveDecision(t, stores, authorizedDecision(t, decided, decidedSuggestion, "RPV-0009"))
	superseded := routeKey(t, "tenant-1", "parcel-superseded")
	saveSuggestion(t, stores, superseded, "trigger-superseded", decisionSuggestionAt)
	saveOtherReassessment(t, stores, superseded, "trigger-superseded-later", decisionSuggestionAt.Add(time.Minute))
	saveSuggestion(t, stores, routeKey(t, "tenant-2", "parcel-elsewhere"), "trigger-elsewhere", decisionSuggestionAt)

	rows, err := stores.catalogue.ListRerouteSuggestions(ctx, scalar(t, domain.NewTenantID, "tenant-1"), 50)
	if err != nil {
		t.Fatalf("list：%v", err)
	}
	states := map[string]ports.RerouteSuggestionCatalogueRow{}
	for _, row := range rows {
		states[row.CorrelationID] = row
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3（他租户与不带建议的复核都不在）", len(rows))
	}
	if row := states["trigger-pending"]; row.State != ports.RerouteSuggestionPending || row.HasDecision ||
		len(row.Candidates) != 2 || row.Candidates[1].Outcome != "ELIMINATED" || row.Candidates[1].Reason != "customs-restriction" ||
		len(row.Blockers) != 1 || row.ReviewedPlan != "RPV-0001" || !row.TriggerOccurredAt.Equal(decisionSuggestionAt) {
		t.Fatalf("待决定那一行 = %+v", row)
	}
	if row := states["trigger-decided"]; row.State != ports.RerouteSuggestionDecided || !row.HasDecision ||
		row.Decision.NewPlanVersion != "RPV-0009" || row.Decision.Candidate != "candidate-1" ||
		row.Decision.DecidedBy != decisionOperator || row.Decision.Basis != "SYN-APPROVAL-7" {
		t.Fatalf("已决定那一行 = %+v", row)
	}
	if row := states["trigger-superseded"]; row.State != ports.RerouteSuggestionSuperseded {
		t.Fatalf("已过时那一行 = %+v", row)
	}
	if _, err := stores.catalogue.ListRerouteSuggestions(ctx, scalar(t, domain.NewTenantID, "tenant-1"), 0); err == nil {
		t.Fatal("limit 0 没被拒")
	}
}

// PBC-08 行为面负向证据：决定册写口在无事务上下文必须被 RequireExecutor 拒绝——决定要与它取的判断键锁同一笔。
func TestTheDecisionRegisterRefusesToWriteOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	decisions, err := adapter.NewAuthorizedRerouteDecisions(db)
	if err != nil {
		t.Fatalf("构造决定册：%v", err)
	}
	key := routeKey(t, "tenant-1", "parcel-1")
	decision := authorizedDecision(t, key, scalar(t, domain.NewRequestCorrelationID, "trigger-2"), "RPV-0002")
	if _, err := decisions.Save(t.Context(), decision); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落决定应返回 ErrTransactionRequired，实得：%v", err)
	}
}
