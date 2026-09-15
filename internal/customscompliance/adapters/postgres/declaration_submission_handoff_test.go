package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证申报提交意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺幂等键响亮报错。信封 ID 由幂等键认领。入队走 EnqueueOnce。

type declarationHandoffClock struct{ at time.Time }

func (clock declarationHandoffClock) Now() time.Time { return clock.at }

type declarationHandoffFixture struct {
	submissions *adapter.DeclarationSubmissions
	handoff     *adapter.OutboxDeclarationSubmissionHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newDeclarationHandoffFixture(t *testing.T) *declarationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submissions, err := adapter.NewDeclarationSubmissions(db)
	if err != nil {
		t.Fatalf("构造申报链库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxDeclarationSubmissionHandoff(db, store, declarationHandoffClock{
		at: time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &declarationHandoffFixture{
		submissions: submissions,
		handoff:     handoff,
		transactor:  db.Transactor(),
		pool:        pool,
	}
}

func (fixture *declarationHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func declarationIntent(t *testing.T, tenant, unit, procedure, version string) ports.DeclarationSubmissionHandoffIntent {
	t.Helper()
	return ports.DeclarationSubmissionHandoffIntent{
		Record: submissionRecord(t, tenant, unit, procedure, version),
		// 与 submissionRecord 里单元所属的案件同值：意图的案件维就是单元身份上那一个。
		Case: declarationValue(t, domain.NewCustomsCaseID, "case-1"),
	}
}

// declarationEventID 与被测拼法同构：目标三维加版本维——原案内更正在同一目标下换版
// 出第二封，ID 不带版本维时第二封会被 EnqueueOnce 静默吞掉。
// declarationEventID 按生产同一公式重算信封 ID（票 sa-cc/34 裁决 3：口名 + 目标键三维 + 版本全进哈希）。
func declarationEventID(tenant, unit, procedure, version string) string {
	return string(outboxintent.FingerprintEventID("declaration-submission", tenant, unit, procedure, version))
}

// Covers: 票 sa-cc/34 判据 (1)——单元、程序与版本三个引用维取到旧串接形必然超过 eventing.MaxEventIDLength 的长度
// （引用多长归实例半边，本仓给不出上界），信封仍入队成功、ID 定长在上限内、重发同一份仍一行。分区键取目标三维、
// 不含版本，本格三维之和仍在 eventing.MaxPartitionKeyLength 内——证的是 ID 那一维。
func TestOverlongDeclarationReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	long := func(prefix string) string { return prefix + "-" + strings.Repeat("x", 60) }
	unit, procedure, version := long("unit"), long("procedure"), long("version")
	concatenated := "tenant-a/" + unit + "/" + procedure + "/" + version
	if len(concatenated) <= eventing.MaxEventIDLength {
		t.Fatalf("夹具没造出超长：串接形 %d 字节没超过上限 %d", len(concatenated), eventing.MaxEventIDLength)
	}
	intent := declarationIntent(t, "tenant-a", unit, procedure, version)

	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent) })
	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent) })

	eventID := declarationEventID("tenant-a", unit, procedure, version)
	if len(eventID) > eventing.MaxEventIDLength || len(eventID) != len("declaration-submission/")+64 {
		t.Fatalf("ID = %q（%d 字节）；该是口名前缀加六十四位十六进制、在上限 %d 内", eventID, len(eventID), eventing.MaxEventIDLength)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("超长引用下 outbox 行数 = %d，want 1", count)
	}
}

func TestDeclarationSubmissionIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1", "version-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})

	if _, exists, err := fixture.submissions.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := declarationIntentType(t, fixture.pool, eventID); got != "customs-compliance.declaration-submission.formed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
	// 载荷带案件维（ADR-0073 决定五）：下游译码把它缺席判毒丸，这里钉住它真的在场。
	var payload []byte
	if err := fixture.pool.QueryRow(ctx,
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&payload); err != nil {
		t.Fatalf("读意图载荷：%v", err)
	}
	var body struct {
		CaseID string `json:"caseId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("解意图载荷：%v", err)
	}
	if body.CaseID != "case-1" {
		t.Fatalf("载荷案件维 = %q：%s", body.CaseID, payload)
	}
}

func TestDeclarationSubmissionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1", "version-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffDeclarationSubmission(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.submissions.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameDeclarationSubmissionIntentIsIdempotent(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1", "version-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestDeclarationSubmissionIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1", "version-1")
	if err := fixture.handoff.HandOffDeclarationSubmission(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignDeclarationSubmissionIntentIsLoud(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, ports.DeclarationSubmissionHandoffIntent{})
	}); err == nil {
		t.Fatal("缺幂等键的意图必须响亮报错")
	}

	// 缺案件维同样响亮（ADR-0073 决定五必填）：静默发出去会在下游译码处变毒丸。
	missingCase := ports.DeclarationSubmissionHandoffIntent{
		Record: submissionRecord(t, "tenant-a", "unit-9", "export-procedure/v1", "version-9"),
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, missingCase)
	}); err == nil {
		t.Fatal("缺案件维的意图必须响亮报错")
	}
}

func countDeclarationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.declaration-submission.formed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func declarationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var eventType string
	err := pool.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&eventType)
	if err != nil {
		t.Fatalf("读事件类型：%v", err)
	}
	return eventType
}
