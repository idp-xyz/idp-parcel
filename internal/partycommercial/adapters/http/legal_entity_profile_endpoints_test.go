package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对法人资料的两个端点证传输面：登记写口未配置即拒且不读载荷、答案逐名转写；修订历史口行体逐字段转写，
// 集合格恒为数组、开票资料缺席显式标出。

type legalEntityProfileIntakeDouble struct {
	command application.RegisterLegalEntityProfileCommand
}

func (double legalEntityProfileIntakeDouble) IntakeLegalEntityProfileRegistration(
	context.Context, *http.Request,
) (application.RegisterLegalEntityProfileCommand, error) {
	return double.command, nil
}

// legalEntityProfileRegistrarDouble 交回预置答案；called 记下编排有没有被调到。
type legalEntityProfileRegistrarDouble struct {
	result application.LegalEntityProfileResult
	err    error
	called bool
}

func (double *legalEntityProfileRegistrarDouble) Register(
	context.Context, application.RegisterLegalEntityProfileCommand,
) (application.LegalEntityProfileResult, error) {
	double.called = true
	return double.result, double.err
}

// legalEntityProfileAnswers 经真登记用例造出两种答案：空登记册里修订 1 落库、修订 3 跳号未受理。
func legalEntityProfileAnswers(t *testing.T) (registered, refused application.LegalEntityProfileResult) {
	t.Helper()
	handler := application.NewRegisterLegalEntityProfileHandler(&profileRegistryDouble{}, legalEntityLookupDouble{t: t}, nil)
	address, err := domain.NewRegisteredAddress(catValue(t, domain.NewRegistrationCountryCode, "XA"), []string{"SYN 一号路"})
	if err != nil {
		t.Fatalf("new address: %v", err)
	}
	command := application.RegisterLegalEntityProfileCommand{
		Tenant:        catValue(t, domain.NewTenantID, "tenant-1"),
		Entity:        catValue(t, domain.NewLegalEntityReference, "le-1"),
		Revision:      1,
		Basis:         catValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"),
		EffectiveFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Address:       address,
	}
	registered, err = handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	command.Revision = 3
	refused, err = handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return registered, refused
}

type profileRegistryDouble struct {
	saved []domain.LegalEntityProfileRevision
}

func (double *profileRegistryDouble) SaveLegalEntityProfile(
	_ context.Context, revision domain.LegalEntityProfileRevision,
) (ports.LegalEntityProfileSaveOutcome, error) {
	double.saved = append(double.saved, revision)
	return ports.LegalEntityProfileRegistrySaved, nil
}

func (double *profileRegistryDouble) LoadLatestLegalEntityProfile(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) (domain.LegalEntityProfileRevision, bool, error) {
	if len(double.saved) == 0 {
		return domain.LegalEntityProfileRevision{}, false, nil
	}
	return double.saved[len(double.saved)-1], true, nil
}

// legalEntityLookupDouble 交回一个带 XA 身份层、在用的 le-1。
type legalEntityLookupDouble struct{ t *testing.T }

func (double legalEntityLookupDouble) LoadLatestLegalEntity(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) (domain.LegalEntityRegistration, bool, error) {
	t := double.t
	party, err := domain.NewBusinessParty(
		catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewPartyID, "party-1"), catValue(t, domain.NewPartyName, "SYN 参与方"),
	)
	if err != nil {
		t.Fatalf("new party: %v", err)
	}
	entity, err := domain.NewResponsibleLegalEntity(catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewLegalEntityReference, "le-1"), party)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(entity, 1, catValue(t, domain.NewIdentityBasisReference, "SYN-BASIS"), lifecycle)
	if err != nil {
		t.Fatalf("new registration: %v", err)
	}
	number, err := domain.NewLifetimeRegistrationNumber(
		catValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"), catValue(t, domain.NewRegistrationNumber, "SYN-XA-000001"),
	)
	if err != nil {
		t.Fatalf("new lifetime number: %v", err)
	}
	layer, err := domain.NewLegalEntityIdentityLayer(catValue(t, domain.NewRegistrationCountryCode, "XA"), []domain.LifetimeRegistrationNumber{number})
	if err != nil {
		t.Fatalf("new identity layer: %v", err)
	}
	registration, err = registration.WithIdentityLayer(layer, nil)
	if err != nil {
		t.Fatalf("with identity layer: %v", err)
	}
	return registration, true, nil
}

const legalEntityProfileRegistrations = "/commercial-legal-entity-profile-registrations"

