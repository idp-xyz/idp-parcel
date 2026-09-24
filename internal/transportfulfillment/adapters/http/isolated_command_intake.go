package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// IsolatedCommandIntake 是隔离写路径准入（ADR-0091）在运输履约主链命令面的注入式放行（票 operator-channel/08）。
//
// 它与本包的 IsolatedOperationsReadIntake 同层同款：租户格在构造时由装配点给定，进请求路径后不读任何授权输入——本包各口
// 的 Intake 契约都写着「租户身份只能来自认证结果（ADR-0003）」，隔离形态的认证结果就是开关值。与读面不同的是它会构造
// 命令、命令会落行：可分辨物由租户维的 `SYN-` 前缀承担（ADR-0091 决定三）；前缀门禁、启用与否与启动日志都在
// cmd/parcel-api，本类型只收立得住的值。
//
// 事实内容与发生时间照 ADR-0023 从载荷如实收，本类型不拿服务端时钟或随机数顶替任何一样。每口的线格式是该口文件里的
// `…Payload`，翻译走它的 Command——载荷只有内容没有身份，本类型只负责把注入的那几格交进去。
//
// 逐口放行（ADR-0091 Consequences）：本类型只实现已成笔的口的 Intake 接口，未成笔的口在装配点仍挂字面量
// UnconfiguredIntake{}，且在类型上就装不进本类型——每放一口在这里多一个方法，装配点多换一行，两处都看得见。
type IsolatedCommandIntake struct {
	tenant domain.TenantID
}

// 已成笔的口。每放一口在这里多一行断言、多一个方法，装配点多换一行。
var (
	_ PickupRegistrationIntake    = (*IsolatedCommandIntake)(nil)
	_ PickupAttemptIntake         = (*IsolatedCommandIntake)(nil)
	_ CarrierPickupJudgmentIntake = (*IsolatedCommandIntake)(nil)
	_ HandoverRegistrationIntake  = (*IsolatedCommandIntake)(nil)
)

// IsolatedCommandIntakeDeps 是构造本 Intake 的全部输入，全部是装配点给定的合成值。
type IsolatedCommandIntakeDeps struct {
	Tenant string
}

// NewIsolatedCommandIntake 由装配点以显式合成值构造。立不起来的值在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedCommandIntake(deps IsolatedCommandIntakeDeps) (*IsolatedCommandIntake, error) {
	tenant, err := domain.NewTenantID(deps.Tenant)
	if err != nil {
		return nil, fmt.Errorf("transport fulfillment http: isolated command intake: %w", err)
	}
	return &IsolatedCommandIntake{tenant: tenant}, nil
}

// IntakePickupRegistration 译单对象场外揽收登记（`/transport-fulfillment/offsite-pickups`）。
func (intake *IsolatedCommandIntake) IntakePickupRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterOffsitePickupCommand, error) {
	var payload OffsitePickupRegistrationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterOffsitePickupCommand{}, err
	}
	return payload.Command(intake.tenant)
}

// IntakePickupAttempt 译一次到访多对象的揽收执行（`/transport-fulfillment/offsite-pickup-attempts`）。
func (intake *IsolatedCommandIntake) IntakePickupAttempt(
	_ context.Context,
	request *http.Request,
) (application.PerformOffsitePickupCommand, error) {
	var payload OffsitePickupAttemptPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.PerformOffsitePickupCommand{}, err
	}
	return payload.Command(intake.tenant)
}

// IntakeCarrierPickupJudgment 译实际承运商首次有效收寄的显式判断（`/transport-fulfillment-carrier-first-effective-pickup-judgments`）。
// 线格式是本包既有的 CarrierPickupJudgmentPayload；解码走封闭门而不是 DecodeCarrierPickupJudgmentPayload——后者不拒尾随内容，
// 而本类型各口对同一种畸形要答同一格。
func (intake *IsolatedCommandIntake) IntakeCarrierPickupJudgment(
	_ context.Context,
	request *http.Request,
) (application.JudgeCarrierFirstEffectivePickupCommand, error) {
	var payload CarrierPickupJudgmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, err
	}
	return payload.Command(intake.tenant)
}

// IntakeHandoverRegistration 译交接判断首登（`/transport-fulfillment/handovers`）。更正口不在本票，本类型不实现它。
func (intake *IsolatedCommandIntake) IntakeHandoverRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterTransportHandoverCommand, error) {
	var payload HandoverRegistrationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterTransportHandoverCommand{}, err
	}
	return payload.Command(intake.tenant)
}
