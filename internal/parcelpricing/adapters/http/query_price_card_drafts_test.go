package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

const priceCardDraftViewsTarget = "/pricing-price-card-draft-views"

type priceCardDraftQueryIntakeDouble struct {
	query pricinghttp.PriceCardDraftQuery
	err   error
}

func (double priceCardDraftQueryIntakeDouble) IntakePriceCardDraftQuery(context.Context, *http.Request) (pricinghttp.PriceCardDraftQuery, error) {
	return double.query, double.err
}

type priceCardDraftReaderDouble struct {
	drafts []domain.PriceCardDraft
	err    error
	tenant domain.TenantID
	status domain.PriceCardDraftStatus
	limit  int
}

func (double *priceCardDraftReaderDouble) ListPriceCardDrafts(_ context.Context, tenant domain.TenantID, status domain.PriceCardDraftStatus, limit int) ([]domain.PriceCardDraft, error) {
	double.tenant, double.status, double.limit = tenant, status, limit
	return double.drafts, double.err
}

func getPriceCardDrafts(intake pricinghttp.PriceCardDraftQueryIntake, reader pricinghttp.PriceCardDraftReader) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	pricinghttp.NewQueryPriceCardDraftsEndpoint(intake, reader).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, priceCardDraftViewsTarget, nil))
	return recorder
}

