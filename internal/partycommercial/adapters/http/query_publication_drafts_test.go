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
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对待批准发布载体的两个读回端点（ADR-0314 决定一）证传输面：列表不带正文、带摘要与状态与痕迹；单份读回带规范化文档
// （正是摘要盖住的那份）；不在册答 200 加 DRAFT_NOT_FOUND；方法门、路径畸形、读失败各自的答复。同一条规则也钉在批准口与发布口的载荷上：
// 期望摘要必填。

// draftReaderDouble 把内存载体册当读口：载体是以 storedUnder 这个租户录入的，只有认证出的租户就是 serves 时才读得到——
// 跨租户与不在册同形（读不到），与真库一致。
type draftReaderDouble struct {
	store       *draftStore
	storedUnder domain.TenantID
	serves      domain.TenantID
	open        []domain.PublicationDraft
	err         error
}

func (double draftReaderDouble) LoadDraft(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (domain.PublicationDraft, bool, error) {
	if double.err != nil {
		return domain.PublicationDraft{}, false, double.err
	}
	if tenant != double.serves {
		return domain.PublicationDraft{}, false, nil
	}
	return double.store.LoadDraft(ctx, double.storedUnder, kind, objectID, version)
}

func (double draftReaderDouble) ListOpenDrafts(_ context.Context, _ domain.TenantID, _ int) ([]domain.PublicationDraft, error) {
	return double.open, double.err
}

// submittedAndApprovedDrafts 经真端点录入两份载体（一份指名引用、一份不指名），再批准后一份：读回要证的就是这条链上落下的内容。
func submittedAndApprovedDrafts(t *testing.T) (*draftStore, domain.PublicationDraft, domain.PublicationDraft) {
	t.Helper()
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store := newDraftStore()
	rule, err := domain.NewApprovalDutyRule(tenant, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	submitter := pcNew(t, domain.NewOperatorSubjectReference, "op-submitter")
	for _, raw := range []string{creditPolicyPayload, strings.Replace(standalonePayload, `"objectId": "credit-1"`, `"objectId": "credit-2"`, 1)} {
		command, err := decodePublication(t, raw).SubmitCommand(tenant, submitter)
		if err != nil {
			t.Fatalf("submit command: %v", err)
		}
		endpoint := commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{submit: command}, newDraftOperator(store, rules, pcClock{at: pcNow}))
		if response := serve(endpoint, http.MethodPost); response.Code != http.StatusCreated {
			t.Fatalf("submit = %d %s", response.Code, response.Body.Bytes())
		}
	}
	load := func(objectID string) domain.PublicationDraft {
		draft, found, err := store.LoadDraft(context.Background(), tenant, domain.CreditPolicyObject,
			pcNew(t, domain.NewCommercialObjectID, objectID), pcNew(t, domain.NewCommercialVersionLabel, "v1"))
		if err != nil || !found {
			t.Fatalf("draft %s found = %v, err = %v", objectID, found, err)
		}
		return draft
	}
	pending := load("credit-1")
	second := load("credit-2")
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	approved, err := second.Approve(approver, rule, pcNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return store, pending, approved
}

func draftReadbackRequest(kind, objectID, version string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/commercial-publication-draft-readbacks/"+kind+"/"+objectID+"/"+version, nil)
	// 单测不经路由，路径参数由这里按路由会做的方式填进去；装配后的形状由 cmd/parcel-api 的隔离读用例经真路由再证一次。
	request.SetPathValue("kind", kind)
	request.SetPathValue("objectId", objectID)
	request.SetPathValue("version", version)
	return request
}

// Covers: AT-PC-068——列表交出还没发布的载体，每份带摘要、录入者与时刻、状态；已批准的那份带批准者与时刻，待批准的没有；列表不带正文。
func TestOpenPublicationDraftsAreListedWithoutTheirContent(t *testing.T) {
	_, pending, approved := submittedAndApprovedDrafts(t)
	endpoint := commercialhttp.NewQueryPublicationDraftsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, draftReaderDouble{open: []domain.PublicationDraft{approved, pending}})

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-open-publication-drafts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Drafts  []map[string]json.RawMessage `json:"drafts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PUBLICATION_DRAFTS_LISTED" || len(body.Drafts) != 2 {
		t.Fatalf("body = %s", recorder.Body)
	}
	str := func(draft map[string]json.RawMessage, key string) string {
		var value string
		_ = json.Unmarshal(draft[key], &value)
		return value
	}
	first, second := body.Drafts[0], body.Drafts[1]
	if str(first, "objectId") != "credit-2" || str(first, "status") != "APPROVED" || str(first, "approver") != "op-approver" ||
		str(first, "approvedAt") == "" || str(first, "contentDigest") != approved.Canonical().Digest().String() {
		t.Fatalf("已批准的一份 = %v", first)
	}
	if str(second, "objectId") != "credit-1" || str(second, "status") != "PENDING_APPROVAL" || str(second, "submitter") != "op-submitter" {
		t.Fatalf("待批准的一份 = %v", second)
	}
	for _, key := range []string{"approver", "approvedAt", "publishedAt"} {
		if _, has := second[key]; has {
			t.Fatalf("待批准的载体长出了 %q", key)
		}
	}
	for _, draft := range body.Drafts {
		if _, has := draft["content"]; has {
			t.Fatal("列表带了正文：列表随载体数膨胀，正文只在单份读回上给")
		}
	}
	if string(second["references"]) != `[{"kind":"SERVICE_PRODUCT","objectId":"product-1"}]` {
		t.Fatalf("指名引用 = %s", second["references"])
	}
}

// Covers: AT-PC-068——没有待批准的载体答 200 加空数组（空是内容）；读失败答 5xx，不伪装成空列表。
func TestOpenPublicationDraftsAnswerEmptyAsContentAndFailureAsNoAnswer(t *testing.T) {
	empty := commercialhttp.NewQueryPublicationDraftsEndpoint(intakeDouble{query: catalogueQuery(t)}, draftReaderDouble{})
	recorder := httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || string(body["drafts"]) != "[]" {
		t.Fatalf("空列表 = %d %s", recorder.Code, recorder.Body)
	}

	failing := commercialhttp.NewQueryPublicationDraftsEndpoint(intakeDouble{query: catalogueQuery(t)}, draftReaderDouble{err: errors.New("boom")})
	failed := httptest.NewRecorder()
	failing.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/", nil))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 = %d, want 500", failed.Code)
	}

	post := httptest.NewRecorder()
	empty.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d allow=%q", post.Code, post.Header().Get("Allow"))
	}
}

// Covers: AT-PC-068——单份读回带规范化文档，且文档正是摘要盖住的那份（载体重建门已校验）；不在册答 200 加 DRAFT_NOT_FOUND；路径畸形答 400。
func TestAPublicationDraftIsReadBackWithTheContentItsDigestCovers(t *testing.T) {
	store, pending, _ := submittedAndApprovedDrafts(t)
	query := catalogueQuery(t)
	reader := draftReaderDouble{store: store, storedUnder: pcNew(t, domain.NewTenantID, pcTenant), serves: query.Scope.Tenant()}
	endpoint := commercialhttp.NewQueryPublicationDraftEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, draftReadbackRequest("CREDIT_POLICY", "credit-1", "v1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Draft   struct {
			ContentDigest string          `json:"contentDigest"`
			Content       json.RawMessage `json:"content"`
			Status        string          `json:"status"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PUBLICATION_DRAFT_READ" || body.Draft.Status != "PENDING_APPROVAL" ||
		body.Draft.ContentDigest != pending.Canonical().Digest().String() {
		t.Fatalf("body = %s", recorder.Body)
	}
	var document, want map[string]any
	if err := json.Unmarshal(body.Draft.Content, &document); err != nil || len(document) == 0 {
		t.Fatalf("content 不是 JSON 对象：%s (%v)", body.Draft.Content, err)
	}
	if err := json.Unmarshal(pending.Canonical().Document(), &want); err != nil {
		t.Fatalf("canonical document: %v", err)
	}
	if got, _ := json.Marshal(document); string(got) == "" {
		t.Fatal("content 为空")
	}
	if document["kind"] != want["kind"] || document["canonicalization"] != want["canonicalization"] {
		t.Fatalf("content = %v, want the canonical document %v", document, want)
	}

	missing := httptest.NewRecorder()
	endpoint.ServeHTTP(missing, draftReadbackRequest("CREDIT_POLICY", "credit-nobody", "v1"))
	var absent map[string]json.RawMessage
	if err := json.Unmarshal(missing.Body.Bytes(), &absent); err != nil || missing.Code != http.StatusOK || string(absent["outcome"]) != `"DRAFT_NOT_FOUND"` {
		t.Fatalf("不在册 = %d %s", missing.Code, missing.Body)
	}
	if _, has := absent["draft"]; has {
		t.Fatal("不在册的答复带了 draft")
	}

	// 认证出的是另一个租户：同名载体读不到，与不在册同形。
	foreign := httptest.NewRecorder()
	otherTenant := commercialhttp.NewQueryPublicationDraftEndpoint(intakeDouble{query: query},
		draftReaderDouble{store: store, storedUnder: pcNew(t, domain.NewTenantID, pcTenant), serves: pcNew(t, domain.NewTenantID, "tenant-other")})
	otherTenant.ServeHTTP(foreign, draftReadbackRequest("CREDIT_POLICY", "credit-1", "v1"))
	if !strings.Contains(foreign.Body.String(), `"DRAFT_NOT_FOUND"`) {
		t.Fatalf("跨租户读到了别家的载体：%s", foreign.Body)
	}

	bad := httptest.NewRecorder()
	endpoint.ServeHTTP(bad, draftReadbackRequest("NOT_A_KIND", "credit-1", "v1"))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("集外类别 = %d, want 400", bad.Code)
	}
}

