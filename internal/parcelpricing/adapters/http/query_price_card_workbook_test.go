package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

func TestPriceCardWorkbookEndpointOnlyAcceptsGet(t *testing.T) {
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, &workbookReaderDouble{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-cards/workbook?id=plan-1&version=v1", nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status %d allow %s", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestPriceCardWorkbookEndpointRejectsAClosedQuery(t *testing.T) {
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, &workbookReaderDouble{})
	for _, target := range []string{
		"/pricing-price-cards/workbook?id=plan-1&version=v1&tenant=SYN-TENANT-01",
		"/pricing-price-cards/workbook?id=plan-1&version=v1&carrier=csv",
		"/pricing-price-cards/workbook?id=plan-1",
	} {
		recorder := getContent(endpoint, target)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", target, recorder.Code)
		}
	}
}

func TestPriceCardWorkbookEndpointEmbedsTheStructuredWorkbook(t *testing.T) {
	reader := &workbookReaderDouble{result: application.ReadPriceCardWorkbookResult{
		Outcome: application.PriceCardWorkbookReady, Origin: application.PriceCardWorkbookFromRegister,
		PlanID: "plan-1", PlanVersion: "v1", TemplateVersion: "PPT-2",
		Canonicalization: "PPC-6", ContentDigest: "digest-1",
		Body: []byte(`{"templateVersion":"PPT-2","sheets":{}}`), MediaType: "application/json",
	}}
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/workbook?id=plan-1&version=v1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Outcome  string          `json:"outcome"`
		Workbook json.RawMessage `json:"workbook"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Outcome != "WORKBOOK" || string(body.Workbook) != `{"templateVersion":"PPT-2","sheets":{}}` {
		t.Fatalf("body %s err %v", recorder.Body.String(), err)
	}
	if reader.carrier != ports.PriceCardWorkbookStructured || reader.tenant != catalogueQuery(t).Scope.Tenant() {
		t.Fatalf("carrier %s tenant %s", reader.carrier, reader.tenant)
	}
}

func TestPriceCardWorkbookEndpointDownloadsTheSpreadsheet(t *testing.T) {
	reader := &workbookReaderDouble{result: application.ReadPriceCardWorkbookResult{
		Outcome: application.PriceCardWorkbookReady, Body: []byte("PK\x03\x04"),
		MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", FileName: "price-card-plan-1-v1.xlsx",
	}}
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/workbook?id=plan-1&version=v1&carrier=spreadsheet")
	if recorder.Code != http.StatusOK || recorder.Body.String() != "PK\x03\x04" {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Disposition") != `attachment; filename="price-card-plan-1-v1.xlsx"` {
		t.Fatalf("disposition %s", recorder.Header().Get("Content-Disposition"))
	}
	if reader.carrier != ports.PriceCardWorkbookSpreadsheet {
		t.Fatalf("carrier %s", reader.carrier)
	}
}

func TestPriceCardWorkbookEndpointKeepsAnUnrepresentableCardAsAConclusion(t *testing.T) {
	reader := &workbookReaderDouble{result: application.ReadPriceCardWorkbookResult{
		Outcome: application.PriceCardWorkbookNotRepresentable, Origin: application.PriceCardWorkbookFromRegister,
		PlanID: "plan-1", PlanVersion: "v1", Where: "rate-table T/v1", Conclusion: "版本引用带指纹",
	}}
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/workbook?id=plan-1&version=v1&carrier=spreadsheet")
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") == "" {
		t.Fatalf("status %d type %s", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	var body struct {
		Outcome    string `json:"outcome"`
		Where      string `json:"where"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Outcome != "NOT_REPRESENTABLE" || body.Where != "rate-table T/v1" {
		t.Fatalf("body %s err %v", recorder.Body.String(), err)
	}
}

func TestPriceCardWorkbookEndpointDoesNotDisguiseAReadFailure(t *testing.T) {
	reader := &workbookReaderDouble{err: errors.New("db")}
	endpoint := pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/workbook?id=plan-1&version=v1")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", recorder.Code)
	}
}

type workbookReaderDouble struct {
	result  application.ReadPriceCardWorkbookResult
	err     error
	tenant  domain.TenantID
	carrier ports.PriceCardWorkbookCarrier
}

func (double *workbookReaderDouble) Handle(
	_ context.Context,
	query application.ReadPriceCardWorkbookQuery,
) (application.ReadPriceCardWorkbookResult, error) {
	double.tenant = query.Tenant
	double.carrier = query.Carrier
	return double.result, double.err
}
