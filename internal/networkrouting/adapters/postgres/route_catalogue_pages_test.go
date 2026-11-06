package postgres_test

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证初始路由、路由复核、改路建议与初始路由并列各册的翻页（ADR-0144 决定一、三至五）：翻页期间落新行，
// 已翻过的页不重出、未翻的页不漏行；首维同值的几行按判断键各段同向决胜；total 随筛选与 q 变，q 的通配符按字面算；空册答空页、
// total 为 0。夹具一律 SYN- 合成值。

const pagingTenant = "SYN-TENANT-01"

var pagingBaseAt = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// nextPage 在同一组条件下解出带游标的下一页查询。
func nextPage(t *testing.T, catalogue *cataloguepage.Catalogue, conditions url.Values, cursor string) cataloguepage.Query {
	t.Helper()
	values := url.Values{"after": {cursor}}
	for key, given := range conditions {
		values[key] = given
	}
	return decodeWith(t, catalogue, values)
}

// rowsOf 只取一页里的行：照列转写的用例只看行，翻页另有用例。
func rowsOf[Row any](page cataloguepage.Result[Row], err error) ([]Row, error) {
	return page.Rows, err
}

func namesOf[Row any](page cataloguepage.Result[Row], name func(Row) string) ([]string, string, int64) {
	names := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		names = append(names, name(row))
	}
	return names, page.Next, page.Total
}

// synJudgmentKey 是一条全由合成值组成的判断键，segments 依次是客户账户、委托、受理基线、包裹、服务目的。
func synJudgmentKey(t *testing.T, segments [5]string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           scalar(t, domain.NewTenantID, pagingTenant),
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, segments[0]),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, segments[1]),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, segments[2]),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, segments[3]),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, segments[4]),
	}
}

// parcelKey 是只在包裹一段上彼此不同的合成判断键。
func parcelKey(t *testing.T, parcel string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return synJudgmentKey(t, [5]string{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1", parcel, "SYN-PURPOSE-1"})
}

func keyRowOf(key domain.InitialRouteJudgmentKey) [6]string {
	return [6]string{
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
	}
}

// keyText 把一行判断键里租户以外的各段按客户账户、委托、受理基线、包裹、服务目的的次序拼起来，初始路由册与并列册的行都照这一种写法比。
func keyText(account, request, baseline, parcel, purpose string) string {
	return strings.Join([]string{account, request, baseline, parcel, purpose}, "/")
}

func pagingTenantID(t *testing.T) domain.TenantID {
	t.Helper()
	return scalar(t, domain.NewTenantID, pagingTenant)
}

// saveSynSuggestion 在 key 上落一行带建议的复核（计划已失效、一家合格候选、只建议不自动），再把落册时刻挪到 recordedAt：
// 落册时刻由库取当前时刻，翻页用例要把行摆在缺省序那一维的指定位置上。
func saveSynSuggestion(t *testing.T, stores decisionStores, key domain.InitialRouteJudgmentKey, correlation string, recordedAt time.Time) {
	t.Helper()
	id := scalar(t, domain.NewRequestCorrelationID, correlation)
	candidate, err := domain.NewRouteCandidate(scalar(t, domain.NewCandidateID, "SYN-LINE-A@1"), domain.CandidateQualified, domain.CandidateReason{})
	if err != nil {
		t.Fatalf("构造候选：%v", err)
	}
	reassessedAt := recordedAt.Add(-time.Minute)
	suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         key,
		Trigger:     scalar(t, domain.NewRerouteTriggerReference, correlation),
		Candidates:  []domain.RouteCandidate{candidate},
		Blockers:    []string{"IMPROVEMENT_BELOW_THRESHOLD"},
		SuggestedAt: reassessedAt,
	})
	if err != nil {
		t.Fatalf("构造建议：%v", err)
	}
	saveReassessment(t, stores.transactor, t.Context(), stores.reassessments, id, ports.ReassessmentRecord{
		Correlation:       id,
		Key:               key,
		Conclusion:        ports.ReassessmentPlanLapsed,
		ReviewedPlan:      scalar(t, domain.NewRoutePlanVersionID, "SYN-RPV-1"),
		LapseBasis:        scalar(t, domain.NewApplicabilityBasisReference, "SYN-LAPSE-1"),
		CandidateState:    ports.CandidatesAvailable,
		RerouteState:      domain.SuggestionOnly,
		RerouteBlockers:   []string{"IMPROVEMENT_BELOW_THRESHOLD"},
		Suggestion:        suggestion,
		HasSuggestion:     true,
		ReassessedAt:      reassessedAt,
		TriggerOccurredAt: recordedAt.Add(-time.Hour),
	})
	tag, err := stores.pool.Exec(t.Context(),
		`UPDATE network_routing.route_reassessment SET recorded_at = $3 WHERE tenant_id = $1 AND correlation_id = $2`,
		key.TenantID.String(), correlation, recordedAt)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("挪落册时刻 %s：%v（%d 行）", correlation, err, tag.RowsAffected())
	}
}

