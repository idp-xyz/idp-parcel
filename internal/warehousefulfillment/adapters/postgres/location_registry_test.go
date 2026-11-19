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

// Covers: AT-WF-001（库位的部分）持久化面——首笔落库；换登记责任方与登记时刻的原样重投答已在册，在册那一笔的登记责任方与登记时刻
// 不变；同一修订号换内容答冲突，不覆盖；修订 2 落册，旧修订照留，按（租户，仓编码，库位编码，修订号）取得到任何一笔，取最新得修订 2。
func TestLocationRegistrySavesReplaysAndRefusesToOverwrite(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")
	full := combined(typedAs(t, "SYN-LT-01"), prioritisedAs(5))

	first := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 1, full)
	if outcome := saveLocation(t, stores, first); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 1 = %s，want SAVED", outcome)
	}

	replay := locationRegisteredBy(t, first, "operator-2", baseInstant.Add(48*time.Hour))
	if outcome := saveLocation(t, stores, replay); outcome != ports.RegistryRevisionAlreadyRegistered {
		t.Fatalf("换登记责任方与登记时刻的重放 = %s，want ALREADY_REGISTERED", outcome)
	}
	for name, edit := range map[string]func(*domain.LocationContentSpec){
		"异优先级":  combined(typedAs(t, "SYN-LT-01"), prioritisedAs(6)),
		"异库位类型": combined(typedAs(t, "SYN-LT-02"), prioritisedAs(5)),
		"异库区":   combined(inZoneOf(t, "SYN-ZONE-02"), typedAs(t, "SYN-LT-01"), prioritisedAs(5)),
		"去掉类型":  prioritisedAs(5),
		"去掉优先级": typedAs(t, "SYN-LT-01"),
		"停用":    combined(full, deactivatedLocation),
	} {
		conflicting := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 1, edit)
		if outcome := saveLocation(t, stores, conflicting); outcome != ports.RegistryRevisionContentConflict {
			t.Fatalf("同修订号%s = %s，want CONTENT_CONFLICT", name, outcome)
		}
	}

	loaded, found, err := stores.locations.LoadLocationRevision(ctx, tenant, warehouse, code, 1)
	if err != nil || !found {
		t.Fatalf("读回修订 1 = (%v, %v)", found, err)
	}
	if !loaded.SameContentAs(first) || loaded.Registrant().String() != "operator-1" || !loaded.RegisteredAt().Equal(first.RegisteredAt()) {
		t.Fatalf("重放与冲突之后在册的修订 1 变了：登记责任方 %q、登记时刻 %v、内容 %+v", loaded.Registrant(), loaded.RegisteredAt(), loaded.Content())
	}

	second := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 2, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(9), deactivatedLocation))
	if outcome := saveLocation(t, stores, second); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 2 = %s，want SAVED", outcome)
	}
	latest, found, err := stores.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
	if err != nil || !found || latest.Revision() != 2 || !latest.SameContentAs(second) || !latest.Content().Deactivated() {
		t.Fatalf("取最新 = (修订 %d, %v, %v)，want 修订 2、已停用", latest.Revision(), found, err)
	}
	if older, found, err := stores.locations.LoadLocationRevision(ctx, tenant, warehouse, code, 1); err != nil || !found || !older.SameContentAs(first) {
		t.Fatalf("修订 2 落册之后再取修订 1 = (%v, %v)，旧修订该照留", found, err)
	}
	if _, found, err := stores.locations.LoadLocationRevision(ctx, tenant, warehouse, code, 3); err != nil || found {
		t.Fatalf("取没登记的修订 3 = (%v, %v)，want (false, nil)", found, err)
	}
}

