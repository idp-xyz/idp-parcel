package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func arrivalPoolFixture(t *testing.T) (
	*bentopg.DB, *pgxpool.Pool, *adapter.CollectionRegistrations, *adapter.CollectionPointView,
) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}
	view, err := adapter.NewCollectionPointView(db)
	if err != nil {
		t.Fatalf("构造代收面读口：%v", err)
	}
	return db, pool, collections, view
}

// arrival 造一条引用 funds 的真实到账（钉版本 SYN-VERSION-<id>）。
func arrival(t *testing.T, id, instruction, funds string, amountMinor int64) domain.CollectionFact {
	t.Helper()
	evidence, err := domain.NewEvidenceReference(funds)
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	version, err := domain.NewAdoptedFundsVersion("SYN-VERSION-" + id)
	if err != nil {
		t.Fatalf("构造采用版本：%v", err)
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:                  fixtureFactID(t, id),
		Instruction:         fixtureInstructionID(t, instruction),
		Layer:               domain.OperatorBankCredit,
		Evidence:            evidence,
		Amount:              fixtureMoney(t, amountMinor),
		OccurredAt:          fixtureAt(t, "2026-08-24T01:00:00Z"),
		AdoptedFundsVersion: version,
	})
	if err != nil {
		t.Fatalf("接受真实到账：%v", err)
	}
	return fact
}

func registerInstructions(t *testing.T, db *bentopg.DB, collections *adapter.CollectionRegistrations, ids ...string) {
	t.Helper()
	tenant := fixtureTenant(t)
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		for _, id := range ids {
			if _, err := collections.RegisterInstruction(ctx, tenant, fixtureInstruction(t, id, 12000)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("登记指令：%v", err)
	}
}

// Covers: AT-CR-020——真实到账钉的采用版本随登记落库、读回同值；按引用读到的是引用这条事实的真实到账，别的三层即便证据串
// 撞上同一串也不在其中；别的租户读不到。
func TestAnArrivalPinsItsAdoptedFundsVersionAndTheReferenceReadReturnsOnlyArrivals(t *testing.T) {
	db, _, collections, view := arrivalPoolFixture(t)
	tenant := fixtureTenant(t)
	registerInstructions(t, db, collections, "SYN-INSTR-1", "SYN-INSTR-2")

	funds, _ := domain.NewAdoptedFundsReference("SYN-BANK-RECEIPT-1")
	lookalike := fixtureFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	sameEvidence, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID: lookalike.ID(), Instruction: lookalike.Instruction(), Layer: domain.ChannelCollectionReport,
		Evidence: mustEvidence(t, "SYN-BANK-RECEIPT-1"), Amount: lookalike.Amount(), OccurredAt: lookalike.OccurredAt(),
	})
	if err != nil {
		t.Fatalf("造证据串相同的渠道报告：%v", err)
	}
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		for _, fact := range []domain.CollectionFact{
			arrival(t, "SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", 6000),
			arrival(t, "SYN-FACT-B", "SYN-INSTR-2", "SYN-BANK-RECEIPT-1", 4000),
			arrival(t, "SYN-FACT-OTHER", "SYN-INSTR-1", "SYN-BANK-RECEIPT-2", 100),
			sameEvidence,
		} {
			saved, err := collections.AcceptCollectionFact(ctx, tenant, fact)
			if err != nil {
				return err
			}
			if saved != ports.SaveRegistered {
				return errors.New("落册没有答已登记：" + fact.ID().String())
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("登记：%v", err)
	}

	read, found, err := view.LoadCollectionFact(t.Context(), tenant, fixtureFactID(t, "SYN-FACT-A"))
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if version, pinned := read.AdoptedFundsVersion(); !pinned || version.String() != "SYN-VERSION-SYN-FACT-A" {
		t.Fatalf("读回的版本 = (%q, %v)", version, pinned)
	}
	report, _, err := view.LoadCollectionFact(t.Context(), tenant, fixtureFactID(t, "SYN-FACT-REPORT"))
	if err != nil {
		t.Fatal(err)
	}
	if _, pinned := report.AdoptedFundsVersion(); pinned {
		t.Fatal("渠道报告读回带了结算事实的版本")
	}

	referenced, err := view.LoadAdoptedFundsReferences(t.Context(), tenant, funds)
	if err != nil {
		t.Fatalf("按引用读到账：%v", err)
	}
	if len(referenced) != 2 || referenced[0].ID().String() != "SYN-FACT-A" || referenced[1].ID().String() != "SYN-FACT-B" {
		t.Fatalf("引用 SYN-BANK-RECEIPT-1 的真实到账 = %v，要 A、B 两条（不含别的引用与别的层）", referenced)
	}
	other, _ := domain.NewTenantID("SYN-T2")
	if none, err := view.LoadAdoptedFundsReferences(t.Context(), other, funds); err != nil || len(none) != 0 {
		t.Fatalf("别的租户读到 %d 条（%v）", len(none), err)
	}
}

