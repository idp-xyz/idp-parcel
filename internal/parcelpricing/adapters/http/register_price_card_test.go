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

// 本文件对价卡登记端点（ADR-0085 首切片）证传输面：方法门、未配置 Intake 403 且不
// 构造命令、Intake 失败分流、答案逐名转写、依赖故障 5xx；并钉住 ADR-0078 的排除仍然
// 成立——隔离读 Intake 装不进登记口。断言助手复用 query_pricing_catalogue_test.go 既有件。

type priceCardIntakeDouble struct {
	command application.RegisterPriceCardCommand
	err     error
}

func (double priceCardIntakeDouble) IntakePriceCardRegistration(
	context.Context,
	*http.Request,
) (application.RegisterPriceCardCommand, error) {
	if double.err != nil {
		return application.RegisterPriceCardCommand{}, double.err
	}
	return double.command, nil
}

type priceCardRegistrarDouble struct {
	outcome application.RegisterPriceCardOutcome
	err     error
	called  bool
}

func (double *priceCardRegistrarDouble) Handle(
	context.Context,
	application.RegisterPriceCardCommand,
) (application.RegisterPriceCardOutcome, error) {
	double.called = true
	return double.outcome, double.err
}

func TestRegisterPriceCardEndpointOnlyAcceptsPost(t *testing.T) {
	endpoint := pricinghttp.NewRegisterPriceCardEndpoint(priceCardIntakeDouble{}, &priceCardRegistrarDouble{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-price-card-registrations", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", allow)
	}
}

func TestRegisterPriceCardEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	registrar := &priceCardRegistrarDouble{}
	endpoint := pricinghttp.NewRegisterPriceCardEndpoint(pricinghttp.UnconfiguredIntake{}, registrar)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-registrations", nil))

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

func TestRegisterPriceCardEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := pricinghttp.NewRegisterPriceCardEndpoint(
		priceCardIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)},
		&priceCardRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-registrations", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

func TestRegisterPriceCardEndpointMapsOtherIntakeFailureToIntakeFailed(t *testing.T) {
	endpoint := pricinghttp.NewRegisterPriceCardEndpoint(
		priceCardIntakeDouble{err: errors.New("认证后端寄了")},
		&priceCardRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-registrations", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Intake 故障答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "INTAKE_FAILED" {
		t.Fatalf("错误码 = %q", code)
	}
}

func TestRegisterPriceCardEndpointTranscribesAnswersVerbatim(t *testing.T) {
	cases := []struct {
		outcome    application.RegisterPriceCardOutcome
		wantStatus int
		wantName   string
	}{
		{application.PriceCardRecorded, http.StatusCreated, "RECORDED"},
		{application.PriceCardAlreadyOnRegister, http.StatusOK, "ALREADY_REGISTERED"},
		{application.PriceCardRegistrationConflict, http.StatusOK, "CONTENT_CONFLICT"},
		{application.PriceCardRegistrationIncomparable, http.StatusOK, "CANONICALIZATION_DIFFERS"},
		{application.PriceCardRegistrationNotAccepted, http.StatusOK, "NOT_ACCEPTED"},
	}
	for _, testCase := range cases {
		endpoint := pricinghttp.NewRegisterPriceCardEndpoint(
			priceCardIntakeDouble{},
			&priceCardRegistrarDouble{outcome: testCase.outcome},
		)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-registrations", nil))

		if recorder.Code != testCase.wantStatus {
			t.Fatalf("%s 答 %d, want %d", testCase.wantName, recorder.Code, testCase.wantStatus)
		}
		if body := decodeBody(t, recorder); body["outcome"] != testCase.wantName {
			t.Fatalf("outcome = %v, want %s", body["outcome"], testCase.wantName)
		}
	}
}

func TestRegisterPriceCardEndpointAnswersServerErrorWhenRegistrationUndecided(t *testing.T) {
	endpoint := pricinghttp.NewRegisterPriceCardEndpoint(
		priceCardIntakeDouble{},
		&priceCardRegistrarDouble{
			outcome: application.PriceCardRegistrationUndecided,
			err:     errors.New("库连不上"),
		},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-registrations", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
}

// TestIsolatedReadIntakeCannotServeRegistration 钉住 ADR-0078 的编译期排除在写面成立：
// 隔离读放行类型不满足任何登记命令 Intake 接口。这条一旦变红，说明有人把隔离放行
// 扩到了写行——那是 ADR-0085 Decision 二明文维持的原判。
func TestIsolatedReadIntakeCannotServeRegistration(t *testing.T) {
	var intake any = pricinghttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(pricinghttp.PriceCardRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进价卡登记口")
	}
	if _, ok := intake.(pricinghttp.ReferenceSeriesRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进序列登记口")
	}
	// 复核口同属写面（票 pricing-reference-series-operations/04）。它比两个登记口更不能放
	// ——复核责任方是四眼门的一半，让隔离读那个注入的合成身份装进来，四眼门就没了。
	if _, ok := intake.(pricinghttp.ReferenceSeriesReviewIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进序列复核口")
	}
}
