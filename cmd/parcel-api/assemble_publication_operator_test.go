package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件钉运营操作者面发布路径换上操作者渠道之后装配点那一半（票 operator-channel/06）。四个 POST 口与回放口的拒绝格、
// 未带令牌与自报租户在 swappedRegistryFaces 那两条里；这里是 GET 的词表口、仍挂字面量的受控批文镜像，以及真库载体上
// 记下的录入者与批准者。

func serveThroughOperatorRegistries(t *testing.T, register accessidentity.OperatorRegistry, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	minter, err := buildOperatorMinter(acceptingVerifier{}, register)
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(unconfiguredOperatorDecisions(), registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil, nil))
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func answeredOutcome(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.Outcome
}

// Covers: ADR-0151 决定五——词表读口与发布四口挂同一个操作者渠道 Intake、答同一组格。授予齐备的那一格越过 Intake 照常
// 列词：词表不读存储，未接线的装配也答得出。
func TestThePublicationVocabularyAnswersFromTheOperatorChannel(t *testing.T) {
	const vocabulary = "/commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE"
	cases := map[string]struct {
		register accessidentity.OperatorRegistry
		token    string
		status   int
		code     string
	}{
		"no bearer token":  {registryWriterRegister{}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"read grant only":  {readerRegister{}, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"register is down": {failingRegister{}, "presented.operator.token", http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"granted":          {registryWriterRegister{}, "presented.operator.token", http.StatusOK, ""},
	}
	for name, testCase := range cases {
		response := serveThroughOperatorRegistries(t, testCase.register, http.MethodGet, vocabulary, testCase.token, "")
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		if testCase.status == http.StatusOK && answeredOutcome(t, response) != "PUBLICATION_VOCABULARY_LISTED" {
			t.Fatalf("%s: body = %s", name, response.Body.Bytes())
		}
	}
}

// Covers: 票 operator-channel/06 判断项——`/commercial-publications` 不随发布路径四口换：操作者渠道配好、授予齐备，它仍答
// 403 未配置，带不带令牌都一样（未配置的 Intake 不读请求）。
func TestCommercialPublicationsStaysUnconfiguredOnTheOperatorChannel(t *testing.T) {
	for _, token := range []string{"", "presented.operator.token"} {
		response := serveThroughOperatorRegistries(t, registryWriterRegister{}, http.MethodPost, "/commercial-publications", token, `{}`)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("token %q: answer = %d %q, want 403 ACCESS_CHANNEL_NOT_CONFIGURED", token, response.Code, problemCode(t, response))
		}
	}
}

// tokenSubjects 是替身核验方：令牌就是 sub——录入与批准各持一枚，同一租户下认证出两个操作者。
type tokenSubjects struct{}

func (tokenSubjects) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	if credential.Token() == "" {
		return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
	}
	return accessidentity.NewOperatorSubject("https://id.syn.example/dex", credential.Token())
}

const operatorCreditPolicyPayload = `{
  "kind": "CREDIT_POLICY",
  "objectId": "SYN-OC06-CREDIT-1",
  "version": "v1",
  "scope": "SYN-OC06-SCOPE",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "creditPolicy": {
    "legalEntity": "SYN-OC06-LEGAL",
    "authorityLevel": "SYN-LEVEL-COMMERCIAL",
    "chargeType": "SYN-CHARGE-FREIGHT",
    "limitMinor": 500000,
    "effectiveStartsAt": "2026-08-01T00:00:00Z"
  }
}`

// Covers: 票 operator-channel/06 完成判据「发布待批准载体上录入者与批准者都来自信封」的装配那一半——真铸造、真 PC 认证
// 适配器、操作者 Intake 与真库载体编排连起来。规则没登时批准答未配置；登了「录入者与批准者不同」后，录入那枚令牌自批
// 答需换人，另一枚批得下来；库里读回的录入者与批准者就是两枚令牌认证出的主体。规则要的等级取登记册配置写那一格——
// 授予集照信封持有的能力面译（PC 认证适配器里的暂定译法）。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelRecordsEnvelopeSubjectsOnThePublicationCarrier(t *testing.T) {
	const submitter, approver = "SYN-OPERATOR-SUBMITTER", "SYN-OPERATOR-APPROVER"
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	minter, err := buildOperatorMinter(tokenSubjects{}, registryWriterRegister{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	submit := commercialhttp.NewSubmitPublicationDraftEndpoint(registries.commercial, registration.publicationDrafts)
	approve := commercialhttp.NewApprovePublicationDraftEndpoint(registries.commercial, registration.publicationDrafts)
	post := func(endpoint http.Handler, token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}
	const reference = `{"kind": "CREDIT_POLICY", "objectId": "SYN-OC06-CREDIT-1", "version": "v1"}`

	if response := post(submit, submitter, operatorCreditPolicyPayload); response.Code != http.StatusCreated {
		t.Fatalf("录入 = %d %s", response.Code, response.Body.Bytes())
	}
	if response := post(approve, approver, reference); answeredOutcome(t, response) != "NOT_CONFIGURED" {
		t.Fatalf("规则未登时批准 = %d %s", response.Code, response.Body.Bytes())
	}

	ctx := t.Context()
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-01")
	rules, err := pcpostgres.NewApprovalDutyRules(db)
	if err != nil {
		t.Fatalf("规则册：%v", err)
	}
	rule, err := commercialdomain.NewApprovalDutyRule(tenant, true, pcSynthetic(t, commercialdomain.NewAuthorityLevel, "REGISTRY_CONFIGURATION_WRITE"))
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := rules.SaveApprovalDutyRule(txCtx, rule)
		return err
	}); err != nil {
		t.Fatalf("登记审批职责规则：%v", err)
	}

	if response := post(approve, submitter, reference); answeredOutcome(t, response) != "NEEDS_ANOTHER_APPROVER" {
		t.Fatalf("录入者自批 = %d %s", response.Code, response.Body.Bytes())
	}
	if response := post(approve, approver, reference); response.Code != http.StatusOK || answeredOutcome(t, response) != "DRAFT_APPROVED" {
		t.Fatalf("另一个操作者批准 = %d %s", response.Code, response.Body.Bytes())
	}

	drafts, err := pcpostgres.NewPublicationDrafts(db)
	if err != nil {
		t.Fatalf("载体册：%v", err)
	}
	draft, found, err := drafts.LoadDraft(ctx, tenant, commercialdomain.CreditPolicyObject,
		pcSynthetic(t, commercialdomain.NewCommercialObjectID, "SYN-OC06-CREDIT-1"), pcSynthetic(t, commercialdomain.NewCommercialVersionLabel, "v1"))
	if err != nil || !found {
		t.Fatalf("读回载体：found = %v, err = %v", found, err)
	}
	recorded, approved := draft.Approver()
	if draft.Submitter().String() != "https://id.syn.example/dex#"+submitter || !approved || recorded.String() != "https://id.syn.example/dex#"+approver {
		t.Fatalf("录入者 = %q, 批准者 = %q（已批 %v）", draft.Submitter(), recorded, approved)
	}
}
