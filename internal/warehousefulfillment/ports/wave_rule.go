package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 成波规则圈单时认的件型。ANY 是不限。
const (
	WaveRuleProfileAny        = "ANY"
	WaveRuleProfileSingleUnit = "SINGLE_UNIT"
	WaveRuleProfileMultiUnit  = "MULTI_UNIT"
)

// WaveRule 是一条成波规则的一版：圈哪些已预占的出库请求、一个波次最多几张、怎么拣。
// Owner 与 Dispatch 为空是不限。规则是租户按仓登记的取值，产品不带默认的规则。
type WaveRule struct {
	Warehouse   domain.WarehouseCode
	Code        domain.WaveRuleCode
	Revision    int
	Name        string
	Owner       string
	Dispatch    string
	Profile     string
	MaxRequests int
	// MaxLines 与 MaxUnits 是一个波次最多几行、最多几件（基本单位）；0 是不限。
	MaxLines    int
	MaxUnits    int64
	BatchPick   bool
	Deactivated bool
	// ReleaseTimes 是每天到点放的时点，HH:MM，按仓的业务时区，从早到晚不重复；空是只由人点。
	ReleaseTimes []string
	Operator     string
	RegisteredAt time.Time
}

// WaveRuleRun 是按规则成波的一次执行。Released 是放进波次的请求数，Skipped 是圈到了而分配不出、留在已预占的，
// Remaining 是超过一次执行的上限、这一次没轮到的。
type WaveRuleRun struct {
	RunID     string
	Warehouse domain.WarehouseCode
	Rule      domain.WaveRuleCode
	Revision  int
	Operator  string
	RanAt     time.Time
	Released  int
	Skipped   int
	Remaining int
}

// RuleWave 是一次执行放出的一个波次和它里面的请求数。
type RuleWave struct {
	Wave     string
	Requests int
}

// WaveRuleStore 读写成波规则与它的执行。写入在没有环境事务时拒绝。
type WaveRuleStore interface {
	// LoadWaveRule 取这条规则最新的一版。
	LoadWaveRule(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.WaveRuleCode) (WaveRule, bool, error)
	SaveWaveRule(ctx context.Context, tenant domain.TenantID, rule WaveRule) error
	// WaveRuleCandidates 照规则圈出这个仓里已预占的出库请求，受理得早的在前，至多交回 limit 张；
	// total 是圈得到的总数，比交回的多时就是还有没轮到的。
	WaveRuleCandidates(ctx context.Context, tenant domain.TenantID, rule WaveRule, limit int) (requests []domain.OutboundRequestID, total int, err error)
	LoadWaveRuleRun(ctx context.Context, tenant domain.TenantID, runID string) (WaveRuleRun, bool, error)
	SaveWaveRuleRun(ctx context.Context, tenant domain.TenantID, run WaveRuleRun) error
	// ScheduledWaveRules 列出登了时点、最新一版没停用的规则；warehouse 为空是本租户各仓的。按仓、规则码排。
	ScheduledWaveRules(ctx context.Context, tenant domain.TenantID, warehouse string) ([]WaveRule, error)
	// ClaimWaveRuleSlot 占下一条规则在一个业务日的一个时点；已经有人占过（办过，或另一笔事务正在办）答否。
	ClaimWaveRuleSlot(ctx context.Context, tenant domain.TenantID, rule WaveRule, date, slot, runID, operator string, at time.Time) (bool, error)
	// SettleWaveRuleSlot 记下这个时点办的结果。
	SettleWaveRuleSlot(ctx context.Context, tenant domain.TenantID, rule WaveRule, date, slot, outcome string) error
	// ReleaseWaveRuleSlot 放开占下的时点：这一次没办成（未配置、撞号），下一次调用再办。
	ReleaseWaveRuleSlot(ctx context.Context, tenant domain.TenantID, rule WaveRule, date, slot string) error
	// AnyWaveStartingWith 回答有没有波次号以这个前缀开头的波次。按规则成波放出的波次号是执行号加序号，
	// 动手之前用它核有没有撞号。
	AnyWaveStartingWith(ctx context.Context, tenant domain.TenantID, prefix string) (bool, error)
	// TagWaveRun 在波次上记下它是哪条规则的哪一次执行放出来的。
	TagWaveRun(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, rule domain.WaveRuleCode, runID string) error
	WavesOfRun(ctx context.Context, tenant domain.TenantID, runID string) ([]RuleWave, error)
}

// 放不出的例外是哪一种放波次记下的。
const (
	AllocationExceptionFromWave = "WAVE"
	AllocationExceptionFromRule = "RULE"
)

// AllocationExceptionStore 记下放波次时分配不出、被跳过的已预占请求。写入在没有环境事务时拒绝。
type AllocationExceptionStore interface {
	// SaveAllocationException 记下这张请求最近一次被跳过：source 是哪一种放波次，reference 是波次号或执行号。
	SaveAllocationException(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, source, reference string, at time.Time) error
	HasAllocationException(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (bool, error)
}

// WaveRuleOverview 是设置总览里的一条成波规则：它最新的一版。
type WaveRuleOverview struct {
	WarehouseCode string
	RuleCode      string
	Revision      int
	Name          string
	OwnerAccount  string
	Dispatch      string
	Profile       string
	MaxRequests   int
	MaxLines      int
	MaxUnits      int64
	ReleaseTimes  []string
	BatchPick     bool
	Deactivated   bool
	Operator      string
	RegisteredAt  time.Time
}
