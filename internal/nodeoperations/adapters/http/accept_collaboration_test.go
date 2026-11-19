package nodeopshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证节点监管协作两口的接入面（UC-NO-001；ADR-0338）：租户只从信封来，载荷封闭，词表外的决定、动作与缺口答 400；执行事实口先认证
// 后解载荷，作业范围种类只认 NODE。编排是替身，只记下译出的命令。夹具全为合成（S 级）。

type collaborationDecisionAuthenticator struct{ err error }

func (authenticator collaborationDecisionAuthenticator) AuthenticateOperatorDecision(
	_ context.Context, _ string, decision nodeopshttp.OperatorDecision,
) (nodeopshttp.OperatorIdentity, error) {
	if authenticator.err != nil {
		return nodeopshttp.OperatorIdentity{}, authenticator.err
	}
	if decision != nodeopshttp.OperatorDecisionNodeCollaborationAcceptance {
		return nodeopshttp.OperatorIdentity{}, errors.New("wrong decision kind")
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return nodeopshttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example#SYN-OPERATOR"}, err
}

type collaborationExecutionAuthenticator struct{}

func (collaborationExecutionAuthenticator) AuthenticateCollaborationExecution(
	_ context.Context, _, _, _, reference string,
) (domain.TenantID, domain.NodeReference, string, error) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return domain.TenantID{}, domain.NodeReference{}, "", err
	}
	node, err := domain.NewNodeReference(reference)
	return tenant, node, "https://id.syn.example#SYN-DEVICE-OPERATOR", err
}

type recordingCollaboration struct {
	accepted []application.AcceptCollaborationCommand
	executed []application.RecordExecutionCommand
	outcome  application.CollaborationResult
}

func (handler *recordingCollaboration) Accept(_ context.Context, command application.AcceptCollaborationCommand) (application.CollaborationResult, error) {
	handler.accepted = append(handler.accepted, command)
	return handler.outcome, nil
}

func (handler *recordingCollaboration) RecordExecution(_ context.Context, command application.RecordExecutionCommand) (application.CollaborationResult, error) {
	handler.executed = append(handler.executed, command)
	return handler.outcome, nil
}

const validAcceptance = `{"node":"N1","item":"I1","decision":"ACCEPTED","acceptedUnits":["U1"],"acceptedActions":["UNSEAL"],"authority":"A1","decidedAt":"2026-10-27T08:00:00Z"}`