// Covers: 写口只收钉了采用版本的真实到账——没钉就来写是调用方编程错误，写口拒、什么都不落（不让它撞库上的 CHECK 折成依赖故障）。
func TestTheWriteSideRefusesAnUnpinnedArrival(t *testing.T) {
	db, pool, collections, _ := arrivalPoolFixture(t)
	tenant := fixtureTenant(t)
	registerInstructions(t, db, collections, "SYN-INSTR-1")

	unpinned := fixtureFact(t, "SYN-FACT-A", "SYN-INSTR-1", domain.OperatorBankCredit, 6000)
	bare, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID: unpinned.ID(), Instruction: unpinned.Instruction(), Layer: domain.OperatorBankCredit,
		Evidence: unpinned.Evidence(), Amount: unpinned.Amount(), OccurredAt: unpinned.OccurredAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		_, err := collections.AcceptCollectionFact(ctx, tenant, bare)
		return err
	}); err == nil {
		t.Fatal("没钉版本的真实到账被写口收下了")
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM collection_remittance.collection_fact`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("拒写之后册上 %d 行（%v）", rows, err)
	}
}

// Covers: 一条已采用事实对一条指令至多一条真实到账——换事实标识再登同一指令，写口撞唯一索引答已登记、不落第二行（编排按标识
// 读不回就判内容冲突）；另一条指令、另一条事实、别的租户同引用都不挡。
func TestOneAdoptedFactBacksAtMostOneArrivalPerInstruction(t *testing.T) {
	db, pool, collections, view := arrivalPoolFixture(t)
	tenant := fixtureTenant(t)
	registerInstructions(t, db, collections, "SYN-INSTR-1", "SYN-INSTR-2")

	save := func(fact domain.CollectionFact) ports.SaveOutcome {
		t.Helper()
		var saved ports.SaveOutcome
		if err := withinTransaction(t, db, func(ctx context.Context) error {
			var err error
			saved, err = collections.AcceptCollectionFact(ctx, tenant, fact)
			return err
		}); err != nil {
			t.Fatalf("写 %s：%v", fact.ID(), err)
		}
		return saved
	}
	if got := save(arrival(t, "SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", 6000)); got != ports.SaveRegistered {
		t.Fatalf("首条 = %v", got)
	}
	if got := save(arrival(t, "SYN-FACT-A2", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", 6000)); got != ports.SaveAlreadyRegistered {
		t.Fatalf("同一指令换事实标识 = %v，要已登记（撞唯一索引）", got)
	}
	if _, found, _ := view.LoadCollectionFact(t.Context(), tenant, fixtureFactID(t, "SYN-FACT-A2")); found {
		t.Fatal("撞索引的那条仍落了册")
	}
	if got := save(arrival(t, "SYN-FACT-B", "SYN-INSTR-2", "SYN-BANK-RECEIPT-1", 4000)); got != ports.SaveRegistered {
		t.Fatalf("另一条指令 = %v", got)
	}
	if got := save(arrival(t, "SYN-FACT-C", "SYN-INSTR-1", "SYN-BANK-RECEIPT-2", 100)); got != ports.SaveRegistered {
		t.Fatalf("另一条事实、同一指令 = %v", got)
	}

	// 库面 CHECK：版本只属于真实到账；新登记的真实到账必有版本。
	insert := func(layer string, version *string) error {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO collection_remittance.collection_fact
				(tenant_id, fact_ref, instruction_ref, source_layer, evidence_ref, currency, amount_minor,
				 occurred_at, funds_fact_version)
			 VALUES ('SYN-T1', $1, 'SYN-INSTR-1', $2, 'SYN-EV', 'EUR', 1, now(), $3)`,
			"SYN-RAW-"+layer, layer, version)
		return err
	}
	pinned := "v1"
	var pgErr *pgconn.PgError
	if err := insert("CHANNEL_COLLECTION_REPORT", &pinned); !errors.As(err, &pgErr) ||
		pgErr.ConstraintName != "collection_fact_funds_version_only_on_arrival" {
		t.Fatalf("渠道报告带采用版本：%v", err)
	}
	if err := insert("OPERATOR_BANK_CREDIT", nil); !errors.As(err, &pgErr) ||
		pgErr.ConstraintName != "collection_fact_arrival_pins_funds_version" {
		t.Fatalf("真实到账不带采用版本：%v", err)
	}
	blank := "  "
	if err := insert("OPERATOR_BANK_CREDIT", &blank); !errors.As(err, &pgErr) {
		t.Fatalf("真实到账带空白版本：%v", err)
	}
}

