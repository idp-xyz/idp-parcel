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
	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件对四类配置登记端点（ADR-0085，票 admin-write-faces/02 切片 02b）证传输面：
// 方法门、未配置 403 且不读内容、Intake 失败分流、答案逐名转写、未决不冒充治理答案、
// 没有名字的答案不上线；并钉住 ADR-0078 的排除仍然成立——隔离读 Intake 装不进任何一个
// 登记口。
//
// 未配置态与本包既有两口的写法一致：各端点族在自己的测试文件里覆盖自己的未配置形态
// （receive_external_result 那一口的表在 unconfigured_intake_test.go，查阅两口各在自己
// 文件里），本文件因此自带那张表而不去改别人的。

// configurationIntakeDouble 交回测试预先备好的命令，对请求零读取。
//
// 四个方法长在同一个类型上，形照生产侧的 UnconfiguredIntake：四类的命令类型互不相同，
// 装配点把一类的译装接到另一类的端点上编译期就红，因此拆成四个替身类型换不来第二道
// 保障，只会把同一段替身抄四遍。
//
// 刻意不让它去解析 body：读请求取租户正是生产侧被禁的那一件（采信报文自称的租户穿透
// ADR-0003 的隔离边界），替身长成那个形状会让「本包没有采信实现」这句话变得要靠人去
// 分辨。命令由测试用 registrationjson 从登记快照本体译出——与受控 CLI 同源因此仍然
// 被钉住，且钉在真正要紧的那一侧。
type configurationIntakeDouble struct {
	interpretationRule application.RegisterInterpretationRuleCommand
	gateCatalog        application.RegisterGateCatalogCommand
	candidatePort      application.RegisterCandidatePortCommand
	declarationPath    application.RegisterDeclarationPathCommand
	err                error
}

func (double configurationIntakeDouble) IntakeInterpretationRuleRegistration(
	context.Context,
	*http.Request,
) (application.RegisterInterpretationRuleCommand, error) {
	return double.interpretationRule, double.err
}

func (double configurationIntakeDouble) IntakeGateCatalogRegistration(
	context.Context,
	*http.Request,
) (application.RegisterGateCatalogCommand, error) {
	return double.gateCatalog, double.err
}

func (double configurationIntakeDouble) IntakeCandidatePortRegistration(
	context.Context,
	*http.Request,
) (application.RegisterCandidatePortCommand, error) {
	return double.candidatePort, double.err
}

func (double configurationIntakeDouble) IntakeDeclarationPathRegistration(
	context.Context,
	*http.Request,
) (application.RegisterDeclarationPathCommand, error) {
	return double.declarationPath, double.err
}

// configurationRegistrarDouble 顶替编排，交回测试给定的那一格。零值 outcome 正是
// 「应用层交回了没有名字的答案」那一格，用来证传输层不把它当业务答案发出去。
type configurationRegistrarDouble[Command any] struct {
	outcome application.CaseConfigurationOutcome
	err     error
	called  bool
}

func (double *configurationRegistrarDouble[Command]) Handle(
	context.Context,
	Command,
) (application.CaseConfigurationOutcome, error) {
	double.called = true
	return double.outcome, double.err
}

// unreachableConfigurationRegistrar 是四类共用的「被调即失败」编排替身。泛型按命令
// 类型实例化，因此它顶替得了四个 Registrar 契约中的任何一个，而实例之间互不相容——把
// 一类的替身接到另一类的端点上编译期就红。
type unreachableConfigurationRegistrar[Command any] struct{ t *testing.T }

func (registrar unreachableConfigurationRegistrar[Command]) Handle(
	context.Context,
	Command,
) (application.CaseConfigurationOutcome, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the registration orchestration")
	return application.CaseConfigurationOutcomeInvalid, nil
}

// candidatePortUseCase 把真用例接到端点上。答案代数只有用例造得出（受理门与冲突判定
// 都在它里面），所以转写断言走真用例而不是伪造的结果值——伪造得出的那份代数与生产的
// 是不是同一份，本来就是这几条断言要证的东西。
//
// 生产装配同样要这么一层：登记 handler 的方法名按册分（RegisterCandidatePort 等），
// 端点收的是 Handle，装配点在那里连同环境事务一起包（形照登记 CLI 的 execute）。
type candidatePortUseCase struct {
	handler *application.RegisterPortsPathsHandler
}

func (useCase candidatePortUseCase) Handle(
	ctx context.Context,
	command application.RegisterCandidatePortCommand,
) (application.CaseConfigurationOutcome, error) {
	return useCase.handler.RegisterCandidatePort(ctx, command)
}

