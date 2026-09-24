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
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 试算端点（UC-PP-001，ADR-0152 决定六、七）：`outcome` 只说编排，逐卡评价自己的状态原样透出；试算什么都不入册，
// 所以金额、费用行、版本清单与解释都随响应交回——这一点与回放口不同。载荷严格解码，身份格按未知键拒。

const estimateTarget = "/pricing-estimates"

type estimateIntakeDouble struct {
	command application.FormEstimateEvaluationsCommand
	err     error
}

func (double estimateIntakeDouble) IntakeEstimate(context.Context, *http.Request) (application.FormEstimateEvaluationsCommand, error) {
	if double.err != nil {
		return application.FormEstimateEvaluationsCommand{}, double.err
	}
	return double.command, nil
}

type estimateFormerDouble struct {
	result application.FormEstimateEvaluationsResult
	err    error
	called bool
}

func (double *estimateFormerDouble) Handle(
	context.Context,
	application.FormEstimateEvaluationsCommand,
) (application.FormEstimateEvaluationsResult, error) {
	double.called = true
	return double.result, double.err
}

func estimatePlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-EST", "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-EST", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-EST", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             domain.PricingDirectionSell,
		Purpose:               domain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndsAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-Z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "55",
		WeightRounding:        domain.RoundingCeiling,
		WeightStepKilograms:   "0.5",
	})
}

func estimatedEvaluation(t *testing.T, plan domain.PricingPlanVersion) domain.PricingEvaluation {
	t.Helper()
	input := pptest.Input(t, pptest.InputSpec{
		Tenant: "SYN-TENANT-01", Scope: "SYN-SCOPE-01", PackageID: "SYN-PKG-EST", Zone: "Z1",
		Kilograms: "1", BusinessAt: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
	})
	return pptest.Evaluate(t, "EST-0000000000000001", plan, input)
}

func postEstimate(t *testing.T, intake pricinghttp.EstimateIntake, former pricinghttp.EstimateFormer) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, estimateTarget, strings.NewReader(`{}`))
	pricinghttp.NewEstimateEndpoint(intake, former).ServeHTTP(recorder, request)
	return recorder
}

