package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证送站交接单五口在 parcel-api 装配点的几件（票 ps-external-mark-relations/12 完成判据「信封各格」「真库用例 `AT-NO-075`」）：
// 作业事实登记、运营决定、客户面三族各自的未配置、令牌无效、未授予、不在准入范围；别族令牌一律拒；真库上经生产装配的同一组构造
// 函数开立、收寄带它、归入、确认，最后一个绑定确认的那一笔自投门铃。合成主体、账户与号码只记 `S`。

const (
	handoverNotesPath         = "/node-operations/delivery-handover-notes"
	handoverAssignmentsPath   = "/node-operations/delivery-handover-assignments"
	handoverClosuresPath      = "/node-operations/delivery-handover-closures"
	handoverConfirmationsPath = "/node-operations/delivery-handover-confirmations"
	handoverConclusionsPath   = "/node-operations/delivery-handover-conclusions"
	scopeEstablishedType      = "node-operations.delivery-scope.established"
)

// unwiredDeliveryHandoverNotes 是五口编排的占位：装配测试用它证端点表各行都接到了处理器、且 Intake 拒在编排之前。
type unwiredDeliveryHandoverNotes struct{}

func (unwiredDeliveryHandoverNotes) Open(context.Context, nodeopsapp.OpenDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return nodeopsapp.DeliveryHandoverResult{}, errOrchestrationNotWired
}

func (unwiredDeliveryHandoverNotes) Assign(context.Context, nodeopsapp.AssignToDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return nodeopsapp.DeliveryHandoverResult{}, errOrchestrationNotWired
}

func (unwiredDeliveryHandoverNotes) Close(context.Context, nodeopsapp.CloseDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return nodeopsapp.DeliveryHandoverResult{}, errOrchestrationNotWired
}

func (unwiredDeliveryHandoverNotes) ConfirmByOperationDecision(context.Context, nodeopsapp.ConfirmDeliveryHandoverCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return nodeopsapp.DeliveryHandoverResult{}, errOrchestrationNotWired
}

func (unwiredDeliveryHandoverNotes) ConcludeByCustomer(context.Context, nodeopsapp.ConcludeDeliveryHandoverCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return nodeopsapp.DeliveryHandoverResult{}, errOrchestrationNotWired
}

// admitAll 是准入范围的替身：一切生产写都在准入范围内。用来证授予齐备之后走到了编排。
type admitAll struct{}

func (admitAll) Admits(context.Context, string, accessidentity.AdmissionRequirement, time.Time) (bool, error) {
	return true, nil
}

// handoverEndpoints 按端点表那五行的写法装五口。
func handoverEndpoints(faces deliveryHandoverNoteFaces, decisions operatorDecisionIntakes) map[string]http.Handler {
	return map[string]http.Handler{
		handoverNotesPath:         nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(faces.openingIntake(), faces.handler),
		handoverAssignmentsPath:   nodeopshttp.NewAssignToDeliveryHandoverNoteEndpoint(faces.assignmentIntake(), faces.handler),
		handoverClosuresPath:      nodeopshttp.NewCloseDeliveryHandoverNoteEndpoint(faces.closureIntake(), faces.handler),
		handoverConfirmationsPath: nodeopshttp.NewConfirmDeliveryHandoverEndpoint(decisions.nodeOperations, faces.handler),
		handoverConclusionsPath:   nodeopshttp.NewConcludeDeliveryHandoverEndpoint(faces.conclusionIntake(), faces.handler),
	}
}

var handoverBodies = map[string]string{
	handoverNotesPath:         `{"noteId":"SYN-HANDOVER-API-01","deliveringParty":"SYN-DRIVER-API","claimedAccounts":["SYN-ACCOUNT-01"],"openedAt":"2026-10-19T08:00:00Z"}`,
	handoverAssignmentsPath:   `{"factId":"SYN-ASSIGN-API-01","noteId":"SYN-HANDOVER-API-01","unit":"SYN-UNIT-API-01","assignedAt":"2026-10-19T08:10:00Z"}`,
	handoverClosuresPath:      `{"factId":"SYN-CLOSE-API-01","noteId":"SYN-HANDOVER-API-01","closedAt":"2026-10-19T08:20:00Z"}`,
	handoverConfirmationsPath: `{"noteId":"SYN-HANDOVER-API-01","customerAccount":"SYN-ACCOUNT-01","deliveringParticipant":"SYN-PARTY-DELIVERER-01","basis":"SYN-BASIS-API"}`,
	handoverConclusionsPath:   `{"accessRequestId":"SYN-ACCESS-REQUEST-HN-0001","noteId":"SYN-HANDOVER-API-01","conclusion":"CONFIRMED"}`,
}