// stubPortsPathsRegistry 是写口替身：写入代数只有两格（已登记 / 同键已在册），用例
// 据以决定要不要读回比对。err 那一格是本文件最要紧一条断言的输入——用例把依赖故障
// 折成 UNDECIDED 这一格**值**而不是 error。
type stubPortsPathsRegistry struct {
	outcome ports.CaseConfigurationSaveOutcome
	err     error
}

func (stub stubPortsPathsRegistry) RegisterCandidatePort(
	context.Context, domain.TenantID, domain.CustomsPortReference, time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	return stub.outcome, stub.err
}

func (stub stubPortsPathsRegistry) RegisterDeclarationPath(
	context.Context, domain.TenantID, domain.DeclarationPathReference, domain.DeclarationPathRoute, time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	return stub.outcome, stub.err
}

// stubPortsPathsView 交回冲突判定要读回的那一行。found=false 即「写口说已在册、按请求
// 起点却读不回」——用例判内容冲突（撞上了起点不同的既有区间）。
type stubPortsPathsView struct {
	entry ports.CandidatePortEntry
	found bool
}

func (stub stubPortsPathsView) LoadCandidatePort(
	context.Context, domain.TenantID, domain.CustomsPortReference, time.Time,
) (ports.CandidatePortEntry, bool, error) {
	return stub.entry, stub.found, nil
}

func (stub stubPortsPathsView) LoadDeclarationPath(
	context.Context, domain.TenantID, domain.DeclarationPathReference, time.Time,
) (ports.DeclarationPathEntry, bool, error) {
	return ports.DeclarationPathEntry{}, false, nil
}

// candidatePortSnapshot 是一份立得住的最小登记快照，形状与受控登记 CLI 的 -input 逐字
// 同源（身份全取 SYN- 前缀，隔离合成只记 `S`）。
const candidatePortSnapshot = `{
	"tenantId": "SYN-TEN-CC02B",
	"portRef": "SYN-PORT-1",
	"appliesFrom": "2026-09-01T00:00:00Z"
}`

func candidatePortCommand(t *testing.T) application.RegisterCandidatePortCommand {
	t.Helper()
	command, err := registrationjson.CandidatePortFromJSON([]byte(candidatePortSnapshot))
	if err != nil {
		t.Fatalf("译登记快照：%v", err)
	}
	return command
}

// candidatePortEndpointOver 把真用例装在给定的写口与读口替身上。
func candidatePortEndpointOver(
	t *testing.T,
	command application.RegisterCandidatePortCommand,
	registry stubPortsPathsRegistry,
	view stubPortsPathsView,
) http.Handler {
	t.Helper()
	handler := application.NewRegisterPortsPathsHandler(application.RegisterPortsPathsDeps{
		Registry: registry,
		View:     view,
	})
	return customshttp.NewRegisterCandidatePortEndpoint(
		configurationIntakeDouble{candidatePort: command},
		candidatePortUseCase{handler: handler},
	)
}

// configurationRegistrationEndpoints 遍历四个登记端点，各配一个「被调即失败」的编排
// 替身与生产侧的未配置 Intake。逐个走一遍而不是只测一类：端点体虽由
// newConfigurationRegistrationEndpoint 共用，四个构造函数各自把哪个 Intake 方法接到
// 哪个编排上是逐类写的。
func configurationRegistrationEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	unconfigured := customshttp.UnconfiguredIntake{}
	return map[string]http.Handler{
		"解释规则": customshttp.NewRegisterInterpretationRuleEndpoint(
			unconfigured,
			unreachableConfigurationRegistrar[application.RegisterInterpretationRuleCommand]{t: t}),
		"门禁目录": customshttp.NewRegisterGateCatalogEndpoint(
			unconfigured,
			unreachableConfigurationRegistrar[application.RegisterGateCatalogCommand]{t: t}),
		"候选口岸": customshttp.NewRegisterCandidatePortEndpoint(
			unconfigured,
			unreachableConfigurationRegistrar[application.RegisterCandidatePortCommand]{t: t}),
		"申报路径": customshttp.NewRegisterDeclarationPathEndpoint(
			unconfigured,
			unreachableConfigurationRegistrar[application.RegisterDeclarationPathCommand]{t: t}),
	}
}

