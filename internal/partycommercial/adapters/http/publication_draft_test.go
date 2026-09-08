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
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对运营操作者面发布路径的载荷与四个端点（ADR-0126 Decision 三、四；票 admin-write-faces/08）证传输面：
// 载荷严格解码（身份与摘要的键一律拒）、逐格问题收齐、同一份载荷过预览与过录入逐字节同摘要；预览口把逐格
// 问题答成一格；四口未配置 403 且不读内容；三个命令口按真用例逐名转写答案；隔离读 Intake 装不进任何一口
// （编译期，见文末）。

const creditPolicyPayload = `{
  "kind": "CREDIT_POLICY",
  "objectId": "credit-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "references": {"SERVICE_PRODUCT": "product-1"},
  "creditPolicy": {
    "legalEntity": "legal-1",
    "authorityLevel": "level-commercial",
    "chargeType": "charge-freight",
    "limitMinor": 500000,
    "effectiveStartsAt": "2026-08-01T00:00:00Z"
  }
}`

// standalonePayload 是同一份载荷去掉指名引用：发布用例对指名引用未发布答`发布未决`（AT-PC-005），要看落定就得
// 不指名任何对象——两份并存，正好让一条用例演落定、另一条演未落定。
var standalonePayload = strings.Replace(creditPolicyPayload, `  "references": {"SERVICE_PRODUCT": "product-1"},
`, "", 1)

func decodePublication(t *testing.T, raw string) commercialhttp.CommercialPublicationPayload {
	t.Helper()
	payload, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return payload
}

