package psinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// operatorRegistrationResumeConsumerName 是「参数已登记」续办门在 inbox 键上的稳定名。与
// 提交门、复核续办门各占一个名：三扇门转交同一条链，但各收各的信，共名会让名册读起来像
// 同一扇（理由同 manualReviewResumeConsumerName）。
const operatorRegistrationResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-operator-registration"

// OperatorRegistrationCompletedEventType 是本门认的事件类型（ADR-0094 Decision 四的续办触发）。
// 发布侧是 party-commercial 的 Outbox 交接适配器，两边各写各的字面——跨上下文导入对方的常量
// 等于让消费方依赖提供方的内部形状。契约原文记在票 first-tenant-runway/07 的 Comments；两串
// 漂开撞 dispatch.no_subscriber，载荷字段漂开由 cmd/parcel-dispatch 那条用真 PC 适配器铸封的
// 往返用例揪红。
const OperatorRegistrationCompletedEventType eventing.EventType = "party-commercial.commercial-authority.operator-registration-completed"

// operatorRegistrationCompleted 是译出来的信封内容：只有租户。登记种类在载荷里但本门不按它
// 分派（ADR-0094 Decision 四原话「不在消费门认原因名字」）——一次登记解开的是该租户下所有停
// 在`等待运营登记`的委托，链自己会在重跑时发现哪一格仍未配置并再次停下。
//
// passOver 为真：这封信说的登记解不开任何一份等运营登记的委托（计价的 BUY 方向），本门照常入账、不问队列。
type operatorRegistrationCompleted struct {
	tenant   domain.TenantID
	passOver bool
}

// OperatorRegistrationCompletedConsumer 把「参数已登记」信封译成一轮重驱：按租户取回全部停在
// `等待运营登记`的`已提交`委托，逐份把同一条接受判断链再驱一拍（ADR-0094 Decision 四）。
//
// 它与另两扇门的差别在**一封信对多份委托**，而这决定了事务形状。inbox 那一行必须在同一笔
// 事务里从 PROCESSING 走到 PROCESSED（bento 的 Start 把一行已提交的 PROCESSING 当一致性失败），
// 而框架的嵌套事务没有保存点——一份委托的回滚会带翻同笔里其余委托已经推进的工作。因此本门
// 借 inboxconsume 的门只托住账本那一行，重驱本身在门外：每份委托各开一笔**顶层**事务，
// 决定形成或等待态入账即各自提交，停在会自愈的依赖上即各自回滚，一份不拖累其余。
//
// 整封信最后答什么由各份的结局汇总：有任一份停在回滚重投那一格，本门交回未决哨兵——账本
// 那一行随之回滚，派发器按重投节奏再送同一封，下一轮只会列出仍在等的那几份（已决的已不在
// 队列里，已入账等待的各自续办），于是这封信就是那几份的重投驱动，重投不重做已提交的工作。
// 全部落定即入账。装配或编程错误（集合外结果、缺件）不折进未决，响亮上抛让整封 publish_failed。
//
// 「参数已登记」类信封不止一个发布方：party-commercial 发声明通道那几类，settlement-accounting 发结算账户与受理前估价
// 采用那两类，parcel-pricing 发价卡、序列与目录那一类（NewPricingParameterRegisteredConsumer）。几封信重驱同一张队列，
// 所以是同一扇门的几份实例，各认各的类型、各记各的账本名——共名会让 inbox 键在几个来源之间互相吞没；载荷各按发布侧的
// 契约译，译法随实例给。
type OperatorRegistrationCompletedConsumer struct {
	name       string
	eventType  eventing.EventType
	verb       string
	decode     func([]byte) (operatorRegistrationCompleted, error)
	transactor bentoapp.Transactor
	store      *inbox.Store
	queue      ports.OperatorRegistrationQueue
	advance    inboxconsume.Handler[psapplication.AdvanceAcceptanceChainCommand]
	pageSize   int
}

// NewOperatorRegistrationCompletedConsumer 装配收 party-commercial「参数已登记」的那一份。pageSize 是每次
// 向队列读口要多少行；读口要求正数，本门按位置往后一页页取，取到不足一页为止。
func NewOperatorRegistrationCompletedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.OperatorRegistrationQueue,
	advancer AcceptanceChainAdvancer,
	pageSize int,
) (*OperatorRegistrationCompletedConsumer, error) {
	return newRegistrationResumeConsumer(
		operatorRegistrationResumeConsumerName,
		OperatorRegistrationCompletedEventType,
		"advance acceptance chain on operator registration",
		decodeOperatorRegistrationCompleted,
		transactor, store, queue, advancer, pageSize,
	)
}

