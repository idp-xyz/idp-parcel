package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 揽收更正口的第二参是真编排（票 tf-segment-lifecycle-closure/08）——首登口与更正口各自装配、
// 共用同一册：首登经控制事实那一格落 v1，更正经本格在真实 PostgreSQL 上落 v2 回指 v1、原行不动、
// 按键读回当前版是 v2；意图走真 Outbox 入了第二份（更正版本带版本段的 ID）；重放走已有版本，证首笔
// 事务真的提交了。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	correction, err := buildOffsitePickupCorrectionOrchestration(db)
	if err != nil {
		t.Fatalf("装配揽收更正编排：%v", err)
	}
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		t.Fatalf("揽收登记册读面：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-8")
	occurredAt := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)

	registered, err := controlFacts.pickupRegistration.Register(t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:   tenant,
		Object:     "SYN-PARCEL-8",
		Task:       "SYN-PICKUP-TASK-8",
		Attempt:    "SYN-ATTEMPT-8",
		Place:      "SYN-DOOR-8",
		Control:    "SYN-TRANSPORT-CONTROL-8",
		ExecutedBy: "SYN-COURIER-8",
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.PickupRegistered {
		t.Fatalf("outcome = %v, want PICKUP_REGISTERED", got)
	}
	first, _ := registered.Record()

	command := tfapp.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             "SYN-PARCEL-8",
		Attempt:            "SYN-ATTEMPT-8",
		PredecessorVersion: first.Pickup.Version().String(),
		Place:              "SYN-DOOR-8-RECHECK",
		Control:            "SYN-TRANSPORT-CONTROL-8-RECHECK",
		ExecutedBy:         "SYN-COURIER-9",
		OccurredAt:         occurredAt.Add(-time.Hour),
		CorrectedAt:        time.Now().UTC().Add(time.Minute),
	}
	corrected, err := correction.Correct(t.Context(), command)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.PickupCorrected {
		t.Fatalf("outcome = %v, want PICKUP_CORRECTED", got)
	}
	if ref := corrected.PickupHandoffReference(); ref != "" {
		t.Fatalf("意图交付走真 Outbox 应当成功，却留了续办引用 %q", ref)
	}
	record, has := corrected.Record()
	if !has {
		t.Fatal("更正成功却没带回登记")
	}
	if predecessor, ok := record.Pickup.Corrects(); !ok || predecessor != first.Pickup.Version() {
		t.Fatalf("corrects = (%q, %v)，版本链没回指前版 %q", predecessor.String(), ok, first.Pickup.Version())
	}
	if record.Pickup.Version() == first.Pickup.Version() {
		t.Fatal("更正沿用了原版本号")
	}

	current, found, err := registrations.FindByKey(t.Context(), tfports.OffsitePickupKey{
		TenantID: tenant,
		Object:   mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-8"),
		Attempt:  mustValue(t, tfdomain.NewAttemptReference, "SYN-ATTEMPT-8"),
	})
	if err != nil || !found {
		t.Fatalf("按键读回当前版：found=%v err=%v", found, err)
	}
	if current.Pickup.Version() != record.Pickup.Version() || current.Pickup.Control().String() != "SYN-TRANSPORT-CONTROL-8-RECHECK" {
		t.Fatalf("当前版 = %+v，want 更正版本", current.Pickup)
	}

	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-8' AND attempt_ref = 'SYN-ATTEMPT-8'`,
		tenant.String()).Scan(&rows); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("同键行数 = %d, want 2（原行不动，更正是新行）", rows)
	}

	correctionEventID := tenant.String() + "/SYN-PARCEL-8/SYN-ATTEMPT-8/" + record.Pickup.Version().String() + "/offsite-pickup-registration"
	var enqueued int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		correctionEventID).Scan(&enqueued); err != nil {
		t.Fatalf("数 Outbox：%v", err)
	}
	if enqueued != 1 {
		t.Fatalf("更正版本意图入队 %d 份, want 1——PS 收不到新版本", enqueued)
	}

	replay, err := correction.Correct(t.Context(), command)
	if err != nil {
		t.Fatalf("重放更正：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.PickupExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}

	// 裁决点名的拒绝格（无前版、早于被更正版本的登记时刻、沿用已有版本号）在真库装配上同样成立，且都不落行、不入队。
	t.Run("a correction without a registered predecessor is not accepted", func(t *testing.T) {
		orphan := command
		orphan.Object = "SYN-PARCEL-NEVER-REGISTERED"
		result, err := correction.Correct(t.Context(), orphan)
		if err != nil {
			t.Fatalf("更正无中生有：%v", err)
		}
		if got := result.Outcome(); got != tfapp.PickupRegistrationNotAccepted {
			t.Fatalf("outcome = %v, want SOURCE_NOT_ACCEPTED（更正不出无中生有的揽收）", got)
		}
		if rows := pickupRowsFor(t, pool, tenant.String(), "SYN-PARCEL-NEVER-REGISTERED"); rows != 0 {
			t.Fatalf("无中生有的更正落了 %d 行", rows)
		}
	})

	t.Run("a correction dated before the registration it corrects is not accepted", func(t *testing.T) {
		early := command
		early.PredecessorVersion = record.Pickup.Version().String()
		early.CorrectedAt = current.RecordedAt.Add(-time.Second)
		result, err := correction.Correct(t.Context(), early)
		if err != nil {
			t.Fatalf("早于登记的更正：%v", err)
		}
		if got := result.Outcome(); got != tfapp.PickupRegistrationNotAccepted {
			t.Fatalf("outcome = %v, want SOURCE_NOT_ACCEPTED（更正时刻不得早于被更正版本的登记时刻）", got)
		}
		if rows := pickupRowsFor(t, pool, tenant.String(), "SYN-PARCEL-8"); rows != 2 {
			t.Fatalf("被拒的更正落了行：同键 %d 行, want 2", rows)
		}
	})

	t.Run("reusing an existing version number under the same key is refused by the primary key", func(t *testing.T) {
		duplicate := current
		duplicate.ContentDigest = "SYN-DIGEST-DUPLICATE"
		var outcome tfports.OffsitePickupSaveOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var saveErr error
			outcome, saveErr = registrations.Save(txCtx, duplicate)
			return saveErr
		}); err != nil {
			t.Fatalf("重用版本号：%v", err)
		}
		if outcome != tfports.OffsitePickupAlreadyRegistered {
			t.Fatalf("outcome = %v, want ALREADY_REGISTERED（沿用已有版本号即覆盖，库面拒）", outcome)
		}
	})
}

func pickupRowsFor(t *testing.T, pool *pgxpool.Pool, tenant, object string) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup WHERE tenant_id = $1 AND object_ref = $2`,
		tenant, object).Scan(&rows); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	return rows
}
