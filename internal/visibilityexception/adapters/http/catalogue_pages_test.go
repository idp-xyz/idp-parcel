package visibilityhttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

const (
	pagedCursor = "SYN-cursor"
	pagedTotal  = 7
)

// listProbe 打一次某册的列表端点：extra 接在选择器（有的话）之后；交回答复与读口收到的查询对象，读口没被调用时
// called 为 false。读口替身在行之外带上 pagedCursor 与 pagedTotal，好看 page 是否原样带出。
type listProbe func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
	response *httptest.ResponseRecorder, query cataloguepage.Query, called bool)

// pagedList 是一册列表端点在本文件里的全部用例输入。
type pagedList struct {
	name      string
	catalogue *cataloguepage.Catalogue
	probe     listProbe
	// selector 是 probe 替调用方带上的选择器；选择器取值进游标摘要，编旧游标时要一并带上。
	selector url.Values
	// good 是声明内的排序、筛选与 q 组合，wantSort、wantFilters 是它解出来应得的。
	good        string
	wantSort    cataloguepage.Sort
	wantFilters map[string][]string
	// badFilter 是词表外的筛选值；该册不开筛选维时为空。
	badFilter string
	// last 是一个合本册缺省序与标识形状的末行位置，用来编一枚旧游标。
	last cataloguepage.Position
}

func withQuery(path, extra string) string {
	switch {
	case extra == "":
		return path
	case strings.Contains(path, "?"):
		return path + "&" + extra
	default:
		return path + "?" + extra
	}
}

func trackingListProbe(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
	*httptest.ResponseRecorder, cataloguepage.Query, bool) {
	t.Helper()
	reader := opsReader(t, operationsProjection(t))
	reader.next, reader.total = pagedCursor, pagedTotal
	response := opsServe(t, intake, reader, http.MethodGet, withQuery("/tracking-projections", extra))
	return response, reader.listQuery, reader.listCalled
}

func caseListProbe(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
	*httptest.ResponseRecorder, cataloguepage.Query, bool) {
	t.Helper()
	reader := &caseReaderDouble{next: pagedCursor, total: pagedTotal}
	response := ecServe(t, intake, reader, http.MethodGet, withQuery("/exception-case-records", extra))
	return response, reader.query, reader.called
}

func triageListProbe(registry string) listProbe {
	return func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
		*httptest.ResponseRecorder, cataloguepage.Query, bool) {
		t.Helper()
		reader := &triageReaderDouble{next: pagedCursor, total: pagedTotal}
		response := etServe(t, intake, reader, http.MethodGet,
			withQuery("/exception-triage-records?registry="+registry, extra))
		return response, reader.query, reader.called
	}
}

func claimsListProbe(registry string) listProbe {
	return func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
		*httptest.ResponseRecorder, cataloguepage.Query, bool) {
		t.Helper()
		reader := &claimsReaderDouble{next: pagedCursor, total: pagedTotal}
		response := crServe(t, intake, reader, http.MethodGet,
			withQuery("/claims-recovery-records?registry="+registry, extra))
		return response, reader.query, reader.called
	}
}

func catalogueListProbe(kind string) listProbe {
	return func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, extra string) (
		*httptest.ResponseRecorder, cataloguepage.Query, bool) {
		t.Helper()
		reader := &catalogueReaderDouble{next: pagedCursor, total: pagedTotal}
		response := vcServe(t, intake, reader, http.MethodGet,
			withQuery("/visibility-catalogues?kind="+kind, extra))
		return response, reader.query, reader.called
	}
}

