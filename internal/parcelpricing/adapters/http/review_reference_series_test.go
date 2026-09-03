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

// 判据与两个登记端点同族（见 register_price_card_test.go 文件头）。本口多守一件：
// 复核的七格答案里有三格是**治理答案**（需换人复核、版本不在册、冲突），它们是形成了的
// 答案不是失败，传输层不合并、不改名、不借 4xx 说话。

const reviewTarget = "/pricing-reference-series-reviews"

type seriesReviewIntakeDouble struct {
	command application.ReviewReferenceSeriesCommand
	err     error
}

func (double seriesReviewIntakeDouble) IntakeReferenceSeriesReview(
	context.Context,
	*http.Request,
) (application.ReviewReferenceSeriesCommand, error) {
	if double.err != nil {
		return application.ReviewReferenceSeriesCommand{}, double.err
	}
	return double.command, nil
}

type seriesReviewerDouble struct {
	outcome application.ReviewReferenceSeriesOutcome
	err     error
	called  bool
}

func (double *seriesReviewerDouble) Handle(
	context.Context,
	application.ReviewReferenceSeriesCommand,
) (application.ReviewReferenceSeriesOutcome, error) {
	double.called = true
	return double.outcome, double.err
}

func TestReviewReferenceSeriesEndpointOnlyAcceptsPost(t *testing.T) {
	endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(
		seriesReviewIntakeDouble{},
		&seriesReviewerDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, reviewTarget, nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", allow)
	}
}

func TestReviewReferenceSeriesEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	reviewer := &seriesReviewerDouble{}
	endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, reviewer)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, reviewTarget, nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 答 %d, want 403", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("错误码 = %q", code)
	}
	if reviewer.called {
		t.Fatal("未配置即拒不构造命令，编排不该被调到——复核责任方尤其不能从请求里铸出来")
	}
}

func TestReviewReferenceSeriesEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(
		seriesReviewIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)},
		&seriesReviewerDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, reviewTarget, nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

// 七格逐字透出。**`需换人复核`与`版本不在册`是治理答案不是调用方错误**：前者的恢复动作是
// 换一个人来，后者是先去登记，两者都不该被折成 4xx——那会让调用方以为自己请求写错了。
func TestReviewReferenceSeriesEndpointTranscribesAnswersVerbatim(t *testing.T) {
	cases := []struct {
		outcome    application.ReviewReferenceSeriesOutcome
		wantStatus int
		wantName   string
	}{
		{application.SeriesReviewRecorded, http.StatusCreated, "RECORDED"},
		{application.SeriesReviewAlreadyOnRegister, http.StatusOK, "ALREADY_RECORDED"},
		{application.SeriesReviewConflict, http.StatusOK, "CONFLICT"},
		{application.SeriesReviewVersionUnknown, http.StatusOK, "VERSION_UNKNOWN"},
		{application.SeriesReviewNeedsAnotherReviewer, http.StatusOK, "NEEDS_ANOTHER_REVIEWER"},
		{application.SeriesReviewNotAccepted, http.StatusOK, "NOT_ACCEPTED"},
		{application.SeriesReviewUndecided, http.StatusOK, "UNDECIDED"},
	}
	for _, testCase := range cases {
		endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(
			seriesReviewIntakeDouble{},
			&seriesReviewerDouble{outcome: testCase.outcome},
		)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, reviewTarget, nil))

		if recorder.Code != testCase.wantStatus {
			t.Fatalf("%s 答 %d, want %d", testCase.wantName, recorder.Code, testCase.wantStatus)
		}
		if body := decodeBody(t, recorder); body["outcome"] != testCase.wantName {
			t.Fatalf("outcome = %v, want %s", body["outcome"], testCase.wantName)
		}
	}
}

// 依赖故障走错误那一支而不是 outcome：复核用例把`未决`连同 error 一起交回，端点只认
// error。**这一格与上面那张表里的 UNDECIDED 不矛盾**——那一行钉的是「若编排单独交回它
// 而不报错，也照实透出」，这一行钉的是「真报了错就不许带业务 outcome 上线」。
func TestReviewReferenceSeriesEndpointAnswersServerErrorWhenReviewUndecided(t *testing.T) {
	endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(
		seriesReviewIntakeDouble{},
		&seriesReviewerDouble{
			outcome: application.SeriesReviewUndecided,
			err:     errors.New("复核册连不上"),
		},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, reviewTarget, nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
	if body := decodeBody(t, recorder); body["outcome"] != nil {
		t.Fatalf("没形成答案的响应带了 outcome：%v", body["outcome"])
	}
}

// 应用层交回没有名字的结果是编程错误，不是业务答案——空 outcome 会被客户端当成一种新的
// 业务结果（判据同价卡登记口的 codeUnnamedOutcome）。
func TestReviewReferenceSeriesEndpointRefusesAnUnnamedOutcome(t *testing.T) {
	endpoint := pricinghttp.NewReviewReferenceSeriesEndpoint(
		seriesReviewIntakeDouble{},
		&seriesReviewerDouble{outcome: application.ReviewReferenceSeriesOutcomeInvalid},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, reviewTarget, nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("无名结果答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "UNNAMED_OUTCOME" {
		t.Fatalf("错误码 = %q", code)
	}
}
