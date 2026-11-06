package pricinghttp_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对本上下文各目录端点（评价、价卡、参考序列、参考序列覆盖与价卡草稿查阅）证 ADR-0144 的传输半边（票
// catalogue-read-pagination-rollout/04，spec「端点先 Intake、后解码查询参数」）：查询参数按各册声明解成查询对象原样交给读口、
// 答复带 page 且 size 取 Intake 给的页大小；声明外的一问答 400 带理由散文、读口不被调用；渠道未配置时参数写对写错都答 403，
// 答复不随请求内容变（ADR-0055 决定三）。

// pagedCatalogue 是一个目录端点在本文件里用到的几样：怎么搭起来（配置了或未配置的渠道）、它的册声明、一条声明内的查询串与
// 读口该收到的条件、一个合法的游标位置，以及只在本册声明外的几问。
type pagedCatalogue struct {
	path      string
	catalogue *cataloguepage.Catalogue
	serve     func(configured bool, answer pageProbe) (http.Handler, *pageProbe)
	accepted  string
	sort      cataloguepage.Sort
	keyword   string
	filters   map[string][]string
	position  cataloguepage.Position
	outside   map[string]string
}

// catalogueIntake 是目录读口的两种渠道：配置了的交出 catalogueQuery 的作用域与页大小 50，未配置的对一切请求答 403。
func catalogueIntake(t *testing.T, configured bool) pricinghttp.PricingCatalogueIntake {
	t.Helper()
	if configured {
		return intakeDouble{query: catalogueQuery(t)}
	}
	return pricinghttp.UnconfiguredIntake{}
}

