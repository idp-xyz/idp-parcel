package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证外部资金事实与监管凭证手工采用各口的生产装配（票 demo-chain-gaps/140，ADR-0236）：端点行按特性登记处
// 那一份 build 与 mount 装上——手工口与集成客户端族的同名口同场，编排是两族共用的那一套真库缝。操作者 Intake 是生产那只铸造器经
// 真操作者册（access_identity 0021 放开的两种决定）铸的信封；集成客户端那一侧用核验方与客户端册的替身（客户端册的真库往返另有用例）。
// 准入范围取放行一切的替身：准入对照与区间各有自己的用例，这里证的是决定种类、操作证据与编排。证据只记 `S`。

const (
	manualFactsTenant  = "SYN-TENANT-01"
	manualFinanceA     = "SYN-FINANCE-DCG140-A"
	manualFinanceB     = "SYN-FINANCE-DCG140-B"
	manualCustomsClerk = "SYN-CUSTOMS-DCG140"
	manualOutsider     = "SYN-SUBMITTER-DCG140"

	manualFundsAdoption   = "/settlement-external-funds-fact-manual-registrations"
	manualFundsCorrection = "/settlement-external-funds-fact-correction-manual-registrations"
	manualCredential      = "/customs-regulatory-credential-manual-registrations"
	clientFundsAdoption   = "/settlement-external-funds-fact-registrations"
	clientCredential      = "/customs-regulatory-credential-registrations"
)

func manualFundsBody(fact string) string {
	return `{"factRef":"` + fact + `","sourceRef":"SYN-FUNDS-SOURCE/bank-statement-01","payerRef":"SYN-PAYER/shipper-01",` +
		`"kind":"RECEIPT_CONFIRMED","currency":"CNY","amountMinor":20000,"version":"v1","occurredAt":"2026-10-20T09:00:00+08:00"}`
}

func manualFundsCorrectionBody(fact string) string {
	return `{"factRef":"` + fact + `","corrects":"v1","version":"v2","amountMinor":19000,"correctedAt":"2026-10-21T09:00:00+08:00"}`
}

func manualCredentialBody(credential string, uses int) string {
	return `{"credentialId":"` + credential + `","issuerRef":"SYN-AUTHORITY/customs-cn","holderRef":"SYN-HOLDER/broker-01",` +
		`"procedureRef":"SYN-PROC-CN-EXPORT","validFrom":"2026-10-01T00:00:00Z","validTo":"2027-09-30T00:00:00Z","uses":` +
		strconv.Itoa(uses) + `}`
}

type manualFactsFixture struct {
	db        *bentopg.DB
	endpoints map[string]http.Handler
}

func newManualFactsFixture(t *testing.T) *manualFactsFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	interval, err := accessidentity.NewEffectiveInterval(now.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for subject, kind := range map[string]accessidentity.DecisionKind{
		manualFinanceA:     accessidentity.DecisionExternalFundsFactAdoption,
		manualFinanceB:     accessidentity.DecisionExternalFundsFactAdoption,
		manualCustomsClerk: accessidentity.DecisionRegulatoryCredentialRegistration,
		manualOutsider:     accessidentity.DecisionCustomsDeclarationSubmission,
	} {
		registerOperatorIn(t, db, operators, manualFactsTenant, subject, interval, kind)
	}
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	clientMinter, err := buildIntegrationClientMinter(acceptingClientVerifier{}, clientFactRegister{
		facts: []accessidentity.ExternalFactType{accessidentity.FactExternalFunds, accessidentity.FactRegulatoryCredential},
		start: now.Add(-time.Hour), end: now.Add(time.Hour),
	}, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}

	features := map[any]bool{
		any((*manualExternalFactsWiring)(nil)):    true,
		any((*settlementRegistrationWiring)(nil)): true,
		any((*customsRegistrationWiring)(nil)):    true,
	}
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if !features[feature.wiringKey] {
			continue
		}
		mounted, err := feature.production(endpointSources{db: db}, endpointChannels{operatorDecisions: decisions, integrationClients: clients})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, endpoint := range mounted {
			endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	for _, pattern := range []string{manualFundsAdoption, manualFundsCorrection, manualCredential, clientFundsAdoption, clientCredential} {
		if endpoints[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}
	return &manualFactsFixture{db: db, endpoints: endpoints}
}

func (fixture *manualFactsFixture) post(t *testing.T, pattern, token, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[pattern].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s 答复不是 JSON：%s", pattern, response.Body.String())
	}
	return response.Code, answer
}

func (fixture *manualFactsFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
	return count
}

func (fixture *manualFactsFixture) evidence(t *testing.T, query string, args ...any) *string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var evidence *string
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&evidence); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
	return evidence
}

