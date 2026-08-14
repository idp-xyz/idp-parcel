package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// sealedSnapshotEventType 是封装快照意图的事件类型。
const sealedSnapshotEventType = "node-operations.sealed-snapshot.recorded"

// OutboxSealedSnapshotHandoff 把封装快照写入 Outbox，实现
// ports.SealedSnapshotHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSealedSnapshotHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSealedSnapshotHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSealedSnapshotHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxSealedSnapshotHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SealedSnapshotHandoff = (*OutboxSealedSnapshotHandoff)(nil)

// sealedSnapshotPayload 是意图载荷的传输形状：只有下游按（租户+单元+封签）定位所需
// 的认领键，不带成员明细。
type sealedSnapshotPayload struct {
	TenantID string `json:"tenantId"`
	UnitID   string `json:"unitId"`
	Seal     string `json:"seal"`
}

func sealedSnapshotEventID(tenant, unit, seal string) string {
	return tenant + "/" + unit + "/" + seal
}

// HandOffSnapshot 把一份意图入队。信封 ID 取租户加单元加封签——单元+快照认领键补
// 租户维（ADR-0003 / ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxSealedSnapshotHandoff) HandOffSnapshot(
	ctx context.Context,
	intent ports.SealedSnapshotHandoffIntent,
) error {
	if intent.TenantID.String() == "" ||
		intent.Unit.String() == "" ||
		intent.Snapshot.Seal().String() == "" {
		return fmt.Errorf("hand off sealed snapshot: tenant, unit and seal are required")
	}

	payload, err := json.Marshal(sealedSnapshotPayload{
		TenantID: intent.TenantID.String(),
		UnitID:   intent.Unit.String(),
		Seal:     intent.Snapshot.Seal().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off sealed snapshot: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := sealedSnapshotEventID(intent.TenantID.String(), intent.Unit.String(), intent.Snapshot.Seal().String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       noEventSource,
		Type:         sealedSnapshotEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Unit.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Snapshot.SealedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off sealed snapshot: %w", err)
	}
	return nil
}