// Covers: 本上下文的事实锁——登记「读合计、判够、落册」要在一把按（租户，已采用事实引用）的事务锁下：同一引用的第二路等第一路
// 提交；不同引用不互等；无环境事务即拒。
func TestTheFundsReferenceLockQueuesRegistrationsOfOneFactOnly(t *testing.T) {
	db, pool, collections, _ := arrivalPoolFixture(t)
	tenant := fixtureTenant(t)
	ctx := t.Context()
	first, _ := domain.NewAdoptedFundsReference("SYN-BANK-RECEIPT-1")
	second, _ := domain.NewAdoptedFundsReference("SYN-BANK-RECEIPT-2")

	if err := collections.LockAdoptedFundsReferences(ctx, tenant, first); err == nil {
		t.Fatal("无环境事务也取了锁")
	}

	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("旁观连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })

	holding := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := collections.LockAdoptedFundsReferences(txCtx, tenant, first); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding

	waiterDone := make(chan error, 1)
	go func() {
		waiterDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return collections.LockAdoptedFundsReferences(txCtx, tenant, first)
		})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := observer.QueryRow(ctx,
			`SELECT count(*) FROM pg_locks l JOIN pg_database d ON d.oid = l.database
			  WHERE d.datname = $1 AND l.locktype = 'advisory' AND NOT l.granted`, database).Scan(&waiting); err != nil {
			t.Fatalf("查等锁：%v", err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-waiterDone:
			t.Fatalf("同一引用的第二路没有等第一路：%v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("第二路既没拿到锁也没等在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if err := <-holderDone; err != nil {
		t.Fatalf("持锁方：%v", err)
	}
	if err := <-waiterDone; err != nil {
		t.Fatalf("等锁方：%v", err)
	}

	if err := withinTransaction(t, db, func(txCtx context.Context) error {
		if err := collections.LockAdoptedFundsReferences(txCtx, tenant, first); err != nil {
			return err
		}
		other := make(chan error, 1)
		go func() {
			other <- db.Transactor().WithinTransaction(ctx, func(inner context.Context) error {
				return collections.LockAdoptedFundsReferences(inner, tenant, second)
			})
		}()
		select {
		case err := <-other:
			return err
		case <-time.After(10 * time.Second):
			return errors.New("不同引用互等了")
		}
	}); err != nil {
		t.Fatalf("不同引用的锁：%v", err)
	}
}

func mustEvidence(t *testing.T, value string) domain.EvidenceReference {
	t.Helper()
	evidence, err := domain.NewEvidenceReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
