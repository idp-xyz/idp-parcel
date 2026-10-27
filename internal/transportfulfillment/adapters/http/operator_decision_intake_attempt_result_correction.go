package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 尝试结果更正两口的运营决定面（票 docs-consistency/code/90，ADR-0297）：复核人核对来源之后认定一格待确认的结果。揽收与派送各成一种
// 决定，与建派送任务、终止参与、承运商首次有效收寄判断都分开授予——能判收寄的不因此能替一次上门认定某件对象到手没有，授予的粒度
// 等于权力的粒度。另立文件，理由同 operator_decision_intake_corrections.go。

const (
	OperatorDecisionPickupAttemptResultConfirmation   OperatorDecision = "PICKUP_ATTEMPT_RESULT_CONFIRMATION"
	OperatorDecisionDeliveryAttemptResultConfirmation OperatorDecision = "DELIVERY_ATTEMPT_RESULT_CONFIRMATION"
)

var (
	_ PickupAttemptResultCorrectionIntake   = (*OperatorDecisionIntake)(nil)
	_ DeliveryAttemptResultCorrectionIntake = (*OperatorDecisionIntake)(nil)
)

// IntakePickupAttemptResultCorrection 译揽收结果认定（`/transport-fulfillment-pickup-attempt-result-confirmations`）。操作者随命令带进去，
// 作为操作证据记在更正上，只进审计（ADR-0303 的待遇）。
func (intake *OperatorDecisionIntake) IntakePickupAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectPickupAttemptResultCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionPickupAttemptResultConfirmation)
	if err != nil {
		return application.CorrectPickupAttemptResultCommand{}, err
	}
	if strings.TrimSpace(operator.Operator) == "" {
		return application.CorrectPickupAttemptResultCommand{}, ErrOperatorIdentityMissing
	}
	var payload PickupAttemptResultCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectPickupAttemptResultCommand{}, err
	}
	return payload.Command(operator.Tenant, operator.Operator)
}

// IntakeDeliveryAttemptResultCorrection 译派送结果认定（`/transport-fulfillment-delivery-attempt-result-confirmations`）。
func (intake *OperatorDecisionIntake) IntakeDeliveryAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectDeliveryAttemptResultCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionDeliveryAttemptResultConfirmation)
	if err != nil {
		return application.CorrectDeliveryAttemptResultCommand{}, err
	}
	if strings.TrimSpace(operator.Operator) == "" {
		return application.CorrectDeliveryAttemptResultCommand{}, ErrOperatorIdentityMissing
	}
	var payload DeliveryAttemptResultCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectDeliveryAttemptResultCommand{}, err
	}
	return payload.Command(operator.Tenant, operator.Operator)
}
