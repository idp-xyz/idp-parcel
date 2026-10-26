package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件在真实 PostgreSQL 上证外部更正（ADR-0285，隔离合成 S）：更正低于未撤销核销合计时照采、答复带出超核金额、同一事务里把
// 这条事实落到的作用域按新金额重算；更正高于原金额时作用域同样跟上；更正与核销在事实身份行上排队，先后哪一种都落到一个
// 自洽的终态。作用域 A 是 acct-1（对账单 stmt-1），作用域 B 是 acct-2。

// correct 把已采用的事实更正为新金额，回指 corrects。
func (rig spreadRig) correct(t *testing.T, fact, corrects, version string, amount int64) application.FundsResult {
	t.Helper()
	return rig.step(t, "更正 "+fact+" 到 "+version, application.FundsFactAdopted,
		func(txCtx context.Context) (application.FundsResult, error) {
			return rig.handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
				TenantID: rig.tenant, Fact: fact, Corrects: corrects, Version: version,
				AmountMinor: amount, CorrectedAt: rig.clock.at,
			})
		})
}

// Covers: 票 code/92 评审手算例——收款 5000 映到 stmt-1 并核满 5000，更正到 3000：超核 2000，答复带出；版本行落下、链头是
// 3000 的 v2；该作用域在同一事务里重算——P 按事实金额封顶为 3000，减去 W 5000，入账余额 -2000（更正前停在 0）；
// 资金事实册上累计未分配 -2000。
func TestACorrectionBelowTheAppliedTotalIsAdoptedAndSaysSoAndRepostsTheScope(t *testing.T) {
	rig := newSpreadRig(t)
	seedConfirmedReceivable(t, t.Context(), rig.pool, 5_000)
	rig.adopt(t, "fact-over", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-over", "fact-over", domain.TargetStatement, "stmt-1")
	rig.apply(t, "app-over", "fact-over", "map-over", domain.TargetStatement, "stmt-1", 5_000)
	assertScope(t, rig.pool, "acct-1", 0, 0)

	corrected := rig.correct(t, "fact-over", "v1", "v2", 3_000)

	if got := corrected.OverAppliedMinor(); got != 2_000 {
		t.Fatalf("更正答复的超核金额 = %d，要 2000", got)
	}
	record, found := corrected.Fact()
	if !found {
		t.Fatal("更正答复该带已采用的那一版")
	}
	if _, minor := record.Fact.Amount(); minor != 3_000 || record.Fact.Version().String() != "v2" {
		t.Fatalf("答复里的那一版 = %s / %d，要 v2 / 3000", record.Fact.Version(), minor)
	}
	assertScope(t, rig.pool, "acct-1", -2_000, 0)

	catalogue, err := adapter.NewFundsApplicationCatalogue(rig.db)
	if err != nil {
		t.Fatalf("构造资金事实册：%v", err)
	}
	rows, err := catalogue.ListExternalFundsFacts(t.Context(), rig.tenant, 50)
	if err != nil {
		t.Fatalf("列资金事实册：%v", err)
	}
	for _, row := range rows {
		if row.Fact == "fact-over" && (row.Version != "v2" || row.AmountMinor != 3_000 || row.UnappliedMinor != -2_000) {
			t.Fatalf("册上 fact-over = 版本 %s、金额 %d、累计未分配 %d，要 v2 / 3000 / -2000",
				row.Version, row.AmountMinor, row.UnappliedMinor)
		}
	}
}

// Covers: 票 code/92 (b)——更正高于原金额，作用域跟着重算：收款单作用域没核销的那一截随金额涨（5000 → 8000，核销 2000：
// 入账 3000 → 6000），退回扣的随金额涨（-5000 → -7000）；多作用域的收款只有核到账户的那一截随封顶变，两个作用域各算各的（B 的
// 分配 4000 被更正到 3000 封顶，A 仍 0）。
func TestACorrectionRepostsEveryScopeTheFactReaches(t *testing.T) {
	t.Run("收款改高", func(t *testing.T) {
		rig := newSpreadRig(t)
		seedConfirmedReceivable(t, t.Context(), rig.pool, 6_000)
		rig.adopt(t, "fact-up", domain.FundsReceiptConfirmed, 5_000)
		rig.mapTo(t, "map-up", "fact-up", domain.TargetStatement, "stmt-1")
		rig.apply(t, "app-up", "fact-up", "map-up", domain.TargetStatement, "stmt-1", 2_000)
		assertScope(t, rig.pool, "acct-1", 3_000, 4_000)

		corrected := rig.correct(t, "fact-up", "v1", "v2", 8_000)
		if got := corrected.OverAppliedMinor(); got != 0 {
			t.Fatalf("改高的超核金额 = %d，要 0", got)
		}
		assertScope(t, rig.pool, "acct-1", 6_000, 4_000)
	})

	t.Run("退回改高", func(t *testing.T) {
		rig := newSpreadRig(t)
		rig.adopt(t, "return-up", domain.FundsReturned, 5_000)
		rig.mapTo(t, "map-return-up", "return-up", domain.TargetStatement, "stmt-1")
		assertScope(t, rig.pool, "acct-1", -5_000, 0)

		rig.correct(t, "return-up", "v1", "v2", 7_000)
		assertScope(t, rig.pool, "acct-1", -7_000, 0)
	})

	t.Run("多作用域收款改低", func(t *testing.T) {
		rig := newSpreadRig(t)
		rig.adopt(t, "fact-two", domain.FundsReceiptConfirmed, 5_000)
		rig.mapTo(t, "map-two-stmt", "fact-two", domain.TargetStatement, "stmt-1")
		rig.mapTo(t, "map-two-acct", "fact-two", domain.TargetAccount, "acct-2")
		rig.apply(t, "app-two", "fact-two", "map-two-acct", domain.TargetAccount, "acct-2", 4_000)
		assertScope(t, rig.pool, "acct-1", 0, 0)
		assertScope(t, rig.pool, "acct-2", 4_000, 0)

		corrected := rig.correct(t, "fact-two", "v1", "v2", 3_000)
		if got := corrected.OverAppliedMinor(); got != 1_000 {
			t.Fatalf("超核金额 = %d，要 1000", got)
		}
		assertScope(t, rig.pool, "acct-1", 0, 0)
		assertScope(t, rig.pool, "acct-2", 3_000, 0)
	})
}

// raceClock 每问一次往后走一分钟：并发的两笔事务各推进同一个作用域，「同键只准往前」要截至时点严格递增，固定时钟会把
// 后一笔吞成未推进。
type raceClock struct {
	from  time.Time
	asked atomic.Int64
}

func (clock *raceClock) Now() time.Time {
	return clock.from.Add(time.Duration(clock.asked.Add(1)) * time.Minute)
}

// gatedFactLock 在事实锁取到之后停一下：先手停在这里就是「握着事实身份行、还没往下读」，后手要么排在锁上、要么绕过了它。
type gatedFactLock struct {
	*adapter.SettlementApplications
	gate *raceGate
}

func (lock gatedFactLock) LockFundsFactApplications(ctx context.Context, key ports.FundsFactKey) (int64, bool, error) {
	applied, found, err := lock.SettlementApplications.LockFundsFactApplications(ctx, key)
	lock.gate.pass()
	return applied, found, err
}

var _ ports.FundsFactApplicationLock = gatedFactLock{}

func (rig spreadRig) gatedHandler(t *testing.T, gate *raceGate) *application.MapExternalFundsHandler {
	t.Helper()
	applications := mustApplications(t, rig.db)
	postings := mustReceiptPostings(t, rig.db)
	handler, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        mustFundsFacts(t, rig.db),
		Mappings:     mustFundsMappings(t, rig.db),
		Applications: applications,
		Downstream:   nopHandoff{},
		FactHandoff:  nopHandoff{},
		Clock:        &raceClock{from: rig.clock.at.Add(time.Hour)},
		Books:        postings,
		Balances:     postings,
		Standings:    postings,
		ClaimTargets: absentClaimTargets{},
		FactLocks:    gatedFactLock{SettlementApplications: applications, gate: gate},
	})
	if err != nil {
		t.Fatalf("构造停在事实锁后的编排：%v", err)
	}
	return handler
}

