package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

const (
	decisionTenant   = "SYN-TENANT-01"
	decisionOperator = "https://id.syn.example/dex#SYN-OPERATOR-01"
)

type decisionAuthenticator struct {
	err   error
	asked *[]shipmenthttp.OperatorDecision
}

func (fake decisionAuthenticator) AuthenticateOperatorDecision(_ context.Context, token string, decision shipmenthttp.OperatorDecision) (shipmenthttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return shipmenthttp.OperatorIdentity{}, shipmenthttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return shipmenthttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID(decisionTenant)
	return shipmenthttp.OperatorIdentity{Tenant: tenant, Operator: decisionOperator}, err
}

type decisionTargets struct {
	missing bool
	err     error
}

func (fake decisionTargets) FindOperatorDecisionTarget(_ context.Context, tenant domain.TenantID, _ domain.ShipmentRequestID) (ports.OperatorDecisionTarget, bool, error) {
	if fake.err != nil {
		return ports.OperatorDecisionTarget{}, false, fake.err
	}
	if fake.missing {
		return ports.OperatorDecisionTarget{}, false, nil
	}
	account, _ := domain.NewCustomerAccountID("SYN-ACCOUNT-01")
	source, _ := domain.NewSource("SYN-SOURCE")
	key, _ := domain.NewSourceRequestKey("SYN-KEY-01")
	identity, err := domain.NewSourceIdentity(tenant, account, source, key)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, err
	}
	version, err := domain.NewSubmissionVersionID("SYN-VERSION-02")
	return ports.OperatorDecisionTarget{Identity: identity, CurrentVersion: version}, err == nil, err
}

func decisionIntake(t *testing.T, authenticator shipmenthttp.OperatorAuthenticator, targets ports.OperatorDecisionTargets) *shipmenthttp.OperatorDecisionIntake {
	t.Helper()
	intake, err := shipmenthttp.NewOperatorDecisionIntake(authenticator, targets)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func decisionRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/shipment-requests/manual-review-completions", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})

	command, err := intake.IntakeManualReviewCompletion(context.Background(),
		decisionRequest("t", `{"shipmentRequestId": "SYN-REQ-01", "reason": "证件与申报一致"}`))
	if err != nil {
		t.Fatalf("manual review completion: %v", err)
	}
	if command.Identity.TenantID().String() != decisionTenant || command.Identity.CustomerAccountID().String() != "SYN-ACCOUNT-01" ||
		command.ShipmentRequestID.String() != "SYN-REQ-01" || command.SubmissionVersion.String() != "SYN-VERSION-02" {
		t.Fatalf("addressing = %+v", command)
	}
	if command.Reviewer.String() != decisionOperator || command.Evidence.String() != "证件与申报一致" {
		t.Fatalf("reviewer %q, evidence %q", command.Reviewer.String(), command.Evidence.String())
	}
}

func TestActiveRejectionAndDispositionTakeTheDeciderFromTheAuthenticatedOperator(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	ctx := context.Background()

	rejection, err := intake.IntakeActiveRejection(ctx, decisionRequest("t", `{"shipmentRequestId": "SYN-REQ-01", "reason": "SYN-REASON-SANCTIONED"}`))
	if err != nil {
		t.Fatalf("active rejection: %v", err)
	}
	if rejection.Decider.String() != decisionOperator || rejection.Reason.String() != "SYN-REASON-SANCTIONED" ||
		rejection.Evidence.String() != "OPERATOR/"+decisionOperator || rejection.SubmissionVersion.String() != "SYN-VERSION-02" {
		t.Fatalf("rejection = %+v", rejection)
	}

	disposition, err := intake.IntakeAuthorizedDisposition(ctx, decisionRequest("t",
		`{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "choice": "CUSTOMER_SUPPLEMENT", "reason": "SYN-REASON-DOCS"}`))
	if err != nil {
		t.Fatalf("authorized disposition: %v", err)
	}
	if disposition.Disposer.String() != decisionOperator || disposition.Choice != domain.DisposeByCustomerSupplement ||
		disposition.SubmissionVersion.String() != "SYN-VERSION-01" || disposition.Reason.String() != "SYN-REASON-DOCS" ||
		disposition.Evidence.String() != "OPERATOR/"+decisionOperator {
		t.Fatalf("disposition = %+v", disposition)
	}
}

