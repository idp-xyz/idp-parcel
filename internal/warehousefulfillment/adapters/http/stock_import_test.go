package warehousehttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type stockImportAuthStub struct {
	calls int
	err   error
}

func (stub *stockImportAuthStub) AuthenticateStockImportSubmission(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	stub.calls++
	if stub.err != nil {
		return warehousehttp.OperatorIdentity{}, stub.err
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	registrant, _ := domain.NewRegistrant("https://id.syn.example/dex#SYN-OP-1")
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: registrant}, nil
}

type stockImportSubmitStub struct {
	calls int
}

func (stub *stockImportSubmitStub) Submit(context.Context, application.SubmitStockImportCommand) (application.StockImportResult, error) {
	stub.calls++
	return application.StockImportResult{}, nil
}

// Covers: AT-WF-080——没授予与不在准入范围在读载荷之前答，坏载荷（空体、自带租户）在认证通过之后答，编排不被调用。
func TestStockImportSubmissionStopsAtGrantAdmissionAndMalformedBody(t *testing.T) {
	post := func(handler http.Handler, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-stock-import-submissions", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	for _, tc := range []struct {
		name string
		err  error
		code int
		body string
	}{
		{"not granted", warehousehttp.ErrOperatorNotGranted, http.StatusForbidden, `{"tenantId":"SYN-TENANT-02"}`},
		{"outside admission", warehousehttp.ErrOutsideAdmissionScope, http.StatusForbidden, `{}`},
	} {
		auth := &stockImportAuthStub{err: tc.err}
		submit := &stockImportSubmitStub{}
		intake, err := warehousehttp.NewOperatorStockImportIntake(auth)
		if err != nil {
			t.Fatal(err)
		}
		response := post(warehousehttp.NewSubmitStockImportEndpoint(intake, submit), tc.body)
		if response.Code != tc.code || auth.calls != 1 || submit.calls != 0 {
			t.Fatalf("%s = %d 认证 %d 编排 %d，正文 %s", tc.name, response.Code, auth.calls, submit.calls, response.Body.String())
		}
	}
	auth := &stockImportAuthStub{}
	submit := &stockImportSubmitStub{}
	intake, err := warehousehttp.NewOperatorStockImportIntake(auth)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := warehousehttp.NewSubmitStockImportEndpoint(intake, submit)
	for _, body := range []string{`{}`, `{"tenantId":"SYN-TENANT-02","importId":"SYN-IMP-01"}`} {
		response := post(endpoint, body)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "MALFORMED_REQUEST") || submit.calls != 0 {
			t.Fatalf("坏载荷 %s = %d %s 编排 %d", body, response.Code, response.Body.String(), submit.calls)
		}
	}
	if auth.calls != 2 {
		t.Fatalf("坏载荷仍应先认证，认证次数 = %d", auth.calls)
	}
}
