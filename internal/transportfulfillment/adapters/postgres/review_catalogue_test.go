package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证运输履约查阅目录的读面：上列经由写侧适配器真实落库
// 的行（测试内插行再读回，票 05），照实转写不重建、容量四量逐维求和、交付册只列
// 当前版、租户隔离进 SQL 条件、排序稳定、空册如实交回空列表。读方与写方共用同一个
// 测试库——pgtest.Pool 每次调用都是一个新库，分开建会读到两个世界。

type reviewCatalogueStores struct {
	catalogue  *adapter.ReviewCatalogue
	schedules  *adapter.TransportSchedules
	pools      *adapter.CapacityPools
	handovers  *adapter.TransportHandovers
	deliveries *adapter.EffectiveDeliveries
	transactor bentoapp.Transactor
}

func newReviewCatalogueStores(t *testing.T) reviewCatalogueStores {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	schedules, err := adapter.NewTransportSchedules(db)
	if err != nil {
		t.Fatalf("构造班次库：%v", err)
	}
	pools, err := adapter.NewCapacityPools(db)
	if err != nil {
		t.Fatalf("构造容量池库：%v", err)
	}
	handovers, err := adapter.NewTransportHandovers(db)
	if err != nil {
		t.Fatalf("构造交接登记库：%v", err)
	}
	deliveries, err := adapter.NewEffectiveDeliveries(db)
	if err != nil {
		t.Fatalf("构造交付登记库：%v", err)
	}
	return reviewCatalogueStores{
		catalogue:  catalogue,
		schedules:  schedules,
		pools:      pools,
		handovers:  handovers,
		deliveries: deliveries,
		transactor: db.Transactor(),
	}
}

func catalogueScheduleRecord(t *testing.T, tenant, id string, departsAt time.Time) ports.ScheduleRecord {
	t.Helper()
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:  deliveryValue(t, domain.NewTenantID, tenant),
		Schedule:  deliveryValue(t, domain.NewScheduleReference, id),
		Direction: "CN-US",
		DepartsAt: departsAt,
	})
	if err != nil {
		t.Fatalf("构造班次：%v", err)
	}
	return ports.ScheduleRecord{
		Key:           ports.ScheduleKey{TenantID: schedule.TenantID(), Schedule: schedule.Schedule()},
		ContentDigest: "digest-" + id,
		Schedule:      schedule,
		RecordedAt:    departsAt,
	}
}

func saveScheduleRecord(t *testing.T, stores reviewCatalogueStores, ctx context.Context, record ports.ScheduleRecord) {
	t.Helper()
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		outcome, err := stores.schedules.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.ScheduleSaved {
			t.Fatalf("save schedule outcome = %d", outcome)
		}
		return nil
	})
}

func savePoolRecord(t *testing.T, stores reviewCatalogueStores, ctx context.Context, record ports.CapacityPoolRecord) {
	t.Helper()
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		outcome, err := stores.pools.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.PoolSaved {
			t.Fatalf("save pool outcome = %d", outcome)
		}
		return nil
	})
}

func catalogueHandoverRecord(t *testing.T, tenant, object, version string) ports.TransportHandoverRecord {
	t.Helper()
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:          handoverValue(t, domain.NewTenantID, tenant),
		Object:            handoverValue(t, domain.NewCarriedObjectReference, object),
		Scope:             handoverValue(t, domain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy:        handoverValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy:        handoverValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out"),
		ReceivingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-in"),
		Rule:              handoverValue(t, domain.NewHandoverRuleReference, "rule/v1"),
		Version:           handoverValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:          handoverJudgedAtFixture,
	})
	if err != nil {
		t.Fatalf("构造交接夹具：%v", err)
	}
	return ports.TransportHandoverRecord{
		Key: ports.TransportHandoverKey{
			TenantID: handoverValue(t, domain.NewTenantID, tenant),
			Object:   handoverValue(t, domain.NewCarriedObjectReference, object),
			Scope:    handoverValue(t, domain.NewHandoverScopeReference, "scope-1"),
			Version:  handoverValue(t, domain.NewHandoverResultVersion, version),
		},
		ContentDigest: "digest-" + version,
		Handover:      handover,
		RecordedAt:    handoverJudgedAtFixture,
	}
}

