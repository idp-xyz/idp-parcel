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

// 本文件对注册号类型目录的三个端点证传输面：两个登记写口未配置即拒且不读载荷、答案逐名转写
// （落库取 201、治理答案取 200、原因随 cause 过线、编排故障答没形成答案）；目录查阅口行体逐字段
// 转写、空册答空数组、读失败答 5xx。

type registrationNumberTypeRegistryDouble struct {
	rows []domain.RegistrationNumberTypeRegistration
}

func (double *registrationNumberTypeRegistryDouble) SaveRegistrationNumberType(
	_ context.Context,
	registration domain.RegistrationNumberTypeRegistration,
) (ports.RegistrationNumberTypeSaveOutcome, error) {
	double.rows = append(double.rows, registration)
	return ports.RegistrationNumberTypeRegistrySaved, nil
}

func (double *registrationNumberTypeRegistryDouble) LoadLatestRegistrationNumberType(
	_ context.Context,
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
	code domain.RegistrationNumberTypeCode,
) (domain.RegistrationNumberTypeRegistration, bool, error) {
	var latest domain.RegistrationNumberTypeRegistration
	found := false
	for _, row := range double.rows {
		if row.Tenant() != tenant || row.Country() != country || row.Code() != code {
			continue
		}
		if !found || row.Revision() > latest.Revision() {
			latest, found = row, true
		}
	}
	return latest, found, nil
}

type registrationNumberTypeIntakeDouble struct {
	register   application.RegisterRegistrationNumberTypeCommand
	deactivate application.DeactivateRegistrationNumberTypeCommand
}

func (double registrationNumberTypeIntakeDouble) IntakeRegistrationNumberTypeRegistration(
	context.Context, *http.Request,
) (application.RegisterRegistrationNumberTypeCommand, error) {
	return double.register, nil
}

func (double registrationNumberTypeIntakeDouble) IntakeRegistrationNumberTypeDeactivation(
	context.Context, *http.Request,
) (application.DeactivateRegistrationNumberTypeCommand, error) {
	return double.deactivate, nil
}

type unreachableRegistrationNumberTypeRegistrar struct{ t *testing.T }

func (registrar unreachableRegistrationNumberTypeRegistrar) Register(
	context.Context, application.RegisterRegistrationNumberTypeCommand,
) (application.RegistrationNumberTypeResult, error) {
	registrar.t.Errorf("未配置态不该调到登记编排")
	return application.RegistrationNumberTypeResult{}, nil
}

func (registrar unreachableRegistrationNumberTypeRegistrar) Deactivate(
	context.Context, application.DeactivateRegistrationNumberTypeCommand,
) (application.RegistrationNumberTypeResult, error) {
	registrar.t.Errorf("未配置态不该调到停用编排")
	return application.RegistrationNumberTypeResult{}, nil
}

type failingRegistrationNumberTypeRegistrar struct{}

func (failingRegistrationNumberTypeRegistrar) Register(
	context.Context, application.RegisterRegistrationNumberTypeCommand,
) (application.RegistrationNumberTypeResult, error) {
	return application.RegistrationNumberTypeResult{}, errors.New("database unavailable")
}

func (failingRegistrationNumberTypeRegistrar) Deactivate(
	context.Context, application.DeactivateRegistrationNumberTypeCommand,
) (application.RegistrationNumberTypeResult, error) {
	return application.RegistrationNumberTypeResult{}, errors.New("database unavailable")
}

func registrationTypeRegisterCommand(t *testing.T, code string, revision int) application.RegisterRegistrationNumberTypeCommand {
	t.Helper()
	format, err := domain.NewRegistrationNumberFormat(`SYN-[0-9]{6}`)
	if err != nil {
		t.Fatalf("new format: %v", err)
	}
	return application.RegisterRegistrationNumberTypeCommand{
		Tenant:   catValue(t, domain.NewTenantID, "tenant-1"),
		Country:  catValue(t, domain.NewRegistrationCountryCode, "XA"),
		Code:     catValue(t, domain.NewRegistrationNumberTypeCode, code),
		Revision: revision,
		Spec: domain.RegistrationNumberTypeSpec{
			Name:   catValue(t, domain.NewRegistrationNumberTypeName, "合成终身注册号"),
			Layer:  domain.RegistrationNumberIdentityLayer,
			Format: format,
			Basis:  catValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-01"),
		},
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func registrationTypeDeactivateCommand(t *testing.T, code string, revision int) application.DeactivateRegistrationNumberTypeCommand {
	t.Helper()
	return application.DeactivateRegistrationNumberTypeCommand{
		Tenant:   catValue(t, domain.NewTenantID, "tenant-1"),
		Country:  catValue(t, domain.NewRegistrationCountryCode, "XA"),
		Code:     catValue(t, domain.NewRegistrationNumberTypeCode, code),
		Revision: revision,
		Basis:    catValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-RETIRE"),
		At:       time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}
}

func postRegistrationType(t *testing.T, endpoint http.Handler, path string) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s 答复不是 JSON：%v（%s）", path, err, recorder.Body)
	}
	return recorder.Code, body
}