// 已形成：逐卡并列，已评价的那格带真评价的状态、证据、合计、费用行与版本清单；输入不全的那格点名缺项、不带评价。
func TestEstimateEndpointReturnsEveryCandidateWithItsOwnEvaluation(t *testing.T) {
	plan := estimatePlan(t)
	evaluation := estimatedEvaluation(t, plan)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("fixture evaluation status = %s, 夹具要一份完成的评价", evaluation.Status())
	}
	other := pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-EST-2", "v3")
	former := &estimateFormerDouble{result: application.FormEstimateEvaluationsResult{
		Outcome: application.EstimateEvaluationsFormed,
		Candidates: []application.EstimateCandidate{
			{Plan: plan.Reference(), Answer: application.EstimateCandidateEvaluated, Evaluation: evaluation},
			{Plan: other, Answer: application.EstimateCandidateInputIncomplete, Missing: []string{application.EstimateMissingPostalRoute}},
		},
	}}

	recorder := postEstimate(t, estimateIntakeDouble{}, former)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome    string `json:"outcome"`
		Candidates []struct {
			Plan       struct{ ID, Version string } `json:"plan"`
			Answer     string                       `json:"answer"`
			Missing    []string                     `json:"missing"`
			Evaluation *struct {
				EvaluationID string `json:"evaluationId"`
				Status       string `json:"status"`
				Evidence     string `json:"evidence"`
				Total        *struct {
					Amount   string `json:"amount"`
					Currency string `json:"currency"`
				} `json:"total"`
				ChargeLines []struct {
					Code   string `json:"code"`
					Amount struct {
						Amount   string `json:"amount"`
						Currency string `json:"currency"`
					} `json:"amount"`
				} `json:"chargeLines"`
				Manifest []struct{ Kind, ID, Version string } `json:"manifest"`
			} `json:"evaluation"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body = %s", err, recorder.Body)
	}
	if body.Outcome != "FORMED" || len(body.Candidates) != 2 {
		t.Fatalf("body = %s, 想要 FORMED 两格", recorder.Body)
	}
	evaluated := body.Candidates[0]
	if evaluated.Answer != "EVALUATED" || evaluated.Plan.ID != "SYN-PLAN-EST" || evaluated.Evaluation == nil ||
		evaluated.Evaluation.Status != "COMPLETED" || evaluated.Evaluation.Evidence != "S" ||
		evaluated.Evaluation.Total == nil || evaluated.Evaluation.Total.Amount != "55" ||
		evaluated.Evaluation.Total.Currency != "CNY" || len(evaluated.Evaluation.ChargeLines) == 0 || len(evaluated.Evaluation.Manifest) == 0 {
		t.Fatalf("已评价那格 = %+v body = %s", evaluated, recorder.Body)
	}
	incomplete := body.Candidates[1]
	if incomplete.Answer != "INPUT_INCOMPLETE" || incomplete.Evaluation != nil || len(incomplete.Missing) != 1 || incomplete.Missing[0] != "POSTAL_ROUTE" {
		t.Fatalf("输入不全那格 = %+v", incomplete)
	}
}

// 冲突与未决都是形成了的答案，取 200，状态码不替 outcome 说话（ADR-0022）；冲突交候选，未决交停在哪一口。
func TestEstimateEndpointAnswersConflictAndUndecidedAsFormedAnswers(t *testing.T) {
	candidate := pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-EST", "v2")
	recorder := postEstimate(t, estimateIntakeDouble{}, &estimateFormerDouble{result: application.FormEstimateEvaluationsResult{
		Outcome:  application.EstimatePriceCardApplicabilityConflict,
		Conflict: []domain.VersionReference{candidate},
	}})
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"outcome":"PRICE_CARD_APPLICABILITY_CONFLICT"`) ||
		!strings.Contains(recorder.Body.String(), `"version":"v2"`) {
		t.Fatalf("status = %d body = %s, 想要 200 适用冲突带候选", recorder.Code, recorder.Body)
	}

	recorder = postEstimate(t, estimateIntakeDouble{}, &estimateFormerDouble{result: application.FormEstimateEvaluationsResult{
		Outcome: application.EstimateUndecided,
		Reason:  application.EstimatePriceCardLoadUnavailable,
	}})
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"reason":"PRICE_CARD_LOAD_UNAVAILABLE"`) {
		t.Fatalf("status = %d body = %s, 想要 200 未决带停处", recorder.Code, recorder.Body)
	}
}

// 没形成答案的几格走传输层：未配置 403、畸形 400、方法不对 405、依赖故障 500；未配置时编排一口不问。
func TestEstimateEndpointTransportAnswers(t *testing.T) {
	former := &estimateFormerDouble{}
	recorder := postEstimate(t, pricinghttp.UnconfiguredIntake{}, former)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") || former.called {
		t.Fatalf("未配置 status = %d body = %s called = %v", recorder.Code, recorder.Body, former.called)
	}

	recorder = postEstimate(t, estimateIntakeDouble{err: pricinghttp.ErrMalformedRequest}, &estimateFormerDouble{})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形 status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	pricinghttp.NewEstimateEndpoint(estimateIntakeDouble{}, &estimateFormerDouble{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, estimateTarget, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("方法 status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}

	recorder = postEstimate(t, estimateIntakeDouble{}, &estimateFormerDouble{err: errors.New("boom")})
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("依赖故障 status = %d body = %s", recorder.Code, recorder.Body)
	}
}

// 载荷严格解码：身份格（租户）与任何未知键都拒；合法载荷逐格译成命令，租户只从调用方（信封）来。
func TestEstimatePayloadDecodesStrictlyAndTranslatesEveryField(t *testing.T) {
	if _, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(`{"tenant":"SYN-TENANT-01","scope":"SYN-SCOPE-01"}`)); err == nil {
		t.Fatalf("带租户格的载荷被收下了")
	}
	payload, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(`{
		"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"2026-06-01T08:00:00Z",
		"weight":{"value":"1.2","unit":"KG"},
		"dimensions":{"length":"30","width":"20","height":"10","unit":"CM"},
		"zone":"Z1","postalRoute":{"origin":"200000","destination":"90210"},"settlementCurrency":"CNY"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	command, err := payload.Command(tenant)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if command.Tenant != tenant || command.Scope.String() != "SYN-SCOPE-01" || command.Direction != domain.PricingDirectionSell ||
		!command.BasisAt.Equal(time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)) || command.Weight.Value().String() != "1.2" ||
		command.Zone != "Z1" || command.Dimensions == nil || command.Route == nil || command.Route.Destination() != "90210" ||
		command.Settlement == nil || command.Settlement.String() != "CNY" {
		t.Fatalf("command = %+v", command)
	}

	for name, raw := range map[string]string{
		"缺范围":   `{"direction":"SELL","basisAt":"2026-06-01T08:00:00Z","weight":{"value":"1","unit":"KG"}}`,
		"方向不认识": `{"scope":"SYN-SCOPE-01","direction":"SIDEWAYS","basisAt":"2026-06-01T08:00:00Z","weight":{"value":"1","unit":"KG"}}`,
		"时点不合式": `{"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"yesterday","weight":{"value":"1","unit":"KG"}}`,
		"缺实重":   `{"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"2026-06-01T08:00:00Z"}`,
		"单位不认识": `{"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"2026-06-01T08:00:00Z","weight":{"value":"1","unit":"STONE"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(raw))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, err := payload.Command(tenant); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, 想要 ErrMalformedRequest", err)
			}
		})
	}
}
