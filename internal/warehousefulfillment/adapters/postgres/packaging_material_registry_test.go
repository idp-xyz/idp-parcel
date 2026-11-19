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

// Covers: AT-WF-001（包材的部分）持久化面——首笔落库；换登记责任方与登记时刻的原样重投答已在册，在册那一笔的登记责任方与登记时刻不变；同一修订号换
// 内容答冲突，不覆盖；修订 2 落册，旧修订照留，按（租户，包材编码，修订号）取得到任何一笔，取最新得修订 2。
func TestPackagingMaterialRegistrySavesReplaysAndRefusesToOverwrite(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")

	first := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, nil)
	if outcome := savePackagingMaterial(t, stores, first); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 1 = %s，want SAVED", outcome)
	}

	replay := packagingMaterialRegisteredBy(t, first, "operator-2", baseInstant.Add(48*time.Hour))
	if outcome := savePackagingMaterial(t, stores, replay); outcome != ports.RegistryRevisionAlreadyRegistered {
		t.Fatalf("换登记责任方与登记时刻的重放 = %s，want ALREADY_REGISTERED", outcome)
	}
	for name, edit := range map[string]func(*domain.PackagingMaterialContentSpec){
		"名称": func(spec *domain.PackagingMaterialContentSpec) {
			spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱（改名）")
		},
		"长":  func(spec *domain.PackagingMaterialContentSpec) { spec.Length = measureValue(t, "30.000001") },
		"重量": func(spec *domain.PackagingMaterialContentSpec) { spec.Weight = measureValue(t, "0.26") },
		"尺寸单位": func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "M")
		},
		"重量单位": func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = valueOf(t, domain.NewUnitCode, "G") },
		"是否停用": func(spec *domain.PackagingMaterialContentSpec) { spec.Deactivated = true },
		"尺寸单位大小": func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "cm")
		},
	} {
		conflicting := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, edit)
		if outcome := savePackagingMaterial(t, stores, conflicting); outcome != ports.RegistryRevisionContentConflict {
			t.Fatalf("同修订号只差%s = %s，want CONTENT_CONFLICT", name, outcome)
		}
	}

	loaded, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, code, 1)
	if err != nil || !found {
		t.Fatalf("读回修订 1 = (%v, %v)", found, err)
	}
	if !loaded.SameContentAs(first) || loaded.Registrant().String() != "operator-1" || !loaded.RegisteredAt().Equal(first.RegisteredAt()) {
		t.Fatalf("重放与冲突之后在册的修订 1 变了：登记责任方 %q、登记时刻 %v、名称 %q",
			loaded.Registrant(), loaded.RegisteredAt(), loaded.Content().Name())
	}

	second := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 2, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱（二）")
		spec.Deactivated = true
	})
	if outcome := savePackagingMaterial(t, stores, second); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 2 = %s，want SAVED", outcome)
	}
	latest, found, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, tenant, code)
	if err != nil || !found || latest.Revision() != 2 || !latest.SameContentAs(second) || !latest.Content().Deactivated() {
		t.Fatalf("取最新 = (修订 %d, %v, %v)，want 修订 2、已停用", latest.Revision(), found, err)
	}
	if older, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, code, 1); err != nil || !found || !older.SameContentAs(first) {
		t.Fatalf("修订 2 落册之后再取修订 1 = (%v, %v)，旧修订该照留", found, err)
	}
	if _, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, code, 3); err != nil || found {
		t.Fatalf("取没登记的修订 3 = (%v, %v)，want (false, nil)", found, err)
	}
}