func operatorEvidenceOf(subject string) string {
	return "OPERATOR/" + customsDecisionIssuer + "#" + subject
}

const (
	fundsEvidenceQuery = `SELECT operation_evidence FROM settlement_accounting.external_funds_fact_version
		 WHERE tenant_id = $1 AND fact_id = $2 AND version = $3`
	credentialEvidenceQuery = `SELECT operation_evidence FROM customs_compliance.regulatory_credential
		 WHERE tenant_id = $1 AND credential_id = $2`
)

// Covers: AT-SA-200、AT-CC-446（生产装配半边）——持外部资金事实采用授予的财务人员经手工口采用首版：形成采用、交出一封采用信封，版本
// 行记下提交操作者；同一份重放答已存在、不翻倍，另一位持同种授予的人重放也一样，操作证据仍是首次那一位；更正由另一位提交，记在新
// 那一版上。持监管凭证登记授予的关务人员经手工口登记一版凭证，凭证行记下登记人；重放答已在册，同身份异内容答内容冲突，都不顶替。
func TestManualExternalFactsAdoptThroughTheOperatorFacesAndRecordTheSubmitter(t *testing.T) {
	fixture := newManualFactsFixture(t)
	const fact = "SYN-FUNDS-FACT-DCG140-01"

	status, adopted := fixture.post(t, manualFundsAdoption, manualFinanceA, manualFundsBody(fact))
	if status != http.StatusCreated || adopted["outcome"] != "FUNDS_FACT_ADOPTED" {
		t.Fatalf("采用：%d %v", status, adopted)
	}
	if got := fixture.evidence(t, fundsEvidenceQuery, manualFactsTenant, fact, "v1"); got == nil || *got != operatorEvidenceOf(manualFinanceA) {
		t.Fatalf("首版的操作证据 = %v", got)
	}
	for _, operator := range []string{manualFinanceA, manualFinanceB} {
		status, replayed := fixture.post(t, manualFundsAdoption, operator, manualFundsBody(fact))
		if status != http.StatusOK || replayed["outcome"] != "EXISTING_FUNDS_FACT" {
			t.Fatalf("%s 重放：%d %v", operator, status, replayed)
		}
	}
	if got := fixture.evidence(t, fundsEvidenceQuery, manualFactsTenant, fact, "v1"); got == nil || *got != operatorEvidenceOf(manualFinanceA) {
		t.Fatalf("重放顶替了首版的操作证据：%v", got)
	}
	if n := fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'settlement-accounting.external-funds-fact.adopted'`); n != 1 {
		t.Fatalf("重放后采用信封 = %d, want 1", n)
	}

	status, corrected := fixture.post(t, manualFundsCorrection, manualFinanceB, manualFundsCorrectionBody(fact))
	if status != http.StatusCreated || corrected["outcome"] != "FUNDS_FACT_ADOPTED" {
		t.Fatalf("更正：%d %v", status, corrected)
	}
	if got := fixture.evidence(t, fundsEvidenceQuery, manualFactsTenant, fact, "v2"); got == nil || *got != operatorEvidenceOf(manualFinanceB) {
		t.Fatalf("更正那一版的操作证据 = %v", got)
	}

	const credential = "SYN-CRED-DCG140-01"
	status, registered := fixture.post(t, manualCredential, manualCustomsClerk, manualCredentialBody(credential, 10))
	if status != http.StatusCreated || registered["outcome"] != "REGISTERED" {
		t.Fatalf("凭证登记：%d %v", status, registered)
	}
	if got := fixture.evidence(t, credentialEvidenceQuery, manualFactsTenant, credential); got == nil || *got != operatorEvidenceOf(manualCustomsClerk) {
		t.Fatalf("凭证的登记人 = %v", got)
	}
	status, replayed := fixture.post(t, manualCredential, manualCustomsClerk, manualCredentialBody(credential, 10))
	if status != http.StatusOK || replayed["outcome"] != "EXISTING" {
		t.Fatalf("凭证重登：%d %v", status, replayed)
	}
	status, conflicting := fixture.post(t, manualCredential, manualCustomsClerk, manualCredentialBody(credential, 11))
	if status != http.StatusOK || conflicting["outcome"] != "CONTENT_CONFLICT" {
		t.Fatalf("凭证同身份异内容：%d %v", status, conflicting)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.regulatory_credential WHERE tenant_id = $1 AND uses = 10`, manualFactsTenant); n != 1 {
		t.Fatalf("凭证册行 = %d, want 1 且是首登那一版", n)
	}
}

