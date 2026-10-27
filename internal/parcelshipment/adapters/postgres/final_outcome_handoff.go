package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

const finalOutcomeEventType = "parcel-shipment.final-outcome.formed"

// OutboxFinalOutcomeHandoff 把已提交的终局判断写入 Outbox，实现
// ports.FinalOutcomeHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxFinalOutcomeHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxFinalOutcomeHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxFinalOutcomeHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxFinalOutcomeHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.FinalOutcomeHandoff = (*OutboxFinalOutcomeHandoff)(nil)

type finalOutcomePayload struct {
	TenantID string `json:"tenantId"`
	Parcel   string `json:"parcel"`
	Kind     string `json:"kind"`
	Version  string `json:"version"`
}

// finalOutcomeEventIDPort 是本口指纹 ID 的口名前缀。
const finalOutcomeEventIDPort = "final-outcome"

// finalOutcomeEventID 铸本口的信封 ID：采用键四维，作废代次大于零时再加一维。代次为零（包裹上没有成立过「取消依据不成立」的
// 决定）时只有四维：只认采用键的信封按这个形式铸 ID，重放去重靠它逐字不变。
//
// 采用键只认（租户，包裹，结果种类，版本），不分这一行是不采用还是终局。依据一份取消的不采用行被决定作废后，续办用同一键重新
// 形成终局：若 ID 仍只由采用键认领，EnqueueOnce 按（来源，事件标识）查到不采用行那一封已在，重新形成的终局就没有信封，下游停在
// 「不采用」。作废代次让决定之前与之后的信封各有各的 ID。
func finalOutcomeEventID(key ports.FinalAdoptionKey, voidGeneration int) string {
	dimensions := []string{key.TenantID.String(), key.Parcel.String(), key.Kind.String(), key.Version.String()}
	if voidGeneration > 0 {
		dimensions = append(dimensions, strconv.Itoa(voidGeneration))
	}
	return string(outboxintent.FingerprintEventID(finalOutcomeEventIDPort, dimensions...))
}

// voidGeneration 数该包裹上已成立的「取消依据不成立」决定（ADR-0295 决定三）。决定只追加、不删，所以个数单调不减。
// 读在调用方事务里：形成终局的编排已持包裹取消边界锁，决定同笔作废也持同一把，所以这里读到的个数与此刻能读到的不采用行互相一致。
func (handoff *OutboxFinalOutcomeHandoff) voidGeneration(
	ctx context.Context,
	key ports.FinalAdoptionKey,
) (int, error) {
	executor, err := handoff.db.RequireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var generation int
	if err := executor.QueryRow(ctx,
		`SELECT count(*)
		   FROM parcel_shipment.cancellation_order_conflict_decision
		  WHERE tenant_id = $1
		    AND parcel_id = $2
		    AND decision_kind = 'CANCELLATION_NOT_UPHELD'`,
		key.TenantID.String(), key.Parcel.String(),
	).Scan(&generation); err != nil {
		return 0, err
	}
	return generation, nil
}

// HandOffFinalOutcome 把一份意图入队。信封 ID 是采用键加该包裹作废代次的指纹加口名前缀——意图由（租户+包裹+
// 责任结果种类+版本+作废代次）认领（ADR-0043、ADR-0295 决定三）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxFinalOutcomeHandoff) HandOffFinalOutcome(
	ctx context.Context,
	intent ports.FinalOutcomeHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Parcel.String() == "" ||
		key.Kind.String() == "" || key.Version.String() == "" {
		return fmt.Errorf("hand off final outcome: adoption key is required")
	}

	payload, err := json.Marshal(finalOutcomePayload{
		TenantID: key.TenantID.String(),
		Parcel:   key.Parcel.String(),
		Kind:     key.Kind.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off final outcome: %w", err)
	}

	generation, err := handoff.voidGeneration(ctx, key)
	if err != nil {
		return fmt.Errorf("hand off final outcome: read void generation: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := finalOutcomeEventID(key, generation)
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(eventID),
		Source:      eventSource,
		Type:        finalOutcomeEventType,
		Version:     1,
		Scope:       key.TenantID.String(),
		Subject:     key.Parcel.String(),
		// 分区按租户加包裹排队，不跟着信封 ID 走：ID 含版本（重派生翻旧插新各占一个 ID，
		// 因而不丢），而顺序要的是同一包裹的先后拍在一条队里——两者跟同一个字符串时，
		// 重派生会落进另一个分区，先于原终局送达时下游最后应用的是已被取代的那一份。
		PartitionKey: key.TenantID.String() + "/" + key.Parcel.String(),
		OccurredAt:   intent.Record.AdoptedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off final outcome: %w", err)
	}
	return nil
}
