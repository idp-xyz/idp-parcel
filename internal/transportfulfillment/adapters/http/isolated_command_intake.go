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
// 逐口放行（ADR-0091 Consequences），又随真渠道逐口退场（ADR-0150 决定三）：某口换上真渠道的同一笔，撤下本类型上对应的方法。
// 运营决定四口（关段、建派送任务、有效时间判断、承运商首次有效收寄判断）已随票 operator-channel/15 撤下。**装配点此刻已不挂
// 本类型**：下面余下的作业事实各口早随票 operator-channel/10 换了作业事实渠道，方法当时没有同笔撤下；它们的译装用例是本包
// 这几口线格式的主要覆盖，要撤得先把用例迁到真渠道 Intake 上，另立票做。
type IsolatedCommandIntake struct {
	tenant domain.TenantID
	// movementSource 是移动事实口的来源格。MovementFactIntake 的契约把「自营还是外部」交给认证结果说，隔离形态的认证结果
	// 就是装配点给定的这一个合成来源；载荷里带 source 即拒。
	movementSource string
}

// 仍在类型上的口。每撤一口在这里删一行断言、删一个方法。
var (
	_ PickupRegistrationIntake      = (*IsolatedCommandIntake)(nil)
	_ PickupAttemptIntake           = (*IsolatedCommandIntake)(nil)
	_ HandoverRegistrationIntake    = (*IsolatedCommandIntake)(nil)
	_ MovementFactIntake            = (*IsolatedCommandIntake)(nil)
	_ DeliveryDispatchTriggerIntake = (*IsolatedCommandIntake)(nil)
	_ DeliveryRegistrationIntake    = (*IsolatedCommandIntake)(nil)
	_ DeliveryAttemptIntake         = (*IsolatedCommandIntake)(nil)
)

// IsolatedCommandIntakeDeps 是构造本 Intake 的全部输入，全部是装配点给定的合成值。
type IsolatedCommandIntakeDeps struct {
	Tenant         string
	MovementSource string
}

// NewIsolatedCommandIntake 由装配点以显式合成值构造。立不起来的值在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedCommandIntake(deps IsolatedCommandIntakeDeps) (*IsolatedCommandIntake, error) {
	tenant, err := domain.NewTenantID(deps.Tenant)
	if err != nil {
		return nil, fmt.Errorf("transport fulfillment http: isolated command intake: %w", err)
	}
	if _, err := domain.NewMovementSourceReference(deps.MovementSource); err != nil {
		return nil, fmt.Errorf("transport fulfillment http: isolated command intake: %w", err)
	}
	return &IsolatedCommandIntake{tenant: tenant, movementSource: deps.MovementSource}, nil
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

// IntakeMovementFact 译自营执行方的一条实际移动事实（`/transport-fulfillment/movement-facts`）。来源格取注入值。
func (intake *IsolatedCommandIntake) IntakeMovementFact(
	_ context.Context,
	request *http.Request,
) (application.RecordMovementFactCommand, error) {
	var payload MovementFactPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RecordMovementFactCommand{}, err
	}
	return payload.Command(intake.tenant, intake.movementSource)
}

// IntakeDeliveryDispatchTrigger 译末端派送任务内部触发的一拍（`/transport-fulfillment-delivery-dispatch-triggers`）。谁按拍调、
// 拍频多大属调用方（实例半边）；隔离形态只让这一拍能被调用，不替它定拍频。
func (intake *IsolatedCommandIntake) IntakeDeliveryDispatchTrigger(
	_ context.Context,
	request *http.Request,
) (application.TriggerDeliveryDispatchCommand, error) {
	var payload DeliveryDispatchTriggerPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.TriggerDeliveryDispatchCommand{}, err
	}
	return payload.Command(intake.tenant)
}

// IntakeRegistration 译交付生效首登（`/transport-fulfillment/deliveries`）。方法名是 DeliveryRegistrationIntake 的通名（交付那份
// 先落、占了它）；POD 更正口不在本票，本类型不实现 IntakeCorrection。
func (intake *IsolatedCommandIntake) IntakeRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterEffectiveDeliveryCommand, error) {
	var payload EffectiveDeliveryPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterEffectiveDeliveryCommand{}, err
	}
	return payload.Command(intake.tenant)
}

// IntakeDeliveryAttempt 译一次派送到场的登记（`/transport-fulfillment/delivery-attempts`，票 product-strategy-boundary/19）。
func (intake *IsolatedCommandIntake) IntakeDeliveryAttempt(
	_ context.Context,
	request *http.Request,
) (application.RecordDeliveryAttemptCommand, error) {
	var payload DeliveryAttemptPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RecordDeliveryAttemptCommand{}, err
	}
	return payload.Command(intake.tenant)
}
