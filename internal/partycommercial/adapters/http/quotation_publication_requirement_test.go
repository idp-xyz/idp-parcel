package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestQuotationPublicationRequirementRegistrationAnswersOnTheWire(t *testing.T) {
	intake := operatorIntake(t)
	recorder := &requirementRecorder{outcome: application.QuotationPublicationRequirementRecorded}
	endpoint := commercialhttp.NewQuotationPublicationRequirementEndpoint(intake, recorder)

	missing := httptest.NewRecorder()
	endpoint.ServeHTTP(missing, operatorRequest("t", `{}`))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("缺 requires = %d", missing.Code)
	}
	unknown := httptest.NewRecorder()
	endpoint.ServeHTTP(unknown, operatorRequest("t", `{"requires":true,"tenantId":"SYN-TENANT-02"}`))
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("未知键 = %d", unknown.Code)
	}

	created := httptest.NewRecorder()
	endpoint.ServeHTTP(created, operatorRequest("t", `{"requires":false}`))
	if created.Code != http.StatusCreated || recorder.requires || recorder.tenant != operatorAuthenticatedTenant {
		t.Fatalf("登记 = %d requires %v tenant %s", created.Code, recorder.requires, recorder.tenant)
	}
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil || body.Outcome != "RECORDED" {
		t.Fatalf("body = %s %v", created.Body.String(), err)
	}

	recorder.outcome = application.QuotationPublicationRequirementReplay
	replay := httptest.NewRecorder()
	endpoint.ServeHTTP(replay, operatorRequest("t", `{"requires":false}`))
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), "ALREADY_RECORDED") {
		t.Fatalf("重放 = %d %s", replay.Code, replay.Body.String())
	}
	recorder.outcome = application.QuotationPublicationRequirementConflict
	conflict := httptest.NewRecorder()
	endpoint.ServeHTTP(conflict, operatorRequest("t", `{"requires":true}`))
	if conflict.Code != http.StatusOK || !strings.Contains(conflict.Body.String(), "CONTENT_CONFLICT") {
		t.Fatalf("冲突 = %d %s", conflict.Code, conflict.Body.String())
	}
}

type requirementRecorder struct {
	outcome  application.QuotationPublicationRequirementOutcome
	tenant   string
	requires bool
}

func (recorder *requirementRecorder) Handle(_ context.Context, tenant domain.TenantID, requires bool) (application.QuotationPublicationRequirementOutcome, error) {
	recorder.tenant = tenant.String()
	recorder.requires = requires
	return recorder.outcome, nil
}
