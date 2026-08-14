package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证对象级揽收登记：往返、撞键译`已登记`且事务保持可用、
// 控制依据必备入库内 CHECK、作用域隔离、键与本体不一致拒写、无事务拒、回滚无痕。

var pickedUpAtFixture = time.Date(2026, 9, 10, 8, 30, 0, 0, time.UTC)

func TestAnOffsitePickupRoundTrips(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	record := pickupRecord(t, "control-1", "PRV-000000000001")
	mustSavePickup(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, pickupKeyFixture(t, "tenant-1"))
	if err != nil {
		t.Fatalf("取回揽收：%v", err)
	}
	if !exists {
		t.Fatal("已登记的揽收读不回来")
	}
	if found.Pickup.Control().String() != "control-1" ||
		found.Pickup.Version().String() != "PRV-000000000001" ||
		found.Pickup.Task().String() != "task-1" ||
		found.Pickup.Place().String() != "dock-1" ||
		found.Pickup.ExecutedBy().String() != "courier-1" ||
		!found.Pickup.OccurredAt().Equal(pickedUpAtFixture) ||
		found.ContentDigest != record.ContentDigest {
		t.Fatalf("揽收往返变形：%+v", found.Pickup)
	}
}

// TestASecondPickupRegistrationKeepsTheFirst 证写入代数：撞键译`已登记`而不是 error，
// 且撞键后同一事务立刻读得回先到者——事务保持可用是这条代数的另一半（ADR-0031）。
func TestASecondPickupRegistrationKeepsTheFirst(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	mustSavePickup(t, transactor, ctx, repository, pickupRecord(t, "control-1", "PRV-000000000001"))

	late := pickupRecord(t, "control-late", "PRV-000000000009")
	var outcome ports.OffsitePickupSaveOutcome
	var winner ports.OffsitePickupRecord
	var exists bool
	mustWithinPickupTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = repository.Save(txCtx, late); err != nil {
			return err
		}
		// 撞键后在同一事务里读回，正是这条代数的另一半：事务必须仍然可用。
		winner, exists, err = repository.FindByKey(txCtx, pickupKeyFixture(t, "tenant-1"))
		return err
	})

	if outcome != ports.OffsitePickupAlreadyRegistered {
		t.Fatalf("outcome = %d, want ALREADY_REGISTERED", outcome)
	}
	if !exists {
		t.Fatal("撞键后同事务读不回赢家")
	}
	if winner.Pickup.Control().String() != "control-1" {
		t.Fatal("后到者覆盖了先到者的登记")
	}
}

// TestControlEvidenceIsRequiredInTheDatabase 证控制依据必备入库内 CHECK：绕过领域
// 直插一行空控制的「揽收」被数据库拒——那正是有效收寄与一次失败到场的分界。
func TestControlEvidenceIsRequiredInTheDatabase(t *testing.T) {
	_, _, pool := newOffsitePickups(t)
	ctx := t.Context()

	_, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.offsite_pickup
			(tenant_id, object_ref, attempt_ref, task_ref, place_ref, control_ref,
			 executed_by, pickup_version, occurred_at, content_digest, recorded_at)
		 VALUES ('tenant-1', 'parcel-1', 'attempt-1', 'task-1', 'dock-1', '   ',
		         'courier-1', 'PRV-000000000001', now(), 'digest-1', now())`)
	if err == nil {
		t.Fatal("一行没有控制依据的「揽收」进了库")
	}
}

// TestPickupScopesAreInvisibleToEachOther 证否定结果不泄露其他租户是否存在该揽收。
func TestPickupScopesAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	mustSavePickup(t, transactor, ctx, repository, pickupRecord(t, "control-1", "PRV-000000000001"))

	_, exists, err := repository.FindByKey(ctx, pickupKeyFixture(t, "tenant-b"))
	if err != nil {
		t.Fatalf("他租户查询出错：%v", err)
	}
	if exists {
		t.Error("他租户读到了本租户的揽收")
	}
}

// TestAPickupRecordDisagreeingWithItsKeyIsRefused 证键与本体不一致拒写：键由平铺列
// 独家拥有，写下去就再也查不出原本该是哪一个。
func TestAPickupRecordDisagreeingWithItsKeyIsRefused(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	record := pickupRecord(t, "control-1", "PRV-000000000001")
	record.Key.Object = pickupValue(t, domain.NewCarriedObjectReference, "parcel-other")

	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, saveErr := repository.Save(txCtx, record)
		return saveErr
	})
	if err == nil {
		t.Fatal("记录键与揽收本体各说各话却写了进去")
	}
}

func TestPickupRegistrationRefusesToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newOffsitePickups(t)

	_, err := repository.Save(t.Context(), pickupRecord(t, "control-1", "PRV-000000000001"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestPickupRegistrationRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, pickupRecord(t, "control-1", "PRV-000000000001")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, pickupKeyFixture(t, "tenant-1"))
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后登记仍在")
	}
}

// ---- 夹具 ----

func newOffsitePickups(t *testing.T) (*adapter.OffsitePickupRegistrations, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewOffsitePickupRegistrations(db)
	if err != nil {
		t.Fatalf("构造揽收登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinPickupTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSavePickup(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.OffsitePickupRegistrations,
	record ports.OffsitePickupRecord,
) {
	t.Helper()
	var outcome ports.OffsitePickupSaveOutcome
	mustWithinPickupTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	if outcome != ports.OffsitePickupSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

func pickupValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func pickupKeyFixture(t *testing.T, tenant string) ports.OffsitePickupKey {
	t.Helper()
	return ports.OffsitePickupKey{
		TenantID: pickupValue(t, domain.NewTenantID, tenant),
		Object:   pickupValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:  pickupValue(t, domain.NewAttemptReference, "attempt-1"),
	}
}

func pickupRecord(t *testing.T, control, version string) ports.OffsitePickupRecord {
	t.Helper()
	pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
		TenantID:   pickupValue(t, domain.NewTenantID, "tenant-1"),
		Object:     pickupValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Task:       pickupValue(t, domain.NewPickupTaskReference, "task-1"),
		Attempt:    pickupValue(t, domain.NewAttemptReference, "attempt-1"),
		Place:      pickupValue(t, domain.NewPickupPlaceReference, "dock-1"),
		Control:    pickupValue(t, domain.NewTransportControlReference, control),
		ExecutedBy: pickupValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Version:    pickupValue(t, domain.NewPickupResultVersion, version),
		OccurredAt: pickedUpAtFixture,
	})
	if err != nil {
		t.Fatalf("构造揽收夹具：%v", err)
	}
	return ports.OffsitePickupRecord{
		Key:           pickupKeyFixture(t, "tenant-1"),
		ContentDigest: "digest-" + control,
		Pickup:        pickup,
		RecordedAt:    pickedUpAtFixture,
	}
}
