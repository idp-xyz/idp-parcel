package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果的两口（票 docs-consistency/code/90）：揽收一口、派送一口。
// 每口挂两个接入面——执行方在作业事实面后补（更正随原事实的族，ADR-0151 决定四）、复核人在运营决定面认定（ADR-0297）——
// 两面交进来的是同一条命令、同一个端点实现，面的区别只在 Intake：作业事实面不带操作者，运营决定面把提交操作者作为操作证据带进去。

// PickupAttemptResultCorrectionIntake 把已认证的接入请求翻译成揽收结果更正命令。另立接口而不给既有 Intake 加方法，理由同
// PickupCorrectionIntake。
type PickupAttemptResultCorrectionIntake interface {
	IntakePickupAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectPickupAttemptResultCommand, error)
}

// DeliveryAttemptResultCorrectionIntake 把已认证的接入请求翻译成派送结果更正命令。
type DeliveryAttemptResultCorrectionIntake interface {
	IntakeDeliveryAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectDeliveryAttemptResultCommand, error)
}

// PickupAttemptResultCorrectionPayload 是揽收结果更正的线格式：被更正的那一格（尝试提交的来源身份加对象）、更正成的确定结果
// 词、依据（失败与拒收的原因）、所凭的后补来源、控制依据与段声明（只属更正成揽收到手）、两个业务时刻。载荷只有内容没有身份。
type PickupAttemptResultCorrectionPayload struct {
	SourceID             string `json:"sourceId"`
	Object               string `json:"object"`
	Outcome              string `json:"outcome"`
	Basis                string `json:"basis,omitempty"`
	Source               string `json:"source"`
	Control              string `json:"control,omitempty"`
	OccurredAt           string `json:"occurredAt"`
	CorrectedAt          string `json:"correctedAt"`
	Segment              string `json:"segment,omitempty"`
	PlannedSegment       string `json:"plannedSegment,omitempty"`
	SegmentServiceAction string `json:"segmentServiceAction,omitempty"`
}

