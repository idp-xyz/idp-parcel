package networkhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对七族网络目录登记端点（ADR-0085，票 admin-write-faces/02 切片 02a）证传输面：
// 方法门、未配置 Intake 403 且不构造命令、Intake 失败分流、七族各自接对了自己那一格、
// 答案逐名转写、依赖故障 5xx、没有名字的答案不上线；并钉住 ADR-0078 的排除仍然成立
// ——隔离读 Intake 装不进任何一个登记口，且这一条落在编译期。
//
// 断言助手（problemCode / assertNoOutcome / endpointBaseAt / allFamilies）复用
// query_network_catalog_test.go 既有件：族名取的就是登记口 `-kind` 的封闭七格，读写两侧
// 共用一份族名，某天七族增减时两侧一起动。

// registrationIntake 是七族登记 Intake 的合体，只在测试里存在。生产装配一族一行、逐族
// 可换（ADR-0085 Decision 二：渠道契约逐族到位，「某族还没有真 Intake」在装配点是看得见
// 的一行），这里合成一个接口是为了让同一份替身走完七个端点，不是主张生产侧该有这种类型。
type registrationIntake interface {
	networkhttp.NodeVersionRegistrationIntake
	networkhttp.ConnectionVersionRegistrationIntake
	networkhttp.LineVersionRegistrationIntake
	networkhttp.ServiceAreaVersionRegistrationIntake
	networkhttp.ServiceCalendarVersionRegistrationIntake
	networkhttp.AvailabilityAdjustmentRegistrationIntake
	networkhttp.RouteStrategyVersionRegistrationIntake
}

// recordingRegistrationIntake 交回测试预先备好的命令，并记下被调到的是哪一族的方法。
//
// 刻意不让它去解析请求：读请求取租户正是生产侧被禁的那一件（采信自报租户穿透 ADR-0003
// 的隔离边界），替身长成那个形状会让「本包没有采信实现」这句话变得要靠人去分辨。参数
// 因此全部匿名——连签名都不给「读一眼再决定」留位置。
type recordingRegistrationIntake struct {
	tenant     domain.TenantID
	node       ports.NodeDefinitionVersion
	connection ports.ConnectionDefinitionVersion
	line       ports.LineDefinitionVersion
	area       ports.ServiceAreaDefinitionVersion
	calendar   ports.ServiceCalendarDefinitionVersion
	adjustment ports.AvailabilityAdjustmentStatement
	strategy   ports.RouteStrategyDefinitionVersion
	err        error

	gotFamily string
}

func (intake *recordingRegistrationIntake) IntakeNodeVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterNodeVersionCommand, error) {
	intake.gotFamily = "node"
	if intake.err != nil {
		return application.RegisterNodeVersionCommand{}, intake.err
	}
	return application.RegisterNodeVersionCommand{TenantID: intake.tenant, Node: intake.node}, nil
}

func (intake *recordingRegistrationIntake) IntakeConnectionVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterConnectionVersionCommand, error) {
	intake.gotFamily = "connection"
	if intake.err != nil {
		return application.RegisterConnectionVersionCommand{}, intake.err
	}
	return application.RegisterConnectionVersionCommand{TenantID: intake.tenant, Connection: intake.connection}, nil
}

func (intake *recordingRegistrationIntake) IntakeLineVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterLineVersionCommand, error) {
	intake.gotFamily = "line"
	if intake.err != nil {
		return application.RegisterLineVersionCommand{}, intake.err
	}
	return application.RegisterLineVersionCommand{TenantID: intake.tenant, Line: intake.line}, nil
}

func (intake *recordingRegistrationIntake) IntakeServiceAreaVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterServiceAreaVersionCommand, error) {
	intake.gotFamily = "service-area"
	if intake.err != nil {
		return application.RegisterServiceAreaVersionCommand{}, intake.err
	}
	return application.RegisterServiceAreaVersionCommand{TenantID: intake.tenant, Area: intake.area}, nil
}

func (intake *recordingRegistrationIntake) IntakeServiceCalendarVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterServiceCalendarVersionCommand, error) {
	intake.gotFamily = "service-calendar"
	if intake.err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, intake.err
	}
	return application.RegisterServiceCalendarVersionCommand{TenantID: intake.tenant, Calendar: intake.calendar}, nil
}

