package customshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

const technicalReceiptBody = `{"sourceId":"SYN-RCPT-0001","messageId":"SYN-MSG-0001","outcome":"FAILED","occurredAt":"2026-10-28T17:00:00+08:00"}`

// 回执的身份与内容摘要只出自它说的话：同一份回执晚到一次，摘要不变（不比到达时刻）；说法变了，摘要跟着变。
// Covers: AT-CC-455（Intake 半边）
func TestIntegrationClientIntakeTranslatesATechnicalReceipt(t *testing.T) {
	intake := clientIntakeForTest(t, nil)
	command, err := intake.IntakeTechnicalReceipt(context.Background(), commandRequest(technicalReceiptBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != clientTenant || command.Source != "SYN-RCPT-0001" || command.ChannelMessage != "SYN-MSG-0001" ||
		command.Outcome != domain.AttemptFailed || !command.ReceivedAt.Equal(clientReceivedAt) || !strings.HasPrefix(command.Digest, "TR-1:") {
		t.Fatalf("command = %+v", command)
	}
	late, _ := customshttp.NewIntegrationClientIntake(clientAuthenticator{}, fixedClock{at: clientReceivedAt.Add(1e9)})
	again, _ := late.IntakeTechnicalReceipt(context.Background(), commandRequest(technicalReceiptBody))
	changed, _ := intake.IntakeTechnicalReceipt(context.Background(),
		commandRequest(strings.Replace(technicalReceiptBody, "FAILED", "ACKNOWLEDGED", 1)))
	if again.Digest != command.Digest || changed.Digest == command.Digest {
		t.Fatalf("摘要 原=%s 晚到=%s 改说法=%s", command.Digest, again.Digest, changed.Digest)
	}
	for name, body := range map[string]string{
		"PENDING 不是回执":  strings.Replace(technicalReceiptBody, "FAILED", "PENDING_CONFIRMATION", 1),
		"时刻不是 RFC 3339": strings.Replace(technicalReceiptBody, "2026-10-28T17:00:00+08:00", "yesterday", 1),
		"未知键":           strings.Replace(technicalReceiptBody, `"sourceId"`, `"tenant":"SYN-TENANT-02","sourceId"`, 1),
	} {
		if _, err := intake.IntakeTechnicalReceipt(context.Background(), commandRequest(body)); !errors.Is(err, customshttp.ErrMalformedRequest) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
}

type receiptHandlerDouble struct {
	result application.TechnicalReceiptResult
	err    error
}

func (double receiptHandlerDouble) Handle(context.Context, application.ReceiveTechnicalReceiptCommand) (application.TechnicalReceiptResult, error) {
	return double.result, double.err
}

func TestTheTechnicalReceiptEndpointGradesItsAnswers(t *testing.T) {
	endpoint := customshttp.NewReceiveTechnicalReceiptEndpoint(clientIntakeForTest(t, nil), receiptHandlerDouble{})
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs/technical-receipts", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", response.Code)
	}
	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, commandRequest(technicalReceiptBody))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("零结果 = %d，不该被当成答案", response.Code)
	}
	failing := customshttp.NewReceiveTechnicalReceiptEndpoint(clientIntakeForTest(t, nil), receiptHandlerDouble{err: errors.New("boom")})
	response = httptest.NewRecorder()
	failing.ServeHTTP(response, commandRequest(technicalReceiptBody))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("编排失败 = %d %s", response.Code, response.Body)
	}
	rejected := customshttp.NewReceiveTechnicalReceiptEndpoint(clientIntakeForTest(t,
		clientAuthenticator{err: customshttp.ErrIntegrationClientCredentialRejected}), receiptHandlerDouble{})
	response = httptest.NewRecorder()
	rejected.ServeHTTP(response, commandRequest(technicalReceiptBody))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("凭据不认 = %d", response.Code)
	}
}
