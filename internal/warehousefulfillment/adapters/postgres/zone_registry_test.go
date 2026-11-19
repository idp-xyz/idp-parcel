package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-001（库区的部分）持久化面——首笔落库；换登记责任方与登记时刻的原样重投答已在册，在册那一笔的登记责任方与登记时刻
// 不变；同一修订号换内容答冲突，不覆盖；修订 2 落册，旧修订照留，按（租户，仓编码，库区编码，修订号）取得到任何一笔，取最新得修订 2。
func TestZoneRegistrySavesReplaysAndRefusesToOverwrite(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")

	first := zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, nil)
	if outcome := saveZone(t, stores, first); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 1 = %s，want SAVED", outcome)
	}

	replay := zoneRegisteredBy(t, first, "operator-2", baseInstant.Add(48*time.Hour))
	if outcome := saveZone(t, stores, replay); outcome != ports.RegistryRevisionAlreadyRegistered {
		t.Fatalf("换登记责任方与登记时刻的重放 = %s，want ALREADY_REGISTERED", outcome)
	}
	conflicting := zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（改名）")
	})
	if outcome := saveZone(t, stores, conflicting); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同修订号异内容 = %s，want CONTENT_CONFLICT", outcome)
	}
	differentUsage := zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsagePicking
	})
	if outcome := saveZone(t, stores, differentUsage); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同修订号异用途 = %s，want CONTENT_CONFLICT", outcome)
	}

	loaded, found, err := stores.zones.LoadZoneRevision(ctx, tenant, warehouse, code, 1)
	if err != nil || !found {
		t.Fatalf("读回修订 1 = (%v, %v)", found, err)
	}
	if !loaded.SameContentAs(first) || loaded.Registrant().String() != "operator-1" || !loaded.RegisteredAt().Equal(first.RegisteredAt()) {
		t.Fatalf("重放与冲突之后在册的修订 1 变了：登记责任方 %q、登记时刻 %v、名称 %q、用途 %s",
			loaded.Registrant(), loaded.RegisteredAt(), loaded.Content().Name(), loaded.Content().Usage())
	}

	second := zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 2, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（二）")
		spec.Deactivated = true
	})
	if outcome := saveZone(t, stores, second); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 2 = %s，want SAVED", outcome)
	}
	latest, found, err := stores.zones.LoadLatestZone(ctx, tenant, warehouse, code)
	if err != nil || !found || latest.Revision() != 2 || !latest.SameContentAs(second) || !latest.Content().Deactivated() {
		t.Fatalf("取最新 = (修订 %d, %v, %v)，want 修订 2、已停用", latest.Revision(), found, err)
	}
	if older, found, err := stores.zones.LoadZoneRevision(ctx, tenant, warehouse, code, 1); err != nil || !found || !older.SameContentAs(first) {
		t.Fatalf("修订 2 落册之后再取修订 1 = (%v, %v)，旧修订该照留", found, err)
	}
	if _, found, err := stores.zones.LoadZoneRevision(ctx, tenant, warehouse, code, 3); err != nil || found {
		t.Fatalf("取没登记的修订 3 = (%v, %v)，want (false, nil)", found, err)
	}
}

// Covers: AT-WF-001（库区的部分）「库区编码逐字保存」——带首尾空白、内部连续空白、换行与大小写混写的库区编码与名称原样读回（点读、
// 最新、修订史与列表四条路）；只差大小写或空白的库区编码是各自独立的库区，不互相当作重放或冲突；同一个库区编码在两个仓下也是
// 两个库区——库区编码在仓内才唯一。
func TestZoneRegistryKeepsEveryByteOfVerbatimCellsAndKeepsCodesApart(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	const name = "  合成  存储区\n二楼 "
	zoneCodes := []string{"  Zone-Mixed  01  ", "Z-1", "z-1", " Z-1", "Z-1 ", "z-1\t"}

	for _, code := range zoneCodes {
		mustSaveZones(t, stores, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", code, 1, func(spec *domain.ZoneContentSpec) {
			spec.Name = valueOf(t, domain.NewZoneName, name)
		}))
	}
	// 同一个库区编码在另一个仓下是另一个库区，从修订 1 起；仓编码也逐字：只差大小写的仓是另一个仓。
	mustSaveZones(t, stores,
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-02", "Z-1", 1, nil),
		zoneFixture(t, "SYN-TENANT-01", "syn-wh-01", "Z-1", 1, nil),
	)

	for _, raw := range zoneCodes {
		code := valueOf(t, domain.NewZoneCode, raw)
		warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
		latest, found, err := stores.zones.LoadLatestZone(ctx, tenant, warehouse, code)
		if err != nil || !found {
			t.Fatalf("取最新 %q = (%v, %v)", raw, found, err)
		}
		if latest.Code().String() != raw || latest.Content().Name().String() != name {
			t.Fatalf("取最新 %q 走样：编码 %q、名称 %q", raw, latest.Code(), latest.Content().Name())
		}
		history, err := stores.catalogue.ListZoneRevisions(ctx, tenant, warehouse, code)
		if err != nil || len(history) != 1 || history[0].ZoneCode != raw || history[0].Name != name || history[0].WarehouseCode != "SYN-WH-01" {
			t.Fatalf("修订史 %q = (%+v, %v)，want 一笔、各格原样", raw, history, err)
		}
	}

	page, err := stores.catalogue.ListZones(ctx, tenant, 50, zoneFirstPage(t, nil))
	if err != nil || page.Total != int64(len(zoneCodes)+2) {
		t.Fatalf("列表 = (共 %d, %v)，want %d 个互相独立的库区", page.Total, err, len(zoneCodes)+2)
	}
	listed := zoneKeysOf(page.Rows)
	for _, raw := range zoneCodes {
		if !slices.Contains(listed, "SYN-WH-01/"+raw) {
			t.Fatalf("列表里没有逐字的库区 %q：%q", raw, listed)
		}
	}
	for _, key := range []string{"SYN-WH-02/Z-1", "syn-wh-01/Z-1"} {
		if !slices.Contains(listed, key) {
			t.Fatalf("列表里没有 %q：%q", key, listed)
		}
	}
}

