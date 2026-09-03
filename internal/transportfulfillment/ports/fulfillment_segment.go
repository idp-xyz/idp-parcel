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

// SegmentJoinOutcome 是一个对象加入既有段的结果。`已在段内`是业务答案不是错误——
// CONTEXT「同一实际控制范围不能因伙伴重投、任务重建或批量重试重复建立履约参与」。
type SegmentJoinOutcome uint8

const (
	SegmentJoinOutcomeInvalid SegmentJoinOutcome = iota
	ObjectJoined
	ObjectAlreadyParticipating
)

// ParticipationEndOutcome 是逐对象离场的结果。`已离场`同样是业务答案：已结束的参与不
// 重复结束也不改写（领域 `end` 的原话），再次进入是新的段。
type ParticipationEndOutcome uint8

const (
	ParticipationEndOutcomeInvalid ParticipationEndOutcome = iota
	ParticipationEnded
	ParticipationAlreadyEnded
)

// SegmentCloseOutcome 是关段的结果。
type SegmentCloseOutcome uint8

const (
	SegmentCloseOutcomeInvalid SegmentCloseOutcome = iota
	SegmentClosed
	SegmentAlreadyClosed
)

// ActualFulfillmentSegmentRegistry 按幂等键找回并保存实际履约段，并按动作开三个窄写口
// （ADR-0097）。写入代数同 ADR-0031：撞键是业务答案不是错误。
//
// **没有通用 Update，这是本口最要紧的一条。** 价值不在"窄"，在于 CONTEXT 禁的那些操作在
// 这个口上**表达不出来**：
//
//   - `Join` 只插不改，`EndParticipation` 与 `CloseSegment` 各自只填自己那几列且带前置
//     条件，**没有任何一条路径能把已发生的写回未发生**——不是不该，是没有那个入参。
//   - 三个口没有一个接受「整段成员集合」，因此「整段结果覆盖成员差异」也表达不出来。
//
// 整段重写口做不到这些：`Save(segment)` 能表达任何状态**包括倒退**，挡住倒退的只有调用方
// 碰巧传了一个向前演进过的聚合；它并发下还会静默丢成员（两个加入各基于一份旧快照重写，
// 后写的抹掉先写的，而两次都"成功"）。
//
// **`Save` 仍只用于首登**（段由首个对象的控制事实成立），不承担演进。
//
// 一条本口守不住、留在领域的：`ErrSegmentStillActive`——仍有在场参与时段关不上。那是跨行
// 条件，`CloseSegment` 的前置条件表达不了。**编排必须先读回整段、走领域的 `CloseSegment`
// 再落库**，不得直接调本口关段。这是 ADR-0097 里唯一一条靠纪律而非结构的约束，如实标出。
type ActualFulfillmentSegmentRegistry interface {
	FindByKey(ctx context.Context, key FulfillmentSegmentKey) (FulfillmentSegmentRecord, bool, error)
	Save(ctx context.Context, record FulfillmentSegmentRecord) (SegmentSaveOutcome, error)

	// Join 把一个对象的参与关系插进既有段。参与关系整体由调用方从领域取出——本口不拆解它，
	// 拆解就等于让适配器重新组装一遍领域已经判完的东西。
	Join(
		ctx context.Context,
		key FulfillmentSegmentKey,
		participation domain.FulfillmentParticipation,
		recordedAt time.Time,
	) (SegmentJoinOutcome, error)

	// EndParticipation 只填离场三列，且只作用于仍在场的那一条。已离场的不被改写。
	EndParticipation(
		ctx context.Context,
		key FulfillmentSegmentKey,
		participation domain.FulfillmentParticipation,
	) (ParticipationEndOutcome, error)

	// CloseSegment 只填关闭两列，且只作用于尚未关闭的段。
	CloseSegment(
		ctx context.Context,
		key FulfillmentSegmentKey,
		closedAt time.Time,
	) (SegmentCloseOutcome, error)
}
