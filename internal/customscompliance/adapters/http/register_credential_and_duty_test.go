package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件对凭证 / 税费付款协作事项 / 税费付款核对三个在线登记端点（票 sa-cc/07 步二）证
// 传输面：只收 POST、未配置 403 且不读内容、Intake 失败分流、答案逐名转写、响应形封闭。
// 协作与核对那族的答案代数与案件配置族不是一张表——`未决`在它那里分两种：义务依据缺席是
// UC-CC-009 步 4 的业务答案（形成了答案，答的是「等税费结果」），存储不可用才是「没形成
// 答案」；本文件的断言就钉这道分界。

// dutyIntakeDouble 交回测试预先备好的命令，对请求零读取（判据同 configurationIntakeDouble）。
type dutyIntakeDouble struct {
	credential    application.RegisterCredentialCommand
	collaboration application.FormDutyCollaborationCommand
	verification  application.VerifyDutyPaymentCommand
	err           error
}

func (double dutyIntakeDouble) IntakeRegulatoryCredentialRegistration(
	context.Context, *http.Request,
) (application.RegisterCredentialCommand, error) {
	return double.credential, double.err
}

func (double dutyIntakeDouble) IntakeDutyCollaborationRegistration(
	context.Context, *http.Request,
) (application.FormDutyCollaborationCommand, error) {
	return double.collaboration, double.err
}

func (double dutyIntakeDouble) IntakeDutyPaymentVerificationRegistration(
	context.Context, *http.Request,
) (application.VerifyDutyPaymentCommand, error) {
	return double.verification, double.err
}

// dutyRegistrarDouble 顶替协作 / 核对编排，交回测试给定的结果。两个方法长在一个类型上：
// 端点各自只消费其中一个接口，接错编译期就红。
type dutyRegistrarDouble struct {
	result application.DutyReconciliationResult
	err    error
	called bool
}

func (double *dutyRegistrarDouble) FormCollaboration(
	context.Context, application.FormDutyCollaborationCommand,
) (application.DutyReconciliationResult, error) {
	double.called = true
	return double.result, double.err
}

func (double *dutyRegistrarDouble) VerifyPayment(
	context.Context, application.VerifyDutyPaymentCommand,
) (application.DutyReconciliationResult, error) {
	double.called = true
	return double.result, double.err
}

type unreachableDutyRegistrar struct{ t *testing.T }

func (registrar unreachableDutyRegistrar) FormCollaboration(
	context.Context, application.FormDutyCollaborationCommand,
) (application.DutyReconciliationResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the collaboration orchestration")
	return application.DutyReconciliationResult{}, nil
}

func (registrar unreachableDutyRegistrar) VerifyPayment(
	context.Context, application.VerifyDutyPaymentCommand,
) (application.DutyReconciliationResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the verification orchestration")
	return application.DutyReconciliationResult{}, nil
}

// credentialAndDutyEndpoints 遍历三个登记端点，各配「被调即失败」的编排替身与生产侧的
// 未配置 Intake。
func credentialAndDutyEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	unconfigured := customshttp.UnconfiguredIntake{}
	return map[string]http.Handler{
		"凭证": customshttp.NewRegisterRegulatoryCredentialEndpoint(
			unconfigured,
			unreachableConfigurationRegistrar[application.RegisterCredentialCommand]{t: t}),
		"协作事项": customshttp.NewRegisterDutyCollaborationEndpoint(unconfigured, unreachableDutyRegistrar{t: t}),
		"付款核对": customshttp.NewRegisterDutyPaymentVerificationEndpoint(unconfigured, unreachableDutyRegistrar{t: t}),
	}
}

func TestCredentialAndDutyRegistrationEndpointsOnlyAcceptPost(t *testing.T) {
	for name, endpoint := range credentialAndDutyEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET 答 %d, want 405", recorder.Code)
			}
			if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("Allow = %q, want POST", allow)
			}
		})
	}
}

// Covers: ADR-0055「未配置即拒、不读内容」；ADR-0085 Decision 二 写准入不另立形。
func TestUnconfiguredCredentialAndDutyRegistrationRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range credentialAndDutyEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", probe))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("未配置答 %d, want 403", recorder.Code)
			}
			if code := problemCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("错误码 = %q", code)
			}
			assertNoOutcome(t, recorder)
			if probe.read {
				t.Fatal("未配置 Intake 读了登记载荷")
			}
		})
	}
}