func TestTheCollaborationAcceptanceIntakeTakesTheTenantFromTheEnvelopeAndClosesThePayload(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(collaborationDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"unknown field":       strings.Replace(validAcceptance, `"item":"I1"`, `"item":"I1","tenant":"SYN-OTHER"`, 1),
		"decision off list":   strings.Replace(validAcceptance, `"ACCEPTED"`, `"MAYBE"`, 1),
		"action off list":     strings.Replace(validAcceptance, `"UNSEAL"`, `"RELEASE"`, 1),
		"gap off list":        strings.Replace(validAcceptance, `"authority"`, `"gaps":["WEATHER"],"authority"`, 1),
		"missing decidedAt":   strings.Replace(validAcceptance, `,"decidedAt":"2026-10-27T08:00:00Z"`, ``, 1),
		"blank authority":     strings.Replace(validAcceptance, `"A1"`, `" "`, 1),
		"blank accepted unit": strings.Replace(validAcceptance, `"U1"`, `" "`, 1),
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if _, err := intake.IntakeCollaborationAcceptance(request.Context(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want malformed request", name, err)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(validAcceptance))
	command, err := intake.IntakeCollaborationAcceptance(request.Context(), request)
	if err != nil {
		t.Fatalf("valid acceptance: %v", err)
	}
	if command.TenantID.String() != "SYN-TENANT-01" || command.Operator != "https://id.syn.example#SYN-OPERATOR" || command.Decision != domain.CollaborationAccepted ||
		len(command.AcceptedActions) != 1 || command.AcceptedActions[0] != domain.UnsealAction {
		t.Fatalf("command = %+v", command)
	}
}

func TestTheCollaborationExecutionIntakeAuthenticatesBeforeReadingAndOnlyTakesANodeScope(t *testing.T) {
	intake, err := nodeopshttp.NewWorkFactCollaborationIntake(collaborationExecutionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	const body = `{"item":"I1","unit":"U1","action":"UNSEAL","evidence":"E1","performedAt":"2026-10-27T09:00:00Z"}`
	build := func(kind, payload string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		request.Header.Set("Parcel-Device-Id", "D1")
		request.Header.Set("Parcel-Work-Scope-Kind", kind)
		request.Header.Set("Parcel-Work-Scope-Reference", "N1")
		return request
	}
	for name, request := range map[string]*http.Request{
		"offsite scope":   build("OFFSITE", body),
		"action off list": build("NODE", strings.Replace(body, "UNSEAL", "RELEASE", 1)),
		"no evidence":     build("NODE", strings.Replace(body, `"evidence":"E1",`, ``, 1)),
	} {
		if _, err := intake.IntakeCollaborationExecution(request.Context(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want malformed request", name, err)
		}
	}
	request := build("NODE", body)
	command, err := intake.IntakeCollaborationExecution(request.Context(), request)
	if err != nil {
		t.Fatalf("valid execution: %v", err)
	}
	if command.Node.String() != "N1" || command.Operator != "https://id.syn.example#SYN-DEVICE-OPERATOR" || command.Action != domain.UnsealAction || command.TenantID.String() != "SYN-TENANT-01" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: 执行事实口的作业范围种类不是 NODE——Intake 交回的错误文本逐字（头名拼在其中；服务端日志靠这句话认出是哪个口），经执行事实
// 端点是 400 与结果码 MALFORMED_REQUEST，响应体不带问题详情文本、也不带任何别的格；编排一次也不调。头名换了出处之后文本与应答都不许变。
func TestTheCollaborationExecutionIntakeAnswersANonNodeScopeWithTheExactTextStatusAndCode(t *testing.T) {
	const (
		body     = `{"item":"I1","unit":"U1","action":"UNSEAL","evidence":"E1","performedAt":"2026-10-27T09:00:00Z"}`
		wantText = "node operations http: malformed request: a collaboration execution fact is recorded at a node; Parcel-Work-Scope-Kind must be NODE"
	)
	intake, err := nodeopshttp.NewWorkFactCollaborationIntake(collaborationExecutionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	build := func(kind string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Parcel-Device-Id", "D1")
		request.Header.Set("Parcel-Work-Scope-Kind", kind)
		request.Header.Set("Parcel-Work-Scope-Reference", "N1")
		return request
	}
	for _, kind := range []string{"OFFSITE", "node"} {
		request := build(kind)
		_, err := intake.IntakeCollaborationExecution(request.Context(), request)
		if !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("种类 %q：err = %v, want ErrMalformedRequest", kind, err)
		}
		if err.Error() != wantText {
			t.Fatalf("种类 %q：错误文本 = %q, want %q", kind, err.Error(), wantText)
		}

		handler := &recordingCollaboration{}
		response := httptest.NewRecorder()
		nodeopshttp.NewRecordCollaborationExecutionEndpoint(intake, handler).ServeHTTP(response, build(kind))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("种类 %q：status = %d, want 400", kind, response.Code)
		}
		if got, want := response.Body.String(), `{"error":{"code":"MALFORMED_REQUEST"}}`+"\n"; got != want {
			t.Fatalf("种类 %q：body = %q, want %q", kind, got, want)
		}
		if len(handler.accepted)+len(handler.executed) != 0 {
			t.Fatalf("种类 %q：被拒的请求进了编排", kind)
		}
	}
}

func TestTheCollaborationEndpointsAnswerMethodAndIntakeFailuresBeforeTheOrchestration(t *testing.T) {
	handler := &recordingCollaboration{}
	denied, err := nodeopshttp.NewOperatorDecisionIntake(collaborationDecisionAuthenticator{err: nodeopshttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := nodeopshttp.NewAcceptCollaborationEndpoint(denied, handler)
	for method, want := range map[string]int{http.MethodGet: http.StatusMethodNotAllowed, http.MethodPost: http.StatusForbidden} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(method, "/", strings.NewReader(validAcceptance)))
		if response.Code != want {
			t.Fatalf("%s = %d, want %d", method, response.Code, want)
		}
	}
	unconfigured := nodeopshttp.NewRecordCollaborationExecutionEndpoint(nodeopshttp.UnconfiguredIntake{}, handler)
	response := httptest.NewRecorder()
	unconfigured.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
	if response.Code != http.StatusForbidden || len(handler.accepted)+len(handler.executed) != 0 {
		t.Fatalf("unconfigured = %d, handler calls = %d", response.Code, len(handler.accepted)+len(handler.executed))
	}
}
