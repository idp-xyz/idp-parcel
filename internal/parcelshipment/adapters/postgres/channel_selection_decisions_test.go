package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「渠道择优决定」登记册（票 label-channel/14）：一条决定整份往返
// 一样不少（含缺席的评价引用仍缺席、并列与出局各自的格）、同一条重放答`已登记`而不是撞键、
// 同一对象多条按决定时刻构成历史、租户隔离在 SQL 条件上、无事务拒写、坏行在重建门与库内 CHECK
// 上暴露。

var selectionDecidedAt = time.Date(2026, 9, 4, 18, 30, 0, 0, time.UTC)

func newChannelSelectionDecisions(t *testing.T) (*adapter.ChannelSelectionDecisions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewChannelSelectionDecisions(db)
	if err != nil {
		t.Fatalf("构造决定登记册：%v", err)
	}
	return registry, db.Transactor(), pool
}

func selectionSubjectFixture(t *testing.T, scope, mapping string) domain.ChannelSelectionSubject {
	t.Helper()

	subject, err := domain.NewChannelSelectionSubject(
		mustBuild(t, domain.NewCommercialScopeReference, scope),
		mustBuild(t, domain.NewProductChannelMappingReference, mapping),
	)
	if err != nil {
		t.Fatalf("造对象引用：%v", err)
	}
	return subject
}

func pricedCandidateFixture(t *testing.T, id, amount, evaluation string) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.PricedChannelCandidate(
		mustBuild(t, domain.NewChannelCandidateID, id),
		mustBuild(t, domain.NewChannelCostAmount, amount),
		mustBuild(t, domain.NewChannelCostCurrency, "SYN"),
	)
	if err != nil {
		t.Fatalf("造已定价候选 %s：%v", id, err)
	}
	if evaluation == "" {
		return cost
	}
	cost, err = cost.WithEvaluation(mustBuild(t, domain.NewChannelCostEvaluationReference, evaluation))
	if err != nil {
		t.Fatalf("带评价引用：%v", err)
	}
	return cost
}

func unpriceableCandidateFixture(t *testing.T, id string, grade domain.ChannelCostUnavailability) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.UnpriceableChannelCandidate(mustBuild(t, domain.NewChannelCandidateID, id), grade)
	if err != nil {
		t.Fatalf("造不可计价候选 %s：%v", id, err)
	}
	return cost
}

func decisionFixture(
	t *testing.T,
	id, tenant string,
	subject domain.ChannelSelectionSubject,
	decidedAt time.Time,
	costs ...domain.ChannelCandidateCost,
) domain.ChannelSelectionDecision {
	t.Helper()

	decision, err := domain.FormChannelSelectionDecision(domain.ChannelSelectionDecisionSpec{
		ID:            mustBuild(t, domain.NewChannelSelectionDecisionID, id),
		Tenant:        mustBuild(t, domain.NewTenantID, tenant),
		Subject:       subject,
		AssembledAsOf: decidedAt.Add(-time.Minute),
		DecidedAt:     decidedAt,
		Costs:         costs,
	})
	if err != nil {
		t.Fatalf("形成决定 %s：%v", id, err)
	}
	return decision
}

func mustAppendDecision(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	registry *adapter.ChannelSelectionDecisions,
	decision domain.ChannelSelectionDecision,
) ports.ChannelSelectionDecisionAppendOutcome {
	t.Helper()

	var outcome ports.ChannelSelectionDecisionAppendOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registry.Append(txCtx, decision)
		return err
	})
	return outcome
}

// Covers: 一条带三格结果（选中带评价引用、落选不带评价引用、出局带因由）的决定整份往返，读回的
// 与写入的逐格相同——缺席的评价引用仍缺席，出局因由与并列格不丢。
func TestAChannelSelectionDecisionRoundTripsThroughPostgres(t *testing.T) {
	registry, transactor, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject := selectionSubjectFixture(t, "scope-a", "mapping-1")
	written := decisionFixture(t, "decision-1", "tenant-1", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-cheap", "10.00", "eval-1"),
		pricedCandidateFixture(t, "cand-dear", "12.50", ""),
		unpriceableCandidateFixture(t, "cand-excluded", domain.ChannelCostRatecardExclusion),
	)

	if outcome := mustAppendDecision(t, transactor, ctx, registry, written); outcome != ports.ChannelSelectionDecisionAppended {
		t.Fatalf("append outcome = %d，want Appended", outcome)
	}

	history, err := registry.ListBySubject(ctx, written.Tenant(), subject)
	if err != nil {
		t.Fatalf("列历史：%v", err)
	}
	if len(history) != 1 {
		t.Fatalf("历史 %d 条，want 1", len(history))
	}
	read := history[0]
	if read.ID() != written.ID() || read.Tenant() != written.Tenant() || read.Subject() != written.Subject() ||
		!read.AssembledAsOf().Equal(written.AssembledAsOf()) || read.Rule() != written.Rule() ||
		!read.DecidedAt().Equal(written.DecidedAt()) || read.Conclusion() != written.Conclusion() {
		t.Fatalf("头部不等：\n读回 %+v\n写入 %+v", read, written)
	}
	wantResults, gotResults := written.Results(), read.Results()
	if len(wantResults) != len(gotResults) {
		t.Fatalf("结果 %d 条，want %d", len(gotResults), len(wantResults))
	}
	for index := range wantResults {
		if wantResults[index] != gotResults[index] {
			t.Fatalf("第 %d 条结果不等：读回 %+v 写入 %+v", index, gotResults[index], wantResults[index])
		}
	}
	selected, chosen := read.Selected()
	if !chosen || selected.String() != "cand-cheap" {
		t.Fatalf("读回的选中者 = %v/%v", selected, chosen)
	}
}

