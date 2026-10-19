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

// 本文件对真实 PostgreSQL 证容量池登记（迁移 0027，ADR-0201 决定一、三与补记）：建池核班次在册、池身份由（班次、
// 维度）派生、周期安排的逐维容量声明随物化建池且幂等、多维分存。夹具全为合成值（S 级）。

func (stores scheduleRegistryStores) poolHandler() *application.PrepareTransportOpportunityHandler {
	return application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Pools:     stores.pools,
		Schedules: stores.schedules,
		Clock:     fixedScheduleClock{at: time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)},
	})
}

func (stores scheduleRegistryStores) establishPool(t *testing.T, command application.EstablishPoolCommand) application.OpportunityResult {
	t.Helper()
	var result application.OpportunityResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := stores.poolHandler().EstablishPool(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		t.Fatalf("建池：%v", err)
	}
	return result
}

func wantPoolOutcome(t *testing.T, result application.OpportunityResult, want application.OpportunityOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("outcome = %s（%s），要 %s", result.Outcome(), result.UndecidedReason(), want)
	}
}

func countPools(t *testing.T, stores scheduleRegistryStores, tenant domain.TenantID) int {
	t.Helper()
	rows, err := stores.catalogue.ListCapacityPools(t.Context(), tenant, 50)
	if err != nil {
		t.Fatalf("上列容量池册：%v", err)
	}
	return len(rows)
}

// Covers: 方案 D03-6 与 `AT-TF-105`——池只能挂在登记册上有的班次上：没登过的班次、只经准备评估物化而没有显式登记
// 的那一班、他租户名下的班次都答`班次不在册`且零写入；班次登了之后同一命令答`已建立`，重放答`已存在`，异内容答
// `池冲突`且库里那一池不改写。同一班次另一维是另一个池（`AT-TF-030`），两池在册上各自一行、各带维度。
func TestEstablishingAPoolOnTheRegistryChecksTheSchedule(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	other := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02")
	wantScheduleOutcome(t, stores.registerTemplate(t, syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")),
		application.ScheduleTemplateRegistered)

	weight := application.EstablishPoolCommand{
		TenantID: tenant, Schedule: "SYN-TPL-1/2026-10-21", Dimension: "WEIGHT", Unit: "KG", Capacity: 5000,
	}
	wantPoolOutcome(t, stores.establishPool(t, weight), application.PoolScheduleNotFound)
	adHoc := weight
	adHoc.Schedule = "SYN-ADHOC-1"
	wantPoolOutcome(t, stores.establishPool(t, adHoc), application.PoolScheduleNotFound)

	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: other, Schedule: "SYN-ADHOC-1", Connection: "SYN-CONN-1", FromNode: "SYN-NODE-A", ToNode: "SYN-NODE-B",
		DepartsAt: time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC), ArrivesAt: time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC),
		ServiceScope: "SYN-SCOPE-01",
	}), application.TransportScheduleRegistered)
	wantPoolOutcome(t, stores.establishPool(t, adHoc), application.PoolScheduleNotFound)
	if countPools(t, stores, tenant) != 0 {
		t.Fatal("班次不在册却落了池")
	}

	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-21",
	}), application.TransportScheduleRegistered)
	established := stores.establishPool(t, weight)
	wantPoolOutcome(t, established, application.PoolEstablished)
	if record, _ := established.Pool(); record.Key.Pool.String() != "SYN-TPL-1/2026-10-21/WEIGHT" {
		t.Fatalf("池身份 = %s", record.Key.Pool)
	}
	wantPoolOutcome(t, stores.establishPool(t, weight), application.PoolExistingResult)
	changed := weight
	changed.Capacity = 6000
	conflict := stores.establishPool(t, changed)
	wantPoolOutcome(t, conflict, application.PoolConflict)
	if record, _ := conflict.Pool(); record.Pool.Capacity() != 5000 {
		t.Fatalf("冲突交回的不是库里那一池：容量 %d", record.Pool.Capacity())
	}

	bags := weight
	bags.Dimension, bags.Unit, bags.Capacity = "BAG_POSITIONS", "BAG", 40
	wantPoolOutcome(t, stores.establishPool(t, bags), application.PoolEstablished)

	for dimension, capacity := range map[domain.CapacityDimension]int64{domain.CapacityWeight: 5000, domain.CapacityBagPositions: 40} {
		pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
			TenantID: tenant, Schedule: deliveryValue(t, domain.NewScheduleReference, "SYN-TPL-1/2026-10-21"),
			Dimension: dimension, Unit: deliveryValue(t, domain.NewQuantityUnitReference, "X"), Capacity: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		found, exists, err := stores.pools.FindByKey(t.Context(), ports.CapacityPoolKey{TenantID: tenant, Pool: pool.Pool()})
		if err != nil || !exists || found.Pool.Dimension() != dimension || found.Pool.Capacity() != capacity {
			t.Fatalf("%s 池读回 = exists=%v %s %d %v", dimension, exists, found.Pool.Dimension(), found.Pool.Capacity(), err)
		}
	}
	rows, err := stores.catalogue.ListCapacityPools(t.Context(), tenant, 10)
	if err != nil || len(rows) != 2 ||
		rows[0].PoolID != "SYN-TPL-1/2026-10-21/BAG_POSITIONS" || rows[0].Dimension != "BAG_POSITIONS" || rows[0].Capacity != 40 ||
		rows[1].PoolID != "SYN-TPL-1/2026-10-21/WEIGHT" || rows[1].Dimension != "WEIGHT" || rows[1].Capacity != 5000 {
		t.Fatalf("池册 = %+v %v", rows, err)
	}
}

