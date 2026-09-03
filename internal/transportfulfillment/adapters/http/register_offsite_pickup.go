package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// PickupRegistrationIntake 把已认证的接入请求翻译成单对象揽收登记命令。接口而非解析代码的理由
// 同 DeliveryIntake；`Segment`/`PlannedSegment` 必须收的理由同 HandoverIntake——收寄与交接是同一道
// 进段门的两个来源侧。
type PickupRegistrationIntake interface {
	IntakePickupRegistration(ctx context.Context, request *http.Request) (application.RegisterOffsitePickupCommand, error)
}

// PickupRegistrationHandler 是本适配器转交的应用编排。
//
// 只有 Register 没有 Correct：揽收登记的应用层今天没有更正编排（票 04「边界与未做」）。端点不
// 替它造一个——更正是不是一格、走版本链还是走新尝试，是 TF application 的领域形状，不在传输层定。
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

// pickupRegistrationResponse 是揽收登记口的封闭响应形状。三个引用格各自透出，理由同 handoverResponse。
type pickupRegistrationResponse struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason,omitempty"`
	PickupVersion                string `json:"pickupVersion,omitempty"`
	Object                       string `json:"object,omitempty"`
	Task                         string `json:"task,omitempty"`
	Attempt                      string `json:"attempt,omitempty"`
	ContinuationReference        string `json:"continuationReference,omitempty"`
	HandoffReference             string `json:"handoffReference,omitempty"`
	SegmentContinuationReference string `json:"segmentContinuationReference,omitempty"`
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
	}
	if record, present := result.Record(); present {
		body.PickupVersion = record.Pickup.Version().String()
		body.Object = record.Pickup.Object().String()
		body.Task = record.Pickup.Task().String()
		body.Attempt = record.Pickup.Attempt().String()
	}

	// ADR-0022：只有首登新落一版用 201；重放、冲突、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.PickupRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
