package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DepartureGateReaskRecordedEventType 是「TF 给一条出发追加了一次门禁补问读数」那封引用式信封的类型（ADR-0305）。消费方是全程追踪
// 的出发门禁阻断信号，它按自己的惯例自写同一串；导出只为 cmd/parcel-dispatch 那条同时装配两边的用例对照两串相等。
const DepartureGateReaskRecordedEventType = "transport-fulfillment.departure-gate-reask.recorded"

// OutboxDepartureGateReaskHandoff 把追加的补问写入 Outbox，实现 ports.DepartureGateReaskHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxDepartureGateReaskHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDepartureGateReaskHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDepartureGateReaskHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxDepartureGateReaskHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DepartureGateReaskHandoff = (*OutboxDepartureGateReaskHandoff)(nil)

// departureGateReaskPayload 恰是补问登记册的键五维。读数不带：它只有登记册一处权威，消费方按键回读——信封上抄一份，就有两份「当时
// 关务怎么答」。次序是数，信封上写成十进制串，与其余各维同为串。
type departureGateReaskPayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
	Object   string `json:"object"`
	Sequence string `json:"sequence"`
}

// HandOffDepartureGateReask 把一次补问入队。ID 由键五维经指纹铸出，同一次补问只有一封；分区取它补的那条出发所在的那一区
// （movementFactPartitionKey）：补问排在那条出发的已形成信封之后，全程追踪据它形成信号时，那条出发本身已经交过一次。
func (handoff *OutboxDepartureGateReaskHandoff) HandOffDepartureGateReask(
	ctx context.Context,
	record ports.DepartureGateReaskRecord,
) error {
	key := record.Key
	if key.TenantID.String() == "" || key.Fact.String() == "" || key.Version.String() == "" || key.Object.String() == "" ||
		key.Sequence < 1 {
		return fmt.Errorf("hand off departure gate reask: departure gate reask key is required")
	}
	sequence := strconv.Itoa(key.Sequence)
	payload, err := json.Marshal(departureGateReaskPayload{
		TenantID: key.TenantID.String(),
		Fact:     key.Fact.String(),
		Version:  key.Version.String(),
		Object:   key.Object.String(),
		Sequence: sequence,
	})
	if err != nil {
		return fmt.Errorf("hand off departure gate reask: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID("departure-gate-reask-recorded",
			key.TenantID.String(), key.Fact.String(), key.Version.String(), key.Object.String(), sequence),
		Source:       tfEventSource,
		Type:         DepartureGateReaskRecordedEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Fact.String(),
		PartitionKey: movementFactPartitionKey(key.Departure()),
		OccurredAt:   record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off departure gate reask: %w", err)
	}
	return nil
}