func pagedCatalogues(t *testing.T) []pagedCatalogue {
	t.Helper()
	tenant := catalogueQuery(t).Scope.Tenant()
	registeredAt := cataloguepage.FormatInstant(pricingBaseAt)
	return []pagedCatalogue{
		{
			path:      "/pricing-evaluations",
			catalogue: ports.EvaluationPages,
			serve: func(configured bool, answer pageProbe) (http.Handler, *pageProbe) {
				reader := &evaluationReaderDouble{pageProbe: answer, tenant: tenant}
				return pricinghttp.NewQueryEvaluationsEndpoint(catalogueIntake(t, configured), reader), &reader.pageProbe
			},
			accepted: "status=FAILED&status=CONFLICT&q=SYN-EVAL&sort=recordedAt",
			sort:     cataloguepage.Sort{Field: "recordedAt"},
			keyword:  "SYN-EVAL",
			filters:  map[string][]string{"status": {"CONFLICT", "FAILED"}},
			position: cataloguepage.Position{Value: registeredAt, Identity: []string{"SYN-EVAL-001"}},
			outside: map[string]string{
				"集外排序维":  "sort=evaluationId",
				"词表外筛选值": "status=ARCHIVED",
			},
		},
		{
			path:      "/pricing-price-cards",
			catalogue: ports.PriceCardPages,
			serve: func(configured bool, answer pageProbe) (http.Handler, *pageProbe) {
				reader := &cardReaderDouble{pageProbe: answer, tenant: tenant}
				return pricinghttp.NewQueryPriceCardsEndpoint(catalogueIntake(t, configured), reader), &reader.pageProbe
			},
			accepted: "direction=SELL&q=SYN-PLAN&sort=-planId",
			sort:     cataloguepage.Sort{Field: "planId", Descending: true},
			keyword:  "SYN-PLAN",
			filters:  map[string][]string{"direction": {"SELL"}},
			position: cataloguepage.Position{Value: registeredAt, Identity: []string{"SYN-PLAN-01", "v1"}},
			outside: map[string]string{
				"集外排序维":  "sort=effectiveFrom",
				"词表外筛选值": "direction=sell",
				"状态不是一维": "status=RETIRED",
			},
		},
		{
			path:      "/pricing-reference-series",
			catalogue: ports.ReferenceSeriesPages,
			serve: func(configured bool, answer pageProbe) (http.Handler, *pageProbe) {
				reader := &seriesReaderDouble{pageProbe: answer, tenant: tenant}
				return pricinghttp.NewQueryReferenceSeriesEndpoint(catalogueIntake(t, configured), reader), &reader.pageProbe
			},
			accepted: "kind=FUEL_RATE&seriesId=SYN-PRC-FUEL-WEEKLY&q=SYN&sort=seriesId",
			sort:     cataloguepage.Sort{Field: "seriesId"},
			keyword:  "SYN",
			filters:  map[string][]string{"kind": {"FUEL_RATE"}, "seriesId": {"SYN-PRC-FUEL-WEEKLY"}},
			position: cataloguepage.Position{Value: registeredAt, Identity: []string{"SYN-PRC-FUEL-WEEKLY", "v1"}},
			outside: map[string]string{
				"集外排序维":  "sort=kind",
				"词表外筛选值": "kind=fuel",
				"空筛选值":   "seriesId=",
			},
		},
		{
			path:      "/pricing-reference-series-coverage",
			catalogue: ports.ReferenceSeriesCoveragePages,
			serve: func(configured bool, answer pageProbe) (http.Handler, *pageProbe) {
				reader := &coverageReaderDouble{pageProbe: answer}
				return pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(catalogueIntake(t, configured), reader,
					coverageClockDouble{at: coverageAt}), &reader.pageProbe
			},
			accepted: "kind=EXCHANGE_RATE&q=SYN-PRC&sort=-seriesId",
			sort:     cataloguepage.Sort{Field: "seriesId", Descending: true},
			keyword:  "SYN-PRC",
			filters:  map[string][]string{"kind": {"EXCHANGE_RATE"}},
			position: cataloguepage.Position{Value: "SYN-PRC-FUEL-WEEKLY", Identity: []string{"SYN-PRC-FUEL-WEEKLY", "FUEL_RATE"}},
			outside: map[string]string{
				"集外排序维": "sort=registeredAt",
				"集外筛选维": "seriesId=SYN-PRC-FUEL-WEEKLY",
			},
		},
		{
			// 草稿查阅挂操作者登记 Intake。状态词迁移前由那个 Intake 解，小写与空值那时就答 400，挪进册声明后照旧；同一维给两次
			// 那时也答 400，现在维内取或（accepted 那一问）。
			path:      priceCardDraftViewsTarget,
			catalogue: ports.PriceCardDraftPages,
			serve: func(configured bool, answer pageProbe) (http.Handler, *pageProbe) {
				var intake pricinghttp.PriceCardDraftQueryIntake = pricinghttp.UnconfiguredIntake{}
				if configured {
					intake = priceCardDraftQueryIntakeDouble{query: pricinghttp.PriceCardDraftQuery{Tenant: tenant, Limit: 50}}
				}
				reader := &priceCardDraftReaderDouble{pageProbe: answer}
				return pricinghttp.NewQueryPriceCardDraftsEndpoint(intake, reader), &reader.pageProbe
			},
			accepted: "status=VALIDATED&status=DRAFT&q=SYN-CARD&sort=submittedAt",
			sort:     cataloguepage.Sort{Field: "submittedAt"},
			keyword:  "SYN-CARD",
			filters:  map[string][]string{"status": {"DRAFT", "VALIDATED"}},
			position: cataloguepage.Position{Value: registeredAt, Identity: []string{"SYN-PLAN-01", "v1"}},
			outside: map[string]string{
				"集外排序维":  "sort=planId",
				"词表外筛选值": "status=ARCHIVED",
				"小写状态词":  "status=draft",
				"空状态词":   "status=",
			},
		},
	}
}

func getPaged(endpoint http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// Covers: ADR-0144 决定五、六——各目录端点按自己的册声明解查询参数，查询对象原样交给读口；答复带 page（size 取 Intake
// 给的页大小，next 与 total 原样带出）；空页答总数 0、没有下一页。
func TestPricingCataloguesPassTheDecodedQueryAndCarryThePage(t *testing.T) {
	for _, paged := range pagedCatalogues(t) {
		t.Run(paged.path, func(t *testing.T) {
			endpoint, probe := paged.serve(true, pageProbe{next: "SYN-cursor", total: 7})
			recorder := getPaged(endpoint, paged.path+"?"+paged.accepted)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d；body = %s", recorder.Code, recorder.Body)
			}
			page := pageOf(t, recorder)
			if string(page["size"]) != "50" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
				t.Fatalf("page 不对：%s", recorder.Body.Bytes())
			}
			if probe.limit != 50 || probe.query.Sort != paged.sort || probe.query.Keyword != paged.keyword ||
				!reflect.DeepEqual(probe.query.Filters, paged.filters) || probe.query.After != nil {
				t.Fatalf("读口收到的查询不对：limit %d，%+v", probe.limit, probe.query)
			}

			endpoint, _ = paged.serve(true, pageProbe{})
			empty := getPaged(endpoint, paged.path)
			if page := pageOf(t, empty); string(page["total"]) != "0" || string(page["next"]) != "null" {
				t.Fatalf("空页的 page 不对：%s", empty.Body.Bytes())
			}
		})
	}
}

