package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 交出门禁读数要问的几口（ADR-0213 决定四）。另立文件而不并进 ports.go，理由同 departure_gate.go：这几口只服务交接
// 登记一处。

// HandoverGatePredecessorView 按键读回一版交接，供更正在写事务之前取前版的节点与业务时间——更正成`已交接`的那一版照
// 前版承继这两样，问关务要的正是它们。只读，不带 Save。TransportHandoverRegistry 天然满足它。
type HandoverGatePredecessorView interface {
	FindByKey(ctx context.Context, key TransportHandoverKey) (TransportHandoverRecord, bool, error)
}

// HandoverGateQuery 是对一件对象的一次交出问询：租户、对象、在哪个节点交出、交出之后往哪去、交出时点。Onward 零值即
// 这一版没声明去向——照问，跨关务区域那一道由关务按 ADR-0206 决定二不列。只交意图，不交「问哪几种受门禁动作」：
// 适用哪些动作是关务的判断（ADR-0206 决定二），本上下文不逐动作问。
type HandoverGateQuery struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Node     domain.NetworkNodeReference
	Onward   domain.NetworkNodeReference
	At       time.Time
}

// HandoverGateView 是 customs-compliance 动作准入判断的消费侧端口（ADR-0025），问的是交出意图。实现只翻译，口径与
// DepartureGateView 相同：关务自己答「读不动」时交回读不动那一格、不报错；error 只说这一问根本没有形成答复，编排把它也
// 记成读不动、交接照登；译不回来的那一类包住 ErrHandoverGateAnswerUntranslatable 交出。
//
// 实现在调用方写事务之外被调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进交接登记那一笔会随它回滚。
type HandoverGateView interface {
	ReadHandoverGate(ctx context.Context, query HandoverGateQuery) (domain.GateFinding, error)
}

// ErrHandoverGateAnswerUntranslatable：关务那一问有了答复，却译不回本上下文的那一格，或本上下文的值（含登记方声明的
// 节点）在关务那边立不起来。编排仍把那一件记成读不动、交接照登，依据种类与调用失败分开，理由同
// ErrDepartureGateAnswerUntranslatable。
var ErrHandoverGateAnswerUntranslatable = errors.New("transport fulfillment: the customs answer cannot be translated into a handover gate finding")
