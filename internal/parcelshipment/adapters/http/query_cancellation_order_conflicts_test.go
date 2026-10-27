package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证取消先后冲突册的运营查阅端点（ADR-0295 决定一）：租户与页大小只来自 Intake（不采信请求）；清单逐字段透出决定命令要的四段与两边的业务时间；计数不被页大小截断
// 且告诉调用方清单是否被截断；读不回答 500 而不是空册；非 GET 405；Intake 的各格原样映射。

type conflictIntakeDouble struct {
	tenant domain.TenantID
	limit  int
	err    error
}

func (double conflictIntakeDouble) IntakeCancellationOrderConflictQuery(context.Context, *http.Request) (shipmenthttp.CancellationOrderConflictQuery, error) {
	if double.err != nil {
		return shipmenthttp.CancellationOrderConflictQuery{}, double.err
	}
	return shipmenthttp.CancellationOrderConflictQuery{Tenant: double.tenant, Limit: double.limit}, nil
}

type conflictCatalogueDouble struct {
	listed ports.UnresolvedCancellationOrderConflicts
	err    error

	tenant domain.TenantID
	limit  int
}

func (double *conflictCatalogueDouble) ListUnresolvedCancellationOrderConflicts(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
) (ports.UnresolvedCancellationOrderConflicts, error) {
	double.tenant, double.limit = tenant, limit
	return double.listed, double.err
}

func conflictRow(t *testing.T, parcel, reference string, kind domain.CancellationBoundaryKind) ports.CancellationOrderConflictEntry {
	t.Helper()
	return ports.CancellationOrderConflictEntry{
		TenantID:     mustValue(t, domain.NewTenantID, "TENANT-1"),
		Parcel:       mustValue(t, domain.NewDeclaredParcelID, parcel),
		Cancellation: mustValue(t, domain.NewParcelCancellationID, "PCXL-1"),
		CancelledAt:  time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC),
		Boundary: domain.CancellationBoundaryFact{
			Kind: kind, Reference: mustValue(t, domain.NewCancellationBoundaryReference, reference),
			OccurredAt: time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC),
		},
		DetectedAt: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC),
	}
}

func getConflicts(t *testing.T, intake shipmenthttp.CancellationOrderConflictQueryIntake, reader shipmenthttp.CancellationOrderConflictCatalogueReader, method string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	shipmenthttp.NewQueryCancellationOrderConflictsEndpoint(intake, reader).
		ServeHTTP(response, httptest.NewRequest(method, "/cancellation-order-conflicts", nil))
	return response
}