// Covers: ADR-0144 决定一——游标只在出它的那组条件下有效：同一组条件带上它，读口收到解出的位置；换了条件（这里换 q）就是
// 另一问，答 400 带决定一点名的那句理由，不从一个不属于它的位置接着翻。
func TestPricingCataloguesHonourACursorOnlyUnderItsOwnConditions(t *testing.T) {
	for _, paged := range pagedCatalogues(t) {
		t.Run(paged.path, func(t *testing.T) {
			issued, err := paged.catalogue.Decode(url.Values{"q": {"SYN-A"}})
			if err != nil {
				t.Fatal(err)
			}
			cursor, err := issued.CursorAfter(paged.position)
			if err != nil {
				t.Fatal(err)
			}

			endpoint, probe := paged.serve(true, pageProbe{})
			recorder := getPaged(endpoint, paged.path+"?q=SYN-A&after="+url.QueryEscape(cursor))
			if recorder.Code != http.StatusOK {
				t.Fatalf("同条件续翻 status = %d；body = %s", recorder.Code, recorder.Body)
			}
			if probe.query.After == nil || !reflect.DeepEqual(*probe.query.After, paged.position) {
				t.Fatalf("读口收到的位置 = %+v，想要 %+v", probe.query.After, paged.position)
			}

			endpoint, probe = paged.serve(true, pageProbe{})
			detail := assertPageQueryRefused(t, getPaged(endpoint, paged.path+"?q=SYN-B&after="+url.QueryEscape(cursor)))
			if detail != "游标与本次的排序或筛选不符，请从第一页重取" {
				t.Fatalf("理由散文：得 %q", detail)
			}
			if probe.called {
				t.Fatal("换了条件的游标不该走到读口")
			}
		})
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、集外的排序维、词表外或为空的筛选值、解不开的游标与坏转义都答 400 带理由散文，
// 理由点到那一格，读口不被调用。解码在 Intake 之后（spec「端点先 Intake、后解码查询参数」）：渠道未配置时，不带查询串、
// 合规的一问与上面每一个坏请求都答 403，答复逐字相同、读口不被调用（ADR-0055 决定三）。
func TestPricingCataloguesRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	everywhere := map[string]string{
		"自报租户":   "tenant=SYN-TENANT-02",
		"自定页大小":  "limit=1000",
		"排序给两次":  "sort=-registeredAt&sort=registeredAt",
		"解不开的游标": "after=SYN-not-a-cursor",
		"坏转义":    "q=%zz",
	}
	for _, paged := range pagedCatalogues(t) {
		for _, cases := range []map[string]string{everywhere, paged.outside} {
			for name, query := range cases {
				t.Run(paged.path+"/"+name, func(t *testing.T) {
					endpoint, probe := paged.serve(true, pageProbe{})
					assertPageQueryRefused(t, getPaged(endpoint, paged.path+"?"+query))
					if probe.called {
						t.Fatal("坏请求不该走到读口")
					}
				})
			}
		}

		t.Run(paged.path+"/理由点到那一格", func(t *testing.T) {
			endpoint, _ := paged.serve(true, pageProbe{})
			if detail := assertPageQueryRefused(t, getPaged(endpoint, paged.path+"?tenant=SYN-TENANT-02")); !strings.Contains(detail, "tenant") {
				t.Fatalf("理由散文应点到那一格：%q", detail)
			}
		})

		t.Run(paged.path+"/渠道未配置", func(t *testing.T) {
			endpoint, probe := paged.serve(false, pageProbe{})
			queries := []string{"", paged.accepted}
			for _, cases := range []map[string]string{everywhere, paged.outside} {
				for _, query := range cases {
					queries = append(queries, query)
				}
			}
			bare := ""
			for _, query := range queries {
				recorder := getPaged(endpoint, paged.path+"?"+query)
				if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
					t.Fatalf("%q：status = %d；body = %s", query, recorder.Code, recorder.Body)
				}
				if query == "" {
					bare = recorder.Body.String()
				} else if recorder.Body.String() != bare {
					t.Fatalf("未配置的渠道答复随查询串变了：%q 答 %s，不带查询串答 %s", query, recorder.Body, bare)
				}
			}
			if probe.called {
				t.Fatal("未配置的渠道走到了读口")
			}
		})
	}
}
