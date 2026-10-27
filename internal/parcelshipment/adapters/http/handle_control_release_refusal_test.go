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
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证释放拒收处理的传输层（ADR-0293 决定二、三）：Intake 先认证（决定种类是 CONTROL_RELEASE_REFUSAL_HANDLING，与授权处置分开）、后封闭解载荷，
// 来源身份由服务端按信封租户与委托标识查出；端点把处理结果各映射到自己的格——已处理 201，其余业务结果 200 带 outcome，不可见 404，
// 没形成答案 500；回执不回显任何一位操作者。

const refusalHandlingBody = `{"shipmentRequestId":"SYN-REQ-01","submissionVersionId":"SYN-VERSION-01","controlResultId":"SYN-REQ-01/SYN-VERSION-01","action":"MARK_HANDLED","basis":"SYN-CHECKED"}`

// Covers: AT-PS-164；ADR-0151 决定二、ADR-0293 决定三——Intake 用「释放拒收处理」这一种决定认证，租户取信封；处理人取认证出的操作者；来源身份按信封租户与
// 委托标识查出；动作是封闭两值；依据必填。
func TestTheRefusalHandlingIntakeAuthenticatesItsOwnDecisionAndTakesTheIdentityFromTheEnvelope(t *testing.T) {
	var asked []shipmenthttp.OperatorDecision
	intake := decisionIntake(t, decisionAuthenticator{asked: &asked}, decisionTargets{})

	command, err := intake.IntakeControlReleaseRefusalHandling(context.Background(), decisionRequest("t", refusalHandlingBody))
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if len(asked) != 1 || asked[0] != shipmenthttp.OperatorDecisionControlReleaseRefusalHandling ||
		string(asked[0]) != "CONTROL_RELEASE_REFUSAL_HANDLING" {
		t.Fatalf("认证用的决定种类 = %v, want CONTROL_RELEASE_REFUSAL_HANDLING——与授权处置共用就是授权处置的授予能销账", asked)
	}
	if command.Identity.TenantID().String() != decisionTenant || command.Identity.CustomerAccountID().String() != "SYN-ACCOUNT-01" ||
		command.ShipmentRequestID.String() != "SYN-REQ-01" || command.SubmissionVersion.String() != "SYN-VERSION-01" ||
		command.ControlResultID.String() != "SYN-REQ-01/SYN-VERSION-01" || command.Action != application.MarkControlReleaseRefusalHandled ||
		command.Handler.String() != decisionOperator || command.Basis.String() != "SYN-CHECKED" {
		t.Fatalf("命令 = %+v", command)
	}

	released, err := intake.IntakeControlReleaseRefusalHandling(context.Background(), decisionRequest("t",
		strings.Replace(refusalHandlingBody, "MARK_HANDLED", "RELEASE_AT_SETTLEMENT", 1)))
	if err != nil || released.Action != application.ReleaseControlReleaseRefusalAtSettlement {
		t.Fatalf("释放动作 = %v err=%v", released.Action, err)
	}
}