// Command 把载荷连同信封给的租户（与运营决定面的操作者）翻成更正命令。时刻解不出、结果词不在封闭集内是坏报文（400）；引用缺席时照
// 零值交进去，由编排答`来源未受理`。decidedBy 在作业事实面为空。
func (payload PickupAttemptResultCorrectionPayload) Command(
	tenant domain.TenantID,
	decidedBy string,
) (application.CorrectPickupAttemptResultCommand, error) {
	none := application.CorrectPickupAttemptResultCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.CorrectPickupAttemptResultCommand{
		TenantID:             tenant,
		SourceID:             payload.SourceID,
		Object:               payload.Object,
		Segment:              payload.Segment,
		PlannedSegment:       payload.PlannedSegment,
		SegmentServiceAction: payload.SegmentServiceAction,
		DecidedBy:            decidedBy,
	}
	var err error
	if payload.Outcome != "" {
		if command.Outcome, err = attemptObjectOutcomeFromWord(payload.Outcome); err != nil {
			return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
	}
	if payload.Basis != "" {
		if command.Basis, err = domain.NewAttemptResultBasisReference(payload.Basis); err != nil {
			return none, fmt.Errorf("%w: basis: %v", ErrMalformedRequest, err)
		}
	}
	if payload.Source != "" {
		if command.Source, err = domain.NewAttemptResultSourceReference(payload.Source); err != nil {
			return none, fmt.Errorf("%w: source: %v", ErrMalformedRequest, err)
		}
	}
	if payload.Control != "" {
		if command.Control, err = domain.NewTransportControlReference(payload.Control); err != nil {
			return none, fmt.Errorf("%w: control: %v", ErrMalformedRequest, err)
		}
	}
	if command.OccurredAt, err = parseOptionalInstant("occurredAt", payload.OccurredAt); err != nil {
		return none, err
	}
	if command.CorrectedAt, err = parseOptionalInstant("correctedAt", payload.CorrectedAt); err != nil {
		return none, err
	}
	return command, nil
}

// DeliveryAttemptResultCorrectionPayload 是派送结果更正的线格式；没有控制依据与段声明，也没有 POD——妥投的证据走交付登记。
type DeliveryAttemptResultCorrectionPayload struct {
	Attempt     string `json:"attempt"`
	Object      string `json:"object"`
	Outcome     string `json:"outcome"`
	Basis       string `json:"basis,omitempty"`
	Source      string `json:"source"`
	OccurredAt  string `json:"occurredAt"`
	CorrectedAt string `json:"correctedAt"`
}

func (payload DeliveryAttemptResultCorrectionPayload) Command(
	tenant domain.TenantID,
	decidedBy string,
) (application.CorrectDeliveryAttemptResultCommand, error) {
	none := application.CorrectDeliveryAttemptResultCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.CorrectDeliveryAttemptResultCommand{
		TenantID:  tenant,
		Attempt:   payload.Attempt,
		Object:    payload.Object,
		DecidedBy: decidedBy,
	}
	var err error
	if payload.Outcome != "" {
		if command.Outcome, err = deliveryObjectOutcomeFromWord(payload.Outcome); err != nil {
			return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
	}
	if payload.Basis != "" {
		if command.Basis, err = domain.NewAttemptResultBasisReference(payload.Basis); err != nil {
			return none, fmt.Errorf("%w: basis: %v", ErrMalformedRequest, err)
		}
	}
	if payload.Source != "" {
		if command.Source, err = domain.NewAttemptResultSourceReference(payload.Source); err != nil {
			return none, fmt.Errorf("%w: source: %v", ErrMalformedRequest, err)
		}
	}
	if command.OccurredAt, err = parseOptionalInstant("occurredAt", payload.OccurredAt); err != nil {
		return none, err
	}
	if command.CorrectedAt, err = parseOptionalInstant("correctedAt", payload.CorrectedAt); err != nil {
		return none, err
	}
	return command, nil
}

// PickupAttemptResultCorrector 与 DeliveryAttemptResultCorrector 是本适配器转交的应用编排。
type PickupAttemptResultCorrector interface {
	Correct(ctx context.Context, command application.CorrectPickupAttemptResultCommand) (application.CorrectPickupAttemptResultResult, error)
}

type DeliveryAttemptResultCorrector interface {
	Correct(ctx context.Context, command application.CorrectDeliveryAttemptResultCommand) (application.CorrectDeliveryAttemptResultResult, error)
}

// NewCorrectPickupAttemptResultEndpoint 交回揽收结果更正的 HTTP 入口。
func NewCorrectPickupAttemptResultEndpoint(intake PickupAttemptResultCorrectionIntake, corrector PickupAttemptResultCorrector) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CorrectPickupAttemptResultResult, error, bool) {
		command, err := intake.IntakePickupAttemptResultCorrection(request.Context(), request)
		if err != nil {
			return application.CorrectPickupAttemptResultResult{}, err, false
		}
		result, err := corrector.Correct(request.Context(), command)
		return result, err, true
	}, writePickupAttemptResultCorrectionOutcome)
}

// NewCorrectDeliveryAttemptResultEndpoint 交回派送结果更正的 HTTP 入口。
func NewCorrectDeliveryAttemptResultEndpoint(intake DeliveryAttemptResultCorrectionIntake, corrector DeliveryAttemptResultCorrector) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CorrectDeliveryAttemptResultResult, error, bool) {
		command, err := intake.IntakeDeliveryAttemptResultCorrection(request.Context(), request)
		if err != nil {
			return application.CorrectDeliveryAttemptResultResult{}, err, false
		}
		result, err := corrector.Correct(request.Context(), command)
		return result, err, true
	}, writeDeliveryAttemptResultCorrectionOutcome)
}

// attemptResultCorrectionBody 是两口共用的答复主体：更正成的确定结果、所凭的后补来源与更正时刻，以及被更正的待确认那一版
// 的来源与缺口——复核的人要从答复里认出更正回指的是哪一格。**不回显操作者**（ADR-0303 的待遇：只进审计，不进回执）。
type attemptResultCorrectionBody struct {
	Outcome     string `json:"outcome"`
	Basis       string `json:"basis,omitempty"`
	Source      string `json:"source"`
	OccurredAt  string `json:"occurredAt"`
	CorrectedAt string `json:"correctedAt"`
	Pending     struct {
		Source string `json:"source"`
		Gap    string `json:"gap"`
	} `json:"pending"`
}