// Covers: ADR-0055「答复对一切请求内容与自报身份一致」——载荷里的 tenantId 是这一面最
// 危险的自报身份。
func TestUnconfiguredCredentialAndDutyRegistrationAnswersEveryRequestIdentically(t *testing.T) {
	for name, endpoint := range credentialAndDutyEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/probe", nil))
			variants := map[string]*http.Request{
				"登记快照": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(`{"tenantId":"SYN-TEN-CC07","credentialId":"SYN-CRED-1"}`)),
				"畸形载荷": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("!!not-json!!")),
				"另一租户": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(`{"tenantId":"SYN-TEN-OTHER"}`)),
			}
			reported := httptest.NewRequest(http.MethodPost, "/probe", nil)
			reported.Header.Set("X-Reported-Tenant", "SYN-TEN-CC07")
			variants["自报身份头部"] = reported
			for variant, request := range variants {
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, request)
				if recorder.Code != baseline.Code || recorder.Body.String() != baseline.Body.String() {
					t.Fatalf("%s：答复与基线不同：%d %s vs %d %s",
						variant, recorder.Code, recorder.Body.String(), baseline.Code, baseline.Body.String())
				}
			}
		})
	}
}

func TestDutyRegistrationMapsIntakeFailures(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"畸形", fmt.Errorf("登记载荷缺格: %w", customshttp.ErrMalformedRequest), http.StatusBadRequest, "MALFORMED_REQUEST"},
		{"Intake 故障", errors.New("认证后端寄了"), http.StatusInternalServerError, "INTAKE_FAILED"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			registrar := &dutyRegistrarDouble{}
			endpoint := customshttp.NewRegisterDutyCollaborationEndpoint(dutyIntakeDouble{err: spec.err}, registrar)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
			if recorder.Code != spec.wantStatus {
				t.Fatalf("答 %d, want %d", recorder.Code, spec.wantStatus)
			}
			if code := problemCode(t, recorder); code != spec.wantCode {
				t.Fatalf("错误码 = %q, want %s", code, spec.wantCode)
			}
			if registrar.called {
				t.Fatal("Intake 没交出命令，编排不该被调到")
			}
		})
	}
}

// TestCredentialRegistrationSharesTheConfigurationTranscription 证凭证端点走案件配置族的
// 转写（同一套 CaseConfigurationOutcome）：已登记 201、其余治理答案 200 原名、未决 5xx。
func TestCredentialRegistrationSharesTheConfigurationTranscription(t *testing.T) {
	cases := []struct {
		outcome    application.CaseConfigurationOutcome
		wantStatus int
		wantAnswer string
	}{
		{application.ConfigurationRegistered, http.StatusCreated, "REGISTERED"},
		{application.ConfigurationExisting, http.StatusOK, "EXISTING"},
		{application.ConfigurationContentConflict, http.StatusOK, "CONTENT_CONFLICT"},
		{application.ConfigurationNotAccepted, http.StatusOK, "NOT_ACCEPTED"},
	}
	for _, spec := range cases {
		t.Run(spec.wantAnswer, func(t *testing.T) {
			endpoint := customshttp.NewRegisterRegulatoryCredentialEndpoint(
				dutyIntakeDouble{},
				&configurationRegistrarDouble[application.RegisterCredentialCommand]{outcome: spec.outcome})
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
			if recorder.Code != spec.wantStatus {
				t.Fatalf("答 %d（%s）, want %d", recorder.Code, recorder.Body.String(), spec.wantStatus)
			}
			body := decodeRegistrationBody(t, recorder)
			if body["outcome"] != spec.wantAnswer {
				t.Fatalf("outcome = %v, want %s", body["outcome"], spec.wantAnswer)
			}
			assertKeys(t, body, "outcome")
		})
	}

	t.Run("UNDECIDED", func(t *testing.T) {
		endpoint := customshttp.NewRegisterRegulatoryCredentialEndpoint(
			dutyIntakeDouble{},
			&configurationRegistrarDouble[application.RegisterCredentialCommand]{outcome: application.ConfigurationUndecided})
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("未决答 %d, want 500", recorder.Code)
		}
		if code := problemCode(t, recorder); code != "NO_ANSWER_FORMED" {
			t.Fatalf("错误码 = %q", code)
		}
		assertNoOutcome(t, recorder)
	})
}

