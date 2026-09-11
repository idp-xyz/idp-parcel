package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对六类配置登记端点（ADR-0085，票 admin-write-faces/02 切片 02d）证传输面：方法门、
// Intake 失败分流、答案逐名转写、依赖故障 5xx、没有名字的答案不上线；并钉住 ADR-0078 的
// 排除仍然成立——隔离读 Intake 装不进任何一个登记口。
//
// 未配置 403 与「不读内容、答复不随请求变」由 unconfigured_intake_test.go 的那张表覆盖，
// 六个登记端点已加进去，此处不重复。

// milestoneMappingIntakeDouble 交回测试预先备好的命令，对请求零读取。
//
// 刻意不让它去解析 body：读请求取租户正是生产侧被禁的那一件（采信自报租户穿透 ADR-0003
// 的隔离），替身长成那个形状会让「本包没有采信实现」这句话变得要靠人去分辨。命令由测试
// 用 registrationjson 从登记快照本体译出——同源因此仍然被钉住，且钉在真正要紧的那一侧。
type milestoneMappingIntakeDouble struct {
	command application.RegisterMilestoneMappingCommand
	err     error
}

func (double milestoneMappingIntakeDouble) IntakeMilestoneMappingRegistration(
	context.Context,
	*http.Request,
) (application.RegisterMilestoneMappingCommand, error) {
	if double.err != nil {
		return application.RegisterMilestoneMappingCommand{}, double.err
	}
	return double.command, nil
}

// milestoneMappingRegistrarDouble 顶替编排。它交回的是构造得出的零值结果——那正是
// 「应用层交回了没有名字的答案」这一格，用来证传输层不把它当业务答案发出去。
type milestoneMappingRegistrarDouble struct {
	result application.RegisterCatalogResult
	err    error
	called bool
}

func (double *milestoneMappingRegistrarDouble) Handle(
	context.Context,
	application.RegisterMilestoneMappingCommand,
) (application.RegisterCatalogResult, error) {
	double.called = true
	return double.result, double.err
}

// milestoneMappingUseCase 把真用例接到端点上。答案代数只有用例造得出（结果的两格由
// application 包内的构造器封着），所以转写断言走真用例而不是伪造的结果值——伪造得出的
// 那份代数与生产的是不是同一份，本来就是这条断言要证的东西。
type milestoneMappingUseCase struct {
	service *application.CatalogRegistration
}

func (useCase milestoneMappingUseCase) Handle(
	ctx context.Context,
	command application.RegisterMilestoneMappingCommand,
) (application.RegisterCatalogResult, error) {
	return useCase.service.RegisterMilestoneMapping(ctx, command)
}

// stubCatalogRegistry 是写入口替身：各方法同答一格。写入口的三格代数（已登记 / 版本
// 已在册 / 区间重叠）由它给出，用例据以译成登记册答案。
type stubCatalogRegistry struct {
	outcome ports.CatalogRegistrationOutcome
}

