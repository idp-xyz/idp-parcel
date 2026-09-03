package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 揽派任务登记册的存取口（tf-unwired-seven/05）。
//
// 另开一个文件而不是并进 ports.go，理由同 fulfillment_segment.go：那份文件已经装了十二组端口，
// 而本包既有惯例是按关注点分文件；分开也顺带避开共享树上两个人改同一个文件那种 pathspec 挡不住
// 的混合。

// DispatchTaskKey 是揽派任务的幂等键。
//
// 任务没有版本维：**改约不改身份**（领域 Reschedule 的原话是「改约或重派形成的是新尝试，不是
// 新任务」），关闭也只是同一任务换状态。要再揽派一次，那是新任务、新键。
type DispatchTaskKey struct {
	TenantID domain.TenantID
	Task     domain.DispatchTaskReference
}

// DispatchTaskRecord 是一项任务连同它全部对象越过提交边界留下的东西。
//
// 对象成员不单列一个字段：它们在聚合内部，取出来就得有人保证两半一致，而那正是聚合要消灭的
// 那种可能。适配器按任务写两张表、按任务读回整图（与实际履约段同形）。
type DispatchTaskRecord struct {
	Key        DispatchTaskKey
	Task       domain.DispatchTask
	RecordedAt time.Time
}

// DispatchTaskSaveOutcome 是首登的结果。撞键是业务答案不是错误（ADR-0031）——同一任务被重投
// 时交回`已建立`，不顶替原任务：一次重投不该改写工作范围。
type DispatchTaskSaveOutcome uint8

const (
	DispatchTaskSaveOutcomeInvalid DispatchTaskSaveOutcome = iota
	DispatchTaskSaved
	DispatchTaskAlreadyOpen
)

// DispatchTaskRegistry 按幂等键找回并保存揽派任务。
//
// **本口只有首登，没有演进写口。** 改约、终止与完成都是带前置条件的状态转换（改约不改身份、
// 关闭带依据且一次为限），照 ADR-0097 的教训它们该各开一个窄口而不是一个通用 Update——一个
// 能表达任何状态的 Save 也就能表达「把已关闭的任务写回开放」，而那是领域明禁的。那几个口等
// 各自的编排来时再开，本票不预先造无人调用的方法。
type DispatchTaskRegistry interface {
	FindByKey(ctx context.Context, key DispatchTaskKey) (DispatchTaskRecord, bool, error)
	Save(ctx context.Context, record DispatchTaskRecord) (DispatchTaskSaveOutcome, error)
}