// Covers: 跨租户隔离（UC-WF-001「权限、隔离与审计」）与 AT-WF-003 的持久化面——别的租户的同编码库区读不到：点读、取最新、列表与修订史
// 都与「从没登记」同形；同一仓编码、库区编码与修订号在另一租户落册是新写入，不是重放也不是冲突，也不动这个租户已在册的那一笔。
func TestZoneRegistryIsTenantScoped(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	stranger := valueOf(t, domain.NewTenantID, "SYN-TENANT-03")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")

	mustSaveZones(t, stores,
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, nil),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 2, nil),
		zoneFixture(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-01", 1, func(spec *domain.ZoneContentSpec) {
			spec.Name = valueOf(t, domain.NewZoneName, "另一租户的存储区")
		}),
	)

	if latest, found, err := stores.zones.LoadLatestZone(ctx, other, warehouse, code); err != nil || !found || latest.Revision() != 1 ||
		latest.Content().Name().String() != "另一租户的存储区" {
		t.Fatalf("另一租户取最新 = (修订 %d, %v, %v)，want 它自己的修订 1", latest.Revision(), found, err)
	}
	if first, found, err := stores.zones.LoadZoneRevision(ctx, own, warehouse, code, 1); err != nil || !found || first.Content().Name().String() != "合成存储区" {
		t.Fatalf("本租户的修订 1 被另一租户的写入动了：(%v, %v)", found, err)
	}

	if _, found, err := stores.zones.LoadLatestZone(ctx, stranger, warehouse, code); err != nil || found {
		t.Fatalf("没登记过的租户取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.zones.LoadZoneRevision(ctx, stranger, warehouse, code, 1); err != nil || found {
		t.Fatalf("没登记过的租户点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if page, err := stores.catalogue.ListZones(ctx, stranger, 50, zoneFirstPage(t, nil)); err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("没登记过的租户列表 = (%d 行, 共 %d, %v)，want 空", len(page.Rows), page.Total, err)
	}
	if history, err := stores.catalogue.ListZoneRevisions(ctx, stranger, warehouse, code); err != nil || len(history) != 0 {
		t.Fatalf("没登记过的租户读修订史 = (%+v, %v)，want 空", history, err)
	}
	if history, err := stores.catalogue.ListZoneRevisions(ctx, other, warehouse, code); err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatalf("另一租户的修订史 = (%+v, %v)，want 它自己的一笔", history, err)
	}
}