// Covers: AT-PC-069——批准口与发布口的载荷必带期望摘要：缺席（或只有空白）逐格问题点名 expectedDigest；带了的原样进命令。
func TestApproveAndPublishPayloadsRequireTheExpectedDigest(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	for _, raw := range []string{
		`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1"}`,
		`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1","expectedDigest":"   "}`,
	} {
		reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		if _, err := reference.ApproveCommand(tenant, approver); !errors.Is(err, commercialhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "expectedDigest") {
			t.Fatalf("批准载荷 %s：err = %v, want 点名 expectedDigest 的畸形请求", raw, err)
		}
		if _, err := reference.PublishCommand(tenant); !errors.Is(err, commercialhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "expectedDigest") {
			t.Fatalf("发布载荷 %s：err = %v", raw, err)
		}
	}

	reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(
		`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1","expectedDigest":"PCC-1:seen"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	approve, err := reference.ApproveCommand(tenant, approver)
	if err != nil || approve.ExpectedDigest != "PCC-1:seen" {
		t.Fatalf("approve command = %+v, err = %v", approve, err)
	}
	publish, err := reference.PublishCommand(tenant)
	if err != nil || publish.ExpectedDigest != "PCC-1:seen" {
		t.Fatalf("publish command = %+v, err = %v", publish, err)
	}
}

// Covers: AT-PC-069——批准与发布的已变更答 200 加 DRAFT_CHANGED（批准口沿用既有格，发布口新增同名格）：对着旧摘要的命令经真用例答出来。
func TestStaleDigestAnswersDraftChangedOnBothCommandEndpoints(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store, _, _ := submittedAndApprovedDrafts(t)
	rule, err := domain.NewApprovalDutyRule(tenant, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	reference := func(digest string) commercialhttp.PublicationDraftReferencePayload {
		payload, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(
			`{"kind":"CREDIT_POLICY","objectId":"credit-1","version":"v1","expectedDigest":"` + digest + `"}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return payload
	}
	approveCommand, err := reference("PCC-1:stale").ApproveCommand(tenant, approver)
	if err != nil {
		t.Fatalf("approve command: %v", err)
	}
	response := serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(time.Hour)})), http.MethodPost)
	if got := decodeDraftAnswer(t, response); response.Code != http.StatusOK || got.Outcome != "DRAFT_CHANGED" || got.Status != "" {
		t.Fatalf("批准口 = %d %s", response.Code, response.Body.Bytes())
	}
	publishCommand, err := reference("PCC-1:stale").PublishCommand(tenant)
	if err != nil {
		t.Fatalf("publish command: %v", err)
	}
	response = serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(2 * time.Hour)})), http.MethodPost)
	if got := decodeDraftAnswer(t, response); response.Code != http.StatusOK || got.Outcome != "DRAFT_CHANGED" {
		t.Fatalf("发布口 = %d %s", response.Code, response.Body.Bytes())
	}
}
