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
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 票 legal-entity-profile/05 的传输层用例：法人资料按时点解析读口把解析用例的各格原样译出，不在读口里另算；
// `at` 由调用方给，不代填。解析结果用领域的 ResolveLegalEntityProfile 现造，各格都是真判断的产物。

type profileResolverDouble struct {
	resolution domain.LegalEntityProfileResolution
	err        error
	tenant     domain.TenantID
	entity     domain.LegalEntityReference
	at         time.Time
	called     bool
}

func (double *profileResolverDouble) Resolve(
	_ context.Context, tenant domain.TenantID, entity domain.LegalEntityReference, at time.Time,
) (domain.LegalEntityProfileResolution, error) {
	double.called, double.tenant, double.entity, double.at = true, tenant, entity, at
	return double.resolution, double.err
}

var resolutionMonth = func(month time.Month) time.Time { return time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC) }

// resolutionEntity 造 tenant-1 / le-1 的一笔法人修订：带 XA 身份层，自 from 起生效；deactivatedAt 非零即已停用。
func resolutionEntity(t *testing.T, from, deactivatedAt time.Time) domain.LegalEntityRegistration {
	t.Helper()
	party, err := domain.NewBusinessParty(catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewPartyID, "party-1"),
		catValue(t, domain.NewPartyName, "SYN 参与方"))
	if err != nil {
		t.Fatalf("new party: %v", err)
	}
	entity, err := domain.NewResponsibleLegalEntity(catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewLegalEntityReference, "le-1"), party)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(from)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(entity, 1, catValue(t, domain.NewIdentityBasisReference, "SYN-BASIS"), lifecycle)
	if err != nil {
		t.Fatalf("new registration: %v", err)
	}
	number, err := domain.NewLifetimeRegistrationNumber(
		catValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"), catValue(t, domain.NewRegistrationNumber, "SYN-XA-000001"))
	if err != nil {
		t.Fatalf("new lifetime number: %v", err)
	}
	layer, err := domain.NewLegalEntityIdentityLayer(catValue(t, domain.NewRegistrationCountryCode, "XA"), []domain.LifetimeRegistrationNumber{number})
	if err != nil {
		t.Fatalf("new identity layer: %v", err)
	}
	if registration, err = registration.WithIdentityLayer(layer, nil); err != nil {
		t.Fatalf("with identity layer: %v", err)
	}
	if !deactivatedAt.IsZero() {
		if registration, err = registration.Deactivate(catValue(t, domain.NewIdentityBasisReference, "SYN-DEACT"), deactivatedAt); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
	}
	return registration
}

// resolutionRevision 造 le-1 的第 revision 笔资料：地址在 XA 一行 line；title 为空即不带开票资料。
func resolutionRevision(t *testing.T, revision int, from time.Time, line, title string) domain.LegalEntityProfileRevision {
	t.Helper()
	address, err := domain.NewRegisteredAddress(catValue(t, domain.NewRegistrationCountryCode, "XA"), []string{line})
	if err != nil {
		t.Fatalf("new address: %v", err)
	}
	var invoicing *domain.InvoicingDetails
	if title != "" {
		details, err := domain.NewInvoicingDetails(catValue(t, domain.NewInvoiceTitle, title))
		if err != nil {
			t.Fatalf("new invoicing: %v", err)
		}
		invoicing = &details
	}
	content, err := domain.NewLegalEntityProfileContent(address, nil, invoicing, nil)
	if err != nil {
		t.Fatalf("new content: %v", err)
	}
	built, err := domain.NewLegalEntityProfileRevision(catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewLegalEntityReference, "le-1"),
		revision, catValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"), from, content)
	if err != nil {
		t.Fatalf("new revision: %v", err)
	}
	return built
}

func resolve(t *testing.T, entity domain.LegalEntityRegistration, chain []domain.LegalEntityProfileRevision, at time.Time) domain.LegalEntityProfileResolution {
	t.Helper()
	resolution, err := domain.ResolveLegalEntityProfile(entity, chain, at)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return resolution
}

type resolutionAnswer struct {
	Outcome           string `json:"outcome"`
	LegalEntityID     string `json:"legalEntityId"`
	At                string `json:"at"`
	IncompleteCause   string `json:"incompleteCause"`
	EffectiveRevision *struct {
		Revision            int    `json:"revision"`
		Basis               string `json:"basis"`
		EffectiveFrom       string `json:"effectiveFrom"`
		InvoicingRegistered bool   `json:"invoicingRegistered"`
		InvoiceTitle        string `json:"invoiceTitle"`
		RegisteredAddress   struct {
			Country string   `json:"country"`
			Lines   []string `json:"lines"`
		} `json:"registeredAddress"`
		TaxRegistrationNumbers []any `json:"taxRegistrationNumbers"`
		Contacts               []any `json:"contacts"`
	} `json:"effectiveRevision"`
}