// —— 协作 / 核对两端点的转写断言走真编排 ——
//
// DutyReconciliationResult 的字段不导出、也没有构造函数，本文件不伪造它：答案代数只有
// 用例造得出（受理门、前置、冲突判定都在它里面），所以每一格都用真 handler 接存储替身
// 造出来——伪造得出的那份代数与生产的是不是同一份，正是这几条断言要证的东西（判据同
// register_configuration_test 的 candidatePortUseCase）。只有「零值答案」与「编排返错」
// 两格用替身，那两格用例本就造不出。

// dutyStoreStub 是存储三口、付款人规则读口加结算交接口一体的替身：每口的写入代数与读回按格给定。交接口
// （票 sa-cc/05）默认成功；它失败不翻核对、只留续办引用，本文件的转写断言不碰那一格——答复
// 面透不透续办引用归 sa-cc/15 与 CLI 一并裁。付款人规则（票 sa-cc/12）零值即「登了、不要求」：核对族
// 既有各格的转写与这一维无关，只有付款人三格自己把它拨到别处。
type dutyStoreStub struct {
	saveOutcome ports.CaseConfigurationSaveOutcome
	saveErr     error

	collaboration      domain.DutyPaymentCollaboration
	collaborationFound bool
	findErr            error

	fundsFound bool
	fundsErr   error

	payerRule             domain.PayerRequirement
	payerRuleUnconfigured bool
	payerRuleErr          error

	handoffErr error
}

func (stub dutyStoreStub) LoadPayerRequirement(
	context.Context, domain.TenantID, domain.CustomsProcedureReference,
) (domain.PayerRequirement, bool, error) {
	if stub.payerRuleErr != nil {
		return domain.PayerRequirementInvalid, false, stub.payerRuleErr
	}
	if stub.payerRuleUnconfigured {
		return domain.PayerRequirementInvalid, false, nil
	}
	if stub.payerRule == domain.PayerRequirementInvalid {
		return domain.PayerNotRequired, true, nil
	}
	return stub.payerRule, true, nil
}

func (stub dutyStoreStub) HandOffDutyPaymentVerification(
	context.Context, ports.DutyPaymentVerificationHandoffIntent,
) error {
	return stub.handoffErr
}

func (stub dutyStoreStub) FindCollaboration(
	context.Context, domain.TenantID, domain.DecisionScopeReference, domain.AssessedDutyReference,
) (domain.DutyPaymentCollaboration, bool, error) {
	return stub.collaboration, stub.collaborationFound, stub.findErr
}

func (stub dutyStoreStub) SaveCollaboration(
	context.Context, domain.TenantID, domain.DutyPaymentCollaboration,
) (ports.CaseConfigurationSaveOutcome, error) {
	return stub.saveOutcome, stub.saveErr
}

func (stub dutyStoreStub) RegisterFundsFact(
	context.Context, domain.TenantID, ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	return stub.saveOutcome, stub.saveErr
}

// LoadFundsFactVersion 交回的那条事实付款人取「来源未提供」：它是付款人三格里唯一会让答案分岔的形，
// 其余格对这一维无感；零值付款人两格都不是，编排会当编程错误抛出，替身不交它。
func (stub dutyStoreStub) LoadFundsFactVersion(
	context.Context, domain.TenantID, domain.ExternalFundsFactReference, domain.FundsFactVersion,
) (ports.ExternalFundsFactRegistration, bool, error) {
	return ports.ExternalFundsFactRegistration{Payer: domain.FundsPayerNotProvided()}, stub.fundsFound, stub.fundsErr
}

// ListFundsFactVersions 只被 ReceiveFundsFact 在同键重登时读；本文件的两口端点都不收资金事实，交空即可。
func (stub dutyStoreStub) ListFundsFactVersions(
	context.Context, domain.TenantID, domain.ExternalFundsFactReference,
) ([]ports.ExternalFundsFactRegistration, error) {
	return nil, stub.fundsErr
}

func (stub dutyStoreStub) FindVerification(
	context.Context, ports.DutyVerificationKey,
) (ports.DutyVerificationRecord, bool, error) {
	return ports.DutyVerificationRecord{}, false, nil
}

// ListVerificationsByFundsFact 只被「资金事实新版本到达」的编排读；本文件的两口端点不走那一路，交空即可。
func (stub dutyStoreStub) ListVerificationsByFundsFact(
	context.Context, domain.TenantID, domain.ExternalFundsFactReference,
) ([]ports.DutyVerificationRecord, error) {
	return nil, nil
}

func (stub dutyStoreStub) SaveVerification(
	context.Context, ports.DutyVerificationRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	return stub.saveOutcome, stub.saveErr
}

type dutyTestClock struct{}

