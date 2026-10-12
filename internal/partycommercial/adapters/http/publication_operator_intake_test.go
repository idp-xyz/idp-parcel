package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证运营操作者面发布路径挂上操作者渠道之后的那一半（票 operator-channel/06）：预览、载体三口与词表读口在
// OperatorRegistryIntake 上的译法——租户、录入者与批准者都取认证出的身份，载荷里的身份格一律拒，令牌不过时请求体一个
// 字节都不读；词表读口把操作者渠道的各格答到线上。载荷形状与用例转写由 publication_draft_test.go 钉，这里不重复。

const envelopeIssuer = "https://id.syn.example/dex"

// envelopeOperators 是认证方替身：令牌就是 sub——同一租户下两枚令牌是两个操作者，录入者与批准者才分得开；levels 给某枚
// 令牌配授予集，没配的持空集。err 非空时带令牌的请求一律答它，演认证方的各格。
type envelopeOperators struct {
	levels map[string][]string
	err    error
}

func (operators envelopeOperators) AuthenticateRegistryWrite(_ context.Context, token string) (commercialhttp.OperatorIdentity, error) {
	if token == "" {
		return commercialhttp.OperatorIdentity{}, commercialhttp.ErrOperatorCredentialRejected
	}
	if operators.err != nil {
		return commercialhttp.OperatorIdentity{}, operators.err
	}
	tenant, err := domain.NewTenantID(pcTenant)
	if err != nil {
		return commercialhttp.OperatorIdentity{}, err
	}
	reference, err := domain.NewOperatorSubjectReference(envelopeIssuer + "#" + token)
	if err != nil {
		return commercialhttp.OperatorIdentity{}, err
	}
	var levels []domain.AuthorityLevel
	for _, name := range operators.levels[token] {
		level, err := domain.NewAuthorityLevel(name)
		if err != nil {
			return commercialhttp.OperatorIdentity{}, err
		}
		levels = append(levels, level)
	}
	subject, err := domain.NewOperatorSubject(reference, levels)
	return commercialhttp.OperatorIdentity{Tenant: tenant, Operator: subject}, err
}

