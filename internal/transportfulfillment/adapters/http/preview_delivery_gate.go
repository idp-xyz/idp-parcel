package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送出发前的交付门禁问询口（ADR-0238）。尾程设备在派送出发之前按件问「这件按这一刻交到收件方手里，关务怎么答」，交回的读数
// 与随后同参登记的那次有效交付同一套（ADR-0213 决定五），线格式也同一个 deliveryGateReadingView：设备拿两份答复逐格比，比的
// 是同一种东西。
//
// 挂作业事实渠道、与有效交付口同一份认证与授予：问的人就是随后登交付的那台设备，能登交付就能问派送出发前那一问，不另立能力种类
// （同 ADR-0219 决定四）。它只读不写：本上下文不落任何东西，关务那一侧照 ADR-0206 决定六记判断。方法与路径的取法同出发前问询口。

// DeliveryGatePreviewIntake 把已认证的问询翻成问询命令：租户从信封给，对象与拟交付时点从载荷收。
type DeliveryGatePreviewIntake interface {
	IntakeDeliveryGatePreview(ctx context.Context, request *http.Request) (application.PreviewDeliveryGateCommand, error)
}

// DeliveryGatePreviewPayload 是问询的线格式：`object` 与 EffectiveDeliveryPayload 的同名一格逐字同读法；`deliversAt` 对随后那次
// 派送尝试里这件对象结果的发生时刻，RFC 3339。只收这两格：读数不取决于尝试、方式、接收方与 POD，收进来就是在暗示它们算数；
// 载荷按封闭形状解，带别的键是坏报文。
type DeliveryGatePreviewPayload struct {
	Object     string `json:"object"`
	DeliversAt string `json:"deliversAt"`
}

// Command 把载荷连同信封给的租户翻成问询命令。时刻给了却解不出是坏报文（400）；空着交回零值，由编排答未受理——不拿服务端
// 时钟补，理由同 parseOptionalInstant。
func (payload DeliveryGatePreviewPayload) Command(tenant domain.TenantID) (application.PreviewDeliveryGateCommand, error) {
	if tenant.String() == "" {
		return application.PreviewDeliveryGateCommand{}, ErrOperatorIdentityMissing
	}
	deliversAt, err := parseOptionalInstant("deliversAt", payload.DeliversAt)
	if err != nil {
		return application.PreviewDeliveryGateCommand{}, err
	}
	return application.PreviewDeliveryGateCommand{TenantID: tenant, Object: payload.Object, DeliversAt: deliversAt}, nil
}

// DeliveryGatePreviewer 是本适配器转交的问询编排。
type DeliveryGatePreviewer interface {
	Preview(ctx context.Context, command application.PreviewDeliveryGateCommand) (application.PreviewDeliveryGateResult, error)
}

// UnconfiguredIntake 对本口的答法与其余各口同：不读请求、不构造命令，一律未配置。声明在本文件的理由同出发前问询口。
var _ DeliveryGatePreviewIntake = UnconfiguredIntake{}

func (UnconfiguredIntake) IntakeDeliveryGatePreview(context.Context, *http.Request) (application.PreviewDeliveryGateCommand, error) {
	return application.PreviewDeliveryGateCommand{}, ErrAccessChannelNotConfigured
}

// NewPreviewDeliveryGateEndpoint 交回派送出发前交付门禁问询的 HTTP 入口。
func NewPreviewDeliveryGateEndpoint(intake DeliveryGatePreviewIntake, previewer DeliveryGatePreviewer) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.PreviewDeliveryGateResult, error, bool) {
		command, err := intake.IntakeDeliveryGatePreview(request.Context(), request)
		if err != nil {
			return application.PreviewDeliveryGateResult{}, err, false
		}
		result, err := previewer.Preview(request.Context(), command)
		return result, err, true
	}, writeDeliveryGatePreviewOutcome)
}

// deliveryGatePreviewResponse 是问询口的封闭响应形状：`outcome` 取应用枚举原名，`gateReading` 只在问到了时有，形状与有效交付口的
// `gateReading` 是同一个类型——关务判断标识与依据原文同样不回给设备，逐对象那一格同样只列没得出放行的那一件（ADR-0237）。
// 不回显请求的两格，理由同出发前问询口。
type deliveryGatePreviewResponse struct {
	Outcome     string                   `json:"outcome"`
	GateReading *deliveryGateReadingView `json:"gateReading,omitempty"`
}

// writeDeliveryGatePreviewOutcome 一律 200：问询不新落任何东西，没有 201 可答（ADR-0022）；读数阻断也是 200——派不派这一件
// 是设备与现场的事，本口只把关务此刻怎么答交回去。
func writeDeliveryGatePreviewOutcome(response http.ResponseWriter, result application.PreviewDeliveryGateResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := deliveryGatePreviewResponse{Outcome: outcome}
	if reading, read := result.Reading(); read {
		body.GateReading = deliveryGateReadingViewOf(reading)
	}
	writeJSON(response, http.StatusOK, body)
}
