package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	return decisionIntakeWithParcels(t, authenticator, targets, parcelTargets{})
}

func decisionIntakeWithParcels(t *testing.T, authenticator shipmenthttp.OperatorAuthenticator, targets ports.OperatorDecisionTargets, parcels ports.CurrentAcceptedParcelTargetView) *shipmenthttp.OperatorDecisionIntake {
	t.Helper()
	intake, err := shipmenthttp.NewOperatorDecisionIntake(authenticator, targets, parcels)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// parcelTargets 是按包裹反查当前已接受委托的替身：查得到时交回 SYN-ACCOUNT-01 名下的那份委托，并记下被问的租户与包裹。
type parcelTargets struct {
	missing bool
	err     error
	asked   *[]string
}

func (fake parcelTargets) FindCurrentAcceptedByParcel(_ context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID) (domain.CurrentAcceptedParcelTarget, bool, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, tenant.String()+"|"+parcel.String())
	}
	if fake.err != nil {
		return domain.CurrentAcceptedParcelTarget{}, false, fake.err
	}
	if fake.missing {
		return domain.CurrentAcceptedParcelTarget{}, false, nil
	}
	account, _ := domain.NewCustomerAccountID("SYN-ACCOUNT-01")
	source, _ := domain.NewSource("SYN-SOURCE")
	key, _ := domain.NewSourceRequestKey("SYN-KEY-01")
	identity, err := domain.NewSourceIdentity(tenant, account, source, key)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, false, err
	}
	requestID, _ := domain.NewShipmentRequestID("SYN-REQ-01")
	version, _ := domain.NewSubmissionVersionID("SYN-VERSION-02")
	target, err := domain.NewCurrentAcceptedParcelTarget(identity, requestID, version)
	return target, err == nil, err
}