// pagedLists 列出本上下文迁到 ADR-0144 的各册：按选择器分派的路由每个取值各是一册。
func pagedLists() []pagedList {
	at := cataloguepage.FormatInstant(time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC))
	noFilters := map[string][]string{}
	versioned := func(kind string, catalogue *cataloguepage.Catalogue) pagedList {
		return pagedList{
			name: kind, catalogue: catalogue, probe: catalogueListProbe(kind), selector: url.Values{"kind": {kind}},
			good: "sort=effectiveFrom&q=SYN", wantSort: cataloguepage.Sort{Field: "effectiveFrom"}, wantFilters: noFilters,
			last: cataloguepage.Position{Value: at, Identity: []string{"SYN-VERSION-1"}},
		}
	}
	keyed := func(kind string, catalogue *cataloguepage.Catalogue, key string) pagedList {
		return pagedList{
			name: kind, catalogue: catalogue, probe: catalogueListProbe(kind), selector: url.Values{"kind": {kind}},
			good: "sort=-" + key + "&q=SYN", wantSort: cataloguepage.Sort{Field: key, Descending: true}, wantFilters: noFilters,
			last: cataloguepage.Position{Value: "SYN-KEY-1", Identity: []string{"SYN-KEY-1"}},
		}
	}
	return []pagedList{
		{
			name: "tracking-projections", catalogue: ports.TrackingProjectionPages, probe: trackingListProbe,
			good: "sort=parcel&q=SYN", wantSort: cataloguepage.Sort{Field: "parcel"}, wantFilters: noFilters,
			last: cataloguepage.Position{Value: at, Identity: []string{"SYN-PARCEL-1"}},
		},
		{
			name: "exception-case-records", catalogue: ports.ExceptionCasePages, probe: caseListProbe,
			good:        "sort=-rootParcel&phase=IN_PROGRESS&phase=AWAITING_RESPONSE&q=SYN",
			wantSort:    cataloguepage.Sort{Field: "rootParcel", Descending: true},
			wantFilters: map[string][]string{"phase": {"AWAITING_RESPONSE", "IN_PROGRESS"}},
			badFilter:   "phase=SYN-ARCHIVED",
			last:        cataloguepage.Position{Value: at, Identity: []string{"SYN-CASE-1"}},
		},
		{
			name: "signal-episode", catalogue: ports.SignalEpisodePages, probe: triageListProbe("signal-episode"),
			selector:    url.Values{"registry": {"signal-episode"}},
			good:        "sort=lastHitAt&outcome=MANUAL_REVIEW&q=SYN",
			wantSort:    cataloguepage.Sort{Field: "lastHitAt"},
			wantFilters: map[string][]string{"outcome": {"MANUAL_REVIEW"}},
			badFilter:   "outcome=SYN-PENDING",
			last:        cataloguepage.Position{Value: at, Identity: []string{"SYN-EPISODE-1"}},
		},
		{
			name: "disposition-request", catalogue: ports.DispositionRequestPages, probe: triageListProbe("disposition-request"),
			selector:    url.Values{"registry": {"disposition-request"}},
			good:        "sort=sentAt&judgment=REFUSED&judgment=ACCEPTED&q=SYN",
			wantSort:    cataloguepage.Sort{Field: "sentAt"},
			wantFilters: map[string][]string{"judgment": {"ACCEPTED", "REFUSED"}},
			badFilter:   "judgment=SYN-UNANSWERED",
			last:        cataloguepage.Position{Value: at, Identity: []string{"SYN-REQUEST-1"}},
		},
		{
			name: "customer-notification", catalogue: ports.CustomerNotificationPages,
			probe:    claimsListProbe("customer-notification"),
			selector: url.Values{"registry": {"customer-notification"}},
			good:     "sort=decidedAt&q=SYN", wantSort: cataloguepage.Sort{Field: "decidedAt"}, wantFilters: noFilters,
			last: cataloguepage.Position{Value: at, Identity: []string{"SYN-NOTICE-1"}},
		},
		{
			name: "claim-item", catalogue: ports.ClaimItemPages, probe: claimsListProbe("claim-item"),
			selector:    url.Values{"registry": {"claim-item"}},
			good:        "sort=submittedAt&screen=ELIGIBLE&conclusion=NOT_ESTABLISHED&q=SYN",
			wantSort:    cataloguepage.Sort{Field: "submittedAt"},
			wantFilters: map[string][]string{"screen": {"ELIGIBLE"}, "conclusion": {"NOT_ESTABLISHED"}},
			badFilter:   "conclusion=SYN-SETTLED",
			last:        cataloguepage.Position{Value: at, Identity: []string{"SYN-BATCH-1", "SYN-ITEM-1"}},
		},
		{
			name: "recovery-matter", catalogue: ports.RecoveryMatterPages, probe: claimsListProbe("recovery-matter"),
			selector: url.Values{"registry": {"recovery-matter"}},
			good:     "sort=openedAt&q=SYN", wantSort: cataloguepage.Sort{Field: "openedAt"}, wantFilters: noFilters,
			last: cataloguepage.Position{Value: at, Identity: []string{"SYN-MATTER-1"}},
		},
		versioned("MILESTONE_MAPPING", ports.MilestoneMappingCataloguePages),
		versioned("TRIAGE_RULE", ports.TriageRuleCataloguePages),
		keyed("NOTIFICATION_POLICY", ports.NotificationPolicyCataloguePages, "policy"),
		keyed("CLAIM_ELIGIBILITY", ports.ClaimEligibilityCataloguePages, "contract"),
		keyed("CLAIM_AUTHORIZATION", ports.ClaimAuthorizationCataloguePages, "customer"),
		versioned("DISCLOSURE_POLICY", ports.DisclosurePolicyCataloguePages),
		versioned("EXCEPTION_DISCLOSURE_RULE", ports.ExceptionDisclosureRuleCataloguePages),
		keyed("CONFLICT_SIGNAL_RULE", ports.ConflictSignalRuleCataloguePages, "signalKind"),
	}
}

