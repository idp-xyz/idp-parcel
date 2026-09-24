package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 admin-web-group-legal-entities/06 完成判据 — 开关设 `SYN-TENANT-01` 时 `legal-entity` 一口对合规载荷
// 答 201 且 outcome 为 REGISTERED（Go 侧 PartyIdentityRegistered 的线上名），对带 tenantId 的载荷答 400。
//
// 走进程自己那道门（buildIsolatedWriteAdmission）取 Intake、走生产装配（buildCommercialRegistrationOrchestration）
// 取编排，中间是生产的端点构造函数：三件都是 main 交给路由的那一份，只有路由表本身不在这里——那一格由
// TestIsolatedWriteAdmissionSwitchesOnlyTheAdmittedCommandLines 钉。载荷形状照票 02 的表单：`legalEntities[0]`
// 五格、不带 tenantId。法人钉在已登记且届时已生效的参与方上，参与方先经编排直登——那一口在本笔尚未放行。
// 测试输入是隔离合成，只记 `S`。
func TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("构造隔离写准入：%v", err)
	}

	party := syntheticBusinessPartyRegistration(t)
	party.Party = pcSynthetic(t, func(id string) (commercialdomain.BusinessParty, error) {
		return commercialdomain.NewBusinessParty(
			pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-01"),
			pcSynthetic(t, commercialdomain.NewPartyID, id),
			pcSynthetic(t, commercialdomain.NewPartyName, "SYN 合成参与方（票 06）"),
		)
	}, "SYN-PARTY-06")
	landed, err := registration.partyIdentity.RegisterBusinessParty(t.Context(), party)
	if err != nil || landed.Outcome() != commercialapp.PartyIdentityRegistered {
		t.Fatalf("参与方首登：outcome = %s（原因 %v），err = %v", landed.Outcome(), landed.Cause(), err)
	}

	// 法人登记按注册号类型目录判身份层（ADR-0145 决定一）：先经同一编排登一类身份层类型。
	numberFormat := pcSynthetic(t, commercialdomain.NewRegistrationNumberFormat, `SYN-XA-[0-9]{6}`)
	numberType, err := registration.registrationNumberType.Register(t.Context(), commercialapp.RegisterRegistrationNumberTypeCommand{
		Tenant:   pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-01"),
		Country:  pcSynthetic(t, commercialdomain.NewRegistrationCountryCode, "XA"),
		Code:     pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"),
		Revision: 1,
		Spec: commercialdomain.RegistrationNumberTypeSpec{
			Name:   pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeName, "SYN 合成终身注册号（票 legal-entity-profile/02）"),
			Layer:  commercialdomain.RegistrationNumberIdentityLayer,
			Format: numberFormat,
			Basis:  pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS/regno-xa"),
		},
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || numberType.Outcome() != commercialapp.RegistrationNumberTypeRegistered {
		t.Fatalf("注册号类型首登：outcome = %s，err = %v", numberType.Outcome(), err)
	}

	endpoint := commercialhttp.NewRegisterLegalEntityEndpoint(admission.partyIdentity, registration.partyIdentity)
	item := `{"legalEntityId":"SYN-LE-06","partyId":"SYN-PARTY-06","revision":1,"basis":"SYN-BASIS/le-06","effectiveFrom":"2026-02-01T00:00:00Z",` +
		`"registrationCountry":"XA","lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":"SYN-XA-000006"}]}`

	registered := httptest.NewRecorder()
	endpoint.ServeHTTP(registered, httptest.NewRequest(http.MethodPost, "/commercial-legal-entity-registrations",
		strings.NewReader(`{"legalEntities":[`+item+`]}`)))
	if registered.Code != http.StatusCreated {
		t.Fatalf("合规载荷答 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if got := registrationOutcome(t, registered); got != commercialapp.PartyIdentityRegistered.String() {
		t.Fatalf("outcome = %q, want %q", got, commercialapp.PartyIdentityRegistered.String())
	}

	refused := httptest.NewRecorder()
	endpoint.ServeHTTP(refused, httptest.NewRequest(http.MethodPost, "/commercial-legal-entity-registrations",
		strings.NewReader(`{"tenantId":"SYN-TENANT-01","legalEntities":[`+item+`]}`)))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400", refused.Code, refused.Body.String())
	}
	if got := problemCode(t, refused); got != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q, want MALFORMED_REQUEST", got)
	}
	assertNoOutcome(t, refused, "/commercial-legal-entity-registrations")
}

func registrationOutcome(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode registration answer %s: %v", response.Body.Bytes(), err)
	}
	return body.Outcome
}