func postHandover(endpoint http.Handler, path, token, device string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(handoverBodies[path]))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if device != "" {
		request.Header.Set("Parcel-Device-Id", device)
		request.Header.Set("Parcel-Work-Scope-Kind", "NODE")
		request.Header.Set("Parcel-Work-Scope-Reference", "SYN-NODE-SHA-HUB")
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

var handoverWorkFactPaths = []string{handoverNotesPath, handoverAssignmentsPath, handoverClosuresPath}

// Covers: 票 12 完成判据「信封各格」的作业事实登记一族——开立、归入、关闭三口经装配点那只构造函数换上作业事实登记面：发行方未配置、
// 无令牌、只持查阅读授予、设备不在用、授予齐备而准入范围没登，各答其格；授予与准入都过才走到编排。测试输入是隔离合成，只记 `S`。
func TestHandoverWorkFactFacesAnswerOnTheOperatorChannel(t *testing.T) {
	const token = "presented.operator.token"
	faces := func(verifier accessidentity.OperatorCredentialVerifier, operators accessidentity.OperatorRegistry, admission accessidentity.AdmissionScope) map[string]http.Handler {
		intake, err := buildDeliveryHandoverNoteWorkFactIntake(verifier, operators, workFactDevices{}, admission)
		if err != nil {
			t.Fatal(err)
		}
		return handoverEndpoints(deliveryHandoverNoteFaces{workFacts: intake, handler: unwiredDeliveryHandoverNotes{}}, unconfiguredOperatorDecisions())
	}
	for _, testCase := range []struct {
		name      string
		endpoints map[string]http.Handler
		token     string
		device    string
		status    int
		code      string
	}{
		{"issuer not configured", faces(accessidentity.UnconfiguredOperatorCredentialVerifier{}, workFactOperators{}, admitAll{}), token, "SYN-DEVICE-NODE", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"no bearer token", faces(acceptingVerifier{}, workFactOperators{}, admitAll{}), "", "SYN-DEVICE-NODE", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"read grant is not a work fact grant", faces(acceptingVerifier{}, readerRegister{}, admitAll{}), token, "SYN-DEVICE-NODE", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"device is not in use", faces(acceptingVerifier{}, workFactOperators{}, admitAll{}), token, "SYN-DEVICE-ABSENT", http.StatusForbidden, "DEVICE_NOT_IN_USE"},
		{"granted, admission not registered", faces(acceptingVerifier{}, workFactOperators{}, admissionScopeUnset()), token, "SYN-DEVICE-NODE", http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		{"granted and admitted", faces(acceptingVerifier{}, workFactOperators{}, admitAll{}), token, "SYN-DEVICE-NODE", http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	} {
		for _, path := range handoverWorkFactPaths {
			response := postHandover(testCase.endpoints[path], path, testCase.token, testCase.device)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("%s, %s: answer = %d %q, want %d %q; body %s",
					path, testCase.name, response.Code, problemCode(t, response), testCase.status, testCase.code, response.Body.String())
			}
			assertNoOutcome(t, response, path)
		}
	}
}

// handoverConclusionRegister 是客户渠道册的替身：唯一的主体绑在（SYN-TENANT-01, SYN-ACCOUNT-01）上，持 capabilities 各项授予。
type handoverConclusionRegister struct {
	capabilities []accessidentity.CustomerCapability
}

func (register handoverConclusionRegister) FindCustomerClient(_ context.Context, subject accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", syntheticCustomerAccount, reference, false, "SYN-BASIS")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capability := range register.capabilities {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), subject, capability, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, nil)
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, grants)
	return standing, err == nil, err
}

// Covers: 票 12 完成判据「信封各格」的运营决定与客户面两族，与「别族令牌一律拒」——同一个发行方签的令牌，受众不是那一族的，在作业
// 事实三口与确认口答操作者令牌不过、在客户结论口答客户令牌不过；客户面：发行方未配置答未配置、不在册或只持预报授予答未授予、持
// 这一口授予而本产品的权威串没设答不在准入范围。运营决定一族的未授予与不在准入范围由 swappedDecisionFaces 那两条用例钉。各格都不
// 带 outcome。
func TestHandoverLinesRefuseOtherFamiliesAndJudgeAdmission(t *testing.T) {
	issuer := newCustomerIssuerDouble(t)
	customerVerifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("customer parameters: configured=%v err=%v", configured, err)
	}
	operatorVerifier, configured, err := buildOperatorCredentialVerifier(fakeGetenv(map[string]string{
		operatorIssuerEnv:   syntheticCustomerIssuer,
		operatorJWKSURLEnv:  issuer.jwks.URL,
		operatorAudienceEnv: syntheticOperatorAudience,
	}))
	if err != nil || !configured {
		t.Fatalf("operator parameters: configured=%v err=%v", configured, err)
	}
	conclusionFaces := func(verifier accessidentity.CustomerClientCredentialVerifier, registry accessidentity.CustomerClientRegistry) map[string]http.Handler {
		minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
		if err != nil {
			t.Fatal(err)
		}
		intake, err := buildCustomerHandoverConclusionIntake(minter)
		if err != nil {
			t.Fatal(err)
		}
		workFacts, err := buildDeliveryHandoverNoteWorkFactIntake(operatorVerifier, workFactOperators{}, workFactDevices{}, admitAll{})
		if err != nil {
			t.Fatal(err)
		}
		operatorMinter, err := buildOperatorMinter(operatorVerifier, decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionDeliveryHandoverConfirmation}}, admitAll{})
		if err != nil {
			t.Fatal(err)
		}
		decisions, err := buildOperatorDecisionIntakes(operatorMinter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
		if err != nil {
			t.Fatal(err)
		}
		return handoverEndpoints(deliveryHandoverNoteFaces{workFacts: workFacts, conclusion: intake, handler: unwiredDeliveryHandoverNotes{}}, decisions)
	}
	granted := handoverConclusionRegister{capabilities: []accessidentity.CustomerCapability{accessidentity.CustomerCapabilityDeliveryHandoverConfirmation}}
	preAdviceOnly := handoverConclusionRegister{capabilities: []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission, accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	}}
	for _, testCase := range []struct {
		name     string
		path     string
		audience string
		registry accessidentity.CustomerClientRegistry
		status   int
		code     string
	}{
		{"customer token at a work fact line", handoverNotesPath, syntheticCustomerAudience, granted, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"integration token at a work fact line", handoverAssignmentsPath, "idp-parcel-integration", granted, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"customer token at the closure line", handoverClosuresPath, syntheticCustomerAudience, granted, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"customer token at the confirmation line", handoverConfirmationsPath, syntheticCustomerAudience, granted, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"operator token at the conclusion line", handoverConclusionsPath, syntheticOperatorAudience, granted, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		{"integration token at the conclusion line", handoverConclusionsPath, "idp-parcel-integration", granted, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		{"customer token, unregistered", handoverConclusionsPath, syntheticCustomerAudience, absentCustomerRegister{}, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		{"customer token, pre-advice only", handoverConclusionsPath, syntheticCustomerAudience, preAdviceOnly, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		{"customer granted, no self authority", handoverConclusionsPath, syntheticCustomerAudience, granted, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			device := "SYN-DEVICE-NODE"
			if testCase.path == handoverConclusionsPath || testCase.path == handoverConfirmationsPath {
				device = ""
			}
			endpoint := conclusionFaces(customerVerifier, testCase.registry)[testCase.path]
			response := postHandover(endpoint, testCase.path, issuer.token(t, testCase.audience, syntheticCustomerSubject), device)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("answer = %d %q, want %d %q；%s", response.Code, problemCode(t, response), testCase.status, testCase.code, response.Body.String())
			}
			assertNoOutcome(t, response, testCase.path)
		})
	}

	unconfigured, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	for _, faces := range []deliveryHandoverNoteFaces{{handler: unwiredDeliveryHandoverNotes{}}} {
		probe := &readProbe{}
		request := httptest.NewRequest(http.MethodPost, handoverConclusionsPath, probe)
		response := httptest.NewRecorder()
		handoverEndpoints(faces, unconfiguredOperatorDecisions())[handoverConclusionsPath].ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" || probe.read {
			t.Fatalf("首方族未配置：%d %s，读了载荷 %v", response.Code, response.Body.String(), probe.read)
		}
	}
	response := postHandover(conclusionFaces(unconfigured, granted)[handoverConclusionsPath], handoverConclusionsPath, "any.token", "")
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("发行方参数未设：%d %s", response.Code, response.Body.String())
	}
}

