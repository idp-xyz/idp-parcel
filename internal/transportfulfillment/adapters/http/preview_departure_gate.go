package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 设备出发前门禁问询口（ADR-0219）。设备在动作之前问「此刻按这班、这段出发，关务怎么答」，交回的读数与随后同参登记的那条
// 出发同一套（ADR-0205 决定二），线格式也同一个 departureGateReadingView：设备拿两份答复逐格比，比的是同一种东西。
//
// 挂作业事实渠道、与移动事实口同一份认证与授予：问的人就是随后报出发的那台设备，能报出发就能问出发前那一问，不另立能力
// 种类（handover-dispatch-plan 代决 C17）。它只读不写：本上下文不落任何东西，关务那一侧照 ADR-0206 决定六记判断。
//
// 方法仍是 POST：意图从载荷进、按封闭形状解，与本渠道其余各口同一道门；路径照本仓「-previews」那一族（预览不写库）。

// DepartureGatePreviewIntake 把已认证的问询翻成问询命令：租户从信封给，班次、段与拟出发时点从载荷收。
type DepartureGatePreviewIntake interface {
	IntakeDepartureGatePreview(ctx context.Context, request *http.Request) (application.PreviewDepartureGateCommand, error)
}

// DepartureGatePreviewPayload 是问询的线格式，三格与 MovementFactPayload 里同名两格（`schedule`、`segment`）逐字同读法；
// `departsAt` 对出发的 `occurredAt`，RFC 3339。只收这三格：读数不取决于事实身份、地点、资源与放行凭据，收进来就是在暗示
// 它们算数；载荷按封闭形状解，带别的键是坏报文。
type DepartureGatePreviewPayload struct {
	Schedule  string `json:"schedule"`
	Segment   string `json:"segment,omitempty"`
	DepartsAt string `json:"departsAt"`
}

// Command 把载荷连同信封给的租户翻成问询命令。时刻给了却解不出是坏报文（400）；空着交回零值，由编排答未受理——不拿服务端
// 时钟补，理由同 parseOptionalInstant。
func (payload DepartureGatePreviewPayload) Command(tenant domain.TenantID) (application.PreviewDepartureGateCommand, error) {
	if tenant.String() == "" {
		return application.PreviewDepartureGateCommand{}, ErrOperatorIdentityMissing
	}
	departsAt, err := parseOptionalInstant("departsAt", payload.DepartsAt)
	if err != nil {
		return application.PreviewDepartureGateCommand{}, err
	}
	return application.PreviewDepartureGateCommand{
		TenantID:  tenant,
		Schedule:  payload.Schedule,
		Segment:   payload.Segment,
		DepartsAt: departsAt,
	}, nil
}

// DepartureGatePreviewer 是本适配器转交的问询编排。
type DepartureGatePreviewer interface {
	Preview(ctx context.Context, command application.PreviewDepartureGateCommand) (application.PreviewDepartureGateResult, error)
}

// UnconfiguredIntake 与 WorkFactCommandIntake 对本口的答法声明在本文件，同 trigger_delivery_dispatch.go 的做法：这一口的
// Intake 形、端点与两种答法同文件可读。语义与各自文件里其余各口一字不差。
var _ DepartureGatePreviewIntake = UnconfiguredIntake{}

func (UnconfiguredIntake) IntakeDepartureGatePreview(context.Context, *http.Request) (application.PreviewDepartureGateCommand, error) {
	return application.PreviewDepartureGateCommand{}, ErrAccessChannelNotConfigured
}

var _ DepartureGatePreviewIntake = (*WorkFactCommandIntake)(nil)

// IntakeDepartureGatePreview 走 tenantFrom 那一道：作业范围只认 OFFSITE、设备要在用、操作者要在这个作业范围上持作业事实
// 登记授予——与 IntakeMovementFact 同一道门，问得了的就是报得了出发的。
func (intake *WorkFactCommandIntake) IntakeDepartureGatePreview(ctx context.Context, request *http.Request) (application.PreviewDepartureGateCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.PreviewDepartureGateCommand{}, err
	}
	var payload DepartureGatePreviewPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.PreviewDepartureGateCommand{}, err
	}
	return payload.Command(tenant)
}

// NewPreviewDepartureGateEndpoint 交回出发前门禁问询的 HTTP 入口。
func NewPreviewDepartureGateEndpoint(intake DepartureGatePreviewIntake, previewer DepartureGatePreviewer) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.PreviewDepartureGateResult, error, bool) {
		command, err := intake.IntakeDepartureGatePreview(request.Context(), request)
		if err != nil {
			return application.PreviewDepartureGateResult{}, err, false
		}
		result, err := previewer.Preview(request.Context(), command)
		return result, err, true
	}, writeDepartureGatePreviewOutcome)
}

// departureGatePreviewResponse 是问询口的封闭响应形状：`outcome` 取应用枚举原名，`gateReading` 只在问到了时有，形状与移动
// 事实口的 `gateReading` 是同一个类型——关务判断标识与依据原文同样不回给设备（ADR-0205 决定一）。不回显请求的三格：设备
// 手里就有，回显只多一处要与请求逐字对齐的东西。
type departureGatePreviewResponse struct {
	Outcome     string                    `json:"outcome"`
	GateReading *departureGateReadingView `json:"gateReading,omitempty"`
}

// writeDepartureGatePreviewOutcome 一律 200：问询不新落任何东西，没有 201 可答（ADR-0022）；读数阻断也是 200——挡不挡出发
// 是设备与现场的事，本口只把关务此刻怎么答交回去。
func writeDepartureGatePreviewOutcome(response http.ResponseWriter, result application.PreviewDepartureGateResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := departureGatePreviewResponse{Outcome: outcome}
	if reading, read := result.Reading(); read {
		body.GateReading = departureGateReadingViewOf(reading)
	}
	writeJSON(response, http.StatusOK, body)
}
