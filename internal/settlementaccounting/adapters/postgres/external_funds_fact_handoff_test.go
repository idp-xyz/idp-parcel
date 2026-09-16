package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉票 sa-cc/02 的 Outbox 半边：事件类型 settlement-accounting.external-funds-fact.adopted；分区主体
// 「租户/资金事实」（裁决 1：更正链是同一事实的版本链，排一条队）；ID 带版本维（同一事实的每个
// 版本各自入队，一个都不丢），自票 sa-cc/32 起是（租户 / 事实 / 版本）三维的定长指纹形，不再是可读串接
// （引用多长归实例半边，串接形把 eventing.MaxEventIDLength 押在别人的长度上）；更正版本同型再发一封、
// 载荷回指前一版本（裁决 2）；载荷只带引用不带金额（票面红线）。

// externalFundsFactEventID 按生产同一公式重算信封 ID（口名 funds-fact 与分区键口名段同词）：用例按 ID 查库，
// 断的是「这一封能按（租户 / 事实 / 版本）重算出来找到」，不断指纹字面。
func externalFundsFactEventID(tenant, fact, version string) string {
	return string(outboxintent.FingerprintEventID("funds-fact", tenant, fact, version))
}

func countSAIntentsInPartition(t *testing.T, pool *pgxpool.Pool, partitionKey string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE partition_key = $1`, partitionKey,
	).Scan(&count); err != nil {
		t.Fatalf("按分区键统计 outbox 行数：%v", err)
	}
	return count
}

// Covers: 票 sa-cc/32 完成判据 (1)——信封 ID 定长且不超 eventing.MaxEventIDLength：三个引用拼起来在旧串接形下
// 必超 128 字节的合成事实引用，照样入队成功；同一版本两次算出同一个 ID（第二次被 EnqueueOnce 按 ID 吞掉、不成
// 第二封）；分区键保留可读形不哈希（ID 管幂等、分区键管顺序，运维从分区键与载荷反查）。合成引用长度不假定任何
// 租户的引用多长，只保证「串接形下超上限」这一件。
func TestExternalFundsFactEnvelopeIDIsAFixedLengthFingerprintWithinTheFrameworkLimit(t *testing.T) {
	handoff, db, pool := newExternalFundsFactHandoffFixture(t)
	ctx := t.Context()

	longFact := "bank-fact-long-" + strings.Repeat("x", eventing.MaxEventIDLength)
	intent := externalFundsFactIntent(t, longFact)
	version := intent.Record.Fact.Version().String()
	if len("tenant-a/funds-fact/"+longFact+"/"+version) <= eventing.MaxEventIDLength {
		t.Fatal("夹具没把串接形顶过上限，这条用例证不了它要证的东西")
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, intent)
	}); err != nil {
		t.Fatalf("超长引用首发：%v", err)
	}

	eventID := externalFundsFactEventID("tenant-a", longFact, version)
	if len(eventID) > eventing.MaxEventIDLength {
		t.Fatalf("信封 ID 长 %d 字节，超过 eventing.MaxEventIDLength %d", len(eventID), eventing.MaxEventIDLength)
	}
	if count := countSAIntents(t, pool, eventID); count != 1 {
		t.Fatalf("按重算的指纹 ID 查到 %d 行, want 1", count)
	}
	if got := partitionKeyOf(t, pool, eventID); got != "tenant-a/funds-fact/"+longFact {
		t.Fatalf("分区键 = %q, want 可读形「租户/funds-fact/事实」——分区键不哈希", got)
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, intent)
	}); err != nil {
		t.Fatalf("同一版本重发：%v", err)
	}
	if count := countSAIntentsInPartition(t, pool, "tenant-a/funds-fact/"+longFact); count != 1 {
		t.Fatalf("同一版本两次交接后分区里 %d 封, want 1——同输入必算出同一个 ID", count)
	}
}

// Covers: 票 sa-cc/32 裁决 2 (a) 的适配器半边——框架 Envelope.Validate 的确定性拒收（这里以事实引用把 Subject 顶过
// eventing.MaxSubjectLength 触发；ID 已是定长形，今天能到这一格的正是 Subject / PartitionKey）要以
// ports.ErrFundsFactHandoffRejected 交出来、原因链保留（errors.Is eventing.ErrInvalidEnvelope），且一行不落：编排据
// 此整笔不作答，而不是折成一个重跑永远补不上的续办引用。
func TestARejectedExternalFundsFactEnvelopeIsReportedAsRejectedAndLeavesNoRow(t *testing.T) {
	handoff, db, pool := newExternalFundsFactHandoffFixture(t)

	overlongFact := "bank-fact-" + strings.Repeat("x", eventing.MaxSubjectLength)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, externalFundsFactIntent(t, overlongFact))
	})
	if !errors.Is(err, ports.ErrFundsFactHandoffRejected) {
		t.Fatalf("err = %v, want errors.Is ports.ErrFundsFactHandoffRejected", err)
	}
	if !errors.Is(err, eventing.ErrInvalidEnvelope) {
		t.Fatalf("err = %v, want 保留框架拒收原因链（errors.Is eventing.ErrInvalidEnvelope）", err)
	}
	if count := countSAIntentsInPartition(t, pool, "tenant-a/funds-fact/"+overlongFact); count != 0 {
		t.Fatalf("被拒的信封落了 %d 行", count)
	}
}

func newExternalFundsFactHandoffFixture(t *testing.T) (*adapter.OutboxExternalFundsFactHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxExternalFundsFactHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func externalFundsFactIntent(t *testing.T, id string) ports.ExternalFundsFactIntent {
	t.Helper()
	return ports.ExternalFundsFactIntent{
		Record: adoptedFactRecord(t, "tenant-a", id, domain.FundsReceiptConfirmed, 8000),
	}
}

type externalFundsFactEnvelopePayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
	Corrects string `json:"corrects"`
	Amount   *int64 `json:"amountMinor"`
	Currency string `json:"currency"`
}

func externalFundsFactPayloadOf(t *testing.T, pool *pgxpool.Pool, eventID string) externalFundsFactEnvelopePayload {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&raw); err != nil {
		t.Fatalf("读载荷：%v", err)
	}
	var payload externalFundsFactEnvelopePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	return payload
}

// TestAnAdoptedFundsFactEnqueuesOneReferenceOnlyEnvelope 钉首发的形：一封、类型、分区主体
// 「租户/资金事实」、载荷只带租户 / 事实 / 版本——不带金额与币种（消费方按引用读 SA）。
func TestAnAdoptedFundsFactEnqueuesOneReferenceOnlyEnvelope(t *testing.T) {
	handoff, db, pool := newExternalFundsFactHandoffFixture(t)
	ctx := t.Context()

	intent := externalFundsFactIntent(t, "bank-fact-1")
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, intent)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}

	partition := "tenant-a/funds-fact/bank-fact-1"
	eventID := externalFundsFactEventID("tenant-a", "bank-fact-1", intent.Record.Fact.Version().String())
	if count := countSAIntents(t, pool, eventID); count != 1 {
		t.Fatalf("行数 = %d, want 1", count)
	}
	if got := settlementApplicationIntentType(t, pool, eventID); got != "settlement-accounting.external-funds-fact.adopted" {
		t.Fatalf("事件类型 = %q, want settlement-accounting.external-funds-fact.adopted", got)
	}
	if got := partitionKeyOf(t, pool, eventID); got != partition {
		t.Fatalf("分区键 = %q, want %q——主体是资金事实，不带版本维", got, partition)
	}

	payload := externalFundsFactPayloadOf(t, pool, eventID)
	if payload.TenantID != "tenant-a" || payload.Fact != "bank-fact-1" || payload.Version != intent.Record.Fact.Version().String() {
		t.Fatalf("载荷引用 = %+v", payload)
	}
	if payload.Corrects != "" {
		t.Fatalf("首版不该带回指，实得 %q", payload.Corrects)
	}
	if payload.Amount != nil || payload.Currency != "" {
		t.Fatalf("载荷不得带金额或币种（消费方按引用读 SA），实得 %+v", payload)
	}
}

// TestACorrectionVersionEnqueuesItsOwnEnvelopeInTheSamePartition 钉裁决 2：更正是新版本，同一事件
// 类型再发一封（ID 带版本维不被 EnqueueOnce 吞）、与原版本同区（更正排在它更正的那一版之后）、
// 载荷回指前一版本。
func TestACorrectionVersionEnqueuesItsOwnEnvelopeInTheSamePartition(t *testing.T) {
	handoff, db, pool := newExternalFundsFactHandoffFixture(t)
	ctx := t.Context()

	original := externalFundsFactIntent(t, "bank-fact-2")
	correctedFact, err := original.Record.Fact.CorrectAmount(
		9000,
		saValue(t, domain.NewFundsFactVersion, "bank-fact-2/v2"),
		original.Record.Fact.OccurredAt().Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("更正金额：%v", err)
	}
	corrected := original
	corrected.Record.Fact = correctedFact
	corrected.Record.RecordedAt = original.Record.RecordedAt.Add(2 * time.Hour)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffExternalFundsFact(txCtx, original); err != nil {
			return err
		}
		return handoff.HandOffExternalFundsFact(txCtx, corrected)
	}); err != nil {
		t.Fatalf("原版与更正版入队：%v", err)
	}

	partition := "tenant-a/funds-fact/bank-fact-2"
	originalID := externalFundsFactEventID("tenant-a", "bank-fact-2", original.Record.Fact.Version().String())
	correctedID := externalFundsFactEventID("tenant-a", "bank-fact-2", "bank-fact-2/v2")
	if count := countSAIntents(t, pool, originalID); count != 1 {
		t.Fatalf("原版行数 = %d, want 1", count)
	}
	if count := countSAIntents(t, pool, correctedID); count != 1 {
		t.Fatalf("更正版行数 = %d, want 1——ID 不带版本维时它会被 EnqueueOnce 静默吞掉", count)
	}
	if got := settlementApplicationIntentType(t, pool, correctedID); got != "settlement-accounting.external-funds-fact.adopted" {
		t.Fatalf("更正版事件类型 = %q, want 同一类型（更正是新事实回指，不另开类型）", got)
	}
	if got := partitionKeyOf(t, pool, correctedID); got != partition {
		t.Fatalf("更正版分区键 = %q, want %q——两版不同分区就没有先后可言", got, partition)
	}
	if got := externalFundsFactPayloadOf(t, pool, correctedID).Corrects; got != original.Record.Fact.Version().String() {
		t.Fatalf("更正版载荷回指 = %q, want %q", got, original.Record.Fact.Version().String())
	}
}

// TestExternalFundsFactFollowsTheTransactionalTemplate 证意图复现样板四条：首发一行、回滚无痕、
// 重发同一份、无事务拒。
func TestExternalFundsFactFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newExternalFundsFactHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	first := externalFundsFactIntent(t, "bank-fact-3")
	firstID := externalFundsFactEventID("tenant-a", "bank-fact-3", first.Record.Fact.Version().String())
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, first)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, firstID); count != 1 {
		t.Fatalf("bank-fact-3 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	second := externalFundsFactIntent(t, "bank-fact-4")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffExternalFundsFact(txCtx, second); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, externalFundsFactEventID("tenant-a", "bank-fact-4", second.Record.Fact.Version().String())); count != 0 {
		t.Fatalf("回滚后 bank-fact-4 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, first)
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, firstID); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffExternalFundsFact(ctx, externalFundsFactIntent(t, "bank-fact-5")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestExternalFundsFactRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newExternalFundsFactHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffExternalFundsFact(txCtx, ports.ExternalFundsFactIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的资金事实意图入了队")
	}
}