func (registry stubCatalogRegistry) RegisterMilestoneMapping(
	context.Context, domain.TenantID, ports.MilestoneMappingRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterTriageRules(
	context.Context, domain.TenantID, ports.TriageRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterNotificationPolicy(
	context.Context, domain.TenantID, ports.NotificationPolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterClaimEligibility(
	context.Context, domain.TenantID, ports.ClaimEligibilityRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterClaimAuthorization(
	context.Context, domain.TenantID, ports.ClaimAuthorizationRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterDisclosurePolicy(
	context.Context, domain.TenantID, ports.DisclosurePolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterExceptionDisclosureRules(
	context.Context, domain.TenantID, ports.ExceptionDisclosureRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

func (registry stubCatalogRegistry) RegisterConflictSignalRule(
	context.Context, domain.TenantID, ports.ConflictSignalRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.outcome, nil
}

// milestoneMappingSnapshot 是一份立得住的最小登记快照，形状与登记 CLI 的 -input 逐字同源
// （身份全取 SYN- 前缀，隔离合成只记 `S`）。
const milestoneMappingSnapshot = `{
	"tenantId": "SYN-TEN-VE02D",
	"version": "SYN-MAP-V1",
	"approvedBy": "SYN-approver-1",
	"effectiveFrom": "2026-09-01T00:00:00Z",
	"entries": [
		{"source": "PARCEL_SHIPMENT", "factKind": "SYN_KIND_DELIVERED", "milestone": "SYN-MILESTONE-DELIVERED"}
	]
}`

// milestoneMappingSnapshotWithoutVersion 缺版本号。翻译层放行（缺件判据在用例），用例据以
// 答 VERSION_MISSING——用它证「用例的拒绝原名走到线上」而不必伪造结果值。
const milestoneMappingSnapshotWithoutVersion = `{
	"tenantId": "SYN-TEN-VE02D",
	"approvedBy": "SYN-approver-1",
	"effectiveFrom": "2026-09-01T00:00:00Z",
	"entries": [
		{"source": "PARCEL_SHIPMENT", "factKind": "SYN_KIND_DELIVERED", "milestone": "SYN-MILESTONE-DELIVERED"}
	]
}`

func commandFromSnapshot(t *testing.T, snapshot string) application.RegisterMilestoneMappingCommand {
	t.Helper()
	command, err := registrationjson.MilestoneMappingFromJSON([]byte(snapshot))
	if err != nil {
		t.Fatalf("译登记快照：%v", err)
	}
	return command
}

func milestoneMappingEndpointOver(
	t *testing.T,
	snapshot string,
	outcome ports.CatalogRegistrationOutcome,
) http.Handler {
	t.Helper()
	service, err := application.NewCatalogRegistration(stubCatalogRegistry{outcome: outcome})
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	return visibilityhttp.NewRegisterMilestoneMappingEndpoint(
		milestoneMappingIntakeDouble{command: commandFromSnapshot(t, snapshot)},
		milestoneMappingUseCase{service: service},
	)
}

// catalogRegistrationEndpoints 遍历六个登记端点，各配一个「被调即失败」的编排替身。
// 逐个走一遍而不是只测一类：端点体虽由 newCatalogRegistrationEndpoint 共用，六个构造函数
// 各自把哪个 Intake 方法接到哪个编排上是逐类写的，接错那一格只有逐类走过才看得见。
func catalogRegistrationEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	unconfigured := visibilityhttp.UnconfiguredIntake{}
	return map[string]http.Handler{
		"milestone mapping": visibilityhttp.NewRegisterMilestoneMappingEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterMilestoneMappingCommand]{t: t}),
		"triage rules": visibilityhttp.NewRegisterTriageRulesEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterTriageRulesCommand]{t: t}),
		"notification policy": visibilityhttp.NewRegisterNotificationPolicyEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterNotificationPolicyCommand]{t: t}),
		"claim eligibility": visibilityhttp.NewRegisterClaimEligibilityEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterClaimEligibilityCommand]{t: t}),
		"claim authorization": visibilityhttp.NewRegisterClaimAuthorizationEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterClaimAuthorizationCommand]{t: t}),
		"disclosure policy": visibilityhttp.NewRegisterDisclosurePolicyEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterDisclosurePolicyCommand]{t: t}),
		"exception disclosure rules": visibilityhttp.NewRegisterExceptionDisclosureRulesEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterExceptionDisclosureRulesCommand]{t: t}),
		"conflict signal rule": visibilityhttp.NewRegisterConflictSignalRuleEndpoint(
			unconfigured, unreachableRegistrar[application.RegisterConflictSignalRuleCommand]{t: t}),
	}
}

// unreachableRegistrar 是六类共用的「被调即失败」编排替身。泛型按命令类型实例化，因此
// 它顶替得了六个 Registrar 契约中的任何一个，而实例之间互不相容——把一类的替身接到另一
// 类的端点上编译期就红。
type unreachableRegistrar[Command any] struct{ t *testing.T }

func (registrar unreachableRegistrar[Command]) Handle(
	context.Context,
	Command,
) (application.RegisterCatalogResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the registration orchestration")
	return application.RegisterCatalogResult{}, nil
}

