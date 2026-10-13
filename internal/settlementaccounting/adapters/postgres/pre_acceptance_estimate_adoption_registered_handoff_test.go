package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

// 本文件对真实 PostgreSQL 16 证「受理前估价采用已登记」意图（票 product-strategy-boundary/25 第 3 条）。事件类型与载荷键名在这里再写
// 一遍而不导入提供方常量，理由同「结算账户已登记」那份用例的头注：它们就是跨上下文契约。
const preAcceptanceEstimateAdoptionRegisteredEventType = "settlement-accounting.pre-acceptance-estimate-adoption.registered"

func newAdoptionRegisteredHandoffFixture(t *testing.T) (*adapter.OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxPreAcceptanceEstimateAdoptionRegisteredHandoff(db, store, saHandoffClock{at: settlementAccountEnqueuedAt})
	if err != nil {
		t.Fatalf("构造受理前估价采用已登记交接：%v", err)
	}
	return handoff, db
}

func adoptionRegisteredIntent(t *testing.T, accountID string) ports.PreAcceptanceEstimateAdoptionRegisteredIntent {
	t.Helper()
	return ports.PreAcceptanceEstimateAdoptionRegisteredIntent{
		Tenant:       settlementTenant(t),
		Account:      mustAccountRef(t, domain.NewSettlementAccountID, accountID),
		RegisteredAt: settlementAccountRegisteredAt,
	}
}

func adoptionRegisteredEventID(tenant, account string) string {
	return string(outboxintent.FingerprintEventID("pre-acceptance-estimate-adoption", tenant, account))
}

// Covers: 跨上下文契约——载荷与「结算账户已登记」同一份两键（租户、登记种类），形态与计价范围不进信封；账户只进 ID 与 Subject；
// 分区键装租户。
func TestAnAdoptionRegisteredIntentCarriesExactlyTheContract(t *testing.T) {
	handoff, db := newAdoptionRegisteredHandoffFixture(t)
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(txCtx, adoptionRegisteredIntent(t, "ACCT-1"))
	}); err != nil {
		t.Fatalf("入队：%v", err)
	}

	envelope := claimSettlementAccountRegisteredEnvelope(t, db, adoptionRegisteredEventID("tenant-a", "ACCT-1"))
	if string(envelope.Type) != preAcceptanceEstimateAdoptionRegisteredEventType || envelope.Source != settlementAccountRegisteredSource {
		t.Fatalf("事件类型 = %q 来源 = %q", envelope.Type, envelope.Source)
	}
	if envelope.Scope != "tenant-a" || envelope.PartitionKey != "tenant-a" {
		t.Fatalf("范围 = %q，分区键 = %q，want 都是租户", envelope.Scope, envelope.PartitionKey)
	}
	if envelope.Subject != "PRE_ACCEPTANCE_ESTIMATE_ADOPTION/ACCT-1" {
		t.Fatalf("主题 = %q", envelope.Subject)
	}
	if !envelope.OccurredAt.Equal(settlementAccountRegisteredAt) || !envelope.RecordedAt.Equal(settlementAccountEnqueuedAt) {
		t.Fatalf("发生时刻 = %s 入队时刻 = %s", envelope.OccurredAt, envelope.RecordedAt)
	}
	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("载荷不是平面 JSON 对象：%v", err)
	}
	if len(payload) != 2 || payload["tenantId"] != "tenant-a" || payload["registrationKind"] != "PRE_ACCEPTANCE_ESTIMATE_ADOPTION" {
		t.Fatalf("载荷 = %v，want 只有租户与登记种类两键", payload)
	}
}

// Covers: PBC-08 行为面——写口在无事务上下文必须被 RequireExecutor 拒绝。
func TestAnAdoptionRegisteredIntentRefusesToRunOutsideATransaction(t *testing.T) {
	handoff, _ := newAdoptionRegisteredHandoffFixture(t)
	err := handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(t.Context(), adoptionRegisteredIntent(t, "ACCT-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 账户标识长到主题过不了框架上限时拒收是确定性的、交确定性拒收哨兵；缺件的意图响亮报错。
func TestAnOverlongOrIncompleteAdoptionRegisteredIntentIsRefused(t *testing.T) {
	handoff, db := newAdoptionRegisteredHandoffFixture(t)
	overlong := "ACCT-" + strings.Repeat("x", eventing.MaxSubjectLength)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(txCtx, adoptionRegisteredIntent(t, overlong))
	})
	if !errors.Is(err, ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected) {
		t.Fatalf("超长账户应交回确定性拒收哨兵，实得：%v", err)
	}
	complete := adoptionRegisteredIntent(t, "ACCT-1")
	for name, intent := range map[string]ports.PreAcceptanceEstimateAdoptionRegisteredIntent{
		"零值租户": {Account: complete.Account, RegisteredAt: complete.RegisteredAt},
		"零值账户": {Tenant: complete.Tenant, RegisteredAt: complete.RegisteredAt},
		"零值时刻": {Tenant: complete.Tenant, Account: complete.Account},
	} {
		err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(txCtx, intent)
		})
		if err == nil || errors.Is(err, ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected) {
			t.Errorf("%s：err = %v，必须响亮报错且不是拒收哨兵", name, err)
		}
	}
}