// Covers: AT-WF-001（库位的部分）与票面「库位」——库位类型与优先级「没有」与「有」是不同的内容，库里存 NULL 与存值分得开：没带类型也没给
// 优先级的修订读回仍是没有（不是空串或 0）；优先级 0 读回是 0、给了；负数与 32 位两端原样读回；同一修订号一笔带优先级 0、一笔不带，是冲突。
func TestLocationRegistryKeepsAbsentAndPresentOptionalCellsApart(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")

	for _, test := range []struct {
		code string
		edit func(*domain.LocationContentSpec)
		// wantType 空串就是没带库位类型；wantPriority 为 nil 就是没给优先级。
		wantType     string
		wantPriority *int32
	}{
		{"SYN-LOC-BARE", nil, "", nil},
		{"SYN-LOC-TYPE", typedAs(t, "SYN-LT-01"), "SYN-LT-01", nil},
		{"SYN-LOC-ZERO", prioritisedAs(0), "", ptrTo(0)},
		{"SYN-LOC-NEGATIVE", prioritisedAs(-7), "", ptrTo(-7)},
		{"SYN-LOC-MAX", prioritisedAs(2147483647), "", ptrTo(2147483647)},
		{"SYN-LOC-MIN", prioritisedAs(-2147483648), "", ptrTo(-2147483648)},
		{"SYN-LOC-BOTH", combined(typedAs(t, "SYN-LT-01"), prioritisedAs(3)), "SYN-LT-01", ptrTo(3)},
	} {
		mustSaveLocations(t, stores, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", test.code, 1, test.edit))

		code := valueOf(t, domain.NewLocationCode, test.code)
		latest, found, err := stores.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
		if err != nil || !found {
			t.Fatalf("取最新 %s = (%v, %v)", test.code, found, err)
		}
		gotType, hasType := latest.Content().LocationType()
		if hasType != (test.wantType != "") || gotType.String() != test.wantType {
			t.Errorf("%s 读回库位类型 = (%q, %v)，want %q", test.code, gotType.String(), hasType, test.wantType)
		}
		if latest.Content().Priority().Given() != (test.wantPriority != nil) ||
			(test.wantPriority != nil && latest.Content().Priority().Value() != *test.wantPriority) {
			t.Errorf("%s 读回优先级 = (%d, %v)，want %v", test.code, latest.Content().Priority().Value(), latest.Content().Priority().Given(), test.wantPriority)
		}

		history, err := stores.catalogue.ListLocationRevisions(ctx, tenant, warehouse, code)
		if err != nil || len(history) != 1 {
			t.Fatalf("修订史 %s = (%+v, %v)", test.code, history, err)
		}
		row := history[0]
		if (row.LocationTypeCode == nil) != (test.wantType == "") || (row.LocationTypeCode != nil && *row.LocationTypeCode != test.wantType) {
			t.Errorf("%s 读面的库位类型 = %v，want %q", test.code, row.LocationTypeCode, test.wantType)
		}
		if (row.Priority == nil) != (test.wantPriority == nil) || (row.Priority != nil && *row.Priority != *test.wantPriority) {
			t.Errorf("%s 读面的优先级 = %v，want %v", test.code, row.Priority, test.wantPriority)
		}
	}

	// 库里确是 NULL，不是空串或 0。
	var nullType, nullPriority int
	if err := stores.pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE location_type_code IS NULL), count(*) FILTER (WHERE priority IS NULL)
		   FROM warehouse_fulfillment.location_revision WHERE location_code = 'SYN-LOC-BARE'`).Scan(&nullType, &nullPriority); err != nil ||
		nullType != 1 || nullPriority != 1 {
		t.Fatalf("没有库位类型与优先级的行在库里 = (类型 NULL %d、优先级 NULL %d, %v)，want 都是 NULL", nullType, nullPriority, err)
	}

	// 同一修订号一笔带优先级 0、一笔不带：冲突，不覆盖。
	zero := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-BARE", 1, prioritisedAs(0))
	if outcome := saveLocation(t, stores, zero); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同一修订号带优先级 0 与不带 = %s，want CONTENT_CONFLICT", outcome)
	}
	// 反过来也是冲突。
	absent := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-ZERO", 1, nil)
	if outcome := saveLocation(t, stores, absent); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同一修订号不带优先级与带 0 = %s，want CONTENT_CONFLICT", outcome)
	}
	// 上一笔带类型、这一笔不带：是一笔合法的新修订。
	if outcome := saveLocation(t, stores, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-TYPE", 2, nil)); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("去掉库位类型的修订 2 = %s，want SAVED", outcome)
	}
}

func ptrTo(value int32) *int32 { return &value }

// Covers: AT-WF-001（库位的部分）「库位编码逐字保存」——带首尾空白、内部连续空白、换行与大小写混写的库位编码与所属库区、库位类型编码原样
// 读回（取最新、修订史与列表三条路）；只差大小写或空白的库位编码是各自独立的库位，不互相当作重放或冲突；同一个库位编码在两个仓下也是
// 两个库位——库位编码在仓内才唯一。
func TestLocationRegistryKeepsEveryByteOfVerbatimCellsAndKeepsCodesApart(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	const zone, locationType = "  Zone-Mixed\t01 ", " Lt  Mixed\n"
	locationCodes := []string{"  Loc-Mixed  01  ", "L-1", "l-1", " L-1", "L-1 ", "l-1\t"}

	for _, code := range locationCodes {
		mustSaveLocations(t, stores, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", code, 1, combined(inZoneOf(t, zone), typedAs(t, locationType))))
	}
	// 同一个库位编码在另一个仓下是另一个库位，从修订 1 起；仓编码也逐字：只差大小写的仓是另一个仓。
	mustSaveLocations(t, stores,
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-02", "L-1", 1, nil),
		locationFixture(t, "SYN-TENANT-01", "syn-wh-01", "L-1", 1, nil),
	)

	for _, raw := range locationCodes {
		code := valueOf(t, domain.NewLocationCode, raw)
		warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
		latest, found, err := stores.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
		if err != nil || !found {
			t.Fatalf("取最新 %q = (%v, %v)", raw, found, err)
		}
		gotType, _ := latest.Content().LocationType()
		if latest.Code().String() != raw || latest.Content().ZoneCode().String() != zone || gotType.String() != locationType {
			t.Fatalf("取最新 %q 走样：编码 %q、库区 %q、库位类型 %q", raw, latest.Code(), latest.Content().ZoneCode(), gotType)
		}
		history, err := stores.catalogue.ListLocationRevisions(ctx, tenant, warehouse, code)
		if err != nil || len(history) != 1 || history[0].LocationCode != raw || history[0].ZoneCode != zone ||
			history[0].LocationTypeCode == nil || *history[0].LocationTypeCode != locationType || history[0].WarehouseCode != "SYN-WH-01" {
			t.Fatalf("修订史 %q = (%+v, %v)，want 一笔、各格原样", raw, history, err)
		}
	}

	page, err := stores.catalogue.ListLocations(ctx, tenant, 50, locationFirstPage(t, nil))
	if err != nil || page.Total != int64(len(locationCodes)+2) {
		t.Fatalf("列表 = (共 %d, %v)，want %d 个互相独立的库位", page.Total, err, len(locationCodes)+2)
	}
	listed := locationKeysOf(page.Rows)
	for _, raw := range locationCodes {
		if !slices.Contains(listed, "SYN-WH-01/"+raw) {
			t.Fatalf("列表里没有逐字的库位 %q：%q", raw, listed)
		}
	}
	for _, key := range []string{"SYN-WH-02/L-1", "syn-wh-01/L-1"} {
		if !slices.Contains(listed, key) {
			t.Fatalf("列表里没有 %q：%q", key, listed)
		}
	}
}

// Covers: 跨租户隔离（UC-WF-001「权限、隔离与审计」）与 AT-WF-003 的持久化面——别的租户的同编码库位读不到：点读、取最新、列表与修订史
// 都与「从没登记」同形；同一仓编码、库位编码与修订号在另一租户落册是新写入，不是重放也不是冲突，也不动这个租户已在册的那一笔。
func TestLocationRegistryIsTenantScoped(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	stranger := valueOf(t, domain.NewTenantID, "SYN-TENANT-03")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")

	mustSaveLocations(t, stores,
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 1, prioritisedAs(1)),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 2, prioritisedAs(2)),
		locationFixture(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-LOC-01", 1, prioritisedAs(77)),
	)

	if latest, found, err := stores.locations.LoadLatestLocation(ctx, other, warehouse, code); err != nil || !found || latest.Revision() != 1 ||
		latest.Content().Priority().Value() != 77 {
		t.Fatalf("另一租户取最新 = (修订 %d, %v, %v)，want 它自己的修订 1", latest.Revision(), found, err)
	}
	if first, found, err := stores.locations.LoadLocationRevision(ctx, own, warehouse, code, 1); err != nil || !found || first.Content().Priority().Value() != 1 {
		t.Fatalf("本租户的修订 1 被另一租户的写入动了：(%v, %v)", found, err)
	}

	if _, found, err := stores.locations.LoadLatestLocation(ctx, stranger, warehouse, code); err != nil || found {
		t.Fatalf("没登记过的租户取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.locations.LoadLocationRevision(ctx, stranger, warehouse, code, 1); err != nil || found {
		t.Fatalf("没登记过的租户点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if page, err := stores.catalogue.ListLocations(ctx, stranger, 50, locationFirstPage(t, nil)); err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("没登记过的租户列表 = (%d 行, 共 %d, %v)，want 空", len(page.Rows), page.Total, err)
	}
	if history, err := stores.catalogue.ListLocationRevisions(ctx, stranger, warehouse, code); err != nil || len(history) != 0 {
		t.Fatalf("没登记过的租户读修订史 = (%+v, %v)，want 空", history, err)
	}
	if history, err := stores.catalogue.ListLocationRevisions(ctx, other, warehouse, code); err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatalf("另一租户的修订史 = (%+v, %v)，want 它自己的一笔", history, err)
	}
}

// Covers: AT-WF-009（库位的部分）——一个库位都没登记的册：取最新、点读都答没有，不报错；迁移不种任何行。
func TestLocationRegistryOfAnEmptyRegisterAnswersNothingAndSeedsNoRows(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")

	if _, found, err := stores.locations.LoadLatestLocation(ctx, tenant, warehouse, code); err != nil || found {
		t.Fatalf("空册取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.locations.LoadLocationRevision(ctx, tenant, warehouse, code, 1); err != nil || found {
		t.Fatalf("空册点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if rows := countLocationRows(t, stores); rows != 0 {
		t.Fatalf("迁移之后表里有 %d 行，want 不带任何默认行", rows)
	}
}

// 读口的入参没经构造器（零值）是装配错了，答 error，不折成「没有」：折成没有会让登记方从修订 1 重登一个本来就在册的库位。
func TestLocationRegistryRefusesUnconstructedKeysOnRead(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")

	for name, call := range map[string]func() error{
		"缺租户的取最新": func() error {
			_, _, err := stores.locations.LoadLatestLocation(ctx, domain.TenantID{}, warehouse, code)
			return err
		},
		"缺仓编码的取最新": func() error {
			_, _, err := stores.locations.LoadLatestLocation(ctx, tenant, domain.WarehouseCode{}, code)
			return err
		},
		"缺库位编码的取最新": func() error {
			_, _, err := stores.locations.LoadLatestLocation(ctx, tenant, warehouse, domain.LocationCode{})
			return err
		},
		"缺租户的点读": func() error {
			_, _, err := stores.locations.LoadLocationRevision(ctx, domain.TenantID{}, warehouse, code, 1)
			return err
		},
		"缺仓编码的点读": func() error {
			_, _, err := stores.locations.LoadLocationRevision(ctx, tenant, domain.WarehouseCode{}, code, 1)
			return err
		},
		"缺库位编码的点读": func() error {
			_, _, err := stores.locations.LoadLocationRevision(ctx, tenant, warehouse, domain.LocationCode{}, 1)
			return err
		},
		"缺租户的修订史": func() error {
			_, err := stores.catalogue.ListLocationRevisions(ctx, domain.TenantID{}, warehouse, code)
			return err
		},
		"缺租户的列表": func() error {
			_, err := stores.catalogue.ListLocations(ctx, domain.TenantID{}, 50, locationFirstPage(t, nil))
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s应报错", name)
		}
	}
}

// 写口在没有环境事务时拒绝（PBC-08）：登记的落册与它的读必须在同一个事务里，无事务就走自动提交会让「查过没有再写」失去意义。
func TestLocationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newLocationStores(t)
	if _, err := stores.locations.SaveLocationRevision(t.Context(), locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 1, nil)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveLocationRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 库上的 CHECK 与主键各守一格：修订号为正、必填文本非空白、可空的库位类型编码一旦给了就不是空白、优先级装得进 32 位整数、同一修订号至多
// 一笔。绕过领域构造器直接插行，逐一证库上真拒（不是只靠领域那一层）。库位表不对仓、库区与库位类型表建外键：「已登记」由应用层判，
// 所以库上插得进一个指名了没登记的库区的库位行——这是有意的，登记册只追加，外键换不来任何应用层没有的保证。
func TestLocationRevisionTableGuardsItsKeysAndNullableCells(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()

	type row struct {
		tenant, warehouse, code string
		revision                int
		zone                    string
		locationType            *string
		priority                any
		registrant              string
	}
	blank, typeCode := "   ", "SYN-LT-01"
	good := row{tenant: "SYN-TENANT-01", warehouse: "SYN-WH-01", code: "SYN-LOC-01", revision: 1, zone: "SYN-ZONE-01", registrant: "operator-1"}
	insert := func(candidate row) error {
		_, err := stores.pool.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.location_revision
				(tenant_id, warehouse_code, location_code, revision, zone_code, location_type_code, priority, deactivated, registrant, registered_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, now())`,
			candidate.tenant, candidate.warehouse, candidate.code, candidate.revision, candidate.zone, candidate.locationType,
			candidate.priority, candidate.registrant)
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
		"修订号为 0":     func(r *row) { r.revision = 0 },
		"仓编码全空格":     func(r *row) { r.warehouse = "  " },
		"库位编码全空格":    func(r *row) { r.code = "   " },
		"所属库区全空格":    func(r *row) { r.zone = " " },
		"所属库区为空":     func(r *row) { r.zone = "" },
		"登记责任方为空":    func(r *row) { r.registrant = "" },
		"库位类型给了却是空格": func(r *row) { r.locationType = &blank },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "23514" {
			t.Errorf("%s：插入应被 CHECK 拒（23514），实得 %v", name, err)
		}
	}

	// 优先级列就是 32 位整数：超出范围由类型本身拒（22003），不靠另写的 CHECK。
	for name, literal := range map[string]string{"大于 32 位最大值": "2147483648", "小于 32 位最小值": "-2147483649"} {
		statement := `INSERT INTO warehouse_fulfillment.location_revision
			(tenant_id, warehouse_code, location_code, revision, zone_code, priority, deactivated, registrant, registered_at)
			VALUES ('SYN-TENANT-01', 'SYN-WH-01', 'SYN-LOC-RANGE', 1, 'SYN-ZONE-01', ` + literal + `, false, 'operator-1', now())`
		if _, err := stores.pool.Exec(ctx, statement); violation(err) != "22003" {
			t.Errorf("优先级%s：插入应被类型拒（22003），实得 %v", name, err)
		}
	}
	// 必填列不可空。
	for name, statement := range map[string]string{
		"所属库区为 NULL": `INSERT INTO warehouse_fulfillment.location_revision
			(tenant_id, warehouse_code, location_code, revision, zone_code, deactivated, registrant, registered_at)
			VALUES ('SYN-TENANT-01', 'SYN-WH-01', 'SYN-LOC-NULL', 1, NULL, false, 'operator-1', now())`,
		"是否停用为 NULL": `INSERT INTO warehouse_fulfillment.location_revision
			(tenant_id, warehouse_code, location_code, revision, zone_code, deactivated, registrant, registered_at)
			VALUES ('SYN-TENANT-01', 'SYN-WH-01', 'SYN-LOC-NULL', 1, 'SYN-ZONE-01', NULL, 'operator-1', now())`,
	} {
		if _, err := stores.pool.Exec(ctx, statement); violation(err) != "23502" {
			t.Errorf("%s：应被 NOT NULL 拒（23502），实得 %v", name, err)
		}
	}

	// 可空的两列给 NULL 与给值都放得进：NULL 是「没有」，不被 CHECK 的三值逻辑误拒。
	withType, withPriority := good, good
	withType.code, withType.locationType = "SYN-LOC-TYPED", &typeCode
	withPriority.code, withPriority.priority = "SYN-LOC-PRIORITISED", int32(-2147483648)
	for name, candidate := range map[string]row{"没有库位类型与优先级": good, "带库位类型": withType, "带优先级": withPriority} {
		if err := insert(candidate); err != nil {
			t.Errorf("%s被库拒了：%v（不建外键，仓、库区与库位类型册里都没有它们也插得进）", name, err)
		}
	}
	if err := insert(good); violation(err) != "23505" {
		t.Errorf("同一（租户，仓编码，库位编码，修订号）插两次应被主键拒（23505），实得 %v", err)
	}
	for name, mutate := range map[string]func(*row){
		"另一租户":      func(r *row) { r.tenant = "SYN-TENANT-02" },
		"另一个仓":      func(r *row) { r.warehouse = "SYN-WH-02" },
		"另一个库位编码":   func(r *row) { r.code = "SYN-LOC-02" },
		"同一库位的修订 2": func(r *row) { r.revision = 2 },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); err != nil {
			t.Errorf("%s不该被拒：%v", name, err)
		}
	}
}