func (dutyTestClock) Now() time.Time { return time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC) }

func dutyValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// collaborationCommand 是一份立得住的核定税费格协作事项（身份全取 SYN- 前缀）。
func collaborationCommand(t *testing.T) application.FormDutyCollaborationCommand {
	t.Helper()
	return application.FormDutyCollaborationCommand{
		TenantID:    dutyValue(t, domain.NewTenantID, "SYN-TEN-CC07"),
		Kind:        domain.ObligationFromAssessedDuty,
		Duty:        dutyValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		Scope:       dutyValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Obligor:     dutyValue(t, domain.NewLegalObligorReference, "SYN-OBLIGOR-01"),
		Requirement: dutyValue(t, domain.NewPaymentRequirementSource, "SYN-ASSESSMENT-01"),
		Target:      dutyValue(t, domain.NewResponsibilityTargetReference, "SYN-DUTY-DESK"),
	}
}

// collaborationOnRegister 是册上那份：与命令同内容（重放）或换了责任交接目标（冲突）。
func collaborationOnRegister(t *testing.T, command application.FormDutyCollaborationCommand, target string) domain.DutyPaymentCollaboration {
	t.Helper()
	collaboration, err := domain.FormDutyCollaboration(domain.DutyCollaborationSpec{
		Kind:        command.Kind,
		Duty:        command.Duty,
		Scope:       command.Scope,
		Obligor:     command.Obligor,
		Requirement: command.Requirement,
		Target:      dutyValue(t, domain.NewResponsibilityTargetReference, target),
		FormedAt:    dutyTestClock{}.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("造册上协作事项：%v", err)
	}
	return collaboration
}

func verificationCommand(t *testing.T, basis string) application.VerifyDutyPaymentCommand {
	t.Helper()
	return application.VerifyDutyPaymentCommand{
		TenantID:     dutyValue(t, domain.NewTenantID, "SYN-TEN-CC07"),
		Duty:         dutyValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		Funds:        dutyValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
		FundsVersion: dutyValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
		Scope:        dutyValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Procedure:    dutyValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-01"),
		Coverage:     domain.CoveragePartial,
		Delta:        domain.DeltaShort,
		Validity:     domain.FundsFactPending,
		Basis:        basis,
	}
}

// dutyHandlerOver 把真编排装在替身上；它本身就满足两个 Registrar 契约（方法名即用例方法名）。
func dutyHandlerOver(t *testing.T, stub dutyStoreStub) *application.DutyPaymentReconciliationHandler {
	t.Helper()
	handler, err := application.NewDutyPaymentReconciliationHandler(application.DutyPaymentReconciliationDeps{
		Collaborations: stub, Funds: stub, Verifications: stub, PayerRules: stub, Handoff: stub, Clock: dutyTestClock{},
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return handler
}

type dutyAnswerCase struct {
	name       string
	wantStatus int
	wantAnswer string
	wantReason string
}

// TestDutyCollaborationRegistrationTranscribesTheAnswerAlgebra 证协作事项端点逐名过线：
// 形成 201；已存在 / 内容冲突 / 未受理 200 原名；义务依据缺席的业务未决 200 带
// undecidedReason（UC-CC-009 步 4 的第四个结果，形成了答案）；存储不可用的未决 5xx
// （没形成答案）。
func TestDutyCollaborationRegistrationTranscribesTheAnswerAlgebra(t *testing.T) {
	command := collaborationCommand(t)
	blankTenant := command
	blankTenant.TenantID = domain.TenantID{}
	basisAbsent := command
	basisAbsent.Kind = domain.DutyObligationKindInvalid
	basisAbsent.Duty = domain.AssessedDutyReference{}

	cases := []struct {
		dutyAnswerCase
		command application.FormDutyCollaborationCommand
		stub    dutyStoreStub
	}{
		{dutyAnswerCase{"形成", http.StatusCreated, "COLLABORATION_FORMED", ""}, command,
			dutyStoreStub{saveOutcome: ports.CaseConfigurationRegistered}},
		{dutyAnswerCase{"已存在", http.StatusOK, "EXISTING_COLLABORATION", ""}, command,
			dutyStoreStub{saveOutcome: ports.CaseConfigurationAlreadyRegistered,
				collaboration: collaborationOnRegister(t, command, "SYN-DUTY-DESK"), collaborationFound: true}},
		{dutyAnswerCase{"内容冲突", http.StatusOK, "COLLABORATION_CONTENT_CONFLICT", ""}, command,
			dutyStoreStub{saveOutcome: ports.CaseConfigurationAlreadyRegistered,
				collaboration: collaborationOnRegister(t, command, "SYN-OTHER-DESK"), collaborationFound: true}},
		{dutyAnswerCase{"未受理", http.StatusOK, "NOT_ACCEPTED", ""}, blankTenant, dutyStoreStub{}},
		{dutyAnswerCase{"义务依据缺席", http.StatusOK, "UNDECIDED", "DUTY_OBLIGATION_BASIS_ABSENT"}, basisAbsent, dutyStoreStub{}},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			endpoint := customshttp.NewRegisterDutyCollaborationEndpoint(
				dutyIntakeDouble{collaboration: spec.command}, dutyHandlerOver(t, spec.stub))
			assertDutyAnswer(t, endpoint, spec.dutyAnswerCase)
		})
	}

	t.Run("存储不可用", func(t *testing.T) {
		endpoint := customshttp.NewRegisterDutyCollaborationEndpoint(
			dutyIntakeDouble{collaboration: command},
			dutyHandlerOver(t, dutyStoreStub{saveErr: errors.New("collaboration store unavailable")}))
		assertNoAnswerFormed(t, endpoint)
	})
	t.Run("编排返错", func(t *testing.T) {
		endpoint := customshttp.NewRegisterDutyCollaborationEndpoint(
			dutyIntakeDouble{collaboration: command}, &dutyRegistrarDouble{err: errors.New("事务壳寄了")})
		assertNoAnswerFormed(t, endpoint)
	})
	t.Run("没有名字的答案不上线", func(t *testing.T) {
		endpoint := customshttp.NewRegisterDutyCollaborationEndpoint(
			dutyIntakeDouble{collaboration: command}, &dutyRegistrarDouble{})
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("零值答案答 %d, want 500", recorder.Code)
		}
		if code := problemCode(t, recorder); code != "UNNAMED_OUTCOME" {
			t.Fatalf("错误码 = %q", code)
		}
	})
}

