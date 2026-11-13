package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

type capturingResultHandler struct {
	commands []application.ReceiveExternalResultCommand
}

func (fake *capturingResultHandler) Handle(_ context.Context, command application.ReceiveExternalResultCommand) (application.ReceiveExternalResultResult, error) {
	fake.commands = append(fake.commands, command)
	return application.ReceiveExternalResultResult{}, nil
}

const manualResultBody = `{"sourceId":"SYN-RESULT-MANUAL-1","layer":"PROCESS_DECISION","role":"SYN-AUTHORITY-1","rawSemantics":"SYN-RAW-RELEASED",` +
	`"claimedVersion":"SYN-CC-VERSION-1","attempt":1,"scope":"SYN-CC-UNIT-1","occurredAt":"2026-11-13T01:00:00Z","original":" SYN-RECEIPT-NO-01 "}`

// Covers: dox/decisions.md「产品形态」——海关结果的手工录入口按「人工录入海关结果」这一种决定认证；租户与录入人只取认证
// 结果，接收时刻由服务端给；载荷与集成客户端口同一份形状，另必带 original；没带原件、带了租户或录入人一类身份格都是
// 坏报文，编排一次也不被调到。
func TestTheManualResultFaceAuthenticatesItsOwnKindAndRecordsWhoEnteredFromWhichOriginal(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	handler := &capturingResultHandler{}
	now := time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC)
	endpoint := customshttp.NewReceiveExternalResultEndpoint(customshttp.ManualResultIntake(intake, func() time.Time { return now }), handler)

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, post("/probe", manualResultBody))
	if len(handler.commands) != 1 || len(asked) != 1 || asked[0] != "CUSTOMS_EXTERNAL_RESULT_ENTRY" {
		t.Fatalf("answer = %d %s, calls = %d, asked = %v", response.Code, response.Body.String(), len(handler.commands), asked)
	}
	command := handler.commands[0]
	if command.TenantID.String() != "SYN-TENANT-01" || command.EnteredBy != "OPERATOR/https://issuer#alice" || command.Original != "SYN-RECEIPT-NO-01" ||
		command.SourceID != "SYN-RESULT-MANUAL-1" || !command.ReceivedAt.Equal(now) || command.OccurredAt.IsZero() {
		t.Fatalf("command = %+v", command)
	}

	for name, body := range map[string]string{
		"没有原件":   `{"sourceId":"S","layer":"PROCESS_DECISION","role":"R","rawSemantics":"X","claimedVersion":"V","attempt":1,"scope":"U","occurredAt":"2026-11-13T01:00:00Z"}`,
		"自报租户":   `{"tenantId":"other","sourceId":"S","rawSemantics":"X","claimedVersion":"V","original":"O"}`,
		"自报录入人":  `{"enteredBy":"someone","sourceId":"S","rawSemantics":"X","claimedVersion":"V","original":"O"}`,
		"自报接收时刻": `{"receivedAt":"2020-01-01T00:00:00Z","sourceId":"S","rawSemantics":"X","claimedVersion":"V","original":"O"}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/probe", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s，想要 400", name, response.Code, response.Body.String())
		}
	}
	if len(handler.commands) != 1 {
		t.Fatalf("坏报文到了编排：%d 次", len(handler.commands))
	}
}
