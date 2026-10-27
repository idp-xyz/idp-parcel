package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func principalFixture(t *testing.T) (
	*bentopg.DB,
	*pgxpool.Pool,
	*application.MapExternalFundsHandler,
	*adapter.CollectedPrincipalFundsView,
	domain.TenantID,
	*movingClock,
) {
	t.Helper()
	db, pool := receiptPostingDB(t)
	seedCustomerReceiptScope(t, pool)
	clock := &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)}
	handler := mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db),
		mustReceiptPostings(t, db), clock)
	view, err := adapter.NewCollectedPrincipalFundsView(db)
	if err != nil {
		t.Fatalf("构造只读视图：%v", err)
	}
	return db, pool, handler, view, receiptTenant(t), clock
}

func principalMap(tenant domain.TenantID, mapping, fact string) application.MapFundsCommand {
	return receiptMap(tenant, mapping, fact, domain.TargetCollectedPrincipal, fact)
}

// Covers: AT-SA-216——ADR-0330 的读口——collection-remittance 按事实引用读到的是链头与认作状态：未采用答 found=false；采用未认作
// SetAside 为假；整条认作之后为真；外部更正后读到的是新版本的金额；别的租户同引用读不到；无环境事务即拒。
func TestTheCollectedPrincipalViewAnswersTheHeadAndWhetherTheFactIsSetAside(t *testing.T) {
	db, _, handler, view, tenant, _ := principalFixture(t)
	ctx := t.Context()
	fact := mustValue(t, domain.NewFundsFactReference, "bank-receipt-p1")

	if _, _, err := view.LockCollectedPrincipalFact(ctx, tenant, fact); err == nil {
		t.Fatal("无环境事务也读了：共享锁没有可依附的事务")
	}

	read := func() (found bool, setAside bool, amount int64, version string) {
		saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			loaded, ok, err := view.LockCollectedPrincipalFact(txCtx, tenant, fact)
			if err != nil {
				return err
			}
			found = ok
			if ok {
				setAside = loaded.SetAside
				_, amount = loaded.Head.Amount()
				version = loaded.Head.Version().String()
			}
			return nil
		})
		return found, setAside, amount, version
	}

	if found, _, _, _ := read(); found {
		t.Fatal("未采用的事实读到了")
	}

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		_, err := handler.AdoptFact(txCtx, receiptFact(tenant, "bank-receipt-p1", domain.FundsReceiptConfirmed, 100_000, "v1"))
		return err
	})
	found, setAside, amount, version := read()
	if !found || setAside || amount != 100_000 || version != "v1" {
		t.Fatalf("采用未认作：found=%v setAside=%v amount=%d version=%s", found, setAside, amount, version)
	}

	var mapped application.FundsOutcome
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		result, err := handler.Map(txCtx, principalMap(tenant, "map-principal-p1", "bank-receipt-p1"))
		mapped = result.Outcome()
		return err
	})
	if mapped != application.FundsMapped {
		t.Fatalf("认作代收本金 = %s", mapped)
	}
	found, setAside, amount, version = read()
	if !found || !setAside || amount != 100_000 || version != "v1" {
		t.Fatalf("认作之后：found=%v setAside=%v amount=%d version=%s", found, setAside, amount, version)
	}

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		_, err := handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
			TenantID: tenant, Fact: "bank-receipt-p1", Corrects: "v1", Version: "v2",
			AmountMinor: 90_000, CorrectedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		})
		return err
	})
	found, setAside, amount, version = read()
	if !found || !setAside || amount != 90_000 || version != "v2" {
		t.Fatalf("更正之后读到的不是链头：found=%v setAside=%v amount=%d version=%s", found, setAside, amount, version)
	}

	other := mustValue(t, domain.NewTenantID, "tenant-other")
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		_, ok, err := view.LockCollectedPrincipalFact(txCtx, other, fact)
		found = ok
		return err
	})
	if found {
		t.Fatal("别的租户读到了同引用的事实")
	}
}

