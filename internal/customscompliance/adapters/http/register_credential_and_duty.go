package customshttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 监管凭证、税费付款协作事项、税费付款核对三册的在线登记口（票 sa-cc/07 步二；ADR-0085
// Decision 一：在线口与登记 CLI 消费同一登记用例，答案代数一致）。三册按裁决「同族一致」
// 都开在线口，写签跟着票 sa-cc/10 落的读签走。
//
// 三册分属两个答案代数，端点体因此是两份而不是一份：凭证册由 RegisterCredentialHandler
// 交回案件配置族的 CaseConfigurationOutcome，端点体直接复用 newConfigurationRegistrationEndpoint
// ——它与解释规则、口岸目录那几本是同一套案件配置族的格，另抄一份转写只会让同一条规则摊到两处；
// 协作与核对两口由 DutyPaymentReconciliationHandler 交回 DutyReconciliationResult，那族的
// 「未决」分业务未决与依赖故障两半（见 writeDutyReconciliationAnswer），配置族的转写把
// UNDECIDED 一律折成「没形成答案」，套用它会把一个形成了的业务答案报成 5xx。
//
// 外部资金事实那一口不在本文件：资金事实进 CC 只经 settlement-accounting 的采用信封
// （ADR-0137 Decision 四），人工补录口是第二个铸造点，票 sa-cc/07 裁决 2 去掉了它。

// RegulatoryCredentialRegistrationIntake 把一次已认证的接入请求翻译成凭证版本登记命令。
//
// 三个 Intake 都是接口而不是本包内的解析代码，判据同配置族（ADR-0085 Decision 二）：登记
// 快照本体的译装在 registrationjson 已有一份（与受控 CLI 的 -input 同源），但「渠道原始
// 载荷 → 登记快照」的边界与操作者认证属渠道接入契约，`PAR-INT-01` 待提供；采信报文自称
// 的租户会穿透 ADR-0003 的隔离边界。逐类分设而不合成一个按种类分派的口子，理由同上——
// 三类命令类型互不相同，合成一个就得在 Intake 里先认种类再定形状。
type RegulatoryCredentialRegistrationIntake interface {
	IntakeRegulatoryCredentialRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterCredentialCommand, error)
}

// DutyCollaborationRegistrationIntake 同上，翻译税费付款协作事项形成。
type DutyCollaborationRegistrationIntake interface {
	IntakeDutyCollaborationRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.FormDutyCollaborationCommand, error)
}

// DutyPaymentVerificationRegistrationIntake 同上，翻译税费付款核对。三轴与关联依据在命令
// 里由登记方交进来（VerifyDutyPaymentCommand），本口不从金额相等推任何一轴——真实程序
// 的关联规则属实例半边。
type DutyPaymentVerificationRegistrationIntake interface {
	IntakeDutyPaymentVerificationRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.VerifyDutyPaymentCommand, error)
}

// RegulatoryCredentialRegistrar 是凭证端点转交的登记编排。事务边界在编排之外给出，适配器
// 只转交与映射，判据同配置族的几个 Registrar。方法名 Handle 与 RegisterCredentialHandler
// 同名，装配点的事务壳照包即可。
type RegulatoryCredentialRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterCredentialCommand,
	) (application.CaseConfigurationOutcome, error)
}

// DutyCollaborationRegistrar 是协作事项端点转交的编排。方法名取用例方法名而不叫 Handle：
// 协作与核对同住一个 DutyPaymentReconciliationHandler，两个契约若都叫 Handle 就得在装配
// 点再包一层只为改名；按用例方法名分设，那个 handler 本身就同时满足两个契约，而端点
// 各自只消费其中一个——接错编译期就红。
type DutyCollaborationRegistrar interface {
	FormCollaboration(
		ctx context.Context,
		command application.FormDutyCollaborationCommand,
	) (application.DutyReconciliationResult, error)
}

// DutyPaymentVerificationRegistrar 是核对端点转交的编排，命名判据同上。
type DutyPaymentVerificationRegistrar interface {
	VerifyPayment(
		ctx context.Context,
		command application.VerifyDutyPaymentCommand,
	) (application.DutyReconciliationResult, error)
}

// NewRegisterRegulatoryCredentialEndpoint 交回监管凭证版本登记的 HTTP 入口。它登的是一版
// 不可变凭证：同身份换期限、换持有人、换额度在登记册上全是内容冲突——那是另一张凭证，
// 走另一个身份登记；额度为零在领域约定为「来源未提供」并原样落册，本口不代补。
func NewRegisterRegulatoryCredentialEndpoint(
	intake RegulatoryCredentialRegistrationIntake,
	registrar RegulatoryCredentialRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeRegulatoryCredentialRegistration, registrar.Handle)
}

