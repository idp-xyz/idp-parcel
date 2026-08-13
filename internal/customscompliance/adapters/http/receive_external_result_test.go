package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var endpointAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

type resultIntakeDouble struct {
	command application.ReceiveExternalResultCommand
	err     error
}

func (double *resultIntakeDouble) IntakeResult(
	_ context.Context,
	_ *http.Request,
) (application.ReceiveExternalResultCommand, error) {
	if double.err != nil {
		return application.ReceiveExternalResultCommand{}, double.err
	}
	return double.command, nil
}

// 端点测试驱动真实编排（替身只在端口层），照 NO 收寄处理器测试同款——传输层不该能
// 捏造业务结果，业务格从真 handler 走出来。
type resultStoreDouble struct {
	byKey map[ports.ExternalResultKey]ports.ExternalResultRecord
}

func (double *resultStoreDouble) FindByKey(
	_ context.Context,
	key ports.ExternalResultKey,
) (ports.ExternalResultRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *resultStoreDouble) Save(
	_ context.Context,
	record ports.ExternalResultRecord,
) (ports.ExternalResultSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.ExternalResultAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return ports.ExternalResultSaved, nil
}

func (double *resultStoreDouble) LoadForSubmission(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SubmissionVersionID,
) ([]domain.ExternalResult, error) {
	return nil, nil
}

type submissionIndexDouble struct{ found bool }

func (double *submissionIndexDouble) FindSubmission(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SubmissionVersionID,
) (bool, error) {
	return double.found, nil
}

type interpretationRuleDouble struct{ configured bool }

func (double *interpretationRuleDouble) LoadInterpretationRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ResultLayer,
) (domain.InterpretationRuleReference, bool, error) {
	if !double.configured {
		return domain.InterpretationRuleReference{}, false, nil
	}
	rule, err := domain.NewInterpretationRuleReference("INTERPRET/US-IMPORT/receipt-v1")
	return rule, true, err
}

type resultDownstreamDouble struct{}

func (double *resultDownstreamDouble) HandOffExternalResult(
	_ context.Context,
	_ ports.ExternalResultHandoffIntent,
) error {
	return nil
}

type endpointClock struct{}

func (endpointClock) Now() time.Time { return endpointAt }

func realResultHandler(t *testing.T, configured bool) *application.ReceiveExternalResultHandler {
	t.Helper()
	return application.NewReceiveExternalResultHandler(application.ReceiveExternalResultDeps{
		Results:     &resultStoreDouble{byKey: map[ports.ExternalResultKey]ports.ExternalResultRecord{}},
		Submissions: &submissionIndexDouble{found: true},
		Rules:       &interpretationRuleDouble{configured: configured},
		Downstream:  &resultDownstreamDouble{},
		Clock:       endpointClock{},
	})
}

func resultCommand(t *testing.T) application.ReceiveExternalResultCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.ReceiveExternalResultCommand{
		TenantID:       tenant,
		SourceID:       "carrier-feed/receipt-001",
		Layer:          domain.RegulatoryReceiptLayer,
		Role:           "CARRIER_CHANNEL",
		RawSemantics:   "ACCEPTED_BY_CUSTOMS_GATEWAY",
		ClaimedVersion: "submission-v1",
		Attempt:        1,
		Scope:          "declaration-unit-1",
		OccurredAt:     endpointAt.Add(-time.Hour),
		ReceivedAt:     endpointAt,
	}
}

type resultBody struct {
	Outcome         string `json:"outcome"`
	UndecidedReason string `json:"undecidedReason"`
	Error           *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func postResult(
	t *testing.T,
	intake customshttp.ResultIntake,
	handler customshttp.ResultHandler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := customshttp.NewReceiveExternalResultEndpoint(intake, handler)
	request := httptest.NewRequest(method, "/customs/external-results", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func decodeResult(t *testing.T, response *httptest.ResponseRecorder) resultBody {
	t.Helper()
	var body resultBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

// Covers: ADR-0022 在 UC-CC-006 端点上——首次入册 201（通道不解体即可出队确认）；重放
// 与解释规则未配置（实例半边的未决格带原因）都是答案 200 带 outcome；状态码不区分
// 业务判别。
func TestResultStatusesReportAnswerFormedNotVerdict(t *testing.T) {
	handler := realResultHandler(t, true)
	intake := &resultIntakeDouble{command: resultCommand(t)}

	recorded := postResult(t, intake, handler, http.MethodPost)
	if recorded.Code != http.StatusCreated {
		t.Fatalf("recorded status = %d, want %d", recorded.Code, http.StatusCreated)
	}
	if body := decodeResult(t, recorded); body.Outcome != "RESULT_RECORDED" {
		t.Fatalf("outcome = %q", body.Outcome)
	}

	replayed := postResult(t, intake, handler, http.MethodPost)
	if replayed.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200——重放是答案不是新建", replayed.Code)
	}
	if body := decodeResult(t, replayed); body.Outcome != "EXISTING_RESULT" {
		t.Fatalf("outcome = %q", body.Outcome)
	}

	unconfigured := postResult(t,
		&resultIntakeDouble{command: resultCommand(t)},
		realResultHandler(t, false),
		http.MethodPost,
	)
	if unconfigured.Code != http.StatusOK {
		t.Fatalf("unconfigured status = %d; 未决是答案不是故障", unconfigured.Code)
	}
	body := decodeResult(t, unconfigured)
	if body.Outcome != "RESULT_UNDECIDED" || body.UndecidedReason == "" {
		t.Fatalf("outcome = %q reason = %q", body.Outcome, body.UndecidedReason)
	}
}

// Covers: 传输层分流纪律——4xx 出队交给人、受理依赖不可用 5xx 留队重发、没形成答案
// 5xx、非 POST 405、未命名结果 5xx；错误体只带稳定 code（监管报文原文与租户存在性
// 都不回显）。
func TestResultTransportFailuresSplitByRetryAction(t *testing.T) {
	malformed := postResult(t,
		&resultIntakeDouble{err: fmt.Errorf("%w: bad envelope", customshttp.ErrMalformedRequest)},
		realResultHandler(t, true),
		http.MethodPost,
	)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if body := decodeResult(t, malformed); body.Error == nil || body.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("error body = %+v", body.Error)
	}

	intakeDown := postResult(t,
		&resultIntakeDouble{err: errors.New("channel authenticator unreachable")},
		realResultHandler(t, true),
		http.MethodPost,
	)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d", intakeDown.Code)
	}

	wrongMethod := postResult(t, &resultIntakeDouble{}, realResultHandler(t, true), http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}
