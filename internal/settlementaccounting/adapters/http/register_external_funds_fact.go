package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 外部资金事实采用与更正的在线登记口（票 sa-cc/31，27 裁决 2 第二步；ADR-0085 决定一：在线口与登记 CLI
// `parcel-settlement-register` 消费同一登记用例、答案代数一致）。这是本包第一份命令文件：此前 settlementhttp
// 只有四张管理台页的目录查阅端点。
//
// 外部资金事实进产品只经本上下文采用这一口（ADR-0137 决定四）：`customs-compliance` 的税费付款核对等的那封
// 采用信封由采用编排在同一笔事务里交出，CC 侧不开第二个铸造或补录入口。本口开的是人工 / 受控写面的门，
// 不自动采用——任何回调 / 文件到达都不在这里（UC-SA-005「不因接收回调自动采用」）。

// ExternalFundsFactRegistrationIntake 把一次已认证的接入请求翻译成外部资金事实首版的采用命令。
//
// 两个 Intake 都是接口而不是本包内的解析代码，判据同 customshttp 的登记口（ADR-0085 决定二）：登记输入本体
// 的译装在 adapters/registrationjson 已有一份（与受控 CLI 的 -input 同源，本包不得另写），但「渠道原始载荷 →
// 登记输入」的边界与操作者认证属渠道接入契约，`PAR-INT-01` 待提供；采信报文自称的租户会穿透 ADR-0003 的隔离
// 边界。逐类分设而不合成一个按种类分派的口子：两类命令类型互不相同，合成一个就得在 Intake 里先认种类再定形状。
type ExternalFundsFactRegistrationIntake interface {
	IntakeExternalFundsFactRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.AdoptFundsFactCommand, error)
}

// ExternalFundsFactCorrectionRegistrationIntake 同上，翻译一次外部更正的采用：同一事实回指当前链头的新版本，
// 只有金额变；来源、付款人、种类、币种与发生时刻从链头照抄，命令上不接（application.CorrectFundsFactCommand 头注）。
type ExternalFundsFactCorrectionRegistrationIntake interface {
	IntakeExternalFundsFactCorrectionRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.CorrectFundsFactCommand, error)
}

// ExternalFundsFactRegistrar 是采用端点转交的编排。事务边界在编排之外给出（装配点的事务壳），适配器只转交与
// 映射。方法名取用例方法名而不叫 Handle（判据同 customshttp 的 DutyCollaborationRegistrar 头注）：采用与更正同住
// 一个 MapExternalFundsHandler，两个契约若都叫 Handle 就得在装配点再包一层只为改名；按用例方法名分设，那个
// handler 本身就同时满足两个契约，而端点各自只消费其中一个——接错编译期就红。
type ExternalFundsFactRegistrar interface {
	AdoptFact(
		ctx context.Context,
		command application.AdoptFundsFactCommand,
	) (application.FundsResult, error)
}

// ExternalFundsFactCorrectionRegistrar 是更正端点转交的编排，命名判据同上。
type ExternalFundsFactCorrectionRegistrar interface {
	CorrectFact(
		ctx context.Context,
		command application.CorrectFundsFactCommand,
	) (application.FundsResult, error)
}

// NewRegisterExternalFundsFactEndpoint 交回外部资金事实首版采用的 HTTP 入口。它采用的是一条事实的首版：
// 同版本字面异内容、同事实第二个首版都是内容冲突（绝不覆盖）；更正走另一口回指链头。
func NewRegisterExternalFundsFactEndpoint(
	intake ExternalFundsFactRegistrationIntake,
	registrar ExternalFundsFactRegistrar,
) http.Handler {
	return newFundsRegistrationEndpoint(intake.IntakeExternalFundsFactRegistration, registrar.AdoptFact)
}

// NewRegisterExternalFundsFactCorrectionEndpoint 交回外部更正采用的 HTTP 入口。回指非链头、更正一条未采用的
// 事实、回指自己都是`未受理`——提交矛盾，改内容再来，不是重试。
func NewRegisterExternalFundsFactCorrectionEndpoint(
	intake ExternalFundsFactCorrectionRegistrationIntake,
	registrar ExternalFundsFactCorrectionRegistrar,
) http.Handler {
	return newFundsRegistrationEndpoint(intake.IntakeExternalFundsFactCorrectionRegistration, registrar.CorrectFact)
}

