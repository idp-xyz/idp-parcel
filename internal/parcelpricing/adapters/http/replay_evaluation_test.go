package pricinghttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 判据与复核端点同族（见 review_reference_series_test.go 文件头）。本口多守两件（ADR-0124）：
// `outcome` 只说编排，回放评价自己的状态与问题项随 `replay` 原样透出、面上不折；载荷三样一样不默认
// ——证据层级缺席是畸形请求，不是 `S`。

const replayTarget = "/pricing-evaluation-replays"

type evaluationReplayIntakeDouble struct {
	command application.ReplayPricingEvaluationCommand
	err     error
}

func (double evaluationReplayIntakeDouble) IntakeEvaluationReplay(
	context.Context,
	*http.Request,
) (application.ReplayPricingEvaluationCommand, error) {
	if double.err != nil {
		return application.ReplayPricingEvaluationCommand{}, double.err
	}
	return double.command, nil
}

type evaluationReplayerDouble struct {
	result application.ReplayPricingEvaluationResult
	err    error
	called bool
}

func (double *evaluationReplayerDouble) Handle(
	context.Context,
	application.ReplayPricingEvaluationCommand,
) (application.ReplayPricingEvaluationResult, error) {
	double.called = true
	return double.result, double.err
}

// replayedEvaluation 造一份真的回放评价（S 重放成 S、单分区费率表命中 Z1）——响应体透出的字段要从真
// 评价上读，替身造不出 ReplayOf。
func replayedEvaluation(t *testing.T) (domain.PricingEvaluation, domain.PricingEvaluation) {
	t.Helper()
	currency, err := domain.NewCurrency("USD")
	if err != nil {
		t.Fatalf("currency: %v", err)
	}
	amount, err := domain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	weight := func(value string) domain.Weight {
		built, err := domain.NewWeight(mustDecimal(t, value), domain.WeightUnitKilogram)
		if err != nil {
			t.Fatalf("weight %s: %v", value, err)
		}
		return built
	}
	entryID, err := domain.NewRateEntryID("entry-1")
	if err != nil {
		t.Fatalf("entry id: %v", err)
	}
	entry, err := domain.NewRateEntry(entryID, "Z1", weight("0"), weight("10"), amount)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	period, err := domain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	reference := func(kind domain.ArtifactKind, id string) domain.VersionReference {
		built, err := domain.NewVersionReferenceIdentity(kind, id, "v1")
		if err != nil {
			t.Fatalf("reference %s: %v", id, err)
		}
		return built
	}
	table, err := domain.NewRateTableVersion(reference(domain.ArtifactRateTable, "table-1"), domain.RateTableFamilyWeightZone,
		currency, domain.WeightUnitKilogram, period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.5"))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(reference(domain.ArtifactWeightPolicy, "weight-1"), domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	scope, err := domain.NewPricingScopeID("scope-1")
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	baseCode, err := domain.NewChargeCode("BASE_FREIGHT")
	if err != nil {
		t.Fatalf("charge code: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(reference(domain.ArtifactPricingPlan, "plan-1"), scope, domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge, baseCode, period, table, weightPolicy, nil, domain.PricingPlanStructures{})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	packageID, err := domain.NewPackageID("package-1")
	if err != nil {
		t.Fatalf("package id: %v", err)
	}
	subject, err := domain.NewAcceptedPackageSubject(packageID)
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(payloadTenant(t), scope, subject, "Z1", weight("5"), nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	originalID, err := domain.NewEvaluationID("eval-original")
	if err != nil {
		t.Fatalf("original id: %v", err)
	}
	request, err := domain.NewEvaluationRequest(originalID, plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	original := domain.EvaluatePricing(request)
	replayID, err := domain.NewEvaluationID("eval-replay")
	if err != nil {
		t.Fatalf("replay id: %v", err)
	}
	replayed, err := domain.ReplayPricingEvaluation(replayID, original, plan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	return original, replayed
}

func TestReplayEvaluationEndpointOnlyAcceptsPost(t *testing.T) {
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(evaluationReplayIntakeDouble{}, &evaluationReplayerDouble{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, replayTarget, nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", allow)
	}
}

// Covers: ADR-0055 决定一至三在本口——未配置即拒、403、不读内容不构造命令；ADR-0124 决定一：这一口
// 与其余命令面同一条缝。
func TestReplayEvaluationEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	replayer := &evaluationReplayerDouble{}
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(pricinghttp.UnconfiguredIntake{}, replayer)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget,
		strings.NewReader(`{"originalEvaluationId":"eval-1","replayEvaluationId":"eval-2","evidence":"R"}`)))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 答 %d, want 403", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("错误码 = %q", code)
	}
	if replayer.called {
		t.Fatal("未配置即拒不构造命令，编排不该被调到——触发者尤其不能从请求里铸出来")
	}
	if body := decodeBody(t, recorder); body["outcome"] != nil {
		t.Fatalf("没形成答案的响应带了 outcome：%v", body["outcome"])
	}
}

func TestReplayEvaluationEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(
		evaluationReplayIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)},
		&evaluationReplayerDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget, nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

// Covers: ADR-0124 决定四——`已入册`取 201 并随 `replay` 透出回放评价的身份与结论（新引用、回指原评价、
// 状态、证据层级、语义摘要、问题项），面上不替评价说话；`replay` 只在形成了评价时出现。
func TestReplayEvaluationEndpointTranscribesTheRecordedReplay(t *testing.T) {
	original, replayed := replayedEvaluation(t)
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(
		evaluationReplayIntakeDouble{},
		&evaluationReplayerDouble{result: replayResult(t, application.ReplayRecorded, replayed)},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget, nil))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("RECORDED 答 %d, want 201", recorder.Code)
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "RECORDED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	replay, ok := body["replay"].(map[string]any)
	if !ok {
		t.Fatalf("响应没带 replay：%v", body)
	}
	if replay["evaluationId"] != "eval-replay" || replay["replayOf"] != original.ID().String() {
		t.Fatalf("回放身份透出错了：%v", replay)
	}
	if replay["status"] != "COMPLETED" || replay["evidence"] != "S" || replay["semanticDigest"] != original.SemanticDigest() {
		t.Fatalf("回放结论透出错了：%v", replay)
	}
	// 问题项逐条照登：这张合成卡没声明金额取整策略，完成的评价带着 AMOUNT_PRECISION_UNDECLARED（ADR-0107）
	// ——重现了的回放同样带着它，面上不因为「重现了」就把问题项抹掉。
	issues, ok := replay["issues"].([]any)
	if !ok || len(issues) != len(replayed.Issues()) || len(issues) == 0 {
		t.Fatalf("issues 没照登：%v vs %d 条", replay["issues"], len(replayed.Issues()))
	}
	if first, _ := issues[0].(map[string]any); first["code"] != replayed.Issues()[0].Code() || first["message"] != replayed.Issues()[0].Message() {
		t.Fatalf("问题项转写变形：%v", issues[0])
	}
}

// Covers: 其余七格逐字透出、取 200；没形成评价的答案不带 `replay`。**`原评价不在册`与`原方案版本不在册`
// 是形成了的答案不是调用方错误**——前者的恢复动作是核对引用或去别的租户，后者是先去登记那一版，两者都
// 不该被折成 4xx。
func TestReplayEvaluationEndpointTranscribesAnswersVerbatim(t *testing.T) {
	cases := []struct {
		outcome  application.ReplayPricingEvaluationOutcome
		wantName string
	}{
		{application.ReplayExistingResult, "EXISTING_RESULT"},
		{application.ReplayIdentityConflict, "IDENTITY_CONFLICT"},
		{application.ReplayOriginalNotFound, "ORIGINAL_NOT_FOUND"},
		{application.ReplayPlanVersionNotOnRegister, "PLAN_VERSION_NOT_ON_REGISTER"},
		{application.ReplayPlanCanonicalizationUnsupported, "PLAN_CANONICALIZATION_UNSUPPORTED"},
		{application.ReplayNotAccepted, "NOT_ACCEPTED"},
		{application.ReplayUndecided, "UNDECIDED"},
	}
	for _, testCase := range cases {
		endpoint := pricinghttp.NewReplayEvaluationEndpoint(
			evaluationReplayIntakeDouble{},
			&evaluationReplayerDouble{result: replayResult(t, testCase.outcome, domain.PricingEvaluation{})},
		)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s 答 %d, want 200", testCase.wantName, recorder.Code)
		}
		body := decodeBody(t, recorder)
		if body["outcome"] != testCase.wantName {
			t.Fatalf("outcome = %v, want %s", body["outcome"], testCase.wantName)
		}
		if _, present := body["replay"]; present {
			t.Fatalf("%s 没形成评价却带了 replay：%v", testCase.wantName, body)
		}
	}
}

