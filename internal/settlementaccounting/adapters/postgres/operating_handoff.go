package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const (
	costAllocationEventType  eventing.EventType = "settlement-accounting.cost-allocation.formed"
	operatingResultEventType eventing.EventType = "settlement-accounting.operating-result.derived"
)

// OutboxOperatingHandoff 把成本分摊或经营结果写入 Outbox，实现 ports.OperatingHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxOperatingHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOperatingHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxOperatingHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxOperatingHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.OperatingHandoff = (*OutboxOperatingHandoff)(nil)

type operatingPayload struct {
	TenantID   string `json:"tenantId"`
	Allocation string `json:"allocation,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Period     string `json:"period,omitempty"`
	Basis      string `json:"basis,omitempty"`
}

type operatingShape struct {
	eventID    string
	eventType  eventing.EventType
	tenant     string
	subject    string
	occurredAt time.Time
	payload    operatingPayload
}

func operatingIdentity(intent ports.OperatingIntent) (operatingShape, error) {
	if intent.Allocation.Key.Allocation.String() != "" {
		tenant := intent.Allocation.Key.TenantID.String()
		allocation := intent.Allocation.Key.Allocation.String()
		if tenant == "" {
			return operatingShape{}, fmt.Errorf("hand off operating: allocation tenant is required")
		}
		return operatingShape{
			eventID:    tenant + "/allocation/" + allocation,
			eventType:  costAllocationEventType,
			tenant:     tenant,
			subject:    allocation,
			occurredAt: intent.Allocation.RecordedAt.UTC(),
			payload: operatingPayload{
				TenantID:   tenant,
				Allocation: allocation,
			},
		}, nil
	}
	key := intent.Result.Key
	if key.TenantID.String() == "" || key.Scope.String() == "" || key.Period.String() == "" || key.Basis.String() == "" {
		return operatingShape{}, fmt.Errorf("hand off operating: allocation or operating result is required")
	}
	return operatingShape{
		eventID:    key.TenantID.String() + "/operating-result/" + key.Scope.String() + "/" + key.Period.String() + "/" + key.Basis.String(),
		eventType:  operatingResultEventType,
		tenant:     key.TenantID.String(),
		subject:    key.Scope.String() + "/" + key.Period.String() + "/" + key.Basis.String(),
		occurredAt: intent.Result.RecordedAt.UTC(),
		payload: operatingPayload{
			TenantID: key.TenantID.String(),
			Scope:    key.Scope.String(),
			Period:   key.Period.String(),
			Basis:    key.Basis.String(),
		},
	}, nil
}

// HandOffOperating 把一份意图入队。分摊由分摊标识认领，指标由（口径+账期+基准）认领；
// 两半都缺是装配缺陷。入队走 EnqueueOnce。
func (handoff *OutboxOperatingHandoff) HandOffOperating(
	ctx context.Context,
	intent ports.OperatingIntent,
) error {
	shape, err := operatingIdentity(intent)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(shape.payload)
	if err != nil {
		return fmt.Errorf("hand off operating: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(shape.eventID),
		Source:       saEventSource,
		Type:         shape.eventType,
		Version:      1,
		Scope:        shape.tenant,
		Subject:      shape.subject,
		PartitionKey: shape.eventID,
		OccurredAt:   shape.occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off operating: %w", err)
	}
	return nil
}
