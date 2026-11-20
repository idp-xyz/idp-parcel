package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

type signingRegistryDouble struct {
	save ports.SigningEvidenceSaveOutcome
}

func (double signingRegistryDouble) SaveSigningEvidence(context.Context, domain.SigningEvidence) (ports.SigningEvidenceSaveOutcome, error) {
	return double.save, nil
}

func (signingRegistryDouble) SaveSigningEvidenceRequirement(context.Context, domain.SigningEvidenceRequirement) (ports.SigningEvidenceRequirementSaveOutcome, error) {
	return ports.SigningEvidenceRequirementSaved, nil
}

func (signingRegistryDouble) LoadSigningEvidenceRequirement(context.Context, domain.TenantID) (domain.SigningEvidenceRequirement, bool, error) {
	return domain.SigningEvidenceRequirement{}, false, nil
}

func (signingRegistryDouble) HasSigningEvidence(context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) (bool, error) {
	return false, nil
}

func (signingRegistryDouble) ListSigningEvidence(context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) ([]domain.SigningEvidence, error) {
	return nil, nil
}

type signingIntakeDouble struct {
	command application.RegisterSigningEvidenceCommand
}

func (double signingIntakeDouble) IntakeSigningEvidenceRegistration(context.Context, *http.Request) (application.RegisterSigningEvidenceCommand, error) {
	return double.command, nil
}

type signingReaderDouble struct {
	read application.SigningEvidenceRead
}

func (double signingReaderDouble) Handle(context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) (application.SigningEvidenceRead, error) {
	return double.read, nil
}

func TestSigningEvidenceRegistrationAnswersByOutcome(t *testing.T) {
	handler := application.NewRegisterSigningEvidenceHandler(signingRegistryDouble{save: ports.SigningEvidenceSaved})
	endpoint := commercialhttp.NewRegisterSigningEvidenceEndpoint(signingIntakeDouble{}, handler)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/commercial-signing-evidence-registrations", nil))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["outcome"] != "SAVED" {
		t.Fatalf("body = %s %v", recorder.Body.String(), err)
	}

	blocked := commercialhttp.NewRegisterSigningEvidenceEndpoint(signingIntakeDouble{},
		application.NewRegisterSigningEvidenceHandler(signingRegistryDouble{save: ports.SigningEvidenceTargetNotFound}))
	recorder = httptest.NewRecorder()
	blocked.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/commercial-signing-evidence-registrations", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("目标不在 status = %d", recorder.Code)
	}
	body = map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["outcome"] != "TARGET_NOT_FOUND" {
		t.Fatalf("body = %s", recorder.Body.String())
	}

	get := httptest.NewRecorder()
	endpoint.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/commercial-signing-evidence-registrations", nil))
	if get.Code != http.StatusMethodNotAllowed || get.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET = %d Allow=%q", get.Code, get.Header().Get("Allow"))
	}
}

func TestSigningEvidenceReadSaysNotConfiguredUntilTheRequirementIsRegistered(t *testing.T) {
	endpoint := commercialhttp.NewQuerySigningEvidencesEndpoint(intakeDouble{query: catalogueQuery(t)}, signingReaderDouble{})
	request := httptest.NewRequest(http.MethodGet, "/commercial-signing-evidences/CUSTOMER_CONTRACT/contract-1/v1", nil)
	request.SetPathValue("kind", "CUSTOMER_CONTRACT")
	request.SetPathValue("objectId", "contract-1")
	request.SetPathValue("version", "v1")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recorder.Code != http.StatusOK || body["requirement"] != "NOT_CONFIGURED" || body["outcome"] != "SIGNING_EVIDENCES_READ" {
		t.Fatalf("未配置 = %d %v", recorder.Code, body)
	}
	if _, present := body["requiresCustomerContract"]; present {
		t.Fatalf("未配置却带了布尔：%v", body)
	}

	other := httptest.NewRequest(http.MethodGet, "/commercial-signing-evidences/CREDIT_POLICY/x/v1", nil)
	other.SetPathValue("kind", "CREDIT_POLICY")
	other.SetPathValue("objectId", "x")
	other.SetPathValue("version", "v1")
	refused := httptest.NewRecorder()
	endpoint.ServeHTTP(refused, other)
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("别的类别 = %d", refused.Code)
	}
}