func TestCatalogRegistrationEndpointsOnlyAcceptPost(t *testing.T) {
	for name, endpoint := range catalogRegistrationEndpoints(t) {
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

func TestCatalogRegistrationEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := visibilityhttp.NewRegisterMilestoneMappingEndpoint(
		milestoneMappingIntakeDouble{
			err: fmt.Errorf("载荷缺格: %w", visibilityhttp.ErrMalformedRegistration),
		},
		&milestoneMappingRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

func TestCatalogRegistrationEndpointMapsOtherIntakeFailureToIntakeFailed(t *testing.T) {
	registrar := &milestoneMappingRegistrarDouble{}
	endpoint := visibilityhttp.NewRegisterMilestoneMappingEndpoint(
		milestoneMappingIntakeDouble{err: errors.New("认证后端寄了")},
		registrar,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Intake 故障答 %d, want 500", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "INTAKE_FAILED" {
		t.Fatalf("错误码 = %q", code)
	}
	if registrar.called {
		t.Fatal("Intake 没交出命令，编排不该被调到")
	}
}

// TestCatalogRegistrationEndpointTranscribesTheUseCaseAnswersVerbatim 证答案逐名过线：
// 已登记走 201，登记册的拒绝走 200 并带上拒绝理由原名。拒绝不用 4xx——它是登记册对这次
// 登记作出的判断，与「请求根本构造不出命令」的恢复动作不同，压成一格登记方就不知道该改
// 内容还是改请求形状。
func TestCatalogRegistrationEndpointTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		snapshot   string
		outcome    ports.CatalogRegistrationOutcome
		wantStatus int
		wantAnswer string
		wantReason string
	}{
		{
			name:       "已登记",
			snapshot:   milestoneMappingSnapshot,
			outcome:    ports.CatalogVersionRegistered,
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "版本不可覆盖是治理答案",
			snapshot:   milestoneMappingSnapshot,
			outcome:    ports.CatalogVersionAlreadyRegistered,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "VERSION_NOT_OVERWRITABLE",
		},
		{
			name:       "区间重叠是治理答案",
			snapshot:   milestoneMappingSnapshot,
			outcome:    ports.CatalogVersionOverlapsExisting,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "VERSION_OVERLAPS_EXISTING",
		},
		{
			name:       "缺件拒绝指名缺的是哪一件",
			snapshot:   milestoneMappingSnapshotWithoutVersion,
			outcome:    ports.CatalogVersionRegistered,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "VERSION_MISSING",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			endpoint := milestoneMappingEndpointOver(t, testCase.snapshot, testCase.outcome)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d, want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			body := registrationAnswer(t, recorder)
			if body.Outcome != testCase.wantAnswer {
				t.Fatalf("outcome = %q, want %q", body.Outcome, testCase.wantAnswer)
			}
			if body.RefusalReason != testCase.wantReason {
				t.Fatalf("refusalReason = %q, want %q", body.RefusalReason, testCase.wantReason)
			}
		})
	}
}