// NewRegisterDutyCollaborationEndpoint 交回税费付款协作事项形成的 HTTP 入口（UC-CC-009
// 步 4–5）。义务依据两格的形状由领域把门；「哪一格都不是」是编排答的业务未决而不是
// 未受理——「缺少税费结果不能被解释为无需付款」，续办是等核定税费或真实程序的无需
// 付款依据到了重发同一份。
func NewRegisterDutyCollaborationEndpoint(
	intake DutyCollaborationRegistrationIntake,
	registrar DutyCollaborationRegistrar,
) http.Handler {
	return newDutyReconciliationEndpoint(
		intake.IntakeDutyCollaborationRegistration, registrar.FormCollaboration)
}

// NewRegisterDutyPaymentVerificationEndpoint 交回税费付款核对的 HTTP 入口（UC-CC-009
// 步 7）。两道前置未齐（资金事实未接收、协作事项未形成）与无权威依据的`待关联`都是核对
// 对这次登记作出的判断，原名过线；同三维换内容是新版本追加，该族没有「内容冲突」格，
// 本口不造。
func NewRegisterDutyPaymentVerificationEndpoint(
	intake DutyPaymentVerificationRegistrationIntake,
	registrar DutyPaymentVerificationRegistrar,
) http.Handler {
	return newDutyReconciliationEndpoint(
		intake.IntakeDutyPaymentVerificationRegistration, registrar.VerifyPayment)
}

// newDutyReconciliationEndpoint 是协作与核对两口共用的端点体。方法门与 Intake 分流与配置
// 族逐字相同，刻意不与 newConfigurationRegistrationEndpoint 再抽一层公共壳：两份端点体
// 只差最后一步的答案转写，为省那几行把「交回哪种结果、怎么转写」参数化，读的人得追两层
// 泛型才知道这一口答什么。
func newDutyReconciliationEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	register func(context.Context, Command) (application.DutyReconciliationResult, error),
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
			// 编排交回 error 即没形成答案（ADR-0022），与配置族同一格。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeDutyReconciliationAnswer(response, result)
	})
}

// dutyReconciliationRegistrationResponse 是协作与核对两口的封闭响应形状。`outcome` 取应用
// 结果枚举原名；`undecidedReason` 只随业务未决在场，说的是在等谁（形状同外部结果接收口
// 的 resultResponse）。
type dutyReconciliationRegistrationResponse struct {
	Outcome         string `json:"outcome"`
	UndecidedReason string `json:"undecidedReason,omitempty"`
}

// writeDutyReconciliationAnswer 把税费付款协作与核对族的答案逐名转写。状态码只答「有没有
// 形成答案」（ADR-0022），业务判别一律在 `outcome`：已存在、内容冲突、未受理、`待关联`、
// 两道前置未齐都是登记册对这次登记作出的判断，走 200 原名——登记 CLI 把它们归进不同的
// 退出码是因为退出码只有一个通道；HTTP 有响应体，把判断折进 4xx 会让离线客户端把一条
// 该改内容的请求当成该改请求形状的请求丢掉（ADR-0022 否决的正是 409 / 422 那条路）。
//
// 本族的「未决」分两半，这是它与配置族转写唯一的分界：
//   - 义务依据缺席（DUTY_OBLIGATION_BASIS_ABSENT）是 UC-CC-009 步 4 的业务答案——编排
//     形成了答案，答的是「等核定税费或明确无需付款依据」，走 200 带 undecidedReason；
//     折成 5xx 会让客户端按退避重发一条内容不变就不会变的请求，且把一个形成了的答案
//     报成没形成。
//   - 其余未决是存储不可用，编排把依赖故障折成这一格值而不是 error，可它说的正是
//     「登记与否未知」，与编排交回 error 同格：500 NO_ANSWER_FORMED 且不带 outcome。
//
// 业务未决那一格逐名点出而不是反过来点依赖故障：用例日后多一格未决原因，默认落进
// 「没形成答案」一侧——ADR-0022 点名判错的方向是把依赖不可用报成客户端不再重试的那类，
// 一格业务答案被多重试几次比一条该重试的请求被丢掉便宜。
//
// 201 只给这两口走得到的两个形成格。资金事实那族格在同一个枚举上，但没有任何在线口能
// 交回它们（资金事实只经 SA 采用信封进 CC，ADR-0137 Decision 四），写出来就是一段走不到的
// 分派，判据同配置族不列撤销格；其余具名格原名过线，用例多一格对新格仍然诚实。
func writeDutyReconciliationAnswer(
	response http.ResponseWriter,
	result application.DutyReconciliationResult,
) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.DutyReconciliationUndecided &&
		result.UndecidedReason() != application.DutyObligationBasisAbsent {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.CollaborationFormed, application.DutyVerificationFormed:
		status = http.StatusCreated
	}
	body := dutyReconciliationRegistrationResponse{Outcome: name}
	if reason := result.UndecidedReason().String(); reason != "" {
		body.UndecidedReason = reason
	}
	writeJSON(response, status, body)
}
