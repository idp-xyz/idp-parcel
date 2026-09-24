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

func TestEachTFDecisionFaceAuthenticatesForItsOwnDecision(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	intake := operatorIntake(t, authenticatorFake{err: tfhttp.ErrOperatorNotGranted, asked: &asked})
	ctx := context.Background()
	faces := []struct {
		want   tfhttp.OperatorDecision
		intake func() error
	}{
		{tfhttp.OperatorDecisionSegmentClosure, func() error { _, err := intake.IntakeSegmentClosure(ctx, bearerRequest("t", `{}`)); return err }},
		{tfhttp.OperatorDecisionDispatchTaskRegistration, func() error { _, err := intake.IntakeDispatchTask(ctx, bearerRequest("t", `{}`)); return err }},
		{tfhttp.OperatorDecisionLoadAssignment, func() error { _, err := intake.IntakeLoadAssignment(ctx, bearerRequest("t", `{}`)); return err }},
		{tfhttp.OperatorDecisionParticipationTermination, func() error {
			_, err := intake.IntakeParticipationTermination(ctx, bearerRequest("t", `{}`))
			return err
		}},
		{tfhttp.OperatorDecisionEffectiveTimeJudgment, func() error { _, err := intake.IntakeEffectiveTimeJudgment(ctx, bearerRequest("t", `{}`)); return err }},
		{tfhttp.OperatorDecisionCarrierFirstEffectivePickupJudgment, func() error {
			_, err := intake.IntakeCarrierPickupJudgment(ctx, bearerRequest("t", `{}`))
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
