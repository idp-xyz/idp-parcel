package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// evaluationParameterRegisteredEventType 是「评价参数已登记」的事件类型，结算自己的续办门按它收信（ADR-0229 决定五）。
// 与消费侧各写各的字面；两串漂开撞 dispatch.no_subscriber，由 cmd/parcel-dispatch 那条用本适配器铸封的用例揪红。
const evaluationParameterRegisteredEventType eventing.EventType = "settlement-accounting.evaluation-parameter.registered"

// evaluationParameterRegisteredEventIDPort 是信封 ID 指纹的口名前缀。
const evaluationParameterRegisteredEventIDPort = "evaluation-parameter"

// evaluationParameterKinds 是本口收的种类：结算自己那几本评价参数册。结算账户另有自己的「已登记」口，计价的登记由计价交。
var evaluationParameterKinds = map[domain.EvaluationParameterKind]bool{
	domain.ParameterSellBillingMomentAdoption: true,
	domain.ParameterFeeItemMapping:            true,
	domain.ParameterBuyEvaluationTrigger:      true,
}

// OutboxEvaluationParameterRegisteredHandoff 实现 ports.EvaluationParameterRegisteredHandoff。
type OutboxEvaluationParameterRegisteredHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEvaluationParameterRegisteredHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEvaluationParameterRegisteredHandoff, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: evaluation parameter handoff needs db, outbox store and clock")
	}
	return &OutboxEvaluationParameterRegisteredHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.EvaluationParameterRegisteredHandoff = (*OutboxEvaluationParameterRegisteredHandoff)(nil)

// evaluationParameterRegisteredPayload 只有租户与种类两键，与「结算账户已登记」同形：续办门按租户与种类取等着的件，
// 登的是哪一行它不需要知道——续办时发起编排照旧问各本册。
type evaluationParameterRegisteredPayload struct {
	TenantID         string `json:"tenantId"`
	RegistrationKind string `json:"registrationKind"`
}

// HandOffEvaluationParameterRegistered 把一份意图入队。信封 ID 取（租户 / 种类 / 落的那一行）的定长指纹：一行一封，重放同一行
// 由 EnqueueOnce 按 ID 吞掉（ADR-0043）；续办把它当依据，同一封信重投不会把同一条代次链推两代。种类不归本口、租户或行键
// 缺席、登记时刻零值都是装配缺陷，响亮报错不入队。
func (handoff *OutboxEvaluationParameterRegisteredHandoff) HandOffEvaluationParameterRegistered(
	ctx context.Context,
	intent ports.EvaluationParameterRegisteredIntent,
) error {
	if !evaluationParameterKinds[intent.Kind] || intent.Tenant.String() == "" || intent.Subject == "" || intent.RegisteredAt.IsZero() {
		return fmt.Errorf("hand off evaluation parameter registered: kind %q, tenant, subject and moment are required", intent.Kind.String())
	}
	payload, err := json.Marshal(evaluationParameterRegisteredPayload{
		TenantID:         intent.Tenant.String(),
		RegistrationKind: intent.Kind.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off evaluation parameter registered: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID(evaluationParameterRegisteredEventIDPort,
			intent.Tenant.String(), intent.Kind.String(), intent.Subject),
		Source:  saEventSource,
		Type:    evaluationParameterRegisteredEventType,
		Version: 1,
		Scope:   intent.Tenant.String(),
		Subject: intent.Kind.String(),
		// 分区装租户，与「结算账户已登记」同一个字符串：同一租户的续办一轮接一轮，不会两轮并发去推同一条代次链。
		PartitionKey: intent.Tenant.String(),
		OccurredAt:   intent.RegisteredAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off evaluation parameter registered: %w: %w", ports.ErrEvaluationParameterHandoffRejected, err)
		}
		return fmt.Errorf("hand off evaluation parameter registered: %w", err)
	}
	return nil
}
