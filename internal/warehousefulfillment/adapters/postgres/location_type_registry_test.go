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

// Covers: AT-WF-001（库位类型的部分）持久化面——首笔落库；换登记责任方与登记时刻的原样重投答已在册，在册那一笔的登记责任方与登记时刻
// 不变；同一修订号换内容答冲突，不覆盖；修订 2 落册，旧修订照留，按（租户，库位类型编码，修订号）取得到任何一笔，取最新得修订 2。
func TestLocationTypeRegistrySavesReplaysAndRefusesToOverwrite(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")

	first := locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, nil)
	if outcome := saveLocationType(t, stores, first); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 1 = %s，want SAVED", outcome)
	}

	replay := locationTypeRegisteredBy(t, first, "operator-2", baseInstant.Add(48*time.Hour))
	if outcome := saveLocationType(t, stores, replay); outcome != ports.RegistryRevisionAlreadyRegistered {
		t.Fatalf("换登记责任方与登记时刻的重放 = %s，want ALREADY_REGISTERED", outcome)
	}
	conflicting := locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（改名）")
	})
	if outcome := saveLocationType(t, stores, conflicting); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同修订号异内容 = %s，want CONTENT_CONFLICT", outcome)
	}
	deactivated := locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Deactivated = true
	})
	if outcome := saveLocationType(t, stores, deactivated); outcome != ports.RegistryRevisionContentConflict {
		t.Fatalf("同修订号只差停用标记 = %s，want CONTENT_CONFLICT", outcome)
	}

	loaded, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, tenant, code, 1)
	if err != nil || !found {
		t.Fatalf("读回修订 1 = (%v, %v)", found, err)
	}
	if !loaded.SameContentAs(first) || loaded.Registrant().String() != "operator-1" || !loaded.RegisteredAt().Equal(first.RegisteredAt()) {
		t.Fatalf("重放与冲突之后在册的修订 1 变了：登记责任方 %q、登记时刻 %v、名称 %q",
			loaded.Registrant(), loaded.RegisteredAt(), loaded.Content().Name())
	}

	second := locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 2, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（二）")
		spec.Deactivated = true
	})
	if outcome := saveLocationType(t, stores, second); outcome != ports.RegistryRevisionSaved {
		t.Fatalf("修订 2 = %s，want SAVED", outcome)
	}
	latest, found, err := stores.locationTypes.LoadLatestLocationType(ctx, tenant, code)
	if err != nil || !found || latest.Revision() != 2 || !latest.SameContentAs(second) || !latest.Content().Deactivated() {
		t.Fatalf("取最新 = (修订 %d, %v, %v)，want 修订 2、已停用", latest.Revision(), found, err)
	}
	if older, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, tenant, code, 1); err != nil || !found || !older.SameContentAs(first) {
		t.Fatalf("修订 2 落册之后再取修订 1 = (%v, %v)，旧修订该照留", found, err)
	}
	if _, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, tenant, code, 3); err != nil || found {
		t.Fatalf("取没登记的修订 3 = (%v, %v)，want (false, nil)", found, err)
	}
}

