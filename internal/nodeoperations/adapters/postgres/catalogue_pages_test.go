package postgres_test

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证本上下文各册迁到 ADR-0144 之后的取页（票 catalogue-read-pagination-rollout/06）：每册按页大小 2 走完
// 全部页，取完第一页之后在已翻过的位置之前与未翻的位置之中各落一行——已翻过的页不重出、未翻的页不漏行，落在前面的那一行这一趟
// 不出现，total 随之变；同一维反向再走一遍，决胜键随之反向；筛选维与 q 改变 total，q 里的 % 与 _ 按字面；没有行的租户答空页且
// total 为 0。键集机制本身由共用件证过，这里证的是各册落列：落错一列，次序就乱、页与页之间就重出或漏行。夹具全为合成（S 级）。

const (
	pagesNode = "SYN-NODE-CRPR06"
	pagesHub  = "SYN-NODE-CRPR06-HUB"
	pagesEdge = "SYN-NODE-CRPR06-EDGE"
)

// pagesWalk 是一趟翻页：每页的行标识，与每页答的 total。
type pagesWalk struct {
	pages  [][]string
	totals []int64
}

// walkWhileArriving 用同一组条件走完全部页；arrive 非空时在取完第一页、接着翻之前调它一次，模拟翻页期间新落的行。
func walkWhileArriving[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	values url.Values,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	id func(Row) string,
	arrive func(),
) pagesWalk {
	t.Helper()
	var walked pagesWalk
	query := pageQuery(t, catalogue, values)
	for {
		page, err := list(query)
		if err != nil {
			t.Fatalf("取页：%v", err)
		}
		ids := make([]string, len(page.Rows))
		for index, row := range page.Rows {
			ids[index] = id(row)
		}
		walked.pages = append(walked.pages, ids)
		walked.totals = append(walked.totals, page.Total)
		if page.Next == "" {
			return walked
		}
		if len(walked.pages) > 20 {
			t.Fatal("翻了二十页还没到末页：游标没有前进")
		}
		if len(walked.pages) == 1 && arrive != nil {
			arrive()
		}
		query = nextPage(t, catalogue, values, page.Next)
	}
}

func expectWalk(t *testing.T, got pagesWalk, pages [][]string, totals []int64) {
	t.Helper()
	if !reflect.DeepEqual(got.pages, pages) || !reflect.DeepEqual(got.totals, totals) {
		t.Fatalf("翻页得 %v / total %v，应为 %v / %v", got.pages, got.totals, pages, totals)
	}
}

// expectTotals 逐组条件（键是查询串）取第一页，断言各自答的 total。
func expectTotals[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	want map[string]int64,
) {
	t.Helper()
	for raw, total := range want {
		values, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		page, err := list(pageQuery(t, catalogue, values))
		if err != nil {
			t.Fatalf("%q：%v", raw, err)
		}
		if page.Total != total {
			t.Errorf("%q 的 total = %d，应为 %d", raw, page.Total, total)
		}
	}
}

// expectEmpty 断言一册在没有行的租户下答空页、total 为 0、没有下一页。
func expectEmpty[Row any](t *testing.T, page cataloguepage.Result[Row], err error) {
	t.Helper()
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册得 %d 行 / total %d / next %q（%v），应为空页、total 0、没有下一页", len(page.Rows), page.Total, page.Next, err)
	}
}

// pagesFixture 把收寄册、集运两册与查阅目录建在同一个测试库上：pgtest.Pool 每调一次是一个新库，分开建会读到两个世界。
type pagesFixture struct {
	transactor bentoapp.Transactor
	catalogue  *adapter.ReviewCatalogue
	receptions *adapter.Receptions
	units      *adapter.ConsolidationUnits
	facts      *adapter.ConsolidationFacts
}

func newPagesFixture(t *testing.T) *pagesFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &pagesFixture{transactor: db.Transactor()}
	if fixture.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatal(err)
	}
	if fixture.receptions, err = adapter.NewReceptions(db); err != nil {
		t.Fatal(err)
	}
	if fixture.units, err = adapter.NewConsolidationUnits(db); err != nil {
		t.Fatal(err)
	}
	if fixture.facts, err = adapter.NewConsolidationFacts(db); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *pagesFixture) save(t *testing.T, records ...ports.ReceptionRecord) {
	t.Helper()
	saveReceptions(t, fixture.transactor, t.Context(), fixture.receptions, records...)
}

