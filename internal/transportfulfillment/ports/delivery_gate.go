package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 交付门禁读数要问的几口（ADR-0213 决定五）。另立文件而不并进 ports.go，理由同 handover_gate.go：这几口只服务交付门禁读数
// 一处（有效交付登记与交付前问询走的是同一条读数路径）。找对象进段那次交接的那一口与出发读数共用，在 entry_handover.go。

// DeliveryGateSegmentView 是读数取 at 时读段的那两问：对象此刻在哪个段里在场，那一段长什么样。只读，不带写口——读数
// 不动段，结束参与是登记编排在写事务里的事。ActualFulfillmentSegmentRegistry 天然满足它。
type DeliveryGateSegmentView interface {
	FindActiveSegments(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]FulfillmentSegmentKey, error)
	FindByKey(ctx context.Context, key FulfillmentSegmentKey) (FulfillmentSegmentRecord, bool, error)
}

// DeliveryGateQuery 是对一件对象的一次交付问询：租户、对象、在哪个节点交付、交付时点。只交意图，不交「问哪几种受门禁
// 动作」：适用哪些动作是关务的判断（ADR-0206 决定二）。
type DeliveryGateQuery struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Node     domain.NetworkNodeReference
	At       time.Time
}

// DeliveryGateView 是 customs-compliance 动作准入判断的消费侧端口（ADR-0025），问的是交付意图。口径与 HandoverGateView
// 相同：关务自己答「读不动」时交回读不动那一格、不报错；error 只说这一问根本没有形成答复，编排把它也记成读不动、交付
// 照登；译不回来的那一类包住 ErrDeliveryGateAnswerUntranslatable 交出。
//
// 实现在调用方写事务之外被调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进交付登记那一笔会随它回滚。
type DeliveryGateView interface {
	ReadDeliveryGate(ctx context.Context, query DeliveryGateQuery) (domain.GateFinding, error)
}

// ErrDeliveryGateAnswerUntranslatable：关务那一问有了答复，却译不回本上下文的那一格，或本上下文的值（含交接登记方声明的
// 节点）在关务那边立不起来。编排仍把那一件记成读不动、交付照登，依据种类与调用失败分开，理由同
// ErrDepartureGateAnswerUntranslatable。
var ErrDeliveryGateAnswerUntranslatable = errors.New("transport fulfillment: the customs answer cannot be translated into a delivery gate finding")
