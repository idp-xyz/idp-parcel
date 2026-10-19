package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是周期安排登记册与运输机会候选来源越过提交边界的形状（ADR-0201）。
//
// 具体班次仍走 TransportScheduleStore：FindByKey 交回当前版，Save 落记录上那一版——首版与下一版是同一个写口，
// 版本号在班次本体上，撞主键就是另一方先落了那一版。

// ScheduleTemplateKey 是一份周期安排的键。版本不在键里：「再登一次」比的是当前版，同代码异内容就是下一版。
type ScheduleTemplateKey struct {
	TenantID domain.TenantID
	Code     domain.ScheduleTemplateCode
}

// ScheduleTemplateRecord 是周期安排一版越过提交边界留下的东西。
type ScheduleTemplateRecord struct {
	Key           ScheduleTemplateKey
	ContentDigest string
	Template      domain.ScheduleTemplate
	RecordedAt    time.Time
}

// ScheduleTemplateSaveOutcome 是登记一版的结果。撞键是业务答案不是错误（ADR-0031）：另一方先落了同一个版本号，
// 是重放还是另一份内容由调用方读回比对。
type ScheduleTemplateSaveOutcome uint8

const (
	ScheduleTemplateSaveOutcomeInvalid ScheduleTemplateSaveOutcome = iota
	ScheduleTemplateSaved
	ScheduleTemplateVersionTaken
)

func (outcome ScheduleTemplateSaveOutcome) String() string {
	switch outcome {
	case ScheduleTemplateSaved:
		return "SAVED"
	case ScheduleTemplateVersionTaken:
		return "VERSION_TAKEN"
	default:
		return ""
	}
}

// ScheduleTemplateRegistry 按键取回当前版并登记新版，只插不改。
type ScheduleTemplateRegistry interface {
	FindCurrentTemplate(ctx context.Context, key ScheduleTemplateKey) (ScheduleTemplateRecord, bool, error)
	SaveTemplateVersion(ctx context.Context, record ScheduleTemplateRecord) (ScheduleTemplateSaveOutcome, error)
}

// ScheduleCandidateSource 是准备评估候选时读登记册的口（ADR-0201 决定五、六）。四问各自独立：
//
//   - ConnectionHasSchedules 答该连接上有没有任何当前版的周期安排或具体班次，不问时间。没有就是`未配置`——缺的是
//     租户取值，不是依赖故障，也不是「这一段时间没车」。
//   - ListCurrentTemplatesOnConnection 交回该连接上全部周期安排的当前版，由领域在评估时物化，不预生成。
//   - ListCurrentSchedulesDepartingWithin 交回该连接上出发时刻落在 [from, to] 的已登记具体班次的当前版。
//   - FindCurrentSchedules 按身份交回已登记班次的当前版，不问连接与时刻：物化出的某一班若已登过，登记册上那一版
//     才是它现在的样子，哪怕它已改到这段时间或这条连接之外。
//
// 只按连接代码取，不按起讫节点筛：起讫不符是候选的淘汰原因，要连同候选一起交给评估方，不能在读口上静默滤掉。
type ScheduleCandidateSource interface {
	ConnectionHasSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		connection domain.NetworkConnectionReference,
	) (bool, error)
	ListCurrentTemplatesOnConnection(
		ctx context.Context,
		tenant domain.TenantID,
		connection domain.NetworkConnectionReference,
	) ([]ScheduleTemplateRecord, error)
	ListCurrentSchedulesDepartingWithin(
		ctx context.Context,
		tenant domain.TenantID,
		connection domain.NetworkConnectionReference,
		from, to time.Time,
	) ([]ScheduleRecord, error)
	FindCurrentSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		schedules []domain.ScheduleReference,
	) ([]ScheduleRecord, error)
}
