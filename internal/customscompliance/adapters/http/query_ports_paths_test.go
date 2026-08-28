package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// stubPortsPaths 是读口替身：记录收到的键，交回预置的册子或故障。
type stubPortsPaths struct {
	portEntries []ports.CandidatePortEntry
	pathEntries []ports.DeclarationPathEntry
	err         error

	gotTenant string
	gotLimit  int
}

func (stub *stubPortsPaths) ListCandidatePorts(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CandidatePortEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.portEntries, stub.err
}

func (stub *stubPortsPaths) ListDeclarationPaths(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.DeclarationPathEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.pathEntries, stub.err
}

// unreachablePortsPaths 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库之前。
type unreachablePortsPaths struct{ t *testing.T }

func (stub unreachablePortsPaths) ListCandidatePorts(
	context.Context, domain.TenantID, int,
) ([]ports.CandidatePortEntry, error) {
	stub.t.Fatal("a refused request reached the ports register")
	return nil, nil
}

func (stub unreachablePortsPaths) ListDeclarationPaths(
	context.Context, domain.TenantID, int,
) ([]ports.DeclarationPathEntry, error) {
	stub.t.Fatal("a refused request reached the paths register")
	return nil, nil
}

func portsPathsRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/customs-ports-paths"+query, nil)
}

func endpointRoute(t *testing.T, port, mode string, direction domain.ManifestDirection) domain.DeclarationPathRoute {
	t.Helper()
	route, err := domain.NewDeclarationPathRoute(
		endpointValue(t, domain.NewCustomsPortReference, port),
		direction,
		endpointValue(t, domain.NewDeclarationModeReference, mode))
	if err != nil {
		t.Fatalf("构造路径三维：%v", err)
	}
	return route
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestPortsPathsQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachablePortsPaths{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-ports-paths?registry=candidate-port", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: registry 封闭集 — 缺席与集外值都是坏请求，拒在 Intake 之前；案件册的
// registry 值在本端点也是集外（两端点各自的封闭集不互相收留），未建模的区域维没有
// 分派格。
func TestPortsPathsQueryRejectsAnAbsentOrUnknownRegistry(t *testing.T) {
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachablePortsPaths{t: t},
	)
	for name, query := range map[string]string{
		"absent":           "",
		"blank":            "?registry=",
		"case-registry":    "?registry=readiness",
		"unmodelled-space": "?registry=candidate-region",
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, portsPathsRequest(t, query))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want %d", name, response.Code, http.StatusBadRequest)
		}
		if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
			t.Fatalf("%s: code = %q, want MALFORMED_REQUEST", name, got)
		}
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 对两个分派分支同答 403，
// 分支选择不泄露任何东西；读口不被触到。
func TestUnconfiguredPortsPathsIntakeRefusesBothRegistriesIdentically(t *testing.T) {
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		customshttp.UnconfiguredIntake{}, unreachablePortsPaths{t: t},
	)
	var bodies []string
	for _, query := range []string{"?registry=candidate-port", "?registry=declaration-path"} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, portsPathsRequest(t, query))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d", query, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", query, got)
		}
		assertNoOutcome(t, response)
		bodies = append(bodies, response.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("两个分支的未配置答复不一致：%v", bodies)
	}
}

// Covers: 版本区间逐格转写 — 开放版终点缺席、已换版行两端在场；租户与页大小从作用
// 域来，不采信请求自报；空册以空数组在场。
func TestCandidatePortListTranscribesVersionIntervalsVerbatim(t *testing.T) {
	successionAt := catalogueBaseAt.Add(48 * time.Hour)
	register := &stubPortsPaths{
		portEntries: []ports.CandidatePortEntry{
			{
				Port:        endpointValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"),
				AppliesFrom: successionAt,
			},
			{
				Port:         endpointValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"),
				AppliesFrom:  catalogueBaseAt,
				AppliesUntil: successionAt,
			},
		},
	}
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, portsPathsRequest(t, "?registry=candidate-port&tenant=TENANT-9&limit=9999"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Ports   []struct {
			Port         string `json:"port"`
			AppliesFrom  string `json:"appliesFrom"`
			AppliesUntil string `json:"appliesUntil"`
		} `json:"ports"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CANDIDATE_PORTS_LISTED" || len(body.Ports) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	open, closed := body.Ports[0], body.Ports[1]
	if open.Port != "SYN-PORT-01" || open.AppliesUntil != "" {
		t.Fatalf("开放版终点该缺席：%+v", open)
	}
	if closed.AppliesFrom != catalogueBaseAt.Format(time.RFC3339Nano) ||
		closed.AppliesUntil != successionAt.Format(time.RFC3339Nano) {
		t.Fatalf("已换版行区间转写走样：%+v", closed)
	}

	// 空册如实答空数组，不折成未配置（ADR-0077 Decision 四）。
	register.portEntries = nil
	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, portsPathsRequest(t, "?registry=candidate-port"))
	var fields struct {
		Ports json.RawMessage `json:"ports"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if response.Code != http.StatusOK || string(fields.Ports) != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", response.Body.String())
	}
}

// Covers: 路径三维逐格转写 — 口岸、方向、申报模式与生效区间原样透出，registry 分派
// 各答各的结果格。
func TestDeclarationPathListTranscribesTheRouteVerbatim(t *testing.T) {
	register := &stubPortsPaths{
		pathEntries: []ports.DeclarationPathEntry{
			{
				Path:        endpointValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"),
				Route:       endpointRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest),
				AppliesFrom: catalogueBaseAt,
			},
		},
	}
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, portsPathsRequest(t, "?registry=declaration-path"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Paths   []struct {
			Path            string `json:"path"`
			Port            string `json:"port"`
			Direction       string `json:"direction"`
			DeclarationMode string `json:"declarationMode"`
			AppliesFrom     string `json:"appliesFrom"`
			AppliesUntil    string `json:"appliesUntil"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "DECLARATION_PATHS_LISTED" || len(body.Paths) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	path := body.Paths[0]
	if path.Path != "SYN-PATH-01" || path.Port != "SYN-PORT-01" ||
		path.Direction != "IMPORT" || path.DeclarationMode != "SYN-MODE-GENERAL" ||
		path.AppliesFrom != catalogueBaseAt.Format(time.RFC3339Nano) || path.AppliesUntil != "" {
		t.Fatalf("路径三维转写走样：%+v", path)
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingPortsPathsReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryPortsPathsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubPortsPaths{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, portsPathsRequest(t, "?registry=declaration-path"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
