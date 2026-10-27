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

// 本文件证本构建折不回正文的载体行在四个口上的传输面（AT-PC-071）：列表照样列出它并标出读不回，单份读回答 200 加
// DRAFT_NOT_READABLE（带库上的事实、没有 content），批准口与发布口答同名一格——都不是 5xx；行本身坏了与读取失败答 5xx。

// unreadableFrom 把一份读得回的载体造成「本构建读不回」的同一行：规范化版本换成本构建不认得的 PCC-9，状态与痕迹相同。
func unreadableFrom(t *testing.T, draft domain.PublicationDraft) *domain.UnreadablePublicationDraft {
	t.Helper()
	approver, _ := draft.Approver()
	approvedAt, _ := draft.ApprovedAt()
	publishedAt, _ := draft.PublishedAt()
	_, err := domain.RehydratePublicationDraft(domain.RehydratePublicationDraftSpec{
		Shell: domain.PublicationDraftShell{
			TenantID: draft.Tenant(), Kind: draft.Kind(), ObjectID: draft.ObjectID(), Version: draft.Version(),
			Scope: draft.Scope(), Effective: draft.Effective(), References: referenceMapOf(draft),
		},
		Canonicalization: "PCC-9",
		Document:         draft.Canonical().Document(),
		Digest:           pcNew(t, domain.NewCommercialContentDigest, "PCC-9:future-build"),
		Submitter:        draft.Submitter(),
		SubmittedAt:      draft.SubmittedAt(),
		Status:           draft.Status(),
		Approver:         approver,
		ApprovedAt:       approvedAt,
		PublishedAt:      publishedAt,
	})
	var unreadable *domain.UnreadablePublicationDraft
	if !errors.As(err, &unreadable) {
		t.Fatalf("夹具应读不回：err = %v", err)
	}
	return unreadable
}

// assertCauseIsTheDomainCause 钉成因的文字来源：答复里的成因正是领域折不回的成因，不是整个错误值的文字——后者在成因前拼着
// 哨兵那句英文，管理台会原样显示成「原因：party commercial: publication draft cannot be read back…」。
func assertCauseIsTheDomainCause(t *testing.T, where, got string, unreadable *domain.UnreadablePublicationDraft) {
	t.Helper()
	if got == "" || got != unreadable.Cause().Error() || strings.Contains(got, "cannot be read back by this build") {
		t.Fatalf("%s的成因 = %q, want the domain cause %q", where, got, unreadable.Cause().Error())
	}
	if !strings.Contains(got, "PCC-9") {
		t.Fatalf("%s的成因没说清是哪个规范化版本读不回：%q", where, got)
	}
}

func referenceMapOf(draft domain.PublicationDraft) map[domain.CommercialObjectKind]domain.CommercialObjectID {
	references := map[domain.CommercialObjectKind]domain.CommercialObjectID{}
	for _, reference := range draft.DeclaredReferences() {
		references[reference.Kind()] = reference.ObjectID()
	}
	if len(references) == 0 {
		return nil
	}
	return references
}

