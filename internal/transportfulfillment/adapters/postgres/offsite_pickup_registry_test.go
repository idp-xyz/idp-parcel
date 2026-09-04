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
// 控制依据必备入库内 CHECK、作用域隔离、键与本体不一致拒写、无事务拒、回滚无痕；以及更正
// 走新版本那一半（票 tf-segment-lifecycle-closure/08）：新行回指前版、原行不动、按键读回当前版、
// 一版最多被更正一次、链一致性入库内 CHECK。

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

// TestAPickupCorrectionLandsAsANewVersionAndTheOriginalStays 证票 tf-segment-lifecycle-closure/08 裁决 A
// 在库面的形状：更正是同键下的新行新版本，回指前版；原行一字不动；FindByKey 答的是当前版——链尾那一版，
// 「当前」按回指派生，表上没有 current 列（同 0014 effective_time_rule 的取法）。
func TestAPickupCorrectionLandsAsANewVersionAndTheOriginalStays(t *testing.T) {
	repository, transactor, pool := newOffsitePickups(t)
	ctx := t.Context()

	original := pickupRecord(t, "control-1", "PRV-000000000001")
	mustSavePickup(t, transactor, ctx, repository, original)
	corrected := correctedPickupRecord(t, original, "control-2", "PRV-000000000002")
	mustSavePickup(t, transactor, ctx, repository, corrected)

	current, exists, err := repository.FindByKey(ctx, pickupKeyFixture(t, "tenant-1"))
	if err != nil {
		t.Fatalf("取回当前版：%v", err)
	}
	if !exists {
		t.Fatal("更正后按键读不回任何版本")
	}
	if current.Pickup.Version().String() != "PRV-000000000002" || current.Pickup.Control().String() != "control-2" {
		t.Fatalf("FindByKey 答的不是链尾：%+v", current.Pickup)
	}
	if predecessor, present := current.Pickup.Corrects(); !present || predecessor.String() != "PRV-000000000001" {
		t.Fatalf("链读回来断了：corrects = %q present=%v", predecessor, present)
	}
	if at, present := current.Pickup.CorrectedAt(); !present || !at.Equal(correctedAtFixture) {
		t.Fatalf("更正时刻读回来变形：%v present=%v", at, present)
	}
	if current.ContentDigest != corrected.ContentDigest {
		t.Fatalf("content digest = %q, want %q", current.ContentDigest, corrected.ContentDigest)
	}

	var rows int
	var originalControl string
	var originalCorrects *string
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = 'tenant-1' AND object_ref = 'parcel-1' AND attempt_ref = 'attempt-1'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("同键行数 = %d, want 2——更正必须是新行，不是改写", rows)
	}
	if err := pool.QueryRow(ctx,
		`SELECT control_ref, corrects_version FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = 'tenant-1' AND object_ref = 'parcel-1' AND attempt_ref = 'attempt-1'
		    AND pickup_version = 'PRV-000000000001'`).Scan(&originalControl, &originalCorrects); err != nil {
		t.Fatalf("读原行：%v", err)
	}
	if originalControl != "control-1" || originalCorrects != nil {
		t.Fatalf("原行被更正动了：control=%q corrects=%v", originalControl, originalCorrects)
	}
}

// TestFindByKeyAndVersionReadsBackAnyGenerationOfAPickup 证按（键+版本）取回指名的那一代，不问它是不是
// 当前版（票 label-channel/24 的 PS 消费方按信封所指版本读回，ADR-0117 决定四）：更正后旧代仍读得回且
// 不带回指、新代带回指与更正时刻——PS 靠这两格判「回指链能不能接到已采用版本」，不必再读一次；
// 不存在的版本与他租户答无不报错，与 FindByKey 同纪律。
func TestFindByKeyAndVersionReadsBackAnyGenerationOfAPickup(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	original := pickupRecord(t, "control-1", "PRV-000000000001")
	mustSavePickup(t, transactor, ctx, repository, original)
	mustSavePickup(t, transactor, ctx, repository, correctedPickupRecord(t, original, "control-2", "PRV-000000000002"))

	key := pickupKeyFixture(t, "tenant-1")
	old, exists, err := repository.FindByKeyAndVersion(ctx, key, pickupValue(t, domain.NewPickupResultVersion, "PRV-000000000001"))
	if err != nil || !exists {
		t.Fatalf("旧代读回：exists=%v err=%v", exists, err)
	}
	if old.Pickup.Version().String() != "PRV-000000000001" || old.Pickup.Control().String() != "control-1" || old.Key != key {
		t.Fatalf("按版本读回的不是那一代：%+v", old.Pickup)
	}
	if _, corrected := old.Pickup.Corrects(); corrected {
		t.Fatal("首登那一代读回时长出了回指")
	}

	current, exists, err := repository.FindByKeyAndVersion(ctx, key, pickupValue(t, domain.NewPickupResultVersion, "PRV-000000000002"))
	if err != nil || !exists {
		t.Fatalf("新代读回：exists=%v err=%v", exists, err)
	}
	if predecessor, corrected := current.Pickup.Corrects(); !corrected || predecessor.String() != "PRV-000000000001" {
		t.Fatalf("更正那一代读回时丢了回指：%v %v", predecessor, corrected)
	}
	if at, present := current.Pickup.CorrectedAt(); !present || !at.Equal(correctedAtFixture) {
		t.Fatalf("更正时刻读回变形：%v present=%v", at, present)
	}

	if _, exists, err := repository.FindByKeyAndVersion(ctx, key, pickupValue(t, domain.NewPickupResultVersion, "PRV-000000000009")); err != nil || exists {
		t.Fatalf("不存在的版本：exists=%v err=%v，想要 false 且不报错", exists, err)
	}
	if _, exists, err := repository.FindByKeyAndVersion(ctx, pickupKeyFixture(t, "tenant-2"), pickupValue(t, domain.NewPickupResultVersion, "PRV-000000000001")); err != nil || exists {
		t.Fatalf("他租户按同键同版本读到了：exists=%v err=%v", exists, err)
	}
}

