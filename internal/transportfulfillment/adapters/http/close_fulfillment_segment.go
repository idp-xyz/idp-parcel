package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 票 tf-segment-lifecycle-closure/07 的四个 admin 写面里的第一个：关段声明。
//
// 它是运营决定不是承运方回传口（票 03 简报第 6 行）：CONTEXT 生命周期④「全部有效参与关系已经结束
// **且不再接受新对象**」的后半句是一个决定，任何状态都推导不出来，所以只能由人经这个口给。写面沿
// ADR-0085：Intake 接口 + 处理器接口 + 封闭响应形状，装配以字面量 UnconfiguredIntake{} 起步。
//
// 路径前缀取读面册名 `transport-fulfillment-`（与 /transport-fulfillment-records 同源），不取控制事实
// 那组的 `/transport-fulfillment/...`：一组是接入方回传事实，一组是运营写决定，两种口不混一个前缀。

// SegmentClosureIntake 把已认证的运营写请求翻译成关段声明。
type SegmentClosureIntake interface {
	IntakeSegmentClosure(ctx context.Context, request *http.Request) (application.CloseFulfillmentSegmentCommand, error)
}

// SegmentClosurePayload 是关段声明的线格式：段与关段时刻（RFC 3339）两格，逐格镜像 application.CloseFulfillmentSegmentCommand
// 去掉租户。
type SegmentClosurePayload struct {
	Segment  string `json:"segment"`
	ClosedAt string `json:"closedAt"`
}

// Command 把载荷连同信封给的租户翻成关段命令。时刻解不出是坏报文（400）；段在不在册、还有没有在场参与由编排答。
func (payload SegmentClosurePayload) Command(tenant domain.TenantID) (application.CloseFulfillmentSegmentCommand, error) {
	if tenant.String() == "" {
		return application.CloseFulfillmentSegmentCommand{}, ErrOperatorIdentityMissing
	}
	closedAt, err := parseOptionalInstant("closedAt", payload.ClosedAt)
	if err != nil {
		return application.CloseFulfillmentSegmentCommand{}, err
	}
	return application.CloseFulfillmentSegmentCommand{TenantID: tenant, Segment: payload.Segment, ClosedAt: closedAt}, nil
}

// SegmentCloser 是本适配器转交的应用编排。
type SegmentCloser interface {
	Close(
		ctx context.Context,
		command application.CloseFulfillmentSegmentCommand,
	) (application.CloseFulfillmentSegmentResult, error)
}

// segmentClosureAnswer 把编排结果与命令里要回显的段引用捆在一起交给映射：关段的结果类型自己不带段
// （它只答 outcome 与续办引用），而调用方要在响应里认出这是哪个段的答案。
type segmentClosureAnswer struct {
	result  application.CloseFulfillmentSegmentResult
	segment string
}

// NewCloseFulfillmentSegmentEndpoint 交回关段声明的 HTTP 入口。
func NewCloseFulfillmentSegmentEndpoint(intake SegmentClosureIntake, closer SegmentCloser) http.Handler {
	return commandEndpoint(func(request *http.Request) (segmentClosureAnswer, error, bool) {
		command, err := intake.IntakeSegmentClosure(request.Context(), request)
		if err != nil {
			return segmentClosureAnswer{}, err, false
		}
		result, err := closer.Close(request.Context(), command)
		return segmentClosureAnswer{result: result, segment: command.Segment}, err, true
	}, writeSegmentClosureOutcome)
}

// segmentClosureResponse 是关段口的封闭响应形状。`SEGMENT_STILL_ACTIVE` 与 `INPUT_NOT_ACCEPTED`、
// `SEGMENT_ALREADY_CLOSED` 与 `SEGMENT_CLOSED` 各自成格：续办动作两两不同（去结束剩下的参与 / 改输入 /
// 什么都不做 / 无事），传输层不合并。
type segmentClosureResponse struct {
	Outcome               string `json:"outcome"`
	Segment               string `json:"segment,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

func writeSegmentClosureOutcome(response http.ResponseWriter, answer segmentClosureAnswer) {
	outcome := answer.result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := segmentClosureResponse{
		Outcome:               outcome,
		ContinuationReference: answer.result.ContinuationReference(),
	}
	if answer.result.Outcome() != application.SegmentCloseNotAccepted {
		body.Segment = answer.segment
	}
	// ADR-0022：只有这次声明真把段关上了用 201；早已关闭、仍有在场、不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if answer.result.Outcome() == application.SegmentClosedNow {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
