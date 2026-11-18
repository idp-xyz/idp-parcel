package postgres_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-001（仓的部分）持久化面——首笔落库；换登记责任方与登记时刻的原样重投答已在册，在册那一笔的登记责任方与登记时刻
// 不变；同一修订号换内容答冲突，不覆盖；修订 2 落册，旧修订照留，按（租户，仓编码，修订号）取得到任何一笔，取最新得修订 2。
func TestWarehouseRegistrySavesReplaysAndRefusesToOverwrite(t *testing.T) {
	warehouses, _, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")

	first := revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil)
	if outcome := saveRevision(t, transactor, warehouses, first); outcome != ports.WarehouseRevisionSaved {
		t.Fatalf("修订 1 = %s，want SAVED", outcome)
	}

	replay := registeredBy(t, first, "operator-2", baseInstant.Add(48*time.Hour))
	if outcome := saveRevision(t, transactor, warehouses, replay); outcome != ports.WarehouseRevisionAlreadyRegistered {
		t.Fatalf("换登记责任方与登记时刻的重放 = %s，want ALREADY_REGISTERED", outcome)
	}
	conflicting := revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（改名）")
	})
	if outcome := saveRevision(t, transactor, warehouses, conflicting); outcome != ports.WarehouseRevisionContentConflict {
		t.Fatalf("同修订号异内容 = %s，want CONTENT_CONFLICT", outcome)
	}

	loaded, found, err := warehouses.LoadWarehouseRevision(ctx, tenant, code, 1)
	if err != nil || !found {
		t.Fatalf("读回修订 1 = (%v, %v)", found, err)
	}
	if !loaded.SameContentAs(first) || loaded.Registrant().String() != "operator-1" || !loaded.RegisteredAt().Equal(first.RegisteredAt()) {
		t.Fatalf("重放与冲突之后在册的修订 1 变了：登记责任方 %q、登记时刻 %v、名称 %q",
			loaded.Registrant(), loaded.RegisteredAt(), loaded.Content().Name())
	}

	second := revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（迁址）")
	})
	if outcome := saveRevision(t, transactor, warehouses, second); outcome != ports.WarehouseRevisionSaved {
		t.Fatalf("修订 2 = %s，want SAVED", outcome)
	}
	latest, found, err := warehouses.LoadLatestWarehouse(ctx, tenant, code)
	if err != nil || !found || latest.Revision() != 2 || !latest.SameContentAs(second) {
		t.Fatalf("取最新 = (修订 %d, %v, %v)，want 修订 2", latest.Revision(), found, err)
	}
	if older, found, err := warehouses.LoadWarehouseRevision(ctx, tenant, code, 1); err != nil || !found || !older.SameContentAs(first) {
		t.Fatalf("修订 2 落册之后再取修订 1 = (%v, %v)，旧修订该照留", found, err)
	}
	if _, found, err := warehouses.LoadWarehouseRevision(ctx, tenant, code, 3); err != nil || found {
		t.Fatalf("取没登记的修订 3 = (%v, %v)，want (false, nil)", found, err)
	}
}

