package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// PickupAttemptIntake 把已认证的接入请求翻译成一次到访多对象的揽收执行命令。
//
// 段引用在这里是**两层**：整次到访一个 `Segment`（同一次到访取得控制的对象进同一个共同控制
// 范围），逐对象各自的 `PlannedSegment`（CONTEXT 要求每个对象分别关联自己的计划履约段）。
// Intake 两层都要收——收了整次那一格而漏掉逐对象那一格，成员差异就在端点层被抹平了。
type PickupAttemptIntake interface {
	IntakePickupAttempt(ctx context.Context, request *http.Request) (application.PerformOffsitePickupCommand, error)
}

// OffsitePickupAttemptPayload 是一次到访多对象揽收执行的线格式，逐格镜像 application.PerformOffsitePickupCommand 去掉租户；
// 三个时刻取 RFC 3339。段引用两层照命令分设：整次一个 segment，逐对象各自一个 plannedSegment。
type OffsitePickupAttemptPayload struct {
	SourceID        string                              `json:"sourceId"`
	Task            string                              `json:"task"`
	Attempt         string                              `json:"attempt"`
	ExecutedBy      string                              `json:"executedBy"`
	Place           string                              `json:"place"`
	PlannedFrom     string                              `json:"plannedFrom"`
	PlannedTo       string                              `json:"plannedTo"`
	ArrivedAt       string                              `json:"arrivedAt"`
	Evidence        string                              `json:"evidence"`
	RescheduledFrom string                              `json:"rescheduledFrom,omitempty"`
	Segment         string                              `json:"segment,omitempty"`
	Objects         []OffsitePickupAttemptObjectPayload `json:"objects"`
}

// OffsitePickupAttemptObjectPayload 是逐对象的一项。outcome 取 domain.AttemptObjectOutcome 的封闭词；basis 与 control 可缺
// ——失败对象不带控制依据，带了由编排拒。
type OffsitePickupAttemptObjectPayload struct {
	Object         string `json:"object"`
	Outcome        string `json:"outcome"`
	Basis          string `json:"basis,omitempty"`
	Control        string `json:"control,omitempty"`
	OccurredAt     string `json:"occurredAt"`
	PlannedSegment string `json:"plannedSegment,omitempty"`
}

// Command 把载荷连同信封给的租户翻成执行命令。时刻解不出、成败词不在封闭集内是坏报文（400）；引用缺席时照零值交进去，
// 由编排答`未受理`——命令上这几格是领域值对象，Intake 只在给了值时过它们的构造门。
func (payload OffsitePickupAttemptPayload) Command(tenant domain.TenantID) (application.PerformOffsitePickupCommand, error) {
	none := application.PerformOffsitePickupCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.PerformOffsitePickupCommand{
		TenantID:        tenant,
		SourceID:        payload.SourceID,
		Task:            payload.Task,
		Attempt:         payload.Attempt,
		ExecutedBy:      payload.ExecutedBy,
		Place:           payload.Place,
		Evidence:        payload.Evidence,
		RescheduledFrom: payload.RescheduledFrom,
		Segment:         payload.Segment,
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

func (object OffsitePickupAttemptObjectPayload) submission() (application.ObjectPickupSubmission, error) {
	submission := application.ObjectPickupSubmission{PlannedSegment: object.PlannedSegment}
	var err error
	if object.Object != "" {
		if submission.Object, err = domain.NewCarriedObjectReference(object.Object); err != nil {
			return submission, err
		}
	}
	if object.Outcome != "" {
		if submission.Outcome, err = attemptObjectOutcomeFromWord(object.Outcome); err != nil {
			return submission, err
		}
	}
	if object.Basis != "" {
		if submission.Basis, err = domain.NewAttemptResultBasisReference(object.Basis); err != nil {
			return submission, err
		}
	}
	if object.Control != "" {
		if submission.Control, err = domain.NewTransportControlReference(object.Control); err != nil {
			return submission, err
		}
	}
	submission.OccurredAt, err = parseOptionalInstant("occurredAt", object.OccurredAt)
	return submission, err
}

// attemptObjectOutcomeFromWord 是 domain.AttemptObjectOutcome 封闭集的名称镜像，词取各常量自己的 String()，不另立一份词表。
func attemptObjectOutcomeFromWord(raw string) (domain.AttemptObjectOutcome, error) {
	for _, outcome := range []domain.AttemptObjectOutcome{
		domain.ObjectPickedUp, domain.CustomerAbsent, domain.GoodsNotReady, domain.PackagingUnacceptable,
	} {
		if outcome.String() == raw {
			return outcome, nil
		}
	}
	return domain.AttemptObjectOutcomeInvalid, fmt.Errorf("outcome=%q is not an attempt object outcome word", raw)
}

// PickupAttemptHandler 是本适配器转交的应用编排。
type PickupAttemptHandler interface {
	Handle(
		ctx context.Context,
		command application.PerformOffsitePickupCommand,
	) (application.PerformOffsitePickupResult, error)
}

// NewPerformOffsitePickupEndpoint 交回一次到访多对象揽收执行的 HTTP 入口。与单对象登记口同组
// 不同口：命令类型互不相同，合成一口就得在 Intake 里先认形状再分路。
func NewPerformOffsitePickupEndpoint(intake PickupAttemptIntake, handler PickupAttemptHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.PerformOffsitePickupResult, error, bool) {
		command, err := intake.IntakePickupAttempt(request.Context(), request)
		if err != nil {
			return application.PerformOffsitePickupResult{}, err, false
		}
		result, err := handler.Handle(request.Context(), command)
		return result, err, true
	}, writePickupAttemptOutcome)
}

