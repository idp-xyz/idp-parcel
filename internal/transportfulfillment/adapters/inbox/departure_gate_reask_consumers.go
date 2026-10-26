package tfinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是出发门禁补问的两扇门（ADR-0305）：本上下文的权威交接登记与对象级场外揽收登记各一扇，信封到了就替那件对象把该补问的
// 出发问一遍。没有跨上下文翻译，处理方直接是应用编排。
//
// 两扇门都每次现建：补问那一拍要先在任何写事务之外问关务（关务的判断在它自己的事务里记下、在调用方事务里即拒，ADR-0206 决定六），
// 处理函数因此要拿到进消费门事务之前的 ctx——先由消费门在事务里认领这封信（已处理就停，不白问一遍关务），再用事务外的 ctx 读段、
// 读事实、问关务，最后回到消费门那一笔里把问来的各份追加落册、各交一封信，与 inbox 的已处理同笔提交。先例同结算那几扇续办门。

var (
	// ErrDepartureGateReaskUndecided 表示补问停在一个读得到的依赖上（控制事实还读不回、段册或移动事实册或补问册此刻读不动、补问
	// 交不出去）。重投可能就过；生产装配把它登进 WithUndecidedSentinels。停在哪一口留在错误正文里。
	ErrDepartureGateReaskUndecided = errors.New("transport fulfillment inbox: departure gate reask is undecided")
	// ErrDepartureGateReaskDefect 表示某一行装不回、记录与键不符，或本上下文自己交出的键立不起来。重投不会变，禁止进
	// WithUndecidedSentinels（ADR-0029）：让它落发布失败，响给人看。
	ErrDepartureGateReaskDefect = errors.New("transport fulfillment inbox: departure gate reask hit a defect")
)

// 两扇门在 inbox 键上的稳定名。必须与同一封信上的其余各路分家：同一封信扇出给几路，inbox 键只由（消费者名 + 来源 + 事件 ID）
// 认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const (
	departureGateReaskOnHandoverConsumerName = "transport-fulfillment/reask-departure-gates-on-transport-handover"
	departureGateReaskOnPickupConsumerName   = "transport-fulfillment/reask-departure-gates-on-offsite-pickup"
)

// OffsitePickupRegisteredEventType 是对象级揽收登记信封的类型。串在本包写出，理由同 TransportHandoverRegisteredEventType；它与
// 提供方交出的、与全程追踪与小包托运各路认的是同一个串，相等由 cmd/parcel-dispatch 用生产交接口入队的真库用例钉住。
const OffsitePickupRegisteredEventType eventing.EventType = "transport-fulfillment.offsite-pickup.registered"

// RegisteredOffsitePickup 是译码后的揽收登记键，三维都是登记册的键维。
type RegisteredOffsitePickup struct {
	TenantID string
	Object   string
	Attempt  string
}

// DepartureGateReasker 是两扇门转交的那一口，真实装配接 application.ReaskDepartureGatesHandler。
type DepartureGateReasker interface {
	PlanOnHandover(ctx context.Context, command application.ReaskOnHandoverCommand) (application.DepartureGateReaskResult, error)
	PlanOnPickup(ctx context.Context, command application.ReaskOnPickupCommand) (application.DepartureGateReaskResult, error)
	Record(ctx context.Context, planned application.DepartureGateReaskResult) (application.DepartureGateReaskResult, error)
}

// DepartureGateReaskOnHandoverConsumer 把权威交接登记信封推进补问那一扇门。
type DepartureGateReaskOnHandoverConsumer struct {
	transactor bentoapp.Transactor
	store      *inbox.Store
	reasker    DepartureGateReasker
}

func NewDepartureGateReaskOnHandoverConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	reasker DepartureGateReasker,
) (*DepartureGateReaskOnHandoverConsumer, error) {
	if err := departureGateReaskParts(transactor, store, reasker); err != nil {
		return nil, err
	}
	return &DepartureGateReaskOnHandoverConsumer{transactor: transactor, store: store, reasker: reasker}, nil
}