// Covers: AT-SA-216（并发半边）——同一笔银行回款只有一个去处——并发的两路（一路整条认作代收本金，一路映到客户对账单或审核应付，或另一路也认作
// 代收本金）在事实锁上排队：恰好一路成功、另一路映射冲突，事实上恰好一条映射；成功的是对账单时入账余额记了这笔钱，
// 成功的是代收本金时入账余额一个作用域也没碰（受托资金不进运营余额）。
func TestAReceiptHasExactlyOneDestinationUnderConcurrentMappings(t *testing.T) {
	for _, rival := range []struct {
		name string
		kind domain.SettlementTargetKind
		to   string
	}{
		{"statement", domain.TargetStatement, "stmt-1"},
		{"payable", domain.TargetPayable, "payable-1"},
		{"another principal mapping", domain.TargetCollectedPrincipal, ""},
	} {
		t.Run(rival.name, func(t *testing.T) {
			db, pool, handler, _, tenant, clock := principalFixture(t)
			ctx := t.Context()
			const rounds = 12
			for round := 0; round < rounds; round++ {
				factID := "bank-receipt-race-" + string(rune('a'+round))
				clock.at = clock.at.Add(time.Hour) // 入账余额的截至时点只进不退；同一时刻第二次推进答未推进
				saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
					_, err := handler.AdoptFact(txCtx, receiptFact(tenant, factID, domain.FundsReceiptConfirmed, 10_000, "v1"))
					return err
				})

				target := rival.to
				if rival.kind == domain.TargetCollectedPrincipal {
					target = factID
				}
				commands := []application.MapFundsCommand{
					principalMap(tenant, "map-principal-"+factID, factID),
					receiptMap(tenant, "map-rival-"+factID, factID, rival.kind, target),
				}
				outcomes := make([]application.FundsOutcome, len(commands))
				errs := make([]error, len(commands))
				start := make(chan struct{})
				var group sync.WaitGroup
				for i := range commands {
					group.Add(1)
					go func() {
						defer group.Done()
						<-start
						errs[i] = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
							result, err := handler.Map(txCtx, commands[i])
							outcomes[i] = result.Outcome()
							return err
						})
					}()
				}
				close(start)
				group.Wait()

				for i, err := range errs {
					if err != nil {
						t.Fatalf("第 %d 轮第 %d 路：%v", round, i, err)
					}
				}
				mapped, conflicts := 0, 0
				for _, outcome := range outcomes {
					switch outcome {
					case application.FundsMapped:
						mapped++
					case application.FundsMappingConflict:
						conflicts++
					}
				}
				if mapped != 1 || conflicts != 1 {
					t.Fatalf("第 %d 轮：两路并发答 %v，要恰好一路映射成功、一路映射冲突", round, outcomes)
				}
				var rows int
				if err := pool.QueryRow(ctx,
					`SELECT count(*) FROM settlement_accounting.funds_mapping
					  WHERE tenant_id = 'tenant-receipt' AND fact_id = $1`, factID).Scan(&rows); err != nil {
					t.Fatalf("数映射：%v", err)
				}
				if rows != 1 {
					t.Fatalf("第 %d 轮：事实上映射行 = %d，要 1——同一笔回款落了两个去处", round, rows)
				}
			}
		})
	}
}

