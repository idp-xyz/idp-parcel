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
type operatorRegistrationCompleted struct {
	tenant domain.TenantID
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
type OperatorRegistrationCompletedConsumer struct {
	transactor bentoapp.Transactor
	store      *inbox.Store
	queue      ports.OperatorRegistrationQueue
	advance    inboxconsume.Handler[psapplication.AdvanceAcceptanceChainCommand]
	pageSize   int
}

// NewOperatorRegistrationCompletedConsumer 装配本门。pageSize 是每次向队列读口要多少行；读口
// 要求正数，本门按页反复取直到没有新的委托可驱（已驱过的不再驱，停住的那几份下一页仍会
// 出现，靠「本页有没有新面孔」终止）。
func NewOperatorRegistrationCompletedConsumer(
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
		Name:       operatorRegistrationResumeConsumerName,
		EventType:  OperatorRegistrationCompletedEventType,
		Decode:     decodeOperatorRegistrationCompleted,
		Handle: func(_ context.Context, registration operatorRegistrationCompleted) error {
			return consumer.redriveWaitingRequests(ctx, registration.tenant)
		},
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "advance acceptance chain on operator registration",
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
func (consumer *OperatorRegistrationCompletedConsumer) redriveWaitingRequests(
	ctx context.Context,
	tenant domain.TenantID,
) error {
	attempted := map[domain.ShipmentRequestID]bool{}
	var (
		total, settled int
		undecided      []error
		failures       []error
	)
	for {
		records, err := consumer.queue.ListWaitingOnOperatorRegistration(ctx, tenant, consumer.pageSize)
		if err != nil {
			return fmt.Errorf("list requests waiting on operator registration: %w", err)
		}
		fresh := 0
		for _, record := range records {
			if attempted[record.ShipmentRequestID] {
				continue
			}
			attempted[record.ShipmentRequestID] = true
			fresh++
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
		// 一页里没有新面孔即到底：停住的那几份下一页仍会排在前面，只按它们判会无限翻页；
		// 不足一页也到底。
		if fresh == 0 || len(records) < consumer.pageSize {
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
