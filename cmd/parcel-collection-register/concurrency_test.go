package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真实 PostgreSQL 上证记账的两道并发防线（隔离合成 S），走 buildRegistrar 的生产
// 装配：分户账事务锁让两笔并发移位排先后，后到的读到落过的账面；绕过这把锁的写入撞上
// 按依据的唯一约束，编排折成内容冲突。

type concurrencyRig struct {
	registrar registrar
	newConn   func(t *testing.T) *pgx.Conn
	pool      *pgxpool.Pool
}

func newConcurrencyRig(t *testing.T) concurrencyRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	built, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	// 屏障与看锁的连接单独开：池只有两条，两路记账各占一条，再从池里取就会和停住的那一路互等。
	newConn := func(t *testing.T) *pgx.Conn {
		t.Helper()
		conn, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
		if err != nil {
			t.Fatalf("开旁路连接：%v", err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	return concurrencyRig{registrar: built, newConn: newConn, pool: pool}
}

func (rig concurrencyRig) must(t *testing.T, command, raw string, wantCode int) {
	t.Helper()
	if message, code := execute(t.Context(), command, []byte(raw), rig.registrar); code != wantCode {
		t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
	}
}

// seedOneReport 开账、登一条指令 100 与它的渠道报告 100 并入账。
func (rig concurrencyRig) seedOneReport(t *testing.T) {
	t.Helper()
	rig.must(t, commandSubledger, `{
		"tenantId": "SYN-T1", "customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
		"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
		"custodyBasisRef": "SYN-COD-V1", "openedAt": "2026-08-24T00:00:00Z"
	}`, exitRegistered)
	rig.must(t, commandInstruction, `{
		"tenantId": "SYN-T1", "instructionRef": "SYN-INSTR-1",
		"parcelRef": "SYN-PARCEL-1", "requirementRef": "SYN-COD-REQ-V1",
		"customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
		"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
		"amountMinor": 100, "instructedAt": "2026-08-24T00:30:00Z"
	}`, exitRegistered)
	rig.must(t, commandFact, concurrencyFact("SYN-FACT-REPORT", "CHANNEL_COLLECTION_REPORT", "100"), exitRegistered)
	rig.must(t, commandPosting, concurrencyPosting("SYN-POST-REPORT",
		"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "100", "SYN-FACT-REPORT"), exitRegistered)
}

func concurrencyFact(ref, layer, amount string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "` + ref + `", "instructionRef": "SYN-INSTR-1",
		"sourceLayer": "` + layer + `", "evidenceRef": "SYN-EV-` + ref + `",
		"currency": "EUR", "amountMinor": ` + amount + `, "occurredAt": "2026-08-24T01:00:00Z"
	}`
}

func concurrencyPosting(ref, from, to, amount, fact string) string {
	return `{
		"tenantId": "SYN-T1", "postingRef": "` + ref + `",
		"fromPosition": "` + from + `", "toPosition": "` + to + `",
		"amountMinor": ` + amount + `, "basisKind": "COLLECTION_FACT",
		"basisRef": "` + fact + `", "postedAt": "2026-08-24T02:00:00Z"
	}`
}

// waitForLockWaiters 轮询本测试库里正等在某类锁上、还没拿到的事务数，到 want 就答 true，
// 到时限答 false。只看本库：pgtest 给每个用例一个物理库，别的用例的锁不该算进来。
func waitForLockWaiters(conn *pgx.Conn, locktype string, want int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		var waiting int
		if err := conn.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_locks
			  WHERE locktype = $1 AND NOT granted
			    AND (database IS NULL OR database = (SELECT oid FROM pg_database WHERE datname = current_database()))`,
			locktype,
		).Scan(&waiting); err == nil && waiting >= want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// 同一指令两笔到账各 100、报告只有 100，两路并发移位。屏障连接先占住这本账的事务锁，
// 等看见两路都等在咨询锁上才放——证的是两路确实被锁挡在读回账面之前，而不是碰巧先后
// 跑。放开后先拿到锁的那一路落账，另一路读到的已是落过的账面，答余额不足；`渠道在途`
// 不透支。
func TestConcurrentArrivalsOfOneInstructionPostOnlyOne(t *testing.T) {
	rig := newConcurrencyRig(t)
	rig.seedOneReport(t)
	seedPrincipalFunds(t, rig.pool, "SYN-EV-SYN-FACT-CREDIT-A", "EUR", 100)
	seedPrincipalFunds(t, rig.pool, "SYN-EV-SYN-FACT-CREDIT-B", "EUR", 100)
	rig.must(t, commandFact, concurrencyFact("SYN-FACT-CREDIT-A", "OPERATOR_BANK_CREDIT", "100"), exitRegistered)
	rig.must(t, commandFact, concurrencyFact("SYN-FACT-CREDIT-B", "OPERATOR_BANK_CREDIT", "100"), exitRegistered)

	barrier := rig.newConn(t)
	watcher := rig.newConn(t)
	tx, err := barrier.Begin(t.Context())
	if err != nil {
		t.Fatalf("开屏障事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(),
		`SELECT pg_advisory_xact_lock(hashtext('collection_remittance.subledger_posting.ledger'),
		        hashtext('SYN-T1' || chr(31) || 'SYN-CUST-1' || chr(31) || 'SYN-ENTITY-1' || chr(31) || 'EUR' || chr(31) || 'SYN-CHANNEL-1'))`,
	); err != nil {
		t.Fatalf("屏障取锁：%v", err)
	}

	codes := make([]int, 2)
	var group sync.WaitGroup
	for index, fact := range []string{"SYN-FACT-CREDIT-A", "SYN-FACT-CREDIT-B"} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, codes[index] = execute(context.Background(), commandPosting,
				[]byte(concurrencyPosting("SYN-POST-"+fact,
					"IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "100", fact)), rig.registrar)
		}()
	}
	sawBoth := waitForLockWaiters(watcher, "advisory", 2, 10*time.Second)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("放屏障：%v", err)
	}
	group.Wait()
	if !sawBoth {
		t.Fatal("两路移位没有都等在分户账锁上：读回账面之前没有锁")
	}

	registered, underfunded := 0, 0
	for _, code := range codes {
		switch code {
		case exitRegistered:
			registered++
		case exitUnderfunded:
			underfunded++
		}
	}
	if registered != 1 || underfunded != 1 {
		t.Fatalf("两路移位退出码 = %v，要一路 0（落账）、一路 4（余额不足）", codes)
	}

	var inTransit int64
	if err := watcher.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(CASE WHEN to_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END)
		              - SUM(CASE WHEN from_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END), 0)
		   FROM collection_remittance.subledger_posting WHERE tenant_id = 'SYN-T1'`,
	).Scan(&inTransit); err != nil {
		t.Fatalf("读渠道在途：%v", err)
	}
	if inTransit != 0 {
		t.Fatalf("渠道在途 = %d，要 0——透支了", inTransit)
	}
}

// 一个不走分户账锁的写入者先凭同一条报告插了一行、尚未提交。编排拿得到锁、读不到那一行，
// 判重与守卫都过，落账时等在唯一约束上；那一行提交后写口 DO NOTHING，编排按记账标识读
// 不回自己那一笔，答内容冲突（退出码 2），不答已登记。
// Covers: AT-CR-008（并发半边）
func TestAFactPostedConcurrentlyOutsideTheLockAnswersConflict(t *testing.T) {
	rig := newConcurrencyRig(t)
	rig.seedOneReport(t)
	rig.must(t, commandFact, concurrencyFact("SYN-FACT-REPORT-2", "CHANNEL_COLLECTION_REPORT", "20"), exitRegistered)

	bypass := rig.newConn(t)
	watcher := rig.newConn(t)
	tx, err := bypass.Begin(t.Context())
	if err != nil {
		t.Fatalf("开旁路事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(),
		`INSERT INTO collection_remittance.subledger_posting
			(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
		 VALUES ('SYN-T1', 'SYN-POST-BYPASS', 'SYN-CUST-1', 'SYN-ENTITY-1', 'EUR', 'SYN-CHANNEL-1',
		         'EXTERNAL_SOURCE', 'IN_TRANSIT_AT_CHANNEL', 20, 'COLLECTION_FACT', 'SYN-FACT-REPORT-2',
		         '2026-08-24T02:00:00Z')`,
	); err != nil {
		t.Fatalf("旁路插行：%v", err)
	}

	var code int
	var message string
	done := make(chan struct{})
	go func() {
		defer close(done)
		message, code = execute(context.Background(), commandPosting,
			[]byte(concurrencyPosting("SYN-POST-REPORT-2",
				"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "20", "SYN-FACT-REPORT-2")), rig.registrar)
	}()
	sawWaiting := waitForLockWaiters(watcher, "transactionid", 1, 10*time.Second)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("提交旁路行：%v", err)
	}
	<-done
	if !sawWaiting {
		t.Fatal("编排的落账没有等在唯一约束上")
	}
	if code != exitNeedsReview || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("撞唯一约束的记账退出码 = %d（%s），要 2（内容冲突）", code, message)
	}
}
