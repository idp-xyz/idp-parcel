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
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func newExternalTrackingHandoffFixture(t *testing.T) (*adapter.OutboxExternalTrackingFactHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxExternalTrackingFactHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

// TestAJudgedVersionAndItsSuccessorShareAPartitionButNotAnID 证 ID 带版本（两代都入队）而分区键
// 只到载运对象（两代同队），与交付/交接两路同一条纪律。
func TestAJudgedVersionAndItsSuccessorShareAPartitionButNotAnID(t *testing.T) {
	handoff, db, pool := newExternalTrackingHandoffFixture(t)
	ctx := t.Context()
	judgment, _ := domain.JudgeEffectiveTimeExplicitly(trackingReceivedAtDB)
	first := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-H1", version: "EXTV-H1a", event: "evt-h1", effective: judgment})
	rejudged, err := first.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "EXTV-H1b"))
	if err != nil {
		t.Fatalf("再判断：%v", err)
	}
	second := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: first.Key.TenantID, Fact: first.Key.Fact, Version: rejudged.Version()},
		Fact:       rejudged,
		RecordedAt: first.RecordedAt.Add(time.Minute),
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffExternalTrackingFact(txCtx, ports.ExternalTrackingFactHandoffIntent{Record: first}); err != nil {
			return err
		}
		return handoff.HandOffExternalTrackingFact(txCtx, ports.ExternalTrackingFactHandoffIntent{Record: second})
	}); err != nil {
		t.Fatalf("两代入队：%v", err)
	}

	firstID := "tenant-1/EXTF-H1/EXTV-H1a/external-carrier-tracking"
	secondID := "tenant-1/EXTF-H1/EXTV-H1b/external-carrier-tracking"
	if count := countTFIntents(t, pool, firstID); count != 1 {
		t.Fatalf("首版行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, secondID); count != 1 {
		t.Fatalf("后版行数 = %d, want 1——ID 不带版本时它会被 EnqueueOnce 静默吞掉", count)
	}
	const wantPartition = "tenant-1/PCL-1/external-carrier-tracking"
	if got := partitionKeyOf(t, pool, firstID); got != wantPartition {
		t.Fatalf("首版分区键 = %q, want %q", got, wantPartition)
	}
	if got := partitionKeyOf(t, pool, secondID); got != wantPartition {
		t.Fatalf("后版分区键 = %q；同一对象的两版分了区就没有先后可言", got)
	}

	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, secondID).Scan(&raw); err != nil {
		t.Fatalf("读取载荷：%v", err)
	}
	var payload struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("译载荷：%v", err)
	}
	if payload.TenantID != "tenant-1" || payload.Fact != "EXTF-H1" || payload.Version != "EXTV-H1b" {
		t.Fatalf("载荷应指名自己那一代：%+v", payload)
	}
}

// TestExternalTrackingHandoffRefusesToRunOutsideATransaction 证意图与登记同生共死：无环境事务即拒。
func TestExternalTrackingHandoffRefusesToRunOutsideATransaction(t *testing.T) {
	handoff, _, _ := newExternalTrackingHandoffFixture(t)
	judgment, _ := domain.JudgeEffectiveTimeExplicitly(trackingReceivedAtDB)
	judged := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-H3", version: "EXTV-H3", event: "evt-h3", effective: judgment})

	err := handoff.HandOffExternalTrackingFact(t.Context(), ports.ExternalTrackingFactHandoffIntent{Record: judged})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestAPendingVersionIsRefusedAtTheHandoff 证待判断的版本没有可交的东西：适配器响亮拒绝，
// 不入队一份 VE 造不出 AcceptedSourceFact 的信封。
func TestAPendingVersionIsRefusedAtTheHandoff(t *testing.T) {
	handoff, db, pool := newExternalTrackingHandoffFixture(t)
	pending := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-H2", version: "EXTV-H2", event: "evt-h2"})

	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffExternalTrackingFact(txCtx, ports.ExternalTrackingFactHandoffIntent{Record: pending})
	})
	if err == nil {
		t.Fatal("待判断的版本入了队")
	}
	if count := countTFIntents(t, pool, "tenant-1/EXTF-H2/EXTV-H2/external-carrier-tracking"); count != 0 {
		t.Fatalf("待判断版本行数 = %d, want 0", count)
	}
}
