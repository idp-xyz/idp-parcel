package warehousehttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type receiptAuthenticatorStub struct {
	calls     int
	device    string
	kind      string
	reference string
	err       error
}

func (stub *receiptAuthenticatorStub) AuthenticateReceiptConfirmation(
	_ context.Context,
	_, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	stub.calls++
	stub.device, stub.kind, stub.reference = deviceID, scopeKind, scopeReference
	if stub.err != nil {
		return domain.TenantID{}, domain.ReceiptOperatorID{}, domain.ReceiptDeviceID{}, stub.err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return domain.TenantID{}, domain.ReceiptOperatorID{}, domain.ReceiptDeviceID{}, err
	}
	operator, err := domain.NewReceiptOperatorID("https://id.syn.example/dex#SYN-OPERATOR-01")
	if err != nil {
		return domain.TenantID{}, domain.ReceiptOperatorID{}, domain.ReceiptDeviceID{}, err
	}
	device, err := domain.NewReceiptDeviceID(deviceID)
	if err != nil {
		return domain.TenantID{}, domain.ReceiptOperatorID{}, domain.ReceiptDeviceID{}, err
	}
	return tenant, operator, device, nil
}

type receiptConfirmerStub struct {
	calls     int
	command   application.ConfirmReceiptCommand
	result    application.ConfirmReceiptResult
	hasResult bool
	err       error
}

func (stub *receiptConfirmerStub) Confirm(_ context.Context, command application.ConfirmReceiptCommand) (application.ConfirmReceiptResult, error) {
	stub.calls++
	stub.command = command
	if stub.err != nil {
		return application.ConfirmReceiptResult{}, stub.err
	}
	if stub.hasResult {
		return stub.result, nil
	}
	return application.ConfirmReceiptResult{}, errors.New("no answer")
}

func receiptConfirmationDocument(extra string) string {
	body := `{"factId":"SYN-FACT-01","occurredAt":"2026-11-20T01:00:00Z","warehouse":"SYN-WH-01","preAdviceId":"SYN-IPA-01","shape":"BY_GOODS","lines":[{"observed":"SYN-GOODS-01","unit":"EA","stockState":"GOOD","quantity":1,"locationCode":"SYN-LOC-01"}]}`
	if extra == "" {
		return body
	}
	return body[:len(body)-1] + "," + extra + "}"
}

