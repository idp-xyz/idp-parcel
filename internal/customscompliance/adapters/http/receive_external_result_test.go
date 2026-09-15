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
	_ domain.RegulatoryJurisdictionReference,
	_ time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	if !double.configured {
		return domain.InterpretationRuleReference{}, false, nil
	}
	rule, err := domain.NewInterpretationRuleReference("INTERPRET/US-IMPORT/receipt-v1")
	return rule, true, err
}

// unitStoreDouble 与 caseStoreDouble 支起辖区回指链（范围→单元→案件），命令的范围
// declaration-unit-1 由此解析到带辖区的案件。
type unitStoreDouble struct{}

func (unitStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclarationUnit,
	_ time.Time,
) (ports.DeclarationUnitSaveOutcome, error) {
	return ports.DeclarationUnitSaved, nil
}

func (unitStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	unitID domain.DeclarationUnitID,
) (domain.DeclarationUnit, bool, error) {
	caseID, err := domain.NewCustomsCaseID("case-1")
	if err != nil {
		return domain.DeclarationUnit{}, false, err
	}
	procedure, err := domain.NewCustomsProcedureReference("procedure-1")
	if err != nil {
		return domain.DeclarationUnit{}, false, err
	}
	parcel, err := domain.NewDeclaredParcelReference("parcel-1")
	if err != nil {
		return domain.DeclarationUnit{}, false, err
	}
	unit, err := domain.FormDeclarationUnit(unitID, caseID, procedure, []domain.DeclaredParcelReference{parcel})
	if err != nil {
		return domain.DeclarationUnit{}, false, err
	}
	return unit, true, nil
}

type caseStoreDouble struct{}

func (caseStoreDouble) FindByKey(
	_ context.Context,
	_ ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	return domain.CustomsCase{}, false, nil
}

func (caseStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	caseID domain.CustomsCaseID,
) (domain.CustomsCase, bool, error) {
	jurisdiction, err := domain.NewRegulatoryJurisdictionReference("jurisdiction-1")
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	procedure, err := domain.NewCustomsProcedureReference("procedure-1")
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	obligation, err := domain.NewObligationScopeReference("obligation-1")
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	customsCase, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:           caseID,
		Jurisdiction: jurisdiction,
		Direction:    domain.ImportManifest,
		Procedure:    procedure,
		Obligation:   obligation,
		Parcels: []domain.CaseParcelAssociation{{
			Parcel: "parcel-1", Customer: "customer-1", SourceRef: "source-ref-1",
		}},
		EstablishedAt: endpointAt.Add(-24 * time.Hour),
	})
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	return customsCase, true, nil
}

func (caseStoreDouble) Save(
	_ context.Context,
	_ ports.CustomsCaseKey,
	_ domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	return ports.CustomsCaseSaved, nil
}

type resultDownstreamDouble struct{ err error }

func (double *resultDownstreamDouble) HandOffExternalResult(
	_ context.Context,
	_ ports.ExternalResultHandoffIntent,
) error {
	return double.err
}

// Covers: 票 sa-cc/34 裁决 4 的端点半边——交接信封被框架确定性拒收时编排返 ErrExternalResultHandoffRejected，端点映成
// 4xx（400 HANDOFF_ENVELOPE_REJECTED）出队交给人：重发同样的内容不会改变结果，与 MALFORMED_REQUEST 同一条分流纪律、
// 同一个状态码——ADR-0022 否决了「按语义贴近码（409 / 422）」那条逐例裁量口，差别只进响应体的 code；
// 不再是 5xx NO_ANSWER_FORMED 留队重发，也不再 201 带 handoffReference 让通道方重试到死。
func TestARejectedHandoffEnvelopeIsAFourHundredNotARetry(t *testing.T) {
	handler := application.NewReceiveExternalResultHandler(application.ReceiveExternalResultDeps{
		Results:     &resultStoreDouble{byKey: map[ports.ExternalResultKey]ports.ExternalResultRecord{}},
		Submissions: &submissionIndexDouble{found: true},
		Rules:       &interpretationRuleDouble{configured: true},
		Units:       unitStoreDouble{},
		Cases:       caseStoreDouble{},
		Downstream:  &resultDownstreamDouble{err: fmt.Errorf("%w: partition key exceeds 512 bytes", ports.ErrHandoffEnvelopeRejected)},
		Clock:       endpointClock{},
	})
	rejected := postResult(t, &resultIntakeDouble{command: resultCommand(t)}, handler, http.MethodPost)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("rejected handoff status = %d, want %d", rejected.Code, http.StatusBadRequest)
	}
	if body := decodeResult(t, rejected); body.Error == nil || body.Error.Code != "HANDOFF_ENVELOPE_REJECTED" {
		t.Fatalf("error body = %+v", body.Error)
	}
}

type endpointClock struct{}

func (endpointClock) Now() time.Time { return endpointAt }

func realResultHandler(t *testing.T, configured bool) *application.ReceiveExternalResultHandler {
	t.Helper()
	return application.NewReceiveExternalResultHandler(application.ReceiveExternalResultDeps{
		Results:     &resultStoreDouble{byKey: map[ports.ExternalResultKey]ports.ExternalResultRecord{}},
		Submissions: &submissionIndexDouble{found: true},
		Rules:       &interpretationRuleDouble{configured: configured},
		Units:       unitStoreDouble{},
		Cases:       caseStoreDouble{},
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
