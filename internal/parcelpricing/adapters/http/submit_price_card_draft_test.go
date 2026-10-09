package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

const priceCardDraftTarget = "/pricing-price-card-drafts"

type priceCardDraftSubmissionIntakeDouble struct{ err error }

func (double priceCardDraftSubmissionIntakeDouble) IntakePriceCardDraftSubmission(context.Context, *http.Request) (application.SubmitPriceCardDraftCommand, error) {
	return application.SubmitPriceCardDraftCommand{}, double.err
}

type priceCardDraftSubmitterDouble struct {
	result application.SubmitPriceCardDraftResult
	err    error
}

func (double priceCardDraftSubmitterDouble) Handle(context.Context, application.SubmitPriceCardDraftCommand) (application.SubmitPriceCardDraftResult, error) {
	return double.result, double.err
}

func postPriceCardDraft(intake pricinghttp.PriceCardDraftSubmissionIntake, submitter pricinghttp.PriceCardDraftSubmitter) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	pricinghttp.NewSubmitPriceCardDraftEndpoint(intake, submitter).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, priceCardDraftTarget, strings.NewReader("")))
	return recorder
}

var draftSubmittedAt = time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)

func draftSourceFile(t *testing.T) domain.SourceFileIdentity {
	t.Helper()
	source, err := domain.NewSourceFileIdentity("card.xlsx", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// validatedDraft 是一份读通了的录入：读法与草稿出自同一张估价方案。
func validatedDraft(t *testing.T) (application.PriceCardImportPreview, domain.PriceCardDraft) {
	t.Helper()
	plan := estimatePlan(t)
	authorization := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH", "v1")
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: pptest.Value(t, domain.NewTenantID, "SYN-TENANT-01"), Plan: plan.Reference(), Source: draftSourceFile(t),
		Content:   &domain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: authorization},
		Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", SubmittedAt: draftSubmittedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.PriceCardImportPreview{
		Outcome: application.PriceCardImportValidated, TemplateVersion: "PPT-1", SourceFile: draftSourceFile(t), Plan: plan.Reference(),
		Content: &ports.PriceCardTemplateContent{Plan: plan, DirectionAuthorization: authorization},
	}, draft
}

// problemDraft 是一份读出了方案身份、没过构造门的录入：只记逐格问题。
func problemDraft(t *testing.T) (application.PriceCardImportPreview, domain.PriceCardDraft) {
	t.Helper()
	plan := pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-EST", "v1")
	problem, err := domain.NewPriceCardDraftProblem("tables", 2, "currency", "CELL_INVALID", "币种立不住")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: pptest.Value(t, domain.NewTenantID, "SYN-TENANT-01"), Plan: plan, Source: draftSourceFile(t),
		Problems: []domain.PriceCardDraftProblem{problem}, Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", SubmittedAt: draftSubmittedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.PriceCardImportPreview{
		Outcome: application.PriceCardImportHasProblems, TemplateVersion: "PPT-1", SourceFile: draftSourceFile(t), Plan: plan,
		Problems: []ports.PriceCardTemplateProblem{{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "币种立不住"}},
	}, draft
}

type draftProblemBody struct {
	Sheet, Column, Code, Message string
	Row                          int
}

type draftRowBody struct {
	Plan        map[string]string `json:"plan"`
	Status      string            `json:"status"`
	SourceFile  map[string]string `json:"sourceFile"`
	Submitter   string            `json:"submitter"`
	SubmittedAt string            `json:"submittedAt"`
	Approver    *string           `json:"approver"`
	Content     *struct {
		ContentDigest          string            `json:"contentDigest"`
		DirectionAuthorization map[string]string `json:"directionAuthorization"`
	} `json:"content"`
	Problems []draftProblemBody `json:"problems"`
}

type draftSubmissionBody struct {
	Outcome string `json:"outcome"`
	Reading struct {
		Outcome         string            `json:"outcome"`
		TemplateVersion string            `json:"templateVersion"`
		SourceFile      map[string]string `json:"sourceFile"`
		Plan            map[string]string `json:"plan"`
		Content         *struct {
			ContentDigest string `json:"contentDigest"`
		} `json:"content"`
		Problems []draftProblemBody `json:"problems"`
	} `json:"reading"`
	Draft *draftRowBody `json:"draft"`
}

