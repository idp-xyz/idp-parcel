package customshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

type capturingGateVerifier struct {
	command application.VerifyReleaseGateCommand
	calls   int
}

func (fake *capturingGateVerifier) Handle(_ context.Context, command application.VerifyReleaseGateCommand) (application.VerifyReleaseGateResult, error) {
	fake.command = command
	fake.calls++
	return application.VerifyReleaseGateResult{}, nil
}

// Covers: UC-CC-009 门禁核对（Intake 半边，票 launch-wiring/03）——核对口按放行门禁核对这一种运营决定认证；租户取认证结果，载荷里只有申报范围、
// 拟执行动作与监管边界三格；载荷是封闭的（不收租户），动作不在封闭四值里是坏报文，编排一次也不被调到。
func TestTheReleaseGateVerificationFaceTakesTenantFromTheEnvelopeAndRefusesAnythingElse(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &capturingGateVerifier{}
	customshttp.NewVerifyReleaseGateEndpoint(intake, verifier).ServeHTTP(httptest.NewRecorder(),
		post("/customs-release-gate-verifications", `{"scope":"SCOPE-1","action":"OUTBOUND_RELEASE","boundary":"PROC-1"}`))
	if len(asked) != 1 || asked[0] != "RELEASE_GATE_VERIFICATION" {
		t.Fatalf("asked = %v", asked)
	}
	command := verifier.command
	if command.TenantID.String() != "SYN-TENANT-01" || command.Scope.String() != "SCOPE-1" ||
		command.Action != domain.OutboundRelease || command.Boundary.String() != "PROC-1" {
		t.Fatalf("command = %+v", command)
	}

	refused := &capturingGateVerifier{}
	endpoint := customshttp.NewVerifyReleaseGateEndpoint(intake, refused)
	for name, body := range map[string]string{
		"带租户":      `{"tenantId":"T","scope":"S"}`,
		"动作不在封闭集合": `{"scope":"S","action":"RECEIVE","boundary":"P"}`,
		"多余内容":     `{"scope":"S"} {}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/customs-release-gate-verifications", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d，want 400", name, response.Code)
		}
	}
	if refused.calls != 0 {
		t.Fatalf("坏报文把编排调了 %d 次", refused.calls)
	}
}

// 读数挂在判断上才进响应。没挂时省略这一键：空对象会让调用方分不出「这一道没判」和「判了但字段是空的」。
func TestTheReleaseGateResponseIncludesWeightDiscrepancyOnlyWhenAttached(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	waived := httptest.NewRecorder()
	customshttp.NewVerifyReleaseGateEndpoint(intake, releaseGateHandler(t, false)).ServeHTTP(waived,
		post("/customs-release-gate-verifications", `{"scope":"declaration-unit-1","action":"OUTBOUND_RELEASE","boundary":"SYN-PROC-IMPORT"}`))
	waivedBody := decodeGateResponse(t, waived)
	if waived.Code != http.StatusCreated || waivedBody["outcome"] != "RECORDED" || waivedBody["conclusion"] != "NOT_APPLICABLE" {
		t.Fatalf("不构成前置条件 = %d %v", waived.Code, waivedBody)
	}
	if _, present := waivedBody["weightDiscrepancy"]; present {
		t.Fatalf("没挂读数却带了 weightDiscrepancy：%v", waivedBody["weightDiscrepancy"])
	}

	attached := httptest.NewRecorder()
	customshttp.NewVerifyReleaseGateEndpoint(intake, releaseGateHandler(t, true)).ServeHTTP(attached,
		post("/customs-release-gate-verifications", `{"scope":"declaration-unit-1","action":"OUTBOUND_RELEASE","boundary":"SYN-PROC-IMPORT"}`))
	attachedBody := decodeGateResponse(t, attached)
	if attached.Code != http.StatusCreated || attachedBody["outcome"] != "RECORDED" || attachedBody["conclusion"] != "UNMET" {
		t.Fatalf("挂了读数 = %d %v", attached.Code, attachedBody)
	}
	assertWeightDiscrepancyJSON(t, attachedBody)
}

func decodeGateResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("答复不是 JSON：%s", response.Body.String())
	}
	return body
}

func assertWeightDiscrepancyJSON(t *testing.T, body map[string]any) {
	t.Helper()
	weight, ok := body["weightDiscrepancy"].(map[string]any)
	if !ok {
		t.Fatalf("没有 weightDiscrepancy：%v", body)
	}
	if weight["state"] != "UNMET" || weight["submissionVersion"] != "submission-wd-1" ||
		weight["maxDelta"] != "0.5" || weight["unit"] != "KG" {
		t.Fatalf("读数头 = %v", weight)
	}
	members, ok := weight["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("成员 = %v", weight["members"])
	}
	member, ok := members[0].(map[string]any)
	if !ok {
		t.Fatalf("成员不是对象：%v", members[0])
	}
	if member["parcel"] != "parcel-a" ||
		member["declaredWeight"] != "2.60" || member["declaredUnit"] != "KG" || member["declaredSource"] != "WEIGH-0A" ||
		member["measuredWeight"] != "1.80" || member["measuredUnit"] != "KG" || member["measurementSource"] != "WEIGH-1" ||
		member["withinTolerance"] != false {
		t.Fatalf("成员读数 = %v", member)
	}
}

type releaseGateClock struct{ at time.Time }

func (clock releaseGateClock) Now() time.Time { return clock.at }

type releaseGateConditions struct{}

func (releaseGateConditions) LoadPreconditionFindings(
	context.Context, domain.TenantID, domain.DecisionScopeReference, domain.GuardedAction, domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	return nil, true, nil
}

type releaseGateDutyRules struct{}

func (releaseGateDutyRules) LoadDutyPaymentGateRule(
	context.Context, domain.TenantID, domain.DecisionScopeReference, domain.GuardedAction, domain.CustomsProcedureReference,
) (domain.DutyPaymentGateRule, bool, error) {
	return domain.DutyPaymentNotAPrecondition(), true, nil
}

type releaseGateDutyVerifications struct{}

func (releaseGateDutyVerifications) LoadCurrentDutyVerification(
	context.Context, domain.TenantID, domain.DecisionScopeReference,
) (ports.DutyVerificationRecord, bool, error) {
	return ports.DutyVerificationRecord{}, false, nil
}

type releaseGateWeightRules struct {
	rule domain.WeightDiscrepancyGateRule
}

func (rules releaseGateWeightRules) LoadWeightDiscrepancyGateRule(
	context.Context, domain.TenantID, domain.DecisionScopeReference, domain.GuardedAction, domain.CustomsProcedureReference,
) (domain.WeightDiscrepancyGateRule, bool, error) {
	return rules.rule, true, nil
}

type releaseGateSubmissions struct {
	record ports.DeclarationSubmissionRecord
}

func (submissions releaseGateSubmissions) FindByKey(
	context.Context, ports.DeclarationSubmissionKey,
) (ports.DeclarationSubmissionRecord, bool, error) {
	return submissions.record, true, nil
}

type releaseGateMemberWeights struct{}

func (releaseGateMemberWeights) LoadMemberMeasuredWeight(
	context.Context, domain.TenantID, domain.DeclaredParcelReference,
) (ports.MeasuredWeightReading, error) {
	return ports.MeasuredWeightReading{
		Answer: ports.MeasuredWeightOne, GrossWeight: "1.80", GrossWeightUnit: "KG", MeasurementSource: "WEIGH-1",
	}, nil
}

type releaseGateStore struct {
	byKey map[ports.GateVerificationKey]domain.ReleaseGateVerification
}

func (store *releaseGateStore) FindByKey(
	_ context.Context, key ports.GateVerificationKey,
) (domain.ReleaseGateVerification, bool, error) {
	gate, found := store.byKey[key]
	return gate, found, nil
}

func (store *releaseGateStore) Save(
	_ context.Context, key ports.GateVerificationKey, gate domain.ReleaseGateVerification,
) (ports.GateVerificationSaveOutcome, error) {
	if _, exists := store.byKey[key]; exists {
		return ports.GateVerificationAlreadyRecorded, nil
	}
	store.byKey[key] = gate
	return ports.GateVerificationSaved, nil
}

type releaseGateHandoff struct{}

func (releaseGateHandoff) HandOffGate(context.Context, ports.GateVerificationHandoffIntent) error {
	return nil
}

func releaseGateHandler(t *testing.T, withReading bool) *application.VerifyReleaseGateHandler {
	t.Helper()
	rule := domain.WeightDiscrepancyNotAPrecondition()
	var submission ports.DeclarationSubmissionRecord
	if withReading {
		built, err := domain.AllowWeightDeltaUpTo("0.50", "KG")
		if err != nil {
			t.Fatal(err)
		}
		rule = built
		submission = releaseGateSubmission(t)
	}
	handler, err := application.NewVerifyReleaseGateHandler(application.VerifyReleaseGateDeps{
		Conditions:        releaseGateConditions{},
		DutyRules:         releaseGateDutyRules{},
		DutyVerifications: releaseGateDutyVerifications{},
		WeightRules:       releaseGateWeightRules{rule: rule},
		Submissions:       releaseGateSubmissions{record: submission},
		MemberWeights:     releaseGateMemberWeights{},
		Store:             &releaseGateStore{byKey: map[ports.GateVerificationKey]domain.ReleaseGateVerification{}},
		Downstream:        releaseGateHandoff{},
		Clock:             releaseGateClock{at: time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func releaseGateSubmission(t *testing.T) ports.DeclarationSubmissionRecord {
	t.Helper()
	parcel := releaseGateValue(t, domain.NewDeclaredParcelReference, "parcel-a")
	unitID := releaseGateValue(t, domain.NewDeclarationUnitID, "declaration-unit-1")
	procedure := releaseGateValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT")
	unit, err := domain.FormDeclarationUnit(
		unitID, releaseGateValue(t, domain.NewCustomsCaseID, "case-1"), procedure,
		[]domain.DeclaredParcelReference{parcel})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	readiness, err := domain.JudgeReady(unitID, releaseGateValue(t, domain.NewReadinessBasisReference, "readiness-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unitID, releaseGateValue(t, domain.NewSubmissionAuthorityReference, "authority-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewMemberMeasuredWeight(parcel, "2.60", "KG", "WEIGH-0A", at)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            releaseGateValue(t, domain.NewSubmissionVersionID, "submission-wd-1"),
		Unit:          unit,
		Dossier:       releaseGateValue(t, domain.NewDossierSnapshotReference, "dossier-1"),
		Roles:         releaseGateValue(t, domain.NewRoleSnapshotReference, "roles-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       at.Add(time.Hour),
		MemberWeights: []domain.MemberMeasuredWeight{weight},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.DeclarationSubmissionRecord{Version: version}
}

func releaseGateValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
