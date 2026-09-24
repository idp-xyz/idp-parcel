package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DeliveryAttemptIntake 把已认证的接入请求翻译成一次派送到场的登记命令（票 product-strategy-boundary/19）。
//
// 租户身份只能来自认证结果（ADR-0003）；任务、尝试、执行方、地点、时间与逐对象结果都是执行方记录的事实，必须从
// 请求体收（ADR-0023）。它是作业事实，生产渠道归操作者渠道的「作业事实登记」能力面（ADR-0149）。
type DeliveryAttemptIntake interface {
	IntakeDeliveryAttempt(ctx context.Context, request *http.Request) (application.RecordDeliveryAttemptCommand, error)
}

// DeliveryAttemptPayload 是一次派送到场的线格式，逐格镜像 application.RecordDeliveryAttemptCommand 去掉租户；
// 时刻一律取 RFC 3339。
type DeliveryAttemptPayload struct {
	Task            string                         `json:"task"`
	Attempt         string                         `json:"attempt"`
	ExecutedBy      string                         `json:"executedBy"`
	Place           string                         `json:"place"`
	PlannedFrom     string                         `json:"plannedFrom"`
	PlannedTo       string                         `json:"plannedTo"`
	ArrivedAt       string                         `json:"arrivedAt"`
	Evidence        string                         `json:"evidence"`
	RescheduledFrom string                         `json:"rescheduledFrom,omitempty"`
	Objects         []DeliveryAttemptObjectPayload `json:"objects"`
}

// DeliveryAttemptObjectPayload 是逐对象的一项。outcome 取 domain.DeliveryObjectOutcome 的封闭词；basis 可缺——
// 妥投不带，失败与拒收必带，缺了由编排答`未受理`。
type DeliveryAttemptObjectPayload struct {
	Object     string `json:"object"`
	Outcome    string `json:"outcome"`
	Basis      string `json:"basis,omitempty"`
	OccurredAt string `json:"occurredAt"`
}

// Command 把载荷连同信封给的租户翻成登记命令。时刻解不出、成败词不在封闭集内是坏报文（400）；引用缺席时照零值
// 交进去，由编排答`未受理`。
func (payload DeliveryAttemptPayload) Command(tenant domain.TenantID) (application.RecordDeliveryAttemptCommand, error) {
	none := application.RecordDeliveryAttemptCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.RecordDeliveryAttemptCommand{
		TenantID:        tenant,
		Task:            payload.Task,
		Attempt:         payload.Attempt,
		ExecutedBy:      payload.ExecutedBy,
		Place:           payload.Place,
		Evidence:        payload.Evidence,
		RescheduledFrom: payload.RescheduledFrom,
	}
	var err error
	if command.PlannedFrom, err = parseOptionalInstant("plannedFrom", payload.PlannedFrom); err != nil {
		return none, err
	}
	if command.PlannedTo, err = parseOptionalInstant("plannedTo", payload.PlannedTo); err != nil {
		return none, err
	}
	if command.ArrivedAt, err = parseOptionalInstant("arrivedAt", payload.ArrivedAt); err != nil {
		return none, err
	}
	for index, object := range payload.Objects {
		submission, err := object.submission()
		if err != nil {
			return none, fmt.Errorf("%w: objects[%d]: %v", ErrMalformedRequest, index, err)
		}
		command.Objects = append(command.Objects, submission)
	}
	return command, nil
}

func (object DeliveryAttemptObjectPayload) submission() (application.ObjectDeliverySubmission, error) {
	var submission application.ObjectDeliverySubmission
	var err error
	if object.Object != "" {
		if submission.Object, err = domain.NewCarriedObjectReference(object.Object); err != nil {
			return submission, err
		}
	}
	if object.Outcome != "" {
		if submission.Outcome, err = deliveryObjectOutcomeFromWord(object.Outcome); err != nil {
			return submission, err
		}
	}
	if object.Basis != "" {
		if submission.Basis, err = domain.NewAttemptResultBasisReference(object.Basis); err != nil {
			return submission, err
		}
	}
	submission.OccurredAt, err = parseOptionalInstant("occurredAt", object.OccurredAt)
	return submission, err
}

// deliveryObjectOutcomeFromWord 是 domain.DeliveryObjectOutcome 封闭集的名称镜像，词取各常量自己的 String()，不另立
// 一份词表。揽收侧的成败词（PICKED_UP 等）不在其中——两族结果各有各的封闭集。
func deliveryObjectOutcomeFromWord(raw string) (domain.DeliveryObjectOutcome, error) {
	for _, outcome := range []domain.DeliveryObjectOutcome{
		domain.ObjectDelivered, domain.DeliveryRefused, domain.NoOneToReceive, domain.WrongAddress,
	} {
		if outcome.String() == raw {
			return outcome, nil
		}
	}
	return domain.DeliveryObjectOutcomeInvalid, fmt.Errorf("outcome=%q is not a delivery object outcome word", raw)
}

// DeliveryAttemptHandler 是本适配器转交的应用编排。
type DeliveryAttemptHandler interface {
	Handle(
		ctx context.Context,
		command application.RecordDeliveryAttemptCommand,
	) (application.RecordDeliveryAttemptResult, error)
}

// NewRecordDeliveryAttemptEndpoint 交回派送尝试登记的 HTTP 入口。与交付生效首登是两口：交付生效只引用已登记的
// 尝试，合成一口就回到「先声称到过场再声称交付成功」那条路。
func NewRecordDeliveryAttemptEndpoint(intake DeliveryAttemptIntake, handler DeliveryAttemptHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RecordDeliveryAttemptResult, error, bool) {
		command, err := intake.IntakeDeliveryAttempt(request.Context(), request)
		if err != nil {
			return application.RecordDeliveryAttemptResult{}, err, false
		}
		result, err := handler.Handle(request.Context(), command)
		return result, err, true
	}, writeDeliveryAttemptOutcome)
}

// deliveryAttemptResponse 是派送尝试口的封闭响应形状。`objects` 逐对象透出成败与依据：任务汇总只能由对象结果派生
// （UC-TF-006），传输层不替它汇总成一个「本次到场成功/失败」。
type deliveryAttemptResponse struct {
	Outcome               string                        `json:"outcome"`
	UndecidedReason       string                        `json:"undecidedReason,omitempty"`
	Attempt               string                        `json:"attempt,omitempty"`
	Task                  string                        `json:"task,omitempty"`
	Objects               []deliveryAttemptObjectResult `json:"objects,omitempty"`
	ContinuationReference string                        `json:"continuationReference,omitempty"`
}

type deliveryAttemptObjectResult struct {
	Object  string `json:"object"`
	Outcome string `json:"outcome"`
	Basis   string `json:"basis,omitempty"`
}

func writeDeliveryAttemptOutcome(response http.ResponseWriter, result application.RecordDeliveryAttemptResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := deliveryAttemptResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		body.Attempt = record.Attempt.Attempt().String()
		body.Task = record.Attempt.Task().String()
		for _, objectResult := range record.Results {
			entry := deliveryAttemptObjectResult{
				Object:  objectResult.Object().String(),
				Outcome: objectResult.Outcome().String(),
			}
			if basis, present := objectResult.Basis(); present {
				entry.Basis = basis.String()
			}
			body.Objects = append(body.Objects, entry)
		}
	}

	// ADR-0022：只有这次新落了一次尝试用 201；重放、冲突、任务不对、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.DeliveryAttemptRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