func TestConfigurationRegistrationEndpointsOnlyAcceptPost(t *testing.T) {
	for name, endpoint := range configurationRegistrationEndpoints(t) {
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

// Covers: ADR-0055 「未配置即拒、不读内容」 — 四个登记端点各答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读报文、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。写准入不另立形（ADR-0085 Decision 二），因此这一格与命令面同答。
func TestUnconfiguredConfigurationRegistrationRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range configurationRegistrationEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", probe))

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("未配置答 %d, want 403", recorder.Code)
			}
			if code := problemCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("错误码 = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", code)
			}
			assertNoOutcome(t, recorder)
			if probe.read {
				t.Fatal("未配置 Intake 读了登记载荷")
			}
		})
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — 登记载荷
// 里的 tenantId 是这一面最危险的自报身份（ADR-0003 隔离边界）；答复随它变化就是采信的
// 第一个征兆。
func TestUnconfiguredConfigurationRegistrationAnswersEveryRequestIdentically(t *testing.T) {
	for name, endpoint := range configurationRegistrationEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/probe", nil))

			variants := map[string]*http.Request{
				"空载荷":  httptest.NewRequest(http.MethodPost, "/probe", nil),
				"登记快照": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(candidatePortSnapshot)),
				"畸形载荷": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("!!not-json!!")),
				"查询串":  httptest.NewRequest(http.MethodPost, "/probe?tenant=SYN-TEN-CC02B", nil),
				"另一租户": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(`{"tenantId":"SYN-TEN-OTHER"}`)),
			}
			reported := httptest.NewRequest(http.MethodPost, "/probe", nil)
			reported.Header.Set("X-Reported-Tenant", "SYN-TEN-CC02B")
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