// Covers: ADR-0183 决定一、二、三——受控关闭按包裹寻址、委托来源身份由服务端反查；货主指令的关闭，请求方与责任主体都取那份
// 委托的客户账户，载荷给不了；运营操作与外部限制的关闭没有请求方，责任主体由载荷给、必填。货主指令那份指令的证据随载荷进
// 命令（ADR-0187 决定一、五）；提交操作者只作操作证据，取认证出的操作者（ADR-0187 决定三）。
func TestControlledClosureTakesTheShipperFromTheAcceptedRequestOfTheParcel(t *testing.T) {
	var asked []string
	intake := decisionIntakeWithParcels(t, decisionAuthenticator{}, decisionTargets{}, parcelTargets{asked: &asked})
	ctx := context.Background()

	byShipper, err := intake.IntakeControlledClosure(ctx, decisionRequest("t",
		`{"parcel": "SYN-PARCEL-01", "reason": "SYN-REASON-SHIPPER-HOLD", "effectiveAt": "2026-10-16T09:00:00+08:00", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "SYN-CALL-01"}`))
	if err != nil {
		t.Fatalf("closure on shipper instruction: %v", err)
	}
	if len(asked) != 1 || asked[0] != decisionTenant+"|SYN-PARCEL-01" {
		t.Fatalf("parcel lookup asked %v, want the envelope tenant and the named parcel", asked)
	}
	if byShipper.Identity.CustomerAccountID().String() != "SYN-ACCOUNT-01" || byShipper.Parcel.String() != "SYN-PARCEL-01" ||
		byShipper.Requester.String() != "CUSTOMER-ACCOUNT/SYN-ACCOUNT-01" ||
		byShipper.ResponsibilitySourceKind != application.ShipperInstructionResponsibilitySource ||
		byShipper.ResponsibilitySourceSubject != "SYN-ACCOUNT-01" ||
		byShipper.Reason.String() != "SYN-REASON-SHIPPER-HOLD" || !byShipper.EffectiveAt.Equal(time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("closure on shipper instruction = %+v", byShipper)
	}
	if byShipper.RequestEvidence != "SYN-CALL-01" || byShipper.OperationEvidence != "OPERATOR/"+decisionOperator {
		t.Fatalf("closure evidences = %q / %q", byShipper.RequestEvidence, byShipper.OperationEvidence)
	}

	for source, subject := range map[string]string{"OPERATOR-ACTION": "SYN-OPS-CASE-01", "EXTERNAL-RESTRICTION": "SYN-RESTRICTION/embargo-01"} {
		closure, err := intake.IntakeControlledClosure(ctx, decisionRequest("t",
			`{"parcel": "SYN-PARCEL-01", "reason": "SYN-REASON-HOLD", "effectiveAt": "2026-10-16T01:00:00Z", "responsibilitySource": "`+source+`", "responsibilitySubject": "`+subject+`"}`))
		if err != nil {
			t.Fatalf("%s closure: %v", source, err)
		}
		if closure.Requester.String() != "" || closure.RequestEvidence != "" || closure.OperationEvidence != "OPERATOR/"+decisionOperator ||
			closure.ResponsibilitySourceKind.String() != source || closure.ResponsibilitySourceSubject != subject {
			t.Fatalf("%s closure = requester %q, source %s/%q", source, closure.Requester.String(), closure.ResponsibilitySourceKind, closure.ResponsibilitySourceSubject)
		}
	}

	for name, body := range map[string]string{
		"shipper subject given twice":   `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "responsibilitySubject": "SYN-ACCOUNT-09"}`,
		"operator action without topic": `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "OPERATOR-ACTION"}`,
		"unknown responsibility source": `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "CUSTOMER-WHIM", "responsibilitySubject": "x"}`,
		"requester smuggled":            `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "requester": "CUSTOMER-ACCOUNT/SYN-ACCOUNT-09"}`,
		"request id smuggled":           `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "shipmentRequestId": "SYN-REQ-09"}`,
		"tenant smuggled":               `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "tenantId": "SYN-TENANT-02"}`,
		"operation evidence smuggled":   `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "operationEvidence": "OPERATOR/SYN-OTHER"}`,
		"request evidence smuggled":     `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "requestEvidence": "SYN-OTHER"}`,
		"instant garbled":               `{"parcel": "SYN-PARCEL-01", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e", "effectiveAt": "tomorrow"}`,
		"parcel missing":                `{"reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "e"}`,
	} {
		if _, err := intake.IntakeControlledClosure(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// Covers: ADR-0183 决定四——重开的货主账户恒取包裹所属委托的客户账户；载荷带了货主重授权证据，请求方同取它，没带则请求方缺席。
// 核「原关闭因货主指令形成时要同一账户的新授权」那道门仍在编排，本 Intake 不预判。
func TestReopeningTakesTheShipperAccountFromTheAcceptedRequestOfTheParcel(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	ctx := context.Background()

	reauthorized, err := intake.IntakeReopening(ctx, decisionRequest("t",
		`{"parcel": "SYN-PARCEL-01", "reason": "SYN-REASON-RESUME", "effectiveAt": "2026-10-17T01:00:00Z", "relatedPriorClosure": "SYN-CAD-01", "shipperAuthorizationEvidence": "SYN-EVIDENCE/shipper-letter-02"}`))
	if err != nil {
		t.Fatalf("reopening with shipper reauthorization: %v", err)
	}
	if reauthorized.Identity.CustomerAccountID().String() != "SYN-ACCOUNT-01" || reauthorized.Parcel.String() != "SYN-PARCEL-01" ||
		reauthorized.ShipperAccount.String() != "CUSTOMER-ACCOUNT/SYN-ACCOUNT-01" || reauthorized.Requester.String() != "CUSTOMER-ACCOUNT/SYN-ACCOUNT-01" ||
		reauthorized.ShipperAuthorizationEvidence != "SYN-EVIDENCE/shipper-letter-02" || reauthorized.RelatedPriorClosure.String() != "SYN-CAD-01" ||
		reauthorized.Reason.String() != "SYN-REASON-RESUME" || !reauthorized.EffectiveAt.Equal(time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("reopening with shipper reauthorization = %+v", reauthorized)
	}
	if reauthorized.OperationEvidence != "OPERATOR/"+decisionOperator {
		t.Fatalf("reopening operation evidence = %q", reauthorized.OperationEvidence)
	}

	byOperator, err := intake.IntakeReopening(ctx, decisionRequest("t",
		`{"parcel": "SYN-PARCEL-01", "reason": "SYN-REASON-RESUME", "effectiveAt": "2026-10-17T01:00:00Z", "relatedPriorClosure": "SYN-CAD-01"}`))
	if err != nil {
		t.Fatalf("reopening without shipper reauthorization: %v", err)
	}
	if byOperator.ShipperAccount.String() != "CUSTOMER-ACCOUNT/SYN-ACCOUNT-01" || byOperator.Requester.String() != "" || byOperator.ShipperAuthorizationEvidence != "" ||
		byOperator.OperationEvidence != "OPERATOR/"+decisionOperator {
		t.Fatalf("reopening without shipper reauthorization = %+v", byOperator)
	}

	for name, body := range map[string]string{
		"shipper account smuggled":    `{"parcel": "SYN-PARCEL-01", "reason": "r", "relatedPriorClosure": "SYN-CAD-01", "shipperAccount": "CUSTOMER-ACCOUNT/SYN-ACCOUNT-09"}`,
		"requester smuggled":          `{"parcel": "SYN-PARCEL-01", "reason": "r", "relatedPriorClosure": "SYN-CAD-01", "requester": "CUSTOMER-ACCOUNT/SYN-ACCOUNT-09"}`,
		"operation evidence smuggled": `{"parcel": "SYN-PARCEL-01", "reason": "r", "relatedPriorClosure": "SYN-CAD-01", "operationEvidence": "OPERATOR/SYN-OTHER"}`,
		"parcel missing":              `{"reason": "r", "relatedPriorClosure": "SYN-CAD-01"}`,
		"trailing content":            `{"parcel": "SYN-PARCEL-01", "reason": "r", "relatedPriorClosure": "SYN-CAD-01"} {"x": 1}`,
	} {
		if _, err := intake.IntakeReopening(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// Covers: ADR-0183 决定一——包裹查不到当前已接受委托与越权探针同答；反查读不动是依赖故障，不是拒绝。
func TestClosureAndReopeningOnAnUnknownParcelAnswerLikeAProbe(t *testing.T) {
	ctx := context.Background()
	const closure = `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "SYN-CALL-99"}`
	const reopening = `{"parcel": "SYN-PARCEL-99", "reason": "r", "relatedPriorClosure": "SYN-CAD-01"}`

	missing := decisionIntakeWithParcels(t, decisionAuthenticator{}, decisionTargets{}, parcelTargets{missing: true})
	if _, err := missing.IntakeControlledClosure(ctx, decisionRequest("t", closure)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Fatalf("closure on an unknown parcel: err = %v, want the same answer as an unauthorized probe", err)
	}
	if _, err := missing.IntakeReopening(ctx, decisionRequest("t", reopening)); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Fatalf("reopening on an unknown parcel: err = %v, want the same answer as an unauthorized probe", err)
	}
	ambiguous := decisionIntakeWithParcels(t, decisionAuthenticator{}, decisionTargets{}, parcelTargets{err: domain.ErrAmbiguousParcelTarget})
	if _, err := ambiguous.IntakeControlledClosure(ctx, decisionRequest("t", closure)); err == nil || errors.Is(err, shipmenthttp.ErrOperatorNotGranted) || errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("ambiguous parcel: err = %v, want a server-side failure, never picking one", err)
	}
}

func decisionRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/shipment-requests/manual-review-completions", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: 票 operator-channel/34——复核完成的提交版本取载荷指名的那一份，不由服务端当前版本顶替：替身目标的当前版是
// SYN-VERSION-02，载荷指名 SYN-VERSION-01，命令必须签给 SYN-VERSION-01，`版本已换代`那一格才在真渠道上走得到
// （ADR-0132 决定二、ADR-0045 决定三）。来源身份照旧按委托寻址取服务端那一份，复核人取认证出的操作者。
func TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})

	command, err := intake.IntakeManualReviewCompletion(context.Background(),
		decisionRequest("t", `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "reason": "证件与申报一致"}`))
	if err != nil {
		t.Fatalf("manual review completion: %v", err)
	}
	if command.Identity.TenantID().String() != decisionTenant || command.Identity.CustomerAccountID().String() != "SYN-ACCOUNT-01" ||
		command.ShipmentRequestID.String() != "SYN-REQ-01" {
		t.Fatalf("addressing = %+v", command)
	}
	if command.SubmissionVersion.String() != "SYN-VERSION-01" {
		t.Fatalf("submission version = %q, want the one the payload names (SYN-VERSION-01), never the server's current SYN-VERSION-02",
			command.SubmissionVersion.String())
	}
	if command.Reviewer.String() != decisionOperator || command.Evidence.String() != "证件与申报一致" {
		t.Fatalf("reviewer %q, evidence %q", command.Reviewer.String(), command.Evidence.String())
	}
}

// Covers: 票 operator-channel/34——版本是复核的对象，缺了或立不起来是坏报文；版本与理由两道纯载荷校验排在委托寻址之前，
// 委托查不到时也答 400，不先落进「与越权探针同答」那一格（写法同授权处置口）。
func TestAManualReviewCompletionWithoutItsVersionAnswers400EvenWhenTheRequestIsUnknown(t *testing.T) {
	ctx := context.Background()
	for name, targets := range map[string]decisionTargets{"known request": {}, "unknown request": {missing: true}} {
		intake := decisionIntake(t, decisionAuthenticator{}, targets)
		for why, body := range map[string]string{
			"version missing":      `{"shipmentRequestId": "SYN-REQ-01", "reason": "r"}`,
			"version blank":        `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "   ", "reason": "r"}`,
			"version not a string": `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": 1, "reason": "r"}`,
			"reason missing":       `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01"}`,
			"reason blank":         `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "reason": " "}`,
		} {
			if _, err := intake.IntakeManualReviewCompletion(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
				t.Fatalf("%s, %s: err = %v, want ErrMalformedRequest", name, why, err)
			}
		}
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
		"customer account smuggled": `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "reason": "r", "customerAccountId": "SYN-ACCOUNT-09"}`,
		"reviewer smuggled":         `{"shipmentRequestId": "SYN-REQ-01", "submissionVersionId": "SYN-VERSION-01", "reason": "r", "reviewer": "someone"}`,
		"request id missing":        `{"submissionVersionId": "SYN-VERSION-01", "reason": "r"}`,
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
		{shipmenthttp.OperatorDecisionControlledClosure, func() error {
			_, err := intake.IntakeControlledClosure(ctx, decisionRequest("t", `{}`))
			return err
		}},
		{shipmenthttp.OperatorDecisionControlledReopening, func() error {
			_, err := intake.IntakeReopening(ctx, decisionRequest("t", `{}`))
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

// Covers: ADR-0183 决定三与票 operator-channel/28「一并看」——两道纯载荷的责任主体校验先于按包裹寻址：坏载荷在包裹查不到时
// 也答 400，不先落到「与越权探针同答」那一格；全空白的主体与没给同读。ADR-0187 决定五的货主指令证据两道校验同样只看载荷、
// 同样先于寻址：货主指令没带或全空白、另两种来源带了，都答 400。
func TestABadClosurePayloadAnswers400EvenWhenTheParcelIsUnknown(t *testing.T) {
	ctx := context.Background()
	missing := decisionIntakeWithParcels(t, decisionAuthenticator{}, decisionTargets{}, parcelTargets{missing: true})
	for name, body := range map[string]string{
		"shipper instruction with a subject":         `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "responsibilitySubject": "SYN-OTHER"}`,
		"operator action without a subject":          `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "OPERATOR-ACTION"}`,
		"operator action with a blank one":           `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "OPERATOR-ACTION", "responsibilitySubject": "   "}`,
		"shipper instruction without evidence":       `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION"}`,
		"shipper instruction with blank evidence":    `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "SHIPPER-INSTRUCTION", "shipperInstructionEvidence": "   "}`,
		"operator action with shipper evidence":      `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "OPERATOR-ACTION", "responsibilitySubject": "SYN-OPS-CASE-01", "shipperInstructionEvidence": "SYN-CALL-99"}`,
		"external restriction with shipper evidence": `{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "EXTERNAL-RESTRICTION", "responsibilitySubject": "SYN-RESTRICTION-01", "shipperInstructionEvidence": "SYN-CALL-99"}`,
	} {
		if _, err := missing.IntakeControlledClosure(ctx, decisionRequest("t", body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// Covers: ADR-0183 决定四「没带时请求方缺席」与票 operator-channel/28「一并看」——全空白的货主重授权证据与没带同读，和编排
// 那道门同一口径；否则请求方被记成客户，门却当作没带证据。
func TestABlankShipperAuthorizationEvidenceLeavesTheRequesterAbsent(t *testing.T) {
	intake := decisionIntake(t, decisionAuthenticator{}, decisionTargets{})
	blank, err := intake.IntakeReopening(context.Background(), decisionRequest("t",
		`{"parcel": "SYN-PARCEL-01", "reason": "SYN-REASON-RESUME", "effectiveAt": "2026-10-17T01:00:00Z", "relatedPriorClosure": "SYN-CAD-01", "shipperAuthorizationEvidence": "   "}`))
	if err != nil {
		t.Fatalf("reopening with blank evidence: %v", err)
	}
	if blank.Requester.String() != "" {
		t.Fatalf("requester = %q, want absent", blank.Requester.String())
	}
}

// Covers: 票 operator-channel/37——认不出的`关闭责任来源`种类词是坏报文：答 400、报文原样带出那个词，排在按包裹寻址之前，
// 包裹查不到时也不落进「与越权探针同答」那一格；认词交给 application，本口只把认不出译成坏报文。
func TestAnUnrecognisedResponsibilitySourceWordAnswers400CarryingTheWord(t *testing.T) {
	ctx := context.Background()
	missing := decisionIntakeWithParcels(t, decisionAuthenticator{}, decisionTargets{}, parcelTargets{missing: true})
	for _, word := range []string{"", "CUSTOMER-WHIM", "shipper-instruction", "SHIPPER-INSTRUCTION/SYN-ACCOUNT-01"} {
		_, err := missing.IntakeControlledClosure(ctx, decisionRequest("t",
			`{"parcel": "SYN-PARCEL-99", "reason": "r", "responsibilitySource": "`+word+`", "responsibilitySubject": "x"}`))
		if !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Fatalf("%q: err = %v, want ErrMalformedRequest", word, err)
		}
		if quoted, _ := json.Marshal(word); !strings.Contains(err.Error(), string(quoted)) {
			t.Fatalf("%q: err = %v, want the unrecognised word quoted", word, err)
		}
	}
}
