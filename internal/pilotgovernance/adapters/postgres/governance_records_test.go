package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证治理记录三库的行为：候选组不可扩张（无 UPDATE 路径、
// 重复固定拒）、评审决定写入代数、区间只追加且同维重复行幂等、Go/No-Go 形状由迁移
// CHECK 把关、事务纪律由 RequireExecutor 拦住。断言一律在事务闭包外。

var reviewDecidedAt = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)

func newStores(t *testing.T) (*adapter.CandidateSets, *adapter.ReviewDecisions, *adapter.AuthorityIntervals, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	sets, err := adapter.NewCandidateSets(db)
	if err != nil {
		t.Fatalf("构造候选组库：%v", err)
	}
	reviews, err := adapter.NewReviewDecisions(db)
	if err != nil {
		t.Fatalf("构造评审库：%v", err)
	}
	intervals, err := adapter.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatalf("构造区间库：%v", err)
	}
	return sets, reviews, intervals, db.Transactor()
}

func govTx(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func govRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func candidateSet(t *testing.T, id string) domain.CandidateVersionSet {
	t.Helper()
	set, err := domain.FixCandidateVersionSet(
		govRef(t, domain.NewCandidateVersionSetID, id),
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		govRef(t, domain.NewParameterSnapshotReference, "par-snapshot/r12"),
		govRef(t, domain.NewRuleVersionsReference, "rule-versions/r7"),
		reviewDecidedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("固定候选组：%v", err)
	}
	return set
}

func goDecision(t *testing.T, setID string) domain.StageReviewDecision {
	t.Helper()
	decision, err := domain.RecordStageReview(domain.StageReviewDecisionSpec{
		Stage:        domain.ShadowRun,
		Objective:    "enter-limited-production",
		Scope:        govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		Candidates:   govRef(t, domain.NewCandidateVersionSetID, setID),
		EvidencePack: "evidence-pack/r4",
		Verdict:      domain.StageGo,
		Deviations: []domain.AcceptedDeviation{{
			Scope:         "one-lane-latency",
			Control:       "daily-replay-check",
			Owner:         "ops-owner-1",
			CloseBy:       reviewDecidedAt.Add(30 * 24 * time.Hour),
			ResidualRisk:  "delayed-visibility",
			AcceptanceRef: "risk-acceptance/r1",
		}},
		DecidedBy:   "pilot-business-owner",
		DecidedAt:   reviewDecidedAt,
		EffectiveAt: reviewDecidedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("形成 Go 决定：%v", err)
	}
	return decision
}

func noGoDecision(t *testing.T, setID string) domain.StageReviewDecision {
	t.Helper()
	decision, err := domain.RecordStageReview(domain.StageReviewDecisionSpec{
		Stage:        domain.LimitedProduction,
		Objective:    "expand-scope",
		Scope:        govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		Candidates:   govRef(t, domain.NewCandidateVersionSetID, setID),
		EvidencePack: "evidence-pack/r5",
		Verdict:      domain.StageNoGo,
		Disposition:  domain.SuspendNewAdmission,
		DecidedBy:    "pilot-business-owner",
		DecidedAt:    reviewDecidedAt,
		EffectiveAt:  reviewDecidedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("形成 No-Go 决定：%v", err)
	}
	return decision
}

// TestCandidateSetsFixOnceAndReadBackUnchanged 证候选组原样读回，且同标识第二次固定
// 被拒不改写——不可扩张连「重新固定」的口都没有。
func TestCandidateSetsFixOnceAndReadBackUnchanged(t *testing.T) {
	sets, _, _, transactor := newStores(t)
	ctx := t.Context()

	fixed := candidateSet(t, "set-1")
	govTx(t, transactor, ctx, func(txCtx context.Context) error {
		return sets.Save(txCtx, fixed)
	})

	found, exists, err := sets.FindByID(ctx, fixed.ID())
	if err != nil || !exists {
		t.Fatalf("取回候选组：%v exists=%v", err, exists)
	}
	if found != fixed {
		t.Errorf("候选组读回变形：%+v，应为 %+v", found, fixed)
	}

	mutated, err := domain.FixCandidateVersionSet(
		fixed.ID(),
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v4"),
		fixed.Parameters(),
		fixed.Rules(),
		fixed.FormedAt(),
	)
	if err != nil {
		t.Fatalf("构造变体：%v", err)
	}
	saveErr := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return sets.Save(txCtx, mutated)
	})
	if !errors.Is(saveErr, adapter.ErrCandidateSetAlreadyFixed) {
		t.Fatalf("重复固定应拒 ErrCandidateSetAlreadyFixed，实得：%v", saveErr)
	}
	kept, _, err := sets.FindByID(ctx, fixed.ID())
	if err != nil {
		t.Fatalf("复读候选组：%v", err)
	}
	if kept.Scope() != fixed.Scope() {
		t.Error("重复固定改写了已固定的组")
	}
}

// TestReviewDecisionsRoundTripBothShapes 证 Go（带六件齐全偏差）与 No-Go（带显式
// 处理方式）都原样读回，重建经领域不变量。
func TestReviewDecisionsRoundTripBothShapes(t *testing.T) {
	sets, reviews, _, transactor := newStores(t)
	ctx := t.Context()

	set := candidateSet(t, "set-1")
	goKey := ports.ReviewKey{Objective: "enter-limited-production", Candidates: set.ID()}
	noGoKey := ports.ReviewKey{Objective: "expand-scope", Candidates: set.ID()}
	goRecord := goDecision(t, "set-1")
	noGoRecord := noGoDecision(t, "set-1")
	govTx(t, transactor, ctx, func(txCtx context.Context) error {
		if err := sets.Save(txCtx, set); err != nil {
			return err
		}
		if _, err := reviews.Save(txCtx, goKey, goRecord); err != nil {
			return err
		}
		_, err := reviews.Save(txCtx, noGoKey, noGoRecord)
		return err
	})

	foundGo, exists, err := reviews.FindByKey(ctx, goKey)
	if err != nil || !exists {
		t.Fatalf("取回 Go 决定：%v exists=%v", err, exists)
	}
	if foundGo.Verdict() != domain.StageGo || len(foundGo.Deviations()) != 1 ||
		foundGo.Deviations()[0].AcceptanceRef != "risk-acceptance/r1" {
		t.Errorf("Go 决定读回变形：%+v", foundGo)
	}
	if _, noGo := foundGo.Disposition(); noGo {
		t.Error("Go 决定凭空长出了处理方式")
	}
	if foundGo.EvidencePack() != "evidence-pack/r4" || foundGo.DecidedBy() != "pilot-business-owner" {
		t.Errorf("证据包或决定方读回变形：%+v", foundGo)
	}

	foundNoGo, exists, err := reviews.FindByKey(ctx, noGoKey)
	if err != nil || !exists {
		t.Fatalf("取回 No-Go 决定：%v exists=%v", err, exists)
	}
	disposition, noGo := foundNoGo.Disposition()
	if !noGo || disposition != domain.SuspendNewAdmission {
		t.Errorf("No-Go 处理方式读回变形：%v noGo=%v", disposition, noGo)
	}
}

// TestASecondReviewWriterGetsAlreadyRecorded 证同（目标+候选组）第二份决定拿到
// `已有记录`且事务保持可用——同一事务里紧接着读回先到者作答；先到者不被改写。
func TestASecondReviewWriterGetsAlreadyRecorded(t *testing.T) {
	sets, reviews, _, transactor := newStores(t)
	ctx := t.Context()

	set := candidateSet(t, "set-1")
	key := ports.ReviewKey{Objective: "enter-limited-production", Candidates: set.ID()}
	first := goDecision(t, "set-1")
	govTx(t, transactor, ctx, func(txCtx context.Context) error {
		if err := sets.Save(txCtx, set); err != nil {
			return err
		}
		_, err := reviews.Save(txCtx, key, first)
		return err
	})

	second, err := domain.RecordStageReview(domain.StageReviewDecisionSpec{
		Stage:        domain.ShadowRun,
		Objective:    key.Objective,
		Scope:        govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		Candidates:   key.Candidates,
		EvidencePack: "evidence-pack/r9",
		Verdict:      domain.StageNoGo,
		Disposition:  domain.FixAndReassess,
		DecidedBy:    "someone-else",
		DecidedAt:    reviewDecidedAt.Add(time.Hour),
		EffectiveAt:  reviewDecidedAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成第二决定：%v", err)
	}
	var outcome ports.ReviewSaveOutcome
	var winner domain.StageReviewDecision
	var winnerFound bool
	govTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := reviews.Save(txCtx, key, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = reviews.FindByKey(txCtx, key)
		return err
	})

	if outcome != ports.ReviewAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %q，应为 ALREADY_RECORDED", outcome)
	}
	if !winnerFound || winner.Verdict() != domain.StageGo {
		t.Fatalf("同事务读回赢家失败：found=%v verdict=%v", winnerFound, winner.Verdict())
	}
}