// Covers: AT-WF-009（库区的部分）——一个库区都没登记的册：取最新、点读都答没有，不报错；迁移不种任何行。
func TestZoneRegistryOfAnEmptyRegisterAnswersNothingAndSeedsNoRows(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")

	if _, found, err := stores.zones.LoadLatestZone(ctx, tenant, warehouse, code); err != nil || found {
		t.Fatalf("空册取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.zones.LoadZoneRevision(ctx, tenant, warehouse, code, 1); err != nil || found {
		t.Fatalf("空册点读 = (%v, %v)，want (false, nil)", found, err)
	}
	var rows int
	if err := stores.pool.QueryRow(ctx, `SELECT count(*) FROM warehouse_fulfillment.zone_revision`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("迁移之后表里有 %d 行（%v），want 不带任何默认行", rows, err)
	}
}

// 读口的入参没经构造器（零值）是装配错了，答 error，不折成「没有」：折成没有会让登记方从修订 1 重登一个本来就在册的库区。
func TestZoneRegistryRefusesUnconstructedKeysOnRead(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")

	for name, call := range map[string]func() error{
		"缺租户的取最新": func() error {
			_, _, err := stores.zones.LoadLatestZone(ctx, domain.TenantID{}, warehouse, code)
			return err
		},
		"缺仓编码的取最新": func() error {
			_, _, err := stores.zones.LoadLatestZone(ctx, tenant, domain.WarehouseCode{}, code)
			return err
		},
		"缺库区编码的取最新": func() error {
			_, _, err := stores.zones.LoadLatestZone(ctx, tenant, warehouse, domain.ZoneCode{})
			return err
		},
		"缺租户的点读": func() error {
			_, _, err := stores.zones.LoadZoneRevision(ctx, domain.TenantID{}, warehouse, code, 1)
			return err
		},
		"缺仓编码的点读": func() error {
			_, _, err := stores.zones.LoadZoneRevision(ctx, tenant, domain.WarehouseCode{}, code, 1)
			return err
		},
		"缺库区编码的点读": func() error {
			_, _, err := stores.zones.LoadZoneRevision(ctx, tenant, warehouse, domain.ZoneCode{}, 1)
			return err
		},
		"缺租户的修订史": func() error {
			_, err := stores.catalogue.ListZoneRevisions(ctx, domain.TenantID{}, warehouse, code)
			return err
		},
		"缺租户的列表": func() error {
			_, err := stores.catalogue.ListZones(ctx, domain.TenantID{}, 50, zoneFirstPage(t, nil))
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s应报错", name)
		}
	}
}

// 写口在没有环境事务时拒绝（PBC-08）：登记的落册与它的读必须在同一个事务里，无事务就走自动提交会让「查过没有再写」失去意义。
func TestZoneWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newZoneStores(t)
	if _, err := stores.zones.SaveZoneRevision(t.Context(), zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, nil)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveZoneRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 库上的 CHECK 与主键各守一格：封闭的六种用途（与领域的封闭集逐种对得上）、修订号为正、非空白、同一修订号至多一笔。绕过领域构造器
// 直接插行，逐一证库上真拒（不是只靠领域那一层）。库区表不对仓表建外键：「仓须已登记」由应用层判，所以库上插得进一个指名了没登记的
// 仓的库区行——这是有意的，登记册只追加，外键换不来任何应用层没有的保证。
func TestZoneRevisionTableGuardsItsClosedSetsAndKeys(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()

	type row struct {
		tenant, warehouse, code string
		revision                int
		name, usage             string
		registrant              string
	}
	good := row{tenant: "SYN-TENANT-01", warehouse: "SYN-WH-01", code: "SYN-ZONE-01", revision: 1,
		name: "合成存储区", usage: "STORAGE", registrant: "operator-1"}
	insert := func(candidate row) error {
		_, err := stores.pool.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.zone_revision
				(tenant_id, warehouse_code, zone_code, revision, zone_name, zone_usage, deactivated, registrant, registered_at)
			 VALUES ($1, $2, $3, $4, $5, $6, false, $7, now())`,
			candidate.tenant, candidate.warehouse, candidate.code, candidate.revision, candidate.name, candidate.usage,
			candidate.registrant)
		return err
	}
	violation := func(err error) string {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return pgErr.Code
		}
		return ""
	}

	for name, mutate := range map[string]func(*row){
		"集外的用途":   func(r *row) { r.usage = "COLD_STORAGE" },
		"小写的用途":   func(r *row) { r.usage = "storage" },
		"用途带空白":   func(r *row) { r.usage = "STORAGE " },
		"用途为空":    func(r *row) { r.usage = "" },
		"修订号为 0":  func(r *row) { r.revision = 0 },
		"仓编码全空格":  func(r *row) { r.warehouse = "  " },
		"库区编码全空格": func(r *row) { r.code = "   " },
		"库区名称为空":  func(r *row) { r.name = "" },
		"登记责任方为空": func(r *row) { r.registrant = "" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "23514" {
			t.Errorf("%s：插入应被 CHECK 拒（23514），实得 %v", name, err)
		}
	}

	// 领域的封闭集逐种放得进：库上的封闭集与领域的封闭集是同一份。
	for index, usage := range domain.ZoneUsages() {
		candidate := good
		candidate.code, candidate.usage = "SYN-ZONE-USAGE-"+usage.String(), usage.String()
		if err := insert(candidate); err != nil {
			t.Errorf("第 %d 种用途 %s 被库拒了：%v", index+1, usage, err)
		}
	}

	if err := insert(good); err != nil {
		t.Fatalf("合法行应能插入（仓册里没有这个仓也插得进：不建外键）：%v", err)
	}
	if err := insert(good); violation(err) != "23505" {
		t.Errorf("同一（租户，仓编码，库区编码，修订号）插两次应被主键拒（23505），实得 %v", err)
	}
	for name, mutate := range map[string]func(*row){
		"另一租户":      func(r *row) { r.tenant = "SYN-TENANT-02" },
		"另一个仓":      func(r *row) { r.warehouse = "SYN-WH-02" },
		"另一个库区编码":   func(r *row) { r.code = "SYN-ZONE-02" },
		"同一库区的修订 2": func(r *row) { r.revision = 2 },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); err != nil {
			t.Errorf("%s不该被拒：%v", name, err)
		}
	}
}

// Covers: AT-WF-003 与 AT-WF-009（库区的部分）端到端——登记用例接真的仓册与库区册：仓没登记时答 REFERENCE_NOT_FOUND 并指出仓那一格，
// 不落册；登记仓之后同一修订原样重交落册；别的租户登记的同编码仓不算本租户的仓；停用的仓下照常登记库区。仓册与库区册是两张表，
// 「仓已登记」靠应用层经仓册的读法判。
func TestRegisterZoneOnTheRealRegistersNeedsTheWarehouseRegisteredForTheSameTenant(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	handler := application.NewRegisterZoneHandler(stores.zones, stores.warehouses, fixedClock{at: baseInstant})

	register := func(tenant, warehouse string, revision int) application.RegisterZoneResult {
		t.Helper()
		command := application.RegisterZoneCommand{
			Tenant:     valueOf(t, domain.NewTenantID, tenant),
			Warehouse:  valueOf(t, domain.NewWarehouseCode, warehouse),
			Code:       valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"),
			Revision:   revision,
			Content:    zoneContentWith(t, nil),
			Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
		}
		var result application.RegisterZoneResult
		err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			result, registerErr = handler.Register(txCtx, command)
			return registerErr
		})
		if err != nil {
			t.Fatalf("登记库区：%v", err)
		}
		return result
	}
	expectRefused := func(label string, result application.RegisterZoneResult) {
		t.Helper()
		if result.Outcome() != application.RegistrationNotAccepted ||
			result.Refusal() != application.RegistrationRefusalReferenceNotFound || result.Field() != domain.ZoneFieldWarehouseCode {
			t.Fatalf("%s = %s/%s 指向 %q，want NOT_ACCEPTED/REFERENCE_NOT_FOUND 指向 warehouseCode",
				label, result.Outcome(), result.Refusal(), result.Field())
		}
	}

	expectRefused("一个仓都没登记", register("SYN-TENANT-01", "SYN-WH-01", 1))
	if rows := countZoneRows(t, stores); rows != 0 {
		t.Fatalf("被拒之后册里有 %d 笔，want 不落册", rows)
	}

	// 别的租户登记了同编码的仓：不算本租户的仓。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-02", "SYN-WH-01", 1, nil))
	expectRefused("别的租户登记了同编码的仓", register("SYN-TENANT-01", "SYN-WH-01", 1))

	// 本租户登记仓之后，同一修订原样重交落册。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil))
	if landed := register("SYN-TENANT-01", "SYN-WH-01", 1); landed.Outcome() != application.RegistrationRegistered {
		t.Fatalf("登记仓之后原样重交 = %s，want REGISTERED", landed.Outcome())
	}
	if replay := register("SYN-TENANT-01", "SYN-WH-01", 1); replay.Outcome() != application.RegistrationAlreadyRegistered {
		t.Fatalf("再原样重交 = %s，want ALREADY_REGISTERED", replay.Outcome())
	}

	// 仓停用之后，库区照常登记新修订；另一个仓名下的库区也照常登记。
	mustSave(t, stores.transactor, stores.warehouses,
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) { spec.Deactivated = true }),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-02", 1, func(spec *domain.WarehouseContentSpec) { spec.Deactivated = true }),
	)
	if renamed := register("SYN-TENANT-01", "SYN-WH-01", 2); renamed.Outcome() != application.RegistrationRegistered {
		t.Fatalf("仓停用之后登记库区修订 2 = %s，want REGISTERED", renamed.Outcome())
	}
	if second := register("SYN-TENANT-01", "SYN-WH-02", 1); second.Outcome() != application.RegistrationRegistered {
		t.Fatalf("停用的仓名下登记库区 = %s，want REGISTERED", second.Outcome())
	}
	if rows := countZoneRows(t, stores); rows != 3 {
		t.Fatalf("册里 %d 笔，want 3（一号仓两笔、二号仓一笔；被拒与重放都不落册）", rows)
	}
}

func countZoneRows(t *testing.T, stores zoneStores) int {
	t.Helper()
	var rows int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM warehouse_fulfillment.zone_revision`).Scan(&rows); err != nil {
		t.Fatalf("数册里的行：%v", err)
	}
	return rows
}
