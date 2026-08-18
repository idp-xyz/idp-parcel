// Package inboxconsume 拥有 inbox 消费门的事务舞步：事件类型校验、Start、重复跳过、
// 毒丸拒收、处理失败整体回滚、成功 MarkProcessed。它对载荷类型与事件字符串一无所知，
// 第三个同形消费者出现后按 rule-of-three 从 NR 两例抽到这里（对照 outboxintent）。
package inboxconsume

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
)

// PoisonFailureCode 是毒丸拒收落账的失败码。形状跟框架先例一致（小写点分）。
const PoisonFailureCode = "inbox.poison_envelope"

// Decoder 把信封载荷译成处理方要的值。返回错误即毒丸：重投同样内容不会改变结果。
type Decoder[T any] func(payload []byte) (T, error)

// Handler 在消费门已经占住的同一事务里处理译码结果。返回错误让整笔事务回滚，inbox
// 无痕，重投可以再试。
type Handler[T any] func(ctx context.Context, decoded T) error

// Spec 装配一扇消费门。Name / EventType / Decode / Handle 都由调用方注入——平台层
// 不写任何上下文的消费者名或事件类型字符串。
type Spec[T any] struct {
	Transactor bentoapp.Transactor
	Store      *inbox.Store
	Name       string
	EventType  eventing.EventType
	Decode     Decoder[T]
	Handle     Handler[T]
	// UnexpectedType 是认不得的类型那句错误的前缀（「network routing inbox」）。
	// 空则用本包默认前缀。
	UnexpectedType string
	// HandleVerb 包住处理方错误（「handle accepted decision」）。空则用 "handle"。
	HandleVerb string
}

// Gate 是一扇消费门。调用方（各上下文的 inbox 适配器）持有它，对外仍导出自己的
// Consume，以保住既有构造器与错误前缀。
type Gate[T any] struct {
	transactor     bentoapp.Transactor
	store          *inbox.Store
	name           string
	eventType      eventing.EventType
	decode         Decoder[T]
	handle         Handler[T]
	unexpectedType string
	handleVerb     string
}

func New[T any](spec Spec[T]) (*Gate[T], error) {
	if spec.Transactor == nil {
		return nil, fmt.Errorf("inbox consume: transactor is nil")
	}
	if spec.Store == nil {
		return nil, fmt.Errorf("inbox consume: inbox store is nil")
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("inbox consume: consumer name is empty")
	}
	if spec.EventType == "" {
		return nil, fmt.Errorf("inbox consume: event type is empty")
	}
	if spec.Decode == nil {
		return nil, fmt.Errorf("inbox consume: decoder is nil")
	}
	if spec.Handle == nil {
		return nil, fmt.Errorf("inbox consume: handler is nil")
	}
	unexpected := spec.UnexpectedType
	if unexpected == "" {
		unexpected = "inbox consume"
	}
	verb := spec.HandleVerb
	if verb == "" {
		verb = "handle"
	}
	return &Gate[T]{
		transactor:     spec.Transactor,
		store:          spec.Store,
		name:           spec.Name,
		eventType:      spec.EventType,
		decode:         spec.Decode,
		handle:         spec.Handle,
		unexpectedType: unexpected,
		handleVerb:     verb,
	}, nil
}

// Consume 处理一份投递。顺序与原先两例逐字相同：先认类型（认不得响亮报错、不入账），
// 再译码（错误先记下，进事务后才拒收），然后同一事务里 Start → 重复跳过 / 毒丸拒收 /
// 处理 / MarkProcessed。处理失败返回错误，事务回滚。
func (gate *Gate[T]) Consume(ctx context.Context, envelope eventing.Envelope) error {
	if envelope.Type != gate.eventType {
		return fmt.Errorf("%s: unexpected event type %q", gate.unexpectedType, envelope.Type)
	}

	decoded, decodeErr := gate.decode(envelope.Payload)

	return gate.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		started, err := gate.store.Start(txCtx, eventing.InboxKey{
			Consumer: eventing.InboxConsumer(gate.name),
			Source:   envelope.Source,
			EventID:  envelope.ID,
		}, envelope.RecordedAt)
		if err != nil {
			return fmt.Errorf("start inbox entry: %w", err)
		}
		if !started.Acquired {
			return nil
		}

		if decodeErr != nil {
			if err := gate.store.MarkRejected(
				txCtx, started.Receipt, envelope.RecordedAt, PoisonFailureCode); err != nil {
				return fmt.Errorf("mark rejected: %w", err)
			}
			return nil
		}

		if err := gate.handle(txCtx, decoded); err != nil {
			return fmt.Errorf("%s: %w", gate.handleVerb, err)
		}
		if err := gate.store.MarkProcessed(txCtx, started.Receipt, envelope.RecordedAt); err != nil {
			return fmt.Errorf("mark processed: %w", err)
		}
		return nil
	})
}
