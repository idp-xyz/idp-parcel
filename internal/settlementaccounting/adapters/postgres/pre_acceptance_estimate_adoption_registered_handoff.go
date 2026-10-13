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
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// preAcceptanceEstimateAdoptionRegisteredEventType 是「受理前估价采用已登记」的事件类型，parcel-shipment 那扇金额续办门按它收信
// （票 product-strategy-boundary/25）。与消费侧各写各的字面，理由同「结算账户已登记」。
const preAcceptanceEstimateAdoptionRegisteredEventType eventing.EventType = "settlement-accounting.pre-acceptance-estimate-adoption.registered"

// preAcceptanceEstimateAdoptionRegisteredEventIDPort 是信封 ID 指纹的口名前缀，只求本来源名下几只口的 ID 空间互不相交。
const preAcceptanceEstimateAdoptionRegisteredEventIDPort = "pre-acceptance-estimate-adoption"

// preAcceptanceEstimateAdoptionRegistrationKind 是载荷里的登记种类原词，进载荷的理由同 settlementAccountRegistrationKind。
const preAcceptanceEstimateAdoptionRegistrationKind = "PRE_ACCEPTANCE_ESTIMATE_ADOPTION"

// OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff 把「受理前估价采用已登记」写入 Outbox，实现
// ports.PreAcceptanceEstimateAdoptionRegisteredHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxPreAcceptanceEstimateAdoptionRegisteredHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.PreAcceptanceEstimateAdoptionRegisteredHandoff = (*OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff)(nil)

// HandOffPreAcceptanceEstimateAdoptionRegistered 把一份意图入队。载荷与「结算账户已登记」同一份两键契约（tenantId、
// registrationKind），PS 的登记续办门按租户重驱等待队列。ID 取（租户 / 账户）指纹：一户一封，重放由 EnqueueOnce 按 ID 吞掉
// （ADR-0043）。租户、账户或登记时刻缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff) HandOffPreAcceptanceEstimateAdoptionRegistered(
	ctx context.Context,
	intent ports.PreAcceptanceEstimateAdoptionRegisteredIntent,
) error {
	if intent.Tenant.String() == "" || intent.Account.String() == "" {
		return fmt.Errorf("hand off pre-acceptance estimate adoption registered: tenant and account are required")
	}
	if intent.RegisteredAt.IsZero() {
		return fmt.Errorf("hand off pre-acceptance estimate adoption registered: registration moment is required")
	}

	payload, err := json.Marshal(settlementAccountRegisteredPayload{
		TenantID:         intent.Tenant.String(),
		RegistrationKind: preAcceptanceEstimateAdoptionRegistrationKind,
	})
	if err != nil {
		return fmt.Errorf("hand off pre-acceptance estimate adoption registered: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID(preAcceptanceEstimateAdoptionRegisteredEventIDPort,
			intent.Tenant.String(), intent.Account.String()),
		Source:  saEventSource,
		Type:    preAcceptanceEstimateAdoptionRegisteredEventType,
		Version: 1,
		Scope:   intent.Tenant.String(),
		Subject: preAcceptanceEstimateAdoptionRegistrationKind + "/" + intent.Account.String(),
		// 分区装租户，与「结算账户已登记」、party-commercial「参数已登记」同一个字符串：几口的信封驱的是同一张等待队列，同队让
		// 同一租户的重驱一轮接一轮、不并发去推同一份委托。
		PartitionKey: intent.Tenant.String(),
		OccurredAt:   intent.RegisteredAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off pre-acceptance estimate adoption registered: %w: %w",
				ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected, err)
		}
		return fmt.Errorf("hand off pre-acceptance estimate adoption registered: %w", err)
	}
	return nil
}
