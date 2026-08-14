package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// caseClosureEventType 是关务案件关闭意图的事件类型。
const caseClosureEventType = "customs-compliance.case-closure.recorded"

// OutboxCaseClosureHandoff 把关闭决定写入 Outbox，实现 ports.CaseClosureHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCaseClosureHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCaseClosureHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCaseClosureHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxCaseClosureHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CaseClosureHandoff = (*OutboxCaseClosureHandoff)(nil)

// caseClosurePayload 是意图载荷的传输形状：只有下游 FindByCase 所需的租户与案件引用。
type caseClosurePayload struct {
	TenantID string `json:"tenantId"`
	CaseRef  string `json:"caseRef"`
}

func caseClosureEventID(tenant, caseRef string) string {
	return tenant + "/" + caseRef
}

// HandOffClosure 把一份意图入队。信封 ID 取租户加案件引用——关闭按案件定位。租户或
// 案件引用空白、关闭缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCaseClosureHandoff) HandOffClosure(
	ctx context.Context,
	intent ports.CaseClosureHandoffIntent,
) error {
	if intent.Closure == nil || intent.TenantID.String() == "" || intent.Closure.CaseRef() == "" {
		return fmt.Errorf("hand off case closure: tenant and case ref are required")
	}

	payload, err := json.Marshal(caseClosurePayload{
		TenantID: intent.TenantID.String(),
		CaseRef:  intent.Closure.CaseRef(),
	})
	if err != nil {
		return fmt.Errorf("hand off case closure: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := caseClosureEventID(intent.TenantID.String(), intent.Closure.CaseRef())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         caseClosureEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Closure.CaseRef(),
		PartitionKey: eventID,
		OccurredAt:   intent.Closure.ClosedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off case closure: %w", err)
	}
	return nil
}