// Covers: AT-WF-001、AT-WF-002、AT-WF-003 与 AT-WF-009（库位的部分）端到端——登记用例接真的仓册、库区册、库位类型册与库位册：
// 没登记的仓、没登记的库区、另一个仓下的库区、没登记的库位类型各答 REFERENCE_NOT_FOUND 并指出对应的格，不落册；别的租户登记的同编码
// 仓、库区与库位类型不算本租户的；全登记齐之后同一笔受理，原样重投答已在册；把库位类型停用之后，新库位指名它答 REFERENCE_DEACTIVATED，
// 原来的库位沿用它只改优先级答 REGISTERED；把库位改到另一个库区答 IMMUTABLE_FIELD；停用的仓与库区下照常登记库位。
func TestRegisterLocationOnTheRealRegistersChecksEveryNamedObject(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	handler := application.NewRegisterLocationHandler(stores.locations, stores.warehouses, stores.zones, stores.locationTypes, fixedClock{at: baseInstant})

	register := func(warehouse, code string, revision int, edit func(*domain.LocationContentSpec)) application.RegisterLocationResult {
		t.Helper()
		command := application.RegisterLocationCommand{
			Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
			Warehouse:  valueOf(t, domain.NewWarehouseCode, warehouse),
			Code:       valueOf(t, domain.NewLocationCode, code),
			Revision:   revision,
			Content:    locationContentWith(t, edit),
			Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
		}
		var result application.RegisterLocationResult
		err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			result, registerErr = handler.Register(txCtx, command)
			return registerErr
		})
		if err != nil {
			t.Fatalf("登记库位：%v", err)
		}
		return result
	}
	expectRefused := func(label string, result application.RegisterLocationResult, refusal application.RegistrationRefusal, field domain.LocationField) {
		t.Helper()
		if result.Outcome() != application.RegistrationNotAccepted || result.Refusal() != refusal || result.Field() != field {
			t.Fatalf("%s = %s/%s 指向 %q，want NOT_ACCEPTED/%s 指向 %q", label, result.Outcome(), result.Refusal(), result.Field(), refusal, field)
		}
	}
	expectOutcome := func(label string, result application.RegisterLocationResult, want application.RegistrationOutcome) {
		t.Helper()
		if result.Outcome() != want {
			t.Fatalf("%s = %s/%s 指向 %q，want %s", label, result.Outcome(), result.Refusal(), result.Field(), want)
		}
	}
	notFound := application.RegistrationRefusalReferenceNotFound

	// 一个仓都没登记。
	expectRefused("一个仓都没登记", register("SYN-WH-01", "SYN-LOC-01", 1, nil), notFound, domain.LocationFieldWarehouseCode)
	if rows := countLocationRows(t, stores); rows != 0 {
		t.Fatalf("被拒之后册里有 %d 笔，want 不落册", rows)
	}

	// 别的租户登记了同编码的仓、库区与库位类型：不算本租户的。
	stores.mustSaveWarehouse(t, "SYN-TENANT-02", "SYN-WH-01", 1, false)
	stores.mustSaveZone(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-01", false)
	stores.mustSaveLocationType(t, "SYN-TENANT-02", "SYN-LT-01", 1, false)
	expectRefused("别的租户登记了同编码的仓", register("SYN-WH-01", "SYN-LOC-01", 1, nil), notFound, domain.LocationFieldWarehouseCode)

	// 仓登记了，库区没有；库区在另一个仓下登记了，这个仓下仍没有。
	stores.mustSaveWarehouse(t, "SYN-TENANT-01", "SYN-WH-01", 1, false)
	stores.mustSaveWarehouse(t, "SYN-TENANT-01", "SYN-WH-02", 1, false)
	expectRefused("库区没登记", register("SYN-WH-01", "SYN-LOC-01", 1, nil), notFound, domain.LocationFieldZoneCode)
	stores.mustSaveZone(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-01", false)
	expectRefused("库区在另一个仓下登记了", register("SYN-WH-01", "SYN-LOC-01", 1, nil), notFound, domain.LocationFieldZoneCode)

	// 库区在本仓下登记了，库位类型没有（别的租户登记的同编码类型不算）。
	stores.mustSaveZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", false)
	expectRefused("库位类型没登记", register("SYN-WH-01", "SYN-LOC-01", 1, typedAs(t, "SYN-LT-01")), notFound, domain.LocationFieldLocationTypeCode)
	if rows := countLocationRows(t, stores); rows != 0 {
		t.Fatalf("各种拒绝之后册里有 %d 笔，want 不落册", rows)
	}

	// 不带库位类型的库位这时就能登记；登记齐库位类型之后，带类型与优先级的库位受理，原样重投答已在册。
	expectOutcome("不带库位类型的库位", register("SYN-WH-01", "SYN-LOC-BARE", 1, nil), application.RegistrationRegistered)
	stores.mustSaveLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 1, false)
	full := combined(typedAs(t, "SYN-LT-01"), prioritisedAs(5))
	expectOutcome("带类型与优先级的库位", register("SYN-WH-01", "SYN-LOC-01", 1, full), application.RegistrationRegistered)
	expectOutcome("原样重投", register("SYN-WH-01", "SYN-LOC-01", 1, full), application.RegistrationAlreadyRegistered)

	// 把库位类型停用之后：新库位指名它答所指已停用；原来的库位沿用它只改优先级照常受理；去掉类型再指名同一个类型算新指名。
	stores.mustSaveLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 2, true)
	expectRefused("新库位指名已停用的类型", register("SYN-WH-01", "SYN-LOC-02", 1, typedAs(t, "SYN-LT-01")),
		application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	expectOutcome("沿用已停用的类型只改优先级", register("SYN-WH-01", "SYN-LOC-01", 2, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(6))),
		application.RegistrationRegistered)
	expectOutcome("去掉类型", register("SYN-WH-01", "SYN-LOC-01", 3, prioritisedAs(6)), application.RegistrationRegistered)
	expectRefused("去掉类型之后又指名同一个已停用的类型", register("SYN-WH-01", "SYN-LOC-01", 4, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(6))),
		application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)

	// 把库位改到另一个库区答不可改；册不变。
	stores.mustSaveZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-02", false)
	rowsBefore := countLocationRows(t, stores)
	expectRefused("改库区", register("SYN-WH-01", "SYN-LOC-01", 4, inZoneOf(t, "SYN-ZONE-02")),
		application.RegistrationRefusalImmutableField, domain.LocationFieldZoneCode)
	if rowsAfter := countLocationRows(t, stores); rowsAfter != rowsBefore {
		t.Fatalf("改库区被拒之后册里 %d 笔，want 仍是 %d", rowsAfter, rowsBefore)
	}

	// 停用的仓与库区下照常登记库位。
	stores.mustSaveWarehouse(t, "SYN-TENANT-01", "SYN-WH-01", 2, true)
	stores.mustSaveZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-D", true)
	expectOutcome("停用的仓与库区下登记库位", register("SYN-WH-01", "SYN-LOC-D", 1, inZoneOf(t, "SYN-ZONE-D")), application.RegistrationRegistered)
	if rows := countLocationRows(t, stores); rows != rowsBefore+1 {
		t.Fatalf("册里 %d 笔，want %d（被拒与重放都不落册）", rows, rowsBefore+1)
	}
}
