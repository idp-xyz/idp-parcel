package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

type capturingLodgementRecorder struct {
	commands []application.RecordManualLodgementCommand
}

func (fake *capturingLodgementRecorder) Record(_ context.Context, command application.RecordManualLodgementCommand) (application.RecordManualLodgementResult, error) {
	fake.commands = append(fake.commands, command)
	return application.RecordManualLodgementResult{}, nil
}

// Covers: dox/decisions.md「产品形态」——线下递交标记口按它自己那一种决定认证；租户与递交人只取认证结果；递交凭据必填，
// 递交时刻给了却不成形、带了递交人一类的格、凭据带控制字符都是坏报文，编排一次也不被调到。
func TestTheManualLodgementFaceAuthenticatesItsOwnKindAndRequiresTheLodgementSlip(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &capturingLodgementRecorder{}
	endpoint := customshttp.NewRecordManualLodgementEndpoint(intake, recorder)

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, post("/probe", `{"version":" SYN-VERSION-1 ","lodgedAt":"2026-11-13T08:00:00+08:00","reference":" ACK-1 ","original":" SLIP-1 "}`))
	if len(recorder.commands) != 1 || len(asked) != 1 || asked[0] != "CUSTOMS_MANUAL_LODGEMENT" {
		t.Fatalf("answer = %d %s, calls = %d, asked = %v", response.Code, response.Body.String(), len(recorder.commands), asked)
	}
	// 替身交回一份没有名字的答复：答 500，不是空 200。
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("没有名字的答复 = %d，想要 500", response.Code)
	}
	command := recorder.commands[0]
	if command.TenantID.String() != "SYN-TENANT-01" || command.LodgedBy != "OPERATOR/https://issuer#alice" || command.Version != "SYN-VERSION-1" ||
		command.Reference != "ACK-1" || command.Original != "SLIP-1" || command.LodgedAt.Hour() != 0 {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"没有凭据":    `{"version":"V"}`,
		"时刻不成形":   `{"version":"V","original":"O","lodgedAt":"昨天"}`,
		"自报递交人":   `{"version":"V","original":"O","lodgedBy":"someone"}`,
		"凭据带控制字符": `{"version":"V","original":"O\u0000"}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/probe", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s，想要 400", name, response.Code, response.Body.String())
		}
	}
	if len(recorder.commands) != 1 {
		t.Fatalf("坏报文到了编排：%d 次", len(recorder.commands))
	}
}
