package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 服务形态与产品—渠道映射的在线登记口（ADR-0085 Decision 一；票 admin-write-faces/02
// 商业片），与受控 CLI 的 register-products 同源。
//
// 两类各一个端点而不合成一口：形态挂在发布登记册的形态切面上、映射是自己的登记册
// （ADR-0050：形态缺席不使解析退化，两册各自为政），命令类型与引用检查也各是各的。
// 合成一口就得先认种类再定形状，那会让「形态还没有真 Intake」在装配点看不出来。

// ServiceProductFormRegistrationIntake 把一次已认证的接入请求翻译成服务形态登记命令。
//
// 两个 Intake 都是接口（ADR-0085 Decision 二，机制同 ADR-0055）：引用检查与领域构造门在用例侧，
// 租户格只能来自认证结果——采信批文自称的 tenantId 会穿透 ADR-0003 的隔离边界（判据与参与方身份
// 那一族同一条，不复述）。真实现是操作者渠道的 OperatorRegistryIntake（ADR-0100），译装与受控
// CLI 共用 registrationjson 那一份。
type ServiceProductFormRegistrationIntake interface {
	IntakeServiceProductFormRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterServiceProductFormCommand, error)
}

// ProductChannelMappingRegistrationIntake 同上，翻译产品—渠道映射修订登记。
type ProductChannelMappingRegistrationIntake interface {
	IntakeProductChannelMappingRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterProductChannelMappingCommand, error)
}

// ProductChannelRegistrar 是两个端点转交的登记编排。两类共一个接口而 Intake 分两个，
// 判据同参与方身份族：Intake 的实现方是渠道契约、逐类到位，编排的实现方是登记用例
// 本身、对两类是同一个对象与同一套答案代数（`ProductChannelResult`）。
//
// 事务边界在编排侧给出（登记写口无环境事务即拒，形照登记 CLI 的 execute），适配器只
// 转交与映射，不判断任何业务结果。
type ProductChannelRegistrar interface {
	RegisterServiceProductForm(
		ctx context.Context,
		command application.RegisterServiceProductFormCommand,
	) (application.ProductChannelResult, error)
	RegisterMapping(
		ctx context.Context,
		command application.RegisterProductChannelMappingCommand,
	) (application.ProductChannelResult, error)
}

// 编译期锁缝：本端点族不新造登记语义，只消费 application 里那一个登记用例，签名漂移
// 在编译期暴露。
var _ ProductChannelRegistrar = (*application.RegisterProductChannelHandler)(nil)

// NewRegisterServiceProductFormEndpoint 交回服务形态登记的 HTTP 入口（ADR-0085
// Decision 一）。它登的是「这份已发布且已生效的服务产品版本是哪一种服务形态」；版本
// 不在册或未生效由用例侧拒，本端点不替发布面造版本。
func NewRegisterServiceProductFormEndpoint(
	intake ServiceProductFormRegistrationIntake,
	registrar ProductChannelRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeServiceProductFormRegistration,
		registrar.RegisterServiceProductForm,
		writeProductChannelAnswer,
	)
}

// NewRegisterProductChannelMappingEndpoint 交回产品—渠道映射修订登记的 HTTP 入口。
// 映射标识钉着它的产品版本，改指产品是另一笔映射而不是本映射的新修订（用例侧的门）；
// 端点因此也没有「改指」这一动作可言，只有登记。
func NewRegisterProductChannelMappingEndpoint(
	intake ProductChannelMappingRegistrationIntake,
	registrar ProductChannelRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeProductChannelMappingRegistration,
		registrar.RegisterMapping,
		writeProductChannelAnswer,
	)
}

// writeProductChannelAnswer 把服务形态与映射登记的答案逐名转写，判据与身份族那一份
// 逐字相同（`已登记`取 201、治理答案取 200、散文原因随 `cause` 过线、具名格原名过线）。
// 两族各写一份而不共用一个：`已停用`与`未找到`只在身份族有，共用就得在这里判一个本族
// 永远取不到的值，而那格将来在身份族收紧时会被一并改掉。
func writeProductChannelAnswer(
	response http.ResponseWriter,
	result application.ProductChannelResult,
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
	if outcome == application.ProductChannelRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
