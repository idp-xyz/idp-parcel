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

// Covers: ADR-0144 决定二、五、六——各册的列表端点各过一遍：答复带 page（size 取 Intake 给的页大小，next 与 total 原样
// 带出）；查询参数解成查询对象原样交给读口，选择器只选册、不进筛选。Intake 的页大小取一个不像缺省值的数，size 对上
// 才说明它来自 Intake。
func TestListEndpointsCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	for _, list := range pagedLists() {
		t.Run(list.name, func(t *testing.T) {
			intake := operationsIntakeDouble{query: operationsScope(t)}
			intake.query.Limit = 13
			response, query, called := list.probe(t, intake, list.good)
			if response.Code != http.StatusOK || !called {
				t.Fatalf("status = %d called = %v；body = %s", response.Code, called, response.Body)
			}
			page := pageOf(t, response)
			if string(page["size"]) != "13" || string(page["next"]) != `"`+pagedCursor+`"` || string(page["total"]) != "7" {
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

// Covers: spec「端点先 Intake、后解码查询参数」与 ADR-0055 决定三——渠道未配置时参数写对写错都答同一个 403，读口不被
// 调用，没有凭据的调用方探不出各册开放了哪些查询维度。
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

// selectorEndpoint 是按选择器或定位参数分派的一个端点，读口一律换成被调用就判失败的替身。good 是该端点上合规的几问；
// refusals 是迁移前就有、不经各册声明的坏请求：选择器缺席、取空或集外，定位参数取空或同时在场。
type selectorEndpoint struct {
	name     string
	serve    func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, target string) *httptest.ResponseRecorder
	good     []string
	refusals map[string]string
}

func selectorEndpoints() []selectorEndpoint {
	return []selectorEndpoint{
		{
			name: "tracking-projections",
			serve: func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, target string) *httptest.ResponseRecorder {
				return opsServe(t, intake, unreachableProjectionReader{t: t}, http.MethodGet, target)
			},
			good: []string{
				"/tracking-projections?sort=parcel",
				"/tracking-projections?parcel=SYN-PARCEL-1",
				"/tracking-projections?version=SYN-VERSION-1",
			},
			refusals: map[string]string{
				"定位参数同时在场":   "/tracking-projections?parcel=SYN-PARCEL-1&version=SYN-VERSION-1",
				"parcel 取空":  "/tracking-projections?parcel=",
				"version 取空": "/tracking-projections?version=",
			},
		},
		{
			name: "exception-triage-records",
			serve: func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, target string) *httptest.ResponseRecorder {
				return etServe(t, intake, unreachableTriageReader{t: t}, http.MethodGet, target)
			},
			good: []string{"/exception-triage-records?registry=signal-episode&sort=lastHitAt"},
			refusals: map[string]string{
				"registry 缺席": "/exception-triage-records?sort=lastHitAt",
				"registry 取空": "/exception-triage-records?registry=",
				"registry 集外": "/exception-triage-records?registry=SYN-REGISTRY",
			},
		},
		{
			name: "claims-recovery-records",
			serve: func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, target string) *httptest.ResponseRecorder {
				return crServe(t, intake, unreachableClaimsReader{t: t}, http.MethodGet, target)
			},
			good: []string{"/claims-recovery-records?registry=claim-item&sort=submittedAt"},
			refusals: map[string]string{
				"registry 缺席": "/claims-recovery-records?sort=submittedAt",
				"registry 取空": "/claims-recovery-records?registry=",
				"registry 集外": "/claims-recovery-records?registry=SYN-REGISTRY",
			},
		},
		{
			name: "visibility-catalogues",
			serve: func(t *testing.T, intake visibilityhttp.OperationsTrackingIntake, target string) *httptest.ResponseRecorder {
				return vcServe(t, intake, unreachableCatalogueReader{t: t}, http.MethodGet, target)
			},
			good: []string{"/visibility-catalogues?kind=TRIAGE_RULE&sort=effectiveFrom"},
			refusals: map[string]string{
				"kind 缺席": "/visibility-catalogues?sort=effectiveFrom",
				"kind 取空": "/visibility-catalogues?kind=",
				"kind 集外": "/visibility-catalogues?kind=SYN-KIND",
			},
		},
	}
}

// Covers: 选择器与定位参数的既有拒绝照旧——配置了的渠道上，它们写错（含传空值）都答迁移前那个不带理由的 400，
// 不落到各册声明的解码上，读口不被调用。
func TestSelectorEndpointsKeepTheirOwnRefusals(t *testing.T) {
	for _, endpoint := range selectorEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			for name, target := range endpoint.refusals {
				response := endpoint.serve(t, operationsIntakeDouble{query: operationsScope(t)}, target)
				var body struct {
					Error struct {
						Code   string `json:"code"`
						Detail string `json:"detail"`
					} `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatalf("%s：decode %s: %v", name, response.Body.Bytes(), err)
				}
				if response.Code != http.StatusBadRequest || body.Error.Code != "MALFORMED_REQUEST" || body.Error.Detail != "" {
					t.Fatalf("%s：status = %d body = %s，want 400 MALFORMED_REQUEST 不带 detail", name, response.Code, response.Body)
				}
			}
		})
	}
}

// Covers: spec「端点先 Intake、后解码查询参数」与 ADR-0055 决定三，落在选择器与定位参数上——渠道未配置时，合规的几问与
// 选择器、定位参数写错的各问同答一个 403，读口不被调用：选择器在 Intake 之后才读，没有凭据的调用方探不出端点下有哪些册子。
func TestSelectorEndpointsAnswerTheUnconfiguredChannelBeforeReadingSelectors(t *testing.T) {
	for _, endpoint := range selectorEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			targets := append([]string{}, endpoint.good...)
			for _, target := range endpoint.refusals {
				targets = append(targets, target)
			}
			var first []byte
			for _, target := range targets {
				response := endpoint.serve(t, visibilityhttp.UnconfiguredIntake{}, target)
				if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
					t.Fatalf("%s 渠道未配置：status = %d；body = %s", target, response.Code, response.Body)
				}
				if first == nil {
					first = response.Body.Bytes()
				} else if !bytes.Equal(first, response.Body.Bytes()) {
					t.Fatalf("答复随参数变了：%s / %s", first, response.Body.Bytes())
				}
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
