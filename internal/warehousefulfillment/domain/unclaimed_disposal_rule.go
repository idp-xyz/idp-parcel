package domain

import (
	"errors"
	"math"
	"slices"
	"time"
)

// maxClaimWindowSeconds 是 time.Duration 能表示的最大整秒。再大则秒数乘 time.Second 回绕成负数。
const maxClaimWindowSeconds = math.MaxInt64 / int64(time.Second)

var (
	// ErrInvalidDisposalRuleRevision 是一笔处置规则修订缺了身份、登记责任方或登记时刻。
	ErrInvalidDisposalRuleRevision = errors.New("warehouse fulfillment: invalid unclaimed disposal rule revision")
)

// DisposalRuleField 指出待认领处置规则登记未受理时差在哪一格。
type DisposalRuleField uint8

const (
	DisposalRuleFieldInvalid DisposalRuleField = iota
	DisposalRuleFieldWarehouseCode
	DisposalRuleFieldClaimWindowSeconds
	DisposalRuleFieldDisposalMethods
)

func (field DisposalRuleField) String() string {
	switch field {
	case DisposalRuleFieldWarehouseCode:
		return "warehouseCode"
	case DisposalRuleFieldClaimWindowSeconds:
		return "claimWindowSeconds"
	case DisposalRuleFieldDisposalMethods:
		return "disposalMethods"
	default:
		return ""
	}
}

// DisposalRuleContent 是一笔待认领处置规则修订的完整内容：认领期限（按经过秒数，须为正，且不得大到乘进 time.Duration 会回绕）与期满后可用的处置方式（租户原词，至少一种、不重复）。
// 构造器不拦这些：让登记用例在判过重放、冲突、仓存在与修订连续之后答 NOT_ACCEPTED/CONTENT_INVALID 并指出那一格。
type DisposalRuleContent struct {
	claimWindowSeconds int64
	methods            []UnclaimedDisposalMethod
}

func NewDisposalRuleContent(claimWindowSeconds int64, methods []UnclaimedDisposalMethod) DisposalRuleContent {
	return DisposalRuleContent{
		claimWindowSeconds: claimWindowSeconds,
		methods:            slices.Clone(methods),
	}
}

func (content DisposalRuleContent) ClaimWindowSeconds() int64 { return content.claimWindowSeconds }

// ClaimWindowElapsed 报告从 occurredAt 到 now 是否已经走完认领期限。
// 按整秒比较，不把秒数乘进 time.Duration：超过 maxClaimWindowSeconds 会回绕成负数，期限未到会被当成已到（UC-WF-005、AT-WF-065）。
// 期限本身放不进 time.Duration 时答未走完。now.Sub 自身回绕（间隔超过可表示范围）时，认领期限一定已经走完。
func (content DisposalRuleContent) ClaimWindowElapsed(occurredAt, now time.Time) bool {
	if content.claimWindowSeconds <= 0 || content.claimWindowSeconds > maxClaimWindowSeconds {
		return false
	}
	if now.Before(occurredAt) {
		return false
	}
	elapsed := now.Sub(occurredAt)
	if elapsed < 0 {
		return true
	}
	return int64(elapsed/time.Second) >= content.claimWindowSeconds
}

func (content DisposalRuleContent) Methods() []UnclaimedDisposalMethod {
	return slices.Clone(content.methods)
}

// HasMethod 逐字比对处置方式在不在这版规则里。
func (content DisposalRuleContent) HasMethod(method UnclaimedDisposalMethod) bool {
	return slices.Contains(content.methods, method)
}

// InvalidField 核新修订的业务门：认领期限须为正且不超过 time.Duration 能表示的最大整秒，处置方式至少一种、非空白且不重复。
func (content DisposalRuleContent) InvalidField() DisposalRuleField {
	if content.claimWindowSeconds <= 0 || content.claimWindowSeconds > maxClaimWindowSeconds {
		return DisposalRuleFieldClaimWindowSeconds
	}
	if len(content.methods) == 0 {
		return DisposalRuleFieldDisposalMethods
	}
	seen := make(map[UnclaimedDisposalMethod]bool, len(content.methods))
	for _, method := range content.methods {
		if !method.valid() || seen[method] {
			return DisposalRuleFieldDisposalMethods
		}
		seen[method] = true
	}
	return DisposalRuleFieldInvalid
}

func (content DisposalRuleContent) SameAs(other DisposalRuleContent) bool {
	return content.claimWindowSeconds == other.claimWindowSeconds &&
		slices.Equal(content.methods, other.methods)
}

// DisposalRuleRevision 是一个仓的待认领处置规则的一笔修订，身份是（租户，仓编码），修订从 1 起只追加。
type DisposalRuleRevision struct {
	tenant       TenantID
	warehouse    WarehouseCode
	revision     int
	content      DisposalRuleContent
	registrant   Registrant
	registeredAt time.Time
}

func NewDisposalRuleRevision(
	tenant TenantID,
	warehouse WarehouseCode,
	revision int,
	content DisposalRuleContent,
	registrant Registrant,
	registeredAt time.Time,
) (DisposalRuleRevision, error) {
	if revision < 1 {
		return DisposalRuleRevision{}, ErrInvalidRevisionNumber
	}
	if !tenant.valid() || !warehouse.valid() || !registrant.valid() || registeredAt.IsZero() {
		return DisposalRuleRevision{}, ErrInvalidDisposalRuleRevision
	}
	return DisposalRuleRevision{
		tenant:       tenant,
		warehouse:    warehouse,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: normalizedInstant(registeredAt),
	}, nil
}

func (rule DisposalRuleRevision) Tenant() TenantID             { return rule.tenant }
func (rule DisposalRuleRevision) Warehouse() WarehouseCode     { return rule.warehouse }
func (rule DisposalRuleRevision) Revision() int                { return rule.revision }
func (rule DisposalRuleRevision) Content() DisposalRuleContent { return rule.content }
func (rule DisposalRuleRevision) Registrant() Registrant       { return rule.registrant }
func (rule DisposalRuleRevision) RegisteredAt() time.Time      { return rule.registeredAt }

func (rule DisposalRuleRevision) SameContentAs(other DisposalRuleRevision) bool {
	return rule.content.SameAs(other.content)
}