// Covers: AT-PS-164；ADR-0151 决定二——先认证、后解载荷；载荷里任何身份格（租户、客户账户、处理人）封闭解码即拒；缺字段、动作认不出、依据空白答坏报文；
// 委托查不到与没授予同答（越权探针同答）。
func TestTheRefusalHandlingIntakeRefusesAMalformedPayloadAndHidesForeignRequests(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	ctx := context.Background()

	for name, body := range map[string]string{
		"带租户":     strings.Replace(refusalHandlingBody, `"action"`, `"tenantId":"SYN-TENANT-99","action"`, 1),
		"带处理人":    strings.Replace(refusalHandlingBody, `"action"`, `"handler":"someone","action"`, 1),
		"动作认不出":   strings.Replace(refusalHandlingBody, "MARK_HANDLED", "DELETE_ROW", 1),
		"动作缺席":    strings.Replace(refusalHandlingBody, `"action":"MARK_HANDLED",`, "", 1),
		"依据空白":    strings.Replace(refusalHandlingBody, `"basis":"SYN-CHECKED"`, `"basis":"  "`, 1),
		"缺版本":     strings.Replace(refusalHandlingBody, `"submissionVersionId":"SYN-VERSION-01",`, "", 1),
		"缺控制结果":   strings.Replace(refusalHandlingBody, `"controlResultId":"SYN-REQ-01/SYN-VERSION-01",`, "", 1),
		"尾随内容":    refusalHandlingBody + `{}`,
		"不是 JSON": `not json`,
	} {
		if _, err := intake.IntakeControlReleaseRefusalHandling(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}

	missing := decisionIntake(t, decisionAuthenticator{}, decisionTargets{missing: true})
	if _, err := missing.IntakeControlReleaseRefusalHandling(ctx, decisionRequest("t", refusalHandlingBody)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Errorf("委托查不到: err = %v, want ErrOperatorNotGranted（与越权探针同答）", err)
	}
	if _, err := intake.IntakeControlReleaseRefusalHandling(ctx, decisionRequest("", refusalHandlingBody)); !errors.Is(err, shipmenthttp.ErrOperatorCredentialRejected) {
		t.Errorf("没有令牌: err = %v, want ErrOperatorCredentialRejected", err)
	}
	notGranted := decisionIntake(t, decisionAuthenticator{err: shipmenthttp.ErrOperatorNotGranted}, decisionTargets{})
	if _, err := notGranted.IntakeControlReleaseRefusalHandling(ctx, decisionRequest("t", `not even json`)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Errorf("没有授予: err = %v, want ErrOperatorNotGranted（先认证、后解载荷）", err)
	}
}

// Covers: AT-PS-164；ADR-0151 决定二——租户只取自认证出的信封：租户 B 的操作者在请求上自报 `?tenant=` 别的租户，委托寻址与命令里的租户仍是 B。
func TestTheRefusalHandlingIntakeIgnoresATenantTheRequestClaims(t *testing.T) {
	asked := &askedTenants{}
	intake := decisionIntake(t, decisionAuthenticator{}, asked)
	request := decisionRequest("t", refusalHandlingBody)
	request.URL.RawQuery = "tenant=SYN-TENANT-99"

	command, err := intake.IntakeControlReleaseRefusalHandling(context.Background(), request)
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if command.Identity.TenantID().String() != decisionTenant || len(asked.tenants) != 1 || asked.tenants[0] != decisionTenant {
		t.Fatalf("命令的租户 = %q，委托寻址问的租户 = %v, want 信封里的 %s，请求里自报的一概不采信",
			command.Identity.TenantID(), asked.tenants, decisionTenant)
	}
}

// askedTenants 记下委托寻址被问的租户，其余同 decisionTargets。
type askedTenants struct{ tenants []string }

func (fake *askedTenants) FindOperatorDecisionTarget(ctx context.Context, tenant domain.TenantID, request domain.ShipmentRequestID) (ports.OperatorDecisionTarget, bool, error) {
	fake.tenants = append(fake.tenants, tenant.String())
	return decisionTargets{}.FindOperatorDecisionTarget(ctx, tenant, request)
}

type refusalHandlingIntakeDouble struct {
	command application.HandleControlReleaseRefusalCommand
	err     error
}

func (double refusalHandlingIntakeDouble) IntakeControlReleaseRefusalHandling(context.Context, *http.Request) (application.HandleControlReleaseRefusalCommand, error) {
	return double.command, double.err
}

type refusalHandlingHandlerDouble struct {
	result application.HandleControlReleaseRefusalResult
	err    error
	called int
}

func (double *refusalHandlingHandlerDouble) Handle(context.Context, application.HandleControlReleaseRefusalCommand) (application.HandleControlReleaseRefusalResult, error) {
	double.called++
	return double.result, double.err
}

func postRefusalHandling(intake shipmenthttp.ControlReleaseRefusalHandlingIntake, handler shipmenthttp.ControlReleaseRefusalHandlingHandler, method string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	shipmenthttp.NewHandleControlReleaseRefusalEndpoint(intake, handler).
		ServeHTTP(response, httptest.NewRequest(method, "/control-release-refusal-handlings", strings.NewReader(refusalHandlingBody)))
	return response
}

// Covers: AT-PS-164；ADR-0022——失败各归其格：不可见 404；没形成答案 500 不带 outcome；未命名结果 500；Intake 各格先于编排、编排一次不被调；非 POST 405。
func TestTheRefusalHandlingEndpointMapsFailuresToTheirOwnCells(t *testing.T) {
	handler := &refusalHandlingHandlerDouble{err: application.ErrShipmentRequestNotVisible}
	response := postRefusalHandling(refusalHandlingIntakeDouble{}, handler, http.MethodPost)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SHIPMENT_REQUEST_NOT_VISIBLE") {
		t.Fatalf("不可见 = %d %s, want 404 SHIPMENT_REQUEST_NOT_VISIBLE", response.Code, response.Body.String())
	}
	handler = &refusalHandlingHandlerDouble{err: errors.New("boom")}
	response = postRefusalHandling(refusalHandlingIntakeDouble{}, handler, http.MethodPost)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "outcome") {
		t.Fatalf("编排报错 = %d %s, want 500 不带 outcome", response.Code, response.Body.String())
	}
	for err, status := range map[error]int{
		shipmenthttp.ErrAccessChannelNotConfigured: http.StatusForbidden,
		shipmenthttp.ErrOperatorNotGranted:         http.StatusForbidden,
		shipmenthttp.ErrOperatorCredentialRejected: http.StatusUnauthorized,
		shipmenthttp.ErrMalformedRequest:           http.StatusBadRequest,
	} {
		handler = &refusalHandlingHandlerDouble{}
		response = postRefusalHandling(refusalHandlingIntakeDouble{err: err}, handler, http.MethodPost)
		if response.Code != status || handler.called != 0 {
			t.Errorf("Intake %v = %d（编排被调 %d 次）, want %d 且编排不被调", err, response.Code, handler.called, status)
		}
	}
	// 没有名字的结果是编程错误：不带 outcome 上线。每一格的 201/200 映射与回执不带操作者，由 cmd/parcel-api 的真库用例逐格钉——结果字段
	// 未导出是刻意的，传输层不该能捏造业务结果。
	response = postRefusalHandling(refusalHandlingIntakeDouble{}, &refusalHandlingHandlerDouble{}, http.MethodPost)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
		t.Fatalf("未命名结果 = %d %s, want 500 UNNAMED_OUTCOME", response.Code, response.Body.String())
	}
	handler = &refusalHandlingHandlerDouble{}
	response = postRefusalHandling(refusalHandlingIntakeDouble{}, handler, http.MethodGet)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost || handler.called != 0 {
		t.Fatalf("GET = %d Allow=%q, want 405 与 Allow: POST", response.Code, response.Header().Get("Allow"))
	}
	if response := postRefusalHandling(shipmenthttp.UnconfiguredIntake{}, &refusalHandlingHandlerDouble{}, http.MethodPost); response.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", response.Code)
	}
}
