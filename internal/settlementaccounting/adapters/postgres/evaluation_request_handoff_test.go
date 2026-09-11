package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉票 sa-cc/08 完成判据 3 的 Outbox 半边：事件类型 settlement-accounting.evaluation-request.submitted；
// 分区主体「租户/评价请求」（裁决 1 与 ADR-0069 决定二：ID 管幂等、分区键管顺序）；载荷只带
// {tenantId, evaluationRequestId}——三件来源引用由消费方按 ID 读登记册，信封里不抄第二份；同一份意图
// 重交由 EnqueueOnce 认领吞掉、不翻倍；意图复现样板四条（首发一行 / 回滚无痕 / 重发同一份 / 无事务拒）。

func newEvaluationRequestHandoffFixture(t *testing.T) (*adapter.OutboxEvaluationRequestHandoff, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxEvaluationRequestHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func evaluationRequestIntent(t *testing.T, id string) ports.EvaluationRequestIntent {
	t.Helper()
	return ports.EvaluationRequestIntent{Record: evaluationRequestRecord(t, "tenant-a", id, "syn-fee-1")}
}

// evaluationRequestEnvelopePayload 比生产载荷多列了几个**不该出现**的键：解出来非零就是信封抄了登记册的
// 内容（票面红线：载荷只带引用）。
type evaluationRequestEnvelopePayload struct {
	TenantID            string `json:"tenantId"`
	EvaluationRequestID string `json:"evaluationRequestId"`
	Scope               string `json:"scope"`
	FeeItem             string `json:"feeItem"`
	Agreement           string `json:"agreement"`
	Occurrence          string `json:"occurrence"`
	Digest              string `json:"sourceDigest"`
}

func evaluationRequestPayloadOf(t *testing.T, pool *pgxpool.Pool, eventID string) evaluationRequestEnvelopePayload {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&raw); err != nil {
		t.Fatalf("读载荷：%v", err)
	}
	var payload evaluationRequestEnvelopePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	return payload
}

// Covers: 判据 3「入队一封、分区键「租户 / 评价请求」」与做法 2「载荷只带引用 {tenantId, evaluationRequestId}」。
func TestASubmittedEvaluationRequestEnqueuesOneReferenceOnlyEnvelope(t *testing.T) {
	handoff, db, pool := newEvaluationRequestHandoffFixture(t)
	ctx := t.Context()

	intent := evaluationRequestIntent(t, "EVREQ-SYN-1")
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEvaluationRequest(txCtx, intent)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}

	partition := "tenant-a/evaluation-request/EVREQ-SYN-1"
	eventID := partition + "/submitted"
	if count := countSAIntents(t, pool, eventID); count != 1 {
		t.Fatalf("行数 = %d, want 1", count)
	}
	if got := settlementApplicationIntentType(t, pool, eventID); got != "settlement-accounting.evaluation-request.submitted" {
		t.Fatalf("事件类型 = %q, want settlement-accounting.evaluation-request.submitted", got)
	}
	if got := partitionKeyOf(t, pool, eventID); got != partition {
		t.Fatalf("分区键 = %q, want %q——主体是评价请求，状态段只进 ID", got, partition)
	}

	payload := evaluationRequestPayloadOf(t, pool, eventID)
	if payload.TenantID != "tenant-a" || payload.EvaluationRequestID != "EVREQ-SYN-1" {
		t.Fatalf("载荷引用 = %+v", payload)
	}
	if payload.Scope != "" || payload.FeeItem != "" || payload.Agreement != "" || payload.Occurrence != "" || payload.Digest != "" {
		t.Fatalf("载荷不得带主要范围、三件引用或摘要（消费方按 ID 读登记册），实得 %+v", payload)
	}
}

// Covers: 判据 3「`EnqueueOnce` 同键不翻倍」与样板其余三条：首发一行、回滚无痕、无事务拒。
func TestEvaluationRequestFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newEvaluationRequestHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	first := evaluationRequestIntent(t, "EVREQ-SYN-3")
	firstID := "tenant-a/evaluation-request/EVREQ-SYN-3/submitted"
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEvaluationRequest(txCtx, first)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, firstID); count != 1 {
		t.Fatalf("EVREQ-SYN-3 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	second := evaluationRequestIntent(t, "EVREQ-SYN-4")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffEvaluationRequest(txCtx, second); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/evaluation-request/EVREQ-SYN-4/submitted"); count != 0 {
		t.Fatalf("回滚后 EVREQ-SYN-4 行数 = %d, want 0", count)
	}

	// 重放交的是同一份（同租户、同请求 ID）：编排在`已存在`那格照样调交接口，不翻倍靠这里的认领。
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEvaluationRequest(txCtx, first)
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, firstID); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffEvaluationRequest(ctx, evaluationRequestIntent(t, "EVREQ-SYN-5")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 两份不同的请求各自成区：分区主体是评价请求本身，两份请求之间没有先后可言。
func TestTwoEvaluationRequestsLandInTheirOwnPartitions(t *testing.T) {
	handoff, db, pool := newEvaluationRequestHandoffFixture(t)

	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if err := handoff.HandOffEvaluationRequest(txCtx, evaluationRequestIntent(t, "EVREQ-SYN-6")); err != nil {
			return err
		}
		return handoff.HandOffEvaluationRequest(txCtx, evaluationRequestIntent(t, "EVREQ-SYN-7"))
	}); err != nil {
		t.Fatalf("两份入队：%v", err)
	}
	six := partitionKeyOf(t, pool, "tenant-a/evaluation-request/EVREQ-SYN-6/submitted")
	seven := partitionKeyOf(t, pool, "tenant-a/evaluation-request/EVREQ-SYN-7/submitted")
	if six == seven {
		t.Fatalf("两份请求共用了分区 %q", six)
	}
}

func TestEvaluationRequestHandoffRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newEvaluationRequestHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffEvaluationRequest(txCtx, ports.EvaluationRequestIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的评价请求意图入了队")
	}
}
