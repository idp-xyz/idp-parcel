package pricinghttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// 判据与价卡登记端点同族（见 register_price_card_test.go 文件头）；隔离读排除的
// 两口断言也在那份文件，一处钉两口。

type referenceSeriesIntakeDouble struct {
	command application.RegisterReferenceSeriesCommand
	err     error
}

func (double referenceSeriesIntakeDouble) IntakeReferenceSeriesRegistration(
	context.Context,
	*http.Request,
) (application.RegisterReferenceSeriesCommand, error) {
	if double.err != nil {
		return application.RegisterReferenceSeriesCommand{}, double.err
	}
	return double.command, nil
}

type referenceSeriesRegistrarDouble struct {
	outcome application.RegisterReferenceSeriesOutcome
	err     error
	called  bool
}

func (double *referenceSeriesRegistrarDouble) Handle(
	context.Context,
	application.RegisterReferenceSeriesCommand,
) (application.RegisterReferenceSeriesOutcome, error) {
	double.called = true
	return double.outcome, double.err
}

func TestRegisterReferenceSeriesEndpointOnlyAcceptsPost(t *testing.T) {
	endpoint := pricinghttp.NewRegisterReferenceSeriesEndpoint(
		referenceSeriesIntakeDouble{},
		&referenceSeriesRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-registrations", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", allow)
	}
}

func TestRegisterReferenceSeriesEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	registrar := &referenceSeriesRegistrarDouble{}
	endpoint := pricinghttp.NewRegisterReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, registrar)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-registrations", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 答 %d, want 403", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("错误码 = %q", code)
	}
	if registrar.called {
		t.Fatal("未配置即拒不构造命令，编排不该被调到")
	}
}

func TestRegisterReferenceSeriesEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := pricinghttp.NewRegisterReferenceSeriesEndpoint(
		referenceSeriesIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)},
		&referenceSeriesRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-registrations", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

func TestRegisterReferenceSeriesEndpointTranscribesAnswersVerbatim(t *testing.T) {
	cases := []struct {
		outcome    application.RegisterReferenceSeriesOutcome
		wantStatus int
		wantName   string
	}{
		{application.ReferenceSeriesRecorded, http.StatusCreated, "RECORDED"},
		{application.ReferenceSeriesAlreadyOnRegister, http.StatusOK, "ALREADY_REGISTERED"},
		{application.ReferenceSeriesRegistrationConflict, http.StatusOK, "CONTENT_CONFLICT"},
		{application.ReferenceSeriesRegistrationIncomparable, http.StatusOK, "CANONICALIZATION_DIFFERS"},
		{application.ReferenceSeriesRegistrationNotAccepted, http.StatusOK, "NOT_ACCEPTED"},
	}
	for _, testCase := range cases {
		endpoint := pricinghttp.NewRegisterReferenceSeriesEndpoint(
			referenceSeriesIntakeDouble{},
			&referenceSeriesRegistrarDouble{outcome: testCase.outcome},
		)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-registrations", nil))

		if recorder.Code != testCase.wantStatus {
			t.Fatalf("%s 答 %d, want %d", testCase.wantName, recorder.Code, testCase.wantStatus)
		}
		if body := decodeBody(t, recorder); body["outcome"] != testCase.wantName {
			t.Fatalf("outcome = %v, want %s", body["outcome"], testCase.wantName)
		}
	}
}

func TestRegisterReferenceSeriesEndpointAnswersServerErrorWhenRegistrationUndecided(t *testing.T) {
	endpoint := pricinghttp.NewRegisterReferenceSeriesEndpoint(
		referenceSeriesIntakeDouble{},
		&referenceSeriesRegistrarDouble{
			outcome: application.ReferenceSeriesRegistrationUndecided,
			err:     errors.New("库连不上"),
		},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-registrations", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
}
