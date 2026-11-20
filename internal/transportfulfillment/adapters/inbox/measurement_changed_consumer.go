package tfinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// RePrepareOnMeasurementChangeConsumerName 是本消费者在 inbox 键上的稳定名。同一封实测变化还投给路由复核与结算重评，
// inbox 键按（消费者名 + 来源 + 事件 ID）认领，共名会把对方的投递当重复跳过。
const RePrepareOnMeasurementChangeConsumerName = "transport-fulfillment/re-prepare-on-measurement-change"

// ActualMeasurementChangedEventType 与节点作业交出的那一串同字面。串在本包写出，不导入提供方的常量。
const ActualMeasurementChangedEventType eventing.EventType = "node-operations.actual-measurement.changed"

// ChangedMeasurement 是译码后的一次测量变化。登记与作废都是这个类型。本路按此刻的当前有效实测再准备，不读信封上的
// 业务时间：比窗口用的是那条实测自己的发生时刻。
type ChangedMeasurement struct {
	TenantID   string
	SourceID   string
	ObjectKind string
	ObjectID   string
}

// ChangedMeasurementHandler 是本消费者转交的处理方。真实装配接 adapters/nodeoperations 的 RePrepareOnMeasurementChange。
type ChangedMeasurementHandler interface {
	HandleChangedMeasurement(ctx context.Context, changed ChangedMeasurement) error
}

// MeasurementChangedConsumer 把实际测量变化交给运输准备再判一份。
type MeasurementChangedConsumer struct {
	gate *inboxconsume.Gate[ChangedMeasurement]
}

func NewMeasurementChangedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler ChangedMeasurementHandler,
) (*MeasurementChangedConsumer, error) {
	if transactor == nil || store == nil || handler == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: measurement changed consumer is missing a dependency")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[ChangedMeasurement]{
		Transactor:     transactor,
		Store:          store,
		Name:           RePrepareOnMeasurementChangeConsumerName,
		EventType:      ActualMeasurementChangedEventType,
		Decode:         decodeChangedMeasurement,
		Handle:         handler.HandleChangedMeasurement,
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "re-prepare transport opportunity on measurement change",
	})
	if err != nil {
		return nil, err
	}
	return &MeasurementChangedConsumer{gate: gate}, nil
}

func (consumer *MeasurementChangedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeChangedMeasurement 译载荷。四个字段缺一即毒信封：重投同样内容不会长出字段。
func decodeChangedMeasurement(payload []byte) (ChangedMeasurement, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		SourceID   string `json:"sourceId"`
		ObjectKind string `json:"objectKind"`
		ObjectID   string `json:"objectId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ChangedMeasurement{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" || body.ObjectKind == "" || body.ObjectID == "" {
		return ChangedMeasurement{}, fmt.Errorf("%w: missing measurement change fields", ErrPoisonEnvelope)
	}
	return ChangedMeasurement{
		TenantID: body.TenantID, SourceID: body.SourceID, ObjectKind: body.ObjectKind, ObjectID: body.ObjectID,
	}, nil
}
