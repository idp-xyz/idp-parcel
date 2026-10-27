package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 尝试结果更正两口的作业事实面（票 docs-consistency/code/90）：执行方凭同一次上门的后补来源（伙伴迟到的签收……）更正自己登下的待确认。
// 认证与授予同尝试登记两口那一份（更正随原事实的族，ADR-0151 决定四），另立文件不并进 work_fact_intake.go，理由同
// operator_decision_intake_corrections.go。作业事实面不带操作者：设备身份不进载荷（ADR-0023），更正命令的操作证据一格留空。

var (
	_ PickupAttemptResultCorrectionIntake   = (*WorkFactCommandIntake)(nil)
	_ DeliveryAttemptResultCorrectionIntake = (*WorkFactCommandIntake)(nil)
)

func (intake *WorkFactCommandIntake) IntakePickupAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectPickupAttemptResultCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.CorrectPickupAttemptResultCommand{}, err
	}
	var payload PickupAttemptResultCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectPickupAttemptResultCommand{}, err
	}
	return payload.Command(tenant, "")
}

func (intake *WorkFactCommandIntake) IntakeDeliveryAttemptResultCorrection(ctx context.Context, request *http.Request) (application.CorrectDeliveryAttemptResultCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.CorrectDeliveryAttemptResultCommand{}, err
	}
	var payload DeliveryAttemptResultCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectDeliveryAttemptResultCommand{}, err
	}
	return payload.Command(tenant, "")
}
