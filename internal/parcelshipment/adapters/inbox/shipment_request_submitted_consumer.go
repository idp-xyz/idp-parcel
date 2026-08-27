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
	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// acceptanceChainConsumerName 是本消费者在 inbox 键上的稳定名，与同包其余消费者各占
// 一个名（理由见 nodeIntakeConsumerName）。
const acceptanceChainConsumerName = "parcel-shipment/advance-acceptance-chain"

// ShipmentRequestSubmittedEventType 是本消费者认的事件类型。发布侧是本上下文自己的
// Outbox 交接适配器，但两边仍各写各的字符串——它们在不同包里，导入对方的未导出常量
// 等于让消费方依赖提供方的内部形状。
//
// 两串万一漂开不会静默：本进程的路由表按这个类型登记，发布侧发出的那一封会撞
// `dispatch.no_subscriber`（装配错误里最响的一格）；`cmd/parcel-dispatch` 的合成纵向
// 用例走的正是「真发布 → 真派发 → 真消费」，字符串对不上时它当场红。
const ShipmentRequestSubmittedEventType eventing.EventType = "parcel-shipment.shipment-request.submitted"

// ErrAcceptanceChainUndecided 是本路的未决哨兵：接受链收到并推进了这一轮，但停在自己
// 等的依赖上。它在路由条目处翻成 dispatch.ErrConsumerUndecided（不在这里翻，方向见
// dispatch.WithUndecidedSentinels 的注释）。
//
// 未决整笔回滚是消费门的既有语义，本路照此。代价要如实记下：推进途中记下的处理尝试
// 与已记录判断一并回滚，因此「未决按原因分类统计」不跨重投累计——重投会从同一份信封
// 重跑，结论不变，只是那几次尝试不留痕。
var ErrAcceptanceChainUndecided = errors.New("parcel shipment inbox: acceptance chain is undecided")

// ErrUnexpectedAcceptanceChainOutcome 表示编排交回了封闭集合以外的结果。它不进未决名单：
// 集合外的取值是装配或编程错误，重投同一份信封改不了它，而未决的处置正是回滚重投——
// 折进未决，这一封会一路重投到失败预算耗尽，日志上看起来像「一直在等某个依赖」。
var ErrUnexpectedAcceptanceChainOutcome = errors.New(
	"parcel shipment inbox: unexpected acceptance chain outcome")

// AcceptanceChainAdvancer 是本消费者转交的处理方。真实装配接
// psapplication.AdvanceAcceptanceChainHandler。
type AcceptanceChainAdvancer interface {
	Handle(
		ctx context.Context,
		command psapplication.AdvanceAcceptanceChainCommand,
	) (psapplication.AdvanceAcceptanceChainResult, error)
}

// ShipmentRequestSubmittedConsumer 把「委托已提交」信封推进接受判断链。
type ShipmentRequestSubmittedConsumer struct {
	gate *inboxconsume.Gate[psapplication.AdvanceAcceptanceChainCommand]
}

func NewShipmentRequestSubmittedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	advancer AcceptanceChainAdvancer,
) (*ShipmentRequestSubmittedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if advancer == nil {
		return nil, fmt.Errorf("parcel shipment inbox: acceptance chain advancer is nil")
	}
	consumer := &ShipmentRequestSubmittedConsumer{}
	gate, err := inboxconsume.New(inboxconsume.Spec[psapplication.AdvanceAcceptanceChainCommand]{
		Transactor: transactor,
		Store:      store,
		Name:       acceptanceChainConsumerName,
		EventType:  ShipmentRequestSubmittedEventType,
		Decode:     decodeSubmittedShipmentRequest,
		Handle:     consumer.advanceThrough(advancer),
		// 同包三个跨上下文消费者译到字符串再由处理适配器翻；本路的载荷是本上下文
		// 自己铸的，标识就在自家 ID 空间里，因此直接译成领域标识——「译不出标识」
		// 与「重投也长不出字段」是同一格，正是毒丸该判的那一格。
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "advance acceptance chain",
	})
	if err != nil {
		return nil, err
	}
	consumer.gate = gate
	return consumer, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与同包其余消费者共用同一扇门。
func (consumer *ShipmentRequestSubmittedConsumer) Consume(
	ctx context.Context,
	envelope eventing.Envelope,
) error {
	return consumer.gate.Consume(ctx, envelope)
}

// advanceThrough 把编排的三值结果折成消费门认的两格：形成决定即本份投递处理完毕，
// 未决即整笔回滚等重投。编排上抛的错误原样上抛——那是端口交回了集合外的答复或装配
// 缺件，重投改不了它，不该混进未决。
func (consumer *ShipmentRequestSubmittedConsumer) advanceThrough(
	advancer AcceptanceChainAdvancer,
) inboxconsume.Handler[psapplication.AdvanceAcceptanceChainCommand] {
	return func(ctx context.Context, command psapplication.AdvanceAcceptanceChainCommand) error {
		result, err := advancer.Handle(ctx, command)
		if err != nil {
			return err
		}
		switch result.Outcome() {
		case psapplication.AcceptanceChainDecided:
			return nil
		case psapplication.AcceptanceChainUndecided:
			// 停在哪一步与未决原因都写进错误正文：路由条目只把哨兵翻成失败码，
			// 原错误原样留在链上供运维读（dispatch.WithUndecidedSentinels 两个都用 %w）。
			return fmt.Errorf("%w: stage %s, reason %s",
				ErrAcceptanceChainUndecided, result.Stage(), result.PendingReason())
		default:
			// 不留 default 兜底成未决：静默重投等于替编排作判断。
			return fmt.Errorf("%w: %q", ErrUnexpectedAcceptanceChainOutcome, result.Outcome())
		}
	}
}

// decodeSubmittedShipmentRequest 译载荷。
//
// 只取接受链要的那几维：来源身份四维、委托、当前提交版本与声明成员。提交批次在载荷里
// 但本路不用——译进来会让人以为下游按批次做了什么。
//
// 任一维缺席或构造不出领域标识即毒丸：这些都是发布侧铸信封时就该齐的东西，重投同一份
// 内容不会长出字段来。成员清单为空同样是毒丸：`已提交`委托必有声明成员，空清单意味着
// 发布侧发错了，等下去也不会变。
func decodeSubmittedShipmentRequest(payload []byte) (psapplication.AdvanceAcceptanceChainCommand, error) {
	var body struct {
		TenantID            string   `json:"tenantId"`
		CustomerAccountID   string   `json:"customerAccountId"`
		Source              string   `json:"source"`
		SourceRequestKey    string   `json:"sourceRequestKey"`
		ShipmentRequestID   string   `json:"shipmentRequestId"`
		SubmissionVersionID string   `json:"submissionVersionId"`
		DeclaredParcelIDs   []string `json:"declaredParcelIds"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}

	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	account, err := domain.NewCustomerAccountID(body.CustomerAccountID)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	source, err := domain.NewSource(body.Source)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	requestKey, err := domain.NewSourceRequestKey(body.SourceRequestKey)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	identity, err := domain.NewSourceIdentity(tenant, account, source, requestKey)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	requestID, err := domain.NewShipmentRequestID(body.ShipmentRequestID)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	version, err := domain.NewSubmissionVersionID(body.SubmissionVersionID)
	if err != nil {
		return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if len(body.DeclaredParcelIDs) == 0 {
		return psapplication.AdvanceAcceptanceChainCommand{},
			fmt.Errorf("%w: missing declared parcels", ErrPoisonEnvelope)
	}
	parcels := make([]domain.DeclaredParcelID, 0, len(body.DeclaredParcelIDs))
	for _, value := range body.DeclaredParcelIDs {
		parcel, err := domain.NewDeclaredParcelID(value)
		if err != nil {
			return psapplication.AdvanceAcceptanceChainCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
		}
		parcels = append(parcels, parcel)
	}

	return psapplication.AdvanceAcceptanceChainCommand{
		Identity:          identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: version,
		DeclaredParcelIDs: parcels,
	}, nil
}