func (intake *recordingRegistrationIntake) IntakeAvailabilityAdjustmentRegistration(
	context.Context, *http.Request,
) (application.RegisterAvailabilityAdjustmentCommand, error) {
	intake.gotFamily = "availability-adjustment"
	if intake.err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, intake.err
	}
	return application.RegisterAvailabilityAdjustmentCommand{TenantID: intake.tenant, Adjustment: intake.adjustment}, nil
}

func (intake *recordingRegistrationIntake) IntakeRouteStrategyVersionRegistration(
	context.Context, *http.Request,
) (application.RegisterRouteStrategyVersionCommand, error) {
	intake.gotFamily = "route-strategy"
	if intake.err != nil {
		return application.RegisterRouteStrategyVersionCommand{}, intake.err
	}
	return application.RegisterRouteStrategyVersionCommand{TenantID: intake.tenant, Strategy: intake.strategy}, nil
}

// registrarDouble 顶替登记编排：记下被调到的是哪一族，交回预置答案。
//
// unreachable 置位时被调即失败，用来证「拒在编排之前」的那几格（方法不对、未配置、
// Intake 交不出命令）一次也没走到编排。
//
// 预置答案只能是 RegisterCatalogResult 的零值——两格答案的构造器封在 application 包内，
// 外面造不出`已登记`或一个具名拒绝。这不是缺陷而是那套代数在守自己：要证转写就得走真
// 用例（见 TestNetworkCatalogRegistrationTranscribesTheUseCaseAnswersVerbatim），而零值
// 恰好就是「应用层交回了没有名字的答案」那一格。
type registrarDouble struct {
	t           *testing.T
	unreachable bool
	result      application.RegisterCatalogResult
	err         error

	gotFamily string
}

func unreachableRegistrar(t *testing.T) *registrarDouble {
	t.Helper()
	return &registrarDouble{t: t, unreachable: true}
}

func (double *registrarDouble) answer(family string) (application.RegisterCatalogResult, error) {
	if double.unreachable {
		double.t.Fatalf("%s: 被拒的登记走到了登记编排", family)
	}
	double.gotFamily = family
	return double.result, double.err
}

func (double *registrarDouble) RegisterNodeVersion(
	context.Context, application.RegisterNodeVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("node")
}

func (double *registrarDouble) RegisterConnectionVersion(
	context.Context, application.RegisterConnectionVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("connection")
}

func (double *registrarDouble) RegisterLineVersion(
	context.Context, application.RegisterLineVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("line")
}

func (double *registrarDouble) RegisterServiceAreaVersion(
	context.Context, application.RegisterServiceAreaVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("service-area")
}

func (double *registrarDouble) RegisterServiceCalendarVersion(
	context.Context, application.RegisterServiceCalendarVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("service-calendar")
}

func (double *registrarDouble) RegisterAvailabilityAdjustment(
	context.Context, application.RegisterAvailabilityAdjustmentCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("availability-adjustment")
}

func (double *registrarDouble) RegisterRouteStrategyVersion(
	context.Context, application.RegisterRouteStrategyVersionCommand,
) (application.RegisterCatalogResult, error) {
	return double.answer("route-strategy")
}

// stubCatalogRegistry 是写入口替身：七个方法同答一格。写入只有成功与错误两格——三道
// 防线（重复版本号、未闭区间并存、已闭区间重叠）都判给库上约束（ADR-0068 Consequences），
// 端口本身没有出格答案，所以这里只需要一个错误开关。
type stubCatalogRegistry struct{ err error }