// pickupAttemptResponse 是多对象口的封闭响应形状。
//
// `objects` 逐对象透出成败与版本：UC-TF-002 要求任务汇总只能由对象结果派生，传输层不替它
// 汇总成一个「本次到访成功/失败」。`segmentEntries` 逐对象列进段那一半的欠账，只列真有欠账
// 的对象——进去了的、没要求进的、领域正当拒绝的都不在列，与编排交回的形状一致。
// `segmentEntryRefusal` 整次一格（段是整次到访共用的一个，它关了就对每个成功对象都关了）。
type pickupAttemptResponse struct {
	Outcome               string                      `json:"outcome"`
	UndecidedReason       string                      `json:"undecidedReason,omitempty"`
	Attempt               string                      `json:"attempt,omitempty"`
	Task                  string                      `json:"task,omitempty"`
	Objects               []pickupAttemptObjectResult `json:"objects,omitempty"`
	ContinuationReference string                      `json:"continuationReference,omitempty"`
	HandoffReference      string                      `json:"handoffReference,omitempty"`
	SegmentEntries        []segmentEntryResponse      `json:"segmentEntries,omitempty"`
	SegmentEntryRefusal   string                      `json:"segmentEntryRefusal,omitempty"`
}

type pickupAttemptObjectResult struct {
	Object        string `json:"object"`
	Outcome       string `json:"outcome"`
	Basis         string `json:"basis,omitempty"`
	PickupVersion string `json:"pickupVersion,omitempty"`
}

type segmentEntryResponse struct {
	Object                string `json:"object"`
	ContinuationReference string `json:"continuationReference"`
}

func writePickupAttemptOutcome(response http.ResponseWriter, result application.PerformOffsitePickupResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := pickupAttemptResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		HandoffReference:      result.PickupHandoffReference(),
		SegmentEntryRefusal:   result.SegmentEntryRefusal().String(),
	}
	if record, present := result.Record(); present {
		body.Attempt = record.Attempt.Attempt().String()
		body.Task = record.Attempt.Task().String()
		versions := map[string]string{}
		for _, pickup := range record.Pickups {
			versions[pickup.Object().String()] = pickup.Version().String()
		}
		for _, objectResult := range record.Results {
			entry := pickupAttemptObjectResult{
				Object:        objectResult.Object().String(),
				Outcome:       objectResult.Outcome().String(),
				PickupVersion: versions[objectResult.Object().String()],
			}
			if basis, present := objectResult.Basis(); present {
				entry.Basis = basis.String()
			}
			body.Objects = append(body.Objects, entry)
		}
	}
	for _, entry := range result.SegmentEntries() {
		body.SegmentEntries = append(body.SegmentEntries, segmentEntryResponse{
			Object:                entry.Object.String(),
			ContinuationReference: entry.ContinuationReference,
		})
	}

	// ADR-0022：只有这次到访新落了结果用 201；重放、冲突、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.PickupAttemptRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
