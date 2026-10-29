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

// Covers: ADR-0345 决定四——启用范围的登记口与读回口经操作者渠道走到真库：生产铸造器、PC 认证适配器、操作者 Intake 与生产
// 登记编排连起来，只有令牌校验与操作者册是替身。持登记册配置写授予的合成操作者登记答 201，登记责任方落成信封里的
// “发行方#sub”，读回口（持查阅读授予）带出它与此刻的状态；只有查阅读授予的操作者登记答 403，只有登记写授予的操作者读回答 403；
// 载荷自报租户答 400、不带令牌答 401、目录外的码答 200 未受理；写与读都不触及他租户。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelRegistersAndReadsBackReferenceEnablementAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	registry, err := pcpostgres.NewReferenceEnablements(db)
	if err != nil {
		t.Fatal(err)
	}

	const writeTarget = "/reference-data/enablement-registrations"
	write := commercialhttp.NewRegisterReferenceEnablementEndpoint(operatorRegistriesForRegistryWriter(t).commercial, registration.referenceEnablement)

	readerMinter, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	readerIntakes, err := buildOperatorRegistryIntakes(readerMinter)
	if err != nil {
		t.Fatal(err)
	}
	query := commercialapp.NewQueryReferenceEnablementsHandler(registry, systemClock{})
	read := commercialhttp.NewQueryReferenceEnablementsEndpoint(readerIntakes.commercialOperationsRead, query)
	writerAsReader := commercialhttp.NewQueryReferenceEnablementsEndpoint(operatorReadIntakeForRegistryWriter(t), query)

	enableSingapore := `{"kind":"COUNTRY","code":"SG","revision":1,"status":"ENABLED","effectiveFrom":"2026-01-01T00:00:00Z","source":"SYN-MEMO-01"}`

	if refused := postAsOperator(write, writeTarget, presentedOperatorToken, `{"tenantId":"SYN-TENANT-01",`+enableSingapore[1:]); refused.Code != http.StatusBadRequest ||
		problemCode(t, refused) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", refused.Code, refused.Body.String())
	}
	if anonymous := postAsOperator(write, writeTarget, "", enableSingapore); anonymous.Code != http.StatusUnauthorized ||
		problemCode(t, anonymous) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("不带令牌答 %d %s，want 401 OPERATOR_CREDENTIAL_REJECTED", anonymous.Code, anonymous.Body.String())
	}
	readerWrites := commercialhttp.NewRegisterReferenceEnablementEndpoint(readerIntakes.commercial, registration.referenceEnablement)
	if forbidden := postAsOperator(readerWrites, writeTarget, presentedOperatorToken, enableSingapore); forbidden.Code != http.StatusForbidden ||
		problemCode(t, forbidden) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只有查阅读授予的操作者登记答 %d %s，want 403 OPERATOR_NOT_GRANTED", forbidden.Code, forbidden.Body.String())
	}
	if unlisted := postAsOperator(write, writeTarget, presentedOperatorToken,
		`{"kind":"COUNTRY","code":"ZZ","revision":1,"status":"ENABLED","effectiveFrom":"2026-01-01T00:00:00Z"}`); unlisted.Code != http.StatusOK ||
		registrationOutcome(t, unlisted) != commercialapp.ReferenceEnablementNotAccepted.String() {
		t.Fatalf("目录外的码答 %d %s，want 200 NOT_ACCEPTED", unlisted.Code, unlisted.Body.String())
	}

	if landed := postAsOperator(write, writeTarget, presentedOperatorToken, enableSingapore); landed.Code != http.StatusCreated ||
		registrationOutcome(t, landed) != commercialapp.ReferenceEnablementRegistered.String() {
		t.Fatalf("合规载荷答 %d %s，want 201 REGISTERED", landed.Code, landed.Body.String())
	}
	if replay := postAsOperator(write, writeTarget, presentedOperatorToken, enableSingapore); replay.Code != http.StatusOK ||
		registrationOutcome(t, replay) != commercialapp.ReferenceEnablementAlreadyRegistered.String() {
		t.Fatalf("同一笔重放答 %d %s，want 200 ALREADY_REGISTERED", replay.Code, replay.Body.String())
	}
	disable := `{"kind":"COUNTRY","code":"SG","revision":2,"status":"DISABLED","effectiveFrom":"2026-03-01T00:00:00Z"}`
	if landed := postAsOperator(write, writeTarget, presentedOperatorToken, disable); landed.Code != http.StatusCreated {
		t.Fatalf("停用答 %d %s，want 201", landed.Code, landed.Body.String())
	}
	enableAgain := `{"kind":"COUNTRY","code":"SG","revision":3,"status":"ENABLED","effectiveFrom":"2026-04-01T00:00:00Z"}`
	if landed := postAsOperator(write, writeTarget, presentedOperatorToken, enableAgain); landed.Code != http.StatusCreated {
		t.Fatalf("再启用答 %d %s，want 201", landed.Code, landed.Body.String())
	}

	get := func(handler http.Handler, target string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer "+presentedOperatorToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if forbidden := get(writerAsReader, "/reference-data/enablement?kind=COUNTRY"); forbidden.Code != http.StatusForbidden ||
		problemCode(t, forbidden) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只有登记写授予的操作者读回答 %d %s，want 403 OPERATOR_NOT_GRANTED", forbidden.Code, forbidden.Body.String())
	}
	response := get(read, "/reference-data/enablement?kind=COUNTRY")
	if response.Code != http.StatusOK {
		t.Fatalf("读回答 %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Enablements []struct {
			Code           string `json:"code"`
			Standing       string `json:"standing"`
			LatestRevision int    `json:"latestRevision"`
			History        []struct {
				Revision     int    `json:"revision"`
				Status       string `json:"status"`
				Source       string `json:"source"`
				RegisteredBy string `json:"registeredBy"`
			} `json:"history"`
		} `json:"enablements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Enablements) != 1 {
		t.Fatalf("读回应带一个码（目录外的 ZZ 没写）：%s（%v）", response.Body.String(), err)
	}
	singapore := body.Enablements[0]
	if singapore.Code != "SG" || singapore.Standing != "ENABLED" || singapore.LatestRevision != 3 || len(singapore.History) != 3 {
		t.Fatalf("停用后再启用，此刻应是启用、历史三笔：%+v", singapore)
	}
	if first := singapore.History[0]; first.RegisteredBy != "https://id.syn.example/dex#SYN-OPERATOR-01" || first.Source != "SYN-MEMO-01" || first.Status != "ENABLED" {
		t.Fatalf("登记责任方应是信封里的“发行方#sub”：%+v", first)
	}
	if response := get(read, "/reference-data/enablement?kind=CURRENCY"); response.Code != http.StatusOK {
		t.Fatalf("没登记过的种类读空册也是 200：%d", response.Code)
	} else if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Enablements) != 0 {
		t.Fatalf("币种册应为空：%s", response.Body.String())
	}
}

// operatorReadIntakeForRegistryWriter 是只持登记写授予的操作者在读回口上的 Intake：写授予不是查阅读授予。
func operatorReadIntakeForRegistryWriter(t *testing.T) commercialhttp.CommercialCatalogueIntake {
	t.Helper()
	return operatorRegistriesForRegistryWriter(t).commercialOperationsRead
}
