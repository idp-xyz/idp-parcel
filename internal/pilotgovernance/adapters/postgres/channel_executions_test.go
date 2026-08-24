package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证受控通道执行留痕（票 12 身份双轨的第①轨）：留痕
// 只追加、事务纪律（无事务拒写、回滚不留行）、留白拒绝。验收面就是表本身——留痕
// 的消费方是审计查询，没有产品侧读口，断言直接问表。

var executionAt = time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)

type executionFixture struct {
	executions *adapter.ChannelExecutions
	db         *bentopg.DB
}

func newExecutionFixture(t *testing.T) *executionFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	executions, err := adapter.NewChannelExecutions(db)
	if err != nil {
		t.Fatalf("构造留痕库：%v", err)
	}
	return &executionFixture{executions: executions, db: db}
}

func channelExecution(reference string) adapter.ChannelExecution {
	return adapter.ChannelExecution{
		Command:         "suspend",
		RecordReference: reference,
		OSUser:          "OPSHOST\\operator-a",
		Hostname:        "ops-host-01",
		Outcome:         "SUSPENSION_RECORDED",
		ExecutedAt:      executionAt,
	}
}

func (fixture *executionFixture) countTraces(t *testing.T, ctx context.Context, reference string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(ctx)
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(ctx,
		`SELECT count(*) FROM pilot_governance.channel_execution WHERE record_reference = $1`,
		reference,
	).Scan(&count); err != nil {
		t.Fatalf("数留痕行：%v", err)
	}
	return count
}

// TestChannelExecutionTraceLandsWithBothIdentityFields 证留痕落库且两样通道技术身份
// （进程属主、主机名）原样在册；每次执行各留一行——重放登记的执行同样是一次执行，
// 留痕不做幂等。
func TestChannelExecutionTraceLandsWithBothIdentityFields(t *testing.T) {
	fixture := newExecutionFixture(t)
	ctx := t.Context()

	if err := fixture.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.executions.Append(txCtx, channelExecution("suspension-trace-1"))
	}); err != nil {
		t.Fatalf("留痕：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		trace := channelExecution("suspension-trace-1")
		trace.Outcome = "SUSPENSION_EXISTING"
		return fixture.executions.Append(txCtx, trace)
	}); err != nil {
		t.Fatalf("重放留痕：%v", err)
	}

	if count := fixture.countTraces(t, ctx, "suspension-trace-1"); count != 2 {
		t.Fatalf("留痕行数 = %d，要 2（每次执行各一行）", count)
	}

	querier, err := fixture.db.ReadExecutor(ctx)
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var osUser, hostname, outcome string
	var executedAt time.Time
	if err := querier.QueryRow(ctx,
		`SELECT os_user, hostname, outcome, executed_at
		   FROM pilot_governance.channel_execution
		  WHERE record_reference = $1
		  ORDER BY execution_id
		  LIMIT 1`,
		"suspension-trace-1",
	).Scan(&osUser, &hostname, &outcome, &executedAt); err != nil {
		t.Fatalf("读留痕：%v", err)
	}
	if osUser != "OPSHOST\\operator-a" || hostname != "ops-host-01" {
		t.Fatalf("通道技术身份 = %q@%q，要 %q@%q", osUser, hostname, "OPSHOST\\operator-a", "ops-host-01")
	}
	if outcome != "SUSPENSION_RECORDED" {
		t.Fatalf("答案 = %q，要 SUSPENSION_RECORDED", outcome)
	}
	if !executedAt.UTC().Equal(executionAt) {
		t.Fatalf("执行时刻 = %s，要 %s", executedAt.UTC(), executionAt)
	}
}

// TestChannelExecutionRequiresATransaction 证事务纪律：留痕是登记同笔事务的一半
// （登记落则痕落、登记回滚则痕消），无事务写一律拒绝。
func TestChannelExecutionRequiresATransaction(t *testing.T) {
	fixture := newExecutionFixture(t)
	ctx := t.Context()

	if err := fixture.executions.Append(ctx, channelExecution("no-tx-trace")); err == nil {
		t.Fatalf("无事务留痕被接受了；RequireExecutor 应拒绝")
	}
	if count := fixture.countTraces(t, ctx, "no-tx-trace"); count != 0 {
		t.Fatalf("留痕行数 = %d，要 0", count)
	}
}

// TestChannelExecutionRollsBackWithTheRegistration 证回滚半边：事务翻掉时留痕一并
// 消失——痕不能声称一笔没落库的登记。
func TestChannelExecutionRollsBackWithTheRegistration(t *testing.T) {
	fixture := newExecutionFixture(t)
	ctx := t.Context()

	rollback := func(txCtx context.Context) error {
		if err := fixture.executions.Append(txCtx, channelExecution("rolled-back-trace")); err != nil {
			t.Fatalf("事务内留痕：%v", err)
		}
		return context.Canceled
	}
	if err := fixture.db.Transactor().WithinTransaction(ctx, rollback); err == nil {
		t.Fatalf("要求闭包错误传出以触发回滚")
	}
	if count := fixture.countTraces(t, ctx, "rolled-back-trace"); count != 0 {
		t.Fatalf("留痕行数 = %d，要 0（回滚不留行）", count)
	}
}

// TestChannelExecutionRejectsBlankIdentity 证留白拒绝：通道技术身份是入口自取的
// 事实，取不出就不许落一行装作取到了。
func TestChannelExecutionRejectsBlankIdentity(t *testing.T) {
	fixture := newExecutionFixture(t)
	ctx := t.Context()

	blank := channelExecution("blank-identity-trace")
	blank.OSUser = "  "
	err := fixture.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.executions.Append(txCtx, blank)
	})
	if err == nil {
		t.Fatalf("空进程属主被接受了")
	}
	if count := fixture.countTraces(t, ctx, "blank-identity-trace"); count != 0 {
		t.Fatalf("留痕行数 = %d，要 0", count)
	}
}
