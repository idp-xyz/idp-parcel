package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// cancellationNotUpheldEventType 是「取消被认定不成立」意图的事件类型（ADR-0295 决定四）。与消费侧（adapters/inbox）各写各的字面，理由同「释放拒收
// 已处理」那对：两边在不同包，导入对方的常量等于让消费方依赖提供方的内部形状。两串对不上由 cmd/parcel-dispatch 的真链往返用例当场揪红。
const cancellationNotUpheldEventType = "parcel-shipment.parcel-cancellation.not-upheld"

// OutboxCancellationNotUpheldHandoff 把「取消被认定不成立」写入 Outbox，实现 ports.CancellationNotUpheldHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCancellationNotUpheldHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCancellationNotUpheldHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCancellationNotUpheldHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxCancellationNotUpheldHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CancellationNotUpheldHandoff = (*OutboxCancellationNotUpheldHandoff)(nil)

// cancellationNotUpheldPayload 指名哪份取消、哪份冲突：续办要的是把这份冲突挡住的来源重驱一遍。决定人与依据不进信封（留痕在决定表里）。
type cancellationNotUpheldPayload struct {
	TenantID     string `json:"tenantId"`
	Parcel       string `json:"parcel"`
	Cancellation string `json:"cancellation"`
	BoundaryKind string `json:"boundaryKind"`
	BoundaryRef  string `json:"boundaryRef"`
}

const cancellationNotUpheldEventIDPort = "cancellation-not-upheld"

// HandOffCancellationNotUpheld 把一份意图入队。信封 ID 由冲突的身份认领（ADR-0043）：一份冲突至多被决定一次，重放因此重发同一份；同一包裹上
// 另一份冲突的决定是另一封信，不被当成重放吞掉。意图不是「认定取消不成立」的决定，或身份缺项，都是装配缺陷，响亮报错不入队。
func (handoff *OutboxCancellationNotUpheldHandoff) HandOffCancellationNotUpheld(
	ctx context.Context,
	intent ports.CancellationNotUpheldHandoffIntent,
) error {
	decision := intent.Decision
	key := decision.Key
	if key.TenantID.String() == "" || key.Parcel.String() == "" || key.BoundaryKind.String() == "" ||
		key.BoundaryRef.String() == "" || key.Cancellation.String() == "" {
		return fmt.Errorf("hand off cancellation not upheld: the conflict key is required")
	}
	if decision.Kind.String() != "CANCELLATION_NOT_UPHELD" {
		return fmt.Errorf("hand off cancellation not upheld: only a not-upheld decision resumes anything, got %q", decision.Kind)
	}
	payload, err := json.Marshal(cancellationNotUpheldPayload{
		TenantID:     key.TenantID.String(),
		Parcel:       key.Parcel.String(),
		Cancellation: key.Cancellation.String(),
		BoundaryKind: key.BoundaryKind.String(),
		BoundaryRef:  key.BoundaryRef.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off cancellation not upheld: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: eventing.EventID(outboxintent.FingerprintEventID(cancellationNotUpheldEventIDPort,
			key.TenantID.String(), key.Parcel.String(), key.BoundaryKind.String(), key.BoundaryRef.String(), key.Cancellation.String())),
		Source:  eventSource,
		Type:    cancellationNotUpheldEventType,
		Version: 1,
		Scope:   key.TenantID.String(),
		Subject: key.Parcel.String(),
		// 分区按冲突的全部维：一份冲突一条队，不与同包裹的取消、采用、终局信封排队。续办读的是库里已落定的状态（被挡住的来源、作废后的终局账），不依赖队里在前的
		// 信封；若与那些信封同队，同包裹上一封因下游未决而停在队头的信封（例如终局不采用意图停在下游消费的未决上）会把续办堵在后面，而续办正是要放行被它们
		// 之一挡住的来源。多份冲突的续办并发时由包裹取消边界锁排队。
		PartitionKey: key.TenantID.String() + "/" + key.Parcel.String() + "/" + key.Cancellation.String() + "/" +
			key.BoundaryKind.String() + "/" + key.BoundaryRef.String(),
		// 领域发生时间是决定成立的时刻，不是入队时刻。
		OccurredAt:  decision.DecidedAt.UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off cancellation not upheld: %w", err)
	}
	return nil
}
