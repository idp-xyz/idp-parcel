package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 实际移动事实登记册的存取口（tf-unwired-seven/06）。

// MovementFactKey 是一条实际移动事实某一个版本的幂等键。
//
// **版本在键里。** 迟到与更正形成新版本、原记录不被改写，所以一条事实在库里是一条版本链。
// CONTEXT 那句「协作事项取消、替代或缩小范围时……**已经形成的权威交接、实际移动和费用责任
// 继续保留**」正是靠这一点守住的：上游意图取消抹不掉一条已发生的事实。
type MovementFactKey struct {
	TenantID domain.TenantID
	Fact     domain.MovementFactReference
	Version  domain.MovementFactVersion
}

// MovementFactRecord 是一个移动事实版本越过提交边界留下的东西。
type MovementFactRecord struct {
	Key        MovementFactKey
	Fact       domain.TransportMovementFact
	RecordedAt time.Time
}

// MovementFactSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）。
type MovementFactSaveOutcome uint8

const (
	MovementFactSaveOutcomeInvalid MovementFactSaveOutcome = iota
	MovementFactSaved
	MovementFactVersionAlreadyRegistered
)

// MovementFactRegistry 按（租户 + 事实 + 版本）找回并登记实际移动事实。
//
// **只插不改，本口没有也不会有 Update。** 移动是已经发生的事实——它可以被更正（新版本回指
// 前身），但不可回写。给这个口开一个能改既有行的方法，「已经形成的实际移动继续保留」这条就
// 在类型上表达不出来了。
type MovementFactRegistry interface {
	FindByKey(ctx context.Context, key MovementFactKey) (MovementFactRecord, bool, error)
	Save(ctx context.Context, record MovementFactRecord) (MovementFactSaveOutcome, error)
}

// ErrMovementFactRecordUnreadable：库里声明了这一段的某一行读回来了，却过不了重建门。坏行不随重投变好，与读不动分开报，
// 调用方据此不把它答成未决（ADR-0029：确定性错误不进未决名单）。
var ErrMovementFactRecordUnreadable = errors.New("transport fulfillment: a stored movement fact cannot be read back")

// MovementFactsOfSegment 按段列出声明了这一段的在册移动事实（ADR-0224 决定二）。
//
// 排序契约：交回每一个版本，先按事实、同一条事实按登记时刻、同一时刻按版本串排，重放时一字不变。段只是登记方声明的引用、不加
// 外键，所以本口不问段在不在册：段不在册时照样交回声明了它的事实。没有声明这一段的事实交回空切片，不是错误。与登记口分开立：
// 本口只读，给它的消费方不必拿到 Save。
//
// error 有两类：包住 ErrMovementFactRecordUnreadable 的是某一行过不了重建门，其余是读不动。
type MovementFactsOfSegment interface {
	FindBySegment(ctx context.Context, key FulfillmentSegmentKey) ([]MovementFactRecord, error)
}
