package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件在真实 PostgreSQL 上证同一结算作用域上并发的接受前控制互不覆盖（隔离合成 S）：施加与释放两只编排走真账本、真余额与
// 信用状况视图、真作用域锁，商业侧与计价侧的几口用替身。屏障停在先手那一路读完账本之后，此时它手里是整册快照；排队时后手那一路
// 被锁挡在读账之前（看 pg_locks 认），不排队时后手径直提交。两本账各证一遍。

var raceControlAt = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// raceBook 是本文件要各证一遍的那本账。
type raceBook struct {
	name   string
	kind   domain.ControlKind
	method domain.SettlementMethod
}

var raceBooks = []raceBook{
	{name: "冻结", kind: domain.PrepaidFreezeControl, method: domain.PrepaidSettlement},
	{name: "暴露", kind: domain.CreditCheckControl, method: domain.TermsSettlement},
}

// raceGate 只拦一次：arm 之后第一路读完账本的停在这里，等 resume 关上才往下走；后面的路不再停。
type raceGate struct {
	armed   atomic.Bool
	reached chan struct{}
	resume  chan struct{}
}

func newRaceGate() *raceGate {
	return &raceGate{reached: make(chan struct{}), resume: make(chan struct{})}
}

func (gate *raceGate) pass() {
	if gate.armed.CompareAndSwap(true, false) {
		close(gate.reached)
		<-gate.resume
	}
}

// gatedFreezes 与 gatedExposures 只把编排手里那一口包上屏障；余额与信用状况视图读的是未包的账本：屏障停在哪一步不随编排排不排队
// 而变——锁之前还是之后由编排定，屏障不替它定。
type gatedFreezes struct {
	*adapter.FreezeLedgers
	gate *raceGate
}

func (ledgers gatedFreezes) LoadForScope(ctx context.Context, tenant domain.TenantID, scope domain.SettlementScope) (*domain.FreezeLedger, error) {
	ledger, err := ledgers.FreezeLedgers.LoadForScope(ctx, tenant, scope)
	ledgers.gate.pass()
	return ledger, err
}

type gatedExposures struct {
	*adapter.CreditExposureLedgers
	gate *raceGate
}

func (ledgers gatedExposures) LoadForScope(ctx context.Context, tenant domain.TenantID, scope domain.SettlementScope) (*domain.CreditExposureLedger, error) {
	ledger, err := ledgers.CreditExposureLedgers.LoadForScope(ctx, tenant, scope)
	ledgers.gate.pass()
	return ledger, err
}

type raceRig struct {
	book       raceBook
	transactor bentoapp.Transactor
	freezes    *adapter.FreezeLedgers
	exposures  *adapter.CreditExposureLedgers
	balances   *adapter.OperationalBalances
	standings  *adapter.CreditStandings
	apply      *application.ApplyPreAcceptanceControlHandler
	release    *application.ReleasePreAcceptanceControlHandler
	gate       *raceGate
	watcher    *pgx.Conn
	tenant     domain.TenantID
	scope      domain.SettlementScope
}