// Covers: AT-WF-001「仓编码逐字保存」与 AT-WF-005「节点标识逐字保存」——带首尾空白、内部连续空白、换行与大小写混写的仓编码、地址正文与
// 同址物流节点原样读回（点读、最新、修订史与列表四条路）；只差大小写或空白的仓编码是各自独立的仓，不互相当作重放或冲突。
// 领域构造器不拒首尾空白，只拒空白串与存不下的串，所以这里测的是原样读回，不是拒绝。
func TestWarehouseRegistryKeepsEveryByteOfVerbatimCellsAndKeepsCodesApart(t *testing.T) {
	warehouses, catalogue, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	const address = "  合成路  1 号\n二楼 "
	const node = "  SYN-Node-x  y\t"
	codes := []string{"  Wh-Mixed  01  ", "WH-1", "wh-1", " WH-1", "WH-1 ", "wh-1\t"}

	for _, code := range codes {
		mustSave(t, transactor, warehouses, revisionFixture(t, "SYN-TENANT-01", code, 1, func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(spec.Address.Country(), valueOf(t, domain.NewAddressText, address))
			spec.ColocatedNode = valueOf(t, domain.NewColocatedNode, node)
		}))
	}

	for _, raw := range codes {
		code := valueOf(t, domain.NewWarehouseCode, raw)
		latest, found, err := warehouses.LoadLatestWarehouse(ctx, tenant, code)
		if err != nil || !found {
			t.Fatalf("取最新 %q = (%v, %v)", raw, found, err)
		}
		if latest.Code().String() != raw || latest.Content().Address().Text().String() != address || latest.Content().ColocatedNode().String() != node {
			t.Fatalf("取最新 %q 走样：编码 %q、地址 %q、节点 %q", raw, latest.Code(), latest.Content().Address().Text(), latest.Content().ColocatedNode())
		}
		history, err := catalogue.ListWarehouseRevisions(ctx, tenant, code)
		if err != nil || len(history) != 1 || history[0].WarehouseCode != raw || history[0].AddressText != address || history[0].ColocatedNode != node {
			t.Fatalf("修订史 %q = (%+v, %v)，want 一笔、各格原样", raw, history, err)
		}
	}

	page, err := catalogue.ListWarehouses(ctx, tenant, 50, firstPage(t, nil))
	if err != nil || page.Total != int64(len(codes)) {
		t.Fatalf("列表 = (共 %d, %v)，want %d 个互相独立的仓", page.Total, err, len(codes))
	}
	listed := codesOf(page.Rows)
	for _, raw := range codes {
		if !slices.Contains(listed, raw) {
			t.Fatalf("列表里没有逐字的编码 %q：%q", raw, listed)
		}
	}
}

// Covers: 跨租户隔离（UC-WF-001「权限、隔离与审计」）——别的租户的同编码仓读不到：点读、取最新、列表与修订史都与「从没登记」同形；
// 同一仓编码与修订号在另一租户落册是新写入，不是重放也不是冲突，也不动这个租户已在册的那一笔。
func TestWarehouseRegistryIsTenantScoped(t *testing.T) {
	warehouses, catalogue, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	stranger := valueOf(t, domain.NewTenantID, "SYN-TENANT-03")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")

	mustSave(t, transactor, warehouses,
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, nil),
		revisionFixture(t, "SYN-TENANT-02", "SYN-WH-01", 1, func(spec *domain.WarehouseContentSpec) {
			spec.Name = valueOf(t, domain.NewWarehouseName, "另一租户的一号仓")
		}),
	)

	if latest, found, err := warehouses.LoadLatestWarehouse(ctx, other, code); err != nil || !found || latest.Revision() != 1 ||
		latest.Content().Name().String() != "另一租户的一号仓" {
		t.Fatalf("另一租户取最新 = (修订 %d, %v, %v)，want 它自己的修订 1", latest.Revision(), found, err)
	}
	if first, found, err := warehouses.LoadWarehouseRevision(ctx, own, code, 1); err != nil || !found || first.Content().Name().String() != "合成一号仓" {
		t.Fatalf("本租户的修订 1 被另一租户的写入动了：(%v, %v)", found, err)
	}

	if _, found, err := warehouses.LoadLatestWarehouse(ctx, stranger, code); err != nil || found {
		t.Fatalf("没登记过的租户取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := warehouses.LoadWarehouseRevision(ctx, stranger, code, 1); err != nil || found {
		t.Fatalf("没登记过的租户点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if page, err := catalogue.ListWarehouses(ctx, stranger, 50, firstPage(t, nil)); err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("没登记过的租户列表 = (%d 行, 共 %d, %v)，want 空", len(page.Rows), page.Total, err)
	}
	if history, err := catalogue.ListWarehouseRevisions(ctx, stranger, code); err != nil || len(history) != 0 {
		t.Fatalf("没登记过的租户读修订史 = (%+v, %v)，want 空", history, err)
	}
	if history, err := catalogue.ListWarehouseRevisions(ctx, other, code); err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatalf("另一租户的修订史 = (%+v, %v)，want 它自己的一笔", history, err)
	}
}