// TestIntervalsAppendOnlyAndExactDuplicatesStayOneRow 证区间只追加、开放区间往返、
// 同维完全重复的追加不长第二行（重放幂等），不同区间照常并存供应用层预检。
func TestIntervalsAppendOnlyAndExactDuplicatesStayOneRow(t *testing.T) {
	_, _, intervals, transactor := newStores(t)
	ctx := t.Context()

	open := domain.AuthorityInterval{
		ObjectScope: "lane-1-parcels",
		Capability:  "shipment-intake",
		FactKind:    "acceptance-decision",
		Authority:   "parcel-product",
		From:        reviewDecidedAt,
	}
	closed := domain.AuthorityInterval{
		ObjectScope: "lane-1-parcels",
		Capability:  "shipment-intake",
		FactKind:    "acceptance-decision",
		Authority:   "legacy-system",
		From:        reviewDecidedAt.Add(-48 * time.Hour),
		To:          reviewDecidedAt,
	}
	govTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, interval := range []domain.AuthorityInterval{open, closed, open} {
			if err := intervals.Append(txCtx, interval); err != nil {
				return err
			}
		}
		return nil
	})

	listed, err := intervals.ListCurrent(ctx)
	if err != nil {
		t.Fatalf("读回区间：%v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("区间数 = %d，应为 2（同维完全重复的追加不长第二行）", len(listed))
	}
	var foundOpen, foundClosed bool
	for _, interval := range listed {
		if interval.Authority == "parcel-product" && interval.To.IsZero() {
			foundOpen = true
		}
		if interval.Authority == "legacy-system" && interval.To.Equal(reviewDecidedAt) {
			foundClosed = true
		}
	}
	if !foundOpen || !foundClosed {
		t.Errorf("区间读回变形：%+v", listed)
	}
}

