package shipmenthttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证取消先后冲突解开决定的传输层（ADR-0295 决定一至三）：Intake 先认证（决定种类是 CANCELLATION_ORDER_CONFLICT_RESOLUTION，与授权处置、释放拒收处理分开）、后封闭解载荷；
// 租户只取自认证出的信封，决定人取认证出的操作者；端点把结果各映射到自己的格——不可见 404，没形成答案 500，Intake 各格先于编排；回执不回显任何一位操作者。

const conflictDecisionBody = `{"parcel":"SYN-PARCEL-01","boundaryKind":"NETWORK_INTAKE","boundaryRef":"NODE_INTAKE/v1","cancellation":"SYN-PCXL-01","decision":"CANCELLATION_NOT_UPHELD","basis":"SYN-NODE-SCAN"}`

// Covers: AT-PS-166；ADR-0295 决定二——Intake 用「取消先后冲突解开」这一种决定认证，租户取信封；决定人取认证出的操作者；冲突由四段指名；去向取封闭两词；依据必填。
func TestTheConflictDecisionIntakeAuthenticatesItsOwnDecisionAndTakesTheTenantFromTheEnvelope(t *testing.T) {
	var asked []shipmenthttp.OperatorDecision
	intake := decisionIntake(t, decisionAuthenticator{asked: &asked}, decisionTargets{})

	command, err := intake.IntakeCancellationOrderConflictDecision(context.Background(), decisionRequest("t", conflictDecisionBody))
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if len(asked) != 1 || asked[0] != shipmenthttp.OperatorDecisionCancellationOrderConflictResolution ||
		string(asked[0]) != "CANCELLATION_ORDER_CONFLICT_RESOLUTION" {
		t.Fatalf("认证用的决定种类 = %v, want CANCELLATION_ORDER_CONFLICT_RESOLUTION——与授权处置共用就是授权处置的授予能裁断取消先后", asked)
	}
	if command.Tenant.String() != decisionTenant || command.Parcel.String() != "SYN-PARCEL-01" ||
		command.BoundaryKind != domain.NetworkIntakeBoundary || command.BoundaryRef.String() != "NODE_INTAKE/v1" ||
		command.Cancellation.String() != "SYN-PCXL-01" || command.Kind != domain.CancellationNotUpheld ||
		command.Decider.String() != decisionOperator || command.Basis.String() != "SYN-NODE-SCAN" {
		t.Fatalf("命令 = %+v", command)
	}

	referred, err := intake.IntakeCancellationOrderConflictDecision(context.Background(), decisionRequest("t",
		strings.Replace(strings.Replace(conflictDecisionBody, "CANCELLATION_NOT_UPHELD", "BOUNDARY_FACT_REFERRED", 1), "NETWORK_INTAKE", "CARRIER_FIRST_EFFECTIVE_PICKUP", 1)))
	if err != nil || referred.Kind != domain.BoundaryFactReferred || referred.BoundaryKind != domain.CarrierFirstEffectivePickupBoundary {
		t.Fatalf("交来源方更正 = %+v err=%v", referred, err)
	}
}

