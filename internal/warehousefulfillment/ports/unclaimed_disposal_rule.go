package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// DisposalRuleLookup 读一个仓最新一版的待认领处置规则，供处置用例判定认领期限与允许的方式。
type DisposalRuleLookup interface {
	LoadLatestDisposalRule(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
	) (domain.DisposalRuleRevision, bool, error)
}

// DisposalRuleRegistry 是待认领处置规则修订册的写口。SaveDisposalRuleRevision 在没有环境事务时拒绝。
type DisposalRuleRegistry interface {
	DisposalRuleLookup
	LoadDisposalRuleRevision(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		revision int,
	) (domain.DisposalRuleRevision, bool, error)
	SaveDisposalRuleRevision(
		ctx context.Context,
		revision domain.DisposalRuleRevision,
	) (RegistrySaveOutcome, error)
}

// DisposalRuleRow 是一笔待认领处置规则修订的读出形状，列表与修订史共用。
type DisposalRuleRow struct {
	WarehouseCode      string
	Revision           int
	ClaimWindowSeconds int64
	DisposalMethods    []string
	Registrant         string
	RegisteredAt       time.Time
}

// DisposalRuleCatalogueRead 分页列出各仓当前生效的处置规则。空册答空页，不答未配置。
type DisposalRuleCatalogueRead interface {
	ListDisposalRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[DisposalRuleRow], error)
}

// DisposalRuleHistoryRead 按修订号升序列出一个仓的全部处置规则修订。没登记答空切片，不答 404。
type DisposalRuleHistoryRead interface {
	ListDisposalRuleRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
	) ([]DisposalRuleRow, error)
}