// newRaceRig 登好余额或信用状况：余额与授信额度都是 capacity，每笔控制的估价都是 raceAmount。
func newRaceRig(t *testing.T, book raceBook, capacity int64) *raceRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	rig := &raceRig{book: book, transactor: db.Transactor(), gate: newRaceGate(), tenant: saTenant(t, "tenant-1"), scope: saScope(t)}
	if rig.freezes, err = adapter.NewFreezeLedgers(db); err != nil {
		t.Fatalf("构造冻结册：%v", err)
	}
	if rig.exposures, err = adapter.NewCreditExposureLedgers(db); err != nil {
		t.Fatalf("构造暴露册：%v", err)
	}
	if rig.balances, err = adapter.NewOperationalBalances(db, rig.freezes); err != nil {
		t.Fatalf("构造余额视图：%v", err)
	}
	if rig.standings, err = adapter.NewCreditStandings(db, rig.exposures); err != nil {
		t.Fatalf("构造信用状况视图：%v", err)
	}
	locks, err := adapter.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		t.Fatalf("构造作用域锁：%v", err)
	}
	// 屏障只装在要证的那本账上：释放先读冻结那本，装在两本上，证暴露时先手会停在一本空的冻结账之后，还没读到要争的那一笔。
	var freezes ports.FreezeLedgerRepository = rig.freezes
	var exposures ports.CreditExposureLedgerRepository = rig.exposures
	if book.kind == domain.PrepaidFreezeControl {
		freezes = gatedFreezes{FreezeLedgers: rig.freezes, gate: rig.gate}
	} else {
		exposures = gatedExposures{CreditExposureLedgers: rig.exposures, gate: rig.gate}
	}

	item, err := domain.NewControlItem(book.kind, 1)
	if err != nil {
		t.Fatalf("控制项：%v", err)
	}
	policy, err := domain.NewRequiredControlPolicy([]domain.ControlItem{item}, domain.AllControlsPass,
		saValue(t, domain.NewControlPolicyReference, "SYN-CONTROL-POLICY/v1"), book.method,
		saValue(t, domain.NewAdoptedPolicyReference, "SYN-SETTLEMENT-POLICY/v1"))
	if err != nil {
		t.Fatalf("控制策略：%v", err)
	}
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(rig.scope.Account(), domain.PerParcelSellTrialSum,
		saValue(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL"))
	if err != nil {
		t.Fatalf("估价采用：%v", err)
	}
	trial, err := domain.NewPreAcceptanceTrial("P-1", "EST-P-1", []string{"PRICE_CARD:SYN-PLAN-SELL@v1"}, raceAmount,
		rig.scope.Currency())
	if err != nil {
		t.Fatalf("试算：%v", err)
	}
	basis, err := domain.NewCreditAmountBasis(saValue(t, domain.NewCreditPolicyReference, "SYN-CREDIT-POLICY/v1"), capacity)
	if err != nil {
		t.Fatalf("授信依据：%v", err)
	}

	rig.apply, err = application.NewApplyPreAcceptanceControlHandler(application.ApplyPreAcceptanceControlDeps{
		Policy:    racePolicy{policy: policy},
		Adoptions: raceAdoption{adoption: adoption},
		Trials: raceTrials{answer: ports.PreAcceptanceTrialAnswer{
			Outcome: ports.PreAcceptanceTrialsFormed, PriceCard: "PRICE_CARD:SYN-PLAN-SELL@v1", Trials: []domain.PreAcceptanceTrial{trial},
		}},
		Traces:      raceTraces{},
		Balance:     rig.balances,
		Freezes:     freezes,
		Credit:      rig.standings,
		CreditBasis: raceCreditBasis{basis: basis},
		RatioBases:  raceRatioBases{},
		Exposures:   exposures,
		ScopeLock:   locks,
		Clock:       fixedRaceClock{at: raceControlAt},
	})
	if err != nil {
		t.Fatalf("构造施加编排：%v", err)
	}
	rig.release = application.NewReleasePreAcceptanceControlHandler(freezes, exposures, locks,
		&steppingRaceClock{from: raceControlAt})

	saWithin(t, rig.transactor, t.Context(), func(txCtx context.Context) error {
		if book.kind == domain.PrepaidFreezeControl {
			_, err := rig.balances.RecordBalance(txCtx, rig.tenant, rig.scope, adapter.BalancePosting{
				PostedMinor: capacity, AsOf: raceControlAt.Add(-time.Hour),
			})
			return err
		}
		_, err := rig.standings.RecordStanding(txCtx, rig.tenant, rig.scope, adapter.CreditPosition{
			AsOf: raceControlAt.Add(-time.Hour),
		})
		return err
	})

	// 看锁的连接单独开：两路各占池里一条，再从池里取可能与停住的那一路互等。
	watcher, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	rig.watcher = watcher
	return rig
}

const raceAmount = int64(3_000)

func (rig *raceRig) command(t *testing.T, request string) application.ApplyPreAcceptanceControlCommand {
	t.Helper()
	asOf, err := domain.NewControlAsOf(saValue(t, domain.NewAsOfSemantic, "CONTROL_EVALUATION_AT"), raceControlAt,
		saValue(t, domain.NewAsOfStrategyVersion, "control-strategy-v1"))
	if err != nil {
		t.Fatalf("控制时点：%v", err)
	}
	return application.ApplyPreAcceptanceControlCommand{
		TenantID:          rig.tenant,
		RequestID:         saValue(t, domain.NewControlRequestID, request),
		Scope:             rig.scope,
		ShipmentRequest:   "SHR-" + request,
		SubmissionVersion: "SV-1",
		Association:       saValue(t, domain.NewBusinessAssociationReference, "SAC-"+request),
		AsOf:              asOf,
		Resolution:        saValue(t, domain.NewCommercialResolutionReference, "RES-1"),
	}
}

func (rig *raceRig) mustApply(t *testing.T, request string) application.ApplyPreAcceptanceControlResult {
	t.Helper()
	var result application.ApplyPreAcceptanceControlResult
	saWithin(t, rig.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = rig.apply.Handle(txCtx, rig.command(t, request))
		return err
	})
	if result.Outcome() != application.ControlApplied {
		t.Fatalf("施加 %s = %s/%s，want CONTROL_APPLIED", request, result.Outcome(), result.NotFormedReason())
	}
	return result
}