// Covers: AT-SA-201、AT-CC-447（生产装配半边）——手工各口各按自己那一种决定认证：管关务的人用不了资金事实的采用与更正，管钱的人
// 用不了凭证口，只持别种决定（申报提交执行）的人哪一口都用不了，一律答 OPERATOR_NOT_GRANTED、什么都不写；没带令牌答 401。
func TestEachManualFactFaceOpensOnlyForItsOwnDecisionKind(t *testing.T) {
	fixture := newManualFactsFixture(t)
	const fact, credential = "SYN-FUNDS-FACT-DCG140-02", "SYN-CRED-DCG140-02"

	for _, attempt := range []struct{ pattern, operator, body string }{
		{manualFundsAdoption, manualCustomsClerk, manualFundsBody(fact)},
		{manualFundsCorrection, manualCustomsClerk, manualFundsCorrectionBody(fact)},
		{manualCredential, manualFinanceA, manualCredentialBody(credential, 10)},
		{manualFundsAdoption, manualOutsider, manualFundsBody(fact)},
		{manualFundsCorrection, manualOutsider, manualFundsCorrectionBody(fact)},
		{manualCredential, manualOutsider, manualCredentialBody(credential, 10)},
	} {
		status, answer := fixture.post(t, attempt.pattern, attempt.operator, attempt.body)
		code, _ := answer["error"].(map[string]any)
		if status != http.StatusForbidden || code["code"] != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("%s 由 %s 出示：%d %v", attempt.pattern, attempt.operator, status, answer)
		}
	}
	if status, _ := fixture.post(t, manualFundsAdoption, "", manualFundsBody(fact)); status != http.StatusUnauthorized {
		t.Fatalf("没令牌：%d", status)
	}
	if n := fixture.count(t, `SELECT (SELECT count(*) FROM settlement_accounting.external_funds_fact_version)
		 + (SELECT count(*) FROM customs_compliance.regulatory_credential)
		 + (SELECT count(*) FROM `+migrate.SchemaBento+`.outbox)`); n != 0 {
		t.Fatalf("越权请求写下了 %d 行", n)
	}
}

// Covers: AT-SA-201、AT-CC-447（生产装配半边：两族并存）——集成客户端族的原口照旧可用：经它采用的资金事实与登记的凭证不记操作证据
// （NULL）；同一条事实随后由财务人员经手工口重放答已存在，不补记、不顶替成手工那一位。
func TestTheIntegrationClientFacesStillWorkAlongsideAndRecordNoOperator(t *testing.T) {
	fixture := newManualFactsFixture(t)
	const fact, credential = "SYN-FUNDS-FACT-DCG140-03", "SYN-CRED-DCG140-03"

	status, adopted := fixture.post(t, clientFundsAdoption, "presented.client.token", manualFundsBody(fact))
	if status != http.StatusCreated || adopted["outcome"] != "FUNDS_FACT_ADOPTED" {
		t.Fatalf("集成客户端采用：%d %v", status, adopted)
	}
	if got := fixture.evidence(t, fundsEvidenceQuery, manualFactsTenant, fact, "v1"); got != nil {
		t.Fatalf("集成客户端那一路记了操作证据 %q", *got)
	}
	status, replayed := fixture.post(t, manualFundsAdoption, manualFinanceA, manualFundsBody(fact))
	if status != http.StatusOK || replayed["outcome"] != "EXISTING_FUNDS_FACT" {
		t.Fatalf("手工口重放集成客户端采用过的事实：%d %v", status, replayed)
	}
	if got := fixture.evidence(t, fundsEvidenceQuery, manualFactsTenant, fact, "v1"); got != nil {
		t.Fatalf("手工重放给集成客户端那一版补记了 %q", *got)
	}

	status, registered := fixture.post(t, clientCredential, "presented.client.token", manualCredentialBody(credential, 10))
	if status != http.StatusCreated || registered["outcome"] != "REGISTERED" {
		t.Fatalf("集成客户端登凭证：%d %v", status, registered)
	}
	if got := fixture.evidence(t, credentialEvidenceQuery, manualFactsTenant, credential); got != nil {
		t.Fatalf("集成客户端那一路给凭证记了操作证据 %q", *got)
	}
}
