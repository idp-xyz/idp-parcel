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

// 本文件钉运营操作者面发布路径换上操作者渠道之后装配点那一半（票 operator-channel/06、18）。各 POST 口的拒绝格、未带令牌
// 与自报租户由 TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades 与 TestSwappedRegistryFacesAnswerFromTheOperatorChannel
// 逐口钉；这里是 GET 的词表口、受控批文镜像在真库上的业务结果，以及真库载体上记下的录入者与批准者。

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
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(unconfiguredOperatorDecisions(), registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil))
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

// controlledRulePackageItem 是受控批文里的一项、不带 tenantId：在线镜像就是这一形。
const controlledRulePackageItem = `{"kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "SYN-OC18-RULES", "version": "v1",
  "scope": "SYN-OC18-SCOPE", "contentDigest": "sha256:SYN-OC18-RULES", "effectiveStartsAt": "2026-01-01T00:00:00Z",
  "approval": {"reference": "SYN-OC18-APPROVAL", "source": "SYN-OC18-SOURCE", "approvedAt": "2026-01-02T00:00:00Z"},
  "approvalRoleStanding": "CONFIRMED"}`

// Covers: 票 operator-channel/18 完成判据二的业务结果那一格——真铸造、真 PC 认证适配器、操作者 Intake 与真库发布编排连起来：
// 授予齐备时受控批文的一项发在信封绑的租户下（201 已发布已生效），同一份再发答重放；项上自报租户答请求畸形，库里一行
// 不写。三格与发行方未配置在 swappedRegistryFaces 与 TestEveryAssembledEndpointAnswersUnconfigured 里。测试输入是隔离合成，
// 只记 `S`。
func TestTheOperatorChannelPublishesTheControlledDocumentMirrorAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	publish := commercialhttp.NewPublishCommercialAuthorityEndpoint(registries.commercial, registration.publication)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/commercial-publications", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		publish.ServeHTTP(response, request)
		return response
	}
	versions := func(tenant string) int {
		var count int
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM party_commercial.commercial_version WHERE tenant_id = $1 AND object_id = $2`,
			tenant, "SYN-OC18-RULES").Scan(&count); err != nil {
			t.Fatalf("统计版本行：%v", err)
		}
		return count
	}

	selfReported := strings.Replace(controlledRulePackageItem, `{`, `{"tenantId": "SYN-TENANT-02", `, 1)
	if response := post(`{"items": [` + selfReported + `]}`); response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("项上自报租户 = %d %s", response.Code, response.Body.Bytes())
	}
	if response := post(`{"items": [` + controlledRulePackageItem + `]}`); response.Code != http.StatusCreated || answeredOutcome(t, response) != "PUBLISHED_EFFECTIVE" {
		t.Fatalf("首发 = %d %s", response.Code, response.Body.Bytes())
	}
	if response := post(`{"items": [` + controlledRulePackageItem + `]}`); response.Code != http.StatusOK || answeredOutcome(t, response) != "REPLAYED" {
		t.Fatalf("重放 = %d %s", response.Code, response.Body.Bytes())
	}
	if inEnvelopeTenant, inSelfReported := versions("SYN-TENANT-01"), versions("SYN-TENANT-02"); inEnvelopeTenant != 1 || inSelfReported != 0 {
		t.Fatalf("版本行：信封租户 %d、自报租户 %d，want 1、0", inEnvelopeTenant, inSelfReported)
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