func (rig *raceRig) applyStep(t *testing.T, request string, into *application.ApplyPreAcceptanceControlResult) func(context.Context) error {
	command := rig.command(t, request)
	return func(txCtx context.Context) error {
		result, err := rig.apply.Handle(txCtx, command)
		*into = result
		return err
	}
}

func (rig *raceRig) releaseStep(t *testing.T, request string, into *application.ReleasePreAcceptanceControlResult) func(context.Context) error {
	command := application.ReleasePreAcceptanceControlCommand{
		TenantID: rig.tenant, RequestID: saValue(t, domain.NewControlRequestID, request), Scope: rig.scope,
	}
	return func(txCtx context.Context) error {
		result, err := rig.release.Handle(txCtx, command)
		*into = result
		return err
	}
}

// race 让 first 停在读完账本之后，再放 second 去跑；等到 second 要么已经提交、要么正等在咨询锁上，才放 first 走完。两路各在自己
// 的事务里，事务错误原样交回。
func (rig *raceRig) race(t *testing.T, first, second func(context.Context) error) (error, error) {
	t.Helper()
	ctx := t.Context()
	rig.gate.armed.Store(true)
	var once sync.Once
	resume := func() { once.Do(func() { close(rig.gate.resume) }) }
	defer resume()

	firstDone := make(chan error, 1)
	go func() { firstDone <- rig.transactor.WithinTransaction(ctx, first) }()
	select {
	case <-rig.gate.reached:
	case err := <-firstDone:
		t.Fatalf("先手那一路没读到账本就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("先手那一路十秒内没走到屏障")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- rig.transactor.WithinTransaction(ctx, second) }()
	var secondErr error
	secondFinished := false
	deadline := time.Now().Add(10 * time.Second)
	for !secondFinished && !rig.waitingOnAdvisoryLock(t) {
		select {
		case secondErr = <-secondDone:
			secondFinished = true
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("后手那一路十秒内既没提交、也没等在咨询锁上")
		}
	}
	resume()

	var firstErr error
	select {
	case firstErr = <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("放开屏障后先手那一路十秒内没走完")
	}
	if !secondFinished {
		select {
		case secondErr = <-secondDone:
		case <-time.After(10 * time.Second):
			t.Fatal("先手提交后后手那一路十秒内没走完")
		}
	}
	return firstErr, secondErr
}

// waitingOnAdvisoryLock 只看本测试库：pgtest 给每个用例一个物理库，别的用例的锁不该算进来。
func (rig *raceRig) waitingOnAdvisoryLock(t *testing.T) bool {
	t.Helper()
	var waiting int
	if err := rig.watcher.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_locks
		  WHERE locktype = 'advisory' AND NOT granted
		    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`,
	).Scan(&waiting); err != nil {
		t.Fatalf("看锁：%v", err)
	}
	return waiting > 0
}

// occupation 读回某一笔请求在本书上的状态与本书当前占用合计。
func (rig *raceRig) occupation(t *testing.T, request string) (string, bool, int64) {
	t.Helper()
	id := saValue(t, domain.NewControlRequestID, request)
	if rig.book.kind == domain.PrepaidFreezeControl {
		ledger, err := rig.freezes.LoadForScope(t.Context(), rig.tenant, rig.scope)
		if err != nil {
			t.Fatalf("读回冻结册：%v", err)
		}
		freeze, found := ledger.FindByRequest(id)
		return freeze.Status().String(), found, ledger.HeldMinor()
	}
	ledger, err := rig.exposures.LoadForScope(t.Context(), rig.tenant, rig.scope)
	if err != nil {
		t.Fatalf("读回暴露册：%v", err)
	}
	exposure, found := ledger.FindByRequest(id)
	return exposure.Status().String(), found, ledger.ExposedMinor()
}

func (rig *raceRig) releasedAt(t *testing.T, request string) time.Time {
	t.Helper()
	id := saValue(t, domain.NewControlRequestID, request)
	if rig.book.kind == domain.PrepaidFreezeControl {
		ledger, err := rig.freezes.LoadForScope(t.Context(), rig.tenant, rig.scope)
		if err != nil {
			t.Fatalf("读回冻结册：%v", err)
		}
		freeze, _ := ledger.FindByRequest(id)
		return freeze.ReleasedAt()
	}
	ledger, err := rig.exposures.LoadForScope(t.Context(), rig.tenant, rig.scope)
	if err != nil {
		t.Fatalf("读回暴露册：%v", err)
	}
	exposure, _ := ledger.FindByRequest(id)
	return exposure.ReleasedAt()
}

func releasedAtOf(result application.ReleasePreAcceptanceControlResult, book raceBook) time.Time {
	if book.kind == domain.PrepaidFreezeControl {
		freeze, _ := result.Freeze()
		return freeze.ReleasedAt()
	}
	exposure, _ := result.Exposure()
	return exposure.ReleasedAt()
}

func (rig *raceRig) heldStatus() string {
	if rig.book.kind == domain.PrepaidFreezeControl {
		return domain.FreezeHeld.String()
	}
	return domain.ExposureRecorded.String()
}

func (rig *raceRig) releasedStatus() string {
	if rig.book.kind == domain.PrepaidFreezeControl {
		return domain.FreezeReleased.String()
	}
	return domain.ExposureReleased.String()
}

func restrictedOf(result application.ApplyPreAcceptanceControlResult, book raceBook) bool {
	if book.kind == domain.PrepaidFreezeControl {
		freeze, _ := result.Freeze()
		return freeze.Status() == domain.FreezeRestricted
	}
	exposure, _ := result.Exposure()
	return exposure.Status() == domain.ExposureRestricted
}

// Covers: SA CONTEXT「重复请求、查询和补偿不得形成重复冻结或重复释放」——先手那一路为 Y 施加控制，读完账本时 X 还占着；后手那一路
// 释放 X。失败形态：后手径直提交，先手把读到的整册快照写回，X 回到占着、释放时间清空，客户的钱或额度被静默占回去。要的是先手那一路
// 的账写回之后 X 仍是已释放，Y 占着，本书占用合计只剩 Y 那一笔。
func TestAReleaseCommittedDuringAnotherControlOnTheScopeIsNotWrittenBack(t *testing.T) {
	for _, book := range raceBooks {
		t.Run(book.name, func(t *testing.T) {
			rig := newRaceRig(t, book, 10_000)
			rig.mustApply(t, "control-X")

			var applied application.ApplyPreAcceptanceControlResult
			var released application.ReleasePreAcceptanceControlResult
			firstErr, secondErr := rig.race(t, rig.applyStep(t, "control-Y", &applied), rig.releaseStep(t, "control-X", &released))
			if firstErr != nil || secondErr != nil {
				t.Fatalf("施加 Y 的事务 → %v；释放 X 的事务 → %v；两路都该提交", firstErr, secondErr)
			}
			if applied.Outcome() != application.ControlApplied || restrictedOf(applied, book) {
				t.Fatalf("施加 Y = %s/%s，want 占用成立", applied.Outcome(), applied.NotFormedReason())
			}
			if released.Outcome() != application.ControlReleased {
				t.Fatalf("释放 X = %s/%s，want CONTROL_RELEASED", released.Outcome(), released.NotFormedReason())
			}

			if status, _, _ := rig.occupation(t, "control-X"); status != rig.releasedStatus() {
				t.Fatalf("X 落库后是 %s，want %s——释放被另一路的整册快照压回去了", status, rig.releasedStatus())
			}
			status, found, total := rig.occupation(t, "control-Y")
			if !found || status != rig.heldStatus() || total != raceAmount {
				t.Fatalf("Y = %s（在册 %t），本书占用合计 %d，want %s 且合计 %d", status, found, total, rig.heldStatus(), raceAmount)
			}
		})
	}
}

// Covers: SA CONTEXT「重复请求、查询和补偿不得形成重复冻结或重复释放」与 ReleasePreAcceptanceControlHandler 的「重复释放返回与首次
// 相同的答案（含原释放时间）」——同一笔的两次释放并发。不排队时两路都读到占着、各报一个释放时刻，库里只留先落的那个，另一路交回的
// 释放时刻对不上账；排队时后到的那一路读到的已是先到那一路落的释放，两路交回同一个释放时刻，就是库里那一个。
func TestConcurrentReleasesOfOneControlAnswerTheReleaseThatLanded(t *testing.T) {
	for _, book := range raceBooks {
		t.Run(book.name, func(t *testing.T) {
			rig := newRaceRig(t, book, 10_000)
			rig.mustApply(t, "control-X")

			var first, second application.ReleasePreAcceptanceControlResult
			firstErr, secondErr := rig.race(t, rig.releaseStep(t, "control-X", &first), rig.releaseStep(t, "control-X", &second))
			if firstErr != nil || secondErr != nil {
				t.Fatalf("两次释放的事务 → %v / %v；两路都该提交", firstErr, secondErr)
			}
			landed := rig.releasedAt(t, "control-X")
			for name, result := range map[string]application.ReleasePreAcceptanceControlResult{"先手": first, "后手": second} {
				if result.Outcome() != application.ControlReleased {
					t.Fatalf("%s释放 = %s/%s，want CONTROL_RELEASED", name, result.Outcome(), result.NotFormedReason())
				}
				if at := releasedAtOf(result, book); !at.Equal(landed) {
					t.Errorf("%s交回的释放时刻 %v，库里是 %v——同一笔释放报出了两个时刻", name, at, landed)
				}
			}
		})
	}
}

// Covers: SA CONTEXT 运营结算余额「可用余额 … 再扣除冻结金额」与账期额度同理——余额与额度只够一笔，两路并发为 Y、Z 施加控制。
// 不排队时两路各读到零占用、各判够，先手写回时与后手撞在账本自编的内部编号上，先手那一路整笔事务失败、留给续办；排队时后手被锁
// 挡在读余额之前，读到的已是 Y 那一笔，答业务限制。两路都提交，本书只占 Y 那一笔。
func TestConcurrentControlsOnOneScopeJudgeAgainstEachOthersOccupation(t *testing.T) {
	for _, book := range raceBooks {
		t.Run(book.name, func(t *testing.T) {
			rig := newRaceRig(t, book, raceAmount+raceAmount/2)

			var first, second application.ApplyPreAcceptanceControlResult
			firstErr, secondErr := rig.race(t, rig.applyStep(t, "control-Y", &first), rig.applyStep(t, "control-Z", &second))
			if firstErr != nil || secondErr != nil {
				t.Fatalf("施加 Y 的事务 → %v；施加 Z 的事务 → %v；两路都该提交", firstErr, secondErr)
			}
			if first.Outcome() != application.ControlApplied || restrictedOf(first, book) {
				t.Fatalf("施加 Y = %s/%s，want 占用成立", first.Outcome(), first.NotFormedReason())
			}
			if second.Outcome() != application.ControlApplied || !restrictedOf(second, book) {
				t.Fatalf("施加 Z = %s/%s（受限 %t），want 业务限制——只够一笔", second.Outcome(), second.NotFormedReason(), restrictedOf(second, book))
			}
			if _, found, total := rig.occupation(t, "control-Z"); found || total != raceAmount {
				t.Fatalf("Z 在册 %t，本书占用合计 %d，want 不在册且合计 %d", found, total, raceAmount)
			}
		})
	}
}

// ---- 商业侧与计价侧的替身：两路并发共用，都不记状态 ----

type racePolicy struct {
	policy domain.PreAcceptanceControlPolicy
}

func (double racePolicy) LoadControlPolicy(context.Context, domain.TenantID, domain.SettlementScope, domain.CommercialResolutionReference) (domain.PreAcceptanceControlPolicy, bool, error) {
	return double.policy, true, nil
}

type raceAdoption struct {
	adoption domain.PreAcceptanceEstimateAdoption
}

func (double raceAdoption) FindAdoption(context.Context, domain.TenantID, domain.SettlementAccountID) (domain.PreAcceptanceEstimateAdoption, bool, error) {
	return double.adoption, true, nil
}

type raceTrials struct {
	answer ports.PreAcceptanceTrialAnswer
}

func (double raceTrials) FormPreAcceptanceTrials(context.Context, ports.PreAcceptanceTrialQuery) (ports.PreAcceptanceTrialAnswer, error) {
	return double.answer, nil
}

type raceTraces struct{}

func (raceTraces) RecordEstimate(context.Context, ports.PreAcceptanceEstimateTrace) (ports.PreAcceptanceEstimateTraceEffect, error) {
	return ports.PreAcceptanceEstimateTraceRecorded, nil
}

type raceCreditBasis struct{ basis domain.CreditBasis }

func (double raceCreditBasis) LoadCreditBasis(context.Context, domain.TenantID, domain.SettlementScope, domain.CommercialResolutionReference) (domain.CreditBasis, bool, error) {
	return double.basis, true, nil
}

type raceRatioBases struct{}

func (raceRatioBases) LoadCreditRatioBase(context.Context, domain.TenantID, domain.SettlementScope, domain.CreditRatioBase) (int64, bool, error) {
	return 0, false, errors.New("本夹具的授信是金额额度，不该问基数")
}

type fixedRaceClock struct{ at time.Time }

func (clock fixedRaceClock) Now() time.Time { return clock.at }

// steppingRaceClock 每问一次往后走一小时：两次释放各报的释放时刻因此分得开，看得出交回的是不是落库的那一个。
type steppingRaceClock struct {
	from  time.Time
	asked atomic.Int64
}

func (clock *steppingRaceClock) Now() time.Time {
	return clock.from.Add(time.Duration(clock.asked.Add(1)) * time.Hour)
}