func catalogueDeliveryRecord(
	t *testing.T,
	tenant, object, attempt, proof, version string,
	occurredAt time.Time,
	corrects string,
	correctedAt time.Time,
) ports.EffectiveDeliveryRecord {
	t.Helper()
	spec := domain.RehydrateEffectiveDeliverySpec{
		TenantID:   deliveryValue(t, domain.NewTenantID, tenant),
		Object:     deliveryValue(t, domain.NewCarriedObjectReference, object),
		Attempt:    deliveryValue(t, domain.NewAttemptReference, attempt),
		Place:      deliveryValue(t, domain.NewAttemptPlaceReference, "door-1"),
		Method:     deliveryValue(t, domain.NewDeliveryMethodReference, "signature"),
		Recipient:  deliveryValue(t, domain.NewReceivingPartyReference, "recipient-1"),
		Proof:      deliveryValue(t, domain.NewDeliveryProofReference, proof),
		Version:    deliveryValue(t, domain.NewDeliveryResultVersion, version),
		OccurredAt: occurredAt,
	}
	if corrects != "" {
		spec.Corrects = deliveryValue(t, domain.NewDeliveryResultVersion, corrects)
		spec.CorrectedAt = correctedAt
	}
	delivery, err := domain.RehydrateEffectiveDelivery(spec)
	if err != nil {
		t.Fatalf("重建交付夹具：%v", err)
	}
	return ports.EffectiveDeliveryRecord{
		Key: ports.EffectiveDeliveryKey{
			TenantID: deliveryValue(t, domain.NewTenantID, tenant),
			Object:   deliveryValue(t, domain.NewCarriedObjectReference, object),
			Attempt:  deliveryValue(t, domain.NewAttemptReference, attempt),
		},
		ContentDigest: "digest-" + proof,
		Delivery:      delivery,
		RecordedAt:    occurredAt,
	}
}

// TestReviewCatalogueListsTransportSchedulesLatestDepartureFirst 证班次册照行转写、
// 晚出发在前、租户隔离与页大小都由读口执行。
func TestReviewCatalogueListsTransportSchedulesLatestDepartureFirst(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")

	saveScheduleRecord(t, stores, ctx, catalogueScheduleRecord(t, "tenant-1", "sched-1", scheduleDeparts))
	saveScheduleRecord(t, stores, ctx, catalogueScheduleRecord(t, "tenant-1", "sched-2", scheduleDeparts.Add(2*time.Hour)))
	saveScheduleRecord(t, stores, ctx, catalogueScheduleRecord(t, "tenant-2", "sched-9", scheduleDeparts))

	rows, err := stores.catalogue.ListTransportSchedules(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列班次册：%v", err)
	}
	if len(rows) != 2 || rows[0].ScheduleID != "sched-2" || rows[1].ScheduleID != "sched-1" {
		t.Fatalf("册面行序变形：%+v", rows)
	}
	if rows[0].Direction != "CN-US" ||
		!rows[0].DepartsAt.Equal(scheduleDeparts.Add(2*time.Hour)) ||
		!rows[1].DepartsAt.Equal(scheduleDeparts) {
		t.Errorf("班次行转写变形：%+v", rows)
	}

	limited, err := stores.catalogue.ListTransportSchedules(ctx, tenant, 1)
	if err != nil || len(limited) != 1 || limited[0].ScheduleID != "sched-2" {
		t.Errorf("页大小未生效：rows=%+v err=%v", limited, err)
	}
}

