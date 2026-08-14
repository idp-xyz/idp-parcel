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

// statementHandoffShape 把信封 ID 与分区键分开持有。两者管的不是一回事：ID 管幂等
// （同一份不重发），分区键管顺序（同一对象的先后拍排队）。写成同一个字符串会二选一
// 地坏掉其中一样——本口此前正是如此：ID 上加了 `/voided` 所以作废不丢，但分区键跟着
// ID 走，于是发布与作废落进两个分区，作废可能先于它作废的那份送达。
type statementHandoffShape struct {
	eventID      string
	partitionKey string
	eventType    eventing.EventType
	tenant       string
	subject      string
	occurredAt   time.Time
	payload      statementHandoffPayload
}

func statementHandoffIdentity(intent ports.StatementIntent) (statementHandoffShape, error) {
	if intent.Inclusion.Key.Inclusion.String() != "" {
		tenant := intent.Inclusion.Key.TenantID.String()
		inclusion := intent.Inclusion.Key.Inclusion.String()
		if tenant == "" {
			return statementHandoffShape{}, fmt.Errorf("hand off statement: inclusion tenant is required")
		}
		// 纳入的主体就是纳入本身：它没有更正入口，一次纳入只有一个状态，因此不存在
		// 需要排队的先后拍。分区键取到它这一层不会让任何顺序保证落空。
		return statementHandoffShape{
			eventID:      tenant + "/inclusion/" + inclusion,
			partitionKey: tenant + "/inclusion/" + inclusion,
			eventType:    statementInclusionEventType,
			tenant:       tenant,
			subject:      inclusion,
			occurredAt:   intent.Inclusion.Inclusion.IncludedAt().UTC(),
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

	// 主体取「其先后状态必须保序的那个对象」——这里是对账单本身。发布与作废是同一
	// 张单的两拍，作废必须排在它作废的那份之后，因此两者共用一个分区；单号不同的
	// 对账单互不排队。刻意不把 `/voided` 拼进分区键：那会把这两拍切成互不排队的
	// 两段，正是本口原先的毛病。
	partitionKey := tenant + "/statement/" + number

	eventType := statementPublishedEventType
	eventID := partitionKey
	occurredAt := intent.Statement.Statement.PublishedAt().UTC()
	if voided {
		eventType = statementVoidedEventType
		eventID += "/voided"
		if _, at, ok := intent.Statement.Statement.Voided(); ok {
			occurredAt = at.UTC()
		}
	}
	return statementHandoffShape{
		eventID:      eventID,
		partitionKey: partitionKey,
		eventType:    eventType,
		tenant:       tenant,
		subject:      number,
		occurredAt:   occurredAt,
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
		PartitionKey: shape.partitionKey,
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
