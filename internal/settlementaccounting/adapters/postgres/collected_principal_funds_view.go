package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// CollectedPrincipalFundsView 实现 ports.CollectedPrincipalFundsView：给 collection-remittance 读一条资金事实的链头
// 与它是否被整条认作代收本金（ADR-0330）。与 ExternalFundsFacts 分型：读方拿不到 Save。
type CollectedPrincipalFundsView struct {
	db *bentopg.DB
}

func NewCollectedPrincipalFundsView(db *bentopg.DB) (*CollectedPrincipalFundsView, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CollectedPrincipalFundsView{db: db}, nil
}

var _ ports.CollectedPrincipalFundsView = (*CollectedPrincipalFundsView)(nil)

// LockCollectedPrincipalFact 先以 FOR SHARE 锁事实身份行（external_funds_fact），再另起两句读链头与认作状态。
// 锁身份行而不锁链头版本行的理由同 SettlementApplications.LockFundsFactApplications：身份行一条事实一行、终身不换，
// 链头随每次更正换一行。共享锁而不是 FOR UPDATE：读方之间不必排队（几条到账同时引用同一条事实时，它们的先后由
// collection-remittance 自己的事实锁排），只需要挡住结算一侧的更正、映射与核销。两句分开的理由同那一处：
// 等锁的那一路另起的读句读得到先到那笔已提交的更正。
func (view *CollectedPrincipalFundsView) LockCollectedPrincipalFact(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.FundsFactReference,
) (ports.CollectedPrincipalFundsFact, bool, error) {
	executor, err := view.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CollectedPrincipalFundsFact{}, false, fmt.Errorf("lock collected principal fact: %w", err)
	}
	var locked int
	err = executor.QueryRow(ctx,
		`SELECT 1 FROM settlement_accounting.external_funds_fact
		  WHERE tenant_id = $1
		    AND fact_id = $2
		    FOR SHARE`,
		tenant.String(),
		fact.String(),
	).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CollectedPrincipalFundsFact{}, false, nil
	}
	if err != nil {
		return ports.CollectedPrincipalFundsFact{}, false, fmt.Errorf("lock collected principal fact: %w", err)
	}

	key := ports.FundsFactKey{TenantID: tenant, Fact: fact}
	record, found, err := scanExternalFundsFact(executor.QueryRow(ctx,
		externalFundsFactSelect+`
		  WHERE v.tenant_id = $1
		    AND v.fact_id = $2
		    AND NOT EXISTS (
		        SELECT 1
		          FROM settlement_accounting.external_funds_fact_version AS successor
		         WHERE successor.tenant_id = v.tenant_id
		           AND successor.fact_id = v.fact_id
		           AND successor.corrects = v.version)`,
		tenant.String(),
		fact.String(),
	), key)
	if err != nil {
		return ports.CollectedPrincipalFundsFact{}, false, fmt.Errorf("lock collected principal fact: %w", err)
	}
	if !found {
		// 身份行在而一个版本都没有：首版与身份行同事务落，走到这里是库被改坏了，不当作「没采用」答。
		return ports.CollectedPrincipalFundsFact{}, false, fmt.Errorf(
			"lock collected principal fact: identity row without a version")
	}

	var setAside bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
		        SELECT 1
		          FROM settlement_accounting.funds_mapping
		         WHERE tenant_id = $1
		           AND fact_id = $2
		           AND target_kind = $3)`,
		tenant.String(),
		fact.String(),
		domain.TargetCollectedPrincipal.String(),
	).Scan(&setAside); err != nil {
		return ports.CollectedPrincipalFundsFact{}, false, fmt.Errorf("lock collected principal fact: %w", err)
	}
	return ports.CollectedPrincipalFundsFact{Head: record.Fact, SetAside: setAside}, true, nil
}