// Covers: 两个登记写口在接入渠道未配置时一律 403 ACCESS_CHANNEL_NOT_CONFIGURED，不读载荷、不调编排。
func TestRegistrationNumberTypeWriteEndpointsRefuseWithoutAChannel(t *testing.T) {
	registrar := unreachableRegistrationNumberTypeRegistrar{t: t}
	endpoints := map[string]http.Handler{
		"/commercial-registration-number-type-registrations": commercialhttp.NewRegisterRegistrationNumberTypeEndpoint(
			commercialhttp.UnconfiguredIntake{}, registrar),
		"/commercial-registration-number-type-deactivations": commercialhttp.NewDeactivateRegistrationNumberTypeEndpoint(
			commercialhttp.UnconfiguredIntake{}, registrar),
	}
	for path, endpoint := range endpoints {
		probe := &bodyReadProbe{}
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, probe))
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
			t.Fatalf("%s：status = %d, body = %s, want 403 ACCESS_CHANNEL_NOT_CONFIGURED", path, recorder.Code, recorder.Body)
		}
		if probe.read {
			t.Fatalf("%s：未配置态读了登记载荷", path)
		}
	}
}

// Covers: 答案逐名转写——已登记与已停用取 201；未受理带 cause、未找到取 200；编排故障答 5xx 没形成
// 答案；写口只放 POST。
func TestRegistrationNumberTypeWriteEndpointsTranscribeAnswers(t *testing.T) {
	registry := &registrationNumberTypeRegistryDouble{}
	handler := application.NewRegisterRegistrationNumberTypeHandler(registry)
	const registrations = "/commercial-registration-number-type-registrations"
	const deactivations = "/commercial-registration-number-type-deactivations"
	register := func(command application.RegisterRegistrationNumberTypeCommand) http.Handler {
		return commercialhttp.NewRegisterRegistrationNumberTypeEndpoint(
			registrationNumberTypeIntakeDouble{register: command}, handler)
	}
	deactivate := func(command application.DeactivateRegistrationNumberTypeCommand) http.Handler {
		return commercialhttp.NewDeactivateRegistrationNumberTypeEndpoint(
			registrationNumberTypeIntakeDouble{deactivate: command}, handler)
	}

	if status, body := postRegistrationType(t, register(registrationTypeRegisterCommand(t, "SYN-LIFETIME", 1)), registrations); status != http.StatusCreated || body["outcome"] != "REGISTERED" {
		t.Fatalf("登记修订 1 = %d %v, want 201 REGISTERED", status, body)
	}
	status, body := postRegistrationType(t, register(registrationTypeRegisterCommand(t, "SYN-LIFETIME", 3)), registrations)
	if cause, _ := body["cause"].(string); status != http.StatusOK || body["outcome"] != "NOT_ACCEPTED" || !strings.Contains(cause, "连续") {
		t.Fatalf("跳号 = %d %v, want 200 NOT_ACCEPTED 带连续原因", status, body)
	}
	if status, body := postRegistrationType(t, deactivate(registrationTypeDeactivateCommand(t, "SYN-LIFETIME", 2)), deactivations); status != http.StatusCreated || body["outcome"] != "DEACTIVATED" {
		t.Fatalf("停用 = %d %v, want 201 DEACTIVATED", status, body)
	}
	if status, body := postRegistrationType(t, deactivate(registrationTypeDeactivateCommand(t, "SYN-GHOST", 2)), deactivations); status != http.StatusOK || body["outcome"] != "NOT_FOUND" {
		t.Fatalf("停用从未登记的类型 = %d %v, want 200 NOT_FOUND", status, body)
	}

	failing := commercialhttp.NewRegisterRegistrationNumberTypeEndpoint(
		registrationNumberTypeIntakeDouble{register: registrationTypeRegisterCommand(t, "SYN-LIFETIME", 1)},
		failingRegistrationNumberTypeRegistrar{})
	if status, body := postRegistrationType(t, failing, registrations); status != http.StatusInternalServerError ||
		!strings.Contains(body["error"].(map[string]any)["code"].(string), "NO_ANSWER_FORMED") {
		t.Fatalf("编排故障 = %d %v, want 500 NO_ANSWER_FORMED", status, body)
	}

	recorder := httptest.NewRecorder()
	register(registrationTypeRegisterCommand(t, "SYN-LIFETIME", 1)).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, registrations, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET 登记口 = %d Allow=%q, want 405 Allow=POST", recorder.Code, recorder.Header().Get("Allow"))
	}
}