// Consume 处理一份投递。门每次现建，理由在本文件头注。
func (consumer *DepartureGateReaskOnHandoverConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor: consumer.transactor,
		Store:      consumer.store,
		Name:       departureGateReaskOnHandoverConsumerName,
		EventType:  TransportHandoverRegisteredEventType,
		Decode:     decodeRegisteredTransportHandover,
		Handle: func(txCtx context.Context, registered RegisteredTransportHandover) error {
			tenant, err := domain.NewTenantID(registered.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrDepartureGateReaskDefect, err)
			}
			planned, err := consumer.reasker.PlanOnHandover(ctx, application.ReaskOnHandoverCommand{
				TenantID: tenant,
				Object:   registered.Object,
				Scope:    registered.Scope,
				Version:  registered.Version,
			})
			if err != nil {
				return err
			}
			return recordDepartureGateReasks(txCtx, consumer.reasker, planned)
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "reask departure gates on registered transport handover",
	})
	if err != nil {
		return err
	}
	return gate.Consume(ctx, envelope)
}

// DepartureGateReaskOnPickupConsumer 把对象级场外揽收登记信封推进补问那一扇门。
type DepartureGateReaskOnPickupConsumer struct {
	transactor bentoapp.Transactor
	store      *inbox.Store
	reasker    DepartureGateReasker
}

func NewDepartureGateReaskOnPickupConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	reasker DepartureGateReasker,
) (*DepartureGateReaskOnPickupConsumer, error) {
	if err := departureGateReaskParts(transactor, store, reasker); err != nil {
		return nil, err
	}
	return &DepartureGateReaskOnPickupConsumer{transactor: transactor, store: store, reasker: reasker}, nil
}

// Consume 处理一份投递。门每次现建，理由在本文件头注。
func (consumer *DepartureGateReaskOnPickupConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredOffsitePickup]{
		Transactor: consumer.transactor,
		Store:      consumer.store,
		Name:       departureGateReaskOnPickupConsumerName,
		EventType:  OffsitePickupRegisteredEventType,
		Decode:     decodeRegisteredOffsitePickup,
		Handle: func(txCtx context.Context, registered RegisteredOffsitePickup) error {
			tenant, err := domain.NewTenantID(registered.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrDepartureGateReaskDefect, err)
			}
			planned, err := consumer.reasker.PlanOnPickup(ctx, application.ReaskOnPickupCommand{
				TenantID: tenant,
				Object:   registered.Object,
				Attempt:  registered.Attempt,
			})
			if err != nil {
				return err
			}
			return recordDepartureGateReasks(txCtx, consumer.reasker, planned)
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "reask departure gates on registered offsite pickup",
	})
	if err != nil {
		return err
	}
	return gate.Consume(ctx, envelope)
}

func departureGateReaskParts(transactor bentoapp.Transactor, store *inbox.Store, reasker DepartureGateReasker) error {
	if transactor == nil {
		return fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if reasker == nil {
		return fmt.Errorf("transport fulfillment inbox: departure gate reasker is nil")
	}
	return nil
}

// recordDepartureGateReasks 在消费门那一笔里追加问来的各份，并按编排答出的处置落账。
func recordDepartureGateReasks(
	txCtx context.Context,
	reasker DepartureGateReasker,
	planned application.DepartureGateReaskResult,
) error {
	recorded, err := reasker.Record(txCtx, planned)
	if err != nil {
		return err
	}
	switch recorded.Disposition() {
	case application.DepartureGateReaskSettled:
		return nil
	case application.DepartureGateReaskRetry:
		return fmt.Errorf("%w: %s %s", ErrDepartureGateReaskUndecided, recorded.Outcome(), recorded.Stop())
	default:
		return fmt.Errorf("%w: %s %s", ErrDepartureGateReaskDefect, recorded.Outcome(), recorded.Stop())
	}
}

// decodeRegisteredOffsitePickup 译载荷。三维缺一即毒丸：处理方按（租户 + 对象 + 尝试）读回揽收，缺一维永远取不着。
func decodeRegisteredOffsitePickup(payload []byte) (RegisteredOffsitePickup, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Attempt  string `json:"attempt"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredOffsitePickup{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Attempt == "" {
		return RegisteredOffsitePickup{}, fmt.Errorf("%w: missing offsite pickup key fields", ErrPoisonEnvelope)
	}
	return RegisteredOffsitePickup{TenantID: body.TenantID, Object: body.Object, Attempt: body.Attempt}, nil
}