// registerCustomerAccountInPartyCommercial 在 party-commercial 登记 account，所属参与方是 owner：授权交付判断据它认「交付方就是该
// 客户账户所属参与方本身」。
func registerCustomerAccountInPartyCommercial(t *testing.T, db *bentopg.DB, tenant, account, owner string) {
	t.Helper()
	registrations, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := mustValue(t, pcdomain.NewTenantID, tenant)
	rehydrated, err := pcdomain.RehydrateCustomerAccount(tenantID, mustValue(t, pcdomain.NewCustomerAccountID, account), mustValue(t, pcdomain.NewPartyID, owner))
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := pcdomain.NewIdentityLifecycle(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := pcdomain.NewCustomerAccountRegistration(rehydrated, 1, mustValue(t, pcdomain.NewIdentityBasisReference, "SYN-BASIS-"+account), lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	var outcome pcports.PartyRegistrySaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = registrations.SaveCustomerAccount(txCtx, registration)
		return saveErr
	}); err != nil || outcome != pcports.PartyRegistrySaved {
		t.Fatalf("登记客户账户：%s %v", outcome, err)
	}
}

// Covers: `AT-NO-071`、`AT-NO-072` 与 `AT-NO-075` 的 parcel-api 半边（真库，经生产装配 buildReceptionOrchestration 与
// buildDeliveryHandoverNoteOrchestration）：委托带客户包裹件号接受；同号实物不带依据送站落范围未证的待识别；开立交接单、另一件带它
// 的标识收寄（落成员、范围未证·绑定未全部确认）、把那件待识别归入（不改写来源、不追加判断）；运营决定确认：交付方不是本人且本租户
// 一条授权交付关系都没登记答未配置、不落结论、不响门铃；交付方就是账户所属参与方本身答绑定已确认、范围成立，同一笔自投门铃恰一封；
// 原样重投答已有结果、不多响。重核与追加关联版归 parcel-dispatch 那一半。
func TestADeliveryHandoverNoteIsOpenedFilledAndConfirmedAgainstARealDatabase(t *testing.T) {
	db, reception := wiredReceptionOnRealDatabase(t)
	ctx := t.Context()
	const number = "SYN-HANDOVER-NUMBER-0001"
	submission := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-HANDOVER-1", "SYN-PARCEL-HANDOVER-1", number)
	submitOnRealAssembly(t, db, submission)
	acceptNumbered(t, db, submission)
	registerCustomerAccountInPartyCommercial(t, db, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-PARTY-OWNER-PSXM12")

	unadvised := deliverMarked(t, reception, "HANDOVER-UNADVISED", "SYN-CUSTOMER-1", number)
	assertScopeUnverifiedOnRealAssembly(t, unadvised, "UNVERIFIED/NO_BASIS/", "SYN-PARCEL-HANDOVER-1")
	pending, _ := unadvised.Record()

	handover, err := buildDeliveryHandoverNoteOrchestration(db)
	if err != nil {
		t.Fatalf("装配交接单编排：%v", err)
	}
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1")
	node := mustValue(t, nodomain.NewNodeReference, "SYN-NODE-ORIGIN")
	noteID := mustValue(t, nodomain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-API-75")
	opened, err := handover.Open(ctx, nodeopsapp.OpenDeliveryHandoverNoteCommand{
		TenantID: tenant, Node: node, Note: noteID,
		DeliveringParty: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM12"),
		ClaimedAccounts: []nodomain.CustomerAccountReference{mustValue(t, nodomain.NewCustomerAccountReference, "SYN-CUSTOMER-1")},
		OpenedAt:        time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond),
	})
	if err != nil || opened.Outcome() != nodeopsapp.DeliveryHandoverNoteOpened {
		t.Fatalf("开立：%v %v", opened.Outcome(), err)
	}

	carried := receptionCommand(t, "SYN-DELIVERY-HANDOVER-CARRIED", nodeopsapp.ExplicitReception)
	carried.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-HANDOVER-CARRIED")
	carried.Mark = noports.ExternalMarkObservation{Mark: "SYN-HANDOVER-NUMBER-UNKNOWN"}
	carried.ScopeBasis = mustValue(t, func(raw string) (nodomain.DeliveryScopeBasisReference, error) {
		return nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryHandoverNote, raw)
	}, noteID.String())
	if result, err := reception.Handle(ctx, carried); err != nil || result.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("带交接单收寄：%v %v", result.Outcome(), err)
	}

	assigned, err := handover.Assign(ctx, nodeopsapp.AssignToDeliveryHandoverNoteCommand{
		TenantID: tenant, Node: node, FactID: "SYN-ASSIGN-API-75", Note: noteID, Unit: pending.Intake.Unit(), AssignedAt: time.Now().UTC(),
	})
	if err != nil || assigned.Outcome() != nodeopsapp.DeliveryHandoverUnitAssigned {
		t.Fatalf("归入：%v %v", assigned.Outcome(), err)
	}
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	members, err := notes.ListDeliveryHandoverMembers(ctx, tenant, noteID)
	if err != nil || len(members) != 2 || members[0].Kind() != nodomain.MembershipByReception || members[0].FactID() != carried.SourceID ||
		members[1].Kind() != nodomain.MembershipByAssignment || members[1].Unit() != pending.Intake.Unit() {
		t.Fatalf("成员 = %+v（err %v）, want 收寄带来的一个、归入的一个", members, err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	if stored, found, err := receptions.FindByKey(ctx, pending.Key); err != nil || !found || stored.Source.ScopeBasis.Present() || stored.JudgmentVersion != pending.JudgmentVersion {
		t.Fatalf("归入之后那件的来源 = %+v（found %v err %v）, want 原样", stored.Source, found, err)
	}
	if current, err := receptions.CurrentVersion(ctx, tenant, pending.JudgmentVersion); err != nil || current != pending.JudgmentVersion {
		t.Fatalf("归入追加了判断：%s %v", current, err)
	}

	confirm := nodeopsapp.ConfirmDeliveryHandoverCommand{
		TenantID: tenant, Operator: "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-PSXM12", Note: noteID,
		Account:               mustValue(t, nodomain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		DeliveringParticipant: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-PARTY-DELIVERER-PSXM12"),
		Basis:                 "SYN-BASIS-COUNTER-PSXM12",
	}
	unconfigured, err := handover.ConfirmByOperationDecision(ctx, confirm)
	if err != nil || unconfigured.Outcome() != nodeopsapp.DeliveryHandoverAuthorizationNotConfigured {
		t.Fatalf("交付方不是本人、没登授权交付关系：%v %v, want 未配置", unconfigured.Outcome(), err)
	}
	if n := envelopeCountOfType(t, db, scopeEstablishedType); n != 0 {
		t.Fatalf("未配置却响了门铃 %d 封", n)
	}

	confirm.DeliveringParticipant = mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-PARTY-OWNER-PSXM12")
	confirmed, err := handover.ConfirmByOperationDecision(ctx, confirm)
	if err != nil || confirmed.Outcome() != nodeopsapp.DeliveryHandoverBindingConfirmed {
		t.Fatalf("交付方就是本人：%v %v, want 绑定已确认", confirmed.Outcome(), err)
	}
	if _, established := confirmed.ScopeEstablishedAt(); !established {
		t.Fatal("唯一声称的账户确认之后范围没成立")
	}
	if n := envelopeCountOfType(t, db, scopeEstablishedType); n != 1 {
		t.Fatalf("范围成立后门铃 %d 封, want 1", n)
	}
	replay, err := handover.ConfirmByOperationDecision(ctx, confirm)
	if err != nil || replay.Outcome() != nodeopsapp.DeliveryHandoverExistingResult {
		t.Fatalf("原样重投：%v %v", replay.Outcome(), err)
	}
	if n := envelopeCountOfType(t, db, scopeEstablishedType); n != 1 {
		t.Fatalf("重投后门铃 %d 封, want 仍是 1", n)
	}
	state, found, err := notes.FindDeliveryHandoverNote(ctx, tenant, noteID)
	if err != nil || !found || !state.ScopeEstablished() || len(state.Bindings) != 1 || state.Bindings[0].Route() != nodomain.ConcludedViaOperationDecision {
		t.Fatalf("册上的交接单 = %+v（found %v err %v）", state, found, err)
	}
}
