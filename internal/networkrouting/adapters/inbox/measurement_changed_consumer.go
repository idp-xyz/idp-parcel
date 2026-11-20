package nrinbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// ReassessOnMeasurementChangeConsumerName 是本消费者在 inbox 键上的稳定名。同一封实测变化还投给结算重评，
// inbox 键按（消费者名 + 来源 + 事件 ID）认领，共名会把对方的投递当重复跳过。
const ReassessOnMeasurementChangeConsumerName = "network-routing/reassess-on-measurement-change"

// ActualMeasurementChangedEventType 与节点作业交出的那一串同字面。串在本包写出，不导入提供方的常量。
const ActualMeasurementChangedEventType eventing.EventType = "node-operations.actual-measurement.changed"

// ChangedActualMeasurement 是译码后的一次测量变化。登记与作废都是这个类型，各有自己的 sourceId。
// OccurredAt 取信封上的业务时间，不在载荷里。
type ChangedActualMeasurement struct {
	TenantID   string
	SourceID   string
	ObjectKind string
	ObjectID   string
	OccurredAt time.Time
}

// ChangedActualMeasurementHandler 是本消费者转交的处理方。
type ChangedActualMeasurementHandler interface {
	HandleChangedActualMeasurement(ctx context.Context, changed ChangedActualMeasurement) error
}

// MeasurementChangedConsumer 把实际测量变化交给路由复核。结算那一路另记自己的 inbox 账。
type MeasurementChangedConsumer struct {
	gate *inboxconsume.Gate[ChangedActualMeasurement]
}

func NewMeasurementChangedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler ChangedActualMeasurementHandler,
) (*MeasurementChangedConsumer, error) {
	if transactor == nil || store == nil || handler == nil {
		return nil, fmt.Errorf("network routing inbox: measurement changed consumer is missing a dependency")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[ChangedActualMeasurement]{
		Transactor:     transactor,
		Store:          store,
		Name:           ReassessOnMeasurementChangeConsumerName,
		EventType:      ActualMeasurementChangedEventType,
		DecodeEnvelope: decodeChangedActualMeasurement,
		Handle:         handler.HandleChangedActualMeasurement,
		UnexpectedType: "network routing inbox",
		HandleVerb:     "handle changed measurement",
	})
	if err != nil {
		return nil, err
	}
	return &MeasurementChangedConsumer{gate: gate}, nil
}

func (consumer *MeasurementChangedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeChangedActualMeasurement 译载荷并取信封的业务时间。载荷缺字段，或信封没有业务时间，都是毒信封：
// 重投同样内容不会长出字段，也不会补上 OccurredAt。
func decodeChangedActualMeasurement(envelope eventing.Envelope) (ChangedActualMeasurement, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		SourceID   string `json:"sourceId"`
		ObjectKind string `json:"objectKind"`
		ObjectID   string `json:"objectId"`
	}
	if err := json.Unmarshal(envelope.Payload, &body); err != nil {
		return ChangedActualMeasurement{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" || body.ObjectKind == "" || body.ObjectID == "" {
		return ChangedActualMeasurement{}, fmt.Errorf("%w: missing measurement change fields", ErrPoisonEnvelope)
	}
	if envelope.OccurredAt.IsZero() {
		return ChangedActualMeasurement{}, fmt.Errorf("%w: measurement change has no business time", ErrPoisonEnvelope)
	}
	return ChangedActualMeasurement{
		TenantID:   body.TenantID,
		SourceID:   body.SourceID,
		ObjectKind: body.ObjectKind,
		ObjectID:   body.ObjectID,
		OccurredAt: envelope.OccurredAt,
	}, nil
}
