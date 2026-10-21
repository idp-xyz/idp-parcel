package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// SegmentCommissionAdoptionRegisteredEventType 是「这一段采用了委托」那封门铃的类型（ADR-0231 决定一）。消费方是本上下文自己的实际履约
// 补形成；导出只为 inbox 消费门与 cmd/parcel-dispatch 那条同时装配两边的用例对照两串相等。
const SegmentCommissionAdoptionRegisteredEventType = "transport-fulfillment.segment-commission-adoption.registered"

// OutboxSegmentCommissionAdoptionHandoff 把一次采用的门铃写入 Outbox，实现 ports.SegmentCommissionAdoptionHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxSegmentCommissionAdoptionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSegmentCommissionAdoptionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSegmentCommissionAdoptionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxSegmentCommissionAdoptionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SegmentCommissionAdoptionHandoff = (*OutboxSegmentCommissionAdoptionHandoff)(nil)

// segmentCommissionAdoptionPayload 只带键：租户、段与它采用的委托。服务方与开始依据一样不带，它们只有采用册一处权威，消费方按段回读。
type segmentCommissionAdoptionPayload struct {
	TenantID   string `json:"tenantId"`
	Segment    string `json:"segment"`
	Commission string `json:"commission"`
}

// HandOffSegmentCommissionAdoption 把一封门铃入队。信封 ID 由（租户、段、委托）经指纹铸出：今天一段只采用一份委托，委托进 ID 是为了
// 日后段改采用另一份委托时那一封不被 EnqueueOnce 当成重发吞掉。分区取「租户/履约段」：门铃彼此没有先后，同一段的门铃排一条队只为
// 不与别的段互相挡。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxSegmentCommissionAdoptionHandoff) HandOffSegmentCommissionAdoption(
	ctx context.Context,
	intent ports.SegmentCommissionAdoptedIntent,
) error {
	key := intent.Record.Key
	commission := intent.Record.Adoption.Commission()
	if key.TenantID.String() == "" || key.Segment.String() == "" || commission.String() == "" {
		return fmt.Errorf("hand off segment commission adoption: adoption key is required")
	}
	payload, err := json.Marshal(segmentCommissionAdoptionPayload{
		TenantID:   key.TenantID.String(),
		Segment:    key.Segment.String(),
		Commission: commission.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off segment commission adoption: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID("segment-commission-adoption-registered",
			key.TenantID.String(), key.Segment.String(), commission.String()),
		Source:       tfEventSource,
		Type:         SegmentCommissionAdoptionRegisteredEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Segment.String(),
		PartitionKey: key.TenantID.String() + "/fulfillment-segment/" + key.Segment.String() + "/segment-commission-adoption",
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off segment commission adoption: %w", err)
	}
	return nil
}