func TestCatalogRegistrationEndpointAnswersServerErrorWhenRegistrationUndecided(t *testing.T) {
	endpoint := visibilityhttp.NewRegisterMilestoneMappingEndpoint(
		milestoneMappingIntakeDouble{command: commandFromSnapshot(t, milestoneMappingSnapshot)},
		&milestoneMappingRegistrarDouble{err: errors.New("库连不上")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
}

// TestCatalogRegistrationEndpointRefusesToShipAnAnswerWithoutAName 证没有名字的答案不上线：
// 空 `outcome` 会被客户端当成一种新的业务结果，而空 `refusalReason` 让登记方无从知道该改
// 什么——拒绝理由在这套代数里是答案的另一半（登记 CLI 的退出码正按它分路）。
func TestCatalogRegistrationEndpointRefusesToShipAnAnswerWithoutAName(t *testing.T) {
	endpoint := visibilityhttp.NewRegisterMilestoneMappingEndpoint(
		milestoneMappingIntakeDouble{command: commandFromSnapshot(t, milestoneMappingSnapshot)},
		&milestoneMappingRegistrarDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("无名答案答 %d, want 500", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "UNNAMED_OUTCOME" {
		t.Fatalf("错误码 = %q", code)
	}
}

// TestOnlineRegistrationTakesTheSameSnapshotShapeAsTheCLI 是一条编译期断言：六类 Intake
// 契约要交出的命令，正是 registrationjson 从登记快照本体译出的那一个。两侧共用一个类型
// 参数，谁另写一份翻译、或让某一类的在线口收起了与 CLI -input 不同的形状，这里就编译不过。
func TestOnlineRegistrationTakesTheSameSnapshotShapeAsTheCLI(t *testing.T) {
	intake := visibilityhttp.UnconfiguredIntake{}
	sameSnapshotShape(intake.IntakeMilestoneMappingRegistration, registrationjson.MilestoneMappingFromJSON)
	sameSnapshotShape(intake.IntakeTriageRulesRegistration, registrationjson.TriageRulesFromJSON)
	sameSnapshotShape(intake.IntakeNotificationPolicyRegistration, registrationjson.NotificationPolicyFromJSON)
	sameSnapshotShape(intake.IntakeClaimEligibilityRegistration, registrationjson.ClaimEligibilityFromJSON)
	sameSnapshotShape(intake.IntakeClaimAuthorizationRegistration, registrationjson.ClaimAuthorizationFromJSON)
	sameSnapshotShape(intake.IntakeDisclosurePolicyRegistration, registrationjson.DisclosurePolicyFromJSON)
	sameSnapshotShape(intake.IntakeExceptionDisclosureRulesRegistration, registrationjson.ExceptionDisclosureRulesFromJSON)
	sameSnapshotShape(intake.IntakeConflictSignalRuleRegistration, registrationjson.ConflictSignalRuleFromJSON)
}

// sameSnapshotShape 两个参数都不使用：它表达的是类型相等，不是一次调用。
func sameSnapshotShape[Command any](
	_ func(context.Context, *http.Request) (Command, error),
	_ func([]byte) (Command, error),
) {
}

// registrationAnswer 按登记端点的封闭响应形状解出答案两件。用具名结构而不是 map：字段名
// 若与响应对不上，map 那种写法会静默地拿到空串并通过，而字段名正是这几条断言要钉的东西。
func registrationAnswer(t *testing.T, response *httptest.ResponseRecorder) struct {
	Outcome       string `json:"outcome"`
	RefusalReason string `json:"refusalReason"`
} {
	t.Helper()
	var body struct {
		Outcome       string `json:"outcome"`
		RefusalReason string `json:"refusalReason"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode registration answer %s: %v", response.Body.Bytes(), err)
	}
	return body
}

// TestIsolatedReadIntakeCannotServeCatalogRegistration 钉住 ADR-0078 的编译期排除在写面
// 成立：隔离读放行类型不满足任何登记命令 Intake 接口。这条一旦变红，说明有人把隔离放行
// 扩到了写行——那是 ADR-0085 Decision 二明文维持的原判。
func TestIsolatedReadIntakeCannotServeCatalogRegistration(t *testing.T) {
	var intake any = visibilityhttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(visibilityhttp.MilestoneMappingRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进里程碑映射登记口")
	}
	if _, ok := intake.(visibilityhttp.TriageRulesRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进分诊规则登记口")
	}
	if _, ok := intake.(visibilityhttp.NotificationPolicyRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进通知策略登记口")
	}
	if _, ok := intake.(visibilityhttp.ClaimEligibilityRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进索赔资格登记口")
	}
	if _, ok := intake.(visibilityhttp.ClaimAuthorizationRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进申请人授权登记口")
	}
	if _, ok := intake.(visibilityhttp.DisclosurePolicyRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进披露策略登记口")
	}
	if _, ok := intake.(visibilityhttp.ExceptionDisclosureRulesRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进异常披露规则登记口")
	}
	if _, ok := intake.(visibilityhttp.ConflictSignalRuleRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进冲突信号规则登记口")
	}
}

// 两册规则登记端点（票 ve-disclosure-policy-view/02 步二）的三态转写：端点体虽与其余登记
// 端点共用，两个构造函数各自把哪个 Intake 方法接到哪个编排上是逐类写的，接错那一格只有走
// 真用例才看得见——这里用 registrationjson 译出的真命令穿过真用例，写入口替身给三格代数。

type exceptionDisclosureRulesIntakeDouble struct {
	command application.RegisterExceptionDisclosureRulesCommand
}

func (double exceptionDisclosureRulesIntakeDouble) IntakeExceptionDisclosureRulesRegistration(
	context.Context,
	*http.Request,
) (application.RegisterExceptionDisclosureRulesCommand, error) {
	return double.command, nil
}

type exceptionDisclosureRulesUseCase struct {
	service *application.CatalogRegistration
}

func (useCase exceptionDisclosureRulesUseCase) Handle(
	ctx context.Context,
	command application.RegisterExceptionDisclosureRulesCommand,
) (application.RegisterCatalogResult, error) {
	return useCase.service.RegisterExceptionDisclosureRules(ctx, command)
}

type conflictSignalRuleIntakeDouble struct {
	command application.RegisterConflictSignalRuleCommand
}

func (double conflictSignalRuleIntakeDouble) IntakeConflictSignalRuleRegistration(
	context.Context,
	*http.Request,
) (application.RegisterConflictSignalRuleCommand, error) {
	return double.command, nil
}

type conflictSignalRuleUseCase struct {
	service *application.CatalogRegistration
}

func (useCase conflictSignalRuleUseCase) Handle(
	ctx context.Context,
	command application.RegisterConflictSignalRuleCommand,
) (application.RegisterCatalogResult, error) {
	return useCase.service.RegisterConflictSignalRule(ctx, command)
}

// 快照身份全取 SYN- 前缀，隔离合成只记 `S`；两份快照都过得了翻译，缺件那一格由用例答。
const exceptionDisclosureRulesSnapshot = `{
	"tenantId": "SYN-TEN-VE02",
	"version": "SYN-EDR-V1",
	"approvedBy": "SYN-approver-1",
	"effectiveFrom": "2026-09-01T00:00:00Z",
	"entries": [
		{"customer": "SYN-CUSTOMER-1", "signalKind": "SYN-SIGNAL-STALL", "confidence": "SYN-CONF-HIGH",
		 "disclosable": true, "autoRelease": false, "content": "SYN-CONTENT-STALL"}
	]
}`

const conflictSignalRuleSnapshot = `{
	"tenantId": "SYN-TEN-VE02",
	"signalKind": "SYN-SIGNAL-FACT-CONFLICT",
	"version": "SYN-RULE-V1",
	"confidence": "SYN-CONF-MEDIUM",
	"approvedBy": "SYN-approver-1"
}`

// conflictSignalRuleSnapshotWithoutApproval 缺批准责任：翻译放行（缺件判据在用例），用例答
// APPROVAL_MISSING——用它证用例的拒绝原名走到线上。
const conflictSignalRuleSnapshotWithoutApproval = `{
	"tenantId": "SYN-TEN-VE02",
	"signalKind": "SYN-SIGNAL-FACT-CONFLICT",
	"version": "SYN-RULE-V1",
	"confidence": "SYN-CONF-MEDIUM",
	"approvedBy": " "
}`

func TestRuleRegistrationEndpointsTranscribeTheUseCaseAnswersVerbatim(t *testing.T) {
	buildEndpoint := func(t *testing.T, name, snapshot string, outcome ports.CatalogRegistrationOutcome) http.Handler {
		t.Helper()
		service, err := application.NewCatalogRegistration(stubCatalogRegistry{outcome: outcome})
		if err != nil {
			t.Fatalf("构造登记用例：%v", err)
		}
		switch name {
		case "exception disclosure rules":
			command, err := registrationjson.ExceptionDisclosureRulesFromJSON([]byte(snapshot))
			if err != nil {
				t.Fatalf("译异常披露规则快照：%v", err)
			}
			return visibilityhttp.NewRegisterExceptionDisclosureRulesEndpoint(
				exceptionDisclosureRulesIntakeDouble{command: command},
				exceptionDisclosureRulesUseCase{service: service},
			)
		default:
			command, err := registrationjson.ConflictSignalRuleFromJSON([]byte(snapshot))
			if err != nil {
				t.Fatalf("译冲突信号规则快照：%v", err)
			}
			return visibilityhttp.NewRegisterConflictSignalRuleEndpoint(
				conflictSignalRuleIntakeDouble{command: command},
				conflictSignalRuleUseCase{service: service},
			)
		}
	}

	cases := []struct {
		name       string
		endpoint   string
		snapshot   string
		outcome    ports.CatalogRegistrationOutcome
		wantStatus int
		wantAnswer string
		wantReason string
	}{
		{
			name:       "异常披露规则已登记",
			endpoint:   "exception disclosure rules",
			snapshot:   exceptionDisclosureRulesSnapshot,
			outcome:    ports.CatalogVersionRegistered,
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "异常披露规则区间重叠是治理答案",
			endpoint:   "exception disclosure rules",
			snapshot:   exceptionDisclosureRulesSnapshot,
			outcome:    ports.CatalogVersionOverlapsExisting,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "VERSION_OVERLAPS_EXISTING",
		},
		{
			name:       "冲突信号规则已登记",
			endpoint:   "conflict signal rule",
			snapshot:   conflictSignalRuleSnapshot,
			outcome:    ports.CatalogVersionRegistered,
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			// 一租户一条（0025）：撞既有行是这册唯一的治理答案，原行不被顶替。
			name:       "冲突信号规则撞既有行是治理答案",
			endpoint:   "conflict signal rule",
			snapshot:   conflictSignalRuleSnapshot,
			outcome:    ports.CatalogVersionAlreadyRegistered,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "VERSION_NOT_OVERWRITABLE",
		},
		{
			name:       "冲突信号规则缺件拒绝指名缺的是哪一件",
			endpoint:   "conflict signal rule",
			snapshot:   conflictSignalRuleSnapshotWithoutApproval,
			outcome:    ports.CatalogVersionRegistered,
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "APPROVAL_MISSING",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			endpoint := buildEndpoint(t, testCase.endpoint, testCase.snapshot, testCase.outcome)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d, want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			body := registrationAnswer(t, recorder)
			if body.Outcome != testCase.wantAnswer {
				t.Fatalf("outcome = %q, want %q", body.Outcome, testCase.wantAnswer)
			}
			if body.RefusalReason != testCase.wantReason {
				t.Fatalf("refusalReason = %q, want %q", body.RefusalReason, testCase.wantReason)
			}
		})
	}
}
