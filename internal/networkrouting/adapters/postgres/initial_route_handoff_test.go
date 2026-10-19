package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newInitialRouteHandoffFixture(t *testing.T) (*adapter.OutboxInitialRouteHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxInitialRouteHandoff(db, store, handoffTestClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func initialRouteHandoffIntent(t *testing.T, parcel, corr string) ports.InitialRouteHandoffIntent {
	t.Helper()
	key := routeKey(t, "tenant-a", parcel)
	return ports.InitialRouteHandoffIntent{
		Correlation: scalar(t, domain.NewRequestCorrelationID, corr),
		Record: ports.InitialRouteRecord{
			Key:     key,
			Plan:    formedPlan(t, key, "RPV-0001"),
			HasPlan: true,
		},
	}
}

func initialRouteEventID(parcel string) string {
	return string(outboxintent.FingerprintEventID("initial-route",
		"tenant-a", "customer-a", "request-1", parcel, "baseline-v1", "LAST_MILE_DELIVERY"))
}

// Covers: 信封 ID 不随判断键各维的长度涨——演示委托的真实标识（委托、包裹、提交版本各是前缀加二十六位编码）原样
// 串接就越过 eventing.MaxEventIDLength，入队被拒、交接只剩没人重发的续办引用。指纹形照样入队；分区键保留可读的
// 串接形，顺序语义不变。
func TestAnInitialRouteWithFullLengthIdentifiersStillEnqueues(t *testing.T) {
	handoff, db, pool := newInitialRouteHandoffFixture(t)
	ctx := t.Context()
	key := domain.InitialRouteJudgmentKey{
		TenantID:           scalar(t, domain.NewTenantID, "SYN-TENANT-01"),
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "SYN-ACCOUNT-01"),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "SHR-6JGY53O5J6G2INJ55GZBYPOKOA"),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "SUBV-V3KCF4UQ5EKYLMK5OYB57LPGJ4"),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, "DPCL-VSW4NO6ZJE2BXSYAUSOOTCD2DU"),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
	}
	readable := "SYN-TENANT-01/initial-route/SHR-6JGY53O5J6G2INJ55GZBYPOKOA/DPCL-VSW4NO6ZJE2BXSYAUSOOTCD2DU/" +
		"SUBV-V3KCF4UQ5EKYLMK5OYB57LPGJ4/NETWORK_SERVICE"
	if len(readable) <= eventing.MaxEventIDLength {
		t.Fatalf("夹具不够长：%d 字节没越过信封 ID 上限，测不到这一格", len(readable))
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, ports.InitialRouteHandoffIntent{
			Correlation: scalar(t, domain.NewRequestCorrelationID, "corr-long"),
			Record:      ports.InitialRouteRecord{Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true},
		})
	}); err != nil {
		t.Fatalf("真实长度的判断键入不了队：%v", err)
	}
	var id, partition string
	if err := pool.QueryRow(ctx,
		`SELECT event_id, partition_key FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'network-routing.initial-route.formed'`,
	).Scan(&id, &partition); err != nil {
		t.Fatalf("读回信封：%v", err)
	}
	if len(id) > eventing.MaxEventIDLength || partition != readable {
		t.Fatalf("id = %q（%d 字节），partition = %q", id, len(id), partition)
	}
}

// TestInitialRouteFollowsTheTransactionalTemplate 证初始路由意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由判断键认领并加类型段。
func TestInitialRouteFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newInitialRouteHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-1", "corr-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-1")); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-rollback", "corr-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-rollback")); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-1", "corr-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffInitialRoute(ctx, initialRouteHandoffIntent(t, "parcel-ntx", "corr-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestInitialRoutePayloadCarriesTheCompleteJudgmentKey 证载荷带齐判断键全部六维——
// 尤其 customerAccountId：FindByKey 的主键含 customer_account_id，载荷缺它，任何
// 消费方都拼不出能命中的完整键。
func TestInitialRoutePayloadCarriesTheCompleteJudgmentKey(t *testing.T) {
	handoff, db, pool := newInitialRouteHandoffFixture(t)
	ctx := t.Context()

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-key", "corr-key"))
	}); err != nil {
		t.Fatalf("入队：%v", err)
	}

	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		initialRouteEventID("parcel-key"),
	).Scan(&raw); err != nil {
		t.Fatalf("读回载荷：%v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("译载荷：%v", err)
	}
	want := map[string]string{
		"tenantId":          "tenant-a",
		"customerAccountId": "customer-a",
		"shipment":          "request-1",
		"parcel":            "parcel-key",
		"baseline":          "baseline-v1",
		"purpose":           "LAST_MILE_DELIVERY",
		"correlation":       "corr-key",
	}
	for field, value := range want {
		if payload[field] != value {
			t.Errorf("payload[%q] = %q, want %q", field, payload[field], value)
		}
	}
}

func TestInitialRouteRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newInitialRouteHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, ports.InitialRouteHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺判断键的初始路由意图入了队")
	}
}
