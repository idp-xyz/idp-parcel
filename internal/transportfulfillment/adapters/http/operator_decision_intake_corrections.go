package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 更正两口的运营决定（ADR-0232 决定一、四）：解除履约段采用与运输收费发生项失效各成一种，与采用、登记分开授予——能声明的不因此能
// 撤回。另立文件、不并进 operator_decision_intake.go：两口的种类、Intake 与载荷自成一组，别的特性往那份文件里加口时不必与它们排队。

const (
	OperatorDecisionSegmentCommissionAdoptionWithdrawal OperatorDecision = "FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL"
	OperatorDecisionChargeOccurrenceInvalidation        OperatorDecision = "TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION"
)

var _ ChargeOccurrenceInvalidationIntake = (*OperatorDecisionIntake)(nil)

// IntakeChargeOccurrenceInvalidation 译运输收费发生项失效（`/transport-fulfillment-charge-occurrence-invalidations`）。操作者随命令
// 带进去：失效版本记下作出这一次决定的人（ADR-0232 决定四）。
func (intake *OperatorDecisionIntake) IntakeChargeOccurrenceInvalidation(ctx context.Context, request *http.Request) (application.InvalidateChargeOccurrenceCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionChargeOccurrenceInvalidation)
	if err != nil {
		return application.InvalidateChargeOccurrenceCommand{}, err
	}
	var payload ChargeOccurrenceInvalidationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.InvalidateChargeOccurrenceCommand{}, err
	}
	return payload.Command(operator)
}