func decodeDraftSubmission(t *testing.T, recorder *httptest.ResponseRecorder) draftSubmissionBody {
	t.Helper()
	var body draftSubmissionBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}
	return body
}

// 首次录入一份读通了的文件：这一次写下了册上的一行，取 201；读法与预览口逐字段同形，另带写下的那一行。
func TestAFirstSubmissionAnswersCreatedWithTheReadingAndTheRowItWrote(t *testing.T) {
	reading, draft := validatedDraft(t)
	recorder := postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{result: application.SubmitPriceCardDraftResult{
		Outcome: application.PriceCardDraftSubmitted, Reading: reading, Draft: draft, HasDraft: true,
	}})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
	body := decodeDraftSubmission(t, recorder)
	digest := estimatePlan(t).ContentDigest()
	if body.Outcome != "DRAFT_SUBMITTED" || body.Reading.Outcome != "VALIDATED" || body.Reading.TemplateVersion != "PPT-1" ||
		body.Reading.Plan["id"] != "SYN-PLAN-EST" || body.Reading.SourceFile["name"] != "card.xlsx" ||
		body.Reading.Content == nil || body.Reading.Content.ContentDigest != digest || body.Reading.Problems == nil || len(body.Reading.Problems) != 0 {
		t.Fatalf("body = %s", recorder.Body)
	}
	row := body.Draft
	if row == nil || row.Status != "VALIDATED" || row.Plan["id"] != "SYN-PLAN-EST" || row.Plan["version"] != "v1" ||
		row.SourceFile["name"] != "card.xlsx" || row.SourceFile["sha256"] != strings.Repeat("a", 64) ||
		row.Submitter != "https://id.syn.example/dex#SYN-OPERATOR-01" || row.SubmittedAt != "2026-10-09T03:00:00Z" || row.Approver != nil ||
		row.Content == nil || row.Content.ContentDigest != digest || row.Content.DirectionAuthorization["id"] != "SYN-AUTH" ||
		row.Problems == nil || len(row.Problems) != 0 {
		t.Fatalf("draft = %s", recorder.Body)
	}
}

// 只有写下了册上那一行的两格带它并取 201。重放可以落在已批准甚至已发布的行上，内容已固定那一格的行也已批准，
// 两格里应用层交回的是这一次拟录的那份（ADR-0126 决定三的落点由册上那一行判）——把它当成册上的行交出去，
// 就会把一行已批准的草稿答成`已校验`。这三格取 200，只带读法；册上那一行由查阅读口答。
func TestOnlyTheOutcomesThatWroteTheRowCarryIt(t *testing.T) {
	validatedReading, validated := validatedDraft(t)
	problemReading, withProblems := problemDraft(t)
	for outcome, testCase := range map[application.SubmitPriceCardDraftOutcome]struct {
		reading   application.PriceCardImportPreview
		draft     domain.PriceCardDraft
		status    int
		rowStatus string
	}{
		application.PriceCardDraftSubmitted:    {validatedReading, validated, http.StatusCreated, "VALIDATED"},
		application.PriceCardDraftRevised:      {problemReading, withProblems, http.StatusCreated, "DRAFT"},
		application.PriceCardDraftReplayed:     {validatedReading, validated, http.StatusOK, ""},
		application.PriceCardDraftContentFixed: {validatedReading, validated, http.StatusOK, ""},
	} {
		recorder := postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{result: application.SubmitPriceCardDraftResult{
			Outcome: outcome, Reading: testCase.reading, Draft: testCase.draft, HasDraft: true,
		}})
		body := decodeDraftSubmission(t, recorder)
		if recorder.Code != testCase.status || body.Outcome != outcome.String() || body.Reading.Outcome != testCase.reading.Outcome.String() ||
			body.Reading.SourceFile["sha256"] != strings.Repeat("a", 64) || body.Reading.Plan["id"] != "SYN-PLAN-EST" {
			t.Errorf("%s: status = %d body = %s", outcome, recorder.Code, recorder.Body)
			continue
		}
		switch {
		case testCase.rowStatus == "" && body.Draft != nil:
			t.Errorf("%s: 没写行却带了行：%s", outcome, recorder.Body)
		case testCase.rowStatus != "" && (body.Draft == nil || body.Draft.Status != testCase.rowStatus):
			t.Errorf("%s: 写下的行不对：%s", outcome, recorder.Body)
		}
	}

	recorder := postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{result: application.SubmitPriceCardDraftResult{
		Outcome: application.PriceCardDraftRevised, Reading: problemReading, Draft: withProblems, HasDraft: true,
	}})
	body := decodeDraftSubmission(t, recorder)
	if body.Reading.Content != nil || len(body.Reading.Problems) != 1 || body.Reading.Problems[0].Code != "CELL_INVALID" ||
		body.Draft == nil || body.Draft.Content != nil || len(body.Draft.Problems) != 1 || body.Draft.Problems[0] != (draftProblemBody{
		Sheet: "tables", Row: 2, Column: "currency", Code: "CELL_INVALID", Message: "币种立不住",
	}) {
		t.Fatalf("草稿：body = %s", recorder.Body)
	}
}

