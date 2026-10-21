package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// evaluationRequestUnformedEventType 是「评价未形成」的事件类型，结算的消费门按它收信（ADR-0229 决定三）。与消费侧各写
// 各的字面；两串漂开撞 dispatch.no_subscriber，由 cmd/parcel-dispatch 用本包交接口入队的用例揪红。
const evaluationRequestUnformedEventType eventing.EventType = "parcel-pricing.evaluation-request.unformed"

// evaluationRequestUnformedEventIDPort 是信封 ID 指纹的口名前缀。
const evaluationRequestUnformedEventIDPort = "evaluation-request-unformed"

// evaluationRequestUnformedPartitionKey 取「租户/评价未形成/方向/请求」：主体是这一份请求，键带本口的段，不与结算那一侧
// 请求信封的分区同一字符串。
func evaluationRequestUnformedPartitionKey(tenant, direction, request string) string {
	return tenant + "/evaluation-request-unformed/" + direction + "/" + request
}

type evaluationRequestUnformedPayload struct {
	TenantID            string   `json:"tenantId"`
	EvaluationRequestID string   `json:"evaluationRequestId"`
	Direction           string   `json:"direction"`
	Cause               string   `json:"cause"`
	Detail              []string `json:"detail,omitempty"`
}

// HandOffEvaluationRequestUnformed 把一次「评价未形成」入队。ID 取（租户 / 方向 / 请求）的定长指纹：一份请求至多一封，
// 请求信封重投时同一封由 EnqueueOnce 吞掉（ADR-0043）。停因不进 ID：同一份请求两次答出不同的停因，第二封被吞，结算那
// 一侧也不会给同一份请求记两个停因。
func (handoffs *OutboxEvaluationResumptionHandoffs) HandOffEvaluationRequestUnformed(
	ctx context.Context,
	intent ports.EvaluationRequestUnformedIntent,
) error {
	tenant, request, direction := intent.Tenant.String(), intent.Request.String(), string(intent.Direction)
	if tenant == "" || request == "" || direction == "" || intent.Cause.String() == "" {
		return fmt.Errorf("hand off evaluation request unformed: tenant, request, direction and cause are required")
	}
	payload, err := json.Marshal(evaluationRequestUnformedPayload{
		TenantID:            tenant,
		EvaluationRequestID: request,
		Direction:           direction,
		Cause:               intent.Cause.String(),
		Detail:              intent.Detail,
	})
	if err != nil {
		return fmt.Errorf("hand off evaluation request unformed: %w", err)
	}
	now := handoffs.clock.Now().UTC()
	return handoffs.enqueue(ctx, eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          outboxintent.FingerprintEventID(evaluationRequestUnformedEventIDPort, tenant, direction, request),
		Source:      ppEventSource,
		Type:        evaluationRequestUnformedEventType,
		Version:     1,
		Scope:       tenant,
		Subject:     direction + "/" + request,
		// 分区按这一份请求：一份请求至多一封，各请求互不排队。不与「参数已登记」同队——同一租户停着的请求可以成千上万，
		// 排进租户那一条队就要一拍一封地等；代价是「卡登了」可能先于「这份请求停在没卡」被结算处理，那一份要等下一次
		// 登记或运营续办（ADR-0229 越权风险点）。
		PartitionKey: evaluationRequestUnformedPartitionKey(tenant, direction, request),
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}, ports.ErrEvaluationRequestUnformedHandoffRejected)
}