func getProfileResolution(t *testing.T, endpoint http.Handler, method, entity, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	target := "/commercial-group-legal-entities/" + entity + "/profile-resolution"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	request := httptest.NewRequest(method, target, nil)
	request.SetPathValue("legalEntityId", entity)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

// Covers: 各格逐一转写——已解析带有效的那一笔；资料不全两成因（缺开票资料时仍带有效的那一笔，没有有效修订时不带）；
// 法人未登记 / 未生效 / 已停用只答格名。租户取自作用域、法人取自路径、时点取自 `at`。
func TestLegalEntityProfileResolutionEndpointTranscribesEachOutcome(t *testing.T) {
	active := resolutionEntity(t, resolutionMonth(time.January), time.Time{})
	complete := resolutionRevision(t, 1, resolutionMonth(time.March), "SYN 一号路", "SYN 抬头")
	bare := resolutionRevision(t, 1, resolutionMonth(time.March), "SYN 一号路", "")
	april := resolutionMonth(time.April)
	cases := []struct {
		name          string
		resolution    domain.LegalEntityProfileResolution
		outcome       string
		cause         string
		wantRevision  bool
		wantInvoicing bool
	}{
		{"已解析", resolve(t, active, []domain.LegalEntityProfileRevision{complete}, april), "RESOLVED", "", true, true},
		{"资料不全·没有有效修订", resolve(t, active, []domain.LegalEntityProfileRevision{complete}, resolutionMonth(time.February)), "PROFILE_INCOMPLETE", "NO_EFFECTIVE_REVISION", false, false},
		{"资料不全·缺开票资料", resolve(t, active, []domain.LegalEntityProfileRevision{bare}, april), "PROFILE_INCOMPLETE", "NO_INVOICING_DETAILS", true, false},
		{"法人未登记", domain.NotRegisteredLegalEntityProfileResolution(), "LEGAL_ENTITY_NOT_REGISTERED", "", false, false},
		{"法人未生效", resolve(t, resolutionEntity(t, resolutionMonth(time.June), time.Time{}), nil, april), "LEGAL_ENTITY_NOT_EFFECTIVE", "", false, false},
		{"法人已停用", resolve(t, resolutionEntity(t, resolutionMonth(time.January), resolutionMonth(time.March)), []domain.LegalEntityProfileRevision{complete}, april), "LEGAL_ENTITY_DEACTIVATED", "", false, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resolver := &profileResolverDouble{resolution: testCase.resolution}
			endpoint := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(intakeDouble{query: catalogueQuery(t)}, resolver)
			recorder := getProfileResolution(t, endpoint, http.MethodGet, "le-1", "at=2026-04-01T00:00:00Z")
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if resolver.tenant.String() != "tenant-1" || resolver.entity.String() != "le-1" || !resolver.at.Equal(april) {
				t.Fatalf("解析入参 = (%v, %v, %v)", resolver.tenant, resolver.entity, resolver.at)
			}
			var answer resolutionAnswer
			if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if answer.Outcome != testCase.outcome || answer.IncompleteCause != testCase.cause ||
				answer.LegalEntityID != "le-1" || answer.At != "2026-04-01T00:00:00Z" {
				t.Fatalf("答复 = %s", recorder.Body)
			}
			if (answer.EffectiveRevision != nil) != testCase.wantRevision {
				t.Fatalf("有效修订在场 = %v, want %v：%s", answer.EffectiveRevision != nil, testCase.wantRevision, recorder.Body)
			}
			if !testCase.wantRevision {
				return
			}
			revision := answer.EffectiveRevision
			if revision.Revision != 1 || revision.RegisteredAddress.Country != "XA" || len(revision.RegisteredAddress.Lines) != 1 ||
				revision.EffectiveFrom != "2026-03-01T00:00:00Z" || revision.InvoicingRegistered != testCase.wantInvoicing ||
				revision.TaxRegistrationNumbers == nil || revision.Contacts == nil {
				t.Fatalf("有效修订转写走样：%s", recorder.Body)
			}
			if testCase.wantInvoicing != (revision.InvoiceTitle == "SYN 抬头") {
				t.Fatalf("抬头 = %q，开票资料在场 = %v", revision.InvoiceTitle, testCase.wantInvoicing)
			}
		})
	}
}

// Covers: `at` 缺或形状不对答 400 且不调解析（不代填时钟）；未配置答 403；非 GET 答 405；空路径答 400；解析失败答 5xx 没形成答案。
func TestLegalEntityProfileResolutionEndpointGuards(t *testing.T) {
	resolver := &profileResolverDouble{resolution: domain.NotRegisteredLegalEntityProfileResolution()}
	endpoint := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(intakeDouble{query: catalogueQuery(t)}, resolver)

	for name, rawQuery := range map[string]string{"缺 at": "", "at 形状错": "at=2026-04-01", "at 空": "at="} {
		if recorder := getProfileResolution(t, endpoint, http.MethodGet, "le-1", rawQuery); recorder.Code != http.StatusBadRequest ||
			!strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("%s = %d %s, want 400 MALFORMED_REQUEST", name, recorder.Code, recorder.Body)
		}
	}
	if resolver.called {
		t.Fatal("`at` 不成立时不该调解析——那等于替调用方取了时点")
	}
	unconfigured := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(commercialhttp.UnconfiguredIntake{}, resolver)
	if recorder := getProfileResolution(t, unconfigured, http.MethodGet, "le-1", "at=2026-04-01T00:00:00Z"); recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 = %d, want 403", recorder.Code)
	}
	if recorder := getProfileResolution(t, endpoint, http.MethodPost, "le-1", "at=2026-04-01T00:00:00Z"); recorder.Code != http.StatusMethodNotAllowed ||
		recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d Allow=%q, want 405 Allow=GET", recorder.Code, recorder.Header().Get("Allow"))
	}
	if recorder := getProfileResolution(t, endpoint, http.MethodGet, "", "at=2026-04-01T00:00:00Z"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("空路径 = %d, want 400", recorder.Code)
	}
	failing := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(intakeDouble{query: catalogueQuery(t)},
		&profileResolverDouble{err: errors.New("database unavailable")})
	if recorder := getProfileResolution(t, failing, http.MethodGet, "le-1", "at=2026-04-01T00:00:00Z"); recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(recorder.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("解析失败 = %d %s, want 500 NO_ANSWER_FORMED", recorder.Code, recorder.Body)
	}
}