// Covers: AT-SA-216（并发半边）——读口的共享锁与结算一侧的外部更正互斥——代收与清分读到金额、据此判到账之和的那一笔事务提交之前，更正等着；
// 提交之后更正才落，下一次读看见新金额。没有这把锁，读与落册之间插进来的更正会让刚判过的「不超额」当场失效。
func TestTheCollectedPrincipalViewHoldsOffAnExternalCorrectionUntilTheReaderCommits(t *testing.T) {
	db, pool, handler, view, tenant, _ := principalFixture(t)
	ctx := t.Context()
	fact := mustValue(t, domain.NewFundsFactReference, "bank-receipt-lock")
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "bank-receipt-lock", domain.FundsReceiptConfirmed, 100_000, "v1")); err != nil {
			return err
		}
		_, err := handler.Map(txCtx, principalMap(tenant, "map-principal-lock", "bank-receipt-lock"))
		return err
	})

	// 用例池只有两条连接，读方与更正各占一条；旁观等锁的会话要自己的连接（pgtest.AdminDSN）。
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatalf("读库名：%v", err)
	}
	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("旁观连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })

	locked := make(chan struct{})
	release := make(chan struct{})
	releaseReader := sync.OnceFunc(func() { close(release) })
	defer releaseReader() // 断言失败提前退出时也要放读方，否则它握着连接，收尾关池就挂住
	readerDone := make(chan error, 1)
	var readAmount int64
	go func() {
		readerDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			loaded, _, err := view.LockCollectedPrincipalFact(txCtx, tenant, fact)
			if err != nil {
				return err
			}
			_, readAmount = loaded.Head.Amount()
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	correctorDone := make(chan error, 1)
	go func() {
		correctorDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
				TenantID: tenant, Fact: "bank-receipt-lock", Corrects: "v1", Version: "v2",
				AmountMinor: 60_000, CorrectedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			})
			return err
		})
	}()

	// 更正在读方事务提交之前必须卡在锁上：轮询本库里有没有等锁的会话，而不是睡一觉赌它没跑完。
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := observer.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity
			  WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatalf("查等锁会话：%v", err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-correctorDone:
			t.Fatalf("读方还握着共享锁，外部更正已经落了：%v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("更正既没落也没卡在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}

	releaseReader()
	if err := <-readerDone; err != nil {
		t.Fatalf("读方事务：%v", err)
	}
	if err := <-correctorDone; err != nil {
		t.Fatalf("更正：%v", err)
	}
	if readAmount != 100_000 {
		t.Fatalf("读方读到 %d，要更正之前的 100000", readAmount)
	}

	var after int64
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		loaded, _, err := view.LockCollectedPrincipalFact(txCtx, tenant, fact)
		if err != nil {
			return err
		}
		_, after = loaded.Head.Amount()
		return nil
	})
	if after != 60_000 {
		t.Fatalf("更正提交之后读到 %d，要 60000", after)
	}
}

// Covers: AT-SA-216——SetAside 只在这条事实上有一条目标为代收本金的映射时为真：事实已映到客户对账单、客户账户（计入运营入账余额）或审核应付，
// 读口一律答没认作。把「该事实有任何映射」当作认作，已记进运营余额的钱就还能被引作代收与清分的到账证据，同一笔回款重记一次。
func TestTheCollectedPrincipalViewDoesNotCallAMappedFactSetAside(t *testing.T) {
	for _, mapped := range []struct {
		name string
		kind domain.SettlementTargetKind
		to   string
	}{
		{"statement", domain.TargetStatement, "stmt-1"},
		{"account", domain.TargetAccount, "acct-1"},
		{"payable", domain.TargetPayable, "payable-1"},
	} {
		t.Run(mapped.name, func(t *testing.T) {
			db, _, handler, view, tenant, _ := principalFixture(t)
			ctx := t.Context()
			factID := "bank-receipt-" + mapped.name
			fact := mustValue(t, domain.NewFundsFactReference, factID)

			var outcome application.FundsOutcome
			saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
				if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, factID, domain.FundsReceiptConfirmed, 100_000, "v1")); err != nil {
					return err
				}
				result, err := handler.Map(txCtx, receiptMap(tenant, "map-"+mapped.name, factID, mapped.kind, mapped.to))
				outcome = result.Outcome()
				return err
			})
			if outcome != application.FundsMapped {
				t.Fatalf("映到 %s = %s，要已映射", mapped.name, outcome)
			}

			var found, setAside bool
			saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
				loaded, ok, err := view.LockCollectedPrincipalFact(txCtx, tenant, fact)
				found, setAside = ok, loaded.SetAside
				return err
			})
			if !found {
				t.Fatal("已采用的事实读不到")
			}
			if setAside {
				t.Fatalf("事实已映到 %s，读口答它被整条认作代收本金", mapped.name)
			}
		})
	}
}