func postLegalEntityProfile(t *testing.T, endpoint http.Handler) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, legalEntityProfileRegistrations, strings.NewReader(`{}`)))
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("答复不是 JSON：%v（%s）", err, recorder.Body)
	}
	return recorder.Code, body
}

// Covers: 登记写口在接入渠道未配置时 403 ACCESS_CHANNEL_NOT_CONFIGURED，不读载荷、不调编排。
func TestLegalEntityProfileWriteEndpointRefusesWithoutAChannel(t *testing.T) {
	registrar := &legalEntityProfileRegistrarDouble{}
	endpoint := commercialhttp.NewRegisterLegalEntityProfileEndpoint(commercialhttp.UnconfiguredIntake{}, registrar)
	probe := &bodyReadProbe{}
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, legalEntityProfileRegistrations, probe))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("status = %d, body = %s, want 403 ACCESS_CHANNEL_NOT_CONFIGURED", recorder.Code, recorder.Body)
	}
	if probe.read || registrar.called {
		t.Fatalf("未配置态读了载荷（%v）或调了编排（%v）", probe.read, registrar.called)
	}
}

// Covers: 答案逐名转写——已登记取 201；未受理带 cause 取 200；编排故障答 5xx 没形成答案；写口只放 POST。
func TestLegalEntityProfileWriteEndpointTranscribesAnswers(t *testing.T) {
	registered, refused := legalEntityProfileAnswers(t)
	intake := legalEntityProfileIntakeDouble{}

	if status, body := postLegalEntityProfile(t, commercialhttp.NewRegisterLegalEntityProfileEndpoint(intake, &legalEntityProfileRegistrarDouble{result: registered})); status != http.StatusCreated || body["outcome"] != "REGISTERED" {
		t.Fatalf("已登记 = %d %v, want 201 REGISTERED", status, body)
	}
	status, body := postLegalEntityProfile(t, commercialhttp.NewRegisterLegalEntityProfileEndpoint(intake, &legalEntityProfileRegistrarDouble{result: refused}))
	if cause, _ := body["cause"].(string); status != http.StatusOK || body["outcome"] != "NOT_ACCEPTED" || !strings.Contains(cause, "修订必须连续") {
		t.Fatalf("未受理 = %d %v, want 200 NOT_ACCEPTED 带原因", status, body)
	}
	failing := commercialhttp.NewRegisterLegalEntityProfileEndpoint(intake, &legalEntityProfileRegistrarDouble{err: errors.New("database unavailable")})
	if status, body := postLegalEntityProfile(t, failing); status != http.StatusInternalServerError ||
		!strings.Contains(body["error"].(map[string]any)["code"].(string), "NO_ANSWER_FORMED") {
		t.Fatalf("编排故障 = %d %v, want 500 NO_ANSWER_FORMED", status, body)
	}

	recorder := httptest.NewRecorder()
	commercialhttp.NewRegisterLegalEntityProfileEndpoint(intake, &legalEntityProfileRegistrarDouble{result: registered}).ServeHTTP(
		recorder, httptest.NewRequest(http.MethodGet, legalEntityProfileRegistrations, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET 登记口 = %d Allow=%q, want 405 Allow=POST", recorder.Code, recorder.Header().Get("Allow"))
	}
}

type legalEntityProfileHistoryDouble struct {
	tenant  domain.TenantID
	entity  domain.LegalEntityReference
	rows    []ports.LegalEntityProfileRevisionRow
	loadErr error
}

func (double *legalEntityProfileHistoryDouble) ListLegalEntityProfileRevisions(
	_ context.Context, tenant domain.TenantID, entity domain.LegalEntityReference,
) ([]ports.LegalEntityProfileRevisionRow, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	if tenant != double.tenant || entity != double.entity {
		return nil, nil
	}
	return double.rows, nil
}

func getProfileRevisions(t *testing.T, endpoint http.Handler, method, entity string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/commercial-group-legal-entities/"+entity+"/profile-revisions", nil)
	request.SetPathValue("legalEntityId", entity)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

// Covers: 行体逐字段转写，按读口给的次序；集合格恒为数组；开票资料缺席时 invoicingRegistered 为假、抬头缺席。
func TestLegalEntityProfileRevisionsEndpointTranscribesRows(t *testing.T) {
	registeredAt := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	reader := &legalEntityProfileHistoryDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		entity: catValue(t, domain.NewLegalEntityReference, "le-1"),
		rows: []ports.LegalEntityProfileRevisionRow{
			{
				TenantID: "tenant-1", LegalEntityID: "le-1", Revision: 1, Basis: "SYN-PROFILE-BASIS",
				EffectiveFrom: registeredAt.AddDate(0, -2, 0), AddressCountry: "XA", AddressLines: []string{"SYN 一号路", "SYN 二层"},
				TaxNumbers:   []ports.TaxRegistrationNumberRow{{TypeCode: "SYN-XA-TAX", Number: "SYN-XA-TAX-0001"}},
				HasInvoicing: true, InvoiceTitle: "SYN 抬头",
				Contacts:     []ports.LegalEntityContactRow{{Name: "SYN 联系人", Email: "syn@example.invalid"}},
				RegisteredAt: registeredAt,
			},
			{
				TenantID: "tenant-1", LegalEntityID: "le-1", Revision: 2, Basis: "SYN-PROFILE-BASIS-2",
				EffectiveFrom: registeredAt.AddDate(0, -1, 0), AddressCountry: "XA", AddressLines: []string{"SYN 新址"},
				RegisteredAt: registeredAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryLegalEntityProfileRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := getProfileRevisions(t, endpoint, http.MethodGet, "le-1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var response struct {
		Outcome       string                   `json:"outcome"`
		LegalEntityID string                   `json:"legalEntityId"`
		Revisions     []map[string]interface{} `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Outcome != "LEGAL_ENTITY_PROFILE_REVISIONS_LISTED" || response.LegalEntityID != "le-1" || len(response.Revisions) != 2 {
		t.Fatalf("response = %s", recorder.Body)
	}
	first, second := response.Revisions[0], response.Revisions[1]
	address := first["registeredAddress"].(map[string]interface{})
	if first["revision"] != float64(1) || address["country"] != "XA" || len(address["lines"].([]interface{})) != 2 ||
		first["invoicingRegistered"] != true || first["invoiceTitle"] != "SYN 抬头" ||
		first["taxRegistrationNumbers"].([]interface{})[0].(map[string]interface{})["number"] != "SYN-XA-TAX-0001" ||
		first["contacts"].([]interface{})[0].(map[string]interface{})["email"] != "syn@example.invalid" ||
		first["effectiveFrom"] != "2026-07-24T09:00:00Z" {
		t.Fatalf("第一笔转写走样：%v", first)
	}
	if _, present := first["contacts"].([]interface{})[0].(map[string]interface{})["phone"]; present {
		t.Fatalf("未登记的电话应缺席：%v", first["contacts"])
	}
	if second["invoicingRegistered"] != false {
		t.Fatalf("第二笔没带开票资料，invoicingRegistered 应为假：%v", second)
	}
	if _, present := second["invoiceTitle"]; present {
		t.Fatalf("没带开票资料的修订不该出抬头：%v", second)
	}
	if taxes, ok := second["taxRegistrationNumbers"].([]interface{}); !ok || len(taxes) != 0 {
		t.Fatalf("空税号应是空数组：%v", second["taxRegistrationNumbers"])
	}
	if contacts, ok := second["contacts"].([]interface{}); !ok || len(contacts) != 0 {
		t.Fatalf("空联系人应是空数组：%v", second["contacts"])
	}
}

// Covers: 不在册答 200 + 空数组；未配置答 403；非 GET 答 405；空路径答 400；读失败答 5xx 没形成答案。
func TestLegalEntityProfileRevisionsEndpointGuards(t *testing.T) {
	reader := &legalEntityProfileHistoryDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		entity: catValue(t, domain.NewLegalEntityReference, "le-1"),
	}
	endpoint := commercialhttp.NewQueryLegalEntityProfileRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := getProfileRevisions(t, endpoint, http.MethodGet, "le-ghost")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"revisions":[]`) {
		t.Fatalf("不在册 = %d %s, want 200 + 空数组", recorder.Code, recorder.Body)
	}
	unconfigured := commercialhttp.NewQueryLegalEntityProfileRevisionsEndpoint(commercialhttp.UnconfiguredIntake{}, reader)
	if recorder := getProfileRevisions(t, unconfigured, http.MethodGet, "le-1"); recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", recorder.Code)
	}
	if recorder := getProfileRevisions(t, endpoint, http.MethodPost, "le-1"); recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d Allow=%q, want 405 Allow=GET", recorder.Code, recorder.Header().Get("Allow"))
	}
	if recorder := getProfileRevisions(t, endpoint, http.MethodGet, ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("空路径 = %d, want 400", recorder.Code)
	}
	reader.loadErr = errors.New("database unavailable")
	if recorder := getProfileRevisions(t, endpoint, http.MethodGet, "le-1"); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 = %d, want 500", recorder.Code)
	}
}