// approvedDraft 经重建门造一份已批准的草稿：批准动作归票 04，读口照样要能把那一格如实透出。
func approvedDraft(t *testing.T, validated domain.PriceCardDraft) domain.PriceCardDraft {
	t.Helper()
	document, err := domain.MarshalPriceCardDraftContent(validated)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := domain.RehydratePriceCardDraft(domain.RehydratePriceCardDraftSpec{
		Tenant: validated.Tenant(), Plan: validated.Plan(), Status: domain.PriceCardDraftStatusApproved, Source: validated.SourceFile(),
		Document: document, Submitter: validated.Submitter(), SubmittedAt: validated.SubmittedAt(),
		Approver: "https://id.syn.example/dex#SYN-APPROVER", ApprovedAt: validated.SubmittedAt().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

// 一格一行照实透出：状态、源文件身份、录入者与批准者并排；已校验起带内容概要，`草稿`带逐格问题；读口拿到的是
// Intake 译出的租户、状态与页大小。
func TestPriceCardDraftViewsListEachDraftAsItStands(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	_, validated := validatedDraft(t)
	_, withProblems := problemDraft(t)
	reader := &priceCardDraftReaderDouble{drafts: []domain.PriceCardDraft{approvedDraft(t, validated), withProblems}}
	recorder := getPriceCardDrafts(priceCardDraftQueryIntakeDouble{query: pricinghttp.PriceCardDraftQuery{
		Tenant: tenant, Status: domain.PriceCardDraftStatusInvalid, Limit: 25,
	}}, reader)

	if recorder.Code != http.StatusOK || reader.tenant != tenant || reader.status != domain.PriceCardDraftStatusInvalid || reader.limit != 25 {
		t.Fatalf("status = %d reader = %+v", recorder.Code, reader)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Drafts  []struct {
			Plan        map[string]string `json:"plan"`
			Status      string            `json:"status"`
			SourceFile  map[string]string `json:"sourceFile"`
			Submitter   string            `json:"submitter"`
			SubmittedAt string            `json:"submittedAt"`
			Approver    string            `json:"approver"`
			ApprovedAt  string            `json:"approvedAt"`
			PublishedAt *string           `json:"publishedAt"`
			Content     *struct {
				ContentDigest string `json:"contentDigest"`
			} `json:"content"`
			Problems []struct {
				Sheet, Column, Code, Message string
				Row                          int
			} `json:"problems"`
		} `json:"drafts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "PRICE_CARD_DRAFTS_LISTED" || len(body.Drafts) != 2 {
		t.Fatalf("body = %s", recorder.Body)
	}
	approved, draft := body.Drafts[0], body.Drafts[1]
	if approved.Status != "APPROVED" || approved.Plan["id"] != "SYN-PLAN-EST" || approved.SourceFile["name"] != "card.xlsx" ||
		approved.Submitter != "https://id.syn.example/dex#SYN-OPERATOR-01" || approved.SubmittedAt != "2026-10-09T03:00:00Z" ||
		approved.Approver != "https://id.syn.example/dex#SYN-APPROVER" || approved.ApprovedAt != "2026-10-09T04:00:00Z" ||
		approved.PublishedAt != nil || approved.Content == nil || approved.Content.ContentDigest != estimatePlan(t).ContentDigest() ||
		approved.Problems == nil || len(approved.Problems) != 0 {
		t.Fatalf("approved = %+v", approved)
	}
	if draft.Status != "DRAFT" || draft.Content != nil || draft.Approver != "" || len(draft.Problems) != 1 ||
		draft.Problems[0].Sheet != "tables" || draft.Problems[0].Row != 2 || draft.Problems[0].Code != "CELL_INVALID" || draft.Problems[0].Message != "币种立不住" {
		t.Fatalf("draft = %+v", draft)
	}
}

// 空册是内容不是未配置：答空数组。
func TestPriceCardDraftViewsAnswerAnEmptyRegisterAsAnEmptyList(t *testing.T) {
	recorder := getPriceCardDrafts(priceCardDraftQueryIntakeDouble{}, &priceCardDraftReaderDouble{})
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || string(body["drafts"]) != "[]" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
}

func TestPriceCardDraftViewsTransportAnswers(t *testing.T) {
	recorder := getPriceCardDrafts(pricinghttp.UnconfiguredIntake{}, &priceCardDraftReaderDouble{})
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = getPriceCardDrafts(priceCardDraftQueryIntakeDouble{}, &priceCardDraftReaderDouble{err: errors.New("unreachable")})
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("reader error: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = getPriceCardDrafts(priceCardDraftQueryIntakeDouble{err: pricinghttp.ErrMalformedRequest}, &priceCardDraftReaderDouble{})
	if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	pricinghttp.NewQueryPriceCardDraftsEndpoint(priceCardDraftQueryIntakeDouble{}, &priceCardDraftReaderDouble{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, priceCardDraftViewsTarget, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST: status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

// 查询串只认 status 一格、至多一个值、取四格之一；缺即四格都列。别的键一律不收：租户不从请求里来，页大小由接入面定。
func TestDecodePriceCardDraftQueryIsStrict(t *testing.T) {
	for target, want := range map[string]domain.PriceCardDraftStatus{
		priceCardDraftViewsTarget:                      domain.PriceCardDraftStatusInvalid,
		priceCardDraftViewsTarget + "?status=DRAFT":    domain.PriceCardDraftStatusDraft,
		priceCardDraftViewsTarget + "?status=APPROVED": domain.PriceCardDraftStatusApproved,
	} {
		status, err := pricinghttp.DecodePriceCardDraftQuery(httptest.NewRequest(http.MethodGet, target, nil))
		if err != nil || status != want {
			t.Errorf("%s: status = %v err = %v", target, status, err)
		}
	}
	for _, target := range []string{
		priceCardDraftViewsTarget + "?status=draft",
		priceCardDraftViewsTarget + "?status=",
		priceCardDraftViewsTarget + "?status=DRAFT&status=VALIDATED",
		priceCardDraftViewsTarget + "?tenant=SYN-TENANT-02",
		priceCardDraftViewsTarget + "?status=DRAFT&limit=1000",
	} {
		if _, err := pricinghttp.DecodePriceCardDraftQuery(httptest.NewRequest(http.MethodGet, target, nil)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Errorf("%s: err = %v, want ErrMalformedRequest", target, err)
		}
	}
}

// 操作者渠道：租户取认证出的身份，状态取查询串，页大小由接入面定；没令牌答凭据被拒，坏查询串答请求形状不对。
func TestOperatorIntakeTranslatesTheDraftQuery(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, target string) *http.Request {
		built := httptest.NewRequest(http.MethodGet, target, nil)
		if token != "" {
			built.Header.Set("Authorization", "Bearer "+token)
		}
		return built
	}
	query, err := intake.IntakePriceCardDraftQuery(context.Background(), request("token", priceCardDraftViewsTarget+"?status=VALIDATED"))
	if err != nil || query.Tenant.String() != "SYN-TENANT-01" || query.Status != domain.PriceCardDraftStatusValidated || query.Limit < 1 {
		t.Fatalf("query = %+v err = %v", query, err)
	}
	if _, err := intake.IntakePriceCardDraftQuery(context.Background(), request("", priceCardDraftViewsTarget)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v", err)
	}
	if _, err := intake.IntakePriceCardDraftQuery(context.Background(), request("token", priceCardDraftViewsTarget+"?tenant=SYN-TENANT-02")); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("self-reported tenant: err = %v", err)
	}
	recorder := httptest.NewRecorder()
	pricinghttp.NewQueryPriceCardDraftsEndpoint(intake, &priceCardDraftReaderDouble{}).ServeHTTP(recorder, request("", priceCardDraftViewsTarget))
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("endpoint without token: status = %d body = %s", recorder.Code, recorder.Body)
	}
}