// Covers: 只追加的幂等——同一条决定重放答`已登记`，不撞键、不覆盖；库里仍只有一条。
func TestReplayingTheSameDecisionAnswersAlreadyRecorded(t *testing.T) {
	registry, transactor, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject := selectionSubjectFixture(t, "scope-a", "mapping-1")
	decision := decisionFixture(t, "decision-1", "tenant-1", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
	)

	mustAppendDecision(t, transactor, ctx, registry, decision)
	if outcome := mustAppendDecision(t, transactor, ctx, registry, decision); outcome != ports.ChannelSelectionDecisionAlreadyRecorded {
		t.Fatalf("重放 outcome = %d，want AlreadyRecorded", outcome)
	}

	history, err := registry.ListBySubject(ctx, decision.Tenant(), subject)
	if err != nil || len(history) != 1 {
		t.Fatalf("重放后历史 %d 条 err=%v，want 1", len(history), err)
	}
}

// Covers: 裁决「同一票多条记录按决定时刻构成择优历史」——并列冲突与随后重跑选出唯一一条是两条
// 记录，按决定时刻升序读回，旧记录一字不改；另一个对象与另一个租户的记录不混进来。
func TestDecisionsOnTheSameSubjectFormAHistoryInDecisionOrder(t *testing.T) {
	registry, transactor, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject := selectionSubjectFixture(t, "scope-a", "mapping-1")

	tied := decisionFixture(t, "decision-tied", "tenant-1", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
		pricedCandidateFixture(t, "cand-b", "10.00", ""),
	)
	resolved := decisionFixture(t, "decision-resolved", "tenant-1", subject, selectionDecidedAt.Add(time.Hour),
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
		pricedCandidateFixture(t, "cand-b", "10.50", ""),
	)
	otherSubject := decisionFixture(t, "decision-other-subject", "tenant-1",
		selectionSubjectFixture(t, "scope-a", "mapping-2"), selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
	)
	otherTenant := decisionFixture(t, "decision-other-tenant", "tenant-2", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
	)
	// 刻意先写后发生的那条，证顺序来自决定时刻而不是写入次序。
	for _, decision := range []domain.ChannelSelectionDecision{resolved, tied, otherSubject, otherTenant} {
		mustAppendDecision(t, transactor, ctx, registry, decision)
	}

	history, err := registry.ListBySubject(ctx, tied.Tenant(), subject)
	if err != nil {
		t.Fatalf("列历史：%v", err)
	}
	if len(history) != 2 {
		t.Fatalf("历史 %d 条，want 2", len(history))
	}
	if history[0].ID() != tied.ID() || history[0].Conclusion() != domain.ChannelSelectionConcludedTied {
		t.Fatalf("第一条 = %s/%s，want decision-tied/TIED", history[0].ID(), history[0].Conclusion())
	}
	if history[1].ID() != resolved.ID() || history[1].Conclusion() != domain.ChannelSelectionConcludedSelected {
		t.Fatalf("第二条 = %s/%s，want decision-resolved/SELECTED", history[1].ID(), history[1].Conclusion())
	}

	empty, err := registry.ListBySubject(ctx, mustBuild(t, domain.NewTenantID, "tenant-3"), subject)
	if err != nil || len(empty) != 0 {
		t.Fatalf("从未择优过的租户应得空切片，实得 %d 条 err=%v", len(empty), err)
	}
}

// Covers: PBC-08 写口在无事务上下文必须被 RequireExecutor 拒绝——同一事务里落选中者与决定记录
// 是本口存在的理由，脱离事务写入等于允许只留一半。
func TestChannelSelectionDecisionsRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	decision := decisionFixture(t, "decision-1", "tenant-1", selectionSubjectFixture(t, "scope-a", "mapping-1"),
		selectionDecidedAt, pricedCandidateFixture(t, "cand-a", "10.00", ""))

	if _, err := registry.Append(ctx, decision); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务追加应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 一行坏数据在读回时被重建门拦住，而不是变成一条看着合法的决定；库内 CHECK 先拦得住
// 单行形状（出局无因由），门拦跨行命题（两个选中者）。两层各自有牙，缺一层另一层不补。
func TestACorruptedDecisionRowIsRejectedOnTheWayBack(t *testing.T) {
	registry, transactor, pool := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject := selectionSubjectFixture(t, "scope-a", "mapping-1")
	decision := decisionFixture(t, "decision-1", "tenant-1", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", ""),
		pricedCandidateFixture(t, "cand-b", "12.00", ""),
		unpriceableCandidateFixture(t, "cand-c", domain.ChannelCostNotFormed),
	)
	mustAppendDecision(t, transactor, ctx, registry, decision)

	// 库内 CHECK：出局却抹掉因由，写不进去。
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.channel_selection_candidate SET exclusion = NULL
		  WHERE tenant_id = 'tenant-1' AND decision_id = 'decision-1' AND candidate_ref = 'cand-c'`,
	); err == nil {
		t.Fatal("出局无因由的行写进了库——CHECK 没在守")
	}

	// 跨行命题：把落选者改成第二个选中者，单行各自合法，整条记录不合法，门要拦。
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.channel_selection_candidate SET outcome = 'SELECTED'
		  WHERE tenant_id = 'tenant-1' AND decision_id = 'decision-1' AND candidate_ref = 'cand-b'`,
	); err != nil {
		t.Fatalf("制造坏行：%v", err)
	}
	if _, err := registry.ListBySubject(ctx, decision.Tenant(), subject); !errors.Is(err, domain.ErrInvalidRehydratedChannelSelectionDecision) {
		t.Fatalf("两个选中者的记录读回应被重建门拒绝，实得：%v", err)
	}
}
