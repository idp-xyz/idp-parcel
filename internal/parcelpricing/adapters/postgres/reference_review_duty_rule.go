package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ReferenceReviewDutyRules 实现 ports.ReferenceReviewDutyRuleRegistry：一租户一条参考复核职责规则（0014；ADR-0262 决定一）。
// 行属实例半边，写口给受控登记口、装配与测试用。
//
// 行只增不改，所以复核读规则不加锁：同一租户读到的那一条在复核那笔事务里不会变。
type ReferenceReviewDutyRules struct {
	db *bentopg.DB
}

var _ ports.ReferenceReviewDutyRuleRegistry = (*ReferenceReviewDutyRules)(nil)

func NewReferenceReviewDutyRules(db *bentopg.DB) (*ReferenceReviewDutyRules, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceReviewDutyRules{db: db}, nil
}

// LoadReferenceReviewDutyRule 读该租户的规则；没有行即 found=false，由复核门答`未配置`。在复核那笔事务里调时读的是同一笔。
func (rules *ReferenceReviewDutyRules) LoadReferenceReviewDutyRule(
	ctx context.Context,
	tenant domain.TenantID,
) (domain.ReferenceReviewDutyRule, bool, error) {
	querier, err := rules.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReferenceReviewDutyRule{}, false, fmt.Errorf("load reference review duty rule: %w", err)
	}
	var distinct bool
	var requiredGrant *string
	err = querier.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_grant
		   FROM parcel_pricing.reference_review_duty_rule
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&distinct, &requiredGrant)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReferenceReviewDutyRule{}, false, nil
	}
	if err != nil {
		return domain.ReferenceReviewDutyRule{}, false, fmt.Errorf("load reference review duty rule: %w", err)
	}
	rule, err := referenceReviewDutyRuleOf(tenant, distinct, requiredGrant)
	if err != nil {
		return domain.ReferenceReviewDutyRule{}, false, fmt.Errorf("load reference review duty rule: %w", err)
	}
	return rule, true, nil
}

// SaveReferenceReviewDutyRule 登记一条规则：同租户首登入册；再登同内容是重放，异内容答冲突、原行不被顶替。
//
// 插入与撞键后的读回走同一笔环境事务（RequireExecutor）：并发的两次首登里后到的那一笔停在主键上，等先到者提交后撞键、
// 再读到先到者那一行比内容——不会两笔都答已入册，也不会读到一行还没提交的规则。
func (rules *ReferenceReviewDutyRules) SaveReferenceReviewDutyRule(
	ctx context.Context,
	rule domain.ReferenceReviewDutyRule,
) (ports.ReferenceReviewDutyRuleSaveOutcome, error) {
	executor, err := rules.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceReviewDutyRuleSaveOutcomeInvalid, fmt.Errorf("save reference review duty rule: %w", err)
	}
	if rule.Tenant().String() == "" {
		return ports.ReferenceReviewDutyRuleSaveOutcomeInvalid, fmt.Errorf("save reference review duty rule: %w", domain.ErrInvalidReferenceReviewDutyRule)
	}
	var requiredGrant *string
	if grant, required := rule.RequiredGrant(); required {
		name := grant.String()
		requiredGrant = &name
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_review_duty_rule (tenant_id, requires_distinct_subjects, required_grant)
		 VALUES ($1, $2, $3)
		 ON CONFLICT ON CONSTRAINT reference_review_duty_rule_pkey DO NOTHING`,
		rule.Tenant().String(), rule.RequiresDistinctSubjects(), requiredGrant,
	)
	if err != nil {
		return ports.ReferenceReviewDutyRuleSaveOutcomeInvalid, fmt.Errorf("save reference review duty rule: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ReferenceReviewDutyRuleSaved, nil
	}

	var distinct bool
	var recordedGrant *string
	if err := executor.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_grant
		   FROM parcel_pricing.reference_review_duty_rule
		  WHERE tenant_id = $1`,
		rule.Tenant().String(),
	).Scan(&distinct, &recordedGrant); err != nil {
		return ports.ReferenceReviewDutyRuleSaveOutcomeInvalid, fmt.Errorf("save reference review duty rule: 撞键后读册上那一行：%w", err)
	}
	if distinct == rule.RequiresDistinctSubjects() && sameOptional(recordedGrant, requiredGrant) {
		return ports.ReferenceReviewDutyRuleAlreadyRegistered, nil
	}
	return ports.ReferenceReviewDutyRuleContentConflict, nil
}

func referenceReviewDutyRuleOf(tenant domain.TenantID, distinct bool, requiredGrant *string) (domain.ReferenceReviewDutyRule, error) {
	var grant domain.OperatorGrant
	if requiredGrant != nil {
		var err error
		if grant, err = domain.NewOperatorGrant(*requiredGrant); err != nil {
			return domain.ReferenceReviewDutyRule{}, err
		}
	}
	return domain.NewReferenceReviewDutyRule(tenant, distinct, grant)
}