// Covers: AT-PS-166；ADR-0151 决定二——先认证、后解载荷；载荷里任何身份格（租户、决定人）封闭解码即拒；缺字段、去向与边界种类认不出、依据空白答坏报文。
func TestTheConflictDecisionIntakeRefusesAMalformedPayload(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	ctx := context.Background()

	for name, body := range map[string]string{
		"带租户":     strings.Replace(conflictDecisionBody, `"decision"`, `"tenantId":"SYN-TENANT-99","decision"`, 1),
		"带决定人":    strings.Replace(conflictDecisionBody, `"decision"`, `"decider":"someone","decision"`, 1),
		"去向认不出":   strings.Replace(conflictDecisionBody, "CANCELLATION_NOT_UPHELD", "DELETE_THE_CANCELLATION", 1),
		"去向缺席":    strings.Replace(conflictDecisionBody, `"decision":"CANCELLATION_NOT_UPHELD",`, "", 1),
		"边界种类认不出": strings.Replace(conflictDecisionBody, "NETWORK_INTAKE", "SCAN", 1),
		"依据空白":    strings.Replace(conflictDecisionBody, `"basis":"SYN-NODE-SCAN"`, `"basis":"  "`, 1),
		"缺包裹":     strings.Replace(conflictDecisionBody, `"parcel":"SYN-PARCEL-01",`, "", 1),
		"缺边界那一代":  strings.Replace(conflictDecisionBody, `"boundaryRef":"NODE_INTAKE/v1",`, "", 1),
		"缺取消":     strings.Replace(conflictDecisionBody, `"cancellation":"SYN-PCXL-01",`, "", 1),
		"尾随内容":    conflictDecisionBody + `{}`,
		"不是 JSON": `not json`,
	} {
		if _, err := intake.IntakeCancellationOrderConflictDecision(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}

	if _, err := intake.IntakeCancellationOrderConflictDecision(ctx, decisionRequest("", conflictDecisionBody)); !errors.Is(err, shipmenthttp.ErrOperatorCredentialRejected) {
		t.Errorf("没有令牌: err = %v, want ErrOperatorCredentialRejected", err)
	}
	notGranted := decisionIntake(t, decisionAuthenticator{err: shipmenthttp.ErrOperatorNotGranted}, decisionTargets{})
	if _, err := notGranted.IntakeCancellationOrderConflictDecision(ctx, decisionRequest("t", `not even json`)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Errorf("没有授予: err = %v, want ErrOperatorNotGranted（先认证、后解载荷）", err)
	}
}

type conflictDecisionIntakeDouble struct {
	command application.DecideCancellationOrderConflictCommand
	err     error
}

func (double conflictDecisionIntakeDouble) IntakeCancellationOrderConflictDecision(context.Context, *http.Request) (application.DecideCancellationOrderConflictCommand, error) {
	return double.command, double.err
}

type conflictDecisionHandlerDouble struct {
	result application.DecideCancellationOrderConflictResult
	err    error
	called int
}

func (double *conflictDecisionHandlerDouble) Handle(context.Context, application.DecideCancellationOrderConflictCommand) (application.DecideCancellationOrderConflictResult, error) {
	double.called++
	return double.result, double.err
}

func postConflictDecision(intake shipmenthttp.CancellationOrderConflictDecisionIntake, handler shipmenthttp.CancellationOrderConflictDecisionHandler, method string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	shipmenthttp.NewDecideCancellationOrderConflictEndpoint(intake, handler).
		ServeHTTP(response, httptest.NewRequest(method, "/cancellation-order-conflict-decisions", strings.NewReader(conflictDecisionBody)))
	return response
}

// Covers: AT-PS-166；ADR-0022——失败各归其格：不可见 404 带自己的码；没形成答案 500 不带 outcome；未命名结果 500；Intake 各格先于编排、编排一次不被调；非 POST 405。
func TestTheConflictDecisionEndpointMapsFailuresToTheirOwnCells(t *testing.T) {
	handler := &conflictDecisionHandlerDouble{err: application.ErrCancellationOrderConflictNotVisible}
	response := postConflictDecision(conflictDecisionIntakeDouble{}, handler, http.MethodPost)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE") {
		t.Fatalf("不可见 = %d %s, want 404 CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE", response.Code, response.Body.String())
	}
	handler = &conflictDecisionHandlerDouble{err: errors.New("boom")}
	response = postConflictDecision(conflictDecisionIntakeDouble{}, handler, http.MethodPost)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "outcome") {
		t.Fatalf("编排报错 = %d %s, want 500 不带 outcome", response.Code, response.Body.String())
	}
	for err, status := range map[error]int{
		shipmenthttp.ErrAccessChannelNotConfigured: http.StatusForbidden,
		shipmenthttp.ErrOperatorNotGranted:         http.StatusForbidden,
		shipmenthttp.ErrOperatorCredentialRejected: http.StatusUnauthorized,
		shipmenthttp.ErrMalformedRequest:           http.StatusBadRequest,
	} {
		handler = &conflictDecisionHandlerDouble{}
		response = postConflictDecision(conflictDecisionIntakeDouble{err: err}, handler, http.MethodPost)
		if response.Code != status || handler.called != 0 {
			t.Errorf("Intake %v = %d（编排被调 %d 次）, want %d 且编排不被调", err, response.Code, handler.called, status)
		}
	}
	// 没有名字的结果是编程错误：不带 outcome 上线。每一格的 201/200 映射与回执不带操作者，由 cmd/parcel-api 的真库用例逐格钉——结果字段未导出是刻意的，传输层
	// 不该能捏造业务结果。
	response = postConflictDecision(conflictDecisionIntakeDouble{}, &conflictDecisionHandlerDouble{}, http.MethodPost)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
		t.Fatalf("未命名结果 = %d %s, want 500 UNNAMED_OUTCOME", response.Code, response.Body.String())
	}
	handler = &conflictDecisionHandlerDouble{}
	response = postConflictDecision(conflictDecisionIntakeDouble{}, handler, http.MethodGet)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost || handler.called != 0 {
		t.Fatalf("GET = %d Allow=%q, want 405 与 Allow: POST", response.Code, response.Header().Get("Allow"))
	}
	if response := postConflictDecision(shipmenthttp.UnconfiguredIntake{}, &conflictDecisionHandlerDouble{}, http.MethodPost); response.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", response.Code)
	}
}
