package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// PickupRegistrationIntake 把已认证的接入请求翻译成单对象揽收登记命令。接口而非解析代码的理由
// 同 DeliveryIntake；`Segment`/`PlannedSegment` 必须收的理由同 HandoverIntake——收寄与交接是同一道
// 进段门的两个来源侧。
type PickupRegistrationIntake interface {
	IntakePickupRegistration(ctx context.Context, request *http.Request) (application.RegisterOffsitePickupCommand, error)
}

// OffsitePickupRegistrationPayload 是单对象揽收登记的线格式，逐格镜像 application.RegisterOffsitePickupCommand 去掉租户。
// occurredAt 取 RFC 3339；segment / plannedSegment / segmentServiceAction 可缺，缺席即不立段（命令注释）。
type OffsitePickupRegistrationPayload struct {
	Object               string `json:"object"`
	Task                 string `json:"task"`
	Attempt              string `json:"attempt"`
	Place                string `json:"place"`
	Control              string `json:"control"`
	ExecutedBy           string `json:"executedBy"`
	OccurredAt           string `json:"occurredAt"`
	Segment              string `json:"segment,omitempty"`
	PlannedSegment       string `json:"plannedSegment,omitempty"`
	SegmentServiceAction string `json:"segmentServiceAction,omitempty"`
}

// Command 把载荷连同信封给的租户翻成登记命令。只有时刻在这里解（解不出 400）；对象、尝试与控制证据成不成形留给编排
// 答`未受理`——那是形成了的业务答案，不是坏报文。
func (payload OffsitePickupRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterOffsitePickupCommand, error) {
	if tenant.String() == "" {
		return application.RegisterOffsitePickupCommand{}, ErrOperatorIdentityMissing
	}
	occurredAt, err := parseOptionalInstant("occurredAt", payload.OccurredAt)
	if err != nil {
		return application.RegisterOffsitePickupCommand{}, err
	}
	return application.RegisterOffsitePickupCommand{
		TenantID:             tenant,
		Object:               payload.Object,
		Task:                 payload.Task,
		Attempt:              payload.Attempt,
		Place:                payload.Place,
		Control:              payload.Control,
		ExecutedBy:           payload.ExecutedBy,
		OccurredAt:           occurredAt,
		Segment:              payload.Segment,
		PlannedSegment:       payload.PlannedSegment,
		SegmentServiceAction: payload.SegmentServiceAction,
	}, nil
}

// PickupRegistrationHandler 是本适配器转交的应用编排。
//
// 只有 Register：更正走 PickupCorrectionHandler（票 tf-segment-lifecycle-closure/08 裁了版本链之后另立的
// 接口），不往这里加方法——加了会打断它的每个实现者。
type PickupRegistrationHandler interface {
	Register(
		ctx context.Context,
		command application.RegisterOffsitePickupCommand,
	) (application.RegisterOffsitePickupResult, error)
}

// NewRegisterOffsitePickupEndpoint 交回单对象揽收登记的 HTTP 入口。
func NewRegisterOffsitePickupEndpoint(intake PickupRegistrationIntake, handler PickupRegistrationHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterOffsitePickupResult, error, bool) {
		command, err := intake.IntakePickupRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterOffsitePickupResult{}, err, false
		}
		result, err := handler.Register(request.Context(), command)
		return result, err, true
	}, writePickupRegistrationOutcome)
}

// pickupRegistrationResponse 是揽收登记与揽收更正两口共用的封闭响应形状。三个引用格与
// `segmentEntryRefusal` 各自透出，理由同 handoverResponse；`corrects` 是更正版本回指的前版，首登为空。
type pickupRegistrationResponse struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason,omitempty"`
	PickupVersion                string `json:"pickupVersion,omitempty"`
	Object                       string `json:"object,omitempty"`
	Task                         string `json:"task,omitempty"`
	Attempt                      string `json:"attempt,omitempty"`
	Corrects                     string `json:"corrects,omitempty"`
	ContinuationReference        string `json:"continuationReference,omitempty"`
	HandoffReference             string `json:"handoffReference,omitempty"`
	SegmentContinuationReference string `json:"segmentContinuationReference,omitempty"`
	SegmentEntryRefusal          string `json:"segmentEntryRefusal,omitempty"`
}

func writePickupRegistrationOutcome(response http.ResponseWriter, result application.RegisterOffsitePickupResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := pickupRegistrationResponse{
		Outcome:                      outcome,
		UndecidedReason:              result.UndecidedReason().String(),
		ContinuationReference:        result.ContinuationReference(),
		HandoffReference:             result.PickupHandoffReference(),
		SegmentContinuationReference: result.SegmentContinuationReference(),
		SegmentEntryRefusal:          result.SegmentEntryRefusal().String(),
	}
	if record, present := result.Record(); present {
		body.PickupVersion = record.Pickup.Version().String()
		body.Object = record.Pickup.Object().String()
		body.Task = record.Pickup.Task().String()
		body.Attempt = record.Pickup.Attempt().String()
		if predecessor, corrected := record.Pickup.Corrects(); corrected {
			body.Corrects = predecessor.String()
		}
	}

	// ADR-0022：状态码只报有没有新落一版。首登与更正都持久化了新版本用 201；重放、冲突、未受理、
	// 未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.PickupRegistered ||
		result.Outcome() == application.PickupCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
