package main

import (
	"context"
	"testing"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type estimateAdoptionBeatClock struct{}

func (estimateAdoptionBeatClock) Now() time.Time { return beatInstant() }

// Covers: 路由表「受理前估价采用已登记」那一份登记续办门（票 product-strategy-boundary/25），且它在生产依赖图上接得起来：信封由
// settlement-accounting 真交接适配器铸出（类型与载荷两边各写各的字面，这里对上），一拍投给续办门，载荷译出租户去问真队列读口，
// 该租户没有委托在等，本封入账定稿。漏挂的话，采用登记事务铸出的每一封都撞 dispatch.no_subscriber。
func TestARegisteredEstimateAdoptionReachesItsResumeGateOnTheProductionGraph(t *testing.T) {
	beat, db, store := wiredBeat(t)
	handoff, err := sapostgres.NewOutboxPreAcceptanceEstimateAdoptionRegisteredHandoff(db, store, estimateAdoptionBeatClock{})
	if err != nil {
		t.Fatalf("构造采用已登记交接：%v", err)
	}
	tenant, err := sadomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	account, err := sadomain.NewSettlementAccountID("SYN-SETTLEMENT-ACCOUNT-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(txCtx, saports.PreAcceptanceEstimateAdoptionRegisteredIntent{
			Tenant: tenant, Account: account, RegisteredAt: beatInstant(),
		})
	}); err != nil {
		t.Fatalf("铸封：%v", err)
	}
	eventID := string(outboxintent.FingerprintEventID("pre-acceptance-estimate-adoption", "SYN-TENANT-01", "SYN-SETTLEMENT-ACCOUNT-01"))
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var eventType string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID).Scan(&eventType); err != nil {
		t.Fatalf("读回信封：%v", err)
	}
	if eventType != string(psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType) {
		t.Fatalf("发布侧写 %q，消费侧认 %q", eventType, psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType)
	}

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把受理前估价采用已登记投给它的续办门",
			published, recordedFailureCode(t, db, eventID))
	}
}