// newTiePagingFixture 建一只空库：未形成登记的写口、并列读面，以及直插判断库行用的池。
func newTiePagingFixture(t *testing.T) (*adapter.InitialRoutes, *adapter.InitialRouteTieDecisions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	routes, err := adapter.NewInitialRoutes(db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	ties, err := adapter.NewInitialRouteTieDecisions(db)
	if err != nil {
		t.Fatalf("构造并列读面：%v", err)
	}
	return routes, ties, db.Transactor(), pool
}

// recordSynTie 在 key 上记一行停在并列的未形成登记，最近一次停住的时刻由调用方给。
func recordSynTie(t *testing.T, routes *adapter.InitialRoutes, transactor bentoapp.Transactor, key domain.InitialRouteJudgmentKey, settledAt time.Time) {
	t.Helper()
	record := tiedNotFormed(t, key)
	segments := keyRowOf(key)
	record.Correlation = scalar(t, domain.NewRequestCorrelationID, "SYN-TIE/"+strings.Join(segments[1:], "/"))
	record.SettledAt = settledAt
	within(t, transactor, t.Context(), func(txCtx context.Context) error { return routes.RecordNotFormed(txCtx, record) })
}

// pagedRouteRegister 把一册的落行与上列包成同一形状，「翻页不重不漏」才能按册各跑一遍。
type pagedRouteRegister struct {
	catalogue *cataloguepage.Catalogue
	// open 建一只空库，交回落行与上列两个口：落行给一个合成名与这一行在缺省序那一维上的时刻；上列交回各行的合成名、下一游标与
	// 总数。合成名在初始路由册与并列册是包裹，在复核册与建议册是触发关联。
	open func(t *testing.T) (seed func(name string, at time.Time), list func(limit int, query cataloguepage.Query) ([]string, string, int64))
}

func pagedRouteRegisters() map[string]pagedRouteRegister {
	return map[string]pagedRouteRegister{
		"initial-route": {
			catalogue: ports.InitialRoutePages,
			open: func(t *testing.T) (func(string, time.Time), func(int, cataloguepage.Query) ([]string, string, int64)) {
				catalogue, pool := newRoutePlanCatalogue(t)
				seed := func(parcel string, at time.Time) {
					insertJudgmentRow(t, pool, t.Context(), keyRowOf(parcelKey(t, parcel)), "", at)
				}
				list := func(limit int, query cataloguepage.Query) ([]string, string, int64) {
					page, err := catalogue.ListInitialRoutes(t.Context(), pagingTenantID(t), limit, query)
					if err != nil {
						t.Fatalf("上列：%v", err)
					}
					return namesOf(page, func(row ports.InitialRouteCatalogueRow) string { return row.DeclaredParcelID })
				}
				return seed, list
			},
		},
		"reassessment": {
			catalogue: ports.RouteReassessmentPages,
			open: func(t *testing.T) (func(string, time.Time), func(int, cataloguepage.Query) ([]string, string, int64)) {
				catalogue, pool := newRoutePlanCatalogue(t)
				seed := func(correlation string, at time.Time) {
					insertReassessmentRowOn(t, pool, t.Context(), keyRowOf(parcelKey(t, "SYN-PARCEL-1")),
						correlation, "STILL_APPLICABLE", "SYN-PLAN-1#v1", "", "", at)
				}
				list := func(limit int, query cataloguepage.Query) ([]string, string, int64) {
					page, err := catalogue.ListRouteReassessments(t.Context(), pagingTenantID(t), limit, query)
					if err != nil {
						t.Fatalf("上列：%v", err)
					}
					return namesOf(page, func(row ports.RouteReassessmentCatalogueRow) string { return row.CorrelationID })
				}
				return seed, list
			},
		},
		"reroute-suggestions": {
			catalogue: ports.RerouteSuggestionPages,
			open: func(t *testing.T) (func(string, time.Time), func(int, cataloguepage.Query) ([]string, string, int64)) {
				stores := newDecisionStores(t)
				seed := func(correlation string, at time.Time) {
					saveSynSuggestion(t, stores, parcelKey(t, "SYN-PARCEL-"+correlation), correlation, at)
				}
				list := func(limit int, query cataloguepage.Query) ([]string, string, int64) {
					page, err := stores.catalogue.ListRerouteSuggestions(t.Context(), pagingTenantID(t), limit, query)
					if err != nil {
						t.Fatalf("上列：%v", err)
					}
					return namesOf(page, func(row ports.RerouteSuggestionCatalogueRow) string { return row.CorrelationID })
				}
				return seed, list
			},
		},
		"initial-route-ties": {
			catalogue: ports.InitialRouteTiePages,
			open: func(t *testing.T) (func(string, time.Time), func(int, cataloguepage.Query) ([]string, string, int64)) {
				routes, ties, transactor, _ := newTiePagingFixture(t)
				seed := func(parcel string, at time.Time) {
					recordSynTie(t, routes, transactor, parcelKey(t, parcel), at)
				}
				list := func(limit int, query cataloguepage.Query) ([]string, string, int64) {
					page, err := ties.ListInitialRouteTies(t.Context(), pagingTenantID(t), limit, query)
					if err != nil {
						t.Fatalf("上列：%v", err)
					}
					return namesOf(page, func(row ports.InitialRouteTieCatalogueRow) string { return row.DeclaredParcelID })
				}
				return seed, list
			},
		},
	}
}

// Covers: spec 完成判据「每册翻页期间插入新行，已翻过的页不重出、未翻的页不漏行；空册答空页且 total 为 0」（ADR-0144 决定一、五）
// ——空册先答空页；落五行后缺省序下两行一页地翻，翻完第一页再落两行：一行排在已翻过的区间，一行排在未翻的区间。已翻过的页不重出，
// 未翻的页不漏行、新落的那行在后面的页里出现；total 随落行变化，末页 next 为空。每册各跑一遍。
func TestEachRouteRegisterPagesWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	for name, register := range pagedRouteRegisters() {
		t.Run(name, func(t *testing.T) {
			seed, list := register.open(t)
			if names, next, total := list(2, firstPage(t, register.catalogue)); len(names) != 0 || next != "" || total != 0 {
				t.Fatalf("空册：%v next=%q total=%d", names, next, total)
			}

			for index, row := range []string{"SYN-A", "SYN-B", "SYN-C", "SYN-D", "SYN-E"} {
				seed(row, pagingBaseAt.Add(time.Duration(index)*time.Hour))
			}
			collected, next, total := list(2, firstPage(t, register.catalogue))
			if !reflect.DeepEqual(collected, []string{"SYN-E", "SYN-D"}) || next == "" || total != 5 {
				t.Fatalf("第一页：%v next=%q total=%d", collected, next, total)
			}

			seed("SYN-F", pagingBaseAt.Add(5*time.Hour))
			seed("SYN-0", pagingBaseAt.Add(-time.Hour))

			for pages := 1; next != ""; pages++ {
				if pages > 10 {
					t.Fatal("翻页停不下来")
				}
				var names []string
				names, next, total = list(2, nextPage(t, register.catalogue, nil, next))
				if total != 7 {
					t.Fatalf("落行之后 total 应为 7，得 %d", total)
				}
				collected = append(collected, names...)
			}
			want := []string{"SYN-E", "SYN-D", "SYN-C", "SYN-B", "SYN-A", "SYN-0"}
			if !reflect.DeepEqual(collected, want) {
				t.Fatalf("翻完得 %v，应为 %v", collected, want)
			}
		})
	}
}