// Covers: ADR-0201 决定三「逐维容量声明随物化建池且幂等」与 `AT-TF-106`——周期安排的容量声明在真库上随那一版落册、
// 读回（当前版、候选来源与查阅册三处同一组）；按日期物化登记一班时逐维建池，同一日期再登班次与池都答`已存在`，
// 册上不多一行；周期安排换版改了重量又加了袋位，再登同一日期：重量答`池冲突`且库里那一池不改写，袋位建池。
func TestTemplateCapacitiesMaterializePoolsOnTheRegistry(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	command := syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")
	command.Capacities = []application.CapacityDeclarationInput{
		{Dimension: "PIECES", Unit: "PCS", Capacity: 800},
		{Dimension: "WEIGHT", Unit: "KG", Capacity: 5000},
	}
	wantScheduleOutcome(t, stores.registerTemplate(t, command), application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerTemplate(t, command), application.ScheduleTemplateExisting)

	key := ports.ScheduleTemplateKey{TenantID: tenant, Code: deliveryValue(t, domain.NewScheduleTemplateCode, "SYN-TPL-1")}
	current, found, err := stores.templates.FindCurrentTemplate(t.Context(), key)
	declared := current.Template.Content().Capacities
	if err != nil || !found || len(declared) != 2 || declared[0].Dimension != domain.CapacityWeight || declared[0].Capacity != 5000 ||
		declared[1].Dimension != domain.CapacityPieces || declared[1].Unit.String() != "PCS" {
		t.Fatalf("当前版容量声明读回 = %+v %v", declared, err)
	}
	onConnection, err := stores.candidates.ListCurrentTemplatesOnConnection(t.Context(), tenant,
		deliveryValue(t, domain.NewNetworkConnectionReference, "SYN-CONN-1"))
	if err != nil || len(onConnection) != 1 || len(onConnection[0].Template.Content().Capacities) != 2 {
		t.Fatalf("候选来源读回的周期安排缺容量声明：%+v %v", onConnection, err)
	}
	catalogued, err := stores.catalogue.ListScheduleTemplates(t.Context(), tenant, 10)
	if err != nil || len(catalogued) != 1 || len(catalogued[0].Capacities) != 2 ||
		catalogued[0].Capacities[0].Dimension != "PIECES" || catalogued[0].Capacities[1].Dimension != "WEIGHT" ||
		catalogued[0].Capacities[1].Capacity != 5000 {
		t.Fatalf("周期安排册的容量声明 = %+v %v", catalogued, err)
	}

	materialize := application.RegisterTransportScheduleCommand{TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-21"}
	first := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, first, application.TransportScheduleRegistered)
	if pools := first.Pools(); len(pools) != 2 || pools[0].Outcome != application.PoolEstablished ||
		pools[0].Pool.Key.Pool.String() != "SYN-TPL-1/2026-10-21/WEIGHT" || pools[1].Outcome != application.PoolEstablished ||
		pools[1].Pool.Key.Pool.String() != "SYN-TPL-1/2026-10-21/PIECES" {
		t.Fatalf("物化登记的池 = %+v", pools)
	}
	again := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, again, application.TransportScheduleExisting)
	for _, pool := range again.Pools() {
		if pool.Outcome != application.PoolExistingResult {
			t.Fatalf("同一日期再登 %s = %s", pool.Pool.Key.Pool, pool.Outcome)
		}
	}
	if again.Persisted() || countPools(t, stores, tenant) != 2 {
		t.Fatalf("同一日期再登多落了：persisted=%v 池 %d 个", again.Persisted(), countPools(t, stores, tenant))
	}

	revised := command
	revised.Capacities = []application.CapacityDeclarationInput{
		{Dimension: "WEIGHT", Unit: "KG", Capacity: 6000},
		{Dimension: "PIECES", Unit: "PCS", Capacity: 800},
		{Dimension: "BAG_POSITIONS", Unit: "BAG", Capacity: 40},
	}
	wantScheduleOutcome(t, stores.registerTemplate(t, revised), application.ScheduleTemplateRevised)
	followed := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, followed, application.TransportScheduleExisting)
	outcomes := map[string]application.OpportunityOutcome{}
	for _, pool := range followed.Pools() {
		outcomes[pool.Pool.Key.Pool.String()] = pool.Outcome
	}
	if outcomes["SYN-TPL-1/2026-10-21/WEIGHT"] != application.PoolConflict ||
		outcomes["SYN-TPL-1/2026-10-21/PIECES"] != application.PoolExistingResult ||
		outcomes["SYN-TPL-1/2026-10-21/BAG_POSITIONS"] != application.PoolEstablished || !followed.Persisted() {
		t.Fatalf("换版后再登 = %v persisted=%v", outcomes, followed.Persisted())
	}
	rows, err := stores.catalogue.ListCapacityPools(t.Context(), tenant, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("池册 = %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.Dimension == "WEIGHT" && row.Capacity != 5000 {
			t.Fatalf("周期安排换版改写了已建的重量池：%d", row.Capacity)
		}
	}
}