func (stub stubCatalogRegistry) RegisterNodeVersion(
	context.Context, domain.TenantID, ports.NodeDefinitionVersion,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterConnectionVersion(
	context.Context, domain.TenantID, ports.ConnectionDefinitionVersion,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterLineVersion(
	context.Context, domain.TenantID, ports.LineDefinitionVersion,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterServiceAreaVersion(
	context.Context, domain.TenantID, ports.ServiceAreaDefinitionVersion,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterServiceCalendarVersion(
	context.Context, domain.TenantID, ports.ServiceCalendarDefinitionVersion,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterAvailabilityAdjustment(
	context.Context, domain.TenantID, ports.AvailabilityAdjustmentStatement,
) error {
	return stub.err
}

func (stub stubCatalogRegistry) RegisterRouteStrategyVersion(
	context.Context, domain.TenantID, ports.RouteStrategyDefinitionVersion,
) error {
	return stub.err
}

// registrationEndpoints 把七个登记端点各构一次，键取登记口 `-kind` 的族名。
//
// 逐族走一遍而不是只测一族：端点体由 newRegistrationEndpoint 共用，但七个构造函数各自
// 把哪个 Intake 方法接到哪个编排方法上是逐族写的，接错那一格只有逐族走过才看得见。
func registrationEndpoints(
	intake registrationIntake,
	registrar networkhttp.CatalogRegistrar,
) map[string]http.Handler {
	return map[string]http.Handler{
		"node":                    networkhttp.NewRegisterNodeVersionEndpoint(intake, registrar),
		"connection":              networkhttp.NewRegisterConnectionVersionEndpoint(intake, registrar),
		"line":                    networkhttp.NewRegisterLineVersionEndpoint(intake, registrar),
		"service-area":            networkhttp.NewRegisterServiceAreaVersionEndpoint(intake, registrar),
		"service-calendar":        networkhttp.NewRegisterServiceCalendarVersionEndpoint(intake, registrar),
		"availability-adjustment": networkhttp.NewRegisterAvailabilityAdjustmentEndpoint(intake, registrar),
		"route-strategy":          networkhttp.NewRegisterRouteStrategyVersionEndpoint(intake, registrar),
	}
}

// validRegistrationIntake 备一份七族都立得住的最小登记行。身份全取 SYN- 前缀（隔离合成
// 只记 `S`），内容不多不少：目录内容属实例半边（PAR-NET-01..15 待提供），测试也不替登记
// 方补值——多补一个字段，「受理门放行是因为这一格真的在场」就变成了「因为测试替它填了」。
func validRegistrationIntake(t *testing.T) *recordingRegistrationIntake {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TEN-NR02A")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	return &recordingRegistrationIntake{
		tenant: tenant,
		node: ports.NodeDefinitionVersion{
			Code:             "SYN-NODE-HUB",
			Version:          1,
			BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom:    endpointBaseAt,
		},
		connection: ports.ConnectionDefinitionVersion{
			Code:             "SYN-CONN-A-B",
			Version:          1,
			FromNode:         "SYN-NODE-A",
			ToNode:           "SYN-NODE-B",
			BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom:    endpointBaseAt,
		},
		line: ports.LineDefinitionVersion{
			Code:             "SYN-LINE-1",
			Version:          1,
			Segments:         []string{"SYN-CONN-A-B"},
			BusinessTimezone: "Asia/Shanghai",
			ApplicableScope:  "SYN-SCOPE-1",
			EffectiveFrom:    endpointBaseAt,
		},
		area: ports.ServiceAreaDefinitionVersion{
			Code:          "SYN-AREA-1",
			Version:       1,
			EffectiveFrom: endpointBaseAt,
		},
		calendar: ports.ServiceCalendarDefinitionVersion{
			TargetKind:    ports.TargetNode,
			TargetCode:    "SYN-NODE-HUB",
			Version:       1,
			EffectiveFrom: endpointBaseAt,
		},
		adjustment: ports.AvailabilityAdjustmentStatement{
			Code:        "SYN-ADJ-1",
			Version:     1,
			TargetKind:  ports.TargetLine,
			TargetCode:  "SYN-LINE-1",
			Kind:        ports.AdjustmentSuspension,
			Source:      "SYN-NET-OPS/EVT-1",
			EffectiveAt: endpointBaseAt,
		},
		strategy: ports.RouteStrategyDefinitionVersion{
			Code:            "SYN-STRATEGY-1",
			Version:         1,
			ApplicableScope: "SYN-SCOPE-1",
			EffectiveFrom:   endpointBaseAt,
		},
	}
}

// registrationRequest 造一次登记请求。路径取一个中性探针值：登记端点自己不读路径，
// 而对外契约上的那一个由端点表裁（ADR-0055 那句「路径今天还不是任何租户的对外契约」），
// 这份测试不替它先定一个。
func registrationRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，七族的编排一个也不被触到。
func TestNetworkCatalogRegistrationRefusesNonPostMethods(t *testing.T) {
	for family, endpoint := range registrationEndpoints(
		validRegistrationIntake(t), unreachableRegistrar(t),
	) {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))

		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: GET 答 %d, want %d", family, response.Code, http.StatusMethodNotAllowed)
		}
		if allow := response.Header().Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s: Allow = %q, want POST", family, allow)
		}
	}
}