// Covers: 载荷里只有内容没有身份也没有摘要——tenant / submitter / contentDigest / approval 之类的键按未知键拒
// （ErrMalformedRequest）；尾随第二份文档也拒。
func TestPublicationPayloadRejectsIdentityDigestAndUnknownKeys(t *testing.T) {
	for _, extra := range []string{`"tenantId": "T-9"`, `"submitter": "op-9"`, `"contentDigest": "PCC-1:ff"`, `"approval": {}`, `"roleStanding": "CONFIRMED"`} {
		raw := strings.Replace(creditPolicyPayload, `"kind": "CREDIT_POLICY",`, extra+`, "kind": "CREDIT_POLICY",`, 1)
		if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(raw)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("payload with %s: err = %v, want ErrMalformedRequest", extra, err)
		}
	}
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(creditPolicyPayload + ` {"kind":"CREDIT_POLICY"}`)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("trailing document: err = %v, want ErrMalformedRequest", err)
	}
	if _, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(`{"kind":"CREDIT_POLICY","objectId":"c","version":"v1","approver":"op-9"}`)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("reference payload with approver: err = %v, want ErrMalformedRequest", err)
	}
	if _, _, err := decodePublication(t, creditPolicyPayload).Publication(domain.TenantID{}); !errors.Is(err, commercialhttp.ErrOperatorIdentityMissing) {
		t.Fatalf("no tenant from the envelope: err = %v, want ErrOperatorIdentityMissing", err)
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」——解码把每一格的构造门结果都收齐再答，不撞第一格就停；收齐的问题
// Is ErrMalformedRequest。
func TestPublicationPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "NOT_A_KIND",
	  "objectId": "",
	  "version": "v1",
	  "scope": " ",
	  "effectiveStartsAt": "yesterday",
	  "references": {"NOPE": "x"},
	  "creditPolicy": {
	    "legalEntity": "",
	    "authorityLevel": "level-commercial",
	    "chargeType": "charge-freight",
	    "limitMinor": 1,
	    "limitRatioBasisPoints": 1,
	    "effectiveStartsAt": "2026-08-01T00:00:00Z",
	    "effectiveEndsAt": "2026-07-01T00:00:00Z"
	  }
	}`
	_, _, err := decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) {
		t.Fatalf("err = %T, want *PublicationPayloadProblems", err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	for _, want := range []string{"kind", "objectId", "scope", "effectiveStartsAt", "references.NOPE",
		"creditPolicy.legalEntity", "creditPolicy.limit", "creditPolicy.effectiveEndsAt"} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	if fields["version"] || fields["creditPolicy.authorityLevel"] {
		t.Fatalf("a valid field was reported: %v", fields)
	}
}

// Covers: ADR-0126 Decision 四 — 同一份载荷过预览与过录入逐字节同摘要：两条路径都从 Publication 出发，摘要只在
// 领域一处算。
func TestPreviewAndSubmissionOfTheSamePayloadShareTheDigest(t *testing.T) {
	payload := decodePublication(t, creditPolicyPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v", preview.Outcome(), err)
	}
	previewed, _ := preview.Canonical()

	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitted, err := application.NewSubmitPublicationDraftHandler(newDraftStore(), pcClock{at: pcNow}).Handle(context.Background(), submitCommand)
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("submit = %q, %v", submitted.Outcome(), err)
	}
	draft, _ := submitted.Draft()
	if draft.Canonical().Digest() != previewed.Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest %s", previewed.Digest(), draft.Canonical().Digest())
	}
	if !strings.HasPrefix(draft.Canonical().Digest().String(), "PCC-1:") {
		t.Fatalf("digest %s does not carry its canonicalization", draft.Canonical().Digest())
	}
	if reference, ok := draft.PublicationSpec().References[domain.ServiceProductObject]; !ok || reference.String() != "product-1" {
		t.Fatalf("references did not travel: %#v", draft.PublicationSpec().References)
	}

	if _, err := payload.SubmitCommand(tenant, domain.OperatorSubjectReference{}); !errors.Is(err, commercialhttp.ErrOperatorIdentityMissing) {
		t.Fatalf("no submitter from the envelope: err = %v", err)
	}
}

// ---- 端点 ----

// draftIntakeDouble 交回测试预先备好的命令或错误，对请求零读取（判据同 commercialIntakeDouble）。
type draftIntakeDouble struct {
	preview application.PreviewCommercialPublicationCommand
	submit  application.SubmitPublicationDraftCommand
	approve application.ApprovePublicationDraftCommand
	publish application.PublishPublicationDraftCommand
	err     error
}

func (double draftIntakeDouble) IntakeCommercialPublicationPreview(context.Context, *http.Request) (application.PreviewCommercialPublicationCommand, error) {
	return double.preview, double.err
}

func (double draftIntakeDouble) IntakePublicationDraftSubmission(context.Context, *http.Request) (application.SubmitPublicationDraftCommand, error) {
	return double.submit, double.err
}

func (double draftIntakeDouble) IntakePublicationDraftApproval(context.Context, *http.Request) (application.ApprovePublicationDraftCommand, error) {
	return double.approve, double.err
}

func (double draftIntakeDouble) IntakePublicationDraftPublication(context.Context, *http.Request) (application.PublishPublicationDraftCommand, error) {
	return double.publish, double.err
}

// draftStore 是一份内存里的载体册，按端口代数作答；只为端点转写用例提供真用例可跑的底座。
type draftStore struct {
	rows map[string]domain.PublicationDraft
}

func newDraftStore() *draftStore { return &draftStore{rows: map[string]domain.PublicationDraft{}} }

func draftStoreKey(tenant domain.TenantID, kind domain.CommercialObjectKind, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel) string {
	return tenant.String() + "/" + kind.String() + "/" + objectID.String() + "/" + version.String()
}

func (store *draftStore) SubmitDraft(_ context.Context, draft domain.PublicationDraft) (ports.PublicationDraftSubmitOutcome, error) {
	key := draftStoreKey(draft.Tenant(), draft.Kind(), draft.ObjectID(), draft.Version())
	existing, present := store.rows[key]
	switch {
	case !present:
		store.rows[key] = draft
		return ports.PublicationDraftSaved, nil
	case existing.SameSubmissionAs(draft):
		return ports.PublicationDraftReplayed, nil
	case existing.Status() == domain.PublicationDraftPendingApproval:
		store.rows[key] = draft
		return ports.PublicationDraftRevised, nil
	default:
		return ports.PublicationDraftContentFixed, nil
	}
}

func (store *draftStore) LoadDraft(_ context.Context, tenant domain.TenantID, kind domain.CommercialObjectKind, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel) (domain.PublicationDraft, bool, error) {
	draft, found := store.rows[draftStoreKey(tenant, kind, objectID, version)]
	return draft, found, nil
}

func (store *draftStore) AdvanceDraft(_ context.Context, draft domain.PublicationDraft) (ports.PublicationDraftAdvanceOutcome, error) {
	key := draftStoreKey(draft.Tenant(), draft.Kind(), draft.ObjectID(), draft.Version())
	if _, present := store.rows[key]; !present {
		return ports.PublicationDraftAdvanceNotFound, nil
	}
	store.rows[key] = draft
	return ports.PublicationDraftAdvanced, nil
}

type dutyRuleStore struct {
	rule  domain.ApprovalDutyRule
	found bool
}

func (store dutyRuleStore) LoadApprovalDutyRule(context.Context, domain.TenantID) (domain.ApprovalDutyRule, bool, error) {
	return store.rule, store.found, nil
}

// creditAwareRegistry 让发布用例真登记信用政策正文：stubPublicationRegistry 的那一格对本片说「不用」，这里换掉它。
type creditAwareRegistry struct{ stubPublicationRegistry }

func (creditAwareRegistry) SaveCreditPolicy(context.Context, domain.CreditPolicy) (ports.CreditPolicySaveOutcome, error) {
	return ports.CreditPolicySaved, nil
}

// draftOperator 把三个真用例装成端点要的 PublicationDraftOperator，没有事务壳——传输层用例只看转写。
type draftOperator struct {
	submit  *application.SubmitPublicationDraftHandler
	approve *application.ApprovePublicationDraftHandler
	publish *application.PublishPublicationDraftHandler
}

func (operator draftOperator) Submit(ctx context.Context, command application.SubmitPublicationDraftCommand) (application.SubmitPublicationDraftResult, error) {
	return operator.submit.Handle(ctx, command)
}

func (operator draftOperator) Approve(ctx context.Context, command application.ApprovePublicationDraftCommand) (application.ApprovePublicationDraftResult, error) {
	return operator.approve.Handle(ctx, command)
}

func (operator draftOperator) Publish(ctx context.Context, command application.PublishPublicationDraftCommand) (application.PublishPublicationDraftResult, error) {
	return operator.publish.Handle(ctx, command)
}

func newDraftOperator(store *draftStore, rules dutyRuleStore, clock pcClock) draftOperator {
	publisher := application.NewPublishCommercialAuthorityHandler(creditAwareRegistry{}, clock, pcHandoffDouble{})
	return draftOperator{
		submit:  application.NewSubmitPublicationDraftHandler(store, clock),
		approve: application.NewApprovePublicationDraftHandler(store, rules, clock),
		publish: application.NewPublishPublicationDraftHandler(store, publisher, clock),
	}
}

type draftAnswerBody struct {
	Outcome          string `json:"outcome"`
	Canonicalization string `json:"canonicalization"`
	ContentDigest    string `json:"contentDigest"`
	Status           string `json:"status"`
	Cause            string `json:"cause"`
	Problems         []struct {
		Field   string `json:"field"`
		Problem string `json:"problem"`
	} `json:"problems"`
	Publication *publicationAnswerBody `json:"publication"`
}

func decodeDraftAnswer(t *testing.T, response *httptest.ResponseRecorder) draftAnswerBody {
	t.Helper()
	var body draftAnswerBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body
}

func serve(handler http.Handler, method string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, "/", strings.NewReader(`{"ignored": true}`)))
	return response
}

func unreachableOperator(t *testing.T) draftOperator {
	t.Helper()
	// 未配置 Intake 拒在编排之前：这里的用例一旦被调就会读到空载体册并答业务结果——那正是本测试要证不会发生的。
	// 用一个空 store：任何到达编排的请求都会留下可见的行为差异（答案带 outcome），断言 assertNoOutcome 抓得住。
	return newDraftOperator(newDraftStore(), dutyRuleStore{}, pcClock{at: pcNow})
}

// Covers: ADR-0055 — 四口未配置一律 403 + ACCESS_CHANNEL_NOT_CONFIGURED、响应无 outcome；方法门先于 Intake 答 405。
func TestDraftPathEndpointsAnswerUnconfiguredWithoutReadingTheBody(t *testing.T) {
	unconfigured := commercialhttp.UnconfiguredIntake{}
	operator := unreachableOperator(t)
	endpoints := map[string]http.Handler{
		"preview": commercialhttp.NewPreviewCommercialPublicationEndpoint(unconfigured, application.NewPreviewCommercialPublicationHandler()),
		"submit":  commercialhttp.NewSubmitPublicationDraftEndpoint(unconfigured, operator),
		"approve": commercialhttp.NewApprovePublicationDraftEndpoint(unconfigured, operator),
		"publish": commercialhttp.NewPublishPublicationDraftEndpoint(unconfigured, operator),
	}
	for name, endpoint := range endpoints {
		response := serve(endpoint, http.MethodPost)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", name, response.Code)
		}
		var problem struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: body = %s", name, response.Body.Bytes())
		}
		if strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s: an unconfigured answer carried an outcome", name)
		}
		if wrongMethod := serve(endpoint, http.MethodGet); wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("%s: GET status = %d, Allow = %q", name, wrongMethod.Code, wrongMethod.Header().Get("Allow"))
		}
	}
}

// Covers: ADR-0126 Decision 四 — 预览口：算出来了答 200 + PREVIEWED + 规范化版本与摘要；解码收齐的逐格问题是一格
// 答案（200 + NOT_ACCEPTED + problems），不是 400；用例的`未受理`带成因、不带摘要。
func TestPreviewEndpointAnswersDigestOrFieldProblems(t *testing.T) {
	previewer := application.NewPreviewCommercialPublicationHandler()
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	command, err := decodePublication(t, creditPolicyPayload).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}

	response := serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{preview: command}, previewer), http.MethodPost)
	body := decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "PREVIEWED" || body.Canonicalization != "PCC-1" || !strings.HasPrefix(body.ContentDigest, "PCC-1:") {
		t.Fatalf("preview answer = %d %s", response.Code, response.Body.Bytes())
	}

	problems := &commercialhttp.PublicationPayloadProblems{Problems: []commercialhttp.PayloadProblem{{Field: "scope", Problem: "blank"}}}
	response = serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{err: problems}, previewer), http.MethodPost)
	body = decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "NOT_ACCEPTED" || len(body.Problems) != 1 || body.Problems[0].Field != "scope" || body.ContentDigest != "" {
		t.Fatalf("field problems answer = %d %s", response.Code, response.Body.Bytes())
	}

	notCanonicalized := application.PreviewCommercialPublicationCommand{
		Shell:   command.Shell,
		Content: domain.PublicationContent{Kind: domain.SettlementPolicyObject},
	}
	notCanonicalized.Shell.Kind = domain.SettlementPolicyObject
	response = serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{preview: notCanonicalized}, previewer), http.MethodPost)
	body = decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "NOT_ACCEPTED" || body.Cause == "" || body.ContentDigest != "" {
		t.Fatalf("not canonicalized answer = %d %s", response.Code, response.Body.Bytes())
	}

	malformed := serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{err: commercialhttp.ErrMalformedRequest}, previewer), http.MethodPost)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("plain malformed request: status = %d, want 400", malformed.Code)
	}
}

// Covers: 命令口上的逐格问题走 400 + MALFORMED_REQUEST 并把 problems 带在响应体里——改载荷才会好，只是多说哪几格。
func TestCommandEndpointsCarryFieldProblemsInTheBadRequest(t *testing.T) {
	problems := &commercialhttp.PublicationPayloadProblems{Problems: []commercialhttp.PayloadProblem{{Field: "creditPolicy.limit", Problem: "恰一格"}}}
	response := serve(commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{err: problems}, unreachableOperator(t)), http.MethodPost)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	var body struct {
		Error struct {
			Code     string `json:"code"`
			Problems []struct {
				Field string `json:"field"`
			} `json:"problems"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != "MALFORMED_REQUEST" || len(body.Error.Problems) != 1 || body.Error.Problems[0].Field != "creditPolicy.limit" {
		t.Fatalf("body = %s", response.Body.Bytes())
	}
}