// TestReviewCatalogueSumsCapacityPoolQuantities 证容量池册四量各自照实：有效容量照
// 行转写，已预占、已释放、实际使用按预占子表逐维求和，无预占的池三量为零，另一个
// 租户的池不可见。
func TestReviewCatalogueSumsCapacityPoolQuantities(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")

	savePoolRecord(t, stores, ctx, establishedPoolRecord(t, "tenant-1", "pool-1", 50))

	reservedPool := establishedPoolRecord(t, "tenant-1", "pool-2", 100)
	first := deliveryValue(t, domain.NewCapacityReservationReference, "r-1")
	pool, err := reservedPool.Pool.Reserve(first, 10, reservedAt.Add(24*time.Hour), reservedAt)
	if err != nil {
		t.Fatalf("预占：%v", err)
	}
	pool, err = pool.Release(first, 2, reservedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("释放：%v", err)
	}
	pool, err = pool.Consume(first, 3,
		deliveryValue(t, domain.NewLoadAssignmentReference, "assignment-1"),
		reservedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("消耗：%v", err)
	}
	pool, err = pool.Reserve(
		deliveryValue(t, domain.NewCapacityReservationReference, "r-2"),
		5, reservedAt.Add(24*time.Hour), reservedAt)
	if err != nil {
		t.Fatalf("第二笔预占：%v", err)
	}
	reservedPool.Pool = pool
	savePoolRecord(t, stores, ctx, reservedPool)

	savePoolRecord(t, stores, ctx, establishedPoolRecord(t, "tenant-2", "pool-9", 30))

	rows, err := stores.catalogue.ListCapacityPools(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列容量池册：%v", err)
	}
	if len(rows) != 2 || rows[0].PoolID != "pool-1" || rows[1].PoolID != "pool-2" {
		t.Fatalf("册面行序变形：%+v", rows)
	}
	if rows[0].Capacity != 50 || rows[0].Reserved != 0 || rows[0].Released != 0 || rows[0].Consumed != 0 {
		t.Errorf("空池行转写变形：%+v", rows[0])
	}
	if rows[1].Schedule != "schedule-1" ||
		rows[1].Unit != "kg" ||
		rows[1].Capacity != 100 ||
		rows[1].Reserved != 15 ||
		rows[1].Released != 2 ||
		rows[1].Consumed != 3 {
		t.Errorf("预占求和变形：%+v", rows[1])
	}
}

// TestReviewCatalogueListsHandoverVersionChain 证权威交接册一行一版本、版本链完整
// 可见：已交接行不带依据，拒收行带依据；两方参与方引用照行转写，不折成一个「边界」
// 串；另一个租户的判断不可见。
func TestReviewCatalogueListsHandoverVersionChain(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")

	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, handedOverRecord(t, "handover-v1"))
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, refusedRecord(t, "handover-v2"))
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, catalogueHandoverRecord(t, "tenant-2", "parcel-9", "handover-v9"))

	rows, err := stores.catalogue.ListTransportHandovers(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列交接册：%v", err)
	}
	if len(rows) != 2 || rows[0].Version != "handover-v1" || rows[1].Version != "handover-v2" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	handed := rows[0]
	if handed.Object != "parcel-1" ||
		handed.Scope != "scope-1" ||
		handed.ReleasedBy != "node-1" ||
		handed.ReceivedBy != "carrier-1" ||
		handed.Verdict != "HANDED_OVER" ||
		handed.Basis != "" ||
		handed.CorrectsVersion != "" ||
		handed.CorrectedAt != nil ||
		!handed.JudgedAt.Equal(handoverJudgedAtFixture) {
		t.Errorf("已交接行转写变形：%+v", handed)
	}
	refused := rows[1]
	if refused.Verdict != "REFUSED" || refused.Basis != "seal-broken" {
		t.Errorf("拒收行转写变形：%+v", refused)
	}
}

