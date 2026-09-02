package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 实际履约段登记册的存取口（tf-unwired-seven/01）。
//
// 另开一个文件而不是并进 ports.go：那份文件已经装了十二组端口，而本包既有惯例就是按关注点
// 分文件。分开还顺带避开一件事——共享树上两个人改同一个文件时，pathspec 挡不住那种混合。

// FulfillmentSegmentKey 是实际履约段的幂等键。
//
// 段没有版本维：CONTEXT「已经成立的实际履约段及履约参与关系不能被取消、删除或回写为未发生」，
// 更正走的是**新的段**而不是同一个段的新版本——「再次进入是新的段」这条在领域 join 上就守着。
type FulfillmentSegmentKey struct {
	TenantID domain.TenantID
	Segment  domain.FulfillmentSegmentReference
}

// FulfillmentSegmentRecord 是一个段连同它全部成员越过提交边界留下的东西。
//
// 成员不单列一个字段：它们在聚合内部，取出来就得有人保证两半一致，而那正是聚合要消灭的
// 那种可能。适配器按段写两张表、按段读回整图。
type FulfillmentSegmentRecord struct {
	Key        FulfillmentSegmentKey
	Segment    domain.ActualFulfillmentSegment
	RecordedAt time.Time
}

type SegmentSaveOutcome uint8

const (
	SegmentSaveOutcomeInvalid SegmentSaveOutcome = iota
	SegmentSaved
	SegmentAlreadyRegistered
)

// ActualFulfillmentSegmentRegistry 按幂等键找回并保存实际履约段（写入代数同 ADR-0031：
// 撞键是业务答案不是错误，第二个写入方按键读回赢家）。
//
// **只有首登，没有 Update。** 段成立之后的演进（对象加入、逐对象离场、关段）今天走的是
// 领域转换门加一次整段重写——那一步属票 02 与票 07 的编排，本口先只承载首登与读回，
// 免得在还没有调用方的时候先猜一个写入形状。
type ActualFulfillmentSegmentRegistry interface {
	FindByKey(ctx context.Context, key FulfillmentSegmentKey) (FulfillmentSegmentRecord, bool, error)
	Save(ctx context.Context, record FulfillmentSegmentRecord) (SegmentSaveOutcome, error)
}
