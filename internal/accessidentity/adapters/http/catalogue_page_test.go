package identityhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件证两册查阅（作业设备册、操作者册）迁到 ADR-0144 之后的端点一侧（票 catalogue-read-pagination-rollout/13）：答复带
// page、页大小取操作者渠道的 200、查询对象原样交给读口；集外的参数答 400 带理由散文、读口不被调用；渠道未配置时参数写对
// 写错同答 403。

// readerCall 是读口替身记下的一次调用。
type readerCall struct {
	calls int
	limit int
	query cataloguepage.Query
}

// pagedCatalogue 是一册查阅端点的测试接法：build 用给定的 next 与 total 建端点，交回读口替身的记录。
type pagedCatalogue struct {
	path         string
	sort         string
	build        func(t *testing.T, next string, total int64) (http.Handler, func() readerCall)
	unconfigured func(t *testing.T) http.Handler
}

func pagedCatalogues() map[string]pagedCatalogue {
	return map[string]pagedCatalogue{
		"work-devices": {
			path: identityhttp.WorkDeviceCataloguePath,
			sort: "deviceId",
			build: func(t *testing.T, next string, total int64) (http.Handler, func() readerCall) {
				fake := &catalogueFake{next: next, total: total}
				handler := catalogueHandler(t, authenticatorFake{tenant: catalogueTenant}, fake)
				return handler, func() readerCall { return readerCall{fake.calls, fake.limit, fake.query} }
			},
			unconfigured: func(t *testing.T) http.Handler {
				return catalogueHandler(t, authenticatorFake{err: accessidentity.ErrAccessChannelNotConfigured}, &catalogueFake{})
			},
		},
		"operators": {
			path: identityhttp.OperatorsPath,
			sort: "recordedAt",
			build: func(t *testing.T, next string, total int64) (http.Handler, func() readerCall) {
				fake := &operatorCatalogueFake{next: next, total: total}
				handler := operatorsHandler(t, adminAuthenticatorFake{administrator: administrator(t)}, fake, "https://id.syn.example/dex")
				return handler, func() readerCall { return readerCall{fake.listed, fake.limit, fake.query} }
			},
			unconfigured: func(t *testing.T) http.Handler {
				return operatorsHandler(t, adminAuthenticatorFake{err: accessidentity.ErrAccessChannelNotConfigured}, &operatorCatalogueFake{}, "https://id.syn.example/dex")
			},
		},
	}
}