// Covers: AT-WF-001（库位类型的部分）「编码逐字保存」——带首尾空白、内部连续空白、换行与大小写混写的编码与名称原样读回（点读、最新、修订史
// 与列表四条路）；只差大小写或空白的编码是各自独立的库位类型，不互相当作重放或冲突。
func TestLocationTypeRegistryKeepsEveryByteOfVerbatimCellsAndKeepsCodesApart(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	const name = "  合成  货架位\n二楼 "
	codes := []string{"  Rack-Mixed  01  ", "R-1", "r-1", " R-1", "R-1 ", "r-1\t"}

	for _, code := range codes {
		mustSaveLocationTypes(t, stores, locationTypeFixture(t, "SYN-TENANT-01", code, 1, func(spec *domain.LocationTypeContentSpec) {
			spec.Name = valueOf(t, domain.NewLocationTypeName, name)
		}))
	}

	for _, raw := range codes {
		code := valueOf(t, domain.NewLocationTypeCode, raw)
		latest, found, err := stores.locationTypes.LoadLatestLocationType(ctx, tenant, code)
		if err != nil || !found {
			t.Fatalf("取最新 %q = (%v, %v)", raw, found, err)
		}
		if latest.Code().String() != raw || latest.Content().Name().String() != name {
			t.Fatalf("取最新 %q 走样：编码 %q、名称 %q", raw, latest.Code(), latest.Content().Name())
		}
		history, err := stores.catalogue.ListLocationTypeRevisions(ctx, tenant, code)
		if err != nil || len(history) != 1 || history[0].LocationTypeCode != raw || history[0].Name != name {
			t.Fatalf("修订史 %q = (%+v, %v)，want 一笔、各格原样", raw, history, err)
		}
	}

	page, err := stores.catalogue.ListLocationTypes(ctx, tenant, 50, locationTypeFirstPage(t, nil))
	if err != nil || page.Total != int64(len(codes)) {
		t.Fatalf("列表 = (共 %d, %v)，want %d 个互相独立的库位类型", page.Total, err, len(codes))
	}
	listed := locationTypeCodesOf(page.Rows)
	for _, raw := range codes {
		if !slices.Contains(listed, raw) {
			t.Fatalf("列表里没有逐字的库位类型 %q：%q", raw, listed)
		}
	}
}

