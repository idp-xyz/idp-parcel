package ports

import (
	"context"
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