func TestConfigurationRegistrationMapsMalformedIntakeToBadRequest(t *testing.T) {
	registrar := &configurationRegistrarDouble[application.RegisterCandidatePortCommand]{}
	endpoint := customshttp.NewRegisterCandidatePortEndpoint(
		configurationIntakeDouble{
			err: fmt.Errorf("登记载荷缺格: %w", customshttp.ErrMalformedRequest),
		},
		registrar,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
	if registrar.called {
		t.Fatal("Intake 没交出命令，编排不该被调到")
	}
}

func TestConfigurationRegistrationMapsOtherIntakeFailureToIntakeFailed(t *testing.T) {
	registrar := &configurationRegistrarDouble[application.RegisterCandidatePortCommand]{}
	endpoint := customshttp.NewRegisterCandidatePortEndpoint(
		configurationIntakeDouble{err: errors.New("认证后端寄了")},
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

// TestConfigurationRegistrationTranscribesTheUseCaseAnswersVerbatim 证答案逐名过线：
// 已登记走 201，登记册的其余判断走 200 带原名。拒绝与冲突不用 4xx——它们是登记册对
// 这次登记作出的判断，与「请求根本构造不出命令」的恢复动作不同，压成一格登记方就不
// 知道该改内容还是改请求形状。
func TestConfigurationRegistrationTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	registered := candidatePortCommand(t)
	onRegister := ports.CandidatePortEntry{Port: registered.Port, AppliesFrom: registered.AppliesFrom}

	cases := []struct {
		name       string
		command    application.RegisterCandidatePortCommand
		registry   stubPortsPathsRegistry
		view       stubPortsPathsView
		wantStatus int
		wantAnswer string
	}{
		{
			name:       "已登记",
			command:    registered,
			registry:   stubPortsPathsRegistry{outcome: ports.CaseConfigurationRegistered},
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "重放同一份是已存在",
			command:    registered,
			registry:   stubPortsPathsRegistry{outcome: ports.CaseConfigurationAlreadyRegistered},
			view:       stubPortsPathsView{entry: onRegister, found: true},
			wantStatus: http.StatusOK,
			wantAnswer: "EXISTING",
		},
		{
			name:       "撞上别的区间是内容冲突",
			command:    registered,
			registry:   stubPortsPathsRegistry{outcome: ports.CaseConfigurationAlreadyRegistered},
			view:       stubPortsPathsView{found: false},
			wantStatus: http.StatusOK,
			wantAnswer: "CONTENT_CONFLICT",
		},
		{
			// 缺件的快照在 registrationjson 那一层就被拒（译装自己把门），走不到用例；
			// 这一格要证的是「用例的受理拒绝原名走到线上」，因此直接给一个受理门拒得下
			// 的命令，不绕译装层。
			name:       "受理拒绝也是答案",
			command:    application.RegisterCandidatePortCommand{},
			registry:   stubPortsPathsRegistry{outcome: ports.CaseConfigurationRegistered},
			wantStatus: http.StatusOK,
			wantAnswer: "NOT_ACCEPTED",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			endpoint := candidatePortEndpointOver(t, testCase.command, testCase.registry, testCase.view)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d, want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			if answer := registrationOutcome(t, recorder); answer != testCase.wantAnswer {
				t.Fatalf("outcome = %q, want %q", answer, testCase.wantAnswer)
			}
		})
	}
}

// TestARegistryFailureIsNoAnswerRatherThanAGovernanceAnswer 钉住本片唯一新裁的那一格：
// 本上下文的登记用例把依赖故障折成 UNDECIDED 这一格**值**而不是 error，而它说的正是
// 「登记与否未知」——按 ADR-0022 那不是业务答案，因此走 500 NO_ANSWER_FORMED 且不带
// `outcome`，与登记 CLI 把这一格并进未决退出码同一判据。
//
// 折成 200 会让它挤进管理台「登记册治理答案」那一栏，而那一栏的续办是改内容、重试没有
// 用；未决的续办恰恰是重跑同一份。两件事的恢复动作相反，这条断言守的就是它们不合并。
func TestARegistryFailureIsNoAnswerRatherThanAGovernanceAnswer(t *testing.T) {
	endpoint := candidatePortEndpointOver(
		t,
		candidatePortCommand(t),
		stubPortsPathsRegistry{err: errors.New("库连不上")},
		stubPortsPathsView{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500（body %s）", recorder.Code, recorder.Body)
	}
	if code := problemCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
	assertNoOutcome(t, recorder)
}

func TestConfigurationRegistrationAnswersServerErrorWhenTheOrchestrationFails(t *testing.T) {
	endpoint := customshttp.NewRegisterCandidatePortEndpoint(
		configurationIntakeDouble{candidatePort: candidatePortCommand(t)},
		&configurationRegistrarDouble[application.RegisterCandidatePortCommand]{err: errors.New("库连不上")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("编排故障答 %d, want 500", recorder.Code)
	}
	if code := problemCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
	assertNoOutcome(t, recorder)
}

// TestConfigurationRegistrationRefusesToShipAnAnswerWithoutAName 证没有名字的答案不
// 上线：空 `outcome` 会被调用方当成一种新的业务结果，而它其实是实现坏了。
func TestConfigurationRegistrationRefusesToShipAnAnswerWithoutAName(t *testing.T) {
	endpoint := customshttp.NewRegisterCandidatePortEndpoint(
		configurationIntakeDouble{candidatePort: candidatePortCommand(t)},
		&configurationRegistrarDouble[application.RegisterCandidatePortCommand]{},
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

// TestOnlineConfigurationRegistrationTakesTheSameSnapshotShapeAsTheCLI 是一条编译期
// 断言：四类 Intake 契约要交出的命令，正是 registrationjson 从登记快照本体译出的那一
// 个。两侧共用一个类型参数，谁另写一份译装、或让某一类的在线口收起了与 CLI -input
// 不同的形状，这里就编译不过。
func TestOnlineConfigurationRegistrationTakesTheSameSnapshotShapeAsTheCLI(t *testing.T) {
	intake := customshttp.UnconfiguredIntake{}
	sameSnapshotShape(intake.IntakeInterpretationRuleRegistration, registrationjson.InterpretationRuleFromJSON)
	sameSnapshotShape(intake.IntakeGateCatalogRegistration, registrationjson.GateCatalogFromJSON)
	sameSnapshotShape(intake.IntakeCandidatePortRegistration, registrationjson.CandidatePortFromJSON)
	sameSnapshotShape(intake.IntakeDeclarationPathRegistration, registrationjson.DeclarationPathFromJSON)
}

// sameSnapshotShape 两个参数都不使用：它表达的是类型相等，不是一次调用。
func sameSnapshotShape[Command any](
	_ func(context.Context, *http.Request) (Command, error),
	_ func([]byte) (Command, error),
) {
}

// TestIsolatedReadIntakeCannotServeConfigurationRegistration 钉住 ADR-0078 的编译期
// 排除在写面成立：隔离读放行类型不满足任何一个登记命令 Intake 接口。这条一旦变红，
// 说明有人把隔离放行扩到了写行——那是 ADR-0085 Decision 二明文维持的原判。
func TestIsolatedReadIntakeCannotServeConfigurationRegistration(t *testing.T) {
	var intake any = customshttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(customshttp.InterpretationRuleRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进解释规则登记口")
	}
	if _, ok := intake.(customshttp.GateCatalogRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进门禁目录登记口")
	}
	if _, ok := intake.(customshttp.CandidatePortRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进候选口岸登记口")
	}
	if _, ok := intake.(customshttp.DeclarationPathRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进申报路径登记口")
	}
}

// registrationOutcome 按登记端点的封闭响应形状解出答案。用具名结构而不是 map：字段名
// 若与响应对不上，map 那种写法会静默地拿到空串并通过，而字段名正是这几条断言要钉的
// 东西。
func registrationOutcome(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode registration answer %s: %v", response.Body.Bytes(), err)
	}
	return body.Outcome
}
