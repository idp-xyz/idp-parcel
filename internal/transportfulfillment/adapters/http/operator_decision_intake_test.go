package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

const operatorTenant = "SYN-TENANT-01"

// authenticatorFake 替身认证方：记下每次被问的决定种类；没带令牌答令牌不过，否则答 err 或租户。
type authenticatorFake struct {
	err   error
	asked *[]tfhttp.OperatorDecision
}

func (fake authenticatorFake) AuthenticateOperatorDecision(_ context.Context, token string, decision tfhttp.OperatorDecision) (domain.TenantID, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return domain.TenantID{}, tfhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID(operatorTenant)
}

func operatorIntake(t *testing.T, authenticator tfhttp.OperatorAuthenticator) *tfhttp.OperatorDecisionIntake {
	t.Helper()
	intake, err := tfhttp.NewOperatorDecisionIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func bearerRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-segment-closures", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

const segmentClosureBody = `{"segment": "SYN-SEGMENT-01", "closedAt": "2026-09-25T02:00:00Z"}`

func TestOperatorDecisionIntakeTakesTheTenantFromTheAuthenticationResult(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})

	command, err := intake.IntakeSegmentClosure(context.Background(), bearerRequest("presented.operator.token", segmentClosureBody))
	if err != nil {
		t.Fatalf("authenticated operator: %v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Segment != "SYN-SEGMENT-01" {
		t.Fatalf("command = tenant %q, segment %q", command.TenantID.String(), command.Segment)
	}
	if _, err := intake.IntakeSegmentClosure(context.Background(), bearerRequest("presented.operator.token",
		`{"segment": "SYN-SEGMENT-01", "tenantId": "SYN-TENANT-02"}`)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("payload smuggling a tenant: err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 每个运营决定口问认证方的是它自己那一种决定，且先认证、后解载荷——载荷坏也交回认证方的答复，授予不过的调用方
// 看不到载荷校验的结果。
func TestEachTFDecisionFaceAuthenticatesForItsOwnDecision(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	intake := operatorIntake(t, authenticatorFake{err: tfhttp.ErrOperatorNotGranted, asked: &asked})
	ctx := context.Background()
	faces := []struct {
		want   tfhttp.OperatorDecision
		intake func() error
	}{
		{tfhttp.OperatorDecisionSegmentClosure, func() error {
			_, err := intake.IntakeSegmentClosure(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionDispatchTaskRegistration, func() error {
			_, err := intake.IntakeDispatchTask(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionLoadAssignment, func() error {
			_, err := intake.IntakeLoadAssignment(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionParticipationTermination, func() error {
			_, err := intake.IntakeParticipationTermination(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionEffectiveTimeJudgment, func() error {
			_, err := intake.IntakeEffectiveTimeJudgment(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionCarrierFirstEffectivePickupJudgment, func() error {
			_, err := intake.IntakeCarrierPickupJudgment(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
		{tfhttp.OperatorDecisionTransportCommissionSubmission, func() error {
			_, err := intake.IntakeTransportCommissionSubmission(ctx, bearerRequest("t", "!!not-json!!"))
			return err
		}},
	}
	for _, face := range faces {
		asked = nil
		if err := face.intake(); !errors.Is(err, tfhttp.ErrOperatorNotGranted) {
			t.Fatalf("%s: err = %v, want the authenticator's answer", face.want, err)
		}
		if len(asked) != 1 || asked[0] != face.want {
			t.Fatalf("face asked for %v, want exactly [%s]", asked, face.want)
		}
	}
}

func TestLoadAssignmentAndParticipationTerminationDecodeTheirPayloadsUnderTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	ctx := context.Background()

	assignment, err := intake.IntakeLoadAssignment(ctx, bearerRequest("t",
		`{"assignment": "SYN-LA-01", "schedule": "SYN-SCHED-01", "members": ["SYN-OBJ-01", "SYN-OBJ-02"], "version": "v1", "assignedAt": "2026-09-25T01:00:00Z"}`))
	if err != nil {
		t.Fatalf("load assignment: %v", err)
	}
	if assignment.TenantID.String() != operatorTenant || assignment.Assignment != "SYN-LA-01" || assignment.Schedule != "SYN-SCHED-01" ||
		len(assignment.Members) != 2 || assignment.Version != "v1" || !assignment.AssignedAt.Equal(time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("load assignment command = %+v", assignment)
	}

	termination, err := intake.IntakeParticipationTermination(ctx, bearerRequest("t",
		`{"segment": "SYN-SEG-01", "object": "SYN-OBJ-01", "basis": "SYN-DISPOSITION-01", "endedAt": "2026-09-25T01:30:00Z"}`))
	if err != nil {
		t.Fatalf("participation termination: %v", err)
	}
	if termination.TenantID.String() != operatorTenant || termination.Segment != "SYN-SEG-01" || termination.Object != "SYN-OBJ-01" ||
		termination.Basis != "SYN-DISPOSITION-01" || !termination.EndedAt.Equal(time.Date(2026, 9, 25, 1, 30, 0, 0, time.UTC)) {
		t.Fatalf("termination command = %+v", termination)
	}

	for name, body := range map[string]string{
		"tenant smuggled":  `{"assignment": "SYN-LA-01", "tenantId": "SYN-TENANT-02"}`,
		"instant garbled":  `{"assignment": "SYN-LA-01", "assignedAt": "yesterday"}`,
		"members not list": `{"assignment": "SYN-LA-01", "members": "SYN-OBJ-01"}`,
	} {
		if _, err := intake.IntakeLoadAssignment(ctx, bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("load assignment, %s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// 以下解码用例证各口线格式译装没换字：期望值取自载荷字面量而不是重算，租户只来自认证结果。

const decisionEffectiveTimeJudgmentBody = `{"fact":"SYN-TRACKING-FACT-08-12","effectiveAt":"2026-09-24T16:00:00+08:00"}`

// Covers: 有效时间判断口的线格式是本包既有的 EffectiveTimeJudgmentPayload（指名的事实与所有者给出的有效时间），租户来自认证结果。
func TestOperatorDecisionIntakeTranslatesEffectiveTimeJudgment(t *testing.T) {
	command, err := operatorIntake(t, authenticatorFake{}).IntakeEffectiveTimeJudgment(context.Background(), bearerRequest("t", decisionEffectiveTimeJudgmentBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Fact != "SYN-TRACKING-FACT-08-12" {
		t.Fatalf("command = %+v，与认证结果与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC); !command.EffectiveAt.Equal(want) {
		t.Fatalf("EffectiveAt = %s, want %s", command.EffectiveAt, want)
	}
}

const decisionSegmentClosureBody = `{"segment":"SYN-SEGMENT-08-11","closedAt":"2026-09-25T20:00:00+08:00"}`

// Covers: SegmentClosureIntake——关段声明是运营决定，段与关段时刻两格逐字来自载荷，租户来自认证结果。
func TestOperatorDecisionIntakeTranslatesSegmentClosure(t *testing.T) {
	command, err := operatorIntake(t, authenticatorFake{}).IntakeSegmentClosure(context.Background(), bearerRequest("t", decisionSegmentClosureBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Segment != "SYN-SEGMENT-08-11" {
		t.Fatalf("command = %+v，与认证结果与载荷不符", command)
	}
	if want := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC); !command.ClosedAt.Equal(want) {
		t.Fatalf("ClosedAt = %s, want %s", command.ClosedAt, want)
	}
}

const decisionDispatchTaskBody = `{"task":"SYN-DISPATCH-08-07","kind":"DELIVERY","objects":["SYN-PARCEL-08-07","SYN-PARCEL-08-08"],` +
	`"place":"SYN-PLACE/consignee-07","windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00",` +
	`"conditions":"SYN-CONDITION/signature-required","openedAt":"2026-09-24T20:00:00+08:00"}`

// Covers: DispatchTaskIntake 契约「工作范围七件全从请求收」——任务、种类、对象范围、地点、时间窗、条件与建立时刻逐字来自载荷；
// 任务种类取 domain.DispatchTaskKind 的封闭词，词表外是用法错误（400）。
func TestOperatorDecisionIntakeTranslatesDispatchTask(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	command, err := intake.IntakeDispatchTask(context.Background(), bearerRequest("t", decisionDispatchTaskBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != operatorTenant {
		t.Fatalf("TenantID = %q, want %q", got, operatorTenant)
	}
	if command.Task != "SYN-DISPATCH-08-07" || command.Kind.String() != "DELIVERY" || len(command.Objects) != 2 ||
		command.Objects[0] != "SYN-PARCEL-08-07" || command.Objects[1] != "SYN-PARCEL-08-08" ||
		command.Place != "SYN-PLACE/consignee-07" || command.Conditions != "SYN-CONDITION/signature-required" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	for name, pair := range map[string][2]time.Time{
		"windowFrom": {command.WindowFrom, time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)},
		"windowTo":   {command.WindowTo, time.Date(2026, 9, 25, 4, 0, 0, 0, time.UTC)},
		"openedAt":   {command.OpenedAt, time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if _, err := intake.IntakeDispatchTask(context.Background(), bearerRequest("t", `{"task":"t","kind":"RETURN"}`)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("词表外任务种类：err = %v, want ErrMalformedRequest", err)
	}
}

const decisionCarrierPickupJudgmentBody = `{"object":"SYN-PARCEL-08-04","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SYN-SCAN-08-04",` +
	`"evidenceVersion":"v1","expressesControl":true,"occurredAt":"2026-09-24T11:00:00+08:00","carrierKind":"EXTERNAL_PARTY",` +
	`"carrierReference":"SYN-PARTY/carrier-08","segment":"SYN-SEGMENT-08-04"}`

// Covers: 承运商判断口的线格式是本包既有的 CarrierPickupJudgmentPayload——读法本体（expressesControl）、证据来源与业务发生
// 时间逐字来自载荷，租户来自认证结果。
func TestOperatorDecisionIntakeTranslatesCarrierPickupJudgment(t *testing.T) {
	command, err := operatorIntake(t, authenticatorFake{}).IntakeCarrierPickupJudgment(context.Background(), bearerRequest("t", decisionCarrierPickupJudgmentBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != operatorTenant {
		t.Fatalf("TenantID = %q, want %q", got, operatorTenant)
	}
	if command.Object != "SYN-PARCEL-08-04" || command.Source.String() != "CARRIER_PICKUP_SCAN" ||
		command.EvidenceReference != "SYN-SCAN-08-04" || command.EvidenceVersion != "v1" || !command.ExpressesControl ||
		command.SubjectKind.String() != "EXTERNAL_PARTY" || command.SubjectReference != "SYN-PARTY/carrier-08" ||
		command.Segment != "SYN-SEGMENT-08-04" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
}

// decisionLines 是 OperatorDecisionIntake 实现的各运营决定口：怎么调它的 Intake、一份合规载荷。自报租户与封闭解码两条对每一口
// 都成立，表驱动逐口跑。
var decisionLines = map[string]struct {
	intake func(*tfhttp.OperatorDecisionIntake, *http.Request) error
	valid  string
}{
	"/transport-fulfillment-segment-closures": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeSegmentClosure(context.Background(), request)
		return err
	}, decisionSegmentClosureBody},
	"/transport-fulfillment-dispatch-task-registrations": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeDispatchTask(context.Background(), request)
		return err
	}, decisionDispatchTaskBody},
	"/transport-fulfillment-effective-time-judgments": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeEffectiveTimeJudgment(context.Background(), request)
		return err
	}, decisionEffectiveTimeJudgmentBody},
	"/transport-fulfillment-carrier-first-effective-pickup-judgments": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeCarrierPickupJudgment(context.Background(), request)
		return err
	}, decisionCarrierPickupJudgmentBody},
	"/transport-fulfillment-load-assignment-registrations": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeLoadAssignment(context.Background(), request)
		return err
	}, `{"assignment":"SYN-LA-01","schedule":"SYN-SCHED-01","members":["SYN-OBJ-01"],"version":"v1","assignedAt":"2026-09-25T01:00:00Z"}`},
	"/transport-fulfillment-participation-terminations": {func(intake *tfhttp.OperatorDecisionIntake, request *http.Request) error {
		_, err := intake.IntakeParticipationTermination(context.Background(), request)
		return err
	}, `{"segment":"SYN-SEG-01","object":"SYN-OBJ-01","basis":"SYN-DISPOSITION-01","endedAt":"2026-09-25T01:30:00Z"}`},
}

// Covers: 载荷里带 tenantId 即拒（MALFORMED_REQUEST），值与认证出的租户相同也拒；一份载荷只许一个文档、只许认识的键——尾随的
// 第二个 JSON 值与未知键都拒，放过它们就是无声丢掉一段输入。每一口都一样。
func TestEveryDecisionFaceRefusesSelfReportedTenantTrailingContentAndUnknownKeys(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	for pattern, line := range decisionLines {
		t.Run(pattern, func(t *testing.T) {
			if err := line.intake(intake, bearerRequest("t", line.valid)); err != nil {
				t.Fatalf("合规载荷被拒：%v", err)
			}
			for name, body := range map[string]string{
				"自报租户":    `{"tenantId":"` + operatorTenant + `",` + strings.TrimPrefix(line.valid, "{"),
				"尾随内容":    line.valid + ` {"x":1}`,
				"未知键":     `{"unknownKey":"x",` + strings.TrimPrefix(line.valid, "{"),
				"不是 JSON": "!!not-json!!",
				"空载荷":     "",
			} {
				if err := line.intake(intake, bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
					t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
				}
			}
		})
	}
}

type closerMustNotRun struct{ t *testing.T }

func (closer closerMustNotRun) Close(context.Context, application.CloseFulfillmentSegmentCommand) (application.CloseFulfillmentSegmentResult, error) {
	closer.t.Fatal("orchestration reached although the intake refused")
	return application.CloseFulfillmentSegmentResult{}, nil
}

func TestOperatorAnswerGradesOnTheCommandEndpoint(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		body   string
		status int
		code   string
	}{
		"no bearer token":             {token: "", body: segmentClosureBody, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"issuer parameters unset":     {err: tfhttp.ErrAccessChannelNotConfigured, token: "t", body: segmentClosureBody, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"no grant for this decision":  {err: tfhttp.ErrOperatorNotGranted, token: "t", body: segmentClosureBody, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"outside the admission scope": {err: tfhttp.ErrOutsideAdmissionScope, token: "t", body: segmentClosureBody, status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"identity dependency down":    {err: tfhttp.ErrIdentityDependencyUnavailable, token: "t", body: segmentClosureBody, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"authenticated but malformed": {token: "t", body: `{"segment": 7}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			endpoint := tfhttp.NewCloseFulfillmentSegmentEndpoint(operatorIntake(t, authenticatorFake{err: testCase.err}), closerMustNotRun{t})
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, bearerRequest(testCase.token, testCase.body))
			var problem struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
			if recorder.Code != testCase.status || problem.Error.Code != testCase.code {
				t.Fatalf("answer = %d %q, want %d %q", recorder.Code, problem.Error.Code, testCase.status, testCase.code)
			}
		})
	}
}