func postReceipt(handler http.Handler, kind, reference, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-receipt-confirmations", strings.NewReader(body))
	request.Header.Set(httpapi.HeaderWorkScopeKind, kind)
	request.Header.Set(httpapi.HeaderWorkScopeReference, reference)
	request.Header.Set(httpapi.HeaderWorkDevice, "SYN-DEVICE-WH")
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestReceiptConfirmationRejectsCarriedTenantOrDeviceBeforeTheCore(t *testing.T) {
	for _, extra := range []string{`"tenantId":"SYN-TENANT-01"`, `"deviceId":"SYN-DEVICE-WH"`, `"operator":"SYN-OPERATOR-01"`} {
		authenticator := &receiptAuthenticatorStub{}
		confirmer := &receiptConfirmerStub{}
		intake, err := warehousehttp.NewReceiptWorkFactIntake(authenticator)
		if err != nil {
			t.Fatal(err)
		}
		response := postReceipt(warehousehttp.NewConfirmReceiptEndpoint(intake, confirmer), "WAREHOUSE", "SYN-WH-01", receiptConfirmationDocument(extra))
		if response.Code != http.StatusBadRequest || authenticator.calls != 0 || confirmer.calls != 0 {
			t.Fatalf("%s：答 %d，认证 %d，核心 %d，want 400 且都不调用；body %s", extra, response.Code, authenticator.calls, confirmer.calls, response.Body.String())
		}
	}
}

func TestReceiptConfirmationRejectsAWorkScopeThatIsNotTheWarehouse(t *testing.T) {
	authenticator := &receiptAuthenticatorStub{}
	confirmer := &receiptConfirmerStub{}
	intake, err := warehousehttp.NewReceiptWorkFactIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := warehousehttp.NewConfirmReceiptEndpoint(intake, confirmer)
	for _, kind := range []string{"NODE", "OFFSITE"} {
		response := postReceipt(endpoint, kind, "SYN-WH-01", receiptConfirmationDocument(""))
		if response.Code != http.StatusBadRequest || authenticator.calls != 0 || confirmer.calls != 0 {
			t.Fatalf("种类 %s：答 %d，认证 %d，核心 %d，want 400 且都不调用", kind, response.Code, authenticator.calls, confirmer.calls)
		}
	}
	mismatched := postReceipt(endpoint, "WAREHOUSE", "SYN-WH-02", receiptConfirmationDocument(""))
	if mismatched.Code != http.StatusBadRequest || authenticator.calls != 0 || confirmer.calls != 0 {
		t.Fatalf("引用不等于仓：答 %d，认证 %d，核心 %d，want 400 且不读册", mismatched.Code, authenticator.calls, confirmer.calls)
	}
}

func TestReceiptConfirmationFillsOperatorAndDeviceFromTheEnvelope(t *testing.T) {
	authenticator := &receiptAuthenticatorStub{}
	confirmer := &receiptConfirmerStub{err: errors.New("core unavailable")}
	intake, err := warehousehttp.NewReceiptWorkFactIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	response := postReceipt(warehousehttp.NewConfirmReceiptEndpoint(intake, confirmer), "WAREHOUSE", "SYN-WH-01", receiptConfirmationDocument(""))
	if response.Code != http.StatusInternalServerError || confirmer.calls != 1 {
		t.Fatalf("答 %d，核心 %d，want 调一次核心后 500；body %s", response.Code, confirmer.calls, response.Body.String())
	}
	if authenticator.kind != "WAREHOUSE" || authenticator.reference != "SYN-WH-01" || authenticator.device != "SYN-DEVICE-WH" {
		t.Fatalf("认证看到种类 %s 引用 %s 设备 %s", authenticator.kind, authenticator.reference, authenticator.device)
	}
	command := confirmer.command
	if command.Tenant.String() != "SYN-TENANT-01" ||
		command.Operator.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" ||
		command.Device.String() != "SYN-DEVICE-WH" ||
		command.Warehouse.String() != "SYN-WH-01" {
		t.Fatalf("命令租户 %s 操作者 %s 设备 %s 仓 %s", command.Tenant, command.Operator, command.Device, command.Warehouse)
	}
}

func TestReceiptConfirmationProblemsUseTheWorkFactCodes(t *testing.T) {
	authenticator := &receiptAuthenticatorStub{err: warehousehttp.ErrOperatorNotGranted}
	confirmer := &receiptConfirmerStub{}
	intake, err := warehousehttp.NewReceiptWorkFactIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	response := postReceipt(warehousehttp.NewConfirmReceiptEndpoint(intake, confirmer), "WAREHOUSE", "SYN-WH-01", receiptConfirmationDocument(""))
	if response.Code != http.StatusForbidden || response.Header().Get("Content-Type") != httpapi.ProblemMediaType || confirmer.calls != 0 {
		t.Fatalf("答 %d %s，核心 %d", response.Code, response.Header().Get("Content-Type"), confirmer.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := body["error"]; wrapped || body["code"] != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("问题体 = %s", response.Body.String())
	}
}

func TestReceiptConfirmationContentInvalidReturnsPerLineIssues(t *testing.T) {
	authenticator := &receiptAuthenticatorStub{}
	confirmer := &receiptConfirmerStub{
		hasResult: true,
		result: application.ContentInvalidReceipt([]domain.ReceiptContentIssue{
			domain.ReceiptLineIssue(0, domain.ReceiptIssueLocationNotReceiving),
			domain.ReceiptLineIssue(1, domain.ReceiptIssueLabelAliasNotValid),
		}),
	}
	intake, err := warehousehttp.NewReceiptWorkFactIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	response := postReceipt(warehousehttp.NewConfirmReceiptEndpoint(intake, confirmer), "WAREHOUSE", "SYN-WH-01", receiptConfirmationDocument(""))
	if response.Code != http.StatusOK {
		t.Fatalf("答 %d %s，want 200", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Issues  []struct {
			Kind   string `json:"kind"`
			Line   int    `json:"line"`
			Carton int    `json:"carton"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "CONTENT_INVALID" || len(body.Issues) != 2 ||
		body.Issues[0].Kind != "LOCATION_NOT_RECEIVING_STAGING" || body.Issues[0].Line != 0 || body.Issues[0].Carton != -1 ||
		body.Issues[1].Kind != "LABEL_ALIAS_NOT_VALID" || body.Issues[1].Line != 1 || body.Issues[1].Carton != -1 {
		t.Fatalf("答复体 = %s", response.Body.String())
	}
}