// TestASecondCorrectionOfTheSameVersionKeepsTheFirst 证一版最多被更正一次（库内部分唯一索引）：并发
// 第二次更正同一前版撞索引译`已登记`而不是 error，同事务立刻读回的当前版仍是先到的那一次更正——链因此
// 保持线性，FindByKey 才答得出唯一的当前版。
func TestASecondCorrectionOfTheSameVersionKeepsTheFirst(t *testing.T) {
	repository, transactor, _ := newOffsitePickups(t)
	ctx := t.Context()

	original := pickupRecord(t, "control-1", "PRV-000000000001")
	mustSavePickup(t, transactor, ctx, repository, original)
	mustSavePickup(t, transactor, ctx, repository, correctedPickupRecord(t, original, "control-2", "PRV-000000000002"))

	late := correctedPickupRecord(t, original, "control-3", "PRV-000000000003")
	var outcome ports.OffsitePickupSaveOutcome
	var winner ports.OffsitePickupRecord
	var exists bool
	mustWithinPickupTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = repository.Save(txCtx, late); err != nil {
			return err
		}
		winner, exists, err = repository.FindByKey(txCtx, pickupKeyFixture(t, "tenant-1"))
		return err
	})
	if outcome != ports.OffsitePickupAlreadyRegistered {
		t.Fatalf("outcome = %d, want ALREADY_REGISTERED——同一前版被更正了两次", outcome)
	}
	if !exists || winner.Pickup.Version().String() != "PRV-000000000002" {
		t.Fatalf("撞索引后同事务读回的当前版 = %+v exists=%v, want v2", winner.Pickup, exists)
	}
}

// TestTheDatabaseRefusesAHalfChain 证链一致性入库内 CHECK：前版引用与更正时间同缺席或同在场，且不自指
// ——与领域 Correct / RehydrateOffsitePickup 立的门一一对应，绕过领域直插也进不去。
func TestTheDatabaseRefusesAHalfChain(t *testing.T) {
	_, _, pool := newOffsitePickups(t)
	ctx := t.Context()

	cases := map[string]string{
		"predecessor without corrected at": `('PRV-000000000001', NULL)`,
		"corrected at without predecessor": `(NULL, now())`,
		"predecessor pointing at itself":   `('PRV-000000000002', now())`,
	}
	for name, chain := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				`INSERT INTO transport_fulfillment.offsite_pickup
					(tenant_id, object_ref, attempt_ref, task_ref, place_ref, control_ref,
					 executed_by, pickup_version, occurred_at, content_digest, recorded_at,
					 corrects_version, corrected_at)
				 SELECT 'tenant-x', 'parcel-x', 'attempt-x', 'task-1', 'dock-1', 'control-1',
				        'courier-1', 'PRV-000000000002', now(), 'digest-x', now(), chain.*
				   FROM (VALUES `+chain+`) AS chain(corrects_version, corrected_at)`)
			if err == nil {
				t.Fatal("一行半截版本链进了库")
			}
		})
	}
}

// ---- 夹具 ----

var correctedAtFixture = pickedUpAtFixture.Add(36 * time.Hour)

// correctedPickupRecord 经领域 Correct 形成回指 original 的新版本记录；内容比对锚随新内容换。
func correctedPickupRecord(t *testing.T, original ports.OffsitePickupRecord, control, version string) ports.OffsitePickupRecord {
	t.Helper()
	corrected, err := original.Pickup.Correct(domain.PickupCorrection{
		Place:       pickupValue(t, domain.NewPickupPlaceReference, "dock-2"),
		Control:     pickupValue(t, domain.NewTransportControlReference, control),
		ExecutedBy:  pickupValue(t, domain.NewExecutingPartyReference, "courier-2"),
		OccurredAt:  pickedUpAtFixture.Add(-time.Hour),
		Version:     pickupValue(t, domain.NewPickupResultVersion, version),
		CorrectedAt: correctedAtFixture,
	})
	if err != nil {
		t.Fatalf("形成更正版本：%v", err)
	}
	return ports.OffsitePickupRecord{
		Key:           original.Key,
		ContentDigest: "digest-" + control,
		Pickup:        corrected,
		RecordedAt:    correctedAtFixture,
	}
}

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
