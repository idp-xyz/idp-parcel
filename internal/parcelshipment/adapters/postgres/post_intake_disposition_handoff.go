package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// postIntakeDispositionEventType 是「收寄后服务处置决定已形成」意图的事件类型（ADR-0336 决定四）。消费侧（运输履约等对象所有者）各写各的字面，导入本常量等于让消费方
// 依赖提供方的内部形状。
const postIntakeDispositionEventType = "parcel-shipment.post-intake-disposition.decided"

// OutboxPostIntakeDispositionHandoff 把收寄后服务处置决定写入 Outbox，实现 ports.PostIntakeDispositionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxPostIntakeDispositionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxPostIntakeDispositionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxPostIntakeDispositionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxPostIntakeDispositionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.PostIntakeDispositionHandoff = (*OutboxPostIntakeDispositionHandoff)(nil)

// postIntakeDispositionPayload 是信封载荷（版本 1）：决定的包裹与责任范围、去向、对各所有者的范围化请求、与原旅程的关系，以及运输履约另起旅程时作为决定依据的引用。
// 决定人与依据正文不进信封——它们是运营留痕，不是所有者承接要读的东西，也避免把自由文本扩散出去；留痕在决定表里。
type postIntakeDispositionPayload struct {
	TenantID            string                            `json:"tenantId"`
	Parcel              string                            `json:"parcel"`
	DecisionVersion     int                               `json:"decisionVersion"`
	SupersedesVersion   int                               `json:"supersedesVersion,omitempty"`
	Direction           string                            `json:"direction"`
	Source              string                            `json:"source"`
	RequestKey          string                            `json:"requestKey,omitempty"`
	Destination         string                            `json:"destination,omitempty"`
	ResponsibilityStart string                            `json:"responsibilityStart"`
	OriginalJourney     string                            `json:"originalJourney"`
	BasisKind           string                            `json:"basisKind"`
	BasisReference      string                            `json:"basisReference"`
	Requests            []postIntakeDispositionOwnerAsked `json:"requests"`
	DecidedAt           string                            `json:"decidedAt"`
}

type postIntakeDispositionOwnerAsked struct {
	Owner  string `json:"owner"`
	Action string `json:"action"`
}

const postIntakeDispositionEventIDPort = "post-intake-disposition"

// HandOffPostIntakeDisposition 把一份决定入队。信封 ID 由（租户，包裹，决定版本）认领（ADR-0043）：一个版本至多一封，重放重发同一份；同一包裹链上的下一版是另一封信，
// 不被当成重放吞掉。意图缺项是装配缺陷，响亮报错不入队。
func (handoff *OutboxPostIntakeDispositionHandoff) HandOffPostIntakeDisposition(
	ctx context.Context,
	intent ports.PostIntakeDispositionHandoffIntent,
) error {
	disposition := intent.Disposition
	if disposition.Tenant().String() == "" || disposition.Parcel().String() == "" ||
		disposition.Direction().String() == "" || disposition.Version() < 1 {
		return fmt.Errorf("hand off post-intake disposition: a formed disposition is required")
	}
	requests := make([]postIntakeDispositionOwnerAsked, 0, len(disposition.Direction().OwnerRequests()))
	for _, request := range disposition.Direction().OwnerRequests() {
		requests = append(requests, postIntakeDispositionOwnerAsked{
			Owner:  request.Owner.String(),
			Action: request.Action.String(),
		})
	}
	payload, err := json.Marshal(postIntakeDispositionPayload{
		TenantID:            disposition.Tenant().String(),
		Parcel:              disposition.Parcel().String(),
		DecisionVersion:     disposition.Version(),
		SupersedesVersion:   disposition.Supersedes(),
		Direction:           disposition.Direction().String(),
		Source:              disposition.Source().String(),
		RequestKey:          disposition.RequestKey().String(),
		Destination:         disposition.Destination().String(),
		ResponsibilityStart: disposition.ResponsibilityStart().String(),
		OriginalJourney:     disposition.Direction().OriginalJourneyRelation().String(),
		BasisKind:           domain.ServiceDispositionBasisKind,
		BasisReference:      disposition.BasisReference(),
		Requests:            requests,
		DecidedAt:           disposition.DecidedAt().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("hand off post-intake disposition: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: eventing.EventID(outboxintent.FingerprintEventID(postIntakeDispositionEventIDPort,
			disposition.Tenant().String(), disposition.Parcel().String(), strconv.Itoa(disposition.Version()))),
		Source:  eventSource,
		Type:    postIntakeDispositionEventType,
		Version: 1,
		Scope:   disposition.Tenant().String(),
		Subject: disposition.Parcel().String(),
		// 分区取（租户，包裹）加口名段：同一包裹的决定链按版本先后一条队，后一份取代前一份的次序所有者看得见；口名段让它不与同包裹的取消、采用、终局信封同队——它们的
		// 下游未决不该堵住一份扣住包裹的请求。
		PartitionKey: disposition.Tenant().String() + "/" + disposition.Parcel().String() + "/post-intake-disposition",
		// 领域发生时间是决定成立的时刻，不是入队时刻。
		OccurredAt:  disposition.DecidedAt().UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off post-intake disposition: %w", err)
	}
	return nil
}