// raceOnTheFactLock 让 first 停在握着事实锁之后，再放 second 去跑；second 必须排在锁上（不排队就径直提交，是它没取事实锁）。
// 等到它排上了才放 first 走完，两路各在自己的事务里，事务错误原样交回。每一步带期限，挂死即失败。
func (rig spreadRig) raceOnTheFactLock(t *testing.T, gate *raceGate, first, second func(context.Context) error) (error, error) {
	t.Helper()
	ctx := t.Context()
	watcher, err := pgx.ConnectConfig(ctx, rig.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })

	gate.armed.Store(true)
	var once sync.Once
	resume := func() { once.Do(func() { close(gate.resume) }) }
	defer resume()

	firstDone := make(chan error, 1)
	go func() { firstDone <- rig.db.Transactor().WithinTransaction(ctx, first) }()
	select {
	case <-gate.reached:
	case err := <-firstDone:
		t.Fatalf("先手那一路没握到事实锁就结束了（它没取事实锁）：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("先手那一路十秒内没走到事实锁")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- rig.db.Transactor().WithinTransaction(ctx, second) }()
	deadline := time.Now().Add(10 * time.Second)
	for !waitingOnALock(t, watcher) {
		select {
		case err := <-secondDone:
			t.Fatalf("先手握着事实锁，后手却没排队就结束了（它没取事实锁）：%v", err)
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("后手那一路十秒内没有排在锁上")
		}
	}
	resume()

	var firstErr, secondErr error
	select {
	case firstErr = <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("放开之后先手十秒内没走完")
	}
	select {
	case secondErr = <-secondDone:
	case <-time.After(10 * time.Second):
		t.Fatal("先手提交之后后手十秒内没走完")
	}
	return firstErr, secondErr
}

// raceFixture 铺好：收款 5000 映到 stmt-1、已确认应收 5000，核销 app-1 3000。
func raceFixture(t *testing.T) spreadRig {
	t.Helper()
	rig := newSpreadRig(t)
	seedConfirmedReceivable(t, t.Context(), rig.pool, 5_000)
	rig.adopt(t, "fact-race", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-race", "fact-race", domain.TargetStatement, "stmt-1")
	rig.apply(t, "app-1", "fact-race", "map-race", domain.TargetStatement, "stmt-1", 3_000)
	assertScope(t, rig.pool, "acct-1", 2_000, 2_000)
	return rig
}

func (rig spreadRig) correctionStep(handler *application.MapExternalFundsHandler, amount int64, into *application.FundsResult) func(context.Context) error {
	return func(txCtx context.Context) error {
		result, err := handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
			TenantID: rig.tenant, Fact: "fact-race", Corrects: "v1", Version: "v2",
			AmountMinor: amount, CorrectedAt: rig.clock.at.Add(time.Hour),
		})
		*into = result
		return err
	}
}

