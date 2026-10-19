package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 出发门禁读数要问的几口（ADR-0205）。另立文件而不并进 ports.go，理由同 fulfillment_segment.go：本包按关注点
// 分文件，这几口只服务出发登记一处。

// DepartureSegmentView 按键读回一个实际履约段，供出发读数取「出发那一刻段里在控的对象」。只读，不带 Save：
// 出发登记不建段、不改段——移动是段内的事实，不是段的成立或结束依据。ActualFulfillmentSegmentRegistry 天然满足它。
type DepartureSegmentView interface {
	FindByKey(ctx context.Context, key FulfillmentSegmentKey) (FulfillmentSegmentRecord, bool, error)
}

// DepartureGateQuery 是对一件对象的一次出发问询：租户、对象、出发自哪个节点到哪个节点、出发时点。只交意图，
// 不交「问哪几种受门禁动作」——适用哪些动作是关务的判断（ADR-0206 决定二），本上下文不逐动作问。
type DepartureGateQuery struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	From     domain.NetworkNodeReference
	To       domain.NetworkNodeReference
	At       time.Time
}

// DepartureGateView 是 customs-compliance 动作准入判断的消费侧端口（ADR-0025）。实现只翻译：把关务的答复译成
// 本上下文那件对象的一格并原文保存依据，不重折限制、门禁与放行（ADR-0206 决定五只在关务那边定义一处）。
//
// 关务自己答「读不动」时交回读不动那一格、不报错；error 只说这一问根本没有形成答复（调用失败或答复译不回来），
// 编排把它也记成读不动——出发照记，不因读不动挡下（ADR-0205 决定一）。译不回来的那一类包住
// ErrDepartureGateAnswerUntranslatable 交出。
//
// 实现在调用方写事务之外被调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进出发登记那一笔会随它回滚。
type DepartureGateView interface {
	ReadDepartureGate(ctx context.Context, query DepartureGateQuery) (domain.DepartureGateFinding, error)
}

// ErrDepartureGateAnswerUntranslatable：关务那一问有了答复，却译不回本上下文的那一格（词汇漂移、未受理、答复形状不对），
// 或本上下文的值在关务那边立不起来。编排仍把那一件记成读不动、出发照记，但依据种类与调用失败分开：调用失败过一会儿
// 可能就好，这一类重问也是同一个结果，要查的是两边的口径而不是关务进程或某本册通不通。
var ErrDepartureGateAnswerUntranslatable = errors.New("transport fulfillment: the customs answer cannot be translated into a departure gate finding")
