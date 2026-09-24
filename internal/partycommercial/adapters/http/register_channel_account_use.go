package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 渠道账号使用授权的在线登记口与撤销口（票 party-commercial-context-gaps/01，ADR-0093）。
//
// 登记与撤销各一个端点而不合成一口。合成之后请求里就得带一个动作字段，而那个字段一旦能取
// 「撤销」，载荷里同时还带着授权正文——调用方于是有了一次连正文一起改的机会，正是 ADR-0093
// 决定六要挡的。分成两口之后撤销请求在形状上就装不下正文。

// ChannelAccountUseRegistrationIntake 把一次已认证的接入请求翻译成使用授权登记命令。
//
// 与本包其余 Intake 同样是接口（ADR-0085 Decision 二）：租户格只能来自认证结果，采信请求自称的
// tenantId 会穿透 ADR-0003 的隔离边界。真实现是操作者渠道的 OperatorRegistryIntake（ADR-0100）；
// 本族没有受控 CLI，载荷只有在线那一份（channel_account_use_payload.go）。
//
// 这一族比别族更不能开那个口子：它登的恰恰是「谁获准使用别人的渠道账号」，一个采信自报身份
// 的 Intake 等于让请求方自己声明自己被授权了，而 ADR-0039 整篇讲的就是这件事不成立。
type ChannelAccountUseRegistrationIntake interface {
	IntakeChannelAccountUseRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterChannelAccountUseCommand, error)
}

// ChannelAccountUseRevocationIntake 同上，翻译一次撤销。
type ChannelAccountUseRevocationIntake interface {
	IntakeChannelAccountUseRevocation(
		ctx context.Context,
		request *http.Request,
	) (application.RevokeChannelAccountUseCommand, error)
}

// ChannelAccountUseRegistrar 是两个端点转交的编排。两个动作共一个接口而 Intake 分两个，
// 判据同产品—渠道族：Intake 的实现方是渠道契约、逐类到位，编排的实现方是同一个用例对象与
// 同一套答案代数。
//
// 事务边界在编排侧给出（登记写口无环境事务即拒），适配器只转交与映射，不判断任何业务结果。
type ChannelAccountUseRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterChannelAccountUseCommand,
	) (application.ChannelAccountUseResult, error)
	Revoke(
		ctx context.Context,
		command application.RevokeChannelAccountUseCommand,
	) (application.ChannelAccountUseResult, error)
}

// 编译期锁缝：本端点族不新造登记语义，只消费 application 里那一个用例，签名漂移在编译期暴露。
var _ ChannelAccountUseRegistrar = (*application.RegisterChannelAccountUseHandler)(nil)

// NewRegisterChannelAccountUseEndpoint 交回使用授权登记的 HTTP 入口。
func NewRegisterChannelAccountUseEndpoint(
	intake ChannelAccountUseRegistrationIntake,
	registrar ChannelAccountUseRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeChannelAccountUseRegistration,
		registrar.Register,
		writeChannelAccountUseAnswer,
	)
}

// NewRevokeChannelAccountUseEndpoint 交回撤销的 HTTP 入口。撤销以新修订追加，端点因此没有
// 「删除」这一动作可言——本族不提供 DELETE 语义，那会让调用方以为册上那一行会消失。
func NewRevokeChannelAccountUseEndpoint(
	intake ChannelAccountUseRevocationIntake,
	registrar ChannelAccountUseRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeChannelAccountUseRevocation,
		registrar.Revoke,
		writeChannelAccountUseAnswer,
	)
}

// writeChannelAccountUseAnswer 把答案逐名转写，判据与产品—渠道族相同（`已登记`取 201、
// 治理答案取 200、散文原因随 `cause` 过线、具名格原名过线）。
//
// `业务未授权`同样取 200 而不是 4xx：答案形成了，只是答的是「不行」。给它一个错误状态码会让
// 它在传输层看起来像请求有毛病，而请求没有毛病——缺的是本系统之外的一次授权。这与本包把
// 「治理答案」与「没形成答案」分开的判据是同一条。
func writeChannelAccountUseAnswer(
	response http.ResponseWriter,
	result application.ChannelAccountUseResult,
) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := registrationAnswer{Outcome: name}
	if cause := result.Cause(); cause != nil {
		answer.Cause = cause.Error()
	}
	status := http.StatusOK
	if outcome == application.ChannelAccountUseRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