// Covers: AT-WF-006 与 AT-WF-005 的存取面——业务时区与同址物流节点缺就是没登记：库里存 NULL，读回仍是没登记，不取任何默认；
// 后补的修订带上之后，只那一笔带，旧修订照旧没有。
func TestWarehouseRegistryRoundTripsAbsentOptionalCellsAsNull(t *testing.T) {
	warehouses, catalogue, transactor, pool := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	bare := func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = domain.BusinessTimeZone{}
		spec.ColocatedNode = domain.ColocatedNode{}
	}
	mustSave(t, transactor, warehouses,
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, bare),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, nil),
	)

	var nullZone, nullNode bool
	if err := pool.QueryRow(ctx,
		`SELECT business_time_zone IS NULL, colocated_node IS NULL
		   FROM warehouse_fulfillment.warehouse_revision
		  WHERE tenant_id = 'SYN-TENANT-01' AND warehouse_code = 'SYN-WH-01' AND revision = 1`).Scan(&nullZone, &nullNode); err != nil {
		t.Fatalf("读修订 1 的可缺两格：%v", err)
	}
	if !nullZone || !nullNode {
		t.Fatalf("修订 1 没给的业务时区与同址物流节点该存 NULL，实得 NULL? %v、%v", nullZone, nullNode)
	}

	first, found, err := warehouses.LoadWarehouseRevision(ctx, tenant, code, 1)
	if err != nil || !found || first.Content().BusinessTimeZone().Registered() || first.Content().ColocatedNode().Named() {
		t.Fatalf("读回修订 1 = (%v, %v)，业务时区与同址物流节点该仍是没登记", found, err)
	}
	history, err := catalogue.ListWarehouseRevisions(ctx, tenant, code)
	if err != nil || len(history) != 2 {
		t.Fatalf("修订史 = (%+v, %v)", history, err)
	}
	if history[0].HasBusinessTimeZone || history[0].HasColocatedNode || history[0].BusinessTimeZone != "" || history[0].ColocatedNode != "" {
		t.Fatalf("修订史第 1 笔带了没登记的格：%+v", history[0])
	}
	if !history[1].HasBusinessTimeZone || history[1].BusinessTimeZone != "Asia/Shanghai" || !history[1].HasColocatedNode || history[1].ColocatedNode != "SYN-NODE-01" {
		t.Fatalf("修订史第 2 笔该带业务时区与同址物流节点：%+v", history[1])
	}
}