// Covers: ADR-0144 决定二、五、六——两册的答复都带 page：size 是操作者渠道的页大小 200，next 与 total 原样带出，末页 next 为
// null；本册声明解出的查询对象原样交给读口，不带 sort 时是缺省序（正序）。作业设备册的状态筛选值维内去重、升序后交给读口。
func TestCataloguesCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	for name, catalogue := range pagedCatalogues() {
		t.Run(name, func(t *testing.T) {
			handler, read := catalogue.build(t, "SYN-cursor", 7)
			response := serveTarget(handler, catalogue.path+"?q=%20SYN-Q%20&sort=-"+catalogue.sort)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d；body = %s", response.Code, response.Body)
			}
			page := pageOf(t, response)
			if string(page["size"]) != "200" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
				t.Fatalf("page 不对：%s", response.Body.Bytes())
			}
			call := read()
			if call.calls != 1 || call.limit != 200 || call.query.Sort != (cataloguepage.Sort{Field: catalogue.sort, Descending: true}) ||
				call.query.Keyword != "SYN-Q" || call.query.After != nil {
				t.Fatalf("读口收到的不对：%d 次，limit %d，%+v", call.calls, call.limit, call.query)
			}

			last, readLast := catalogue.build(t, "", 0)
			response = serveTarget(last, catalogue.path)
			if page := pageOf(t, response); string(page["next"]) != "null" || string(page["total"]) != "0" {
				t.Fatalf("末页的 page 不对：%s", response.Body.Bytes())
			}
			if got := readLast().query.Sort; got != (cataloguepage.Sort{Field: catalogue.sort}) {
				t.Fatalf("不带 sort 时应是缺省序 %s，得 %+v", catalogue.sort, got)
			}
		})
	}

	handler, read := pagedCatalogues()["work-devices"].build(t, "", 0)
	response := serveTarget(handler, identityhttp.WorkDeviceCataloguePath+"?status=DEACTIVATED&status=REGISTERED&status=DEACTIVATED")
	if got := read().query.Filters["status"]; response.Code != http.StatusOK || strings.Join(got, ",") != "DEACTIVATED,REGISTERED" {
		t.Fatalf("状态筛选 = %v（%d %s）", got, response.Code, response.Body)
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、排序维与筛选键，词表外的筛选值，解不开的游标、换了条件或换了一册还拿来的旧游标，
// 都答 400 带理由散文，读口不被调用。解码在认证之后：渠道未配置时参数写对写错都答 403，答复不随请求内容变（ADR-0055 决定三；
// 该票 spec「端点先 Intake、后解码查询参数」）。
func TestCataloguesRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	deviceCursor := cursorFor(t, accessidentity.DeviceCataloguePages, cataloguepage.Position{
		Value: "SYN-DEVICE-A", Identity: []string{"SYN-DEVICE-A"},
	})
	operatorCursor := cursorFor(t, accessidentity.OperatorCataloguePages, cataloguepage.Position{
		Value: cataloguepage.FormatInstant(catalogueAt), Identity: []string{"SYN-ISSUER-A", "SYN-OPERATOR-01"},
	})
	refusals := map[string]map[string]struct{ query, points string }{
		"work-devices": {
			"未知键":       {"?tenant=SYN-TENANT-02", "tenant"},
			"页大小不由调用方给": {"?limit=9999", "limit"},
			"集外排序维":     {"?sort=-recordedAt", "recordedAt"},
			"词表外的筛选值":   {"?status=RETIRED", "RETIRED"},
			"解不开的游标":    {"?after=SYN-not-a-cursor", "游标"},
			"换了条件的旧游标":  {"?q=SYN-B&after=" + deviceCursor, "游标"},
			"换了一册的旧游标":  {"?q=SYN-A&after=" + operatorCursor, "游标"},
		},
		"operators": {
			"未知键":       {"?tenant=SYN-TENANT-02", "tenant"},
			"页大小不由调用方给": {"?limit=9999", "limit"},
			"集外排序维":     {"?sort=subject", "subject"},
			"筛选维不在声明里":  {"?status=REGISTERED", "status"},
			"解不开的游标":    {"?after=SYN-not-a-cursor", "游标"},
			"换了条件的旧游标":  {"?q=SYN-B&after=" + operatorCursor, "游标"},
			"换了一册的旧游标":  {"?q=SYN-A&after=" + deviceCursor, "游标"},
		},
	}
	for name, catalogue := range pagedCatalogues() {
		for label, refusal := range refusals[name] {
			t.Run(name+" "+label, func(t *testing.T) {
				handler, read := catalogue.build(t, "", 0)
				response := serveTarget(handler, catalogue.path+refusal.query)
				if detail := assertPageQueryRefused(t, response); !strings.Contains(detail, refusal.points) {
					t.Fatalf("理由散文应点到 %s：%q", refusal.points, detail)
				}
				if call := read(); call.calls != 0 {
					t.Fatalf("拒了还读了册 %d 次", call.calls)
				}
			})
		}

		// 同一个游标在原条件、原册下照常解得开：上面拒的是换了条件与换了册，不是游标本身。
		handler, read := catalogue.build(t, "", 0)
		cursor := map[string]string{"work-devices": deviceCursor, "operators": operatorCursor}[name]
		if response := serveTarget(handler, catalogue.path+"?q=SYN-A&after="+cursor); response.Code != http.StatusOK || read().query.After == nil {
			t.Fatalf("%s 原条件下的游标：%d %s", name, response.Code, response.Body)
		}

		unconfigured := catalogue.unconfigured(t)
		baseline := serveTarget(unconfigured, catalogue.path)
		if baseline.Code != http.StatusForbidden || problemCode(t, baseline) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s 渠道未配置：%d %s", name, baseline.Code, baseline.Body)
		}
		for _, refusal := range refusals[name] {
			if response := serveTarget(unconfigured, catalogue.path+refusal.query); response.Code != http.StatusForbidden ||
				response.Body.String() != baseline.Body.String() {
				t.Fatalf("%s%s 渠道未配置：%d %s，want 与不带参数同答 403", name, refusal.query, response.Code, response.Body)
			}
		}
		if response := serveTarget(unconfigured, catalogue.path+"?q=SYN-A&sort=-"+catalogue.sort); response.Body.String() != baseline.Body.String() {
			t.Fatalf("%s 渠道未配置、参数写对：%d %s", name, response.Code, response.Body)
		}
	}
}

func cursorFor(t *testing.T, pages *cataloguepage.Catalogue, position cataloguepage.Position) string {
	t.Helper()
	query, err := pages.Decode(url.Values{"q": {"SYN-A"}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := query.CursorAfter(position)
	if err != nil {
		t.Fatal(err)
	}
	return cursor
}

func serveTarget(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// assertPageQueryRefused 断言这一问被拒成 400 MALFORMED_REQUEST，且理由散文在 detail 里（ADR-0144 决定一、四）。
func assertPageQueryRefused(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400；body = %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	if body.Error.Code != "MALFORMED_REQUEST" || body.Error.Detail == "" {
		t.Fatalf("应答 MALFORMED_REQUEST 且带理由散文，得 %s", response.Body.Bytes())
	}
	return body.Error.Detail
}

// pageOf 读出答复里的 page 一格；size、next、total 都必须在场。
func pageOf(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body struct {
		Page map[string]json.RawMessage `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	for _, key := range []string{"size", "next", "total"} {
		if _, present := body.Page[key]; !present {
			t.Fatalf("page 缺 %s：%s", key, response.Body.Bytes())
		}
	}
	return body.Page
}