// Covers: ADR-0126 Decision 三 — 三个命令口按真用例逐名转写：录入 201 + DRAFT_SUBMITTED 带规范化版本、摘要与状态，
// 再录 200 + DRAFT_REPLAYED；批准前没有规则 200 + NOT_CONFIGURED，有规则 200 + DRAFT_APPROVED；发布 201 +
// DRAFT_PUBLISHED 并嵌发布用例的答案（与 /commercial-publications 同形）。
func TestDraftCommandEndpointsTranscribeTheUseCaseAnswers(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store := newDraftStore()
	rules := dutyRuleStore{}
	rule, err := domain.NewApprovalDutyRule(tenant, true, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}

	submitCommand, err := decodePublication(t, standalonePayload).SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitEndpoint := func() http.Handler {
		return commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{submit: submitCommand}, newDraftOperator(store, rules, pcClock{at: pcNow}))
	}
	response := serve(submitEndpoint(), http.MethodPost)
	body := decodeDraftAnswer(t, response)
	if response.Code != http.StatusCreated || body.Outcome != "DRAFT_SUBMITTED" || body.Status != "PENDING_APPROVAL" ||
		body.Canonicalization != "PCC-1" || !strings.HasPrefix(body.ContentDigest, "PCC-1:") {
		t.Fatalf("submit answer = %d %s", response.Code, response.Body.Bytes())
	}
	digest := body.ContentDigest
	if response = serve(submitEndpoint(), http.MethodPost); response.Code != http.StatusOK || decodeDraftAnswer(t, response).Outcome != "DRAFT_REPLAYED" {
		t.Fatalf("replay answer = %d %s", response.Code, response.Body.Bytes())
	}

	reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1"}`))
	if err != nil {
		t.Fatalf("reference payload: %v", err)
	}
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	approveCommand, err := reference.ApproveCommand(tenant, approver)
	if err != nil {
		t.Fatalf("approve command: %v", err)
	}
	response = serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, dutyRuleStore{}, pcClock{at: pcNow})), http.MethodPost)
	if body = decodeDraftAnswer(t, response); response.Code != http.StatusOK || body.Outcome != "NOT_CONFIGURED" || body.Status != "" {
		t.Fatalf("approve without a rule = %d %s", response.Code, response.Body.Bytes())
	}
	response = serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, dutyRuleStore{rule: rule, found: true}, pcClock{at: pcNow.Add(time.Hour)})), http.MethodPost)
	if body = decodeDraftAnswer(t, response); response.Code != http.StatusOK || body.Outcome != "DRAFT_APPROVED" || body.Status != "APPROVED" || body.ContentDigest != digest {
		t.Fatalf("approve answer = %d %s", response.Code, response.Body.Bytes())
	}

	publishCommand, err := reference.PublishCommand(tenant)
	if err != nil {
		t.Fatalf("publish command: %v", err)
	}
	response = serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(2 * time.Hour)})), http.MethodPost)
	body = decodeDraftAnswer(t, response)
	if response.Code != http.StatusCreated || body.Outcome != "DRAFT_PUBLISHED" || body.Publication == nil || body.Publication.Outcome != "PUBLISHED_EFFECTIVE" {
		t.Fatalf("publish answer = %d %s", response.Code, response.Body.Bytes())
	}
	if len(body.Publication.Declarations) != 1 || body.Publication.Declarations[0].Channel != "CREDIT_POLICY_BODY" {
		t.Fatalf("publication declarations = %#v", body.Publication.Declarations)
	}
	response = serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(2 * time.Hour)})), http.MethodPost)
	if body = decodeDraftAnswer(t, response); response.Code != http.StatusOK || body.Outcome != "DRAFT_ALREADY_PUBLISHED" || body.Publication != nil {
		t.Fatalf("second publish answer = %d %s", response.Code, response.Body.Bytes())
	}
}

