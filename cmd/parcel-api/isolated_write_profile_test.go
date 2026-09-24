package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 legal-entity-profile/05 完成判据「真库一条经隔离写 Intake 与生产装配登下一笔资料修订」——开关设 `SYN-TENANT-01`
// 时资料登记口对合规载荷答 201 且 outcome 为 REGISTERED、对带 tenantId 的载荷答 400；再登一笔未来生效的修订，按时点解析读口
// 经隔离读 Intake 在今天答第一笔、在未来那一刻答第二笔。Intake 走进程自己那两道门（buildIsolatedWriteAdmission、
// buildIsolatedReadIntakes），编排走生产装配；解析用例照 main 的接法现造。法人与目录先经同一编排直登。测试输入是隔离合成，只记 `S`。
func TestIsolatedLegalEntityProfileRegistrationLandsAndResolvesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	writeAdmission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{isolatedWriteTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatalf("构造隔离写准入：%v", err)
	}
	readIntakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatalf("构造隔离读准入：%v", err)
	}
	registerLegalEntityWithIdentityLayer(t, registration, "SYN-TENANT-01", "SYN-PARTY-LEP05", "SYN-LE-LEP05")

	endpoint := commercialhttp.NewRegisterLegalEntityProfileEndpoint(writeAdmission.partyIdentity, registration.legalEntityProfile)
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/commercial-legal-entity-profile-registrations", strings.NewReader(body)))
		return recorder
	}
	current := `{"legalEntityId":"SYN-LE-LEP05","revision":1,"basis":"SYN-PROFILE-BASIS/lep05-r1","effectiveFrom":"2026-02-01T00:00:00Z",` +
		`"registeredAddress":{"country":"XA","lines":["SYN 合成路 5 号"]},` +
		`"taxRegistrationNumbers":[{"typeCode":"SYN-XA-TAX","number":"SYN-XA-TAX-0005"}],"invoiceTitle":"SYN 合成抬头"}`
	if landed := post(`{"profiles":[` + current + `]}`); landed.Code != http.StatusCreated ||
		registrationOutcome(t, landed) != commercialapp.LegalEntityProfileRegistered.String() {
		t.Fatalf("合规载荷答 %d %s，want 201 REGISTERED", landed.Code, landed.Body.String())
	}
	refused := post(`{"tenantId":"SYN-TENANT-01","profiles":[` + current + `]}`)
	if refused.Code != http.StatusBadRequest || problemCode(t, refused) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", refused.Code, refused.Body.String())
	}
	assertNoOutcome(t, refused, "/commercial-legal-entity-profile-registrations")
	future := `{"legalEntityId":"SYN-LE-LEP05","revision":2,"basis":"SYN-PROFILE-BASIS/lep05-r2","effectiveFrom":"2030-01-01T00:00:00Z",` +
		`"registeredAddress":{"country":"XA","lines":["SYN 合成新址"]},"invoiceTitle":"SYN 合成抬头"}`
	if landed := post(`{"profiles":[` + future + `]}`); landed.Code != http.StatusCreated {
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
	resolution := commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(readIntakes.commercialCatalogue,
		commercialapp.NewResolveLegalEntityProfileHandler(profiles, identities))
	resolveAt := func(at string) (string, int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/commercial-group-legal-entities/SYN-LE-LEP05/profile-resolution?at="+at, nil)
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
