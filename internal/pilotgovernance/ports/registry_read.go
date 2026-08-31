package ports

import (
	"context"
	"time"
)

// 本文件是治理登记册三册的伴生列表读端口（票 admin-skeleton-closure-batch/02，键形
// 依 ADR-0083）：管理台 stage-admission 页的供数面。
//
// 签名按登记册实有维度成形，**不收租户参数**（ADR-0083 Decision 二）：治理是产品级
// 机制，八张表零 tenant_id 是设计不是漏了；对没有这一维的册子收下租户参数只有两种
// 下场——被静默忽略（签名撒谎）或拿去过滤一个不存在的列。查阅读端口另立，不并入
// 也不改造写侧的 AuthorityIntervalStore.ListCurrent——那是冲突预检口，「重叠必须在
// 准入前被抓住」是它的语义，两种消费不共口。
//
// 上列是检索列面的照实转写：恢复决定的在途盘点（jsonb）是决定的携带内容，照
// 登记册纪律另有装载口（ResumptionStore.FindBySuspension），本口不从 jsonb 里抠
// 字段冒充列。行内容全部是脱敏引用——建表纪律（真实客户、线路与阈值在参数登记册）
// 与登记 CLI 纪律（票 12）共同保证，读口不承担第二道脱敏。
//
// limit 非正拒；空册如实答空列表走 2xx 成格（ADR-0077 Decision 四原文适用，
// ADR-0083 Decision 二收尾句）。

// AuthorityIntervalRegistryRow 是生产权威区间册上列的一行。身份是（对象范围×能力×
// 事实类型×权威方）四维加生效区间；to 缺席即开放区间——缺席是真话，不为区间齐整
// 补一个编造的「无限远」。库面代理键（interval_id）不透出：它是追加序，不是身份。
type AuthorityIntervalRegistryRow struct {
	ObjectScope string
	Capability  string
	FactKind    string
	Authority   string
	FromAt      time.Time
	ToAt        time.Time
	HasToAt     bool
	InsertedAt  time.Time
}

// SuspensionRegistryRow 是暂停决定册上列的一行。暂停的是指定范围的新委托纳入，
// 不取消、不迁移、不回退在途——语义边界是结构性的，列面照登记原文转写。
type SuspensionRegistryRow struct {
	SuspensionID  string
	TriggerSource string
	Basis         string
	Evidence      string
	Scope         string
	ExecutedBy    string
	OccurredAt    time.Time
	EffectiveAt   time.Time
	InTransitNote string
}

// ResumptionRegistryRow 是恢复决定册上列的一行。一个暂停至多一次恢复（主键即被
// 恢复的暂停标识）；盘点内容在 jsonb 内不上列，盘点时刻照登透出。
type ResumptionRegistryRow struct {
	SuspensionID     string
	ReleaseEvidence  string
	ConsistencyCheck string
	InventoryTakenAt time.Time
	DecidedBy        string
	DecidedAt        time.Time
	EffectiveAt      time.Time
}

// GovernanceRegistryRead 是治理登记册三册的伴生列表读端口。
type GovernanceRegistryRead interface {
	ListAuthorityIntervals(ctx context.Context, limit int) ([]AuthorityIntervalRegistryRow, error)
	ListSuspensions(ctx context.Context, limit int) ([]SuspensionRegistryRow, error)
	ListResumptions(ctx context.Context, limit int) ([]ResumptionRegistryRow, error)
}