// Covers: 跨租户隔离（UC-WF-001「权限、隔离与审计」）——别的租户的同编码库位类型读不到：点读、取最新、列表与修订史都与「从没登记」同形；
// 同一编码与修订号在另一租户落册是新写入，不是重放也不是冲突，也不动这个租户已在册的那一笔。
func TestLocationTypeRegistryIsTenantScoped(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	stranger := valueOf(t, domain.NewTenantID, "SYN-TENANT-03")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")

	mustSaveLocationTypes(t, stores,
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, nil),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 2, nil),
		locationTypeFixture(t, "SYN-TENANT-02", "SYN-LT-01", 1, func(spec *domain.LocationTypeContentSpec) {
			spec.Name = valueOf(t, domain.NewLocationTypeName, "另一租户的货架位")
		}),
	)

	if latest, found, err := stores.locationTypes.LoadLatestLocationType(ctx, other, code); err != nil || !found || latest.Revision() != 1 ||
		latest.Content().Name().String() != "另一租户的货架位" {
		t.Fatalf("另一租户取最新 = (修订 %d, %v, %v)，want 它自己的修订 1", latest.Revision(), found, err)
	}
	if first, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, own, code, 1); err != nil || !found || first.Content().Name().String() != "合成货架位" {
		t.Fatalf("本租户的修订 1 被另一租户的写入动了：(%v, %v)", found, err)
	}

	if _, found, err := stores.locationTypes.LoadLatestLocationType(ctx, stranger, code); err != nil || found {
		t.Fatalf("没登记过的租户取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, stranger, code, 1); err != nil || found {
		t.Fatalf("没登记过的租户点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if page, err := stores.catalogue.ListLocationTypes(ctx, stranger, 50, locationTypeFirstPage(t, nil)); err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("没登记过的租户列表 = (%d 行, 共 %d, %v)，want 空", len(page.Rows), page.Total, err)
	}
	if history, err := stores.catalogue.ListLocationTypeRevisions(ctx, stranger, code); err != nil || history == nil || len(history) != 0 {
		t.Fatalf("没登记过的租户读修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	if history, err := stores.catalogue.ListLocationTypeRevisions(ctx, other, code); err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatalf("另一租户的修订史 = (%+v, %v)，want 它自己的一笔", history, err)
	}
}

// Covers: AT-WF-009（库位类型的部分）——一个库位类型都没登记的册：取最新、点读都答没有，不报错；迁移不种任何行。
func TestLocationTypeRegistryOfAnEmptyRegisterAnswersNothingAndSeedsNoRows(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")

	if _, found, err := stores.locationTypes.LoadLatestLocationType(ctx, tenant, code); err != nil || found {
		t.Fatalf("空册取最新 = (%v, %v)，want (false, nil)", found, err)
	}
	if _, found, err := stores.locationTypes.LoadLocationTypeRevision(ctx, tenant, code, 1); err != nil || found {
		t.Fatalf("空册点读 = (%v, %v)，want (false, nil)", found, err)
	}
	if rows := countLocationTypeRows(t, stores); rows != 0 {
		t.Fatalf("迁移之后表里有 %d 行，want 不带任何默认行", rows)
	}
}

// 读口的入参没经构造器（零值）是装配错了，答 error，不折成「没有」：折成没有会让登记方从修订 1 重登一个本来就在册的库位类型。
func TestLocationTypeRegistryRefusesUnconstructedKeysOnRead(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")

	for name, call := range map[string]func() error{
		"缺租户的取最新": func() error {
			_, _, err := stores.locationTypes.LoadLatestLocationType(ctx, domain.TenantID{}, code)
			return err
		},
		"缺库位类型编码的取最新": func() error {
			_, _, err := stores.locationTypes.LoadLatestLocationType(ctx, tenant, domain.LocationTypeCode{})
			return err
		},
		"缺租户的点读": func() error {
			_, _, err := stores.locationTypes.LoadLocationTypeRevision(ctx, domain.TenantID{}, code, 1)
			return err
		},
		"缺库位类型编码的点读": func() error {
			_, _, err := stores.locationTypes.LoadLocationTypeRevision(ctx, tenant, domain.LocationTypeCode{}, 1)
			return err
		},
		"缺租户的修订史": func() error {
			_, err := stores.catalogue.ListLocationTypeRevisions(ctx, domain.TenantID{}, code)
			return err
		},
		"缺库位类型编码的修订史": func() error {
			_, err := stores.catalogue.ListLocationTypeRevisions(ctx, tenant, domain.LocationTypeCode{})
			return err
		},
		"缺租户的列表": func() error {
			_, err := stores.catalogue.ListLocationTypes(ctx, domain.TenantID{}, 50, locationTypeFirstPage(t, nil))
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s应报错", name)
		}
	}
}

// 写口在没有环境事务时拒绝（PBC-08）：登记的落册与它的读必须在同一个事务里，无事务就走自动提交会让「查过没有再写」失去意义。
func TestLocationTypeWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newLocationTypeStores(t)
	if _, err := stores.locationTypes.SaveLocationTypeRevision(t.Context(), locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, nil)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveLocationTypeRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// 库上的 CHECK 与主键各守一格：修订号为正、非空白、同一修订号至多一笔。绕过领域构造器直接插行，逐一证库上真拒（不是只靠领域那一层）。
// 库位类型表不对仓表建外键：「本租户一个仓都没登记就答未配置」由应用层判，所以库上插得进一个租户没登记过仓的库位类型行——这是有意的。
func TestLocationTypeRevisionTableGuardsItsKeysAndBlankCells(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()

	type row struct {
		tenant, code string
		revision     int
		name         string
		registrant   string
	}
	good := row{tenant: "SYN-TENANT-01", code: "SYN-LT-01", revision: 1, name: "合成货架位", registrant: "operator-1"}
	insert := func(candidate row) error {
		_, err := stores.pool.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.location_type_revision
				(tenant_id, location_type_code, revision, location_type_name, deactivated, registrant, registered_at)
			 VALUES ($1, $2, $3, $4, false, $5, now())`,
			candidate.tenant, candidate.code, candidate.revision, candidate.name, candidate.registrant)
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
		"修订号为 0":    func(r *row) { r.revision = 0 },
		"修订号为负":     func(r *row) { r.revision = -1 },
		"租户全空格":     func(r *row) { r.tenant = "  " },
		"库位类型编码全空格": func(r *row) { r.code = "   " },
		"库位类型编码为空":  func(r *row) { r.code = "" },
		"库位类型名称为空":  func(r *row) { r.name = "" },
		"登记责任方为空":   func(r *row) { r.registrant = "" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); violation(err) != "23514" {
			t.Errorf("%s：插入应被 CHECK 拒（23514），实得 %v", name, err)
		}
	}

	if err := insert(good); err != nil {
		t.Fatalf("合法行应能插入（仓册里没有任何仓也插得进：不建外键）：%v", err)
	}
	if err := insert(good); violation(err) != "23505" {
		t.Errorf("同一（租户，库位类型编码，修订号）插两次应被主键拒（23505），实得 %v", err)
	}
	for name, mutate := range map[string]func(*row){
		"另一租户":         func(r *row) { r.tenant = "SYN-TENANT-02" },
		"另一个库位类型编码":    func(r *row) { r.code = "SYN-LT-02" },
		"同一库位类型的修订 2":  func(r *row) { r.revision = 2 },
		"只差大小写的库位类型编码": func(r *row) { r.code = "syn-lt-01" },
	} {
		candidate := good
		mutate(&candidate)
		if err := insert(candidate); err != nil {
			t.Errorf("%s不该被拒：%v", name, err)
		}
	}
}

// Covers: AT-WF-001、AT-WF-009（库位类型的部分）端到端——登记用例接真的仓册与库位类型册：一个仓都没登记时答 NOT_CONFIGURED，不落册；别的租户
// 登记的仓不算本租户的仓；登记仓之后同一修订原样重交落册，再原样重投答已在册；停用的仓也算登记过（只有停用的仓时照常登记）；
// 「仓册」与「库位类型册」是两张表，「本租户有没有仓」靠应用层经仓册的读法判。
func TestRegisterLocationTypeOnTheRealRegistersAnswersNotConfiguredUntilTheTenantHasAWarehouse(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	handler := application.NewRegisterLocationTypeHandler(stores.locationTypes, stores.warehouses, fixedClock{at: baseInstant})

	register := func(tenant string, revision int) application.RegisterLocationTypeResult {
		t.Helper()
		command := application.RegisterLocationTypeCommand{
			Tenant:     valueOf(t, domain.NewTenantID, tenant),
			Code:       valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"),
			Revision:   revision,
			Content:    locationTypeContentWith(t, nil),
			Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
		}
		var result application.RegisterLocationTypeResult
		err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			result, registerErr = handler.Register(txCtx, command)
			return registerErr
		})
		if err != nil {
			t.Fatalf("登记库位类型：%v", err)
		}
		return result
	}

	if result := register("SYN-TENANT-01", 1); result.Outcome() != application.RegistrationNotConfigured {
		t.Fatalf("一个仓都没登记 = %s，want NOT_CONFIGURED", result.Outcome())
	}
	if rows := countLocationTypeRows(t, stores); rows != 0 {
		t.Fatalf("未配置之后册里有 %d 笔，want 不落册", rows)
	}

	// 别的租户登记了仓：不算本租户的仓。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-02", "SYN-WH-01", 1, nil))
	if result := register("SYN-TENANT-01", 1); result.Outcome() != application.RegistrationNotConfigured {
		t.Fatalf("只有别的租户登记了仓 = %s，want NOT_CONFIGURED", result.Outcome())
	}

	// 本租户的仓只有一笔、而且是停用的：停用的仓也算登记过。
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	if landed := register("SYN-TENANT-01", 1); landed.Outcome() != application.RegistrationRegistered {
		t.Fatalf("登记（停用的）仓之后原样重交 = %s，want REGISTERED", landed.Outcome())
	}
	if replay := register("SYN-TENANT-01", 1); replay.Outcome() != application.RegistrationAlreadyRegistered {
		t.Fatalf("再原样重交 = %s，want ALREADY_REGISTERED", replay.Outcome())
	}
	if skipped := register("SYN-TENANT-01", 3); skipped.Outcome() != application.RegistrationNotAccepted ||
		skipped.Refusal() != application.RegistrationRefusalRevisionOutOfSequence || skipped.LatestRevision() != 1 {
		t.Fatalf("跳号 = %s/%s 回显 %d，want NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 回显 1", skipped.Outcome(), skipped.Refusal(), skipped.LatestRevision())
	}
	if rows := countLocationTypeRows(t, stores); rows != 1 {
		t.Fatalf("册里 %d 笔，want 1（未配置、重放与错位都不落册）", rows)
	}
}
