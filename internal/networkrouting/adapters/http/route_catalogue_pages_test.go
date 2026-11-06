package networkhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件钉路由判断两册、改路建议与初始路由并列四册的翻页传输面（ADR-0144 决定一、三至六）：先 Intake、后解码，未配置的渠道
// 对一切查询串同答 403；配置了的渠道上坏参数答 400 带理由散文、读口不被触到；解出的查询对象原样交给读口，答复带 page 三格，
// size 取 Intake 页大小。

// pagedRouteFace 把一册的请求目标、查询声明与端点装法包成同一形状，四册才能各过一遍同一组用例。
type pagedRouteFace struct {
	path      string
	selectors url.Values
	catalogue *cataloguepage.Catalogue
	sortField string
	// filter 是本册的筛选维，words 是它词表里的两个词，按升序给。
	filter string
	words  [2]string
	// last 是本册一行的位置，标识段数照声明。
	last cataloguepage.Position
	// sibling 是另一册的名字：拿它的游标来这一册翻页必须被拒。
	sibling string
	// mount 用给定的 Intake 装出端点，读口替身交回 next 与 total；asked 答读口收到的查询对象，没被问到答 false。
	mount func(intake networkhttp.CatalogueQueryIntake, next string, total int64) (endpoint http.Handler, asked func() (cataloguepage.Query, bool))
}

func pagedRouteFaces() map[string]pagedRouteFace {
	judgmentKey := []string{"SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1", "SYN-PARCEL-1", "SYN-PURPOSE-1"}
	routePlans := func(intake networkhttp.CatalogueQueryIntake, next string, total int64) (http.Handler, func() (cataloguepage.Query, bool)) {
		reader := &stubRoutePlanReader{next: next, total: total}
		return networkhttp.NewQueryRoutePlansEndpoint(intake, reader), func() (cataloguepage.Query, bool) { return reader.gotQuery, reader.called }
	}
	return map[string]pagedRouteFace{
		"initial-route": {
			path: "/route-plans", selectors: url.Values{"register": {"initial-route"}},
			catalogue: ports.InitialRoutePages, sortField: "recordedAt",
			filter: "conclusion", words: [2]string{"NO_CURRENT_ROUTE", "ROUTE_FORMED"},
			last:    cataloguepage.Position{Value: "2026-08-20T12:00:00.123456Z", Identity: judgmentKey},
			sibling: "initial-route-ties",
			mount:   routePlans,
		},
		"reassessment": {
			path: "/route-plans", selectors: url.Values{"register": {"reassessment"}},
			catalogue: ports.RouteReassessmentPages, sortField: "recordedAt",
			filter: "conclusion", words: [2]string{"PLAN_LAPSED", "STILL_APPLICABLE"},
			last:    cataloguepage.Position{Value: "2026-08-20T12:00:00.123456Z", Identity: []string{"SYN-CORR-1"}},
			sibling: "reroute-suggestions",
			mount:   routePlans,
		},
		"reroute-suggestions": {
			path:      "/route-reroute-suggestions",
			catalogue: ports.RerouteSuggestionPages, sortField: "recordedAt",
			filter: "state", words: [2]string{"DECIDED", "PENDING"},
			last:    cataloguepage.Position{Value: "2026-08-20T12:00:00.123456Z", Identity: []string{"SYN-CORR-1"}},
			sibling: "reassessment",
			mount: func(intake networkhttp.CatalogueQueryIntake, next string, total int64) (http.Handler, func() (cataloguepage.Query, bool)) {
				reader := &suggestionReader{next: next, total: total}
				return networkhttp.NewQueryRerouteSuggestionsEndpoint(intake, reader), func() (cataloguepage.Query, bool) { return reader.query, reader.called }
			},
		},
		"initial-route-ties": {
			path:      "/route-initial-route-ties",
			catalogue: ports.InitialRouteTiePages, sortField: "settledAt",
			filter: "state", words: [2]string{"DECIDED", "FORMED_OTHERWISE"},
			last:    cataloguepage.Position{Value: "2026-08-20T12:00:00.123456Z", Identity: judgmentKey},
			sibling: "initial-route",
			mount: func(intake networkhttp.CatalogueQueryIntake, next string, total int64) (http.Handler, func() (cataloguepage.Query, bool)) {
				reader := &tieReader{next: next, total: total}
				return networkhttp.NewQueryInitialRouteTiesEndpoint(intake, reader), func() (cataloguepage.Query, bool) { return reader.query, reader.called }
			},
		},
	}
}

