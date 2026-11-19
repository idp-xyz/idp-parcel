package sainbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

const (
	// measurementChangedConsumerName 是这个消费者在收件箱里的身份，收件箱按它记哪些事件已经消费过。名字里的 sell 是历史
	// 叫法，如今销售与采购两路重评都由它转交的处理方办；它是收件箱身份，改了字面已经消费过的事件会被当成没消费过再办一遍，
	// 所以不改。
	measurementChangedConsumerName = "settlement-accounting/reevaluate-sell-on-measurement-change"
	// MeasurementChangedEventType 与节点作业交出的那一串同字面。
	MeasurementChangedEventType = "node-operations.actual-measurement.changed"
)

// ChangedMeasurement 是译码后的一次测量变化。金额不在信封上，处理方按对象回读。
type ChangedMeasurement struct {
	TenantID   string `json:"tenantId"`
	SourceID   string `json:"sourceId"`
	ObjectKind string `json:"objectKind"`
	ObjectID   string `json:"objectId"`
}

// ChangedMeasurementHandler 是本消费者转交的处理方。
type ChangedMeasurementHandler interface {
	HandleChangedMeasurement(ctx context.Context, changed ChangedMeasurement) error
}

// MeasurementChangedConsumer 把实际测量变化交给处理方推进重评：销售评价与采购评价各另起下一代由处理方分办，本消费者
// 只管幂等消费与译码。
type MeasurementChangedConsumer struct {
	gate *inboxconsume.Gate[ChangedMeasurement]
}

func NewMeasurementChangedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler ChangedMeasurementHandler,
) (*MeasurementChangedConsumer, error) {
	if transactor == nil || store == nil || handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: measurement changed consumer is missing a dependency")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[ChangedMeasurement]{
		Transactor:     transactor,
		Store:          store,
		Name:           measurementChangedConsumerName,
		EventType:      MeasurementChangedEventType,
		Decode:         decodeChangedMeasurement,
		Handle:         handler.HandleChangedMeasurement,
		UnexpectedType: "settlement accounting inbox",
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

func decodeChangedMeasurement(payload []byte) (ChangedMeasurement, error) {
	var body ChangedMeasurement
	if err := json.Unmarshal(payload, &body); err != nil {
		return ChangedMeasurement{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" || body.ObjectKind == "" || body.ObjectID == "" {
		return ChangedMeasurement{}, fmt.Errorf("%w: missing measurement change fields", ErrPoisonEnvelope)
	}
	return body, nil
}
