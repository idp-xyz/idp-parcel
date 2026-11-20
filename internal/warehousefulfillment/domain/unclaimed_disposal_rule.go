package domain

import (
	"errors"
	"slices"
	"time"
)

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

// DisposalRuleContent 是一笔待认领处置规则修订的完整内容：认领期限（按经过秒数，须为正）与期满后可用的处置方式（租户原词，至少一种、不重复）。
// 构造器不拦「期限不为正」与「方式为空/重复」：让登记用例在判过重放、冲突、仓存在与修订连续之后答 NOT_ACCEPTED/CONTENT_INVALID 并指出那一格。
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

// ClaimWindow 是自到货发生时刻起经过的认领期限，不依赖仓的业务时区（UC-WF-005、AT-WF-065）。
func (content DisposalRuleContent) ClaimWindow() time.Duration {
	return time.Duration(content.claimWindowSeconds) * time.Second
}

func (content DisposalRuleContent) Methods() []UnclaimedDisposalMethod {
	return slices.Clone(content.methods)
}

// HasMethod 逐字比对处置方式在不在这版规则里。
func (content DisposalRuleContent) HasMethod(method UnclaimedDisposalMethod) bool {
	return slices.Contains(content.methods, method)
}

// InvalidField 核新修订的业务门：认领期限须为正，处置方式至少一种、非空白且不重复。
func (content DisposalRuleContent) InvalidField() DisposalRuleField {
	if content.claimWindowSeconds <= 0 {
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
