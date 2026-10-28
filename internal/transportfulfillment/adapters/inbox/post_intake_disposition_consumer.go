package tfinbox

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

// postIntakeDispositionConsumerName 是本消费者在 inbox 键上的稳定名。同一封处置信封日后还会扇出给节点运营与网络路由各自的承接，
// inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const postIntakeDispositionConsumerName = "transport-fulfillment/start-journey-on-post-intake-disposition"

// PostIntakeDispositionDecidedEventType 是本消费者认的事件类型：PS 收寄后服务处置决定已形成（ADR-0332 决定四）。字符串由消费方自己写出，
// 不 import 提供方（ADR-0025）；与提供方入队的字面相等由 cmd/parcel-dispatch 用生产交接口入队的真库用例钉住。
const PostIntakeDispositionDecidedEventType eventing.EventType = "parcel-shipment.post-intake-disposition.decided"

// PostIntakeDispositionOwnerRequest 是信封里向某个所有者提出的一项请求，两个字面都是 PS 的拼写。
type PostIntakeDispositionOwnerRequest struct {
	Owner  string
	Action string
}

// DecidedPostIntakeDisposition 是译码后的处置决定。决定人与依据正文不在信封里（ADR-0332 决定四），本上下文也不需要：依据引用已指名链上这一版。
type DecidedPostIntakeDisposition struct {
	TenantID        string
	Parcel          string
	DecisionVersion int
	Direction       string
	BasisKind       string
	BasisReference  string
	DecidedAt       time.Time
	Requests        []PostIntakeDispositionOwnerRequest
}

// PostIntakeDispositionHandler 是本消费者转交的处理方。真实装配接 adapters/parcelshipment 的 StartJourneyOnPostIntakeDisposition。
type PostIntakeDispositionHandler interface {
	HandleDecidedPostIntakeDisposition(ctx context.Context, decided DecidedPostIntakeDisposition) error
}

// PostIntakeDispositionConsumer 把处置决定信封推进消费门。
type PostIntakeDispositionConsumer struct {
	gate *inboxconsume.Gate[DecidedPostIntakeDisposition]
}

func NewPostIntakeDispositionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler PostIntakeDispositionHandler,
) (*PostIntakeDispositionConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[DecidedPostIntakeDisposition]{
		Transactor:     transactor,
		Store:          store,
		Name:           postIntakeDispositionConsumerName,
		EventType:      PostIntakeDispositionDecidedEventType,
		Decode:         decodeDecidedPostIntakeDisposition,
		Handle:         handler.HandleDecidedPostIntakeDisposition,
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "start journey on post-intake disposition",
	})
	if err != nil {
		return nil, err
	}
	return &PostIntakeDispositionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *PostIntakeDispositionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeDecidedPostIntakeDisposition 译载荷。键维与时刻缺一即毒丸：重投同样内容不会长出字段来。请求清单可以为空（继续不请求任何所有者）。
func decodeDecidedPostIntakeDisposition(payload []byte) (DecidedPostIntakeDisposition, error) {
	var body struct {
		TenantID        string `json:"tenantId"`
		Parcel          string `json:"parcel"`
		DecisionVersion int    `json:"decisionVersion"`
		Direction       string `json:"direction"`
		BasisKind       string `json:"basisKind"`
		BasisReference  string `json:"basisReference"`
		DecidedAt       string `json:"decidedAt"`
		Requests        []struct {
			Owner  string `json:"owner"`
			Action string `json:"action"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return DecidedPostIntakeDisposition{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.DecisionVersion < 1 || body.Direction == "" ||
		body.BasisKind == "" || body.BasisReference == "" {
		return DecidedPostIntakeDisposition{}, fmt.Errorf("%w: missing post-intake disposition key fields", ErrPoisonEnvelope)
	}
	decidedAt, err := time.Parse(time.RFC3339Nano, body.DecidedAt)
	if err != nil {
		return DecidedPostIntakeDisposition{}, fmt.Errorf("%w: decidedAt: %v", ErrPoisonEnvelope, err)
	}
	decided := DecidedPostIntakeDisposition{
		TenantID:        body.TenantID,
		Parcel:          body.Parcel,
		DecisionVersion: body.DecisionVersion,
		Direction:       body.Direction,
		BasisKind:       body.BasisKind,
		BasisReference:  body.BasisReference,
		DecidedAt:       decidedAt.UTC(),
	}
	for _, request := range body.Requests {
		decided.Requests = append(decided.Requests, PostIntakeDispositionOwnerRequest{Owner: request.Owner, Action: request.Action})
	}
	return decided, nil
}
