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

const externalTrackingFactEventType = "transport-fulfillment.external-carrier-tracking.judged"

// OutboxExternalTrackingFactHandoff 把**有效时间已判断**的外部承运轨迹事实版本写入 Outbox，
// 实现 ports.ExternalTrackingFactHandoff。事件类型词里的 judged 是刻意的：待判断的版本没有可交
// 的东西，本适配器对它响亮拒绝——静默入队一份 VE 造不出 AcceptedSourceFact 的信封，比不入队糟。
type OutboxExternalTrackingFactHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxExternalTrackingFactHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxExternalTrackingFactHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxExternalTrackingFactHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ExternalTrackingFactHandoff = (*OutboxExternalTrackingFactHandoff)(nil)

// externalTrackingFactPayload 是指针式载荷：只带（租户+事实+版本）三维引用，事实本体由下游按
// 引用重新读。版本在里面——下游要读的是「哪一代」，而更正与判断都换版本。
type externalTrackingFactPayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
}

// externalTrackingFactEventID 取键的全部三维。版本必须在里面：ADR-0043 说意图由结果标识认领，
// 而一条事实的结果标识是它的版本——少了它，判断版本与被更正版本算出同一个字符串，
// outboxintent.EnqueueOnce 先查后插，第二份静默不入队。
func externalTrackingFactEventID(key ports.ExternalTrackingFactKey) string {
	return key.TenantID.String() + "/" + key.Fact.String() + "/" + key.Version.String() + "/external-carrier-tracking"
}

// externalTrackingFactPartitionKey 取（租户+载运对象+类型段），不取整个键。
//
// ID 管幂等、分区键管顺序。一个载运对象的外部轨迹是一条链（首次认领，此后源更正与判断各一版），
// 下游按替代关系重新派生投影——后一版先于前一版送达时 VE 的 CurrentlyEffective 照样答得对
// （它按在场集合派生），但同一对象的两条事实若分了区，客户可见面上就会先后颠倒地闪一次。
// 类型段按 ADR-0074：TF 的排队主体是载运对象，与 visibility-exception 的（租户+包裹）分区不共队。
func externalTrackingFactPartitionKey(record ports.ExternalTrackingFactRecord) string {
	return record.Key.TenantID.String() + "/" + record.Fact.Object().String() + "/external-carrier-tracking"
}

// HandOffExternalTrackingFact 把一份意图入队。键缺席或版本待判断是装配缺陷，响亮报错不入队。
func (handoff *OutboxExternalTrackingFactHandoff) HandOffExternalTrackingFact(
	ctx context.Context,
	intent ports.ExternalTrackingFactHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Fact.String() == "" || key.Version.String() == "" {
		return fmt.Errorf("hand off external tracking fact: fact key is required")
	}
	if !intent.Record.Fact.Effective().Judged() {
		return fmt.Errorf("hand off external tracking fact: effective time of %s/%s is still pending judgment",
			key.Fact.String(), key.Version.String())
	}
	if intent.Record.Fact.Object().String() == "" {
		return fmt.Errorf("hand off external tracking fact: carried object is required")
	}

	payload, err := json.Marshal(externalTrackingFactPayload{
		TenantID: key.TenantID.String(),
		Fact:     key.Fact.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off external tracking fact: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(externalTrackingFactEventID(key)),
		Source:       tfEventSource,
		Type:         externalTrackingFactEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Fact.String() + "/" + key.Version.String(),
		PartitionKey: externalTrackingFactPartitionKey(intent.Record),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off external tracking fact: %w", err)
	}
	return nil
}