func newRegistrationResumeConsumer(
	name string,
	eventType eventing.EventType,
	verb string,
	decode func([]byte) (operatorRegistrationCompleted, error),
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.OperatorRegistrationQueue,
	advancer AcceptanceChainAdvancer,
	pageSize int,
) (*OperatorRegistrationCompletedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if queue == nil {
		return nil, fmt.Errorf("parcel shipment inbox: operator registration queue is nil")
	}
	if advancer == nil {
		return nil, fmt.Errorf("parcel shipment inbox: acceptance chain advancer is nil")
	}
	if pageSize <= 0 {
		return nil, fmt.Errorf("parcel shipment inbox: operator registration page size must be positive, got %d", pageSize)
	}
	return &OperatorRegistrationCompletedConsumer{
		name:       name,
		eventType:  eventType,
		verb:       verb,
		decode:     decode,
		transactor: transactor,
		store:      store,
		queue:      queue,
		advance:    advanceAcceptanceChainThrough(advancer),
		pageSize:   pageSize,
	}, nil
}

// Consume 处理一份投递。账本舞步仍走 inboxconsume（类型校验、Start、重复跳过、毒丸拒收、
// MarkProcessed），但门每次现建：处理函数要拿到**进事务之前**的 ctx 去开各份委托自己的顶层
// 事务，而门只把事务内的 ctx 交给处理函数——事务绑定在 ctx 值里，拿它再开事务只会嵌套进
// 账本那一笔。门是无状态的薄壳，现建的代价只是一个结构体。
func (consumer *OperatorRegistrationCompletedConsumer) Consume(
	ctx context.Context,
	envelope eventing.Envelope,
) error {
	gate, err := inboxconsume.New(inboxconsume.Spec[operatorRegistrationCompleted]{
		Transactor: consumer.transactor,
		Store:      consumer.store,
		Name:       consumer.name,
		EventType:  consumer.eventType,
		Decode:     consumer.decode,
		Handle: func(_ context.Context, registration operatorRegistrationCompleted) error {
			if registration.passOver {
				return nil
			}
			return consumer.redriveWaitingRequests(ctx, registration.tenant)
		},
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     consumer.verb,
	})
	if err != nil {
		return err
	}
	return gate.Consume(ctx, envelope)
}

// redriveWaitingRequests 按页取回该租户停在`等待运营登记`的委托，逐份在各自的顶层事务里再驱
// 一拍。三类结局分开收：落定（决定形成或等待态入账，已提交）、停在回滚重投那一格（已回滚，
// 记下等本封重投）、硬错误（集合外结果、装配缺件，记下并继续驱其余）。硬错误压过未决上抛
// ——它要人改代码或装配，折进未决会重投到预算耗尽。
//
// 翻页按位置往后取（ports.OperatorRegistrationQueuePosition 写了为什么不每页重读队首）：被重驱又停回等运营登记的委托
// 位置不变，留在已翻过的那一段，不会再挤占后面的页。
func (consumer *OperatorRegistrationCompletedConsumer) redriveWaitingRequests(
	ctx context.Context,
	tenant domain.TenantID,
) error {
	var (
		after          ports.OperatorRegistrationQueuePosition
		total, settled int
		undecided      []error
		failures       []error
	)
	for {
		records, err := consumer.queue.ListWaitingOnOperatorRegistration(ctx, tenant, after, consumer.pageSize)
		if err != nil {
			return fmt.Errorf("list requests waiting on operator registration: %w", err)
		}
		for _, record := range records {
			after = record.Position()
			total++

			err := consumer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				return consumer.advance(txCtx, psapplication.AdvanceAcceptanceChainCommand{
					Identity:          record.Identity,
					ShipmentRequestID: record.ShipmentRequestID,
					SubmissionVersion: record.SubmissionVersion,
					DeclaredParcelIDs: record.DeclaredParcelIDs,
				})
			})
			switch {
			case err == nil:
				settled++
			case errors.Is(err, ErrAcceptanceChainUndecided):
				undecided = append(undecided, fmt.Errorf("%s: %w", record.ShipmentRequestID, err))
			default:
				failures = append(failures, fmt.Errorf("%s: %w", record.ShipmentRequestID, err))
			}
		}
		if len(records) < consumer.pageSize {
			break
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d waiting requests failed to advance (%d settled, %d undecided); first: %w",
			len(failures), total, settled, len(undecided), failures[0])
	}
	if len(undecided) > 0 {
		// 哨兵留在链首供路由条目翻成失败码；正文带计数与首份的停站与原因（那半由
		// advanceAcceptanceChainThrough 写进各份的错误里）。
		return fmt.Errorf("%w: %d of %d waiting requests stayed undecided on retryable dependencies (%d settled); first: %v",
			ErrAcceptanceChainUndecided, len(undecided), total, settled, undecided[0])
	}
	return nil
}

// decodeOperatorRegistrationCompleted 译载荷：契约只有两键 tenantId 与 registrationKind。租户译成
// 领域标识，立不起来即毒丸；种类只核在场——它是发布侧契约的一半，缺了说明载荷形状漂了，重投
// 长不出字段来——但不解读取值：本门不按种类分派，认了名字就是在适配层重建一份原因映射。
func decodeOperatorRegistrationCompleted(payload []byte) (operatorRegistrationCompleted, error) {
	var body struct {
		TenantID         string `json:"tenantId"`
		RegistrationKind string `json:"registrationKind"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.RegistrationKind == "" {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: missing registration kind", ErrPoisonEnvelope)
	}
	return operatorRegistrationCompleted{tenant: tenant}, nil
}