// Covers: AT-SA-216（并发半边）——先取共享锁、再读链头：外部更正握着事实身份行、尚未提交，读方的共享锁等它；更正提交之后读方读到的是新链头，
// 不是等锁之前就看见的旧金额。读口若在取锁之前先读一次链头并用这次的结果，代收与清分会按旧的大金额判到账之和，落册之后超出更正后的链头
// （与「读方先到、更正等」那一条互补：那一条只证锁互斥，这一条证读发生在锁之后）。
func TestTheCollectedPrincipalViewReadsTheNewHeadOfACorrectionItWaitedFor(t *testing.T) {
	db, pool, handler, view, tenant, _ := principalFixture(t)
	ctx := t.Context()
	fact := mustValue(t, domain.NewFundsFactReference, "bank-receipt-wait")
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "bank-receipt-wait", domain.FundsReceiptConfirmed, 100_000, "v1")); err != nil {
			return err
		}
		_, err := handler.Map(txCtx, principalMap(tenant, "map-principal-wait", "bank-receipt-wait"))
		return err
	})
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })

	corrected := make(chan struct{})
	commit := make(chan struct{})
	release := sync.OnceFunc(func() { close(commit) })
	defer release() // 断言失败提前退出时也要放更正方，否则它握着连接，收尾关池就挂住
	correctorDone := make(chan error, 1)
	go func() {
		correctorDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, err := handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
				TenantID: tenant, Fact: "bank-receipt-wait", Corrects: "v1", Version: "v2",
				AmountMinor: 60_000, CorrectedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			}); err != nil {
				return err
			}
			close(corrected)
			<-commit
			return nil
		})
	}()
	<-corrected

	var got int64
	readerDone := make(chan error, 1)
	go func() {
		readerDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			loaded, _, err := view.LockCollectedPrincipalFact(txCtx, tenant, fact)
			if err != nil {
				return err
			}
			_, got = loaded.Head.Amount()
			return nil
		})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := observer.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-readerDone:
			t.Fatalf("读方没有等未提交的更正：%v 读到 %d", err, got)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("读方既没返回也没等锁")
		}
		time.Sleep(10 * time.Millisecond)
	}
	release()
	if err := <-correctorDone; err != nil {
		t.Fatal(err)
	}
	if err := <-readerDone; err != nil {
		t.Fatal(err)
	}
	if got != 60_000 {
		t.Fatalf("等过未提交更正的读方读到 %d，要更正后的 60000", got)
	}
}

// Covers: AT-SA-216（库约束半边）——迁移 0048 的两道兜底各自守住编排绕不到的写入：认作代收本金的映射，一条事实至多一条
// （唯一索引 funds_mapping_one_collected_principal_per_fact），目标引用必须等于事实自己（CHECK
// funds_mapping_collected_principal_names_the_fact）。绕过映射编排与事实锁，直接往映射表插行；每一道只断言它自己那条约束的名字，
// 去掉任何一道，对应的那次插入就会成功。别的目标种类不受唯一索引约束（它只罩认作）。
func TestTheMappingTableItselfRefusesASecondRecognitionAndATargetThatIsNotTheFact(t *testing.T) {
	_, pool, _, _, _, _ := principalFixture(t)
	ctx := t.Context()
	insert := func(mapping, fact, kind, target string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO settlement_accounting.funds_mapping
				(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis, mapped_at, content_digest, recorded_at)
			 VALUES ('tenant-receipt', $1, $2, $3, $4, 'instruction-1', now(), 'digest', now())`,
			mapping, fact, kind, target)
		return err
	}
	refusedBy := func(err error, code, constraint string) bool {
		var pgErr *pgconn.PgError
		return errors.As(err, &pgErr) && pgErr.Code == code && pgErr.ConstraintName == constraint
	}

	if err := insert("map-principal-1", "bank-receipt-guard", "COLLECTED_PRINCIPAL", "bank-receipt-guard"); err != nil {
		t.Fatalf("第一条认作：%v", err)
	}
	if err := insert("map-principal-2", "bank-receipt-guard", "COLLECTED_PRINCIPAL", "bank-receipt-guard"); !refusedBy(
		err, "23505", "funds_mapping_one_collected_principal_per_fact") {
		t.Fatalf("同一条事实的第二条认作 = %v，要被唯一索引 funds_mapping_one_collected_principal_per_fact 拒", err)
	}
	if err := insert("map-principal-3", "bank-receipt-other", "COLLECTED_PRINCIPAL", "somebody-else"); !refusedBy(
		err, "23514", "funds_mapping_collected_principal_names_the_fact") {
		t.Fatalf("目标不是事实自己的认作 = %v，要被 CHECK funds_mapping_collected_principal_names_the_fact 拒", err)
	}
	// 唯一索引只罩认作：同一条事实上别的目标种类的行库上不拦（互斥靠编排在事实锁下判）。
	if err := insert("map-statement-1", "bank-receipt-guard", "STATEMENT", "stmt-1"); err != nil {
		t.Fatalf("唯一索引不该罩住别的目标种类：%v", err)
	}
}
