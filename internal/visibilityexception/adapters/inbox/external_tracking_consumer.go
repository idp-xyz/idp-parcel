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

// externalTrackingConsumerName 是本消费者在 inbox 键上的稳定名。与交付、交接、揽收三路分账：
// 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。
const externalTrackingConsumerName = "visibility-exception/derive-projection-from-external-carrier-tracking"

// ExternalCarrierTrackingJudgedEventType 是本消费者认的事件类型：TF 对一条外部承运轨迹事实
// **判断过有效时间**的那一版。类型词里的 judged 不是装饰——待判断的版本在 TF 侧就不入队
// （ADR-0102 决定三：不提供给 visibility-exception），本消费者因此永远不该收到一份待判断的引用。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
const ExternalCarrierTrackingJudgedEventType eventing.EventType = "transport-fulfillment.external-carrier-tracking.judged"

// JudgedExternalTracking 是译码后的引用——只有（租户+事实+版本）三维，事实本体由处理方按引用
// 重新取（权威事实留在 transport-fulfillment）。版本是引用的一维：源更正与本仓判断各换一版。
type JudgedExternalTracking struct {
	TenantID string
	Fact     string
	Version  string
}

// JudgedExternalTrackingHandler 是本消费者转交的处理方。真实装配接
// DeriveOnExternalCarrierTrackingAdapter。
type JudgedExternalTrackingHandler interface {
	HandleJudgedExternalTracking(ctx context.Context, judged JudgedExternalTracking) error
}

// ExternalTrackingConsumer 把 TF 外部承运轨迹事实信封推进消费门。
type ExternalTrackingConsumer struct {
	gate *inboxconsume.Gate[JudgedExternalTracking]
}

func NewExternalTrackingConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler JudgedExternalTrackingHandler,
) (*ExternalTrackingConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[JudgedExternalTracking]{
		Transactor:     transactor,
		Store:          store,
		Name:           externalTrackingConsumerName,
		EventType:      ExternalCarrierTrackingJudgedEventType,
		Decode:         decodeJudgedExternalTracking,
		Handle:         handler.HandleJudgedExternalTracking,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle judged external carrier tracking",
	})
	if err != nil {
		return nil, err
	}
	return &ExternalTrackingConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ExternalTrackingConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeJudgedExternalTracking 译载荷。三维缺一即毒丸——处理方按（租户+事实+版本）取回那一代，
// 缺了永远取不着，重投同样内容不会长出字段来。
func decodeJudgedExternalTracking(payload []byte) (JudgedExternalTracking, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return JudgedExternalTracking{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Fact == "" || body.Version == "" {
		return JudgedExternalTracking{}, fmt.Errorf("%w: missing external tracking reference fields", ErrPoisonEnvelope)
	}
	return JudgedExternalTracking{TenantID: body.TenantID, Fact: body.Fact, Version: body.Version}, nil
}