type registrationNumberTypeReaderDouble struct {
	tenant  domain.TenantID
	rows    []ports.RegistrationNumberTypeRow
	loadErr error
}

func (double *registrationNumberTypeReaderDouble) ListRegistrationNumberTypes(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.RegistrationNumberTypeRow, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

// Covers: 行体逐字段转写；停用两件只在已停用的行上出现，在用的行上缺席而不是空串。
func TestRegistrationNumberTypesEndpointTranscribesRows(t *testing.T) {
	listedAt := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	reader := &registrationNumberTypeReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		rows: []ports.RegistrationNumberTypeRow{
			{
				TenantID: "tenant-1", CountryCode: "XA", TypeCode: "SYN-LIFETIME", Revision: 2,
				TypeName: "合成终身注册号", Layer: "IDENTITY", FormatPattern: `SYN-[0-9]{6}`,
				Basis: "SYN-BASIS-01", Status: "EFFECTIVE",
				EffectiveFrom: listedAt.AddDate(0, -1, 0), RegisteredAt: listedAt,
			},
			{
				TenantID: "tenant-1", CountryCode: "XA", TypeCode: "SYN-TAX-OLD", Revision: 3,
				TypeName: "合成旧税务登记号", Layer: "PROFILE", FormatPattern: `SYN-TAX-[0-9]{4}`,
				Basis: "SYN-BASIS-02", Status: "DEACTIVATED",
				EffectiveFrom: listedAt.AddDate(0, -2, 0), DeactivatedAt: listedAt.AddDate(0, 0, -1),
				DeactivationBasis: "SYN-BASIS-RETIRE", HasDeactivation: true, RegisteredAt: listedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryRegistrationNumberTypesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-registration-number-types", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var response struct {
		Outcome string           `json:"outcome"`
		Types   []map[string]any `json:"registrationNumberTypes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Outcome != "REGISTRATION_NUMBER_TYPES_LISTED" || len(response.Types) != 2 {
		t.Fatalf("response = %+v", response)
	}
	active, retired := response.Types[0], response.Types[1]
	if active["countryCode"] != "XA" || active["typeCode"] != "SYN-LIFETIME" || active["revision"] != float64(2) ||
		active["layer"] != "IDENTITY" || active["formatPattern"] != `SYN-[0-9]{6}` || active["status"] != "EFFECTIVE" ||
		active["typeName"] != "合成终身注册号" || active["basis"] != "SYN-BASIS-01" ||
		active["effectiveFrom"] != "2026-08-24T09:00:00Z" {
		t.Fatalf("在用行 = %v", active)
	}
	if _, has := active["deactivatedAt"]; has {
		t.Fatalf("在用行不该出现 deactivatedAt：%v", active)
	}
	if _, has := active["deactivationBasis"]; has {
		t.Fatalf("在用行不该出现 deactivationBasis：%v", active)
	}
	if retired["status"] != "DEACTIVATED" || retired["deactivatedAt"] != "2026-09-23T09:00:00Z" ||
		retired["deactivationBasis"] != "SYN-BASIS-RETIRE" {
		t.Fatalf("已停用行 = %v", retired)
	}
}

// Covers: 空目录答 2xx 空数组而不是 null（ADR-0077 Decision 四）；未配置答 403；读失败答 5xx 而不是
// 伪装成空册；查阅口只放 GET。
func TestRegistrationNumberTypesEndpointAnswersEmptyUnconfiguredAndFailure(t *testing.T) {
	const path = "/commercial-registration-number-types"
	empty := commercialhttp.NewQueryRegistrationNumberTypesEndpoint(intakeDouble{query: catalogueQuery(t)},
		&registrationNumberTypeReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")})
	recorder := httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"registrationNumberTypes":[]`) {
		t.Fatalf("空目录 = %d %s, want 200 空数组", recorder.Code, recorder.Body)
	}

	unconfigured := commercialhttp.NewQueryRegistrationNumberTypesEndpoint(commercialhttp.UnconfiguredIntake{},
		&registrationNumberTypeReaderDouble{})
	recorder = httptest.NewRecorder()
	unconfigured.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", recorder.Code)
	}

	failing := commercialhttp.NewQueryRegistrationNumberTypesEndpoint(intakeDouble{query: catalogueQuery(t)},
		&registrationNumberTypeReaderDouble{loadErr: errors.New("database unavailable")})
	recorder = httptest.NewRecorder()
	failing.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 = %d, want 500", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST 查阅口 = %d Allow=%q, want 405 Allow=GET", recorder.Code, recorder.Header().Get("Allow"))
	}
}
