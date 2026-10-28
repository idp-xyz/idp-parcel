package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

type capturingGateVerifier struct {
	command application.VerifyReleaseGateCommand
	calls   int
}

func (fake *capturingGateVerifier) Handle(_ context.Context, command application.VerifyReleaseGateCommand) (application.VerifyReleaseGateResult, error) {
	fake.command = command
	fake.calls++
	return application.VerifyReleaseGateResult{}, nil
}

// Covers: UC-CC-009 门禁核对（Intake 半边，票 launch-wiring/03）——核对口按放行门禁核对这一种运营决定认证；租户取认证结果，载荷里只有申报范围、
// 拟执行动作与监管边界三格；载荷是封闭的（不收租户），动作不在封闭四值里是坏报文，编排一次也不被调到。
func TestTheReleaseGateVerificationFaceTakesTenantFromTheEnvelopeAndRefusesAnythingElse(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &capturingGateVerifier{}
	customshttp.NewVerifyReleaseGateEndpoint(intake, verifier).ServeHTTP(httptest.NewRecorder(),
		post("/customs-release-gate-verifications", `{"scope":"SCOPE-1","action":"OUTBOUND_RELEASE","boundary":"PROC-1"}`))
	if len(asked) != 1 || asked[0] != "RELEASE_GATE_VERIFICATION" {
		t.Fatalf("asked = %v", asked)
	}
	command := verifier.command
	if command.TenantID.String() != "SYN-TENANT-01" || command.Scope.String() != "SCOPE-1" ||
		command.Action != domain.OutboundRelease || command.Boundary.String() != "PROC-1" {
		t.Fatalf("command = %+v", command)
	}

	refused := &capturingGateVerifier{}
	endpoint := customshttp.NewVerifyReleaseGateEndpoint(intake, refused)
	for name, body := range map[string]string{
		"带租户":      `{"tenantId":"T","scope":"S"}`,
		"动作不在封闭集合": `{"scope":"S","action":"RECEIVE","boundary":"P"}`,
		"多余内容":     `{"scope":"S"} {}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/customs-release-gate-verifications", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d，want 400", name, response.Code)
		}
	}
	if refused.calls != 0 {
		t.Fatalf("坏报文把编排调了 %d 次", refused.calls)
	}
}