// Covers: 迁移 0027 的拒绝门——池表有行时不施加，不替任何人把单位猜成维度。
func TestMigration0027RefusesPoolsItCannotGiveADimension(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移资产：%v", err)
	}
	var dimensioning string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0027_") {
			dimensioning = asset.SQL
		}
	}
	if dimensioning == "" {
		t.Fatal("找不到迁移 0027")
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
		t.Fatalf("插探针行：%v", err)
	}
	if _, err := tx.Exec(ctx, dimensioning); err == nil || !strings.Contains(err.Error(), "已有行") {
		t.Fatalf("池表有行时 0027 应拒绝施加：%v", err)
	}
}

// Covers: 迁移 0027 的库面防线——池维度封闭、池身份必须由班次与维度派生；周期安排的容量声明维度封闭、硬上限为正、
// 必须挂在一版在册的周期安排上，坏行插不进。
func TestCapacityRegistryChecksRefuseMalformedRows(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.schedule_template
			(tenant_id, template_code, template_version, connection_code, from_node, to_node, weekdays,
			 departure_local_minute, duration_minutes, time_zone, service_scope_ref, effective_from, content_digest, recorded_at)
		 VALUES ('T', 'TPL', 1, 'C', 'A', 'B', ARRAY['MON'], 60, 60, 'Asia/Shanghai', 'S', '2026-01-01', 'd', now())`); err != nil {
		t.Fatalf("探针父行插不进去：%v", err)
	}
	probes := map[string]string{
		"池维度不在封闭词里": `INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('T', 'S/TEU', 'S', 'TEU', 'TEU', 10, 'd', now())`,
		"池身份不是派生的": `INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('T', 'POOL-1', 'S', 'WEIGHT', 'KG', 10, 'd', now())`,
		"池缺维度": `INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('T', 'S/WEIGHT', 'S', 'KG', 10, 'd', now())`,
		"容量声明维度不在封闭词里": `INSERT INTO transport_fulfillment.schedule_template_capacity
			(tenant_id, template_code, template_version, dimension, unit_ref, capacity)
		 VALUES ('T', 'TPL', 1, 'weight', 'KG', 10)`,
		"容量声明硬上限为零": `INSERT INTO transport_fulfillment.schedule_template_capacity
			(tenant_id, template_code, template_version, dimension, unit_ref, capacity)
		 VALUES ('T', 'TPL', 1, 'WEIGHT', 'KG', 0)`,
		"容量声明缺单位": `INSERT INTO transport_fulfillment.schedule_template_capacity
			(tenant_id, template_code, template_version, dimension, unit_ref, capacity)
		 VALUES ('T', 'TPL', 1, 'WEIGHT', ' ', 10)`,
		"容量声明挂在不在册的一版上": `INSERT INTO transport_fulfillment.schedule_template_capacity
			(tenant_id, template_code, template_version, dimension, unit_ref, capacity)
		 VALUES ('T', 'TPL', 2, 'WEIGHT', 'KG', 10)`,
	}
	for name, probe := range probes {
		if _, err := pool.Exec(ctx, probe); err == nil {
			t.Errorf("%s：坏行溜进了册子", name)
		}
	}
}
