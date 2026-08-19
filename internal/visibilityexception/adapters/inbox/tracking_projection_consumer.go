package veinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// trackingProjectionConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由
// （消费者名 + 来源 + 事件 ID）认领，与各源上下文消费账分开；改名等于换消费者。
const trackingProjectionConsumerName = "visibility-exception/derive-customer-view-from-projection"

// TrackingProjectionDerivedEventType 是本消费者认的事件类型：VE 自己的追踪投影版本
// 派生。消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量——同一
// 上下文的两个适配器也不例外，共享常量会把出账口和消费口铆死在一次部署里。
//
// 载荷只有（租户+包裹+投影版本）三维引用，投影本体由处理方按（租户+包裹）读回当前
// 版（UC-VE-008：客户视图只基于当前投影形成）。
const TrackingProjectionDerivedEventType eventing.EventType = "visibility-exception.tracking-projection.derived"

// DerivedTrackingProjection 是译码后的投影版本引用——只有引用，投影本体由处理方
// 按引用重新取（权威事实在本上下文的投影库）。
type DerivedTrackingProjection struct {
	TenantID  string
	Parcel    string
	VersionID string
}

// DerivedTrackingProjectionHandler 是本消费者转交的处理方。真实装配接
// veconsume.DeriveCustomerViewOnProjectionAdapter。
type DerivedTrackingProjectionHandler interface {
	HandleDerivedTrackingProjection(ctx context.Context, derived DerivedTrackingProjection) error
}

// TrackingProjectionConsumer 把投影派生信封推进消费门。
type TrackingProjectionConsumer struct {
	gate *inboxconsume.Gate[DerivedTrackingProjection]
}

func NewTrackingProjectionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler DerivedTrackingProjectionHandler,
) (*TrackingProjectionConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[DerivedTrackingProjection]{
		Transactor:     transactor,
		Store:          store,
		Name:           trackingProjectionConsumerName,
		EventType:      TrackingProjectionDerivedEventType,
		Decode:         decodeDerivedTrackingProjection,
		Handle:         handler.HandleDerivedTrackingProjection,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle derived tracking projection",
	})
	if err != nil {
		return nil, err
	}
	return &TrackingProjectionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *TrackingProjectionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeDerivedTrackingProjection 译载荷。三维缺一即毒丸——处理方按（租户+包裹）读
// 回投影、按版本判新旧，缺了永远读不着也判不了，而重投同样内容不会长出字段来。
func decodeDerivedTrackingProjection(payload []byte) (DerivedTrackingProjection, error) {
	var body struct {
		TenantID  string `json:"tenantId"`
		Parcel    string `json:"parcel"`
		VersionID string `json:"versionId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return DerivedTrackingProjection{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.VersionID == "" {
		return DerivedTrackingProjection{}, fmt.Errorf("%w: missing projection reference fields", ErrPoisonEnvelope)
	}
	return DerivedTrackingProjection{
		TenantID:  body.TenantID,
		Parcel:    body.Parcel,
		VersionID: body.VersionID,
	}, nil
}
