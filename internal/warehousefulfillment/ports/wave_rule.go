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
	Warehouse    domain.WarehouseCode
	Code         domain.WaveRuleCode
	Revision     int
	Name         string
	Owner        string
	Dispatch     string
	Profile      string
	MaxRequests  int
	BatchPick    bool
	Deactivated  bool
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
	// TagWaveRun 在波次上记下它是哪条规则的哪一次执行放出来的。
	TagWaveRun(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, rule domain.WaveRuleCode, runID string) error
	WavesOfRun(ctx context.Context, tenant domain.TenantID, runID string) ([]RuleWave, error)
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
	BatchPick     bool
	Deactivated   bool
	Operator      string
	RegisteredAt  time.Time
}
