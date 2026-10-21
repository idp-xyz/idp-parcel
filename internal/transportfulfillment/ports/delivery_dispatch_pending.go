package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送待形成册的存取口（ADR-0230 决定一、二）。执行器答`要求缺失`时立或刷新一条，任务形成时了结，对象已不在段里时作罢；
// 写它的只有派送发起执行器一处，读它的是按册重问与查阅页。

// ErrDeliveryDispatchPendingWriteRefused 是册的写口给出的确定性拒绝：没有环境事务、库上的约束不让写。原样重来一万次都一样，
// 调用方不该把它当成「册此刻读写不动」去重投；别的写入失败（连接断、超时、事务被打断）不带它。
var ErrDeliveryDispatchPendingWriteRefused = errors.New("transport fulfillment: delivery dispatch pending write refused")

// DeliveryDispatchPendingKey 是一件待形成派送的身份：哪个租户的哪个对象在哪个段里。不带入场依据——同一段同一对象只有一条参与
// 替代链，更正长出的替代版本不是进段，派送仍是那一项（票 demo-chain-gaps/129）。
type DeliveryDispatchPendingKey struct {
	TenantID domain.TenantID
	Segment  domain.FulfillmentSegmentReference
	Object   domain.CarriedObjectReference
}

// DeliveryDispatchPendingState 是册上一条的状态。
type DeliveryDispatchPendingState uint8

const (
	DeliveryDispatchPendingStateInvalid DeliveryDispatchPendingState = iota
	// DeliveryDispatchPending：执行器最近一次答的是`要求缺失`。
	DeliveryDispatchPending
	// DeliveryDispatchPendingFormed：之后某一拍形成了任务，或认出了已在册的那一项。
	DeliveryDispatchPendingFormed
	// DeliveryDispatchPendingLapsed：之后某一拍答对象已不在段里（链尾失效或已离场），这一件不会再形成。
	DeliveryDispatchPendingLapsed
)

func (state DeliveryDispatchPendingState) String() string {
	switch state {
	case DeliveryDispatchPending:
		return "PENDING"
	case DeliveryDispatchPendingFormed:
		return "FORMED"
	case DeliveryDispatchPendingLapsed:
		return "LAPSED"
	default:
		return ""
	}
}

// DeliveryDispatchPendingRecord 是册上一条。Missing 是执行器缺件名单的字面（`DELIVERY_PLACE` 等），按地点、时间窗、条件的固定
// 顺序；名单是执行器的词，本层照实搬运。各时刻都是那一拍的业务时间（执行器命令里的 OccurredAt）。LastAskedAt 是最近一次问这一件
// 的时刻，不论那一拍答了什么；排队与到期都按它，不按最近一次答复。
type DeliveryDispatchPendingRecord struct {
	Key             DeliveryDispatchPendingKey
	EntryBasis      domain.ParticipationBasisReference
	Missing         []string
	FirstAnsweredAt time.Time
	LastAnsweredAt  time.Time
	LastAskedAt     time.Time
	State           DeliveryDispatchPendingState
	// Task 只在已形成时给出。
	Task string
	// LapsedBy 只在已作罢时给出：执行器答的那一格。
	LapsedBy  string
	SettledAt time.Time
}

// DeliveryDispatchPendingRegister 是执行器写册的各口，都要环境事务：册上那一行与执行器那一拍的其余写入同笔。各口都把最近一次问
// 的时刻记成这一拍。
//
//   - RecordMissing：立一条，或把待形成、已作罢的那一条刷新为待形成，入场依据、缺件与最近一次答复时刻取这一次；首次答复时刻只在
//     立的那一刻写。已形成的不动。
//   - SettleFormed：把一条改成已形成；册上没有这一件就什么也不写——册只记待形成过的那几件，顺利形成的任务不进册。
//   - SettleLapsed：只把待形成的那一条改成已作罢；已形成的不动——它的任务还在，对象离场是任务之后的事。
//   - MarkAsked：只给待形成的那一条记下这一次问过，别的不动。答复动不了册的那几格（未决、引用被占、不受理……）走这一口。
type DeliveryDispatchPendingRegister interface {
	RecordMissing(ctx context.Context, key DeliveryDispatchPendingKey, entryBasis domain.ParticipationBasisReference, missing []string, answeredAt time.Time) error
	SettleFormed(ctx context.Context, key DeliveryDispatchPendingKey, task domain.DispatchTaskReference, at time.Time) error
	SettleLapsed(ctx context.Context, key DeliveryDispatchPendingKey, lapsedBy string, at time.Time) error
	MarkAsked(ctx context.Context, key DeliveryDispatchPendingKey, at time.Time) error
}

// DeliveryDispatchPendingQueue 是按册重问那一口读的：一个租户待形成的那几件，最近一次问最早的在前。askedBy 非零时只列最近一次
// 问不晚于它的（到期重问按重问间隔算出这一刻）；零值不筛。CountPending 用同一个筛法数件数，重问据它答「这一次没问到的还有几件」。
type DeliveryDispatchPendingQueue interface {
	ListPending(ctx context.Context, tenant domain.TenantID, askedBy time.Time, limit int) ([]DeliveryDispatchPendingRecord, error)
	CountPending(ctx context.Context, tenant domain.TenantID, askedBy time.Time) (int, error)
}
