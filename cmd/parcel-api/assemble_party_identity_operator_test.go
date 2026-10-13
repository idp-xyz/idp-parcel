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
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const presentedOperatorToken = "presented.operator.token"

// operatorRegistriesForRegistryWriter 以生产的铸造器、各上下文认证适配器与 Intake 装登记写面各口，只有令牌校验与操作者册
// 是替身：带了令牌就是同一个合成操作者，绑 SYN-TENANT-01、持登记册配置写的授予（registryWriterRegister）。
func operatorRegistriesForRegistryWriter(t *testing.T) operatorRegistryIntakes {
	t.Helper()
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	return registries
}

// postAsOperator 持一枚令牌向一个登记端点发载荷；token 为空即不带令牌。
func postAsOperator(endpoint http.Handler, target, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: 票 operator-channel/04 完成判据「已换各口对合成操作者答业务结果」——身份族四类登记加停用经操作者渠道走到真库：
// 生产铸造器、PC 认证适配器、操作者 Intake 与生产登记编排连起来，只有令牌校验与操作者册是替身。两个参与方、法人、账户、
// 关系各答 201 REGISTERED，停用答 201 DEACTIVATED，同一笔重放答 200 ALREADY_REGISTERED；法人、账户与关系都要引用的参与方
// 在同一租户下已登记，册上读回的行落在信封给的 SYN-TENANT-01 下、别的租户下查无此行。带 tenantId 答 400、不带令牌答 401，
// 两者都没落行——同一笔随后照常首登。写开关不在这条路上（ADR-0150 决定三），本用例不设它。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelRegistersThePartyIdentityFamilyAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	registries := operatorRegistriesForRegistryWriter(t)
	parties := commercialhttp.NewRegisterBusinessPartyEndpoint(registries.commercial, registration.partyIdentity)
	legalEntities := commercialhttp.NewRegisterLegalEntityEndpoint(registries.commercial, registration.partyIdentity)
	accounts := commercialhttp.NewRegisterCustomerAccountEndpoint(registries.commercial, registration.partyIdentity)
	relationships := commercialhttp.NewRegisterPartyRelationshipEndpoint(registries.commercial, registration.partyIdentity)
	deactivations := commercialhttp.NewDeactivatePartyIdentityEndpoint(registries.commercial, registration.partyIdentity)

	const partiesTarget = "/commercial-business-party-registrations"
	holder := `"businessParties":[{"partyId":"SYN-PARTY-OC04-01","name":"SYN 合成参与方（oc04 持有方）","revision":1,` +
		`"basis":"SYN-BASIS/oc04-party-01","effectiveFrom":"2026-01-01T00:00:00Z"}]`

	selfReported := postAsOperator(parties, partiesTarget, presentedOperatorToken, `{"tenantId":"SYN-TENANT-01",`+holder+`}`)
	if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", selfReported.Code, selfReported.Body.String())
	}
	assertNoOutcome(t, selfReported, partiesTarget)
	anonymous := postAsOperator(parties, partiesTarget, "", `{`+holder+`}`)
	if anonymous.Code != http.StatusUnauthorized || problemCode(t, anonymous) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("不带令牌答 %d %s，want 401 OPERATOR_CREDENTIAL_REJECTED", anonymous.Code, anonymous.Body.String())
	}
	assertNoOutcome(t, anonymous, partiesTarget)

	// 法人登记按注册号类型目录判身份层（ADR-0145 决定一）：这一类型经同一编排直登，它的登记口不是本用例的对象。
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-01")
	numberType, err := registration.registrationNumberType.Register(t.Context(), commercialapp.RegisterRegistrationNumberTypeCommand{
		Tenant:   tenant,
		Country:  pcSynthetic(t, commercialdomain.NewRegistrationCountryCode, "XA"),
		Code:     pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"),
		Revision: 1,
		Spec: commercialdomain.RegistrationNumberTypeSpec{
			Name:   pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeName, "SYN 合成终身注册号（票 operator-channel/04）"),
			Layer:  commercialdomain.RegistrationNumberIdentityLayer,
			Format: pcSynthetic(t, commercialdomain.NewRegistrationNumberFormat, `SYN-XA-[0-9]{6}`),
			Basis:  pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS/regno-xa"),
		},
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || numberType.Outcome() != commercialapp.RegistrationNumberTypeRegistered {
		t.Fatalf("注册号类型首登：outcome = %s，err = %v", numberType.Outcome(), err)
	}

	for _, step := range []struct {
		label    string
		endpoint http.Handler
		target   string
		body     string
		status   int
		outcome  string
	}{
		{"参与方（持有方）", parties, partiesTarget, `{` + holder + `}`, http.StatusCreated, "REGISTERED"},
		{"参与方（相对方）", parties, partiesTarget, `{"businessParties":[{"partyId":"SYN-PARTY-OC04-02","name":"SYN 合成参与方（oc04 相对方）",` +
			`"revision":1,"basis":"SYN-BASIS/oc04-party-02","effectiveFrom":"2026-01-01T00:00:00Z"}]}`, http.StatusCreated, "REGISTERED"},
		{"责任法人", legalEntities, "/commercial-legal-entity-registrations", `{"legalEntities":[{"legalEntityId":"SYN-LE-OC04-01",` +
			`"partyId":"SYN-PARTY-OC04-01","revision":1,"basis":"SYN-BASIS/oc04-le-01","effectiveFrom":"2026-02-01T00:00:00Z",` +
			`"registrationCountry":"XA","lifetimeRegistrationNumbers":[{"typeCode":"SYN-XA-LIFETIME","number":"SYN-XA-000041"}]}]}`,
			http.StatusCreated, "REGISTERED"},
		{"货主客户账户", accounts, "/commercial-customer-account-registrations", `{"customerAccounts":[{"accountId":"SYN-ACCT-OC04-01",` +
			`"customerPartyId":"SYN-PARTY-OC04-01","revision":1,"basis":"SYN-BASIS/oc04-acct-01","effectiveFrom":"2026-02-01T00:00:00Z"}]}`,
			http.StatusCreated, "REGISTERED"},
		{"参与方关系（候选）", relationships, "/commercial-party-relationship-registrations", `{"relationships":[{"relationshipId":"SYN-REL-OC04-01",` +
			`"revision":1,"holder":"SYN-PARTY-OC04-01","counterparty":"SYN-PARTY-OC04-02","role":"CUSTOMER","scope":"SYN-SCOPE/oc04",` +
			`"basis":"SYN-BASIS/oc04-rel-01","effectiveStartsAt":"2026-03-01T00:00:00Z"}]}`, http.StatusCreated, "REGISTERED"},
		{"停用客户账户", deactivations, "/commercial-party-identity-deactivations", `{"deactivations":[{"kind":"CUSTOMER_ACCOUNT",` +
			`"id":"SYN-ACCT-OC04-01","revision":2,"basis":"SYN-BASIS/oc04-acct-01-deactivate","at":"2026-06-01T00:00:00Z"}]}`,
			http.StatusCreated, "DEACTIVATED"},
		{"重放持有方", parties, partiesTarget, `{` + holder + `}`, http.StatusOK, "ALREADY_REGISTERED"},
	} {
		response := postAsOperator(step.endpoint, step.target, presentedOperatorToken, step.body)
		if response.Code != step.status || registrationOutcome(t, response) != step.outcome {
			t.Fatalf("%s：答 %d %s，want %d %s", step.label, response.Code, response.Body.String(), step.status, step.outcome)
		}
	}

	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("身份登记册：%v", err)
	}
	account, found, err := identities.LoadLatestCustomerAccount(t.Context(), tenant,
		pcSynthetic(t, commercialdomain.NewCustomerAccountID, "SYN-ACCT-OC04-01"))
	if err != nil || !found {
		t.Fatalf("SYN-TENANT-01 下读回账户：found = %v，err = %v", found, err)
	}
	if _, _, deactivated := account.Lifecycle().Deactivation(); account.Revision() != 2 || !deactivated {
		t.Fatalf("账户最新修订 = %d（已停用 %v），want 2 且已停用", account.Revision(), deactivated)
	}
	_, found, err = identities.LoadLatestBusinessParty(t.Context(), pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-02"),
		pcSynthetic(t, commercialdomain.NewPartyID, "SYN-PARTY-OC04-01"))
	if err != nil || found {
		t.Fatalf("别的租户下读到了参与方：found = %v，err = %v", found, err)
	}
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