// Covers: AT-WF-009（仓的部分）——一个仓都没登记的册：取最新、点读都答没有，不报错；迁移不种任何行。
func TestWarehouseRegistryOfAnEmptyRegisterAnswersNothingAndSeedsNoRows(t *testing.T) {
	warehouses, _, _, pool := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")

	if _, found, err := warehouses.LoadLatestWarehouse(ctx, tenant, code); err != nil || found {
		t.Fatalf("空册取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := warehouses.LoadWarehouseRevision(ctx, tenant, code, 1); err != nil || found {
		t.Fatalf("空册点读 = (%v, %v)，want (false, nil)", found, err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM warehouse_fulfillment.warehouse_revision`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("迁移之后表里有 %d 行（%v），want 不带任何默认行", rows, err)
	}
}

// 读口的入参没经构造器（零值）是装配错了，答 error，不折成「没有」：折成没有会让登记方从修订 1 重登一个本来就在册的仓。
func TestWarehouseRegistryRefusesUnconstructedKeysOnRead(t *testing.T) {
	warehouses, _, _, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")

	if _, _, err := warehouses.LoadLatestWarehouse(ctx, domain.TenantID{}, code); err == nil {
		t.Errorf("缺租户的取最新应报错")
	}
	if _, _, err := warehouses.LoadLatestWarehouse(ctx, tenant, domain.WarehouseCode{}); err == nil {
		t.Errorf("缺仓编码的取最新应报错")
	}
	if _, _, err := warehouses.LoadWarehouseRevision(ctx, domain.TenantID{}, code, 1); err == nil {
		t.Errorf("缺租户的点读应报错")
	}
	if _, _, err := warehouses.LoadWarehouseRevision(ctx, tenant, domain.WarehouseCode{}, 1); err == nil {
		t.Errorf("缺仓编码的点读应报错")
	}
}

// 写口在没有环境事务时拒绝（PBC-08）：登记的落册与它的读必须在同一个事务里，无事务就走自动提交会让「查过没有再写」失去意义。
func TestWarehouseWritesRefuseToRunOutsideATransaction(t *testing.T) {
	warehouses, _, _, _ := newWarehouseStores(t)
	if _, err := warehouses.SaveWarehouseRevision(t.Context(), revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveWarehouseRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 库上的 CHECK 与主键各守一格：封闭的两种仓、修订号为正、非空白、可缺的两格不兼作空串、同一修订号至多一笔。绕过领域构造器直接插行，
// 逐一证库上真拒（不是只靠领域那一层）。
func TestWarehouseRevisionTableGuardsItsClosedSetsAndKeys(t *testing.T) {
	_, _, _, pool := newWarehouseStores(t)
	ctx := t.Context()

	type row struct {
		tenant, code string
		revision     int
		name, kind   string
		country      string
		address      string
		zone, node   *string
		registrant   string
	}
	good := row{tenant: "SYN-TENANT-01", code: "SYN-WH-01", revision: 1, name: "合成一号仓", kind: "SELF_OPERATED",
		country: "CN", address: "合成路 1 号", registrant: "operator-1"}
	blank, space := "", "  "
	insert := func(candidate row) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.warehouse_revision
				(tenant_id, warehouse_code, revision, warehouse_name, kind, country_code, address_text,
				 business_time_zone, colocated_node, deactivated, registrant, registered_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, $10, now())`,
			candidate.tenant, candidate.code, candidate.revision, candidate.name, candidate.kind, candidate.country,
			candidate.address, candidate.zone, candidate.node, candidate.registrant)
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
		"集外的仓种类":     func(r *row) { r.kind = "OWNED" },
		"小写的仓种类":     func(r *row) { r.kind = "agent" },
		"修订号为 0":     func(r *row) { r.revision = 0 },
		"仓编码全空格":     func(r *row) { r.code = "  " },
		"仓名称为空":      func(r *row) { r.name = "" },
		"地址正文全空格":    func(r *row) { r.address = "   " },
		"国家码为空":      func(r *row) { r.country = "" },
		"登记责任方为空":    func(r *row) { r.registrant = "" },
		"业务时区给了空串":   func(r *row) { r.zone = &blank },
		"业务时区全空格":    func(r *row) { r.zone = &space },
		"同址物流节点给了空串": func(r *row) { r.node = &blank },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "23514" {
			t.Errorf("%s：插入应被 CHECK 拒（23514），实得 %v", name, err)
		}
	}

	if err := insert(good); err != nil {
		t.Fatalf("合法行应能插入：%v", err)
	}
	if err := insert(good); violation(err) != "23505" {
		t.Errorf("同一（租户，仓编码，修订号）插两次应被主键拒（23505），实得 %v", err)
	}
	elsewhere := good
	elsewhere.tenant = "SYN-TENANT-02"
	if err := insert(elsewhere); err != nil {
		t.Errorf("另一租户的同编码同修订号不该被拒：%v", err)
	}
}