// Covers: AT-PC-071——列表里一行读不回的载体照样在场：readable 为假，带成因，字段取自库上的列（列上的规范化版本与摘要原值、壳摘要、状态、
// 录入者），不带 content；读得回的行 readable 为真，顺序与整页 200 不受影响。
func TestAnUnreadableDraftIsListedBesideTheReadableOnes(t *testing.T) {
	_, pending, approved := submittedAndApprovedDrafts(t)
	unreadable := unreadableFrom(t, approved)
	endpoint := commercialhttp.NewQueryPublicationDraftsEndpoint(intakeDouble{query: catalogueQuery(t)}, draftReaderDouble{
		open: []ports.OpenPublicationDraft{ports.UnreadableOpenDraft(unreadable), ports.ReadableOpenDraft(pending)},
	})

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-open-publication-drafts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Drafts  []map[string]json.RawMessage `json:"drafts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Outcome != "PUBLICATION_DRAFTS_LISTED" || len(body.Drafts) != 2 {
		t.Fatalf("body = %s (%v)", recorder.Body, err)
	}
	str := func(draft map[string]json.RawMessage, key string) string {
		var value string
		_ = json.Unmarshal(draft[key], &value)
		return value
	}
	first, second := body.Drafts[0], body.Drafts[1]
	if string(first["readable"]) != "false" || str(first, "objectId") != "credit-2" || str(first, "status") != "APPROVED" ||
		str(first, "canonicalization") != "PCC-9" || str(first, "contentDigest") != "PCC-9:future-build" ||
		str(first, "shellDigest") != approved.ShellDigest() || str(first, "submitter") != "op-submitter" || str(first, "approver") != "op-approver" {
		t.Fatalf("读不回的一行 = %v", first)
	}
	assertCauseIsTheDomainCause(t, "列表", str(first, "unreadableCause"), unreadable)
	if _, has := first["content"]; has {
		t.Fatal("读不回的一行带了 content")
	}
	if string(second["readable"]) != "true" || str(second, "contentDigest") != pending.Canonical().Digest().String() {
		t.Fatalf("读得回的一行 = %v", second)
	}
	if _, has := second["unreadableCause"]; has {
		t.Fatal("读得回的一行带了成因")
	}
}

// Covers: AT-PC-071——单份读回遇到读不回的载体：200 加 DRAFT_NOT_READABLE，draft 带库上的事实与成因、没有 content；
// 行本身坏了与读取失败答 5xx（列表同），不被收进这一格。
func TestAnUnreadableDraftReadsBackAsNotReadableAndABrokenOneStillAnswers5xx(t *testing.T) {
	_, _, approved := submittedAndApprovedDrafts(t)
	unreadable := unreadableFrom(t, approved)
	query := catalogueQuery(t)
	read := func(reader draftReaderDouble) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		commercialhttp.NewQueryPublicationDraftEndpoint(intakeDouble{query: query}, reader).ServeHTTP(recorder, draftReadbackRequest("CREDIT_POLICY", "credit-2", "v1"))
		return recorder
	}

	recorder := read(draftReaderDouble{err: unreadable, serves: query.Scope.Tenant()})
	var body struct {
		Outcome string                     `json:"outcome"`
		Draft   map[string]json.RawMessage `json:"draft"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || body.Outcome != "DRAFT_NOT_READABLE" {
		t.Fatalf("读不回 = %d %s (%v)", recorder.Code, recorder.Body, err)
	}
	if string(body.Draft["readable"]) != "false" || string(body.Draft["status"]) != `"APPROVED"` {
		t.Fatalf("draft = %v", body.Draft)
	}
	var cause string
	_ = json.Unmarshal(body.Draft["unreadableCause"], &cause)
	assertCauseIsTheDomainCause(t, "单份读回", cause, unreadable)
	if _, has := body.Draft["content"]; has {
		t.Fatal("读不回的载体带了 content：折不回的文档不能冒充摘要盖住的内容")
	}

	broken := errors.Join(errors.New("load publication draft"), domain.ErrInvalidRehydratedPublicationDraft)
	for name, failure := range map[string]error{"行坏了": broken, "读取失败": errors.New("boom")} {
		if got := read(draftReaderDouble{err: failure, serves: query.Scope.Tenant()}); got.Code != http.StatusInternalServerError {
			t.Fatalf("单份读回 %s = %d, want 500", name, got.Code)
		}
		listing := httptest.NewRecorder()
		commercialhttp.NewQueryPublicationDraftsEndpoint(intakeDouble{query: query}, draftReaderDouble{err: failure}).
			ServeHTTP(listing, httptest.NewRequest(http.MethodGet, "/", nil))
		if listing.Code != http.StatusInternalServerError {
			t.Fatalf("列表 %s = %d, want 500", name, listing.Code)
		}
	}
}

// failingLoadStore 让载体册的点读交出指定的错误，其余照内存册——批准与发布用例由此撞上读不回的行。
type failingLoadStore struct {
	*draftStore
	loadErr error
}

func (store failingLoadStore) LoadDraft(context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) (domain.PublicationDraft, bool, error) {
	return domain.PublicationDraft{}, false, store.loadErr
}