// Covers: ADR-0055/ADR-0085 Decision 一 — 未配置 Intake 对七族同答 403，且对请求零读取：
// 自报租户与载荷换不来任何差别，编排一次也不会被调到，答复里没有 outcome（那是业务答案，
// 这里没有形成任何答案）。
func TestUnconfiguredIntakeRefusesEveryRegistrationFamilyIdentically(t *testing.T) {
	endpoints := registrationEndpoints(networkhttp.UnconfiguredIntake{}, unreachableRegistrar(t))
	if len(endpoints) != len(allFamilies) {
		t.Fatalf("登记端点 %d 个，族名封闭集 %d 格", len(endpoints), len(allFamilies))
	}

	var baseline string
	for _, family := range allFamilies {
		endpoint, present := endpoints[family]
		if !present {
			t.Fatalf("%s 族没有登记端点", family)
		}
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, registrationRequest(
			`{"tenant_id":"TENANT-9","code":"NODE-9","version":9}`,
		))

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d", family, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", family, got)
		}
		assertNoOutcome(t, response)
		if baseline == "" {
			baseline = response.Body.String()
		} else if response.Body.String() != baseline {
			t.Fatalf("%s: 未配置答复与其他族不一致：%s vs %s", family, response.Body.String(), baseline)
		}
	}
}

// Covers: ADR-0085 Decision 一/ADR-0068 Decision 五 — 七个构造函数各把自己那一族的
// Intake 接到同族的编排方法上：接错一格（拿节点的 Intake 去调连接的登记）在这里就红。
//
// 本用例只证接线与依赖故障那一格，不证答案代数：编排替身造不出具名答案（构造器封在
// application 包内），转写由走真用例的那一条覆盖。
func TestEachRegistrationEndpointDispatchesItsOwnFamily(t *testing.T) {
	for _, family := range allFamilies {
		intake := validRegistrationIntake(t)
		registrar := &registrarDouble{err: errors.New("库连不上")}
		endpoint, present := registrationEndpoints(intake, registrar)[family]
		if !present {
			t.Fatalf("%s 族没有登记端点", family)
		}
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, registrationRequest(`{}`))

		if intake.gotFamily != family {
			t.Fatalf("%s 的端点调到了 %q 族的 Intake", family, intake.gotFamily)
		}
		if registrar.gotFamily != family {
			t.Fatalf("%s 的端点调到了 %q 族的登记编排", family, registrar.gotFamily)
		}
		// 登记与否未知不是业务答案（ADR-0022/ADR-0029）：留队重试，成因去查服务端记录。
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("%s: 依赖故障答 %d, want %d", family, response.Code, http.StatusInternalServerError)
		}
		if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
			t.Fatalf("%s: code = %q, want NO_ANSWER_FORMED", family, got)
		}
		assertNoOutcome(t, response)
	}
}

