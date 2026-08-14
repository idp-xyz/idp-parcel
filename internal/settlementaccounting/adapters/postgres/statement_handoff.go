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
	statementPublishedEventType eventing.EventType = "settlement-accounting.statement.published"
	statementVoidedEventType    eventing.EventType = "settlement-accounting.statement.voided"
	statementInclusionEventType eventing.EventType = "settlement-accounting.statement.included"
)

// OutboxStatementHandoff 把对账单事件意图写进 Outbox，实现 ports.StatementHandoff。
// 发布、作废与后续纳入各携其记录，信封 ID 按所携记录认领。入队走 EnqueueOnce。
type OutboxStatementHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxStatementHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxStatementHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxStatementHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.StatementHandoff = (*OutboxStatementHandoff)(nil)

type statementHandoffPayload struct {
	TenantID  string `json:"tenantId"`
	Statement string `json:"statement,omitempty"`
	Inclusion string `json:"inclusion,omitempty"`
}

type statementHandoffShape struct {
	eventID    string
	eventType  eventing.EventType
	tenant     string
	subject    string
	occurredAt time.Time
	payload    statementHandoffPayload
}

func statementHandoffIdentity(intent ports.StatementIntent) (statementHandoffShape, error) {
	if intent.Inclusion.Key.Inclusion.String() != "" {
		tenant := intent.Inclusion.Key.TenantID.String()
		inclusion := intent.Inclusion.Key.Inclusion.String()
		if tenant == "" {
			return statementHandoffShape{}, fmt.Errorf("hand off statement: inclusion tenant is required")
		}
		return statementHandoffShape{
			eventID:    tenant + "/inclusion/" + inclusion,
			eventType:  statementInclusionEventType,
			tenant:     tenant,
			subject:    inclusion,
			occurredAt: intent.Inclusion.Inclusion.IncludedAt().UTC(),
			payload: statementHandoffPayload{
				TenantID:  tenant,
				Inclusion: inclusion,
			},
		}, nil
	}
	if intent.Statement.Key.Number.String() == "" || intent.Statement.Key.TenantID.String() == "" {
		return statementHandoffShape{}, fmt.Errorf("hand off statement: statement or inclusion is required")
	}
	tenant := intent.Statement.Key.TenantID.String()
	number := intent.Statement.Key.Number.String()
	_, _, voided := intent.Statement.Statement.Voided()
	eventType := statementPublishedEventType
	eventID := tenant + "/statement/" + number
	occurredAt := intent.Statement.Statement.PublishedAt().UTC()
	if voided {
		eventType = statementVoidedEventType
		eventID += "/voided"
		if _, at, ok := intent.Statement.Statement.Voided(); ok {
			occurredAt = at.UTC()
		}
	}
	return statementHandoffShape{
		eventID:    eventID,
		eventType:  eventType,
		tenant:     tenant,
		subject:    number,
		occurredAt: occurredAt,
		payload: statementHandoffPayload{
			TenantID:  tenant,
			Statement: number,
		},
	}, nil
}

// HandOffStatement 把一份意图入队。发布与作废由单号认领（作废另加 /voided，避免与
// 发布撞同一信封）；纳入由纳入标识认领。两半都缺是装配缺陷。
func (handoff *OutboxStatementHandoff) HandOffStatement(
	ctx context.Context,
	intent ports.StatementIntent,
) error {
	shape, err := statementHandoffIdentity(intent)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(shape.payload)
	if err != nil {
		return fmt.Errorf("hand off statement: %w", err)
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
		return fmt.Errorf("hand off statement: %w", err)
	}
	return nil
}