// 依赖故障走错误那一支而不是 outcome（判据同复核口）。
func TestReplayEvaluationEndpointAnswersServerErrorWhenReplayUndecided(t *testing.T) {
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(
		evaluationReplayIntakeDouble{},
		&evaluationReplayerDouble{result: replayResult(t, application.ReplayUndecided, domain.PricingEvaluation{}), err: errors.New("评价册连不上")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget, nil))

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

func TestReplayEvaluationEndpointRefusesAnUnnamedOutcome(t *testing.T) {
	endpoint := pricinghttp.NewReplayEvaluationEndpoint(
		evaluationReplayIntakeDouble{},
		&evaluationReplayerDouble{result: application.ReplayPricingEvaluationResult{}},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, replayTarget, nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("无名结果答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "UNNAMED_OUTCOME" {
		t.Fatalf("错误码 = %q", code)
	}
}

// Covers: ADR-0124 决定三——载荷三样都由触发方声明、一样不默认。合法载荷翻成命令；证据层级缺席或不在
// S / R / P 里、引用为空、带身份键（tenant）、JSON 不合法，都是畸形请求；Intake 没给租户是身份缺席不是
// 载荷的错。
func TestEvaluationReplayPayloadTranslatesAndRefusesDefaults(t *testing.T) {
	tenant := payloadTenant(t)

	payload, err := pricinghttp.DecodeEvaluationReplayPayload(strings.NewReader(
		`{"originalEvaluationId":"eval-1","replayEvaluationId":"eval-2","evidence":"R"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	command, err := payload.Command(tenant)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if command.Tenant != tenant || command.Original.String() != "eval-1" || command.ReplayID.String() != "eval-2" || command.Evidence != domain.EvidenceReplay {
		t.Fatalf("command = %+v", command)
	}

	if _, err := payload.Command(domain.TenantID{}); !errors.Is(err, pricinghttp.ErrOperatorIdentityMissing) {
		t.Fatalf("没给租户 err = %v, want ErrOperatorIdentityMissing", err)
	}

	malformed := map[string]string{
		"证据层级缺席不是默认 S": `{"originalEvaluationId":"eval-1","replayEvaluationId":"eval-2"}`,
		"证据层级不在封闭集":    `{"originalEvaluationId":"eval-1","replayEvaluationId":"eval-2","evidence":"X"}`,
		"原引用为空":        `{"originalEvaluationId":"","replayEvaluationId":"eval-2","evidence":"S"}`,
		"新引用为空":        `{"originalEvaluationId":"eval-1","replayEvaluationId":" ","evidence":"S"}`,
	}
	for name, raw := range malformed {
		decoded, err := pricinghttp.DecodeEvaluationReplayPayload(strings.NewReader(raw))
		if err != nil {
			t.Fatalf("%s：结构解码不该拒（值的对错留给 Command）：%v", name, err)
		}
		if _, err := decoded.Command(tenant); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	structural := map[string]string{
		"载荷里带身份键":  `{"tenant":"t-1","originalEvaluationId":"eval-1","replayEvaluationId":"eval-2","evidence":"S"}`,
		"JSON 不合法": `{"originalEvaluationId":`,
	}
	for name, raw := range structural {
		if _, err := pricinghttp.DecodeEvaluationReplayPayload(strings.NewReader(raw)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// replayResult 借 application 包的构造路径造一份带指定 outcome 的结果：结果类型字段不导出，唯一的合法
// 造法是经编排跑出来——这里用一对替身把编排逼到那一格。
func replayResult(t *testing.T, outcome application.ReplayPricingEvaluationOutcome, evaluation domain.PricingEvaluation) application.ReplayPricingEvaluationResult {
	t.Helper()
	return application.ReplayPricingEvaluationResult{Outcome: outcome, Evaluation: evaluation, HasEvaluation: evaluation.ID().String() != ""}
}