type pickupAttemptResultCorrectionResponse struct {
	Outcome               string                       `json:"outcome"`
	UndecidedReason       string                       `json:"undecidedReason,omitempty"`
	Attempt               string                       `json:"attempt,omitempty"`
	Object                string                       `json:"object,omitempty"`
	Correction            *attemptResultCorrectionBody `json:"correction,omitempty"`
	PickupVersion         string                       `json:"pickupVersion,omitempty"`
	ContinuationReference string                       `json:"continuationReference,omitempty"`
	HandoffReference      string                       `json:"handoffReference,omitempty"`
	SegmentContinuation   string                       `json:"segmentContinuationReference,omitempty"`
	SegmentEntryRefusal   string                       `json:"segmentEntryRefusal,omitempty"`
}

type deliveryAttemptResultCorrectionResponse struct {
	Outcome               string                       `json:"outcome"`
	UndecidedReason       string                       `json:"undecidedReason,omitempty"`
	Attempt               string                       `json:"attempt,omitempty"`
	Object                string                       `json:"object,omitempty"`
	Correction            *attemptResultCorrectionBody `json:"correction,omitempty"`
	ContinuationReference string                       `json:"continuationReference,omitempty"`
}

func pickupCorrectionBody(correction domain.AttemptObjectResultCorrection) *attemptResultCorrectionBody {
	effective, pending := correction.Effective(), correction.Pending()
	body := &attemptResultCorrectionBody{
		Outcome:     effective.Outcome().String(),
		Source:      correction.Source().String(),
		OccurredAt:  rfc3339(effective.OccurredAt()),
		CorrectedAt: rfc3339(correction.CorrectedAt()),
	}
	if basis, present := effective.Basis(); present {
		body.Basis = basis.String()
	}
	if source, present := pending.Source(); present {
		body.Pending.Source = source.String()
	}
	if gap, present := pending.Basis(); present {
		body.Pending.Gap = gap.String()
	}
	return body
}

func deliveryCorrectionBody(correction domain.DeliveryAttemptResultCorrection) *attemptResultCorrectionBody {
	effective, pending := correction.Effective(), correction.Pending()
	body := &attemptResultCorrectionBody{
		Outcome:     effective.Outcome().String(),
		Source:      correction.Source().String(),
		OccurredAt:  rfc3339(effective.OccurredAt()),
		CorrectedAt: rfc3339(correction.CorrectedAt()),
	}
	if basis, present := effective.Basis(); present {
		body.Basis = basis.String()
	}
	if source, present := pending.Source(); present {
		body.Pending.Source = source.String()
	}
	if gap, present := pending.Basis(); present {
		body.Pending.Gap = gap.String()
	}
	return body
}

func writePickupAttemptResultCorrectionOutcome(response http.ResponseWriter, result application.CorrectPickupAttemptResultResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := pickupAttemptResultCorrectionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		Attempt:               result.Attempt().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		body.Object = record.Correction.Pending().Object().String()
		body.Correction = pickupCorrectionBody(record.Correction)
	}
	registration := result.Registration()
	if registered, present := registration.Record(); present {
		body.PickupVersion = registered.Pickup.Version().String()
	}
	body.HandoffReference = registration.PickupHandoffReference()
	body.SegmentContinuation = registration.SegmentContinuationReference()
	body.SegmentEntryRefusal = registration.SegmentEntryRefusal().String()

	// ADR-0022：只有这次新落了更正用 201；重放、冲突、并非待确认、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.AttemptResultCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func writeDeliveryAttemptResultCorrectionOutcome(response http.ResponseWriter, result application.CorrectDeliveryAttemptResultResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := deliveryAttemptResultCorrectionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		body.Attempt = record.Key.Attempt.String()
		body.Object = record.Correction.Pending().Object().String()
		body.Correction = deliveryCorrectionBody(record.Correction)
	}
	status := http.StatusOK
	if result.Outcome() == application.AttemptResultCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