// TestDutyPaymentVerificationRegistrationTranscribesTheAnswerAlgebra 证核对端点逐名过线：
// 形成 201；已存在 / 待关联 / 前置未齐两格 / 未受理 200 原名——待关联与前置未齐是核对对
// 这次登记作出的判断（无权威依据不关联；资金事实未接收、协作事项未形成），不是失败；
// 付款人两格业务未决（程序要求而来源未提供、规则未配置，票 sa-cc/12 裁决 2）200 带 undecidedReason；
// 三种存储不可用与付款人规则读口不可用 5xx。
func TestDutyPaymentVerificationRegistrationTranscribesTheAnswerAlgebra(t *testing.T) {
	command := verificationCommand(t, "SYN-RULE-01: remittance quotes assessment")
	blankTenant := command
	blankTenant.TenantID = domain.TenantID{}
	ready := dutyStoreStub{fundsFound: true, collaborationFound: true}

	cases := []struct {
		dutyAnswerCase
		command application.VerifyDutyPaymentCommand
		stub    dutyStoreStub
	}{
		{dutyAnswerCase{"形成", http.StatusCreated, "DUTY_VERIFICATION_FORMED", ""}, command,
			dutyStoreStub{fundsFound: true, collaborationFound: true, saveOutcome: ports.CaseConfigurationRegistered}},
		{dutyAnswerCase{"已存在", http.StatusOK, "EXISTING_DUTY_VERIFICATION", ""}, command,
			dutyStoreStub{fundsFound: true, collaborationFound: true, saveOutcome: ports.CaseConfigurationAlreadyRegistered}},
		{dutyAnswerCase{"待关联", http.StatusOK, "FUNDS_FACT_PENDING_ASSOCIATION", ""}, verificationCommand(t, ""), ready},
		{dutyAnswerCase{"资金事实未接收", http.StatusOK, "FUNDS_FACT_NOT_RECEIVED", ""}, command, dutyStoreStub{collaborationFound: true}},
		{dutyAnswerCase{"协作事项未形成", http.StatusOK, "COLLABORATION_NOT_FORMED", ""}, command, dutyStoreStub{fundsFound: true}},
		{dutyAnswerCase{"未受理", http.StatusOK, "NOT_ACCEPTED", ""}, blankTenant, ready},
		{dutyAnswerCase{"程序要求付款人而来源未提供", http.StatusOK, "UNDECIDED", "PAYER_REQUIRED_NOT_PROVIDED"}, command,
			dutyStoreStub{fundsFound: true, collaborationFound: true, payerRule: domain.PayerRequired}},
		{dutyAnswerCase{"付款人规则未配置", http.StatusOK, "UNDECIDED", "PAYER_REQUIREMENT_NOT_CONFIGURED"}, command,
			dutyStoreStub{fundsFound: true, collaborationFound: true, payerRuleUnconfigured: true}},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			endpoint := customshttp.NewRegisterDutyPaymentVerificationEndpoint(
				dutyIntakeDouble{verification: spec.command}, dutyHandlerOver(t, spec.stub))
			assertDutyAnswer(t, endpoint, spec.dutyAnswerCase)
		})
	}

	unavailable := errors.New("store unavailable")
	for name, stub := range map[string]dutyStoreStub{
		"资金事实册不可用":   {fundsErr: unavailable},
		"协作事项库不可用":   {fundsFound: true, findErr: unavailable},
		"核对库不可用":     {fundsFound: true, collaborationFound: true, saveErr: unavailable},
		"付款人规则读口不可用": {fundsFound: true, collaborationFound: true, payerRuleErr: unavailable},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := customshttp.NewRegisterDutyPaymentVerificationEndpoint(
				dutyIntakeDouble{verification: command}, dutyHandlerOver(t, stub))
			assertNoAnswerFormed(t, endpoint)
		})
	}
}