// Covers: ADR-0022/ADR-0029 — Intake 的三格各自映射：未配置 403（上一用例逐族证过）、
// 构造不出命令 400、其余是 500 INTAKE_FAILED，且都拒在编排之前。
//
// 这两格只走一族：分流由七族共用的 writeRegistrationIntakeProblem 给出，逐族再走一遍
// 只是把同一段代码走七次。族间的差别在接线上，那由上一个用例逐族盯着。
func TestRegistrationIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := validRegistrationIntake(t)
	malformed.err = fmt.Errorf("载荷缺格: %w", networkhttp.ErrMalformedRequest)
	endpoint := networkhttp.NewRegisterNodeVersionEndpoint(malformed, unreachableRegistrar(t))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, registrationRequest(`{"code":`))
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("畸形请求：%d %s", response.Code, response.Body.String())
	}

	failing := validRegistrationIntake(t)
	failing.err = errors.New("认证后端寄了")
	endpoint = networkhttp.NewRegisterNodeVersionEndpoint(failing, unreachableRegistrar(t))
	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, registrationRequest(`{}`))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("Intake 故障：%d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0085 Decision 一/ADR-0022 — 答案逐名过线：已登记走 201，受理门指名的拒绝
// 走 200 并带上拒绝理由原名。拒绝不折成 4xx——它是登记用例对这一笔作出的判断，压成
// 「请求不合法」登记方就不知道该改内容还是改请求形状，而后者连改什么都指不出来。
//
// 走真用例而不是伪造结果值：答案代数由 application 包内的构造器封着，伪造得出的那份与
// 生产的是不是同一份，本来就是这条断言要证的东西。逐格拒绝理由取四族各自的门（时区、
// 方向、段链、来源），因为它们的续办动作各不相同——折成一格会让人去补错东西。
func TestNetworkCatalogRegistrationTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		family     string
		spoil      func(*recordingRegistrationIntake)
		wantStatus int
		wantAnswer string
		wantReason string
	}{
		{
			name:       "节点版本已登记",
			family:     "node",
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "缺业务时区指名缺的是时区",
			family:     "node",
			spoil:      func(intake *recordingRegistrationIntake) { intake.node.BusinessTimezone = "" },
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "TIMEZONE_MISSING",
		},
		{
			name:       "两端相同没有方向可言",
			family:     "connection",
			spoil:      func(intake *recordingRegistrationIntake) { intake.connection.ToNode = intake.connection.FromNode },
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "ENDPOINTS_NOT_DISTINCT",
		},
		{
			name:       "零段的线路不成链",
			family:     "line",
			spoil:      func(intake *recordingRegistrationIntake) { intake.line.Segments = nil },
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "SEGMENTS_MISSING",
		},
		{
			name:       "调整缺来源说不出凭什么",
			family:     "availability-adjustment",
			spoil:      func(intake *recordingRegistrationIntake) { intake.adjustment.Source = "" },
			wantStatus: http.StatusOK,
			wantAnswer: "REFUSED",
			wantReason: "SOURCE_MISSING",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registration, err := application.NewNetworkCatalogRegistration(stubCatalogRegistry{})
			if err != nil {
				t.Fatalf("构造登记用例：%v", err)
			}
			intake := validRegistrationIntake(t)
			if testCase.spoil != nil {
				testCase.spoil(intake)
			}
			endpoint, present := registrationEndpoints(intake, registration)[testCase.family]
			if !present {
				t.Fatalf("%s 族没有登记端点", testCase.family)
			}
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, registrationRequest(`{}`))

			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d（body %s）",
					response.Code, testCase.wantStatus, response.Body.String())
			}
			// 按字段名解出原始 JSON 而不是解成结构：字段名本身是这几条断言要钉的东西，
			// 而「登记成了就没有拒绝理由」这一格只有看键在不在场才证得到——解成结构时
			// 缺席与空串不可分辨。
			fields := map[string]json.RawMessage{}
			if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
				t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
			}
			if string(fields["outcome"]) != `"`+testCase.wantAnswer+`"` {
				t.Fatalf("outcome 走样：%s", response.Body.String())
			}
			wantReason := ""
			if testCase.wantReason != "" {
				wantReason = `"` + testCase.wantReason + `"`
			}
			if string(fields["refusalReason"]) != wantReason {
				t.Fatalf("refusalReason = %s, want %q（登记成了就不该有这一格）",
					fields["refusalReason"], wantReason)
			}
		})
	}
}