// Covers: AT-WF-004（包材的部分）「库里按精确十进制存（numeric，不用浮点）」「数值按数值比，不按写法比」——数值列是 numeric(15, 6)，不是浮点；
// 落库再读回的数值与登记时的数值逐位相同（0.1、最小的 0.000001、最大的 999999999.999999 都不走样）；换一种数值写法的原样重投答已在册，不是冲突；
// 读回的是去掉多余零的规范写法。
func TestPackagingMaterialNumbersAreStoredAsExactDecimalsAndComparedByValue(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")

	var types int
	if err := stores.pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_schema = 'warehouse_fulfillment' AND table_name = 'packaging_material_revision'
		    AND column_name IN ('length_value', 'width_value', 'height_value', 'weight_value')
		    AND data_type = 'numeric' AND numeric_precision = 15 AND numeric_scale = 6`).Scan(&types); err != nil || types != 4 {
		t.Fatalf("数值列的类型 = (%d 列是 numeric(15, 6), %v)，want 四列都是", types, err)
	}

	extremes := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureValue(t, "999999999.999999"), measureValue(t, "0.000001"), measureValue(t, "0.1")
		spec.Weight = measureValue(t, "123456789.123456")
	})
	mustSavePackagingMaterials(t, stores, extremes)
	loaded, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, code, 1)
	if err != nil || !found || !loaded.SameContentAs(extremes) {
		t.Fatalf("读回 = (%v, %v)，want 与登记时逐位相同", found, err)
	}
	if got := []string{loaded.Content().Length().String(), loaded.Content().Width().String(), loaded.Content().Height().String(), loaded.Content().Weight().String()}; !slices.Equal(
		got, []string{"999999999.999999", "0.000001", "0.1", "123456789.123456"}) {
		t.Fatalf("读回的数值 = %v", got)
	}
	var rawLength, rawHeight string
	if err := stores.pool.QueryRow(ctx,
		`SELECT length_value::text, height_value::text FROM warehouse_fulfillment.packaging_material_revision`).Scan(&rawLength, &rawHeight); err != nil ||
		rawLength != "999999999.999999" || rawHeight != "0.100000" {
		t.Fatalf("库里的数值 = (%q, %q, %v)，want 精确十进制 999999999.999999 与 0.100000", rawLength, rawHeight, err)
	}

	// 另一个包材：换一种写法的原样重投答已在册，不是冲突；写法不同的同一组数落同一行。
	base := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-02", 1, nil)
	mustSavePackagingMaterials(t, stores, base)
	rewritten := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-02", 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureValue(t, "030.00"), measureValue(t, "20.000000"), measureValue(t, "015.500")
		spec.Weight = measureValue(t, "00.2500")
	})
	if outcome := savePackagingMaterial(t, stores, rewritten); outcome != ports.RegistryRevisionAlreadyRegistered {
		t.Fatalf("换一种数值写法的原样重投 = %s，want ALREADY_REGISTERED", outcome)
	}
	if rows := countPackagingMaterialRows(t, stores); rows != 2 {
		t.Fatalf("册里 %d 笔，want 2（换写法的重放不落册）", rows)
	}
	history, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, tenant, valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-02"))
	if err != nil || len(history) != 1 || history[0].Length != "30" || history[0].Width != "20" || history[0].Height != "15.5" || history[0].Weight != "0.25" {
		t.Fatalf("修订史 = (%+v, %v)，want 数值是去掉多余零的 30、20、15.5、0.25", history, err)
	}
}

// Covers: AT-WF-001（包材的部分）「编码逐字保存」——带首尾空白、内部连续空白、换行与大小写混写的编码、名称与单位码原样读回（点读、最新、修订史与列表
// 四条路）；只差大小写或空白的编码是各自独立的包材，不互相当作重放或冲突。
func TestPackagingMaterialRegistryKeepsEveryByteOfVerbatimCellsAndKeepsCodesApart(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	const name = "  合成  纸箱\n二号 "
	codes := []string{"  Box-Mixed  01  ", "B-1", "b-1", " B-1", "B-1 ", "b-1\t"}

	for _, code := range codes {
		mustSavePackagingMaterials(t, stores, packagingMaterialFixture(t, "SYN-TENANT-01", code, 1, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Name = valueOf(t, domain.NewPackagingMaterialName, name)
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, " cm\t") // 单位码也逐字落库：目录里有没有它是应用层的事，库不替它改
		}))
	}

	for _, raw := range codes {
		code := valueOf(t, domain.NewPackagingMaterialCode, raw)
		latest, found, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, tenant, code)
		if err != nil || !found {
			t.Fatalf("取最新 %q = (%v, %v)", raw, found, err)
		}
		if latest.Code().String() != raw || latest.Content().Name().String() != name || latest.Content().DimensionsUnit().String() != " cm\t" {
			t.Fatalf("取最新 %q 走样：编码 %q、名称 %q、单位 %q", raw, latest.Code(), latest.Content().Name(), latest.Content().DimensionsUnit())
		}
		history, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, tenant, code)
		if err != nil || len(history) != 1 || history[0].PackagingMaterialCode != raw || history[0].Name != name || history[0].DimensionsUnit != " cm\t" {
			t.Fatalf("修订史 %q = (%+v, %v)，want 一笔、各格原样", raw, history, err)
		}
	}

	page, err := stores.catalogue.ListPackagingMaterials(ctx, tenant, 50, packagingMaterialFirstPage(t, nil))
	if err != nil || page.Total != int64(len(codes)) {
		t.Fatalf("列表 = (共 %d, %v)，want %d 个互相独立的包材", page.Total, err, len(codes))
	}
	listed := packagingMaterialCodesOf(page.Rows)
	for _, raw := range codes {
		if !slices.Contains(listed, raw) {
			t.Fatalf("列表里没有逐字的包材 %q：%q", raw, listed)
		}
	}
}

// Covers: 跨租户隔离（UC-WF-001「权限、隔离与审计」）——别的租户的同编码包材读不到：点读、取最新、列表与修订史都与「从没登记」同形；同一编码与修订号在
// 另一租户落册是新写入，不是重放也不是冲突，也不动这个租户已在册的那一笔。
func TestPackagingMaterialRegistryIsTenantScoped(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	stranger := valueOf(t, domain.NewTenantID, "SYN-TENANT-03")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")

	mustSavePackagingMaterials(t, stores,
		packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, nil),
		packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 2, nil),
		packagingMaterialFixture(t, "SYN-TENANT-02", "SYN-BOX-01", 1, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Name = valueOf(t, domain.NewPackagingMaterialName, "另一租户的纸箱")
		}),
	)

	if latest, found, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, other, code); err != nil || !found || latest.Revision() != 1 ||
		latest.Content().Name().String() != "另一租户的纸箱" {
		t.Fatalf("另一租户取最新 = (修订 %d, %v, %v)，want 它自己的修订 1", latest.Revision(), found, err)
	}
	if first, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, own, code, 1); err != nil || !found || first.Content().Name().String() != "合成一号纸箱" {
		t.Fatalf("本租户的修订 1 被另一租户的写入动了：(%v, %v)", found, err)
	}

	if _, found, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, stranger, code); err != nil || found {
		t.Fatalf("没登记过的租户取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, stranger, code, 1); err != nil || found {
		t.Fatalf("没登记过的租户点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if page, err := stores.catalogue.ListPackagingMaterials(ctx, stranger, 50, packagingMaterialFirstPage(t, nil)); err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("没登记过的租户列表 = (%d 行, 共 %d, %v)，want 空", len(page.Rows), page.Total, err)
	}
	if history, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, stranger, code); err != nil || history == nil || len(history) != 0 {
		t.Fatalf("没登记过的租户读修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	if history, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, other, code); err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatalf("另一租户的修订史 = (%+v, %v)，want 它自己的一笔", history, err)
	}
}

// Covers: AT-WF-009（包材的部分）——一个包材都没登记的册：取最新、点读都答没有，不报错；迁移不种任何行。
func TestPackagingMaterialRegistryOfAnEmptyRegisterAnswersNothingAndSeedsNoRows(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")

	if _, found, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, tenant, code); err != nil || found {
		t.Fatalf("空册取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, code, 1); err != nil || found {
		t.Fatalf("空册点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if rows := countPackagingMaterialRows(t, stores); rows != 0 {
		t.Fatalf("迁移之后表里有 %d 行，want 不带任何默认行", rows)
	}
}

// 读口的入参没经构造器（零值）是装配错了，答 error，不折成「没有」：折成没有会让登记方从修订 1 重登一个本来就在册的包材。
func TestPackagingMaterialRegistryRefusesUnconstructedKeysOnRead(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")

	for name, call := range map[string]func() error{
		"缺租户的取最新": func() error {
			_, _, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, domain.TenantID{}, code)
			return err
		},
		"缺包材编码的取最新": func() error {
			_, _, err := stores.packagingMaterials.LoadLatestPackagingMaterial(ctx, tenant, domain.PackagingMaterialCode{})
			return err
		},
		"缺租户的点读": func() error {
			_, _, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, domain.TenantID{}, code, 1)
			return err
		},
		"缺包材编码的点读": func() error {
			_, _, err := stores.packagingMaterials.LoadPackagingMaterialRevision(ctx, tenant, domain.PackagingMaterialCode{}, 1)
			return err
		},
		"缺租户的修订史": func() error {
			_, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, domain.TenantID{}, code)
			return err
		},
		"缺包材编码的修订史": func() error {
			_, err := stores.catalogue.ListPackagingMaterialRevisions(ctx, tenant, domain.PackagingMaterialCode{})
			return err
		},
		"缺租户的列表": func() error {
			_, err := stores.catalogue.ListPackagingMaterials(ctx, domain.TenantID{}, 50, packagingMaterialFirstPage(t, nil))
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s应报错", name)
		}
	}
}

// 写口在没有环境事务时拒绝（PBC-08）：登记的落册与它的读必须在同一个事务里，无事务就走自动提交会让「查过没有再写」失去意义。
func TestPackagingMaterialWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	if _, err := stores.packagingMaterials.SavePackagingMaterialRevision(t.Context(), packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, nil)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SavePackagingMaterialRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 库上的 CHECK 与主键各守一格：数值列为正（零与负数写不进，四列各验）、修订号为正、非空白（含两个单位）、同一修订号至多一笔，数值超出 numeric(15, 6)
// 装得下的范围也写不进。绕过领域构造器与应用层的登记门直接插行，逐一证库上真拒（不是只靠上面两层）。包材表不对仓表建外键，也不镜像计量单位目录：
// 所以库上插得进一个没登记过仓的租户的包材行、也插得进目录外的单位码——前者归应用层答未配置，后者归应用层的目录门，这是有意的。
func TestPackagingMaterialRevisionTableGuardsPositivityBlanksAndKeys(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()

	type row struct {
		tenant, code                  string
		revision                      int
		name                          string
		length, width, height, weight string
		dimensionsUnit, weightUnit    string
		registrant                    string
	}
	good := row{
		tenant: "SYN-TENANT-01", code: "SYN-BOX-01", revision: 1, name: "合成一号纸箱",
		length: "30", width: "20", height: "15.5", weight: "0.25", dimensionsUnit: "CM", weightUnit: "KG", registrant: "operator-1",
	}
	insert := func(candidate row) error {
		_, err := stores.pool.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.packaging_material_revision
				(tenant_id, packaging_material_code, revision, packaging_material_name,
				 length_value, width_value, height_value, dimensions_unit, weight_value, weight_unit,
				 deactivated, registrant, registered_at)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7::numeric, $8, $9::numeric, $10, false, $11, now())`,
			candidate.tenant, candidate.code, candidate.revision, candidate.name,
			candidate.length, candidate.width, candidate.height, candidate.dimensionsUnit, candidate.weight, candidate.weightUnit,
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
		"长为零":       func(r *row) { r.length = "0" },
		"宽为零":       func(r *row) { r.width = "0.000000" },
		"高为零":       func(r *row) { r.height = "0.0" },
		"重量为零":      func(r *row) { r.weight = "0" },
		"长为负":       func(r *row) { r.length = "-1" },
		"宽为负":       func(r *row) { r.width = "-0.000001" },
		"高为负":       func(r *row) { r.height = "-15.5" },
		"重量为负":      func(r *row) { r.weight = "-0.25" },
		"小数部分舍入后为零": func(r *row) { r.length = "0.0000004" },
		"修订号为 0":    func(r *row) { r.revision = 0 },
		"租户全空格":     func(r *row) { r.tenant = "  " },
		"包材编码全空格":   func(r *row) { r.code = "   " },
		"包材名称为空":    func(r *row) { r.name = "" },
		"尺寸单位为空":    func(r *row) { r.dimensionsUnit = "" },
		"尺寸单位全空格":   func(r *row) { r.dimensionsUnit = "  " },
		"重量单位为空":    func(r *row) { r.weightUnit = "" },
		"重量单位全空格":   func(r *row) { r.weightUnit = " " },
		"登记责任方为空":   func(r *row) { r.registrant = "" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "23514" {
			t.Errorf("%s：插入应被 CHECK 拒（23514），实得 %v", name, err)
		}
	}
	for name, mutate := range map[string]func(*row){
		"长超出整数部分 9 位": func(r *row) { r.length = "1000000000" },
		"重量超出整数部分":    func(r *row) { r.weight = "1234567890.5" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "22003" {
			t.Errorf("%s：插入应被 numeric 的范围拒（22003），实得 %v", name, err)
		}
	}

	if err := insert(good); err != nil {
		t.Fatalf("合法行应能插入（仓册里没有任何仓也插得进：不建外键）：%v", err)
	}
	if err := insert(good); violation(err) != "23505" {
		t.Errorf("同一（租户，包材编码，修订号）插两次应被主键拒（23505），实得 %v", err)
	}
	for name, mutate := range map[string]func(*row){
		"另一租户":       func(r *row) { r.tenant = "SYN-TENANT-02" },
		"另一个包材编码":    func(r *row) { r.code = "SYN-BOX-02" },
		"同一包材的修订 2":  func(r *row) { r.revision = 2 },
		"只差大小写的包材编码": func(r *row) { r.code = "syn-box-01" },
		"目录外的单位码":    func(r *row) { r.code, r.dimensionsUnit = "SYN-BOX-03", "FURLONG" },
		"最小的正数":      func(r *row) { r.code, r.length = "SYN-BOX-04", "0.000001" },
		"装得下的最大数":    func(r *row) { r.code, r.weight = "SYN-BOX-05", "999999999.999999" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); err != nil {
			t.Errorf("%s不该被拒：%v", name, err)
		}
	}
}

// Covers: AT-WF-001、AT-WF-004、AT-WF-009（包材的部分）端到端——登记用例接真的仓册与包材册：一个仓都没登记时答 NOT_CONFIGURED，不落册（哪怕内容不合）；
// 别的租户登记的仓不算本租户的仓；登记（停用的）仓之后同一修订原样重交落册，再原样重投答已在册，换一种数值写法的原样重投仍答已在册；数值为零与坏单位
// 答 CONTENT_INVALID 并指出那一格，不落册；跳号答错位。
func TestRegisterPackagingMaterialOnTheRealRegistersAnswersNotConfiguredUntilTheTenantHasAWarehouse(t *testing.T) {
	stores := newPackagingMaterialStores(t)
	ctx := t.Context()
	handler := application.NewRegisterPackagingMaterialHandler(stores.packagingMaterials, stores.warehouses, fixedClock{at: baseInstant})

	register := func(tenant string, revision int, edit func(*domain.PackagingMaterialContentSpec)) application.RegisterPackagingMaterialResult {
		t.Helper()
		command := application.RegisterPackagingMaterialCommand{
			Tenant:     valueOf(t, domain.NewTenantID, tenant),
			Code:       valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
			Revision:   revision,
			Content:    packagingMaterialContentWith(t, edit),
			Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
		}
		var result application.RegisterPackagingMaterialResult
		err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			result, registerErr = handler.Register(txCtx, command)
			return registerErr
		})
		if err != nil {
			t.Fatalf("登记包材：%v", err)
		}
		return result
	}
	zeroLength := func(spec *domain.PackagingMaterialContentSpec) { spec.Length = measureValue(t, "0") }

	if result := register("SYN-TENANT-01", 1, nil); result.Outcome() != application.RegistrationNotConfigured {
		t.Fatalf("一个仓都没登记 = %s，want NOT_CONFIGURED", result.Outcome())
	}
	if result := register("SYN-TENANT-01", 1, zeroLength); result.Outcome() != application.RegistrationNotConfigured {
		t.Fatalf("一个仓都没登记且内容不合 = %s，want 仍是 NOT_CONFIGURED", result.Outcome())
	}
	if rows := countPackagingMaterialRows(t, stores); rows != 0 {
		t.Fatalf("未配置之后册里有 %d 笔，want 不落册", rows)
	}

	// 别的租户登记了仓：不算本租户的仓。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-02", "SYN-WH-01", 1, nil))
	if result := register("SYN-TENANT-01", 1, nil); result.Outcome() != application.RegistrationNotConfigured {
		t.Fatalf("只有别的租户登记了仓 = %s，want NOT_CONFIGURED", result.Outcome())
	}

	// 本租户的仓只有一笔、而且是停用的：停用的仓也算登记过。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	for name, edit := range map[string]func(*domain.PackagingMaterialContentSpec){
		"长为零": zeroLength,
		"尺寸单位坏了": func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "KG")
		},
		"重量单位坏了": func(spec *domain.PackagingMaterialContentSpec) {
			spec.WeightUnit = valueOf(t, domain.NewUnitCode, "CM")
		},
		"重量为零": func(spec *domain.PackagingMaterialContentSpec) { spec.Weight = measureValue(t, "0.0") },
		"单位不在目录里": func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "cm")
		},
	} {
		if refused := register("SYN-TENANT-01", 1, edit); refused.Outcome() != application.RegistrationNotAccepted ||
			refused.Refusal() != application.RegistrationRefusalContentInvalid || refused.Field().String() == "" {
			t.Fatalf("%s = %s/%s 指向 %q，want NOT_ACCEPTED/CONTENT_INVALID 并指出那一格", name, refused.Outcome(), refused.Refusal(), refused.Field())
		}
	}
	if rows := countPackagingMaterialRows(t, stores); rows != 0 {
		t.Fatalf("内容不合之后册里有 %d 笔，want 不落册", rows)
	}

	if landed := register("SYN-TENANT-01", 1, nil); landed.Outcome() != application.RegistrationRegistered {
		t.Fatalf("登记（停用的）仓之后原样重交 = %s，want REGISTERED", landed.Outcome())
	}
	if replay := register("SYN-TENANT-01", 1, nil); replay.Outcome() != application.RegistrationAlreadyRegistered {
		t.Fatalf("再原样重交 = %s，want ALREADY_REGISTERED", replay.Outcome())
	}
	rewritten := register("SYN-TENANT-01", 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureValue(t, "30.000"), measureValue(t, "020"), measureValue(t, "15.50")
		spec.Weight = measureValue(t, "0.2500")
	})
	if rewritten.Outcome() != application.RegistrationAlreadyRegistered {
		t.Fatalf("换一种数值写法的原样重投 = %s，want ALREADY_REGISTERED", rewritten.Outcome())
	}
	if echoed, has := rewritten.Revision(); !has || echoed.Content().Height().String() != "15.5" {
		t.Fatalf("重放回显的高 = %q（%v），want 在册的 15.5", echoed.Content().Height(), has)
	}
	if skipped := register("SYN-TENANT-01", 3, nil); skipped.Outcome() != application.RegistrationNotAccepted ||
		skipped.Refusal() != application.RegistrationRefusalRevisionOutOfSequence || skipped.LatestRevision() != 1 {
		t.Fatalf("跳号 = %s/%s 回显 %d，want NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 回显 1", skipped.Outcome(), skipped.Refusal(), skipped.LatestRevision())
	}
	if rows := countPackagingMaterialRows(t, stores); rows != 1 {
		t.Fatalf("册里 %d 笔，want 1（未配置、内容不合、重放与错位都不落册）", rows)
	}
}