func publicationOperatorIntake(t *testing.T, operators envelopeOperators) *commercialhttp.OperatorRegistryIntake {
	t.Helper()
	intake, err := commercialhttp.NewOperatorRegistryIntake(operators)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func serveOperator(handler http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

const creditDraftReference = `{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1"}`

// Covers: 票 operator-channel/06 完成判据「发布待批准载体上录入者与批准者都来自信封」——录入者是录入那枚令牌认证出的
// 主体；批准者是批准那枚令牌认证出的主体整个（引用 + 授予集），批准门拿它比录入者与规则要求的那一级：规则没登答未配置，
// 录入者自批答需换人，换了人却不持那一级答不合格，持那一级的另一个人才批得下来；载体上记下的两个身份就是那两枚令牌的。
func TestPublicationDraftPathTakesSubmitterAndApproverFromTheEnvelope(t *testing.T) {
	const submitter, clerk, approver = "SYN-OPERATOR-SUBMITTER", "SYN-OPERATOR-CLERK", "SYN-OPERATOR-APPROVER"
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	intake := publicationOperatorIntake(t, envelopeOperators{levels: map[string][]string{approver: {"SYN-LEVEL-APPROVER"}}})
	store := newDraftStore()
	rule, err := domain.NewApprovalDutyRule(tenant, true, pcNew(t, domain.NewAuthorityLevel, "SYN-LEVEL-APPROVER"))
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	withRule := dutyRuleStore{rule: rule, found: true}

	submit := commercialhttp.NewSubmitPublicationDraftEndpoint(intake, newDraftOperator(store, withRule, pcClock{at: pcNow}))
	if response := serveOperator(submit, http.MethodPost, "/", submitter, standalonePayload); response.Code != http.StatusCreated {
		t.Fatalf("submit = %d %s", response.Code, response.Body.Bytes())
	}

	for _, step := range []struct {
		rules dutyRuleStore
		token string
		want  string
	}{
		{dutyRuleStore{}, approver, "NOT_CONFIGURED"},
		{withRule, submitter, "NEEDS_ANOTHER_APPROVER"},
		{withRule, clerk, "APPROVER_NOT_QUALIFIED"},
		{withRule, approver, "DRAFT_APPROVED"},
	} {
		approve := commercialhttp.NewApprovePublicationDraftEndpoint(intake, newDraftOperator(store, step.rules, pcClock{at: pcNow.Add(time.Hour)}))
		response := serveOperator(approve, http.MethodPost, "/", step.token, creditDraftReference)
		if got := decodeDraftAnswer(t, response).Outcome; response.Code != http.StatusOK || got != step.want {
			t.Fatalf("approve by %s = %d %s, want %s", step.token, response.Code, got, step.want)
		}
	}

	draft, found, err := store.LoadDraft(context.Background(), tenant, domain.CreditPolicyObject,
		pcNew(t, domain.NewCommercialObjectID, "credit-1"), pcNew(t, domain.NewCommercialVersionLabel, "v1"))
	if err != nil || !found {
		t.Fatalf("draft found = %v, err = %v", found, err)
	}
	recordedApprover, approved := draft.Approver()
	if draft.Submitter().String() != envelopeIssuer+"#"+submitter || !approved || recordedApprover.String() != envelopeIssuer+"#"+approver {
		t.Fatalf("submitter = %q, approver = %q (approved %v)", draft.Submitter(), recordedApprover, approved)
	}
}

// Covers: 载荷里只有内容（ADR-0126 决定四）——四口译出的命令里租户、录入者与批准者都是认证出的那一份；载荷里带身份格的
// 一律答请求畸形，改载荷才会好。
func TestPublicationOperatorIntakeTakesIdentityOnlyFromTheEnvelope(t *testing.T) {
	const token = "SYN-OPERATOR-01"
	ctx := context.Background()
	intake := publicationOperatorIntake(t, envelopeOperators{levels: map[string][]string{token: {"SYN-LEVEL-APPROVER"}}})
	reference := envelopeIssuer + "#" + token

	preview, err := intake.IntakeCommercialPublicationPreview(ctx, operatorRequest(token, creditPolicyPayload))
	if err != nil || preview.Shell.TenantID.String() != pcTenant {
		t.Fatalf("preview shell = %+v, err = %v", preview.Shell, err)
	}
	submission, err := intake.IntakePublicationDraftSubmission(ctx, operatorRequest(token, creditPolicyPayload))
	if err != nil || submission.Shell.TenantID.String() != pcTenant || submission.Submitter.String() != reference {
		t.Fatalf("submission tenant = %q, submitter = %q, err = %v", submission.Shell.TenantID, submission.Submitter, err)
	}
	approval, err := intake.IntakePublicationDraftApproval(ctx, operatorRequest(token, creditDraftReference))
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if levels := approval.Approver.Levels(); approval.Tenant.String() != pcTenant || approval.Approver.Reference().String() != reference ||
		len(levels) != 1 || levels[0].String() != "SYN-LEVEL-APPROVER" {
		t.Fatalf("approval tenant = %q, approver = %q, levels = %v", approval.Tenant, approval.Approver.Reference(), levels)
	}
	publication, err := intake.IntakePublicationDraftPublication(ctx, operatorRequest(token, creditDraftReference))
	if err != nil || publication.Tenant.String() != pcTenant || publication.ObjectID.String() != "credit-1" {
		t.Fatalf("publication = %+v, err = %v", publication, err)
	}

	smuggle := func(raw, field string) string { return "{" + field + "," + strings.TrimPrefix(raw, "{") }
	for name, refusal := range map[string]func() error{
		"preview carrying tenantId": func() error {
			_, err := intake.IntakeCommercialPublicationPreview(ctx, operatorRequest(token, smuggle(creditPolicyPayload, `"tenantId": "SYN-TENANT-02"`)))
			return err
		},
		"submission carrying tenantId": func() error {
			_, err := intake.IntakePublicationDraftSubmission(ctx, operatorRequest(token, smuggle(creditPolicyPayload, `"tenantId": "SYN-TENANT-02"`)))
			return err
		},
		"submission carrying a submitter": func() error {
			_, err := intake.IntakePublicationDraftSubmission(ctx, operatorRequest(token, smuggle(creditPolicyPayload, `"submitter": "someone-else"`)))
			return err
		},
		"approval carrying an approver": func() error {
			_, err := intake.IntakePublicationDraftApproval(ctx, operatorRequest(token, smuggle(creditDraftReference, `"approver": "someone-else"`)))
			return err
		},
		"approval carrying levels": func() error {
			_, err := intake.IntakePublicationDraftApproval(ctx, operatorRequest(token, smuggle(creditDraftReference, `"levels": ["SYN-LEVEL-APPROVER"]`)))
			return err
		},
		"publication carrying tenantId": func() error {
			_, err := intake.IntakePublicationDraftPublication(ctx, operatorRequest(token, smuggle(creditDraftReference, `"tenantId": "SYN-TENANT-02"`)))
			return err
		},
	} {
		if err := refusal(); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// Covers: 先认证、后解载荷——令牌不过、或认证方答了别的格，五口都答认证方那一格，请求体一个字节都不读。
func TestPublicationOperatorIntakeAuthenticatesBeforeReadingTheBody(t *testing.T) {
	ctx := context.Background()
	for name, testCase := range map[string]struct {
		operators envelopeOperators
		token     string
		want      error
	}{
		"no token":    {envelopeOperators{}, "", commercialhttp.ErrOperatorCredentialRejected},
		"not granted": {envelopeOperators{err: commercialhttp.ErrOperatorNotGranted}, "SYN-OPERATOR-01", commercialhttp.ErrOperatorNotGranted},
	} {
		intake := publicationOperatorIntake(t, testCase.operators)
		for face, call := range map[string]func(*http.Request) error{
			"preview": func(request *http.Request) error {
				_, err := intake.IntakeCommercialPublicationPreview(ctx, request)
				return err
			},
			"submission": func(request *http.Request) error {
				_, err := intake.IntakePublicationDraftSubmission(ctx, request)
				return err
			},
			"approval": func(request *http.Request) error {
				_, err := intake.IntakePublicationDraftApproval(ctx, request)
				return err
			},
			"publication": func(request *http.Request) error {
				_, err := intake.IntakePublicationDraftPublication(ctx, request)
				return err
			},
			"vocabulary": func(request *http.Request) error {
				return intake.IntakePublicationVocabularyQuery(ctx, request)
			},
		} {
			probe := &bodyReadProbe{}
			request := httptest.NewRequest(http.MethodPost, "/", probe)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			if err := call(request); !errors.Is(err, testCase.want) || probe.read {
				t.Fatalf("%s, %s: err = %v (want %v), body read = %v", name, face, err, testCase.want, probe.read)
			}
		}
	}
}

// Covers: 预览口挂上操作者渠道后答案形状不变——载荷立得住答 PREVIEWED 带摘要；逐格问题仍是一格答案（200 + NOT_ACCEPTED
// + problems），不是 400。
func TestPreviewEndpointAnswersThroughTheOperatorChannel(t *testing.T) {
	endpoint := commercialhttp.NewPreviewCommercialPublicationEndpoint(
		publicationOperatorIntake(t, envelopeOperators{}), application.NewPreviewCommercialPublicationHandler())

	previewed := serveOperator(endpoint, http.MethodPost, "/", "SYN-OPERATOR-01", creditPolicyPayload)
	if body := decodeDraftAnswer(t, previewed); previewed.Code != http.StatusOK || body.Outcome != "PREVIEWED" || !strings.HasPrefix(body.ContentDigest, "PCC-1:") {
		t.Fatalf("preview = %d %s", previewed.Code, previewed.Body.Bytes())
	}
	broken := serveOperator(endpoint, http.MethodPost, "/", "SYN-OPERATOR-01",
		strings.Replace(creditPolicyPayload, `"scope": "SYN-SCOPE-PC02C"`, `"scope": " "`, 1))
	body := decodeDraftAnswer(t, broken)
	if broken.Code != http.StatusOK || body.Outcome != "NOT_ACCEPTED" || len(body.Problems) != 1 || body.Problems[0].Field != "scope" {
		t.Fatalf("field problems = %d %s", broken.Code, broken.Body.Bytes())
	}
}

// Covers: ADR-0151 决定五——词表读口与发布四口挂同一个操作者渠道 Intake，答复格也是同一组：认证过了照常列词（200），令牌
// 不过 401，未授予 403，身份依赖读不动 503，发行方未配置仍是 403 未配置；拒的各格不带 outcome。
func TestVocabularyEndpointAnswersTheOperatorChannelGrades(t *testing.T) {
	const target = "/commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE"
	vocabulary := func(operators envelopeOperators, token string) *httptest.ResponseRecorder {
		return serveOperator(commercialhttp.NewQueryPublicationVocabularyEndpoint(publicationOperatorIntake(t, operators)), http.MethodGet, target, token, "")
	}

	listed := vocabulary(envelopeOperators{}, "SYN-OPERATOR-01")
	if body := decodeVocabulary(t, listed); listed.Code != http.StatusOK || body.Outcome != "PUBLICATION_VOCABULARY_LISTED" || len(body.Sets) == 0 {
		t.Fatalf("granted = %d %s", listed.Code, listed.Body.Bytes())
	}
	for name, testCase := range map[string]struct {
		operators envelopeOperators
		token     string
		status    int
		code      string
	}{
		"no token":            {envelopeOperators{}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"not granted":         {envelopeOperators{err: commercialhttp.ErrOperatorNotGranted}, "SYN-OPERATOR-01", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"identity down":       {envelopeOperators{err: commercialhttp.ErrIdentityDependencyUnavailable}, "SYN-OPERATOR-01", http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"issuer unconfigured": {envelopeOperators{err: commercialhttp.ErrAccessChannelNotConfigured}, "SYN-OPERATOR-01", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
	} {
		response := vocabulary(testCase.operators, testCase.token)
		if response.Code != testCase.status || errorCode(t, response) != testCase.code || strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s: %d %s, want %d %s", name, response.Code, response.Body.Bytes(), testCase.status, testCase.code)
		}
	}
}
