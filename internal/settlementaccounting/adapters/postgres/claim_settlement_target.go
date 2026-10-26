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

// ClaimSettlementTargets 实现 ports.ClaimSettlementTargets：资金映射与核销指向的两格金额本体。只读不写——那两本册
// 由 UC-SA-007 唯一形成，映射与核销改的是核销状态与未分配余额，不碰金额行。
type ClaimSettlementTargets struct {
	db *bentopg.DB
}

func NewClaimSettlementTargets(db *bentopg.DB) (*ClaimSettlementTargets, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ClaimSettlementTargets{db: db}, nil
}

var _ ports.ClaimSettlementTargets = (*ClaimSettlementTargets)(nil)

// claimTargetRowQuery 交回读那一条金额本体的语句：客户赔付取赔付金额，追偿认可取认可金额——应追偿金额不是这一格的
// 目标（domain.TargetRecoveryReceivable 头注）。键都带租户：他租户的行与不存在同样读不到。
func claimTargetRowQuery(kind domain.SettlementTargetKind) (string, error) {
	switch kind {
	case domain.TargetCustomerCompensation:
		return `SELECT currency, amount_minor
		          FROM settlement_accounting.customer_claim_amount
		         WHERE tenant_id = $1
		           AND amount_id = $2`, nil
	case domain.TargetRecoveryReceivable:
		return `SELECT currency, acknowledged_minor
		          FROM settlement_accounting.recovery_acknowledgement
		         WHERE tenant_id = $1
		           AND acknowledgement_id = $2`, nil
	default:
		return "", fmt.Errorf("settlement target kind %q does not point at a claim amount", kind)
	}
}

func (repository *ClaimSettlementTargets) FindClaimTarget(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.ClaimTargetKey,
) (bool, error) {
	query, err := claimTargetRowQuery(key.Kind)
	if err != nil {
		return false, fmt.Errorf("find claim target: %w", err)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("find claim target: %w", err)
	}
	var currency string
	var amount int64
	err = querier.QueryRow(ctx, query, tenant.String(), key.Target.String()).Scan(&currency, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find claim target: %w", err)
	}
	return true, nil
}

// LockClaimTarget 分两句：先 FOR UPDATE 锁金额行，再另起一句求已核销。并成一句时，等锁的那一路在锁放开后只重验被锁的
// 那一行，子查询仍是它开始等之前的快照，读不到先到那笔刚提交的核销；读已提交级别下另起的一句才拿新快照。
func (repository *ClaimSettlementTargets) LockClaimTarget(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.ClaimTargetKey,
) (domain.ClaimTargetStanding, bool, error) {
	query, err := claimTargetRowQuery(key.Kind)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}

	var currencyCode string
	var amount int64
	err = executor.QueryRow(ctx, query+` FOR UPDATE`, tenant.String(), key.Target.String()).Scan(&currencyCode, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClaimTargetStanding{}, false, nil
	}
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}

	var applied int64
	err = executor.QueryRow(ctx,
		`SELECT COALESCE(SUM(
		            CASE elem->>'direction'
		                WHEN 'DEBIT' THEN (elem->>'amountMinor')::bigint
		                WHEN 'CREDIT' THEN -(elem->>'amountMinor')::bigint
		                ELSE 0
		            END), 0)
		   FROM settlement_accounting.settlement_application AS app
		  CROSS JOIN LATERAL jsonb_array_elements(app.allocations) AS elem
		  WHERE app.tenant_id = $1
		    AND app.reversal_basis IS NULL
		    AND elem->>'targetKind' = $2
		    AND elem->>'target' = $3`,
		tenant.String(), key.Kind.String(), key.Target.String(),
	).Scan(&applied)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}

	currency, err := domain.NewCurrencyCode(currencyCode)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}
	standing, err := domain.NewClaimTargetStanding(currency, amount, applied)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, fmt.Errorf("lock claim target: %w", err)
	}
	return standing, true, nil
}
