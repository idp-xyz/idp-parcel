package networkhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

var endpointBaseAt = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面：作用域与页大小都来自它，不读
// 请求内容。真渠道（操作者渠道，ADR-0100）未就位，生产装配点不会有这样的实现——它只在测试里
// 存在，为的是隔离验证端点的分派与转写。
type grantedCatalogueIntake struct {
	tenant string
	limit  int
}

func (intake grantedCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (networkhttp.NetworkCatalogQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return networkhttp.NetworkCatalogQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return networkhttp.NetworkCatalogQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return networkhttp.NetworkCatalogQuery{}, err
	}
	return networkhttp.NetworkCatalogQuery{Scope: scope, Limit: intake.limit}, nil
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (networkhttp.NetworkCatalogQuery, error) {
	return networkhttp.NetworkCatalogQuery{}, intake.err
}

// stubCatalogReader 是读口替身：记录收到的键与查询对象，交回预置的行、page 两格或故障。
type stubCatalogReader struct {
	nodes       []ports.NodeDefinitionVersion
	connections []ports.ConnectionDefinitionVersion
	lines       []ports.LineDefinitionVersion
	areas       []ports.ServiceAreaDefinitionVersion
	calendars   []ports.ServiceCalendarDefinitionVersion
	adjustments []ports.AvailabilityAdjustmentStatement
	strategies  []ports.RouteStrategyDefinitionVersion
	next        func(cataloguepage.Query) string
	total       int64
	err         error

	gotTenant string
	gotLimit  int
	gotQuery  cataloguepage.Query
}

func pageOf[Row any](stub *stubCatalogReader, tenant domain.TenantID, limit int, query cataloguepage.Query, rows []Row) (ports.CatalogPage[Row], error) {
	stub.gotTenant, stub.gotLimit, stub.gotQuery = tenant.String(), limit, query
	page := ports.CatalogPage[Row]{Rows: rows, Total: stub.total}
	if stub.next != nil {
		page.Next = stub.next(query)
	}
	return page, stub.err
}

func (stub *stubCatalogReader) ListNodeVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.NodeDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.nodes)
}

func (stub *stubCatalogReader) ListConnectionVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.ConnectionDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.connections)
}

func (stub *stubCatalogReader) ListLineVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.LineDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.lines)
}

func (stub *stubCatalogReader) ListServiceAreaVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.ServiceAreaDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.areas)
}

func (stub *stubCatalogReader) ListServiceCalendarVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.ServiceCalendarDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.calendars)
}

func (stub *stubCatalogReader) ListAvailabilityAdjustments(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.AvailabilityAdjustmentStatement], error) {
	return pageOf(stub, tenant, limit, query, stub.adjustments)
}

func (stub *stubCatalogReader) ListRouteStrategyVersions(_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (ports.CatalogPage[ports.RouteStrategyDefinitionVersion], error) {
	return pageOf(stub, tenant, limit, query, stub.strategies)
}

// unreachableCatalogReader 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库
// 之前。
type unreachableCatalogReader struct{ t *testing.T }

func (stub unreachableCatalogReader) fail() {
	stub.t.Fatal("a refused request reached the catalogue reader")
}

func (stub unreachableCatalogReader) ListNodeVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.NodeDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.NodeDefinitionVersion]{}, nil
}

func (stub unreachableCatalogReader) ListConnectionVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.ConnectionDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.ConnectionDefinitionVersion]{}, nil
}

func (stub unreachableCatalogReader) ListLineVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.LineDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.LineDefinitionVersion]{}, nil
}

func (stub unreachableCatalogReader) ListServiceAreaVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.ServiceAreaDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.ServiceAreaDefinitionVersion]{}, nil
}

func (stub unreachableCatalogReader) ListServiceCalendarVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.ServiceCalendarDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.ServiceCalendarDefinitionVersion]{}, nil
}

