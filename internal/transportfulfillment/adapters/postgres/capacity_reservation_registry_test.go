package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证容量预占与释放两种决定的落册（迁移 0030，ADR-0203 决定三至五）：预占固定需求来源与判断时点、
// 释放有身份且逐次留明细、重放不重扣也不重还、查阅册三量照实。夹具全为合成值（S 级）。

func (stores scheduleRegistryStores) reserve(t *testing.T, command application.ReserveCapacityCommand) application.OpportunityResult {
	t.Helper()
	var result application.OpportunityResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := stores.poolHandler().Reserve(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		t.Fatalf("预占：%v", err)
	}
	return result
}

func (stores scheduleRegistryStores) release(t *testing.T, command application.ReleaseCapacityCommand) application.OpportunityResult {
	t.Helper()
	var result application.OpportunityResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := stores.poolHandler().Release(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		t.Fatalf("释放：%v", err)
	}
	return result
}

// registeredWeightPool 在一个登了的物化班次上建一个 5000 KG 的重量池，交回池身份。
func registeredWeightPool(t *testing.T, stores scheduleRegistryStores, tenant domain.TenantID) string {
	t.Helper()
	wantScheduleOutcome(t, stores.registerTemplate(t, syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")),
		application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-21",
	}), application.TransportScheduleRegistered)
	wantPoolOutcome(t, stores.establishPool(t, application.EstablishPoolCommand{
		TenantID: tenant, Schedule: "SYN-TPL-1/2026-10-21", Dimension: "WEIGHT", Unit: "KG", Capacity: 5000,
	}), application.PoolEstablished)
	return "SYN-TPL-1/2026-10-21/WEIGHT"
}

var (
	registryReservedAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	registryValidUntil = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
)

func registryReserveCommand(tenant domain.TenantID, pool, reservation string, quantity int64) application.ReserveCapacityCommand {
	return application.ReserveCapacityCommand{
		TenantID: tenant, Pool: pool, Reservation: reservation, Quantity: quantity, Unit: "KG",
		SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-01/v1",
		ValidUntil: registryValidUntil, At: registryReservedAt,
	}
}

func registryReleaseCommand(tenant domain.TenantID, pool, reservation, release string, quantity int64, at time.Time) application.ReleaseCapacityCommand {
	return application.ReleaseCapacityCommand{
		TenantID: tenant, Pool: pool, Reservation: reservation, Release: release, Quantity: quantity, Unit: "KG",
		SourceKind: "PREPARATION_DEMAND_SNAPSHOT", SourceReference: "SYN-PREP-01/demand-v2", At: at,
	}
}

// Covers: `AT-TF-140`、`AT-TF-141` 与 ADR-0203 决定三、五的落册——预占在册上读回需求来源与判断时点；释放逐次留明细（身份、
// 数量、来源、时点），已释放列等于明细之和；预占与释放原样重交都不多落、不重扣也不重还；查阅册的三个和照实；释放之后
// 再超量预占答`容量超出`并带此刻余量。
func TestReservationsAndReleasesRoundTripOnTheRegistry(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	pool := registeredWeightPool(t, stores, tenant)

	wantPoolOutcome(t, stores.reserve(t, registryReserveCommand(tenant, pool, "SYN-RES-1", 1200)), application.CapacityReserved)
	wantPoolOutcome(t, stores.reserve(t, registryReserveCommand(tenant, pool, "SYN-RES-1", 1200)), application.ReservationExistingResult)
	releasedAt := registryReservedAt.Add(24 * time.Hour)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(tenant, pool, "SYN-RES-1", "SYN-REL-1", 200, releasedAt)), application.CapacityReleased)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(tenant, pool, "SYN-RES-1", "SYN-REL-1", 200, releasedAt)), application.ReleaseExistingResult)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(tenant, pool, "SYN-RES-1", "SYN-REL-1", 300, releasedAt)), application.ReleaseConflict)

	found, exists, err := stores.pools.FindByKey(t.Context(), ports.CapacityPoolKey{
		TenantID: tenant, Pool: deliveryValue(t, domain.NewCapacityPoolReference, pool),
	})
	if err != nil || !exists {
		t.Fatalf("读回池：exists=%v err=%v", exists, err)
	}
	snapshots := found.Pool.ReservationSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("预占 %d 份，want 1", len(snapshots))
	}
	reservation := snapshots[0]
	if reservation.Quantity != 1200 || reservation.Released != 200 || reservation.Consumed != 0 ||
		reservation.Source.Kind() != domain.CapacityDemandOperatorDeclaration || reservation.Source.Reference().String() != "SYN-DEMAND-DECL-01/v1" ||
		!reservation.ReservedAt.Equal(registryReservedAt) || !reservation.ValidUntil.Equal(registryValidUntil) {
		t.Fatalf("预占读回 = %+v", reservation)
	}
	if len(reservation.Releases) != 1 {
		t.Fatalf("释放明细 %d 条，want 1（重放不多落）", len(reservation.Releases))
	}
	release := reservation.Releases[0]
	if release.Reference.String() != "SYN-REL-1" || release.Quantity != 200 ||
		release.Source.Kind() != domain.CapacityDemandPreparationSnapshot || !release.ReleasedAt.Equal(releasedAt) {
		t.Fatalf("释放读回 = %+v", release)
	}

	rows, err := stores.catalogue.ListCapacityPools(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].Reserved != 1200 || rows[0].Released != 200 || rows[0].Consumed != 0 {
		t.Fatalf("查阅册 = %+v %v", rows, err)
	}

	// 释放从它的时点起归还：释放之后起算的预占看得到 4000 余量，声明在释放之前的那一刻只有 3800。
	afterRelease := registryReserveCommand(tenant, pool, "SYN-RES-2", 4001)
	afterRelease.At = releasedAt
	exceeded := stores.reserve(t, afterRelease)
	wantPoolOutcome(t, exceeded, application.CapacityExceededOutcome)
	if exceeded.AvailableQuantity() != 4000 {
		t.Fatalf("释放之后的余量 = %d, want 4000", exceeded.AvailableQuantity())
	}
	beforeRelease := stores.reserve(t, registryReserveCommand(tenant, pool, "SYN-RES-2", 3801))
	wantPoolOutcome(t, beforeRelease, application.CapacityExceededOutcome)
	if beforeRelease.AvailableQuantity() != 3800 {
		t.Fatalf("释放之前起算的余量 = %d, want 3800", beforeRelease.AvailableQuantity())
	}
}

