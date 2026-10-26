package pricinghttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// 目录复核口与序列复核口同族：未配置即拒且编排不被调到、畸形答 400、各格答案逐字透出（治理答案是 200 不是 4xx）、依赖故障
// 答 500 不带 outcome、无名结果答 500；操作者渠道的译法里复核人与授予集只从信封来。

const catalogueReviewTarget = "/pricing-reference-catalogue-reviews"

type catalogueReviewIntakeDouble struct {
	command application.ReviewReferenceCatalogueCommand
	err     error
}

func (double catalogueReviewIntakeDouble) IntakeReferenceCatalogueReview(context.Context, *http.Request) (application.ReviewReferenceCatalogueCommand, error) {
	return double.command, double.err
}

type catalogueReviewerDouble struct {
	outcome application.ReviewReferenceCatalogueOutcome
	err     error
	called  bool
}

func (double *catalogueReviewerDouble) Handle(context.Context, application.ReviewReferenceCatalogueCommand) (application.ReviewReferenceCatalogueOutcome, error) {
	double.called = true
	return double.outcome, double.err
}

func serveCatalogueReview(endpoint http.Handler, method string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, catalogueReviewTarget, nil))
	return recorder
}

func TestReviewReferenceCatalogueEndpointRefusesBeforeTheReviewer(t *testing.T) {
	recorder := serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(catalogueReviewIntakeDouble{}, &catalogueReviewerDouble{}), http.MethodGet)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET 答 %d Allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}

	reviewer := &catalogueReviewerDouble{}
	recorder = serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(pricinghttp.UnconfiguredIntake{}, reviewer), http.MethodPost)
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" || reviewer.called {
		t.Fatalf("未配置 Intake 答 %d，编排被调到 = %v", recorder.Code, reviewer.called)
	}

	recorder = serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(
		catalogueReviewIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)}, &catalogueReviewerDouble{}), http.MethodPost)
	if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "MALFORMED_REQUEST" {
		t.Fatalf("畸形请求答 %d", recorder.Code)
	}
}

// Covers: AT-PP-014、AT-PP-015 的目录操作者面——未配置、须换人、复核者不合格各以自己的名字作 200 答出，`已追加`答 201。
func TestReviewReferenceCatalogueEndpointTranscribesAnswersVerbatim(t *testing.T) {
	cases := []struct {
		outcome    application.ReviewReferenceCatalogueOutcome
		wantStatus int
		wantName   string
	}{
		{application.CatalogueReviewRecorded, http.StatusCreated, "RECORDED"},
		{application.CatalogueReviewAlreadyOnRegister, http.StatusOK, "ALREADY_RECORDED"},
		{application.CatalogueReviewConflict, http.StatusOK, "CONFLICT"},
		{application.CatalogueReviewVersionUnknown, http.StatusOK, "VERSION_UNKNOWN"},
		{application.CatalogueReviewNeedsAnotherReviewer, http.StatusOK, "NEEDS_ANOTHER_REVIEWER"},
		{application.CatalogueReviewNotConfigured, http.StatusOK, "NOT_CONFIGURED"},
		{application.CatalogueReviewReviewerNotQualified, http.StatusOK, "REVIEWER_NOT_QUALIFIED"},
		{application.CatalogueReviewNotAccepted, http.StatusOK, "NOT_ACCEPTED"},
	}
	for _, testCase := range cases {
		recorder := serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(catalogueReviewIntakeDouble{},
			&catalogueReviewerDouble{outcome: testCase.outcome}), http.MethodPost)
		if recorder.Code != testCase.wantStatus {
			t.Fatalf("%s 答 %d, want %d", testCase.wantName, recorder.Code, testCase.wantStatus)
		}
		if body := decodeBody(t, recorder); body["outcome"] != testCase.wantName {
			t.Fatalf("outcome = %v, want %s", body["outcome"], testCase.wantName)
		}
	}

	recorder := serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(catalogueReviewIntakeDouble{},
		&catalogueReviewerDouble{outcome: application.CatalogueReviewUndecided, err: errors.New("复核册连不上")}), http.MethodPost)
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" || decodeBody(t, recorder)["outcome"] != nil {
		t.Fatalf("未决答 %d", recorder.Code)
	}
	recorder = serveCatalogueReview(pricinghttp.NewReviewReferenceCatalogueEndpoint(catalogueReviewIntakeDouble{},
		&catalogueReviewerDouble{outcome: application.ReviewReferenceCatalogueOutcomeInvalid}), http.MethodPost)
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "UNNAMED_OUTCOME" {
		t.Fatalf("无名结果答 %d", recorder.Code)
	}
}

// Covers: AT-PP-015 的操作者面那一半（目录）——复核人与授予集取认证出的身份与信封里的授予；自报复核人、租户、授予集，
// 带系统代写那一格，时刻写坏，一律答坏报文；没令牌答凭证被拒。
func TestCatalogueReviewTakesTheReviewerAndGrantsFromTheAuthenticatedOperator(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{grants: []string{"REGISTRY_CONFIGURATION_WRITE"}})
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, body string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, catalogueReviewTarget, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		return request
	}
	command, err := intake.IntakeReferenceCatalogueReview(context.Background(), request("t",
		`{"catalogueId": "SYN-CAT-ZONE-CN-SG", "catalogueVersion": "v1", "decision": "APPROVED", "basis": "SYN-BASIS", "reviewedAt": "2026-09-25T01:00:00Z"}`))
	if err != nil {
		t.Fatalf("catalogue review: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.Reviewer != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
		command.CatalogueID != "SYN-CAT-ZONE-CN-SG" || command.CatalogueVersion != "v1" || command.ReviewedAt.IsZero() ||
		len(command.ReviewerGrants) != 1 || command.ReviewerGrants[0].String() != "REGISTRY_CONFIGURATION_WRITE" {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"reviewer smuggled":   `{"catalogueId": "SYN-CAT-ZONE-CN-SG", "reviewer": "someone-else"}`,
		"tenant smuggled":     `{"catalogueId": "SYN-CAT-ZONE-CN-SG", "tenant": "SYN-TENANT-02"}`,
		"grants smuggled":     `{"catalogueId": "SYN-CAT-ZONE-CN-SG", "reviewerGrants": ["SYN-GRANT-REFERENCE-REVIEW"]}`,
		"delegation smuggled": `{"catalogueId": "SYN-CAT-ZONE-CN-SG", "systemDelegatedUnderExemption": true}`,
		"instant garbled":     `{"catalogueId": "SYN-CAT-ZONE-CN-SG", "reviewedAt": "yesterday"}`,
		"trailing content":    `{"catalogueId": "SYN-CAT-ZONE-CN-SG"} {}`,
		"trailing brace":      `{"catalogueId": "SYN-CAT-ZONE-CN-SG"}}`,
	} {
		if _, err := intake.IntakeReferenceCatalogueReview(context.Background(), request("t", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeReferenceCatalogueReview(context.Background(), request("", `{}`)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没令牌：err = %v", err)
	}
}
