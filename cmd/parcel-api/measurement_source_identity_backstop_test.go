package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodeopsports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证编排的先锁后读没盖住来源身份时，库约束这道后背被打到，应用上抛错误：既不答成功也不答冲突，不留事实行、不发变化事件。
// 「没盖住」用一个对册撒谎的替身造出来：它不取锁，并且对被指定的那个来源身份谎称另一类没有行。

// blindSourceIdentityStore 对 hidden 这个来源身份装作另一类不在册，也不取来源身份锁；其余一切照真册办。
type blindSourceIdentityStore struct {
	nodeopsports.ActualMeasurementStore
	hidden string
}

func (blindSourceIdentityStore) SerializeSourceIdentity(context.Context, nodeopsports.ActualMeasurementKey) error {
	return nil
}

func (store blindSourceIdentityStore) FindMeasurement(
	ctx context.Context,
	key nodeopsports.ActualMeasurementKey,
) (nodeopsports.ActualMeasurementRecord, bool, error) {
	if key.SourceID == store.hidden {
		return nodeopsports.ActualMeasurementRecord{}, false, nil
	}
	return store.ActualMeasurementStore.FindMeasurement(ctx, key)
}

func (store blindSourceIdentityStore) FindInvalidation(
	ctx context.Context,
	key nodeopsports.ActualMeasurementKey,
) (nodeopsports.MeasurementInvalidationRecord, bool, error) {
	if key.SourceID == store.hidden {
		return nodeopsports.MeasurementInvalidationRecord{}, false, nil
	}
	return store.ActualMeasurementStore.FindInvalidation(ctx, key)
}

func (fixture *sharedIdentityFixture) blindHandler(t *testing.T, hidden string) transactionalMeasurement {
	t.Helper()
	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := nopostgres.NewActualMeasurements(db)
	if err != nil {
		t.Fatalf("构造测量登记册：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox：%v", err)
	}
	notice, err := nopostgres.NewOutboxMeasurementChangeNotice(db, outboxStore, systemClock{})
	if err != nil {
		t.Fatalf("构造变化通知：%v", err)
	}
	inner, err := nodeopsapp.NewRecordActualMeasurementHandler(
		blindSourceIdentityStore{ActualMeasurementStore: store, hidden: hidden}, systemClock{}, notice)
	if err != nil {
		t.Fatalf("构造登记编排：%v", err)
	}
	return transactionalMeasurement{transactor: db.Transactor(), inner: inner}
}

func wantForeignKeyViolation(t *testing.T, what string, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != constraint {
		t.Fatalf("%s：want 外键违例 %s 上抛，实得 %v", what, constraint, err)
	}
}

// Covers: 编排没拦住（测量借用作废的来源身份）时库约束兜底：登记交回外键违例的错误，不是 MEASUREMENT_RECORDED，也不是
// SOURCE_CONFLICT；不落测量行、不发变化事件。
func TestAMeasurementTakingAnInvalidationsSourceIdentityPastTheOrchestrationIsAnError(t *testing.T) {
	fixture := newSharedIdentityFixture(t)
	const (
		unit   = "SYN-HU-10-0001"
		target = "SYN-DEVICE-10/TARGET"
		void   = "SYN-DEVICE-10/VOID"
	)
	fixture.mustRecordMeasurement(t, target, unit)
	fixture.mustRecordInvalidation(t, void, target)
	before := fixture.changeEvents(t)

	result, err := fixture.blindHandler(t, void).Handle(t.Context(), nodeopsapp.RecordActualMeasurementCommand{
		TenantID: sharedIdentityTenant, SourceID: void, PerformedBy: "SYN-SCALE-01", Evidence: "WEIGH/1",
		OccurredAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
		ObjectKind: "HANDLING_UNIT", ObjectID: unit + "-OTHER", Node: "SYN-NODE-SHA-HUB",
		Location: "SYN-BENCH-01", Basis: "SYN-PARCEL-09",
		WeightValue: "2.10", WeightUnit: "KG", Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	wantForeignKeyViolation(t, "测量借用作废的来源身份", err, "actual_measurement_source_identity_fk")
	if result.Outcome() == nodeopsapp.ActualMeasurementRecorded || result.Outcome() == nodeopsapp.ActualMeasurementExisting {
		t.Fatalf("后背被打到却答成 %s", result.Outcome())
	}
	if fixture.measurementRow(t, void) {
		t.Fatal("借用来源身份的测量落了册")
	}
	if after := fixture.changeEvents(t); after != before {
		t.Fatalf("变化事件 %d 封，want %d（后背拦下的登记不发）", after, before)
	}
}

// Covers: 同上，反方向：作废借用测量的来源身份，库约束兜底，作废交回外键违例的错误；不落作废行，被指的那条测量仍有效，不发变化事件。
func TestAnInvalidationTakingAMeasurementsSourceIdentityPastTheOrchestrationIsAnError(t *testing.T) {
	fixture := newSharedIdentityFixture(t)
	const (
		unit   = "SYN-HU-10-0002"
		taken  = "SYN-DEVICE-10/TAKEN"
		target = "SYN-DEVICE-10/TARGET-2"
	)
	fixture.mustRecordMeasurement(t, taken, unit)
	fixture.mustRecordMeasurement(t, target, unit)
	before := fixture.changeEvents(t)

	result, err := fixture.blindHandler(t, taken).Invalidate(t.Context(), nodeopsapp.InvalidateActualMeasurementCommand{
		TenantID: sharedIdentityTenant, SourceID: taken, TargetSourceID: target, Reason: "称错件",
		PerformedBy: "SYN-SCALE-01", Evidence: "WEIGH/2",
		OccurredAt: time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC),
	})
	wantForeignKeyViolation(t, "作废借用测量的来源身份", err, "measurement_invalidation_source_identity_fk")
	if result.Outcome() == nodeopsapp.MeasurementInvalidationRecorded || result.Outcome() == nodeopsapp.MeasurementInvalidationExisting {
		t.Fatalf("后背被打到却答成 %s", result.Outcome())
	}
	if fixture.invalidationRow(t, taken) {
		t.Fatal("借用来源身份的作废落了册")
	}
	if ids := fixture.stillValid(t, unit); !ids[taken] || !ids[target] {
		t.Fatalf("仍有效 = %v，want 两条都在", ids)
	}
	if after := fixture.changeEvents(t); after != before {
		t.Fatalf("变化事件 %d 封，want %d（后背拦下的作废不发）", after, before)
	}
}