// 未受理没有行可落：答 200，读法里带读口交出的那一条问题，连方案身份也没有。
func TestANotAcceptedSubmissionAnswersTheReadingWithoutARow(t *testing.T) {
	recorder := postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{result: application.SubmitPriceCardDraftResult{
		Outcome: application.PriceCardDraftNotAccepted,
		Reading: application.PriceCardImportPreview{
			Outcome: application.PriceCardImportNotAccepted, TemplateVersion: "PPT-1", SourceFile: draftSourceFile(t),
			Problems: []ports.PriceCardTemplateProblem{{Code: ports.TemplatePlanIdentityUnreadable, Message: "planId 与 planVersion 读不出来"}},
		},
	}})
	body := decodeDraftSubmission(t, recorder)
	if recorder.Code != http.StatusOK || body.Outcome != "NOT_ACCEPTED" || body.Reading.Outcome != "NOT_ACCEPTED" || body.Reading.Plan != nil ||
		body.Draft != nil || len(body.Reading.Problems) != 1 || body.Reading.Problems[0].Code != "PLAN_IDENTITY_UNREADABLE" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
}

func TestPriceCardDraftSubmissionTransportAnswers(t *testing.T) {
	recorder := postPriceCardDraft(pricinghttp.UnconfiguredIntake{}, priceCardDraftSubmitterDouble{})
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{err: errors.New("boom")})
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("handler error: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{})
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "UNNAMED_OUTCOME" {
		t.Fatalf("unnamed outcome: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	pricinghttp.NewSubmitPriceCardDraftEndpoint(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, priceCardDraftTarget, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET: status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

// 操作者渠道：租户与录入者都取认证出的身份，上传与预览同一段严格解码；没令牌答凭据被拒，表单里夹带别的格答请求形状不对。
func TestOperatorIntakeTakesTheTenantAndSubmitterFromTheAuthenticatedOperator(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	upload := func(token string, build func(*multipart.Writer)) *http.Request {
		request := uploadRequest(t, build)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		return request
	}
	card := func(writer *multipart.Writer) {
		part, err := writer.CreateFormFile("file", "card.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("bytes"))
	}
	command, err := intake.IntakePriceCardDraftSubmission(context.Background(), upload("token", card))
	if err != nil || command.Tenant.String() != "SYN-TENANT-01" || command.Submitter != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
		command.FileName != "card.xlsx" || string(command.Raw) != "bytes" {
		t.Fatalf("command = %+v err = %v", command, err)
	}
	if _, err := intake.IntakePriceCardDraftSubmission(context.Background(), upload("", card)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v", err)
	}
	smuggled := func(writer *multipart.Writer) {
		card(writer)
		_ = writer.WriteField("submitter", "someone-else")
	}
	if _, err := intake.IntakePriceCardDraftSubmission(context.Background(), upload("token", smuggled)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("self-reported submitter field: err = %v", err)
	}
	recorder := httptest.NewRecorder()
	pricinghttp.NewSubmitPriceCardDraftEndpoint(intake, priceCardDraftSubmitterDouble{}).ServeHTTP(recorder, upload("", card))
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("endpoint without token: status = %d body = %s", recorder.Code, recorder.Body)
	}
}