// Covers: ADR-0144 决定五、六——各册的列表端点各过一遍：答复带 page（size 取 Intake 给的页大小，next 与 total 原样
// 带出）；查询参数解成查询对象原样交给读口，选择器只选册、不进筛选。
func TestListEndpointsCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	for _, list := range pagedLists() {
		t.Run(list.name, func(t *testing.T) {
			response, query, called := list.probe(t, operationsIntakeDouble{query: operationsScope(t)}, list.good)
			if response.Code != http.StatusOK || !called {
				t.Fatalf("status = %d called = %v；body = %s", response.Code, called, response.Body)
			}
			page := pageOf(t, response)
			if string(page["size"]) != "50" || string(page["next"]) != `"`+pagedCursor+`"` || string(page["total"]) != "7" {
				t.Fatalf("page 不对：%s", response.Body.Bytes())
			}
			if query.Sort != list.wantSort || query.Keyword != "SYN" || query.After != nil ||
				!reflect.DeepEqual(query.Filters, list.wantFilters) {
				t.Fatalf("读口收到的查询不对：%+v", query)
			}
		})
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、排序维与筛选值，解不开的游标，换了条件还拿旧游标，都答 400 带理由散文、
// 不带任何行，读口不被调用。
func TestListEndpointsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	for _, list := range pagedLists() {
		t.Run(list.name, func(t *testing.T) {
			values := url.Values{"q": {"SYN-A"}}
			for key, given := range list.selector {
				values[key] = given
			}
			minted, err := list.catalogue.Decode(values)
			if err != nil {
				t.Fatalf("解出查询：%v", err)
			}
			cursor, err := minted.CursorAfter(list.last)
			if err != nil {
				t.Fatalf("编游标：%v", err)
			}
			refusals := map[string]string{
				"未知键":     "tenant=SYN-TENANT-02",
				"集外排序维":   "sort=SYN-unknown",
				"解不开的游标":  "after=SYN-not-a-cursor",
				"换条件拿旧游标": "q=SYN-B&after=" + url.QueryEscape(cursor),
			}
			if list.badFilter != "" {
				refusals["词表外筛选值"] = list.badFilter
			}
			for name, extra := range refusals {
				response, _, called := list.probe(t, operationsIntakeDouble{query: operationsScope(t)}, extra)
				detail := assertPageQueryRefused(t, response)
				if called {
					t.Fatalf("%s：坏请求走到了读口", name)
				}
				if name == "换条件拿旧游标" && !strings.Contains(detail, "从第一页重取") {
					t.Fatalf("%s：理由散文应叫调用方从第一页重取，得 %q", name, detail)
				}
			}

			// 同一枚游标在铸它的那组条件下照常可用：拒的是换了条件，不是游标本身。
			response, query, called := list.probe(t, operationsIntakeDouble{query: operationsScope(t)},
				"q=SYN-A&after="+url.QueryEscape(cursor))
			if response.Code != http.StatusOK || !called || query.After == nil ||
				!reflect.DeepEqual(*query.After, list.last) {
				t.Fatalf("原条件接游标：status = %d called = %v after = %+v", response.Code, called, query.After)
			}
		})
	}
}

// Covers: spec 取舍 8 与 ADR-0055 决定三——解码在 Intake 之后：渠道未配置时参数写对写错都答同一个 403，读口不被调用，
// 没有凭据的调用方探不出各册开放了哪些查询维度。
func TestListEndpointsAnswerTheUnconfiguredChannelBeforeDecoding(t *testing.T) {
	for _, list := range pagedLists() {
		t.Run(list.name, func(t *testing.T) {
			var bodies [][]byte
			for _, extra := range []string{list.good, "sort=SYN-unknown&tenant=SYN-TENANT-02"} {
				response, _, called := list.probe(t, visibilityhttp.UnconfiguredIntake{}, extra)
				if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" || called {
					t.Fatalf("%q 渠道未配置：status = %d called = %v；body = %s", extra, response.Code, called, response.Body)
				}
				bodies = append(bodies, response.Body.Bytes())
			}
			if !bytes.Equal(bodies[0], bodies[1]) {
				t.Fatalf("答复随参数变了：%s / %s", bodies[0], bodies[1])
			}
		})
	}
}

// Covers: 单条读取不迁（票 catalogue-read-pagination-rollout/03）——按包裹、按版本取单件的两支在解码之前分派走：翻页集外
// 的参数照旧不看，答复不带 page，列表读口不被调用。
func TestTrackingSingleProjectionBranchesAreNotDecoded(t *testing.T) {
	projection := operationsProjection(t)
	for _, target := range []string{
		"/tracking-projections?parcel=" + projection.Parcel().String() + "&sort=SYN-unknown&after=SYN-not-a-cursor",
		"/tracking-projections?version=" + projection.Version().String() + "&tenant=SYN-TENANT-02",
	} {
		reader := opsReader(t, projection)
		response := opsServe(t, operationsIntakeDouble{query: operationsScope(t)}, reader, http.MethodGet, target)
		if response.Code != http.StatusOK || reader.listCalled {
			t.Fatalf("%s：status = %d listCalled = %v；body = %s", target, response.Code, reader.listCalled, response.Body)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
		}
		if _, present := fields["page"]; present {
			t.Fatalf("%s：单件答复不该带 page：%s", target, response.Body.Bytes())
		}
	}
}