// TestGovernanceWritesRefuseToRunOutsideATransaction 证三库写入都不会在缺少事务时
// 改用连接池。
func TestGovernanceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	sets, reviews, intervals, _ := newStores(t)
	ctx := t.Context()

	if err := sets.Save(ctx, candidateSet(t, "set-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务固定候选组应拒，实得：%v", err)
	}
	key := ports.ReviewKey{Objective: "enter-limited-production", Candidates: govRef(t, domain.NewCandidateVersionSetID, "set-1")}
	if _, err := reviews.Save(ctx, key, goDecision(t, "set-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存决定应拒，实得：%v", err)
	}
	if err := intervals.Append(ctx, domain.AuthorityInterval{
		ObjectScope: "lane-1-parcels",
		Capability:  "shipment-intake",
		FactKind:    "acceptance-decision",
		Authority:   "parcel-product",
		From:        reviewDecidedAt,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加区间应拒，实得：%v", err)
	}
}

// TestGovernanceRollbackLeavesNothingBehind 证三库写入与所在事务同生共死。
func TestGovernanceRollbackLeavesNothingBehind(t *testing.T) {
	sets, reviews, intervals, transactor := newStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	set := candidateSet(t, "set-1")
	key := ports.ReviewKey{Objective: "enter-limited-production", Candidates: set.ID()}
	decision := goDecision(t, "set-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := sets.Save(txCtx, set); err != nil {
			return err
		}
		if _, err := reviews.Save(txCtx, key, decision); err != nil {
			return err
		}
		if err := intervals.Append(txCtx, domain.AuthorityInterval{
			ObjectScope: "lane-1-parcels",
			Capability:  "shipment-intake",
			FactKind:    "acceptance-decision",
			Authority:   "parcel-product",
			From:        reviewDecidedAt,
		}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := sets.FindByID(ctx, set.ID()); err != nil || exists {
		t.Errorf("回滚后候选组仍在：err=%v exists=%v", err, exists)
	}
	if _, exists, err := reviews.FindByKey(ctx, key); err != nil || exists {
		t.Errorf("回滚后决定仍在：err=%v exists=%v", err, exists)
	}
	listed, err := intervals.ListCurrent(ctx)
	if err != nil {
		t.Fatalf("读回区间：%v", err)
	}
	if len(listed) != 0 {
		t.Errorf("回滚后区间仍在：%+v", listed)
	}
}