func (rig spreadRig) applicationStep(handler *application.MapExternalFundsHandler, id string, amount int64, into *application.FundsResult) func(context.Context) error {
	return func(txCtx context.Context) error {
		result, err := handler.Apply(txCtx, application.ApplySettlementCommand{
			TenantID: rig.tenant, Application: id, Fact: "fact-race", TargetCurrency: "CNY",
			MappingRefs: []string{"map-race"},
			Allocations: []application.AllocationDirective{{
				Mapping: "map-race", TargetKind: domain.TargetStatement, Target: "stmt-1",
				Direction: domain.AllocationDebit, AmountMinor: amount,
			}},
			Basis: "instruction-1", AppliedAt: rig.clock.at.Add(time.Hour),
		})
		*into = result
		return err
	}
}

// Covers: 票 code/92 (c) 更正先到——更正握着事实锁、核销排在锁上：更正提交后核销读到的是更正后的金额 2000 与已核销 3000，
// 再加 1000 越过事实金额，答分配失衡、什么都不落。终态：已核销停在 3000，更正答复说超核 1000，作用域入账 -1000
// （P 封顶 2000，减 W 3000）、已确认未结 2000。核销不取事实锁、或更正不取，都会让核销落在更正之后的金额之上而没人说。
func TestACorrectionHoldingTheFactLockMakesALaterApplicationSeeTheCorrectedAmount(t *testing.T) {
	rig := raceFixture(t)
	gate := newRaceGate()
	handler := rig.gatedHandler(t, gate)
	var corrected, applied application.FundsResult

	correctionErr, applicationErr := rig.raceOnTheFactLock(t, gate,
		rig.correctionStep(handler, 2_000, &corrected), rig.applicationStep(handler, "app-2", 1_000, &applied))

	if correctionErr != nil || applicationErr != nil {
		t.Fatalf("事务错误：更正 %v、核销 %v", correctionErr, applicationErr)
	}
	if corrected.Outcome() != application.FundsFactAdopted || corrected.OverAppliedMinor() != 1_000 {
		t.Fatalf("更正 = %s / 超核 %d，要已采用 / 1000", corrected.Outcome(), corrected.OverAppliedMinor())
	}
	if applied.Outcome() != application.ApplicationImbalanceOutcome {
		t.Fatalf("排在更正后面的核销 = %s，要分配失衡——它读到的该是更正后的金额", applied.Outcome())
	}
	assertScope(t, rig.pool, "acct-1", -1_000, 2_000)
}