// pagedIntake 拼一行带收寄与控制的判断，登记时刻取 recordedAt：association 非空即收寄已形成，否则待识别（已核对、零候选）。
func pagedIntake(t *testing.T, tenant, sourceID, unit string, recordedAt time.Time, association string) ports.ReceptionRecord {
	t.Helper()
	tenantID := ref(t, domain.NewTenantID, tenant)
	spec := domain.NodeIntakeSpec{
		TenantID:    tenantID,
		Unit:        ref(t, domain.NewHandlingUnitID, unit),
		Node:        ref(t, domain.NewNodeReference, pagesNode),
		DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "SYN-COURIER-CRPR06"),
		Evidence:    ref(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-CRPR06"),
		Version:     ref(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-"+sourceID),
		ReceivedAt:  receivedAt,
	}
	record := ports.ReceptionRecord{
		Key:             ports.ReceptionKey{TenantID: tenantID, SourceID: sourceID},
		ContentDigest:   "SYN-DIGEST-" + sourceID,
		Kind:            ports.RecordPendingIdentification,
		JudgmentVersion: spec.Version,
		IdentityCheck:   ports.IdentityChecked,
		RecordedAt:      recordedAt,
	}
	if association != "" {
		spec.Association = ref(t, domain.NewParcelAssociationReference, association)
		record.Kind, record.IdentityCheck, record.ServiceResultCheck = ports.RecordIntakeFormed, ports.IdentityCheckAbsent, ports.ServiceResultsChecked
	}
	intake, err := domain.FormNodeIntake(spec)
	if err != nil {
		t.Fatalf("构造收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID: tenantID, Unit: spec.Unit, Node: spec.Node, Kind: domain.EstablishedByNodeIntake,
		Basis: ref(t, domain.NewControlBasisReference, "NODE-INTAKE/"+spec.Version.String()), EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	record.Intake, record.Control = intake, control
	return record
}

// pagedUnchecked 是未核对的一行待识别：候选为空、不记冲突、来源已保全（迁移 0011 的身份核对约束要这三格）。
func pagedUnchecked(t *testing.T, tenant, sourceID, unit string, recordedAt time.Time) ports.ReceptionRecord {
	t.Helper()
	record := pagedIntake(t, tenant, sourceID, unit, recordedAt, "")
	record.IdentityCheck = ports.IdentityUnchecked
	record.Source = domain.DeliverySource{
		Unit:        record.Intake.Unit(),
		Mark:        "SYN-MARK-" + sourceID,
		Claim:       "RECEIVED",
		DeliveredBy: record.Intake.DeliveredBy(),
		Node:        record.Intake.Node(),
		OccurredAt:  receivedAt,
		Evidence:    ref(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-CRPR06"),
	}
	return record
}

func receptionRowID(row ports.ReceptionCatalogueRow) string { return row.SourceID }

// Covers: 收寄册按登记时刻倒序翻页，翻页期间新落的两行一前一后：前面那行这一趟不出现，后面那行在末页；kind 与 q 改变 total；
// 待识别那一格与形成那一格同册。
func TestReceptionsPageWhileNewIntakesArrive(t *testing.T) {
	fixture := newPagesFixture(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-CRPR06-R"
	at := func(minutes int) time.Time { return receivedAt.Add(time.Duration(minutes) * time.Minute) }
	fixture.save(t,
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-R-F1", "SYN-HU-CRPR06-R-F1", at(4), "SYN-PARCEL-CRPR06-R-F1"),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-R-P1", "SYN-HU-CRPR06-R-P1", at(3), ""),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-R-F2", "SYN-HU-CRPR06-R-F2", at(2), "SYN-PARCEL-CRPR06-R-F2"),
	)
	tenantID := ref(t, domain.NewTenantID, tenant)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ReceptionCatalogueRow], error) {
		return fixture.catalogue.ListReceptions(ctx, tenantID, 2, query)
	}

	walked := walkWhileArriving(t, ports.ReceptionPages, url.Values{}, list, receptionRowID, func() {
		fixture.save(t,
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-R-F3", "SYN-HU-CRPR06-R-F3", at(9), "SYN-PARCEL-CRPR06-R-F3"),
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-R-P2", "SYN-HU-CRPR06-R-P2", at(1), ""),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-SRC-CRPR06-R-F1", "SYN-SRC-CRPR06-R-P1"},
		{"SYN-SRC-CRPR06-R-F2", "SYN-SRC-CRPR06-R-P2"},
	}, []int64{3, 5})
	expectWalk(t, walkWhileArriving(t, ports.ReceptionPages, url.Values{"sort": {"recordedAt"}}, list, receptionRowID, nil), [][]string{
		{"SYN-SRC-CRPR06-R-P2", "SYN-SRC-CRPR06-R-F2"},
		{"SYN-SRC-CRPR06-R-P1", "SYN-SRC-CRPR06-R-F1"},
		{"SYN-SRC-CRPR06-R-F3"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.ReceptionPages, list, map[string]int64{
		"":                            5,
		"kind=INTAKE_FORMED":          3,
		"kind=PENDING_IDENTIFICATION": 2,
		"q=src-crpr06-r-f2":           1,
		"q=SYN-HU-CRPR06-R-P":         2,
		"kind=INTAKE_FORMED&q=R-P":    0,
		"q=%25":                       0,
		"q=_":                         0,
	})
	page, err := fixture.catalogue.ListReceptions(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.ReceptionPages))
	expectEmpty(t, page, err)
}

func unidentifiedItemID(row ports.UnidentifiedItemCatalogueRow) string { return row.SourceID }

// Covers: 待识别实物册按登记时刻倒序翻页，翻页期间新落的两行一前一后；形成那一格不在本册；identityCheck 与 q（含来源里的外部
// 标识观察）改变 total。
func TestUnidentifiedItemsPageWhileNewPendingItemsArrive(t *testing.T) {
	fixture := newPagesFixture(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-CRPR06-I"
	at := func(minutes int) time.Time { return receivedAt.Add(time.Duration(minutes) * time.Minute) }
	fixture.save(t,
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-I-F", "SYN-HU-CRPR06-I-F", at(5), "SYN-PARCEL-CRPR06-I-F"),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-I-A", "SYN-HU-CRPR06-I-A", at(4), ""),
		pagedUnchecked(t, tenant, "SYN-SRC-CRPR06-I-B", "SYN-HU-CRPR06-I-B", at(3)),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-I-C", "SYN-HU-CRPR06-I-C", at(2), ""),
	)
	tenantID := ref(t, domain.NewTenantID, tenant)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.UnidentifiedItemCatalogueRow], error) {
		return fixture.catalogue.ListUnidentifiedItems(ctx, tenantID, 2, query)
	}

	walked := walkWhileArriving(t, ports.UnidentifiedItemPages, url.Values{}, list, unidentifiedItemID, func() {
		fixture.save(t,
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-I-X", "SYN-HU-CRPR06-I-X", at(9), ""),
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-I-Y", "SYN-HU-CRPR06-I-Y", at(1), ""),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-SRC-CRPR06-I-A", "SYN-SRC-CRPR06-I-B"},
		{"SYN-SRC-CRPR06-I-C", "SYN-SRC-CRPR06-I-Y"},
	}, []int64{3, 5})
	expectWalk(t, walkWhileArriving(t, ports.UnidentifiedItemPages, url.Values{"sort": {"recordedAt"}}, list, unidentifiedItemID, nil), [][]string{
		{"SYN-SRC-CRPR06-I-Y", "SYN-SRC-CRPR06-I-C"},
		{"SYN-SRC-CRPR06-I-B", "SYN-SRC-CRPR06-I-A"},
		{"SYN-SRC-CRPR06-I-X"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.UnidentifiedItemPages, list, map[string]int64{
		"":                              5,
		"identityCheck=UNCHECKED":       1,
		"identityCheck=CHECKED":         4,
		"q=SYN-MARK-SYN-SRC-CRPR06-I":   1,
		"q=HU-CRPR06-I-C":               1,
		"identityCheck=UNCHECKED&q=I-C": 0,
		"q=%25":                         0,
		"q=_":                           0,
	})
	page, err := fixture.catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.UnidentifiedItemPages))
	expectEmpty(t, page, err)
}

func identityReviewUnitID(row ports.IdentityReviewUnitRow) string { return row.Unit }

// Covers: 身份复核单元册是聚合册，键集落在导出列（每件实物待识别各版里最近一次登记时刻）上：按它倒序翻页；翻页期间新落两件实物
// 一前一后，前面那件这一趟不出现、后面那件在末页；已翻过的一件实物又登了一版待识别，它的导出列前移，不在后面的页上重出。只有形成
// 那一格的实物不在本册；q 改变 total。
func TestIdentityReviewUnitsPageOnTheirLatestRecording(t *testing.T) {
	fixture := newPagesFixture(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-CRPR06-U"
	at := func(minutes int) time.Time { return receivedAt.Add(time.Duration(minutes) * time.Minute) }
	fixture.save(t,
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-U7", "SYN-HU-CRPR06-U7", at(5), "SYN-PARCEL-CRPR06-U7"),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-U1-NEW", "SYN-HU-CRPR06-U1", at(4), ""),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-U2", "SYN-HU-CRPR06-U2", at(3), ""),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-U3", "SYN-HU-CRPR06-U3", at(2), ""),
		pagedIntake(t, tenant, "SYN-SRC-CRPR06-U1-OLD", "SYN-HU-CRPR06-U1", at(1), ""),
	)
	tenantID := ref(t, domain.NewTenantID, tenant)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.IdentityReviewUnitRow], error) {
		return fixture.catalogue.ListIdentityReviewUnits(ctx, tenantID, 2, query)
	}

	first, err := list(firstPage(t, ports.IdentityReviewUnitPages))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) == 0 || first.Rows[0].Unit != "SYN-HU-CRPR06-U1" || !first.Rows[0].LatestRecordedAt.Equal(at(4)) {
		t.Fatalf("第一页首行 = %+v，应为 SYN-HU-CRPR06-U1、最近一次登记取它两版里较近的那一版", first.Rows)
	}
	walked := walkWhileArriving(t, ports.IdentityReviewUnitPages, url.Values{}, list, identityReviewUnitID, func() {
		fixture.save(t,
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-U0", "SYN-HU-CRPR06-U0", at(8), ""),
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-U1-AGAIN", "SYN-HU-CRPR06-U1", at(9), ""),
			pagedIntake(t, tenant, "SYN-SRC-CRPR06-U9", "SYN-HU-CRPR06-U9", at(0), ""),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-HU-CRPR06-U1", "SYN-HU-CRPR06-U2"},
		{"SYN-HU-CRPR06-U3", "SYN-HU-CRPR06-U9"},
	}, []int64{3, 5})
	expectWalk(t, walkWhileArriving(t, ports.IdentityReviewUnitPages, url.Values{"sort": {"latestRecordedAt"}}, list, identityReviewUnitID, nil), [][]string{
		{"SYN-HU-CRPR06-U9", "SYN-HU-CRPR06-U3"},
		{"SYN-HU-CRPR06-U2", "SYN-HU-CRPR06-U0"},
		{"SYN-HU-CRPR06-U1"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.IdentityReviewUnitPages, list, map[string]int64{
		"":                5,
		"q=hu-crpr06-u1":  1,
		"q=SYN-HU-CRPR06": 5,
		"q=CRPR06-U7":     0,
		"q=%25":           0,
		"q=_":             0,
	})
	page, err := fixture.catalogue.ListIdentityReviewUnits(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.IdentityReviewUnitPages))
	expectEmpty(t, page, err)
}

// pagedConsolidation 开一个集运单元，执行方与证据都取合成值。
func pagedConsolidation(t *testing.T, id string) *domain.ConsolidationUnit {
	t.Helper()
	unit, err := domain.OpenConsolidationUnit(
		ref(t, domain.NewConsolidationUnitID, id),
		ref(t, domain.NewCarrierAssetReference, "SYN-ASSET-"+id),
		workSourceBy(t, "SYN-OPEN-"+id, "SYN-PACKER-CRPR06", consolidationAt),
	)
	if err != nil {
		t.Fatalf("开启集运单元：%v", err)
	}
	return unit
}

func consolidationUnitID(row ports.ConsolidationUnitCatalogueRow) string { return row.UnitID }

// Covers: 集运单元册按单元标识正序翻页（迁移前的次序；标识即行标识），翻页期间新开的两个单元一前一后；phase 落在相位列上、q 命中
// 单元标识，二者改变 total。
func TestConsolidationUnitsPageByUnitIDWhileNewUnitsOpen(t *testing.T) {
	fixture := newPagesFixture(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-C")
	for _, id := range []string{"SYN-BAG-CRPR06-2", "SYN-BAG-CRPR06-6"} {
		saveConsolidation(t, fixture.transactor, ctx, fixture.units, tenant, pagedConsolidation(t, id))
	}
	sealed := pagedConsolidation(t, "SYN-BAG-CRPR06-4")
	saveConsolidation(t, fixture.transactor, ctx, fixture.units, tenant, sealed)
	if err := sealed.AddMember(ref(t, domain.NewHandlingUnitID, "SYN-HU-CRPR06-41")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := sealed.Seal(
		ref(t, domain.NewSealReference, "SYN-SEAL-CRPR06-4"),
		ref(t, domain.NewWorkBasisReference, "SYN-PACK-CRPR06-4"),
		workSourceBy(t, "SYN-SEAL-SRC-CRPR06-4", "SYN-SEALER-CRPR06", consolidationAt),
	); err != nil {
		t.Fatalf("封装：%v", err)
	}
	updateConsolidation(t, fixture.transactor, ctx, fixture.units, tenant, sealed)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ConsolidationUnitCatalogueRow], error) {
		return fixture.catalogue.ListConsolidationUnits(ctx, tenant, 2, query)
	}

	walked := walkWhileArriving(t, ports.ConsolidationUnitPages, url.Values{}, list, consolidationUnitID, func() {
		for _, id := range []string{"SYN-BAG-CRPR06-1", "SYN-BAG-CRPR06-9"} {
			saveConsolidation(t, fixture.transactor, ctx, fixture.units, tenant, pagedConsolidation(t, id))
		}
	})
	expectWalk(t, walked, [][]string{
		{"SYN-BAG-CRPR06-2", "SYN-BAG-CRPR06-4"},
		{"SYN-BAG-CRPR06-6", "SYN-BAG-CRPR06-9"},
	}, []int64{3, 5})
	expectWalk(t, walkWhileArriving(t, ports.ConsolidationUnitPages, url.Values{"sort": {"-unitId"}}, list, consolidationUnitID, nil), [][]string{
		{"SYN-BAG-CRPR06-9", "SYN-BAG-CRPR06-6"},
		{"SYN-BAG-CRPR06-4", "SYN-BAG-CRPR06-2"},
		{"SYN-BAG-CRPR06-1"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.ConsolidationUnitPages, list, map[string]int64{
		"":                      5,
		"phase=OPEN":            4,
		"phase=SEALED":          1,
		"phase=CLOSED":          0,
		"q=bag-crpr06-6":        1,
		"q=SYN-SEAL-CRPR06":     1,
		"phase=OPEN&q=CRPR06-4": 0,
		"q=%25":                 0,
		"q=_":                   0,
	})
	page, err := fixture.catalogue.ListConsolidationUnits(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.ConsolidationUnitPages))
	expectEmpty(t, page, err)
}

// pagedMembership 是一份落册时刻取 recordedAt 的集运作业事实；member 为空即不带成员（开启那一格）。
func pagedMembership(
	t *testing.T,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
	sourceID string,
	action domain.ConsolidationActionKind,
	member string,
	recordedAt time.Time,
) ports.ConsolidationFactRecord {
	t.Helper()
	source := workSourceBy(t, sourceID, "SYN-PACKER-CRPR06", consolidationAt)
	record := ports.ConsolidationFactRecord{
		Key:           ports.ConsolidationFactKey{TenantID: tenant, SourceID: sourceID},
		ContentDigest: "SYN-DIGEST-" + sourceID,
		Action:        action,
		Unit:          unit.ID(),
		PerformedBy:   source.PerformedBy(),
		Evidence:      source.Evidence(),
		OccurredAt:    source.OccurredAt(),
		RecordedAt:    recordedAt,
	}
	if member != "" {
		record.Member = ref(t, domain.NewHandlingUnitID, member)
	}
	return record
}

func consolidationMemberID(row ports.ConsolidationMemberCatalogueRow) string { return row.SourceID }

// Covers: 成员关系册按落册时刻倒序翻页，只列移入与移出两种事实（开启那一份不在）；翻页期间新落的两份一前一后；action 与 q
// 改变 total。
func TestConsolidationMembersPageWhileNewFactsArrive(t *testing.T) {
	fixture := newPagesFixture(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-M")
	unit := pagedConsolidation(t, "SYN-BAG-CRPR06-M")
	saveConsolidation(t, fixture.transactor, ctx, fixture.units, tenant, unit)
	at := func(minutes int) time.Time { return consolidationAt.Add(time.Duration(minutes) * time.Minute) }
	for _, record := range []ports.ConsolidationFactRecord{
		pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-OPEN", domain.OpenUnitAction, "", at(5)),
		pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-A1", domain.AddMemberAction, "SYN-HU-CRPR06-M1", at(4)),
		pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-A2", domain.AddMemberAction, "SYN-HU-CRPR06-M2", at(3)),
		pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-R1", domain.RemoveMemberAction, "SYN-HU-CRPR06-M1", at(2)),
	} {
		saveConsolidationFact(t, fixture.transactor, ctx, fixture.facts, record)
	}
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ConsolidationMemberCatalogueRow], error) {
		return fixture.catalogue.ListConsolidationMembers(ctx, tenant, 2, query)
	}

	walked := walkWhileArriving(t, ports.ConsolidationMemberPages, url.Values{}, list, consolidationMemberID, func() {
		saveConsolidationFact(t, fixture.transactor, ctx, fixture.facts,
			pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-A3", domain.AddMemberAction, "SYN-HU-CRPR06-M3", at(9)))
		saveConsolidationFact(t, fixture.transactor, ctx, fixture.facts,
			pagedMembership(t, tenant, unit, "SYN-FACT-CRPR06-R2", domain.RemoveMemberAction, "SYN-HU-CRPR06-M2", at(1)))
	})
	expectWalk(t, walked, [][]string{
		{"SYN-FACT-CRPR06-A1", "SYN-FACT-CRPR06-A2"},
		{"SYN-FACT-CRPR06-R1", "SYN-FACT-CRPR06-R2"},
	}, []int64{3, 5})
	expectWalk(t, walkWhileArriving(t, ports.ConsolidationMemberPages, url.Values{"sort": {"recordedAt"}}, list, consolidationMemberID, nil), [][]string{
		{"SYN-FACT-CRPR06-R2", "SYN-FACT-CRPR06-R1"},
		{"SYN-FACT-CRPR06-A2", "SYN-FACT-CRPR06-A1"},
		{"SYN-FACT-CRPR06-A3"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.ConsolidationMemberPages, list, map[string]int64{
		"":                                  5,
		"action=ADD_MEMBER":                 3,
		"action=REMOVE_MEMBER":              2,
		"q=hu-crpr06-m3":                    1,
		"q=SYN-HU-CRPR06-M2":                2,
		"action=ADD_MEMBER&q=FACT-CRPR06-R": 0,
		"q=%25":                             0,
		"q=_":                               0,
	})
	page, err := fixture.catalogue.ListConsolidationMembers(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.ConsolidationMemberPages))
	expectEmpty(t, page, err)
}

// unresolvedTransition 是一行对不上作业实物的转出（没推进），记录时刻取 recordedAt。
func unresolvedTransition(t *testing.T, object string, recordedAt time.Time) ports.ControlTransitionRecord {
	t.Helper()
	handover, err := domain.NewAuthoritativeHandoverReference(object, "SYN-SCOPE-CRPR06", "v1")
	if err != nil {
		t.Fatal(err)
	}
	transition, err := domain.ObjectNotResolved(ref(t, domain.NewTenantID, controlTenant), handover, domain.ControlTransitionOut,
		domain.NoAssociatedUnit, controlHandedOverAt)
	if err != nil {
		t.Fatal(err)
	}
	return ports.ControlTransitionRecord{Transition: transition, RecordedAt: recordedAt}
}

func (fixture *controlFixture) saveTransitions(t *testing.T, records ...ports.ControlTransitionRecord) {
	t.Helper()
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := fixture.transitions.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func controlTransitionObject(row ports.PhysicalControlTransitionCatalogueRow) string {
	return row.HandoverObject
}

// Covers: 节点控制转移册按记录时刻倒序翻页（行标识是交接键四段），推进了的与没推进的同册；翻页期间新落的两行一前一后；outcome
// 与 q 改变 total。
func TestPhysicalControlTransitionsPageWhileNewTransitionsArrive(t *testing.T) {
	fixture := newControlFixture(t)
	ctx := t.Context()
	fixture.save(t, intakeWithControl(t, "SYN-SRC-61-A", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-A", "SYN-PARCEL-61", "", "SYN-INTAKEV-61-A"))
	fixture.transferOut(t, "SYN-HU-61", "v1")
	at := func(minutes int) time.Time { return controlHandedOverAt.Add(time.Duration(minutes) * time.Minute) }
	fixture.saveTransitions(t,
		unresolvedTransition(t, "SYN-PARCEL-CRPR06-A", at(4)),
		unresolvedTransition(t, "SYN-PARCEL-CRPR06-B", at(3)),
		unresolvedTransition(t, "SYN-PARCEL-CRPR06-C", at(2)),
	)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.PhysicalControlTransitionCatalogueRow], error) {
		return fixture.catalogue.ListPhysicalControlTransitions(ctx, tenant, 2, query)
	}

	walked := walkWhileArriving(t, ports.PhysicalControlTransitionPages, url.Values{}, list, controlTransitionObject, func() {
		fixture.saveTransitions(t,
			unresolvedTransition(t, "SYN-PARCEL-CRPR06-X", at(9)),
			unresolvedTransition(t, "SYN-PARCEL-CRPR06-Y", at(0)),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-PARCEL-CRPR06-A", "SYN-PARCEL-CRPR06-B"},
		{"SYN-PARCEL-CRPR06-C", "SYN-PARCEL-61"},
		{"SYN-PARCEL-CRPR06-Y"},
	}, []int64{4, 6, 6})
	expectWalk(t, walkWhileArriving(t, ports.PhysicalControlTransitionPages, url.Values{"sort": {"recordedAt"}}, list, controlTransitionObject, nil), [][]string{
		{"SYN-PARCEL-CRPR06-Y", "SYN-PARCEL-61"},
		{"SYN-PARCEL-CRPR06-C", "SYN-PARCEL-CRPR06-B"},
		{"SYN-PARCEL-CRPR06-A", "SYN-PARCEL-CRPR06-X"},
	}, []int64{6, 6, 6})
	expectTotals(t, ports.PhysicalControlTransitionPages, list, map[string]int64{
		"":                                 6,
		"outcome=ADVANCED":                 1,
		"outcome=NOT_ADVANCED":             5,
		"q=parcel-crpr06-b":                1,
		"q=SYN-HU-61":                      1,
		"outcome=ADVANCED&q=PARCEL-CRPR06": 0,
		"q=%25":                            0,
		"q=_":                              0,
	})
	page, err := fixture.catalogue.ListPhysicalControlTransitions(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), 2, firstPage(t, ports.PhysicalControlTransitionPages))
	expectEmpty(t, page, err)
}

func preAdviceID(state domain.DeliveryPreAdviceState) string { return state.PreAdvice.ID().String() }

// Covers: 送站预报列表按预计送达倒序翻页，翻页期间新落的两份一前一后；node 选择器与查询对象同时生效，带着它翻页的游标照常前进；
// q 改变 total。
func TestDeliveryPreAdvicesPageWhileNewPreAdvicesArrive(t *testing.T) {
	fixture := newDeliveryScopeCatalogueFixture(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-CRPR06-D"
	record := func(preAdvices ...domain.DeliveryPreAdvice) {
		t.Helper()
		if err := inTransaction(fixture.transactor, func(txCtx context.Context) error {
			for _, preAdvice := range preAdvices {
				if _, _, err := fixture.preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("落预报：%v", err)
		}
	}
	record(
		preAdviceForCatalogue(t, tenant, "SYN-PREADV-CRPR06-1", pagesHub, catalogueInstant(20, 10), "", "SYN-NUMBER-CRPR06-1"),
		preAdviceForCatalogue(t, tenant, "SYN-PREADV-CRPR06-2", pagesEdge, catalogueInstant(20, 12), "", "SYN-NUMBER-CRPR06-2"),
		preAdviceForCatalogue(t, tenant, "SYN-PREADV-CRPR06-3", pagesHub, catalogueInstant(20, 14), "", "SYN-NUMBER-CRPR06-3"),
	)
	tenantID := must(t, domain.NewTenantID, tenant)
	listAt := func(filter ports.DeliveryScopeCatalogueFilter) func(cataloguepage.Query) (cataloguepage.Result[domain.DeliveryPreAdviceState], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[domain.DeliveryPreAdviceState], error) {
			return fixture.catalogue.ListDeliveryPreAdvices(ctx, tenantID, filter, 2, query)
		}
	}
	list := listAt(ports.DeliveryScopeCatalogueFilter{})

	walked := walkWhileArriving(t, ports.DeliveryPreAdvicePages, url.Values{}, list, preAdviceID, func() {
		record(
			preAdviceForCatalogue(t, tenant, "SYN-PREADV-CRPR06-9", pagesHub, catalogueInstant(21, 9), "", "SYN-NUMBER-CRPR06-9"),
			preAdviceForCatalogue(t, tenant, "SYN-PREADV-CRPR06-0", pagesEdge, catalogueInstant(19, 9), "", "SYN-NUMBER-CRPR06-0"),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-PREADV-CRPR06-3", "SYN-PREADV-CRPR06-2"},
		{"SYN-PREADV-CRPR06-1", "SYN-PREADV-CRPR06-0"},
	}, []int64{3, 5})
	hub := listAt(ports.DeliveryScopeCatalogueFilter{Node: must(t, domain.NewNodeReference, pagesHub)})
	expectWalk(t, walkWhileArriving(t, ports.DeliveryPreAdvicePages, url.Values{"node": {pagesHub}}, hub, preAdviceID, nil), [][]string{
		{"SYN-PREADV-CRPR06-9", "SYN-PREADV-CRPR06-3"},
		{"SYN-PREADV-CRPR06-1"},
	}, []int64{3, 3})
	expectWalk(t, walkWhileArriving(t, ports.DeliveryPreAdvicePages, url.Values{"sort": {"expectedArrivalAt"}}, list, preAdviceID, nil), [][]string{
		{"SYN-PREADV-CRPR06-0", "SYN-PREADV-CRPR06-1"},
		{"SYN-PREADV-CRPR06-2", "SYN-PREADV-CRPR06-3"},
		{"SYN-PREADV-CRPR06-9"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.DeliveryPreAdvicePages, list, map[string]int64{
		"":                      5,
		"q=preadv-crpr06-2":     1,
		"q=CRPR06-EDGE":         2,
		"q=SYN-PREADV-CRPR06":   5,
		"q=SYN-PREADV-CRPR06-7": 0,
		"q=%25":                 0,
		"q=_":                   0,
	})
	expectTotals(t, ports.DeliveryPreAdvicePages, hub, map[string]int64{
		"node=" + pagesHub:                    3,
		"node=" + pagesHub + "&q=CRPR06-EDGE": 0,
	})
	page, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, must(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), ports.DeliveryScopeCatalogueFilter{}, 2, firstPage(t, ports.DeliveryPreAdvicePages))
	expectEmpty(t, page, err)
}

func handoverNoteID(row ports.DeliveryHandoverNoteCatalogueRow) string {
	return row.State.Note.ID().String()
}

// Covers: 送站交接单列表按开立时刻倒序翻页，翻页期间新开的两张一前一后；node 选择器与查询对象同时生效；q 改变 total。
func TestDeliveryHandoverNotesPageWhileNewNotesOpen(t *testing.T) {
	fixture := newDeliveryScopeCatalogueFixture(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-CRPR06-D"
	open := func(notes ...domain.DeliveryHandoverNote) {
		t.Helper()
		if err := inTransaction(fixture.transactor, func(txCtx context.Context) error {
			for _, note := range notes {
				if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("开交接单：%v", err)
		}
	}
	open(
		noteForCatalogue(t, tenant, "SYN-HN-CRPR06-1", pagesHub, catalogueInstant(20, 9), "SYN-ACCOUNT-CRPR06-1"),
		noteForCatalogue(t, tenant, "SYN-HN-CRPR06-2", pagesEdge, catalogueInstant(20, 11), "SYN-ACCOUNT-CRPR06-1"),
		noteForCatalogue(t, tenant, "SYN-HN-CRPR06-3", pagesHub, catalogueInstant(20, 13), "SYN-ACCOUNT-CRPR06-1"),
	)
	tenantID := must(t, domain.NewTenantID, tenant)
	listAt := func(filter ports.DeliveryScopeCatalogueFilter) func(cataloguepage.Query) (cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow], error) {
			return fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenantID, filter, 2, query)
		}
	}
	list := listAt(ports.DeliveryScopeCatalogueFilter{})

	walked := walkWhileArriving(t, ports.DeliveryHandoverNotePages, url.Values{}, list, handoverNoteID, func() {
		open(
			noteForCatalogue(t, tenant, "SYN-HN-CRPR06-9", pagesHub, catalogueInstant(21, 9), "SYN-ACCOUNT-CRPR06-1"),
			noteForCatalogue(t, tenant, "SYN-HN-CRPR06-0", pagesEdge, catalogueInstant(19, 9), "SYN-ACCOUNT-CRPR06-1"),
		)
	})
	expectWalk(t, walked, [][]string{
		{"SYN-HN-CRPR06-3", "SYN-HN-CRPR06-2"},
		{"SYN-HN-CRPR06-1", "SYN-HN-CRPR06-0"},
	}, []int64{3, 5})
	hub := listAt(ports.DeliveryScopeCatalogueFilter{Node: must(t, domain.NewNodeReference, pagesHub)})
	expectWalk(t, walkWhileArriving(t, ports.DeliveryHandoverNotePages, url.Values{"node": {pagesHub}}, hub, handoverNoteID, nil), [][]string{
		{"SYN-HN-CRPR06-9", "SYN-HN-CRPR06-3"},
		{"SYN-HN-CRPR06-1"},
	}, []int64{3, 3})
	expectWalk(t, walkWhileArriving(t, ports.DeliveryHandoverNotePages, url.Values{"sort": {"openedAt"}}, list, handoverNoteID, nil), [][]string{
		{"SYN-HN-CRPR06-0", "SYN-HN-CRPR06-1"},
		{"SYN-HN-CRPR06-2", "SYN-HN-CRPR06-3"},
		{"SYN-HN-CRPR06-9"},
	}, []int64{5, 5, 5})
	expectTotals(t, ports.DeliveryHandoverNotePages, list, map[string]int64{
		"":                    5,
		"q=hn-crpr06-2":       1,
		"q=CRPR06-EDGE":       2,
		"q=SYN-DRIVER-PSXM13": 5,
		"q=SYN-HN-CRPR06-7":   0,
		"q=%25":               0,
		"q=_":                 0,
	})
	page, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, must(t, domain.NewTenantID, "SYN-TENANT-CRPR06-NONE"), ports.DeliveryScopeCatalogueFilter{}, 2, firstPage(t, ports.DeliveryHandoverNotePages))
	expectEmpty(t, page, err)
}