func (stub unreachableCatalogReader) ListAvailabilityAdjustments(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.AvailabilityAdjustmentStatement], error) {
	stub.fail()
	return ports.CatalogPage[ports.AvailabilityAdjustmentStatement]{}, nil
}

func (stub unreachableCatalogReader) ListRouteStrategyVersions(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.CatalogPage[ports.RouteStrategyDefinitionVersion], error) {
	stub.fail()
	return ports.CatalogPage[ports.RouteStrategyDefinitionVersion]{}, nil
}

func catalogRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/network-catalog"+query, nil)
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem response %s: %v", response.Body.Bytes(), err)
	}
	return body.Error.Code
}

func assertNoOutcome(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("a response that formed no answer carried an outcome: %s", response.Body.Bytes())
	}
}

var allFamilies = []string{
	"node", "connection", "line", "service-area",
	"service-calendar", "availability-adjustment", "route-strategy",
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestNetworkCatalogQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableCatalogReader{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/network-catalog?family=node", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: family 封闭七族 — 缺席与未知值都是坏请求（替调用方默认一族就是替它猜），
// 拒在 Intake 之前，属传输形状。
func TestNetworkCatalogQueryRejectsAnAbsentOrUnknownFamily(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachableCatalogReader{t: t},
	)
	for name, query := range map[string]string{
		"absent":  "",
		"blank":   "?family=",
		"unknown": "?family=network-definition",
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, catalogRequest(t, query))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want %d", name, response.Code, http.StatusBadRequest)
		}
		if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
			t.Fatalf("%s: code = %q, want MALFORMED_REQUEST", name, got)
		}
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 对全部七族同答 403，分支选择
// 不泄露任何东西；读口不被触到。
func TestUnconfiguredCatalogueIntakeRefusesAllFamiliesIdentically(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		networkhttp.UnconfiguredIntake{}, unreachableCatalogReader{t: t},
	)
	var baseline string
	for _, family := range allFamilies {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, catalogRequest(t, "?family="+family))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d", family, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", family, got)
		}
		assertNoOutcome(t, response)
		if baseline == "" {
			baseline = response.Body.String()
		} else if response.Body.String() != baseline {
			t.Fatalf("%s: 未配置答复与其他族不一致：%s vs %s", family, response.Body.String(), baseline)
		}
	}
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上一用例）、
// 其余是 500 INTAKE_FAILED。
func TestCatalogueIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := networkhttp.NewQueryNetworkCatalogEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", networkhttp.ErrMalformedRequest)},
		unreachableCatalogReader{t: t},
	)
	response := httptest.NewRecorder()
	malformed.ServeHTTP(response, catalogRequest(t, "?family=node"))
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}

	failing := networkhttp.NewQueryNetworkCatalogEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableCatalogReader{t: t},
	)
	response = httptest.NewRecorder()
	failing.ServeHTTP(response, catalogRequest(t, "?family=node"))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("intake failure: %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0077 Decision 一/五 — 节点族逐字段转写：未闭版 effectiveTo 缺席、已闭版
// 在场；租户与页大小从作用域来（自报的 tenant / limit 在 ADR-0144 之下已是集外键，见
// TestSelfReportedScopeParametersAreRefusedBeforeTheIntake）。
func TestNodeVersionsAreListedVerbatim(t *testing.T) {
	reader := &stubCatalogReader{
		nodes: []ports.NodeDefinitionVersion{
			{
				Code: "node-hub", Version: 2, BusinessTimezone: "Europe/Berlin",
				EffectiveFrom: endpointBaseAt,
			},
			{
				Code: "node-hub", Version: 1, BusinessTimezone: "Asia/Shanghai",
				EffectiveFrom: endpointBaseAt.Add(-48 * time.Hour),
				EffectiveTo:   endpointBaseAt, HasEffectiveTo: true,
			},
		},
	}
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, reader,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=node"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if reader.gotTenant != "TENANT-1" || reader.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（应取自作用域）",
			reader.gotTenant, reader.gotLimit)
	}

	var body struct {
		Outcome  string `json:"outcome"`
		Versions []struct {
			Code             string `json:"code"`
			Version          int32  `json:"version"`
			BusinessTimezone string `json:"businessTimezone"`
			EffectiveFrom    string `json:"effectiveFrom"`
			EffectiveTo      string `json:"effectiveTo"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "NODE_VERSIONS_LISTED" || len(body.Versions) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	open, closed := body.Versions[0], body.Versions[1]
	if open.Version != 2 || open.BusinessTimezone != "Europe/Berlin" || open.EffectiveTo != "" {
		t.Fatalf("未闭版转写走样（终点该缺席）：%+v", open)
	}
	if closed.Version != 1 || closed.EffectiveTo != endpointBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("已闭版转写走样：%+v", closed)
	}
}

// Covers: 线路段链按序转写；调整族的封闭枚举词与解除时间逐字段透出（未解除缺席）。
func TestLineSegmentsAndAdjustmentChainsAreTransliteratedVerbatim(t *testing.T) {
	reader := &stubCatalogReader{
		lines: []ports.LineDefinitionVersion{{
			Code: "line-eu", Version: 1, Segments: []string{"conn-a-b", "conn-b-c"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: "scope-declared",
			EffectiveFrom: endpointBaseAt,
		}},
		adjustments: []ports.AvailabilityAdjustmentStatement{
			{
				Code: "adj-1", Version: 2, TargetKind: ports.TargetLine, TargetCode: "line-eu",
				Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
				EffectiveAt: endpointBaseAt,
				LiftedAt:    endpointBaseAt.Add(time.Hour), HasLiftedAt: true,
			},
			{
				Code: "adj-1", Version: 1, TargetKind: ports.TargetLine, TargetCode: "line-eu",
				Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
				EffectiveAt: endpointBaseAt,
			},
		},
	}
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, reader,
	)

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=line"))
	var lineBody struct {
		Outcome  string `json:"outcome"`
		Versions []struct {
			Segments        []string `json:"segments"`
			ApplicableScope string   `json:"applicableScope"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &lineBody); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if lineBody.Outcome != "LINE_VERSIONS_LISTED" || len(lineBody.Versions) != 1 ||
		len(lineBody.Versions[0].Segments) != 2 || lineBody.Versions[0].Segments[0] != "conn-a-b" {
		t.Fatalf("线路转写走样：%s", response.Body.String())
	}

	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=availability-adjustment"))
	var adjustmentBody struct {
		Outcome  string `json:"outcome"`
		Versions []struct {
			TargetKind string `json:"targetKind"`
			Kind       string `json:"kind"`
			Source     string `json:"source"`
			LiftedAt   string `json:"liftedAt"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &adjustmentBody); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if adjustmentBody.Outcome != "AVAILABILITY_ADJUSTMENTS_LISTED" || len(adjustmentBody.Versions) != 2 {
		t.Fatalf("调整转写走样：%s", response.Body.String())
	}
	lifted, standing := adjustmentBody.Versions[0], adjustmentBody.Versions[1]
	if lifted.TargetKind != "LINE" || lifted.Kind != "SUSPENSION" || lifted.LiftedAt == "" {
		t.Fatalf("已解除陈述转写走样：%+v", lifted)
	}
	if standing.LiftedAt != "" {
		t.Fatalf("未解除陈述不该带解除时间：%+v", standing)
	}
}

// Covers: ADR-0077 Decision 四 — 空族是 2xx 成格 + 空数组（不是 null）：空族的续办
// 是去登记口登记，不是配置渠道，两格不得合并。
func TestAnEmptyFamilyAnswersAnEmptyArray(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, &stubCatalogReader{},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=service-area"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if string(fields["outcome"]) != `"SERVICE_AREA_VERSIONS_LISTED"` {
		t.Fatalf("outcome 走样：%s", response.Body.String())
	}
	if string(fields["versions"]) != "[]" {
		t.Fatalf("空族没有交回空数组：%s", response.Body.String())
	}
	// ADR-0144 决定五：空目录仍如实答空——next 为 null、total 为 0，size 回显作用域给的页大小。
	if string(fields["page"]) != `{"size":25,"next":null,"total":0}` {
		t.Fatalf("空族的 page 走样：%s", response.Body.String())
	}
}

// Covers: ADR-0146 决定二的查阅半边——路由策略版本声明的排序形态随行透出；没声明的版本缺这
// 一格而不是给个空串，读的人分得出「选了哪一种」与「还没选」。
func TestRouteStrategyVersionsShowTheirDeclaredRankingForm(t *testing.T) {
	effective := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCatalogReader{strategies: []ports.RouteStrategyDefinitionVersion{
			{Code: "strategy-declared", Version: 1, ApplicableScope: "scope-1",
				RankingForm: domain.CostSingleDimensionRanking, EffectiveFrom: effective},
			{Code: "strategy-undeclared", Version: 1, ApplicableScope: "scope-1", EffectiveFrom: effective},
		}},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=route-strategy"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Versions []map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if len(body.Versions) != 2 {
		t.Fatalf("versions = %s", response.Body.String())
	}
	if string(body.Versions[0]["rankingForm"]) != `"COST_SINGLE_DIMENSION"` {
		t.Fatalf("声明过形态的版本没透出形态：%s", response.Body.String())
	}
	if _, present := body.Versions[1]["rankingForm"]; present {
		t.Fatalf("没声明形态的版本长出了 rankingForm：%s", response.Body.String())
	}
}

// Covers: ADR-0148 决定二、五的查阅半边——服务区域版本登的覆盖与节点角色随行透出；没登覆盖的版本整格
// 缺席而不是给一份空覆盖，读的人分得出「覆盖整个国家」与「还没登覆盖」。
func TestServiceAreaVersionsShowTheirCoverage(t *testing.T) {
	effective := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCatalogReader{areas: []ports.ServiceAreaDefinitionVersion{
			{Code: "area-covered", Version: 1, EffectiveFrom: effective,
				HasCoverage: true, CoverageCountry: "XA", PostalPrefixes: []string{"10"},
				OriginNodes: []string{"node-a"}},
			{Code: "area-bare", Version: 1, EffectiveFrom: effective},
		}},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=service-area"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Versions []map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if len(body.Versions) != 2 {
		t.Fatalf("versions = %s", response.Body.String())
	}
	if got := string(body.Versions[0]["coverage"]); got !=
		`{"country":"XA","postalPrefixes":["10"],"originNodes":["node-a"]}` {
		t.Fatalf("登过覆盖的版本透出 %s", got)
	}
	if _, present := body.Versions[1]["coverage"]; present {
		t.Fatalf("没登覆盖的版本长出了 coverage：%s", response.Body.String())
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空族：前者该重试，
// 后者是终局答案。
func TestAFailingCatalogueReadIsNoAnswerRatherThanAnEmptyFamily(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCatalogReader{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=route-strategy"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}

func problemDetailText(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem response %s: %v", response.Body.Bytes(), err)
	}
	return body.Error.Detail
}

// Covers: ADR-0144 决定四「未知参数键 → MALFORMED_REQUEST」——自报的 tenant / limit 从前被忽略，
// 如今是集外键，在 Intake 之前就拒（越权风险点 2 预告过的收紧）；读口不被触到。
func TestSelfReportedScopeParametersAreRefusedBeforeTheIntake(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachableCatalogReader{t: t},
	)
	for _, query := range []string{"?family=node&tenant=TENANT-9", "?family=node&limit=9999"} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, catalogRequest(t, query))
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s: %d %s", query, response.Code, response.Body.String())
		}
		if problemDetailText(t, response) == "" {
			t.Fatalf("%s: 400 没带理由散文", query)
		}
	}
}

// Covers: ADR-0144 决定三、四、一——集外排序维、词表外筛选值、别族或换条件后的游标、超长的 q，都在
// Intake 之前答 400，detail 带理由散文；读口不被触到。
func TestMalformedQueryParametersAreRefusedWithAReason(t *testing.T) {
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachableCatalogReader{t: t},
	)
	nodeQuery, err := ports.NodeVersionCatalogue.Decode(nil)
	if err != nil {
		t.Fatal(err)
	}
	nodeCursor, err := nodeQuery.CursorAfter(cataloguepage.Position{
		Value: "SYN-NODE-01", Identity: []string{"SYN-NODE-01", "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"集外排序维":    "?family=node&sort=businessTimezone",
		"词表外筛选值":   "?family=service-calendar&targetKind=AREA",
		"集外筛选维":    "?family=line&applicableScope=SYN-SCOPE",
		"别族的游标":    "?family=line&after=" + nodeCursor,
		"换条件后的旧游标": "?family=node&code=SYN-NODE-02&after=" + nodeCursor,
		"超长的 q":    "?family=node&q=" + strings.Repeat("S", cataloguepage.MaxKeywordRunes+1),
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, catalogRequest(t, query))
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			if problemDetailText(t, response) == "" {
				t.Fatalf("400 没带理由散文：%s", response.Body.String())
			}
		})
	}
}

// Covers: ADR-0144 决定六——端点按所选族的声明解出查询对象原样交给读口：排序、筛选（维内多值）、q。
func TestTheDecodedQueryReachesTheReader(t *testing.T) {
	reader := &stubCatalogReader{}
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, reader,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t,
		"?family=connection&sort=-effectiveFrom&fromNode=SYN-NODE-02&fromNode=SYN-NODE-01&q=%20SYN%20"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	got := reader.gotQuery
	if got.Sort != (cataloguepage.Sort{Field: "effectiveFrom", Descending: true}) {
		t.Fatalf("排序走样：%v", got.Sort)
	}
	if want := []string{"SYN-NODE-01", "SYN-NODE-02"}; !reflect.DeepEqual(got.Filters["fromNode"], want) {
		t.Fatalf("筛选走样：%v", got.Filters)
	}
	if got.Keyword != "SYN" || got.After != nil {
		t.Fatalf("q 或游标走样：%+v", got)
	}
}

// Covers: ADR-0144 决定五——答复的 page 由读口的下一游标与总数拼成，size 回显作用域给的页大小；
// 把 next 原样带回 after，读口收到的就是上一页末行的位置。
func TestTheAnswerCarriesThePageAndItsCursorComesBack(t *testing.T) {
	last := cataloguepage.Position{Value: "SYN-NODE-07", Identity: []string{"SYN-NODE-07", "3"}}
	reader := &stubCatalogReader{
		total: 42,
		next: func(query cataloguepage.Query) string {
			cursor, err := query.CursorAfter(last)
			if err != nil {
				t.Fatalf("替身编游标：%v", err)
			}
			return cursor
		},
	}
	endpoint := networkhttp.NewQueryNetworkCatalogEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, reader,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=node&code=SYN-NODE-07"))
	var body struct {
		Page struct {
			Size  int     `json:"size"`
			Next  *string `json:"next"`
			Total *int64  `json:"total"`
		} `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Page.Size != 25 || body.Page.Next == nil || body.Page.Total == nil || *body.Page.Total != 42 {
		t.Fatalf("page 走样：%s", response.Body.String())
	}

	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogRequest(t, "?family=node&code=SYN-NODE-07&after="+*body.Page.Next))
	if response.Code != http.StatusOK {
		t.Fatalf("带回游标：status = %d body = %s", response.Code, response.Body.String())
	}
	if reader.gotQuery.After == nil || !reflect.DeepEqual(*reader.gotQuery.After, last) {
		t.Fatalf("读口收到的位置：%+v，应为 %+v", reader.gotQuery.After, last)
	}
}