// Covers: 票 code/92 (c) 核销先到——核销握着事实锁、更正排在锁上：核销提交后更正读到的已核销合计是 4000，新金额 2500 低于它，
// 超核 1500 如实说出；作用域按提交后的账重算：P 封顶 2500 减 W 4000，入账 -1500、已确认未结 1000。更正不取事实锁就读不到
// 先到那笔核销，超核金额与作用域都停在旧账上。
func TestAnApplicationHoldingTheFactLockMakesALaterCorrectionCountItInTheOverApply(t *testing.T) {
	rig := raceFixture(t)
	gate := newRaceGate()
	handler := rig.gatedHandler(t, gate)
	var corrected, applied application.FundsResult

	applicationErr, correctionErr := rig.raceOnTheFactLock(t, gate,
		rig.applicationStep(handler, "app-2", 1_000, &applied), rig.correctionStep(handler, 2_500, &corrected))

	if correctionErr != nil || applicationErr != nil {
		t.Fatalf("事务错误：更正 %v、核销 %v", correctionErr, applicationErr)
	}
	if applied.Outcome() != application.SettlementApplied {
		t.Fatalf("先到的核销 = %s，要已核销", applied.Outcome())
	}
	if corrected.Outcome() != application.FundsFactAdopted || corrected.OverAppliedMinor() != 1_500 {
		t.Fatalf("更正 = %s / 超核 %d，要已采用 / 1500——它该数进先到那笔核销", corrected.Outcome(), corrected.OverAppliedMinor())
	}
	assertScope(t, rig.pool, "acct-1", -1_500, 1_000)
}

// Covers: PBC-08 事务外拒跑的负面证据——更正的第一步是事实锁，事务外取不到（bentopg.ErrTransactionRequired），编排折成资金事实库不可用
// 的未决，版本行一行不落、作用域一格不动。
func TestACorrectionOutsideATransactionIsRefusedAndWritesNothing(t *testing.T) {
	rig := newSpreadRig(t)
	seedConfirmedReceivable(t, t.Context(), rig.pool, 5_000)
	rig.adopt(t, "fact-tx", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-tx", "fact-tx", domain.TargetStatement, "stmt-1")

	result, err := rig.handler.CorrectFact(t.Context(), application.CorrectFundsFactCommand{
		TenantID: rig.tenant, Fact: "fact-tx", Corrects: "v1", Version: "v2",
		AmountMinor: 3_000, CorrectedAt: rig.clock.at.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("事务外取锁失败该折成未决而不是 error：%v", err)
	}
	if result.Outcome() != application.FundsUndecided || result.UndecidedReason() != application.FundsFactStoreUnavailable {
		t.Fatalf("事务外的更正 = %s/%s，要未决/资金事实库不可用", result.Outcome(), result.UndecidedReason())
	}
	var versions int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = 'tenant-receipt' AND fact_id = 'fact-tx'`,
	).Scan(&versions); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	if versions != 1 {
		t.Fatalf("事务外的更正落了版本行：%d 行，要仍是 1", versions)
	}
	assertScope(t, rig.pool, "acct-1", 5_000, 5_000)
}
