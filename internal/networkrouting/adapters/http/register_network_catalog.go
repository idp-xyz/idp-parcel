package networkhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
)

// codeUnnamedOutcome 同命令面先例：应用层交回没有名字的结果是编程错误，不是业务
// 答案，不能带空 `outcome` 上线——空字符串会被客户端当成一种新的业务结果。被拒却
// 说不出差哪格同归此码：两者都是实现坏了，续办动作同为去查服务端记录，而登记方
// 拿一个没有指名的拒绝什么也补不了。
const codeUnnamedOutcome = "UNNAMED_OUTCOME"

// 七族各一个登记 Intake 接口，与登记口 `-kind` 的封闭集一一对应。
//
// 它们是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：受理门
// 在用例侧已实现，但「渠道原始载荷 → 登记行」的翻译与操作者认证归操作者渠道
// （ADR-0100），其真 Intake 未就位。这里尤其不能照抄受控登记口的译装——那一份从载荷里读租户，
// 操作员在库网内手跑时那是治理动作，搬到在线口上就是采信自报租户，穿透 ADR-0003 的
// 隔离边界。命令的租户格与行内容格本就分开（见 application 的七个命令类型），真渠道
// Intake 要把认证结果填进租户格、只从载荷取行内容。未决期间本包不带任何实现，包括
// 「开发用」的采信头部版本。
//
// 不合并成一个带 kind 参数的接口：一族一个接口，渠道契约就能逐族到位而不必等齐七族，
// 且「某族还没有真 Intake」在装配点是看得见的一行，合并后会变成一个类型内部的分支。
type NodeVersionRegistrationIntake interface {
	IntakeNodeVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterNodeVersionCommand, error)
}

type ConnectionVersionRegistrationIntake interface {
	IntakeConnectionVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterConnectionVersionCommand, error)
}

type LineVersionRegistrationIntake interface {
	IntakeLineVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterLineVersionCommand, error)
}

type ServiceAreaVersionRegistrationIntake interface {
	IntakeServiceAreaVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterServiceAreaVersionCommand, error)
}

type ServiceCalendarVersionRegistrationIntake interface {
	IntakeServiceCalendarVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterServiceCalendarVersionCommand, error)
}

type AvailabilityAdjustmentRegistrationIntake interface {
	IntakeAvailabilityAdjustmentRegistration(ctx context.Context, request *http.Request) (application.RegisterAvailabilityAdjustmentCommand, error)
}

type RouteStrategyVersionRegistrationIntake interface {
	IntakeRouteStrategyVersionRegistration(ctx context.Context, request *http.Request) (application.RegisterRouteStrategyVersionCommand, error)
}

// CatalogRegistrar 是七个登记端点转交的登记编排。七族共一个接口而 Intake 分七个，
// 因为两侧被谁实现不同：Intake 的实现方是渠道契约，逐族到位；编排的实现方是登记
// 用例本身，它对七族是同一个对象、同一套答案代数（`RegisterCatalogResult`）。
//
// 事务边界在编排侧给出（登记写口无环境事务即拒，先例同登记 CLI 的 execute），适配器
// 只转交与映射，不判断任何业务结果。方法各自具名而不是七个同名 `Handle`：接错一格
// 在端点构造函数那一行就读得出来。
type CatalogRegistrar interface {
	RegisterNodeVersion(ctx context.Context, command application.RegisterNodeVersionCommand) (application.RegisterCatalogResult, error)
	RegisterConnectionVersion(ctx context.Context, command application.RegisterConnectionVersionCommand) (application.RegisterCatalogResult, error)
	RegisterLineVersion(ctx context.Context, command application.RegisterLineVersionCommand) (application.RegisterCatalogResult, error)
	RegisterServiceAreaVersion(ctx context.Context, command application.RegisterServiceAreaVersionCommand) (application.RegisterCatalogResult, error)
	RegisterServiceCalendarVersion(ctx context.Context, command application.RegisterServiceCalendarVersionCommand) (application.RegisterCatalogResult, error)
	RegisterAvailabilityAdjustment(ctx context.Context, command application.RegisterAvailabilityAdjustmentCommand) (application.RegisterCatalogResult, error)
	RegisterRouteStrategyVersion(ctx context.Context, command application.RegisterRouteStrategyVersionCommand) (application.RegisterCatalogResult, error)
}

// 编译期锁缝：登记编排的形状与真用例保持一致——本端点族不新造登记语义，只消费
// ADR-0068 钉住的那一个登记用例，签名漂移在编译期暴露。判据同查阅侧锁到 ports 读面。
var _ CatalogRegistrar = (*application.NetworkCatalogRegistration)(nil)