func operatorOver(store failingLoadStore, rules dutyRuleStore, clock pcClock) draftOperator {
	publisher := application.NewPublishCommercialAuthorityHandler(creditAwareRegistry{}, clock, pcHandoffDouble{})
	return draftOperator{
		submit:  application.NewSubmitPublicationDraftHandler(store, clock),
		approve: application.NewApprovePublicationDraftHandler(store, rules, clock),
		publish: application.NewPublishPublicationDraftHandler(store, publisher, clock),
	}
}

// Covers: AT-PC-071——批准口与发布口遇到读不回的载体：200 加 DRAFT_NOT_READABLE，带载体状态与成因、不带任何摘要（读不回的载体没有可批准的
// 内容，摘要不能被原样交回去批），发布口也没有发布用例的答案；已发布的读不回行答 DRAFT_ALREADY_PUBLISHED；读取失败答 5xx。
func TestCommandEndpointsAnswerAnUnreadableDraftAsABusinessGradeNot5xx(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	_, pending, approved := submittedAndApprovedDrafts(t)
	rule, err := domain.NewApprovalDutyRule(tenant, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	published, err := approved.MarkPublished(pcNow.Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}
	approveCommand := application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: domain.CreditPolicyObject, ObjectID: pcNew(t, domain.NewCommercialObjectID, "credit-1"),
		Version: pcNew(t, domain.NewCommercialVersionLabel, "v1"), Approver: approver,
	}
	publishCommand := application.PublishPublicationDraftCommand{
		Tenant: tenant, Kind: approveCommand.Kind, ObjectID: approveCommand.ObjectID, Version: approveCommand.Version,
	}
	clock := pcClock{at: pcNow.Add(3 * time.Hour)}

	for _, testCase := range []struct {
		draft  domain.PublicationDraft
		status string
		want   string
	}{
		{pending, "PENDING_APPROVAL", "DRAFT_NOT_READABLE"},
		{approved, "APPROVED", "DRAFT_NOT_READABLE"},
		{published, "PUBLISHED", "DRAFT_ALREADY_PUBLISHED"},
	} {
		store := failingLoadStore{draftStore: newDraftStore(), loadErr: unreadableFrom(t, testCase.draft)}
		operator := operatorOver(store, rules, clock)

		response := serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, operator), http.MethodPost)
		got := decodeDraftAnswer(t, response)
		if response.Code != http.StatusOK || got.Outcome != testCase.want {
			t.Fatalf("%s 批准口 = %d %s, want 200 %s", testCase.status, response.Code, response.Body.Bytes(), testCase.want)
		}
		if testCase.want == "DRAFT_NOT_READABLE" {
			if got.Status != testCase.status || got.ContentDigest != "" || got.ShellDigest != "" {
				t.Fatalf("%s 批准口的读不回一格 = %s", testCase.status, response.Body.Bytes())
			}
			assertCauseIsTheDomainCause(t, testCase.status+" 批准口", got.Cause, unreadableFrom(t, testCase.draft))
		}

		response = serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, operator), http.MethodPost)
		got = decodeDraftAnswer(t, response)
		if response.Code != http.StatusOK || got.Outcome != testCase.want || got.Publication != nil {
			t.Fatalf("%s 发布口 = %d %s, want 200 %s 且无发布答案", testCase.status, response.Code, response.Body.Bytes(), testCase.want)
		}
		if testCase.want == "DRAFT_NOT_READABLE" {
			if got.Status != testCase.status {
				t.Fatalf("%s 发布口的读不回一格 = %s", testCase.status, response.Body.Bytes())
			}
			assertCauseIsTheDomainCause(t, testCase.status+" 发布口", got.Cause, unreadableFrom(t, testCase.draft))
		}
	}

	failing := operatorOver(failingLoadStore{draftStore: newDraftStore(), loadErr: errors.New("connection reset")}, rules, clock)
	if response := serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, failing), http.MethodPost); response.Code < 500 {
		t.Fatalf("读取失败的批准口 = %d, want 5xx", response.Code)
	}
	if response := serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, failing), http.MethodPost); response.Code < 500 {
		t.Fatalf("读取失败的发布口 = %d, want 5xx", response.Code)
	}
}
