package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

type capturingCloser struct {
	command application.CloseCustomsCaseCommand
	calls   int
}

func (fake *capturingCloser) Handle(_ context.Context, command application.CloseCustomsCaseCommand) (application.CloseCustomsCaseResult, error) {
	fake.command = command
	fake.calls++
	return application.CloseCustomsCaseResult{}, nil
}

// Covers: UC-CC-010 关闭入口（Intake 半边，票 launch-wiring/02）——关闭口按关务案件关闭这一种运营决定认证；租户与决定人取认证结果，
// 载荷里只有案件与业务截点；载荷是封闭的（不收租户、决定人），截点不是 RFC 3339 是坏报文，编排一次也不被调到。
func TestTheCaseClosureFaceTakesIdentityFromTheEnvelopeAndRefusesAnythingElse(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	closer := &capturingCloser{}
	endpoint := customshttp.NewCloseCaseEndpoint(intake, closer)
	endpoint.ServeHTTP(httptest.NewRecorder(), post("/customs-case-closures", `{"caseId":"CASE-1","cutoffAt":"2026-10-26T00:00:00Z"}`))
	if len(asked) != 1 || asked[0] != "CUSTOMS_CASE_CLOSURE" {
		t.Fatalf("asked = %v", asked)
	}
	command := closer.command
	if command.TenantID.String() != "SYN-TENANT-01" || command.CaseRef.String() != "CASE-1" ||
		command.DecidedBy != "https://issuer#alice" || command.CutoffAt.IsZero() {
		t.Fatalf("command = %+v", command)
	}

	refused := &capturingCloser{}
	endpoint = customshttp.NewCloseCaseEndpoint(intake, refused)
	for name, body := range map[string]string{
		"带租户":    `{"tenantId":"T","caseId":"CASE-1"}`,
		"带决定人":   `{"caseId":"CASE-1","decidedBy":"bob"}`,
		"截点不是时刻": `{"caseId":"CASE-1","cutoffAt":"tomorrow"}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/customs-case-closures", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d，want 400", name, response.Code)
		}
	}
	if refused.calls != 0 {
		t.Fatalf("坏报文把编排调了 %d 次", refused.calls)
	}
}
