package psinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// cancellationNotUpheldConsumerName 是「取消被认定不成立」续办门在 inbox 键上的稳定名。每消费者一个名：同包第二个消费者若沿用同一个名，两路会
// 互相把对方的投递当重复跳过。
const cancellationNotUpheldConsumerName = "parcel-shipment/redrive-on-cancellation-not-upheld"

// CancellationNotUpheldEventType 是本门认的事件类型（ADR-0295 决定四的续办触发）。发布侧是本上下文的取消解开交接 Outbox 适配器，两边各写各的字面，
// 理由同「释放拒收已处理」那对；两串漂开撞 dispatch.no_subscriber，载荷字段漂开由 cmd/parcel-dispatch 的真链往返用例揪红。
const CancellationNotUpheldEventType eventing.EventType = "parcel-shipment.parcel-cancellation.not-upheld"

// NotUpheldCancellation 是译码后的续办引用：哪个租户、哪件包裹、哪份取消、冲突的边界那一代。被挡住的来源回放原料由处理方按这几维取回，不进信封。
type NotUpheldCancellation struct {
	TenantID     string
	Parcel       string
	Cancellation string
	BoundaryKind string
	BoundaryRef  string
}

// NotUpheldCancellationHandler 是本消费者转交的处理方。真实装配接 adapters/cancellationredrive 的 Redriver。
type NotUpheldCancellationHandler interface {
	HandleNotUpheldCancellation(ctx context.Context, notUpheld NotUpheldCancellation) error
}

// CancellationNotUpheldConsumer 把「取消被认定不成立」信封推进消费门，由处理方把被这份取消挡住的来源重驱一遍（ADR-0295 决定四）。
type CancellationNotUpheldConsumer struct {
	gate *inboxconsume.Gate[NotUpheldCancellation]
}

func NewCancellationNotUpheldConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler NotUpheldCancellationHandler,
) (*CancellationNotUpheldConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[NotUpheldCancellation]{
		Transactor:     transactor,
		Store:          store,
		Name:           cancellationNotUpheldConsumerName,
		EventType:      CancellationNotUpheldEventType,
		Decode:         decodeNotUpheldCancellation,
		Handle:         handler.HandleNotUpheldCancellation,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "redrive on cancellation not upheld",
	})
	if err != nil {
		return nil, err
	}
	return &CancellationNotUpheldConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与同包其余消费者共用同一扇门。
func (consumer *CancellationNotUpheldConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeNotUpheldCancellation 译载荷。五维缺一即毒丸——处理方按这几维取回原料与冲突，缺了永远取不着，而重投同样内容不会长出字段来。
func decodeNotUpheldCancellation(payload []byte) (NotUpheldCancellation, error) {
	var body struct {
		TenantID     string `json:"tenantId"`
		Parcel       string `json:"parcel"`
		Cancellation string `json:"cancellation"`
		BoundaryKind string `json:"boundaryKind"`
		BoundaryRef  string `json:"boundaryRef"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return NotUpheldCancellation{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Cancellation == "" || body.BoundaryKind == "" || body.BoundaryRef == "" {
		return NotUpheldCancellation{}, fmt.Errorf("%w: missing not-upheld cancellation fields", ErrPoisonEnvelope)
	}
	return NotUpheldCancellation{
		TenantID: body.TenantID, Parcel: body.Parcel, Cancellation: body.Cancellation,
		BoundaryKind: body.BoundaryKind, BoundaryRef: body.BoundaryRef,
	}, nil
}