// 七个登记端点（ADR-0085 Decision 一：登记写面进端点表带未配置格；在线口与受控登记
// 口消费同一登记用例，答案代数一致）。
//
// 一族一个端点而查阅侧七族共用 `/network-catalog` 的 family 分派，两侧不同形是因为
// 分派参数的位置不同：查阅的 family 只选读哪一册，请求体是空的；登记的族决定请求体
// 是哪一种行，把它塞进查询参数就等于让一个端点收七种互不兼容的载荷。用例侧也是
// 「七族各一个方法、独立成败」（ADR-0068 Decision 五按笔推进），端点与它一一对应。
func NewRegisterNodeVersionEndpoint(
	intake NodeVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeNodeVersionRegistration, registrar.RegisterNodeVersion)
}

func NewRegisterConnectionVersionEndpoint(
	intake ConnectionVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeConnectionVersionRegistration, registrar.RegisterConnectionVersion)
}

func NewRegisterLineVersionEndpoint(
	intake LineVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeLineVersionRegistration, registrar.RegisterLineVersion)
}

func NewRegisterServiceAreaVersionEndpoint(
	intake ServiceAreaVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeServiceAreaVersionRegistration, registrar.RegisterServiceAreaVersion)
}

func NewRegisterServiceCalendarVersionEndpoint(
	intake ServiceCalendarVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeServiceCalendarVersionRegistration, registrar.RegisterServiceCalendarVersion)
}

func NewRegisterAvailabilityAdjustmentEndpoint(
	intake AvailabilityAdjustmentRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeAvailabilityAdjustmentRegistration, registrar.RegisterAvailabilityAdjustment)
}

func NewRegisterRouteStrategyVersionEndpoint(
	intake RouteStrategyVersionRegistrationIntake,
	registrar CatalogRegistrar,
) http.Handler {
	return newRegistrationEndpoint(intake.IntakeRouteStrategyVersionRegistration, registrar.RegisterRouteStrategyVersion)
}

// newRegistrationEndpoint 是七族共用的传输形状：方法门 → Intake → 编排 → 逐名转写。
// 收两个函数值而不是收 kind 再自己分派——七族的命令类型互不相同，泛型参数由方法值
// 推出，接错一格编不过。
func newRegistrationEndpoint[Command any](
	intake func(ctx context.Context, request *http.Request) (Command, error),
	register func(ctx context.Context, command Command) (application.RegisterCatalogResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		result, err := register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障，或撞上库上那两道防线）不是业务答案：状态码只报
			// 「没形成答案」（ADR-0022），调用侧据以重试或转人工，成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, result)
	})
}

// registrationResponse 是登记端点的封闭响应形状。`outcome` 与 `refusalReason` 都取
// 应用结果枚举原名——与受控登记口的答案代数同源：受理门指名的拒绝是答案不是失败，
// 传输层不合并、不改名，也没有「其他」这一格。
//
// `refusalReason` 只在被拒时在场：登记成了就没有「差哪格」可言，给一个空串会让调用侧
// 先判字段有没有值再判答案。
type registrationResponse struct {
	Outcome       string `json:"outcome"`
	RefusalReason string `json:"refusalReason,omitempty"`
}

// writeRegistrationIntakeProblem 与目录查阅侧的分流判据相同（403 未配置 / 400 畸形 /
// 5xx Intake 故障），单立一份不复用查阅侧助手：两侧各随自己的端点族演化，命令面的
// 分流将来要加载荷规范化摘要一格（ADR-0055 Decision 五），不该牵动查阅行。
func writeRegistrationIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrOperatorCredentialRejected) {
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
		return
	}
	if errors.Is(err, ErrOperatorNotGranted) {
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
		return
	}
	if errors.Is(err, ErrIdentityDependencyUnavailable) {
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// writeRegistrationAnswer 把应用答案逐名转写。已登记取 201、受理门拒绝取 200：拒绝
// 是服务端形成的业务答案（ADR-0022），折成 4xx 会让调用侧把一件「改内容再来」的事
// 读成「请求本身不合法」，而后者连改什么都指不出来。
func writeRegistrationAnswer(response http.ResponseWriter, result application.RegisterCatalogResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := registrationResponse{Outcome: name}
	if outcome == application.CatalogRegistrationRefused {
		reason := result.RefusalReason().String()
		if reason == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		body.RefusalReason = reason
	}
	status := http.StatusOK
	if outcome == application.CatalogRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
