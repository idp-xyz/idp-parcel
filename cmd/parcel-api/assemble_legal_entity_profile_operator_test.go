package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 operator-channel/04 完成判据「已换各口对合成操作者答业务结果」（法人资料登记口）——资料登记口经操作者渠道与
// 生产装配登下一笔资料修订：持登记册配置写授予的合成操作者对合规载荷答 201 REGISTERED，带 tenantId 答 400、不带令牌答 401，
// 两者都不落行；再登一笔未来生效的修订，按时点解析读口在今天答第一笔、在未来那一刻答第二笔——资料落在信封给的
// SYN-TENANT-01 下，读口按同一租户解析得到。解析读口挂操作者查阅读（票 operator-channel/05），持查阅读授予的合成操作者
// 去读，解析用例照 main 的接法现造；法人与目录先经同一编排直登。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelRegistersALegalEntityProfileThatResolvesAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	reader, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatalf("构造查阅读铸造器：%v", err)
	}
	readIntakes, err := buildOperatorRegistryIntakes(reader)
	if err != nil {
		t.Fatalf("构造查阅读 Intake：%v", err)
	}
	registerLegalEntityWithIdentityLayer(t, registration, "SYN-TENANT-01", "SYN-PARTY-LEP05", "SYN-LE-LEP05")

	const target = "/commercial-legal-entity-profile-registrations"
	endpoint := commercialhttp.NewRegisterLegalEntityProfileEndpoint(operatorRegistriesForRegistryWriter(t).commercial, registration.legalEntityProfile)
	current := `{"legalEntityId":"SYN-LE-LEP05","revision":1,"basis":"SYN-PROFILE-BASIS/lep05-r1","effectiveFrom":"2026-02-01T00:00:00Z",` +
		`"registeredAddress":{"country":"XA","lines":["SYN 合成路 5 号"]},` +
		`"taxRegistrationNumbers":[{"typeCode":"SYN-XA-TAX","number":"SYN-XA-TAX-0005"}],"invoiceTitle":"SYN 合成抬头"}`

	refused := postAsOperator(endpoint, target, presentedOperatorToken, `{"tenantId":"SYN-TENANT-01","profiles":[`+current+`]}`)
	if refused.Code != http.StatusBadRequest || problemCode(t, refused) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", refused.Code, refused.Body.String())
	}
	assertNoOutcome(t, refused, target)
	anonymous := postAsOperator(endpoint, target, "", `{"profiles":[`+current+`]}`)
	if anonymous.Code != http.StatusUnauthorized || problemCode(t, anonymous) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("不带令牌答 %d %s，want 401 OPERATOR_CREDENTIAL_REJECTED", anonymous.Code, anonymous.Body.String())
	}
	assertNoOutcome(t, anonymous, target)
	if landed := postAsOperator(endpoint, target, presentedOperatorToken, `{"profiles":[`+current+`]}`); landed.Code != http.StatusCreated ||
		registrationOutcome(t, landed) != commercialapp.LegalEntityProfileRegistered.String() {
		t.Fatalf("合规载荷答 %d %s，want 201 REGISTERED", landed.Code, landed.Body.String())
	}
	future := `{"legalEntityId":"SYN-LE-LEP05","revision":2,"basis":"SYN-PROFILE-BASIS/lep05-r2","effectiveFrom":"2030-01-01T00:00:00Z",` +
		`"registeredAddress":{"country":"XA","lines":["SYN 合成新址"]},"invoiceTitle":"SYN 合成抬头"}`
	if landed := postAsOperator(endpoint, target, presentedOperatorToken, `{"profiles":[`+future+`]}`); landed.Code != http.StatusCreated {
		t.Fatalf("未来生效的新修订答 %d %s，want 201", landed.Code, landed.Body.String())
	}

	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("身份登记册：%v", err)
	}
	profiles, err := pcpostgres.NewLegalEntityProfiles(db)
	if err != nil {
		t.Fatalf("资料登记册：%v", err)
	}
	resolution := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(readIntakes.commercialOperationsRead,
		commercialapp.NewResolveLegalEntityProfileHandler(profiles, identities))
	resolveAt := func(at string) (string, int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/commercial-group-legal-entities/SYN-LE-LEP05/profile-resolution?at="+at, nil)
		request.Header.Set("Authorization", "Bearer "+presentedOperatorToken)
		request.SetPathValue("legalEntityId", "SYN-LE-LEP05")
		recorder := httptest.NewRecorder()
		resolution.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("解析 %s 答 %d %s", at, recorder.Code, recorder.Body.String())
		}
		var body struct {
			Outcome           string `json:"outcome"`
			EffectiveRevision *struct {
				Revision int `json:"revision"`
			} `json:"effectiveRevision"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.EffectiveRevision == nil {
			t.Fatalf("解析 %s 答复 %s（%v）", at, recorder.Body.String(), err)
		}
		return body.Outcome, body.EffectiveRevision.Revision
	}
	if outcome, revision := resolveAt("2026-09-24T00:00:00Z"); outcome != "RESOLVED" || revision != 1 {
		t.Fatalf("今天 = %s / 修订 %d，want RESOLVED / 1——未来生效的修订不该提前参与", outcome, revision)
	}
	if outcome, revision := resolveAt("2030-06-01T00:00:00Z"); outcome != "RESOLVED" || revision != 2 {
		t.Fatalf("2030 年 = %s / 修订 %d，want RESOLVED / 2", outcome, revision)
	}
}