// TestOnlineCredentialAndDutyRegistrationTakesTheSameSnapshotShapeAsTheCLI 是一条编译期
// 断言（判据同配置族那条）：三个 Intake 契约要交出的命令，正是步一 registrationjson 从
// 登记快照本体译出的那一个——谁另写一份译装、或让某一口收起了与 CLI -input 不同的形状，
// 这里就编译不过。
func TestOnlineCredentialAndDutyRegistrationTakesTheSameSnapshotShapeAsTheCLI(t *testing.T) {
	intake := customshttp.UnconfiguredIntake{}
	sameSnapshotShape(intake.IntakeRegulatoryCredentialRegistration, registrationjson.RegulatoryCredentialFromJSON)
	sameSnapshotShape(intake.IntakeDutyCollaborationRegistration, registrationjson.DutyCollaborationFromJSON)
	sameSnapshotShape(intake.IntakeDutyPaymentVerificationRegistration, registrationjson.DutyPaymentVerificationFromJSON)
}

// TestIsolatedReadIntakeCannotServeCredentialAndDutyRegistration 钉住 ADR-0078 的编译期排除
// 在这三口同样成立：隔离读放行类型不满足任何一个登记命令 Intake 接口（ADR-0085 Decision 二
// 维持的原判）。
func TestIsolatedReadIntakeCannotServeCredentialAndDutyRegistration(t *testing.T) {
	var intake any = customshttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(customshttp.RegulatoryCredentialRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进凭证登记口")
	}
	if _, ok := intake.(customshttp.DutyCollaborationRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进协作事项登记口")
	}
	if _, ok := intake.(customshttp.DutyPaymentVerificationRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进付款核对登记口")
	}
}

func assertDutyAnswer(t *testing.T, endpoint http.Handler, spec dutyAnswerCase) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
	if recorder.Code != spec.wantStatus {
		t.Fatalf("答 %d（%s）, want %d", recorder.Code, recorder.Body.String(), spec.wantStatus)
	}
	body := decodeRegistrationBody(t, recorder)
	if body["outcome"] != spec.wantAnswer {
		t.Fatalf("outcome = %v, want %s", body["outcome"], spec.wantAnswer)
	}
	if spec.wantReason == "" {
		assertKeys(t, body, "outcome")
		return
	}
	if body["undecidedReason"] != spec.wantReason {
		t.Fatalf("undecidedReason = %v, want %s", body["undecidedReason"], spec.wantReason)
	}
	assertKeys(t, body, "outcome", "undecidedReason")
}

func assertNoAnswerFormed(t *testing.T, endpoint http.Handler) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("没形成答案却答 %d（%s）, want 500", recorder.Code, recorder.Body.String())
	}
	if code := problemCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q, want NO_ANSWER_FORMED", code)
	}
	assertNoOutcome(t, recorder)
}

func decodeRegistrationBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	return body
}

// assertKeys 钉响应形封闭（ADR-0022）：键集恰好是列出的那几个，多一个少一个都红。
func assertKeys(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	got := make([]string, 0, len(body))
	for key := range body {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("响应键集 = %v, want %v", got, want)
	}
}
