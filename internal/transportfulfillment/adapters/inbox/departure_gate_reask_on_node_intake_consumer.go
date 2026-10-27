package tfinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是随袋走过出发的成员后来才识别成功时的出发门禁补问这道门（ADR-0331）：节点作业的节点收寄形成信封到了，替那件新认出的包裹把
// 它随袋走过的出发补问一遍。没有跨上下文翻译，处理方直接是应用编排。
//
// 门每次现建，理由同交接与揽收上的补问门（departure_gate_reask_consumers.go）：补问那一拍要先在任何写事务之外问关务，处理函数因此要拿到
// 进消费门事务之前的 ctx。

// departureGateReaskOnNodeIntakeConsumerName 是本门在 inbox 键上的稳定名。必须与同一封信上的其余各路（VE 投影、PS 来源采用、VE
// 移动事实补派生）分家：同一封信扇出给几路，inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。
// 改名等于换消费者。
const departureGateReaskOnNodeIntakeConsumerName = "transport-fulfillment/reask-departure-gates-on-node-intake"

// NodeIntakeFormedEventType 是本消费者认的事件类型：node-operations 的节点收寄形成。串在本包写出，理由同 OffsitePickupRegisteredEventType；
// 它与提供方交出的、与全程追踪与小包托运各路认的是同一个串，相等由 cmd/parcel-dispatch 用生产交接口入队的真库用例钉住。
const NodeIntakeFormedEventType eventing.EventType = "node-operations.node-intake.formed"

// FormedNodeIntake 是译码后的收寄键引用——只有引用，收寄本体由处理方按引用重新取。
type FormedNodeIntake struct {
	TenantID string
	SourceID string
}

// DepartureGateMemberReasker 是本门转交的那一口，真实装配接 application.ReaskDepartureGatesOnIdentifiedMemberHandler。
type DepartureGateMemberReasker interface {
	Plan(ctx context.Context, command application.ReaskOnNodeIntakeCommand) (application.DepartureGateReaskResult, error)
	Record(ctx context.Context, planned application.DepartureGateReaskResult) (application.DepartureGateReaskResult, error)
}

// DepartureGateReaskOnNodeIntakeConsumer 把节点收寄形成信封推进成员补问这道门。
type DepartureGateReaskOnNodeIntakeConsumer struct {
	transactor bentoapp.Transactor
	store      *inbox.Store
	reasker    DepartureGateMemberReasker
}

func NewDepartureGateReaskOnNodeIntakeConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	reasker DepartureGateMemberReasker,
) (*DepartureGateReaskOnNodeIntakeConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if reasker == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: departure gate member reasker is nil")
	}
	return &DepartureGateReaskOnNodeIntakeConsumer{transactor: transactor, store: store, reasker: reasker}, nil
}

// Consume 处理一份投递。门每次现建，理由在本文件头注。
func (consumer *DepartureGateReaskOnNodeIntakeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedNodeIntake]{
		Transactor: consumer.transactor,
		Store:      consumer.store,
		Name:       departureGateReaskOnNodeIntakeConsumerName,
		EventType:  NodeIntakeFormedEventType,
		Decode:     decodeFormedNodeIntake,
		Handle: func(txCtx context.Context, formed FormedNodeIntake) error {
			tenant, err := domain.NewTenantID(formed.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrDepartureGateReaskDefect, err)
			}
			planned, err := consumer.reasker.Plan(ctx, application.ReaskOnNodeIntakeCommand{TenantID: tenant, Source: formed.SourceID})
			if err != nil {
				return err
			}
			return recordDepartureGateReasks(txCtx, consumer.reasker, planned)
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "reask departure gates on identified member",
	})
	if err != nil {
		return err
	}
	return gate.Consume(ctx, envelope)
}

// decodeFormedNodeIntake 译载荷。tenantId 与 sourceId 缺一即毒丸：处理方按这两维取回收寄，缺了永远取不着。
func decodeFormedNodeIntake(payload []byte) (FormedNodeIntake, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		SourceID string `json:"sourceId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedNodeIntake{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" {
		return FormedNodeIntake{}, fmt.Errorf("%w: missing reception key fields", ErrPoisonEnvelope)
	}
	return FormedNodeIntake{TenantID: body.TenantID, SourceID: body.SourceID}, nil
}