// tiedOnTheSortValue 是同一时刻落下的一组判断键：头一条之外，每条只在一段上与它不同，租户以外的每一段都各有一条。
var tiedOnTheSortValue = [][5]string{
	{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1", "SYN-PARCEL-1", "SYN-PURPOSE-A"},
	{"SYN-ACC-2", "SYN-REQ-1", "SYN-BASELINE-1", "SYN-PARCEL-1", "SYN-PURPOSE-A"},
	{"SYN-ACC-1", "SYN-REQ-2", "SYN-BASELINE-1", "SYN-PARCEL-1", "SYN-PURPOSE-A"},
	{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-2", "SYN-PARCEL-1", "SYN-PURPOSE-A"},
	{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1", "SYN-PARCEL-2", "SYN-PURPOSE-A"},
	{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1", "SYN-PARCEL-1", "SYN-PURPOSE-B"},
}

// collectOneByOne 一行一页地翻完整册，每页都核一次 total。
func collectOneByOne(t *testing.T, catalogue *cataloguepage.Catalogue, total int64, list func(cataloguepage.Query) ([]string, string, int64)) []string {
	t.Helper()
	var collected []string
	query := firstPage(t, catalogue)
	for pages := 0; ; pages++ {
		if pages > len(tiedOnTheSortValue)+2 {
			t.Fatal("翻页停不下来")
		}
		names, next, got := list(query)
		if got != total {
			t.Fatalf("total 应为 %d，得 %d", total, got)
		}
		collected = append(collected, names...)
		if next == "" {
			return collected
		}
		query = nextPage(t, catalogue, nil, next)
	}
}

// Covers: 本票「组合键各段都进行标识」（ADR-0144 决定三）——落册时刻相同的一组行，两两只差判断键的某一段，每一段各有一对；一行一页
// 地翻，每行恰好出一次，次序是各段同向倒序：少了哪一段，只差那一段的两行就会在游标处重出或漏掉。初始路由按客户账户打头的段序。
func TestInitialRoutesTiedOnTheRecordedTimeAreSeparatedByEveryKeySegment(t *testing.T) {
	catalogue, pool := newRoutePlanCatalogue(t)
	for _, segments := range tiedOnTheSortValue {
		insertJudgmentRow(t, pool, t.Context(), keyRowOf(synJudgmentKey(t, segments)), "", pagingBaseAt)
	}

	collected := collectOneByOne(t, ports.InitialRoutePages, int64(len(tiedOnTheSortValue)),
		func(query cataloguepage.Query) ([]string, string, int64) {
			page, err := catalogue.ListInitialRoutes(t.Context(), pagingTenantID(t), 1, query)
			if err != nil {
				t.Fatalf("上列：%v", err)
			}
			return namesOf(page, func(row ports.InitialRouteCatalogueRow) string {
				return keyText(row.CustomerAccountID, row.ShipmentRequestID, row.AcceptanceBaseline, row.DeclaredParcelID, row.ServicePurpose)
			})
		})
	want := []string{
		"SYN-ACC-2/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-2/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-2/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-2/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-B",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
	}
	if !reflect.DeepEqual(collected, want) {
		t.Fatalf("翻完得 %v，应为 %v", collected, want)
	}
}

// Covers: 同上，并列册——停住时刻相同的这组行逐行翻完不重不漏；段序照迁移前的决胜次序以包裹打头，所以包裹那一对排在最前。
func TestInitialRouteTiesSettledAtTheSameTimeAreSeparatedByEveryKeySegment(t *testing.T) {
	routes, ties, transactor, _ := newTiePagingFixture(t)
	for _, segments := range tiedOnTheSortValue {
		recordSynTie(t, routes, transactor, synJudgmentKey(t, segments), pagingBaseAt)
	}

	collected := collectOneByOne(t, ports.InitialRouteTiePages, int64(len(tiedOnTheSortValue)),
		func(query cataloguepage.Query) ([]string, string, int64) {
			page, err := ties.ListInitialRouteTies(t.Context(), pagingTenantID(t), 1, query)
			if err != nil {
				t.Fatalf("上列：%v", err)
			}
			return namesOf(page, func(row ports.InitialRouteTieCatalogueRow) string {
				return keyText(row.CustomerAccountID, row.ShipmentRequestID, row.AcceptanceBaseline, row.DeclaredParcelID, row.ServicePurpose)
			})
		})
	want := []string{
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-2/SYN-PURPOSE-A",
		"SYN-ACC-2/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-2/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-2/SYN-PARCEL-1/SYN-PURPOSE-A",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-B",
		"SYN-ACC-1/SYN-REQ-1/SYN-BASELINE-1/SYN-PARCEL-1/SYN-PURPOSE-A",
	}
	if !reflect.DeepEqual(collected, want) {
		t.Fatalf("翻完得 %v，应为 %v", collected, want)
	}
}

// narrowingCase 是一组查询条件与它在本册上应答出的行（缺省序下），total 应等于行数，一页装得下、next 为空。
type narrowingCase struct {
	values url.Values
	want   []string
}

func assertNarrowing(
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	cases map[string]narrowingCase,
	list func(cataloguepage.Query) ([]string, string, int64),
) {
	t.Helper()
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			names, next, total := list(decodeWith(t, catalogue, test.values))
			if !reflect.DeepEqual(names, test.want) || next != "" || total != int64(len(test.want)) {
				t.Fatalf("得 %v total=%d next=%q，应为 %v", names, total, next, test.want)
			}
		})
	}
}

// Covers: ADR-0144 决定四、五——初始路由册：结论维内为或，q 在判断键的标识列与计划版本上做不分大小写的字面包含匹配，服务目的
// 不进 q，% 按字面算；total 与本页出自同一组条件。
func TestInitialRouteFiltersAndKeywordNarrowTheRowsAndTheTotal(t *testing.T) {
	catalogue, pool := newRoutePlanCatalogue(t)
	ctx := t.Context()
	for _, row := range []struct {
		request, parcel, plan string
		at                    time.Time
	}{
		{"SYN-REQ-1", "SYN-PARCEL-1", "SYN-PLAN-1#v1", pagingBaseAt},
		{"SYN-REQ-2", "SYN-PARCEL-2", "", pagingBaseAt.Add(time.Hour)},
		{"SYN-REQ-3", "SYN-PARCEL-3", "SYN-PLAN-3#v1", pagingBaseAt.Add(2 * time.Hour)},
	} {
		key := synJudgmentKey(t, [5]string{"SYN-ACC-1", row.request, "SYN-BASELINE-1", row.parcel, "SYN-PURPOSE-1"})
		insertJudgmentRow(t, pool, ctx, keyRowOf(key), row.plan, row.at)
	}

	all := []string{"SYN-PARCEL-3", "SYN-PARCEL-2", "SYN-PARCEL-1"}
	assertNarrowing(t, ports.InitialRoutePages, map[string]narrowingCase{
		"不筛":          {url.Values{}, all},
		"一维一值":        {url.Values{"conclusion": {"ROUTE_FORMED"}}, []string{"SYN-PARCEL-3", "SYN-PARCEL-1"}},
		"维内为或":        {url.Values{"conclusion": {"ROUTE_FORMED", "NO_CURRENT_ROUTE"}}, all},
		"q 不分大小写命中计划": {url.Values{"q": {"syn-plan-3"}}, []string{"SYN-PARCEL-3"}},
		"q 命中委托":      {url.Values{"q": {"SYN-REQ-2"}}, []string{"SYN-PARCEL-2"}},
		"q 与筛选同时":     {url.Values{"q": {"SYN-PARCEL"}, "conclusion": {"NO_CURRENT_ROUTE"}}, []string{"SYN-PARCEL-2"}},
		"服务目的不进 q":    {url.Values{"q": {"SYN-PURPOSE"}}, []string{}},
		"q 的百分号按字面算":  {url.Values{"q": {"%"}}, []string{}},
	}, func(query cataloguepage.Query) ([]string, string, int64) {
		page, err := catalogue.ListInitialRoutes(ctx, pagingTenantID(t), 10, query)
		if err != nil {
			t.Fatalf("上列：%v", err)
		}
		return namesOf(page, func(row ports.InitialRouteCatalogueRow) string { return row.DeclaredParcelID })
	})
}

// Covers: ADR-0144 决定四、五——复核册：结论维内为或，q 覆盖触发关联与被复核计划、服务目的不进 q；触发关联里真带着 % 的那一行只被
// 字面的 % 命中。
func TestRouteReassessmentFiltersAndKeywordNarrowTheRowsAndTheTotal(t *testing.T) {
	catalogue, pool := newRoutePlanCatalogue(t)
	ctx := t.Context()
	key := keyRowOf(parcelKey(t, "SYN-PARCEL-1"))
	insertReassessmentRowOn(t, pool, ctx, key, "SYN-CORR-1", "STILL_APPLICABLE", "SYN-PLAN-1#v1", "", "", pagingBaseAt)
	insertReassessmentRowOn(t, pool, ctx, key, "SYN-CORR-2", "PLAN_LAPSED",
		"SYN-PLAN-2#v1", "SYN-LAPSE/line-withdrawn", "NO_QUALIFIED_CANDIDATES", pagingBaseAt.Add(time.Hour))
	insertReassessmentRowOn(t, pool, ctx, key, "SYN-CORR-3", "STILL_APPLICABLE", "SYN-PLAN-3#v1", "", "", pagingBaseAt.Add(2*time.Hour))
	insertReassessmentRowOn(t, pool, ctx, key, "SYN-CORR-4%", "STILL_APPLICABLE", "SYN-PLAN-4#v1", "", "", pagingBaseAt.Add(3*time.Hour))

	all := []string{"SYN-CORR-4%", "SYN-CORR-3", "SYN-CORR-2", "SYN-CORR-1"}
	assertNarrowing(t, ports.RouteReassessmentPages, map[string]narrowingCase{
		"不筛":          {url.Values{}, all},
		"一维一值":        {url.Values{"conclusion": {"PLAN_LAPSED"}}, []string{"SYN-CORR-2"}},
		"维内为或":        {url.Values{"conclusion": {"PLAN_LAPSED", "STILL_APPLICABLE"}}, all},
		"词表内无行":       {url.Values{"conclusion": {"REROUTED"}}, []string{}},
		"q 不分大小写命中计划": {url.Values{"q": {"syn-plan-3"}}, []string{"SYN-CORR-3"}},
		"q 命中触发关联":    {url.Values{"q": {"CORR-1"}}, []string{"SYN-CORR-1"}},
		"q 与筛选同时":     {url.Values{"q": {"SYN-PLAN"}, "conclusion": {"PLAN_LAPSED"}}, []string{"SYN-CORR-2"}},
		"服务目的不进 q":    {url.Values{"q": {"SYN-PURPOSE"}}, []string{}},
		"q 的百分号按字面算":  {url.Values{"q": {"%"}}, []string{"SYN-CORR-4%"}},
	}, func(query cataloguepage.Query) ([]string, string, int64) {
		page, err := catalogue.ListRouteReassessments(ctx, pagingTenantID(t), 10, query)
		if err != nil {
			t.Fatalf("上列：%v", err)
		}
		return namesOf(page, func(row ports.RouteReassessmentCatalogueRow) string { return row.CorrelationID })
	})
}

// Covers: ADR-0144 决定四、五——建议册：状态是读面导出的一列，按它筛与按它答的是同一个判据；q 覆盖触发关联与判断键的标识列。
func TestRerouteSuggestionFiltersAndKeywordNarrowTheRowsAndTheTotal(t *testing.T) {
	stores := newDecisionStores(t)
	saveSynSuggestion(t, stores, parcelKey(t, "SYN-PARCEL-1"), "SYN-CORR-1", pagingBaseAt)
	superseded := parcelKey(t, "SYN-PARCEL-2")
	saveSynSuggestion(t, stores, superseded, "SYN-CORR-2", pagingBaseAt.Add(time.Hour))
	saveOtherReassessment(t, stores, superseded, "SYN-CORR-2-LATER", pagingBaseAt.Add(time.Hour))
	saveSynSuggestion(t, stores, parcelKey(t, "SYN-PARCEL-3"), "SYN-CORR-3", pagingBaseAt.Add(2*time.Hour))

	all := []string{"SYN-CORR-3", "SYN-CORR-2", "SYN-CORR-1"}
	assertNarrowing(t, ports.RerouteSuggestionPages, map[string]narrowingCase{
		"不筛":         {url.Values{}, all},
		"待决定":        {url.Values{"state": {"PENDING"}}, []string{"SYN-CORR-3", "SYN-CORR-1"}},
		"已过时":        {url.Values{"state": {"SUPERSEDED"}}, []string{"SYN-CORR-2"}},
		"维内为或":       {url.Values{"state": {"PENDING", "SUPERSEDED"}}, all},
		"词表内无行":      {url.Values{"state": {"DECIDED"}}, []string{}},
		"q 命中包裹":     {url.Values{"q": {"syn-parcel-2"}}, []string{"SYN-CORR-2"}},
		"q 命中触发关联":   {url.Values{"q": {"CORR-3"}}, []string{"SYN-CORR-3"}},
		"q 与筛选同时":    {url.Values{"q": {"SYN-PARCEL-1"}, "state": {"SUPERSEDED"}}, []string{}},
		"服务目的不进 q":   {url.Values{"q": {"SYN-PURPOSE"}}, []string{}},
		"q 的百分号按字面算": {url.Values{"q": {"%"}}, []string{}},
	}, func(query cataloguepage.Query) ([]string, string, int64) {
		page, err := stores.catalogue.ListRerouteSuggestions(t.Context(), pagingTenantID(t), 10, query)
		if err != nil {
			t.Fatalf("上列：%v", err)
		}
		return namesOf(page, func(row ports.RerouteSuggestionCatalogueRow) string { return row.CorrelationID })
	})
}

// Covers: ADR-0144 决定四、五——并列册：状态是读面导出的一列（判断库里已有这件包裹即另行形成）；q 覆盖判断键的标识列，服务目的不进 q。
func TestInitialRouteTieFiltersAndKeywordNarrowTheRowsAndTheTotal(t *testing.T) {
	routes, ties, transactor, pool := newTiePagingFixture(t)
	recordSynTie(t, routes, transactor, parcelKey(t, "SYN-PARCEL-1"), pagingBaseAt)
	otherwise := parcelKey(t, "SYN-PARCEL-2")
	recordSynTie(t, routes, transactor, otherwise, pagingBaseAt.Add(time.Hour))
	insertJudgmentRow(t, pool, t.Context(), keyRowOf(otherwise), "SYN-PLAN-2#v1", pagingBaseAt.Add(90*time.Minute))
	recordSynTie(t, routes, transactor, parcelKey(t, "SYN-PARCEL-3"), pagingBaseAt.Add(2*time.Hour))

	all := []string{"SYN-PARCEL-3", "SYN-PARCEL-2", "SYN-PARCEL-1"}
	assertNarrowing(t, ports.InitialRouteTiePages, map[string]narrowingCase{
		"不筛":         {url.Values{}, all},
		"待决定":        {url.Values{"state": {"PENDING"}}, []string{"SYN-PARCEL-3", "SYN-PARCEL-1"}},
		"另行形成":       {url.Values{"state": {"FORMED_OTHERWISE"}}, []string{"SYN-PARCEL-2"}},
		"维内为或":       {url.Values{"state": {"PENDING", "FORMED_OTHERWISE"}}, all},
		"词表内无行":      {url.Values{"state": {"DECIDED"}}, []string{}},
		"q 不分大小写":    {url.Values{"q": {"syn-parcel-2"}}, []string{"SYN-PARCEL-2"}},
		"q 与筛选同时":    {url.Values{"q": {"SYN-PARCEL-1"}, "state": {"FORMED_OTHERWISE"}}, []string{}},
		"服务目的不进 q":   {url.Values{"q": {"SYN-PURPOSE"}}, []string{}},
		"q 的百分号按字面算": {url.Values{"q": {"%"}}, []string{}},
	}, func(query cataloguepage.Query) ([]string, string, int64) {
		page, err := ties.ListInitialRouteTies(t.Context(), pagingTenantID(t), 10, query)
		if err != nil {
			t.Fatalf("上列：%v", err)
		}
		return namesOf(page, func(row ports.InitialRouteTieCatalogueRow) string { return row.DeclaredParcelID })
	})
}