// valuesWith 把本册的选择器与本次的查询条件并成一份查询串。
func (face pagedRouteFace) valuesWith(conditions url.Values) url.Values {
	values := url.Values{}
	for key, given := range face.selectors {
		values[key] = append(values[key], given...)
	}
	for key, given := range conditions {
		values[key] = append(values[key], given...)
	}
	return values
}

func (face pagedRouteFace) target(conditions url.Values) string {
	values := face.valuesWith(conditions)
	if len(values) == 0 {
		return face.path
	}
	return face.path + "?" + values.Encode()
}

// cursorFor 照本册的声明在给定条件下编出 last 之后的游标，与读面编出来的同形。
func (face pagedRouteFace) cursorFor(t *testing.T, conditions url.Values) string {
	t.Helper()
	query, err := face.catalogue.Decode(face.valuesWith(conditions))
	if err != nil {
		t.Fatalf("解码 %v：%v", conditions, err)
	}
	cursor, err := query.CursorAfter(face.last)
	if err != nil {
		t.Fatalf("编游标：%v", err)
	}
	return cursor
}

func serveRoute(endpoint http.Handler, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

// routePageOf 读出 200 答复里的 page 一格，三个键都须在场：到了末页 next 是 null，不是缺席。
func routePageOf(t *testing.T, response *httptest.ResponseRecorder) cataloguepage.Page {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var slots struct {
		Page map[string]json.RawMessage `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &slots); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	for _, key := range []string{"size", "next", "total"} {
		if _, present := slots.Page[key]; !present {
			t.Fatalf("page 缺 %s：%s", key, response.Body.String())
		}
	}
	var body struct {
		Page cataloguepage.Page `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.Page
}

// Covers: ADR-0144 决定五——答复的 page 由读口的下一游标与总数拼成，size 回显 Intake 给的页大小；末页 next 为 null，
// 空册 total 为 0。四册各过一遍。
func TestEachRouteCatalogueAnswersThePageWithTheIntakeSize(t *testing.T) {
	for name, face := range pagedRouteFaces() {
		t.Run(name, func(t *testing.T) {
			intake := grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 25}
			endpoint, _ := face.mount(intake, "SYN-NEXT", 42)
			page := routePageOf(t, serveRoute(endpoint, face.target(nil)))
			if page.Size != 25 || page.Next == nil || *page.Next != "SYN-NEXT" || page.Total == nil || *page.Total != 42 {
				t.Fatalf("page 走样：%+v", page)
			}

			endpoint, _ = face.mount(intake, "", 0)
			page = routePageOf(t, serveRoute(endpoint, face.target(nil)))
			if page.Size != 25 || page.Next != nil || page.Total == nil || *page.Total != 0 {
				t.Fatalf("空册的 page 走样：%+v", page)
			}
		})
	}
}

// Covers: ADR-0144 决定六、一——端点按本册声明解出查询对象原样交给读口：换了方向的排序、维内多值的筛选、去掉首尾空白的 q；
// 把游标带回 after，读口收到的就是上一页末行的位置。四册各过一遍。
func TestEachRouteCatalogueHandsTheDecodedQueryToTheReader(t *testing.T) {
	for name, face := range pagedRouteFaces() {
		t.Run(name, func(t *testing.T) {
			endpoint, asked := face.mount(grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 25}, "", 0)
			conditions := url.Values{
				"sort":      {face.sortField},
				face.filter: {face.words[1], face.words[0]},
				"q":         {" SYN-PARCEL "},
			}
			if response := serveRoute(endpoint, face.target(conditions)); response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			got, called := asked()
			if !called {
				t.Fatal("读口没被问到")
			}
			if got.Sort != (cataloguepage.Sort{Field: face.sortField}) {
				t.Fatalf("排序走样：%+v", got.Sort)
			}
			if !reflect.DeepEqual(got.Filters[face.filter], face.words[:]) {
				t.Fatalf("筛选走样：%v", got.Filters)
			}
			if got.Keyword != "SYN-PARCEL" || got.After != nil {
				t.Fatalf("q 或游标走样：%+v", got)
			}

			conditions.Set("after", face.cursorFor(t, conditions))
			if response := serveRoute(endpoint, face.target(conditions)); response.Code != http.StatusOK {
				t.Fatalf("带回游标：status = %d body = %s", response.Code, response.Body.String())
			}
			if got, _ := asked(); got.After == nil || !reflect.DeepEqual(*got.After, face.last) {
				t.Fatalf("读口收到的位置：%+v，应为 %+v", got.After, face.last)
			}
		})
	}
}