// newFundsRegistrationEndpoint 是采用与更正两口共用的端点体：方法门 → Intake 分流 → 编排 → 答案转写。
// Intake 三格（未配置 / 畸形 / 故障）是接入渠道这一层的状态，与读面同一套映射，不按端点族另立。
func newFundsRegistrationEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	register func(context.Context, Command) (application.FundsResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		result, err := register(request.Context(), command)
		if err != nil {
			// 编排交回 error 即没形成答案（ADR-0022）：事务壳已回滚，行没落、信封没出。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeFundsAnswer(response, result)
	})
}

// 传输层错误码（续 catalogue_intake.go 那一组）。业务判别走 `outcome`，这里只说明为什么没有 `outcome`。
const (
	// codeUnnamedOutcome：用例交回一个 String() 为空的格是实现坏了，不是业务答案。
	codeUnnamedOutcome = "UNNAMED_OUTCOME"
	// codeHandoffNotSent：记录已落、向 CC 交的采用信封未出（FundsResult.FundsHandoffReference 非空）。
	// 判据见 writeFundsAnswer 头注。
	codeHandoffNotSent = "HANDOFF_NOT_SENT"
)

// fundsRegistrationResponse 是采用与更正两口的封闭响应形状。`outcome` 取 application.FundsOutcome 原名；
// `undecidedReason` 只随业务未决在场，说的是在等谁（形状同 customshttp 的协作 / 核对两口）。
type fundsRegistrationResponse struct {
	Outcome         string `json:"outcome"`
	UndecidedReason string `json:"undecidedReason,omitempty"`
}

// writeFundsAnswer 把 application.FundsOutcome 那一族的答案逐名转写。状态码只答「有没有形成答案」（ADR-0022），
// 业务判别一律在 `outcome`：已存在、内容冲突、`未受理`都是登记册对这次登记作出的判断，走 200 原名——受控 CLI
// 把它们归进不同的退出码（fundsAnswer）是因为退出码只有一个通道；HTTP 有响应体，把判断折进 4xx 会让离线客户端
// 把一条该改内容的请求当成该改请求形状的请求丢掉。201 只给两口走得到的形成格 FUNDS_FACT_ADOPTED；映射与核销
// 那几格在同一个枚举上，但没有任何在线口能交回它们，不列分派——其余具名格原名过线，用例多一格对新格仍然诚实。
//
// 本族的「未决」今天只有一种来路：FundsUndecidedReason 的三格全是存储不可用——依赖故障，编排把它折成一格值而
// 不是 error，可它说的正是「登记与否未知」，与编排交回 error 同格：500 NO_ANSWER_FORMED 且不带 outcome。
// businessUndecidedFunds 逐名点出业务未决而不是反过来点依赖故障（判据同 customshttp 的 businessUndecided）：用例
// 日后多一格未决原因，默认落进「没形成答案」一侧——ADR-0022 点名判错的方向是把依赖不可用报成客户端不再重试
// 的那类。今天那张名单是空的，函数仍立在这里，让「哪几格是业务答案」这个判断有一处可写。
//
// **行已落、信封未出**（FundsHandoffReference 非空）不与已采用同格，判据与 fundsAnswer 同一条：事实已采用是
// 真的，但 CC 等的那封没出去，它需要人重发同一份补发同一封（Outbox 按认领键吞重，重放交的是同一封）。HTTP 上
// 「重发同一份」正是 5xx 的恢复动作，所以折成 500 并带专名 HANDOFF_NOT_SENT、不带 outcome——答 2xx 带原名会让
// 一个只看状态与 outcome 的调用方（管理台的登记签正是这样读的）把 CC 永远等不到的那封当成已出。续办引用本身
// 不进响应：它由租户、事实引用与版本拼成，调用方手里的正是这三样；专名错误码已经把「该做什么」说清了。
func writeFundsAnswer(response http.ResponseWriter, result application.FundsResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.FundsUndecided && !businessUndecidedFunds(result.UndecidedReason()) {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.FundsHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.FundsFactAdopted {
		status = http.StatusCreated
	}
	body := fundsRegistrationResponse{Outcome: name}
	if reason := result.UndecidedReason().String(); reason != "" {
		body.UndecidedReason = reason
	}
	writeJSON(response, status, body)
}

// businessUndecidedFunds 点名本族里「形成了答案」的未决原因——重发同一份不会变、要等别人补一样东西的那几格；
// 表外的未决一律按依赖故障处理（writeFundsAnswer 头注）。今天 FundsUndecidedReason 的每一格都是存储不可用，
// 名单为空。
func businessUndecidedFunds(application.FundsUndecidedReason) bool {
	return false
}