// Covers: 没有名字的答案不上线——空 `outcome` 会被客户端当成一种新的业务结果，而这一格
// 只可能是实现坏了（用例交回了它自己都不认识的结果），登记方拿一个没有指名的答复什么也
// 补不了。
//
// 这里能造出的只有零值结果那一路。「被拒却说不出理由」走的是同一个错误码，但那一格从包
// 外造不出来：拒绝理由与结果两格由 application 的构造器一起封着，真用例的每次拒绝都带着
// 理由——这正是它想守的东西。
func TestNetworkCatalogRegistrationRefusesToShipAnAnswerWithoutAName(t *testing.T) {
	endpoint := networkhttp.NewRegisterNodeVersionEndpoint(
		validRegistrationIntake(t), &registrarDouble{},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, registrationRequest(`{}`))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("无名答案答 %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "UNNAMED_OUTCOME" {
		t.Fatalf("code = %q, want UNNAMED_OUTCOME", got)
	}
	assertNoOutcome(t, response)
}

// registrationIntakeProbe 把隔离读放行（ADR-0078）与一份实现了七族登记方法的替身嵌在
// **同一层**：放行一旦长出其中任何一个登记方法，两个同名方法在同一深度冲突、提升被取消，
// 下面那组断言当场编不过。
//
// 这是「某类型不实现某接口」在 Go 里唯一落得到编译期的写法——约束只表达得了正向满足，
// 而反向的 `interface.(Concrete)` 恰恰在排除成立时才编不过，方向是反的。IsolatedOperations-
// ReadIntake 的注释说「放行装不进登记端点由编译期决定」，ADR-0085 Decision 二也把这条排除
// 记为明文维持的原判；那句话要么有一处编译期凭据，要么只是注释。
type registrationIntakeProbe struct {
	networkhttp.IsolatedOperationsReadIntake
	*recordingRegistrationIntake
}

var (
	_ networkhttp.NodeVersionRegistrationIntake            = registrationIntakeProbe{}
	_ networkhttp.ConnectionVersionRegistrationIntake      = registrationIntakeProbe{}
	_ networkhttp.LineVersionRegistrationIntake            = registrationIntakeProbe{}
	_ networkhttp.ServiceAreaVersionRegistrationIntake     = registrationIntakeProbe{}
	_ networkhttp.ServiceCalendarVersionRegistrationIntake = registrationIntakeProbe{}
	_ networkhttp.AvailabilityAdjustmentRegistrationIntake = registrationIntakeProbe{}
	_ networkhttp.RouteStrategyVersionRegistrationIntake   = registrationIntakeProbe{}
)

// Covers: ADR-0078/ADR-0085 Decision 二 — 隔离读放行装不进七个登记口中的任何一个。
//
// 与上面那组编译期断言换个方向说同一件事：那一组守的是「放行长出了登记方法」，这一条守的
// 是「登记 Intake 契约被改成了放行已经满足的形状」——后者不会引起同深度冲突，编译期看不见。
// 两条都在，这条排除才两个方向都盯着；任一条变红都说明有人把隔离放行扩到了写行。
func TestIsolatedReadIntakeCannotServeNetworkCatalogRegistration(t *testing.T) {
	var intake any = networkhttp.IsolatedOperationsReadIntake{}
	// 七格逐个断言而不是断言七族的合体：合体只要求「七个方法不全在」，放行长出其中
	// 一族的登记方法时它照样通过，而漏掉的那一族正是写行被撬开的那一族。
	if _, ok := intake.(networkhttp.NodeVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进节点版本登记口")
	}
	if _, ok := intake.(networkhttp.ConnectionVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进连接版本登记口")
	}
	if _, ok := intake.(networkhttp.LineVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进线路版本登记口")
	}
	if _, ok := intake.(networkhttp.ServiceAreaVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进服务区域版本登记口")
	}
	if _, ok := intake.(networkhttp.ServiceCalendarVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进服务日历版本登记口")
	}
	if _, ok := intake.(networkhttp.AvailabilityAdjustmentRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进可用性调整登记口")
	}
	if _, ok := intake.(networkhttp.RouteStrategyVersionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进路由策略版本登记口")
	}
	if _, ok := intake.(networkhttp.CatalogueQueryIntake); !ok {
		t.Fatal("隔离读 Intake 连查阅口都装不进了：这条排除该证的是写面，不是把读面也关掉")
	}
}