// Covers: AT-PS-167；ADR-0295 决定一——清单逐字段透出（决定命令要的包裹、边界种类、边界那一代、取消四段原样给出，两边的业务时间与发现时间一并给出）；租户与页大小取 Intake
// 交出的、原样递给读口；清单被页大小截断时 truncated=true 而计数是未解开的全部行数。
func TestTheConflictQueryListsUnresolvedRowsWithTheirCountAndTruncationFlag(t *testing.T) {
	reader := &conflictCatalogueDouble{listed: ports.UnresolvedCancellationOrderConflicts{
		Conflicts: []ports.CancellationOrderConflictEntry{
			conflictRow(t, "PARCEL-1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary),
			conflictRow(t, "PARCEL-2", "CFEP-1@CFEV-1", domain.CarrierFirstEffectivePickupBoundary),
		},
		Count: 5,
	}}
	intake := conflictIntakeDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-1"), limit: 2}

	response := getConflicts(t, intake, reader, http.MethodGet)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if reader.tenant.String() != "TENANT-1" || reader.limit != 2 {
		t.Fatalf("读口收到 tenant=%q limit=%d, want Intake 交出的 TENANT-1 与 2", reader.tenant, reader.limit)
	}
	var body struct {
		Outcome         string `json:"outcome"`
		UnresolvedCount int    `json:"unresolvedCount"`
		Truncated       bool   `json:"truncated"`
		Conflicts       []struct {
			ParcelID     string `json:"parcelId"`
			BoundaryKind string `json:"boundaryKind"`
			BoundaryRef  string `json:"boundaryRef"`
			Cancellation string `json:"cancellation"`
			CancelledAt  string `json:"cancelledAt"`
			BoundaryAt   string `json:"boundaryAt"`
			DetectedAt   string `json:"detectedAt"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, response.Body.String())
	}
	if body.Outcome != "CANCELLATION_ORDER_CONFLICTS_LISTED" || body.UnresolvedCount != 5 || !body.Truncated || len(body.Conflicts) != 2 {
		t.Fatalf("响应 = %+v, want LISTED、计数 5、被截断、两行", body)
	}
	network, carrier := body.Conflicts[0], body.Conflicts[1]
	if network.ParcelID != "PARCEL-1" || network.BoundaryKind != "NETWORK_INTAKE" || network.BoundaryRef != "NODE_INTAKE/v1" ||
		network.Cancellation != "PCXL-1" || network.CancelledAt != "2026-10-04T08:00:00Z" ||
		network.BoundaryAt != "2026-10-04T07:00:00Z" || network.DetectedAt != "2026-10-26T09:00:00Z" {
		t.Fatalf("行没有逐字段透出：%+v", network)
	}
	if carrier.BoundaryKind != "CARRIER_FIRST_EFFECTIVE_PICKUP" || carrier.BoundaryRef != "CFEP-1@CFEV-1" {
		t.Fatalf("面单渠道一行 = %+v", carrier)
	}
}

// Covers: AT-PS-167；没有被截断时 truncated=false；空册答空数组而非 null 与零计数（「没有未解开的冲突」是如实答案）。
func TestTheConflictQueryAnswersAnEmptyRegisterAsAnEmptyArray(t *testing.T) {
	reader := &conflictCatalogueDouble{}
	intake := conflictIntakeDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-1"), limit: 50}
	response := getConflicts(t, intake, reader, http.MethodGet)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"conflicts":[]`) ||
		!strings.Contains(response.Body.String(), `"unresolvedCount":0`) || !strings.Contains(response.Body.String(), `"truncated":false`) {
		t.Fatalf("空册 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: AT-PS-167；ADR-0029——读不回是答案未形成（500 NO_ANSWER_FORMED），不伪装成空册；Intake 各格原样映射，读口一次也不被调；非 GET 答 405；未配置的 Intake 答 403。
func TestTheConflictQueryMapsFailuresToTheirOwnCells(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "TENANT-1")

	failing := &conflictCatalogueDouble{err: errors.New("read failed")}
	response := getConflicts(t, conflictIntakeDouble{tenant: tenant, limit: 50}, failing, http.MethodGet)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("读不回 = %d %s, want 500 NO_ANSWER_FORMED", response.Code, response.Body.String())
	}

	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{shipmenthttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{shipmenthttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{shipmenthttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{shipmenthttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	} {
		reader := &conflictCatalogueDouble{}
		response := getConflicts(t, conflictIntakeDouble{err: test.err}, reader, http.MethodGet)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Errorf("Intake %v = %d %s, want %d %s", test.err, response.Code, response.Body.String(), test.status, test.code)
		}
		if reader.limit != 0 {
			t.Errorf("Intake %v 之后读口仍被调了", test.err)
		}
	}

	reader := &conflictCatalogueDouble{}
	response = getConflicts(t, conflictIntakeDouble{tenant: tenant, limit: 50}, reader, http.MethodPost)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d Allow=%q, want 405 与 Allow: GET", response.Code, response.Header().Get("Allow"))
	}
	if response := getConflicts(t, shipmenthttp.UnconfiguredIntake{}, &conflictCatalogueDouble{}, http.MethodGet); response.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", response.Code)
	}
}

// Covers: AT-PS-167；ADR-0100 决定四、ADR-0295 决定一——操作者查阅 Intake 的租户只取自认证结果、页大小是渠道产品策略；认证失败原样交回、不构造查询。
func TestTheOperatorReadIntakeTakesTheConflictQueryScopeFromTheEnvelopeOnly(t *testing.T) {
	authenticator := &operationsReadAuthenticatorDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-9")}
	intake, err := shipmenthttp.NewOperatorOperationsReadIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/cancellation-order-conflicts?tenant=TENANT-1&limit=1", nil)
	request.Header.Set("Authorization", "Bearer operator.token")
	query, err := intake.IntakeCancellationOrderConflictQuery(t.Context(), request)
	if err != nil {
		t.Fatalf("认证出的操作者查阅：%v", err)
	}
	if query.Tenant.String() != "TENANT-9" || query.Limit != 200 || authenticator.token != "operator.token" {
		t.Fatalf("查询 = %+v（令牌 %q）, want 信封里的 TENANT-9、页大小 200，请求里自报的租户与页大小一概不采信", query, authenticator.token)
	}

	authenticator.err = shipmenthttp.ErrOperatorNotGranted
	if _, err := intake.IntakeCancellationOrderConflictQuery(t.Context(), request); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Fatalf("认证失败 = %v, want 原样交回", err)
	}
}