// Covers: 迁移 0030 的拒绝门——预占表有行时不施加，不替任何人补需求来源与判断时点。
func TestMigration0030RefusesReservationsItCannotGiveASource(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移资产：%v", err)
	}
	var sourcing string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0030_") {
			sourcing = asset.SQL
		}
	}
	if sourcing == "" {
		t.Fatal("找不到迁移 0030")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('SYN-TENANT-01', 'S-1/WEIGHT', 'S-1', 'WEIGHT', 'KG', 10, 'd', now())`); err != nil {
		t.Fatalf("插探针池：%v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at)
		 VALUES ('SYN-TENANT-01', 'S-1/WEIGHT', 'R-1', 5, now() + interval '1 day', 0, 0, 'OPERATOR_DECLARATION', 'D', now())`); err != nil {
		t.Fatalf("插探针预占：%v", err)
	}
	if _, err := tx.Exec(ctx, sourcing); err == nil || !strings.Contains(err.Error(), "已有行") {
		t.Fatalf("预占表有行时 0030 应拒绝施加：%v", err)
	}
}

// Covers: 迁移 0030 的库面防线——来源种类封闭、来源引用非空、有效期晚于判断时点；释放数量为正、来源种类封闭、释放引用在
// 池内唯一（同一引用挪到另一份预占上插不进）、必须挂在一份在册的预占上；预占随池换存删掉时释放明细跟着删。
func TestReservationRegistryChecksRefuseMalformedRows(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	for _, setup := range []string{
		`INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('T', 'S/WEIGHT', 'S', 'WEIGHT', 'KG', 100, 'd', now())`,
		`INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at)
		 VALUES ('T', 'S/WEIGHT', 'R-1', 50, '2026-10-21T00:00:00Z', 10, 0, 'OPERATOR_DECLARATION', 'D', '2026-10-19T00:00:00Z'),
		        ('T', 'S/WEIGHT', 'R-2', 20, '2026-10-21T00:00:00Z', 0, 0, 'OPERATOR_DECLARATION', 'D', '2026-10-19T00:00:00Z')`,
		`INSERT INTO transport_fulfillment.capacity_release
			(tenant_id, pool_id, release_id, reservation_id, quantity, source_kind, source_ref, released_at)
		 VALUES ('T', 'S/WEIGHT', 'REL-1', 'R-1', 10, 'OPERATOR_DECLARATION', 'D', '2026-10-20T00:00:00Z')`,
	} {
		if _, err := pool.Exec(ctx, setup); err != nil {
			t.Fatalf("探针父行插不进去：%v", err)
		}
	}
	reservation := func(sourceKind, sourceRef, validUntil string) string {
		return `INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at)
		 VALUES ('T', 'S/WEIGHT', 'R-9', 5, '` + validUntil + `', 0, 0, '` + sourceKind + `', '` + sourceRef + `', '2026-10-19T00:00:00Z')`
	}
	release := func(id, reservationID, quantity, sourceKind string) string {
		return `INSERT INTO transport_fulfillment.capacity_release
			(tenant_id, pool_id, release_id, reservation_id, quantity, source_kind, source_ref, released_at)
		 VALUES ('T', 'S/WEIGHT', '` + id + `', '` + reservationID + `', ` + quantity + `, '` + sourceKind + `', 'D', '2026-10-20T00:00:00Z')`
	}
	probes := map[string]string{
		"预占来源种类不在封闭词里":  reservation("MEMBER_COUNT", "D", "2026-10-21T00:00:00Z"),
		"预占来源引用为空":      reservation("OPERATOR_DECLARATION", " ", "2026-10-21T00:00:00Z"),
		"预占有效期不晚于判断时点":  reservation("OPERATOR_DECLARATION", "D", "2026-10-19T00:00:00Z"),
		"释放数量为零":        release("REL-2", "R-1", "0", "OPERATOR_DECLARATION"),
		"释放来源种类不在封闭词里":  release("REL-2", "R-1", "1", "operator_declaration"),
		"释放引用挪到另一份预占上":  release("REL-1", "R-2", "1", "OPERATOR_DECLARATION"),
		"释放挂在不在册的预占上":   release("REL-2", "R-404", "1", "OPERATOR_DECLARATION"),
		"预占缺来源（迁移前的形状）": `INSERT INTO transport_fulfillment.capacity_reservation (tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed) VALUES ('T', 'S/WEIGHT', 'R-8', 5, '2026-10-21T00:00:00Z', 0, 0)`,
	}
	for name, probe := range probes {
		if _, err := pool.Exec(ctx, probe); err == nil {
			t.Errorf("%s：坏行溜进了册子", name)
		}
	}

	if _, err := pool.Exec(ctx, `DELETE FROM transport_fulfillment.capacity_reservation WHERE tenant_id = 'T' AND pool_id = 'S/WEIGHT'`); err != nil {
		t.Fatalf("按池删预占：%v", err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.capacity_release`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("预占删了，释放明细还剩 %d 条（%v）", left, err)
	}
}