// TestReviewCatalogueListsOnlyCurrentEffectiveDeliveries 证有效交付册只列当前版：
// 更正翻旧插新后旧版不再上列，新版带回指前版的更正两键；新近交付在前；另一个租户
// 的交付不可见。
func TestReviewCatalogueListsOnlyCurrentEffectiveDeliveries(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	correctedAt := deliveredAtFixture.Add(time.Hour)

	mustSaveDelivery(t, stores.transactor, ctx, stores.deliveries,
		catalogueDeliveryRecord(t, "tenant-1", "parcel-1", "attempt-1", "pod-1", "delivery-v1", deliveredAtFixture, "", time.Time{}))
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		found, err := stores.deliveries.Supersede(txCtx,
			catalogueDeliveryRecord(t, "tenant-1", "parcel-1", "attempt-1", "pod-2", "delivery-v2", deliveredAtFixture, "delivery-v1", correctedAt))
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("更正没有找到可更正的登记")
		}
		return nil
	})
	mustSaveDelivery(t, stores.transactor, ctx, stores.deliveries,
		catalogueDeliveryRecord(t, "tenant-1", "parcel-2", "attempt-2", "pod-3", "delivery-v3", deliveredAtFixture.Add(-time.Hour), "", time.Time{}))
	mustSaveDelivery(t, stores.transactor, ctx, stores.deliveries,
		catalogueDeliveryRecord(t, "tenant-2", "parcel-9", "attempt-9", "pod-9", "delivery-v9", deliveredAtFixture, "", time.Time{}))

	rows, err := stores.catalogue.ListEffectiveDeliveries(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列交付册：%v", err)
	}
	if len(rows) != 2 || rows[0].Object != "parcel-1" || rows[1].Object != "parcel-2" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	corrected := rows[0]
	if corrected.Version != "delivery-v2" ||
		corrected.Attempt != "attempt-1" ||
		corrected.Place != "door-1" ||
		corrected.Method != "signature" ||
		corrected.Recipient != "recipient-1" ||
		corrected.Proof != "pod-2" ||
		corrected.CorrectsVersion != "delivery-v1" ||
		corrected.CorrectedAt == nil ||
		!corrected.CorrectedAt.Equal(correctedAt) ||
		!corrected.OccurredAt.Equal(deliveredAtFixture) {
		t.Errorf("更正版转写变形：%+v", corrected)
	}
	if first := rows[1]; first.Version != "delivery-v3" || first.CorrectsVersion != "" || first.CorrectedAt != nil {
		t.Errorf("首登版转写变形：%+v", first)
	}
}

// TestFulfillmentReviewCatalogueRejectsNonPositiveLimitAndAnswersEmptyHonestly 证读口
// 只拒绝无意义的页大小；空登记册如实交回空列表——空册是内容，不是错误
// （ADR-0077 Decision 四）。
func TestFulfillmentReviewCatalogueRejectsNonPositiveLimitAndAnswersEmptyHonestly(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")

	if _, err := stores.catalogue.ListTransportSchedules(ctx, tenant, 0); err == nil {
		t.Error("零页大小的班次上列没有被拒")
	}
	if _, err := stores.catalogue.ListCapacityPools(ctx, tenant, -1); err == nil {
		t.Error("负页大小的容量池上列没有被拒")
	}
	if _, err := stores.catalogue.ListTransportHandovers(ctx, tenant, 0); err == nil {
		t.Error("零页大小的交接上列没有被拒")
	}
	if _, err := stores.catalogue.ListEffectiveDeliveries(ctx, tenant, -1); err == nil {
		t.Error("负页大小的交付上列没有被拒")
	}

	schedules, err := stores.catalogue.ListTransportSchedules(ctx, tenant, 5)
	if err != nil || len(schedules) != 0 {
		t.Errorf("空班次册：rows=%+v err=%v", schedules, err)
	}
	pools, err := stores.catalogue.ListCapacityPools(ctx, tenant, 5)
	if err != nil || len(pools) != 0 {
		t.Errorf("空容量池册：rows=%+v err=%v", pools, err)
	}
	handovers, err := stores.catalogue.ListTransportHandovers(ctx, tenant, 5)
	if err != nil || len(handovers) != 0 {
		t.Errorf("空交接册：rows=%+v err=%v", handovers, err)
	}
	deliveries, err := stores.catalogue.ListEffectiveDeliveries(ctx, tenant, 5)
	if err != nil || len(deliveries) != 0 {
		t.Errorf("空交付册：rows=%+v err=%v", deliveries, err)
	}
}