// Covers: 发布用例没落定时载体口答 200 + PUBLICATION_NOT_LANDED 并嵌发布用例的整份答案——这里演`发布未决`（壳
// 指名的服务产品未发布，AT-PC-005）带 pendingCause；载体留在`已批准`，再批答 DRAFT_ALREADY_APPROVED。
func TestDraftPublicationThatDoesNotLandCarriesThePublicationAnswer(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store := newDraftStore()
	rule, err := domain.NewApprovalDutyRule(tenant, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	submitter := pcNew(t, domain.NewOperatorSubjectReference, "op-submitter")

	submitCommand, err := decodePublication(t, creditPolicyPayload).SubmitCommand(tenant, submitter)
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	if response := serve(commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{submit: submitCommand}, newDraftOperator(store, rules, pcClock{at: pcNow})), http.MethodPost); response.Code != http.StatusCreated {
		t.Fatalf("submit = %d %s", response.Code, response.Body.Bytes())
	}
	reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1"}`))
	if err != nil {
		t.Fatalf("reference payload: %v", err)
	}
	approver, err := domain.NewOperatorSubject(submitter, nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	approveCommand, err := reference.ApproveCommand(tenant, approver)
	if err != nil {
		t.Fatalf("approve command: %v", err)
	}
	if response := serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(time.Hour)})), http.MethodPost); decodeDraftAnswer(t, response).Outcome != "DRAFT_APPROVED" {
		t.Fatalf("approve = %d %s", response.Code, response.Body.Bytes())
	}

	publishCommand, err := reference.PublishCommand(tenant)
	if err != nil {
		t.Fatalf("publish command: %v", err)
	}
	response := serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(2 * time.Hour)})), http.MethodPost)
	body := decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "PUBLICATION_NOT_LANDED" || body.Publication == nil ||
		body.Publication.Outcome != "PENDING" || body.Publication.PendingCause == "" {
		t.Fatalf("not landed answer = %d %s", response.Code, response.Body.Bytes())
	}
	response = serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(3 * time.Hour)})), http.MethodPost)
	if decodeDraftAnswer(t, response).Outcome != "DRAFT_ALREADY_APPROVED" {
		t.Fatalf("draft after an unlanded publication = %s", response.Body.Bytes())
	}
}

