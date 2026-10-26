package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉 ADR-0303 的线格式半边：七个运营决定口把认证出的操作者（「发行方#sub」，与租户同一份信封）随命令交给编排，
// 载荷里收不进操作者；判断口的回执照 ADR-0187 决定四不回操作证据，查阅面带出记下的那一位。夹具全为合成引用（S 级）。

// Covers: `AT-TF-335`–`AT-TF-341` 线格式半边——七口各自把信封里的操作者交进命令；载荷里自报一位操作者是坏报文（封闭解码），
// 不会顶替信封给的那一位。
func TestEachOperationDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	ctx := context.Background()
	faces := map[string]struct {
		body     string
		operator func(*http.Request) (string, error)
		smuggled string
	}{
		"segment closure": {decisionSegmentClosureBody, func(request *http.Request) (string, error) {
			command, err := intake.IntakeSegmentClosure(ctx, request)
			return command.ClosedBy, err
		}, "closedBy"},
		"participation termination": {`{"segment":"SYN-SEG-01","object":"SYN-OBJ-01","basis":"SYN-DISPOSITION-01","endedAt":"2026-09-25T01:30:00Z"}`,
			func(request *http.Request) (string, error) {
				termination, err := intake.IntakeParticipationTermination(ctx, request)
				return termination.TerminatedBy, err
			}, "terminatedBy"},
		"dispatch task": {decisionDispatchTaskBody, func(request *http.Request) (string, error) {
			command, err := intake.IntakeDispatchTask(ctx, request)
			return command.OpenedBy, err
		}, "openedBy"},
		"load assignment": {`{"assignment":"SYN-LA-01","schedule":"SYN-SCHED-01","members":["SYN-OBJ-01"],"version":"v1","assignedAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeLoadAssignment(ctx, request)
				return command.DecidedBy, err
			}, "decidedBy"},
		"effective time judgment": {decisionEffectiveTimeJudgmentBody, func(request *http.Request) (string, error) {
			command, err := intake.IntakeEffectiveTimeJudgment(ctx, request)
			return command.JudgedBy, err
		}, "judgedBy"},
		"carrier first effective pickup judgment": {decisionCarrierPickupJudgmentBody, func(request *http.Request) (string, error) {
			command, err := intake.IntakeCarrierPickupJudgment(ctx, request)
			return command.JudgedBy, err
		}, "judgedBy"},
		"transport commission submission": {`{"commission":"SYN-COMMISSION-01","provider":"SYN-PARTNER-01","agreement":"SYN-AGREEMENT-01",` +
			`"agreementVersion":"v1","conditions":"SYN-COND","role":"SYN-ROLE","responsibility":"SYN-RESP","members":["SYN-OBJ-01"],` +
			`"submittedAt":"2026-09-25T01:00:00Z"}`, func(request *http.Request) (string, error) {
			command, err := intake.IntakeTransportCommissionSubmission(ctx, request)
			return command.SubmittedBy, err
		}, "submittedBy"},
	}
	for name, face := range faces {
		t.Run(name, func(t *testing.T) {
			operator, err := face.operator(bearerRequest("presented.operator.token", face.body))
			if err != nil || operator != operatorSubject {
				t.Fatalf("命令里的操作者 = %q err=%v，want 信封给的 %q", operator, err, operatorSubject)
			}
			smuggling := `{"` + face.smuggled + `":"https://id.syn.example/dex#SYN-OPERATOR-SELF",` + strings.TrimPrefix(face.body, "{")
			if _, err := face.operator(bearerRequest("presented.operator.token", smuggling)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
				t.Fatalf("载荷自报操作者：err = %v，want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: `AT-TF-339` 线格式半边——判断口回执不带判断人（与加这一格之前逐字节相同）；收寄链查阅面每一版带出记下的判断人，
// 没记下的那一版整键缺席。
func TestTheCarrierPickupChainShowsTheJudgeAndTheJudgmentReceiptDoesNot(t *testing.T) {
	pickups, judge, query := newCarrierPickupHTTPFixture(t, true)
	recorder := postCarrierPickupJudgment(t, judge, scanPickupBody)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "judgedBy") || strings.Contains(recorder.Body.String(), "issuer-1#judge-1") {
		t.Fatalf("判断口回执带了操作证据：%s", recorder.Body.String())
	}
	if len(pickups.records) != 1 || pickups.records[0].JudgedBy.String() != "issuer-1#judge-1" {
		t.Fatalf("登记册上的判断人 = %+v", pickups.records)
	}
	unattributed := pickups.records[0]
	unattributed.Key.Version = mustHTTPValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-LEGACY")
	unattributed.JudgedBy = domain.RegistrantReference{}
	pickups.records = append(pickups.records, unattributed)

	response := httptest.NewRecorder()
	query.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/transport-fulfillment-carrier-first-effective-pickups?object=PCL-1", nil))
	var chain struct {
		Pickups []map[string]any `json:"pickups"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &chain); err != nil || len(chain.Pickups) != 2 {
		t.Fatalf("译链：%v body=%s", err, response.Body.String())
	}
	if chain.Pickups[0]["judgedBy"] != "issuer-1#judge-1" {
		t.Fatalf("查阅面第一版判断人 = %v", chain.Pickups[0]["judgedBy"])
	}
	if _, present := chain.Pickups[1]["judgedBy"]; present {
		t.Fatalf("没记下判断人的那一版应整键缺席：%v", chain.Pickups[1])
	}
}

// Covers: `AT-TF-340` 线格式半边——轨迹事实查阅面带出显式判断那一版的判断人；按规则判断的那一版整键缺席。
func TestTheTrackingFactReviewShowsTheExplicitJudge(t *testing.T) {
	explicit := judgedReviewRow()
	explicit.EffectiveBasis = "JUDGED_EXPLICITLY"
	explicit.EffectiveRule, explicit.EffectiveRuleVersion = "", ""
	explicit.EffectiveJudgedBy = operatorSubject
	reader := &stubFactReview{rows: []ports.ExternalTrackingFactReviewRow{explicit, judgedReviewRow()}}
	response := getFacts(t, tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), reader), "/x?source=aggregator-a&view=current")
	var view struct {
		Facts []map[string]any `json:"facts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || len(view.Facts) != 2 {
		t.Fatalf("译查阅面：%v body=%s", err, response.Body.String())
	}
	if view.Facts[0]["effectiveJudgedBy"] != operatorSubject {
		t.Fatalf("显式判断那一版的判断人 = %v", view.Facts[0]["effectiveJudgedBy"])
	}
	if _, present := view.Facts[1]["effectiveJudgedBy"]; present {
		t.Fatalf("按规则判断的那一版应整键缺席：%v", view.Facts[1])
	}
}

func mustHTTPValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q：%v", raw, err)
	}
	return value
}