// Covers: ADR-0144 决定三、四、一——配置了的渠道上，集外键（含从前被忽略的自报租户与页大小）、集外排序维、词表外筛选值、换了条件
// 或来自别册的游标、超长的 q，各答 400 MALFORMED_REQUEST，detail 带理由散文、不带 outcome；读口不被触到。四册各过一遍。
func TestEachRouteCatalogueRefusesMalformedParametersWithAReason(t *testing.T) {
	faces := pagedRouteFaces()
	for name, face := range faces {
		t.Run(name, func(t *testing.T) {
			endpoint, asked := face.mount(grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 25}, "", 0)
			cases := map[string]url.Values{
				"自报租户":     {"tenant": {"SYN-TENANT-09"}},
				"自报页大小":    {"limit": {"9999"}},
				"集外排序维":    {"sort": {"-declaredParcelId"}},
				"词表外筛选值":   {face.filter: {"SYN-NOT-A-WORD"}},
				"换条件后的旧游标": {"after": {face.cursorFor(t, url.Values{face.filter: {face.words[0]}})}},
				"别册的游标":    {"after": {faces[face.sibling].cursorFor(t, nil)}},
				"超长的 q":    {"q": {strings.Repeat("S", cataloguepage.MaxKeywordRunes+1)}},
			}
			for caseName, conditions := range cases {
				response := serveRoute(endpoint, face.target(conditions))
				if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
					t.Fatalf("%s：%d %s", caseName, response.Code, response.Body.String())
				}
				if problemDetailText(t, response) == "" {
					t.Fatalf("%s：400 没带理由散文：%s", caseName, response.Body.String())
				}
				assertNoOutcome(t, response)
			}
			if _, called := asked(); called {
				t.Fatal("坏参数问到了读口")
			}
		})
	}
}

// Covers: spec 取舍「端点先 Intake、后解码」（ADR-0055 决定三）——渠道未配置时，参数写对写错都答同一个 403
// ACCESS_CHANNEL_NOT_CONFIGURED，答复不随查询串变；/route-plans 缺 register 或取值不在集内也一样。四册各过一遍。
func TestEachRouteCatalogueAnswersForbiddenWhateverTheQueryWhenUnconfigured(t *testing.T) {
	for name, face := range pagedRouteFaces() {
		t.Run(name, func(t *testing.T) {
			endpoint, asked := face.mount(networkhttp.UnconfiguredIntake{}, "", 0)
			targets := []string{
				face.target(nil),
				face.target(url.Values{"sort": {face.sortField}, face.filter: {face.words[0]}, "q": {"SYN"}}),
				face.target(url.Values{"after": {face.cursorFor(t, nil)}}),
				face.target(url.Values{"tenant": {"SYN-TENANT-09"}}),
				face.target(url.Values{"sort": {"-declaredParcelId"}}),
				face.target(url.Values{face.filter: {"SYN-NOT-A-WORD"}}),
				face.target(url.Values{"after": {"SYN-NOT-A-CURSOR"}}),
			}
			if face.path == "/route-plans" {
				targets = append(targets, "/route-plans", "/route-plans?register=SYN-NOT-A-REGISTER")
			}
			var first string
			for _, target := range targets {
				response := serveRoute(endpoint, target)
				if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
					t.Fatalf("%s：%d %s，want 403 渠道未配置", target, response.Code, response.Body.String())
				}
				if first == "" {
					first = response.Body.String()
				} else if response.Body.String() != first {
					t.Fatalf("%s：答复随查询串变了：%s，头一问是 %s", target, response.Body.String(), first)
				}
			}
			if _, called := asked(); called {
				t.Fatal("未配置的渠道问到了读口")
			}
		})
	}
}