// Covers: 依赖故障不是业务答案（ADR-0022）——用例上抛时端点答 5xx NO_ANSWER_FORMED，不带 outcome。
func TestDraftCommandEndpointsAnswerNoAnswerFormedOnFailure(t *testing.T) {
	failing := failingDraftOperator{err: errors.New("registry down")}
	for name, endpoint := range map[string]http.Handler{
		"submit":  commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{}, failing),
		"approve": commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{}, failing),
		"publish": commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{}, failing),
	} {
		response := serve(endpoint, http.MethodPost)
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") || strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s: %d %s", name, response.Code, response.Body.Bytes())
		}
	}
}

type failingDraftOperator struct{ err error }

func (operator failingDraftOperator) Submit(context.Context, application.SubmitPublicationDraftCommand) (application.SubmitPublicationDraftResult, error) {
	return application.SubmitPublicationDraftResult{}, operator.err
}

func (operator failingDraftOperator) Approve(context.Context, application.ApprovePublicationDraftCommand) (application.ApprovePublicationDraftResult, error) {
	return application.ApprovePublicationDraftResult{}, operator.err
}

func (operator failingDraftOperator) Publish(context.Context, application.PublishPublicationDraftCommand) (application.PublishPublicationDraftResult, error) {
	return application.PublishPublicationDraftResult{}, operator.err
}

// Covers: ADR-0078 的排除在本路径成立——隔离读放行类型不满足预览口与三个命令口的任何一个 Intake 接口。预览虽
// 不落库也在其中：拟录的壳要信封里的租户，采信隔离读的合成租户就让预览与录入两条路径拿到两个壳
// （判据同 TestIsolatedReadIntakeCannotServeCommercialRegistration）。
func TestIsolatedReadIntakeCannotServeTheDraftPath(t *testing.T) {
	var intake any = commercialhttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(commercialhttp.CommercialPublicationPreviewIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进预览口")
	}
	if _, ok := intake.(commercialhttp.PublicationDraftSubmissionIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进录入口")
	}
	if _, ok := intake.(commercialhttp.PublicationDraftApprovalIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进批准口")
	}
	if _, ok := intake.(commercialhttp.PublicationDraftPublicationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进载体发布口")
	}
}
