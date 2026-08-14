package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AllocationRuleApplicability 实现 ports.AllocationRuleView：来源金额身份 → 采用的
// 分摊规则版本。
//
// 表里只有版本引用，没有分摊范围、权重依据或尾差处理。规则内容归声明它的那一侧
// （`PAR-SET-06`），这里登的是「这条来源用哪一版」；把内容抄一份进来就是第二处定义，
// 两处一旦不一致，分出去的钱按哪份算都说不清。
//
// 无行即`未配置`：交回 found=false，编排保持来源金额未分摊。它绝不退化成默认比例、
// 默认分母或平均分摊（AT-SA-123 点名的三样）。
type AllocationRuleApplicability struct {
	db *bentopg.DB
}

func NewAllocationRuleApplicability(db *bentopg.DB) (*AllocationRuleApplicability, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AllocationRuleApplicability{db: db}, nil
}

var _ ports.AllocationRuleView = (*AllocationRuleApplicability)(nil)

func (repository *AllocationRuleApplicability) LoadAllocationRule(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.AllocationSourceReference,
) (domain.AllocationRuleVersionReference, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.AllocationRuleVersionReference{}, false, fmt.Errorf("load allocation rule: %w", err)
	}

	var ruleVersion string
	err = querier.QueryRow(ctx,
		`SELECT rule_version
		   FROM settlement_accounting.allocation_rule_applicability
		  WHERE tenant_id = $1
		    AND source_ref = $2`,
		tenant.String(),
		source.String(),
	).Scan(&ruleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AllocationRuleVersionReference{}, false, nil
	}
	if err != nil {
		return domain.AllocationRuleVersionReference{}, false, fmt.Errorf("load allocation rule: %w", err)
	}

	reference, err := domain.NewAllocationRuleVersionReference(ruleVersion)
	if err != nil {
		return domain.AllocationRuleVersionReference{}, false, fmt.Errorf("load allocation rule: %w", err)
	}
	return reference, true, nil
}

// Register 登记一条来源采用哪一版分摊规则。同键已有行时交回`已登记`且不覆盖——换规则
// 版本会改变已形成分摊的可解释性，那是一次要留痕的决定，不是一次静默的配置更新
// （AT-SA-126：规则版本更正保留原分摊、形成新版本和差额）。
func (repository *AllocationRuleApplicability) Register(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.AllocationSourceReference,
	ruleVersion domain.AllocationRuleVersionReference,
	registeredAt time.Time,
) (RegisterOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("register allocation rule: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.allocation_rule_applicability
			(tenant_id, source_ref, rule_version, registered_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		source.String(),
		ruleVersion.String(),
		registeredAt.UTC(),
	)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("register allocation rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AlreadyRegistered, nil
	}
	return Registered, nil
}
