package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 装载分配登记册的存取口（tf-unwired-seven/05）。

// LoadAssignmentKey 是装载分配某一个版本的幂等键。
//
// **版本在键里，这是本口与揽派任务最大的分别。** 变化与撤回都形成新版本、原版本原样保留
// （CONTEXT「装载分配形成、变化或撤回时保存版本和对象范围」），所以「一次分配」在库里是一
// 条版本链而不是一行会变的记录。把版本挪出键就等于允许覆盖分配历史。
type LoadAssignmentKey struct {
	TenantID   domain.TenantID
	Assignment domain.LoadAssignmentReference
	Version    domain.LoadAssignmentVersion
}

// LoadAssignmentRecord 是一个分配版本连同它的对象范围越过提交边界留下的东西。
//
// **没有成员计数字段。** CONTEXT 要求申请量、接受量、预占量、分配量、释放量与实际装载量分别
// 保存，而那几个量在容量池那一侧；本记录若再存一个「分配了几个」，它会与成员行漂移，且漂移
// 时没有任何东西会报。要数就数成员。
type LoadAssignmentRecord struct {
	Key        LoadAssignmentKey
	Assignment domain.LoadAssignment
	RecordedAt time.Time
}

// LoadAssignmentSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）。
type LoadAssignmentSaveOutcome uint8

const (
	LoadAssignmentSaveOutcomeInvalid LoadAssignmentSaveOutcome = iota
	LoadAssignmentSaved
	// LoadAssignmentVersionAlreadyRegistered 覆盖两种撞：同一版本已在册，或同一分配已有一个首版而这次也是首版。
	// 库面一句 ON CONFLICT DO NOTHING 分不出撞的是哪道唯一，调用方先按本次键读、读不回再按分配读链头。
	LoadAssignmentVersionAlreadyRegistered
)

// LoadAssignmentRegistry 按（租户 + 分配 + 版本）找回并登记装载分配，也按（租户 + 分配）取链头。
//
// **只插不改，本口没有也不会有 Update。** 变化与撤回在领域里各自形成新版本（ReviseMembers 与
// Withdraw 都返回新值、原值不动），库面跟上的方式就是再插一行。给这个口开一个能改既有行的
// 方法，「不修改分配历史」这条就在类型上表达不出来了——而它是 CONTEXT 的硬句。
type LoadAssignmentRegistry interface {
	FindByKey(ctx context.Context, key LoadAssignmentKey) (LoadAssignmentRecord, bool, error)
	// FindChainHead 取回该分配此刻未被任何版本回指的那一版；分配还没有任何版本时只回 false。登记口靠它
	// 认出「这次分配已经形成过」——按本次键查只看得见同一版本，看不见同一分配下的别的版本。
	FindChainHead(
		ctx context.Context,
		tenant domain.TenantID,
		assignment domain.LoadAssignmentReference,
	) (LoadAssignmentRecord, bool, error)
	Save(ctx context.Context, record LoadAssignmentRecord) (LoadAssignmentSaveOutcome, error)
}
