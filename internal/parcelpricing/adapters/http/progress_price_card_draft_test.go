package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证价卡草稿批准口与发布口（ADR-0101 决定五；票 price-card-import/04）：操作者渠道把信封译成命令——租户、批准者
// 主体与授予集都取认证结果，载荷只收草稿引用、夹带身份格即拒；两口逐格交出编排的答复，登记的那一格原名照交。

const draftReference = `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1"}`

// grantedAuthenticator 交回带授予集的身份，同真身份层译出的形状。
type grantedAuthenticator struct{ grants []string }

func (fake grantedAuthenticator) AuthenticateRegistryWrite(_ context.Context, token string) (pricinghttp.OperatorIdentity, error) {
	if token == "" {
		return pricinghttp.OperatorIdentity{}, pricinghttp.ErrOperatorCredentialRejected
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return pricinghttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example/dex#SYN-OPERATOR-02", Grants: fake.grants}, err
}

func progressRequest(path, token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: 票 04 第 2、5 条——批准者主体由 Intake 把信封译成：引用取认证出的操作者，授予集取信封里持有的授予名；租户同取
// 认证结果；载荷只收草稿引用。
func TestTheApprovalIntakeTakesTheApproverFromTheEnvelope(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(grantedAuthenticator{grants: []string{"REGISTRY_CONFIGURATION_WRITE", "SYN-GRANT-PRICING-SUPERVISOR"}})
	if err != nil {
		t.Fatal(err)
	}
	command, err := intake.IntakePriceCardDraftApproval(context.Background(), progressRequest("/pricing-price-card-draft-approvals", "t", draftReference))
	if err != nil {
		t.Fatalf("approval intake: %v", err)
	}
	supervisor, _ := domain.NewOperatorGrant("SYN-GRANT-PRICING-SUPERVISOR")
	if command.Tenant.String() != "SYN-TENANT-01" || command.Approver.Reference() != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
		!command.Approver.Holds(supervisor) || len(command.Approver.Grants()) != 2 ||
		command.Plan.Kind() != domain.ArtifactPricingPlan || command.Plan.ID() != "SYN-PLAN-DRAFT-01" || command.Plan.Version() != "v1" {
		t.Fatalf("command = %+v", command)
	}

	publication, err := intake.IntakePriceCardDraftPublication(context.Background(), progressRequest("/pricing-price-card-draft-publications", "t", draftReference))
	if err != nil || publication.Tenant.String() != "SYN-TENANT-01" || publication.Plan.ID() != "SYN-PLAN-DRAFT-01" {
		t.Fatalf("publication command = %+v, err = %v", publication, err)
	}
}

// 载荷夹带身份格（批准者、租户）、缺草稿引用、多一段尾随内容都答坏报文；令牌不过答凭证被拒，不读载荷。
func TestTheDraftReferencePayloadIsClosed(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(grantedAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"approver smuggled": `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "approver": "someone-else"}`,
		"tenant smuggled":   `{"planId": "SYN-PLAN-DRAFT-01", "planVersion": "v1", "tenant": "SYN-TENANT-02"}`,
		"version missing":   `{"planId": "SYN-PLAN-DRAFT-01"}`,
		"trailing content":  draftReference + `{}`,
	} {
		if _, err := intake.IntakePriceCardDraftApproval(context.Background(), progressRequest("/x", "t", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("approval %s: err = %v, want ErrMalformedRequest", name, err)
		}
		if _, err := intake.IntakePriceCardDraftPublication(context.Background(), progressRequest("/x", "t", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("publication %s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakePriceCardDraftApproval(context.Background(), progressRequest("/x", "", draftReference)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v", err)
	}
}

type approvalIntakeDouble struct{}

func (approvalIntakeDouble) IntakePriceCardDraftApproval(context.Context, *http.Request) (application.ApprovePriceCardDraftCommand, error) {
	return application.ApprovePriceCardDraftCommand{}, nil
}

type approverDouble struct {
	result application.ApprovePriceCardDraftResult
	err    error
}

func (double approverDouble) Handle(context.Context, application.ApprovePriceCardDraftCommand) (application.ApprovePriceCardDraftResult, error) {
	return double.result, double.err
}

type publicationIntakeDouble struct{}

func (publicationIntakeDouble) IntakePriceCardDraftPublication(context.Context, *http.Request) (application.PublishPriceCardDraftCommand, error) {
	return application.PublishPriceCardDraftCommand{}, nil
}

type publisherDouble struct {
	result application.PublishPriceCardDraftResult
	err    error
}

func (double publisherDouble) Handle(context.Context, application.PublishPriceCardDraftCommand) (application.PublishPriceCardDraftResult, error) {
	return double.result, double.err
}

type progressAnswer struct {
	Outcome      string `json:"outcome"`
	Registration string `json:"registration"`
	Draft        *struct {
		Status   string `json:"status"`
		Approver string `json:"approver"`
	} `json:"draft"`
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func serve(t *testing.T, handler http.Handler, request *http.Request) (int, progressAnswer) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var answer progressAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%q", recorder.Body.String())
	}
	return recorder.Code, answer
}

func approvedDraftForAnswer(t *testing.T) domain.PriceCardDraft {
	t.Helper()
	plan := estimatePlan(t)
	source, _ := domain.NewSourceFileIdentity("SYN-CARD-DRAFT-01.xlsx", "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	direction, _ := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01", "v1")
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	at := time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC)
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: tenant, Plan: plan.Reference(), Source: source,
		Content:   &domain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: direction},
		Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", SubmittedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	approver, _ := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	rule, _ := domain.NewPriceCardApprovalDutyRule(tenant, true, domain.OperatorGrant{})
	approved, err := draft.Approve(approver, rule, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

// Covers: 批准口每一格都是答案、取 200，只有`已批准`带回推进后的草稿；没形成答案答 500；渠道未配置答 403。
func TestTheApprovalEndpointAnswersEveryCell(t *testing.T) {
	approved := approvedDraftForAnswer(t)
	status, answer := serve(t, pricinghttp.NewApprovePriceCardDraftEndpoint(approvalIntakeDouble{}, approverDouble{
		result: application.ApprovePriceCardDraftResult{Outcome: application.PriceCardDraftApproved, Draft: approved, HasDraft: true},
	}), progressRequest("/pricing-price-card-draft-approvals", "t", draftReference))
	if status != http.StatusOK || answer.Outcome != "DRAFT_APPROVED" || answer.Draft == nil ||
		answer.Draft.Status != "APPROVED" || answer.Draft.Approver != "https://id.syn.example/dex#SYN-OPERATOR-02" {
		t.Fatalf("approved: %d %+v", status, answer)
	}
	for _, outcome := range []application.ApprovePriceCardDraftOutcome{
		application.PriceCardApprovalNotConfigured, application.PriceCardApprovalNeedsAnotherApprover,
		application.PriceCardApprovalApproverNotQualified, application.PriceCardApprovalDraftNotValidated,
		application.PriceCardApprovalDraftNotFound, application.PriceCardApprovalDraftChanged,
	} {
		status, answer := serve(t, pricinghttp.NewApprovePriceCardDraftEndpoint(approvalIntakeDouble{}, approverDouble{
			result: application.ApprovePriceCardDraftResult{Outcome: outcome},
		}), progressRequest("/x", "t", draftReference))
		if status != http.StatusOK || answer.Outcome != outcome.String() || answer.Draft != nil {
			t.Fatalf("%s: %d %+v", outcome, status, answer)
		}
	}
	if status, answer := serve(t, pricinghttp.NewApprovePriceCardDraftEndpoint(approvalIntakeDouble{}, approverDouble{err: errors.New("down")}),
		progressRequest("/x", "t", draftReference)); status != http.StatusInternalServerError || answer.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("no answer formed: %d %+v", status, answer)
	}
	if status, answer := serve(t, pricinghttp.NewApprovePriceCardDraftEndpoint(pricinghttp.UnconfiguredIntake{}, approverDouble{}),
		progressRequest("/x", "t", draftReference)); status != http.StatusForbidden || answer.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured: %d %+v", status, answer)
	}
}

// Covers: ADR-0101 决定五「答案代数一格不改」在线上那一面——发布口把登记的那一格原名交出：新落一版取 201，重放取 200，
// 没落定取 200 带登记那一格、不带草稿；登记与否未知答没形成答案；渠道未配置答 403。
func TestThePublicationEndpointHandsTheRegistrationAnswerThrough(t *testing.T) {
	published, err := approvedDraftForAnswer(t).MarkPublished(time.Date(2026, 10, 10, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		result       application.PublishPriceCardDraftResult
		status       int
		registration string
		withDraft    bool
	}{
		{application.PublishPriceCardDraftResult{Outcome: application.PriceCardDraftPublished, Registration: application.PriceCardRecorded,
			HasRegistration: true, Draft: published, HasDraft: true}, http.StatusCreated, "RECORDED", true},
		{application.PublishPriceCardDraftResult{Outcome: application.PriceCardDraftPublished, Registration: application.PriceCardAlreadyOnRegister,
			HasRegistration: true, Draft: published, HasDraft: true}, http.StatusOK, "ALREADY_REGISTERED", true},
		{application.PublishPriceCardDraftResult{Outcome: application.PriceCardPublicationNotLanded, Registration: application.PriceCardRegistrationConflict,
			HasRegistration: true}, http.StatusOK, "CONTENT_CONFLICT", false},
		{application.PublishPriceCardDraftResult{Outcome: application.PriceCardPublicationNotLanded, Registration: application.PriceCardRegistrationIncomparable,
			HasRegistration: true}, http.StatusOK, "CANONICALIZATION_DIFFERS", false},
		{application.PublishPriceCardDraftResult{Outcome: application.PriceCardPublicationDraftNotApproved}, http.StatusOK, "", false},
	}
	for _, test := range cases {
		status, answer := serve(t, pricinghttp.NewPublishPriceCardDraftEndpoint(publicationIntakeDouble{}, publisherDouble{result: test.result}),
			progressRequest("/pricing-price-card-draft-publications", "t", draftReference))
		if status != test.status || answer.Outcome != test.result.Outcome.String() || answer.Registration != test.registration ||
			(answer.Draft != nil) != test.withDraft {
			t.Fatalf("%s/%s: %d %+v", test.result.Outcome, test.registration, status, answer)
		}
	}
	if status, answer := serve(t, pricinghttp.NewPublishPriceCardDraftEndpoint(publicationIntakeDouble{}, publisherDouble{err: errors.New("down")}),
		progressRequest("/x", "t", draftReference)); status != http.StatusInternalServerError || answer.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("no answer formed: %d %+v", status, answer)
	}
	if status, answer := serve(t, pricinghttp.NewPublishPriceCardDraftEndpoint(pricinghttp.UnconfiguredIntake{}, publisherDouble{}),
		progressRequest("/x", "t", draftReference)); status != http.StatusForbidden || answer.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured: %d %+v", status, answer)
	}
	recorder := httptest.NewRecorder()
	pricinghttp.NewPublishPriceCardDraftEndpoint(publicationIntakeDouble{}, publisherDouble{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", recorder.Code)
	}
}