func TestDecisionPayloadsCarryNoIdentityAndUnknownRequestsAnswerLikeAProbe(t *testing.T) {
	ctx := context.Background()
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	for name, body := range map[string]string{
		"customer account smuggled": `{"shipmentRequestId": "SYN-REQ-01", "reason": "r", "customerAccountId": "SYN-ACCOUNT-09"}`,
		"reviewer smuggled":         `{"shipmentRequestId": "SYN-REQ-01", "reason": "r", "reviewer": "someone"}`,
		"request id missing":        `{"reason": "r"}`,
	} {
		if _, err := intake.IntakeManualReviewCompletion(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeAuthorizedDisposition(ctx, decisionRequest("t",
		`{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "choice": "APPROVE", "reason": "r"}`)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("unknown disposition choice: err = %v", err)
	}

	missing := decisionIntake(t, decisionAuthenticator{}, decisionTargets{missing: true})
	if _, err := missing.IntakeActiveRejection(ctx, decisionRequest("t", `{"shipmentRequestId": "SYN-REQ-99", "reason": "r"}`)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Fatalf("unknown shipment request: err = %v, want the same answer as an unauthorized probe", err)
	}
	down := decisionIntake(t, decisionAuthenticator{}, decisionTargets{err: errors.New("connection refused")})
	if _, err := down.IntakeActiveRejection(ctx, decisionRequest("t", `{"shipmentRequestId": "SYN-REQ-01", "reason": "r"}`)); errors.Is(err, shipmenthttp.ErrOperatorNotGranted) || err == nil {
		t.Fatalf("target lookup down: err = %v, want a dependency failure, not a refusal", err)
	}
}

func TestEachPSDecisionFaceAuthenticatesForItsOwnDecision(t *testing.T) {
	var asked []shipmenthttp.OperatorDecision
	intake := decisionIntake(t, decisionAuthenticator{err: shipmenthttp.ErrOperatorNotGranted, asked: &asked}, decisionTargets{})
	ctx := context.Background()
	faces := []struct {
		want   shipmenthttp.OperatorDecision
		intake func() error
	}{
		{shipmenthttp.OperatorDecisionManualReviewCompletion, func() error {
			_, err := intake.IntakeManualReviewCompletion(ctx, decisionRequest("t", `{}`))
			return err
		}},
		{shipmenthttp.OperatorDecisionActiveRejection, func() error {
			_, err := intake.IntakeActiveRejection(ctx, decisionRequest("t", `{}`))
			return err
		}},
		{shipmenthttp.OperatorDecisionAuthorizedDisposition, func() error {
			_, err := intake.IntakeAuthorizedDisposition(ctx, decisionRequest("t", `{}`))
			return err
		}},
	}
	for _, face := range faces {
		asked = nil
		if err := face.intake(); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) || len(asked) != 1 || asked[0] != face.want {
			t.Fatalf("%s: err = %v, asked = %v", face.want, err, asked)
		}
	}
}

type reviewMustNotRun struct{ t *testing.T }

func (handler reviewMustNotRun) Handle(context.Context, application.CompleteManualReviewCommand) (application.CompleteManualReviewResult, error) {
	handler.t.Fatal("orchestration reached although the intake refused")
	return application.CompleteManualReviewResult{}, nil
}

func TestOperatorAnswerGradesOnTheDecisionEndpoints(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		body   string
		status int
		code   string
	}{
		"no bearer token":             {token: "", body: `{}`, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"issuer parameters unset":     {err: shipmenthttp.ErrAccessChannelNotConfigured, token: "t", body: `{}`, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"no grant for this decision":  {err: shipmenthttp.ErrOperatorNotGranted, token: "t", body: `{}`, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"outside the admission scope": {err: shipmenthttp.ErrOutsideAdmissionScope, token: "t", body: `{}`, status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"identity dependency down":    {err: shipmenthttp.ErrIdentityDependencyUnavailable, token: "t", body: `{}`, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"authenticated but malformed": {token: "t", body: `{"shipmentRequestId": 7}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			endpoint := shipmenthttp.NewCompleteManualReviewEndpoint(decisionIntake(t, decisionAuthenticator{err: testCase.err}, decisionTargets{}), reviewMustNotRun{t})
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, decisionRequest(testCase.token, testCase.body))
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
