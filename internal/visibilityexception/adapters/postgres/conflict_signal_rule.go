package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ConflictSignalRules 实现 ports.ConflictSignalRuleView：本租户的事实冲突按哪条已登记的
// 规则形成信号（0025）。
//
// 规则内容属实例半边（`PAR-VIS-04` 待提供），实现不属：空表时如实交回「未配置」，编排
// 把冲突记为无适用信号规则，不虚构一种异常类型。依赖调不通才作为 error 返回。租户在
// 装配期固定，理由同本包其余只读视图。
type ConflictSignalRules struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewConflictSignalRules(db *bentopg.DB, tenant domain.TenantID) (*ConflictSignalRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ConflictSignalRules{db: db, tenant: tenant}, nil
}

var _ ports.ConflictSignalRuleView = (*ConflictSignalRules)(nil)

func (view *ConflictSignalRules) ConflictSignalRule(ctx context.Context) (ports.ConflictSignalRule, bool, error) {
	if view.tenant.String() == "" {
		return ports.ConflictSignalRule{}, false, nil
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ConflictSignalRule{}, false, fmt.Errorf("conflict signal rule: %w", err)
	}

	var kindRaw, ruleRaw, confidenceRaw string
	err = querier.QueryRow(ctx,
		`SELECT signal_kind, rule_version, confidence_ref
		   FROM visibility_exception.conflict_signal_rule
		  WHERE tenant_id = $1`,
		view.tenant.String(),
	).Scan(&kindRaw, &ruleRaw, &confidenceRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConflictSignalRule{}, false, nil
	}
	if err != nil {
		return ports.ConflictSignalRule{}, false, fmt.Errorf("conflict signal rule: %w", err)
	}

	var rule ports.ConflictSignalRule
	if rule.Kind, err = domain.NewExceptionSignalKindReference(kindRaw); err != nil {
		return ports.ConflictSignalRule{}, false, fmt.Errorf("conflict signal rule: %w", err)
	}
	if rule.Rule, err = domain.NewSignalRuleVersionReference(ruleRaw); err != nil {
		return ports.ConflictSignalRule{}, false, fmt.Errorf("conflict signal rule: %w", err)
	}
	if rule.Confidence, err = domain.NewConfidenceReference(confidenceRaw); err != nil {
		return ports.ConflictSignalRule{}, false, fmt.Errorf("conflict signal rule: %w", err)
	}
	return rule, true, nil
}

var _ ports.ConflictSignalRuleRegistry = (*CatalogRegistrar)(nil)

// RegisterConflictSignalRule 登记本租户的冲突信号规则。一租户一条、不可覆盖：撞既有行交回
// AlreadyRegistered（ON CONFLICT DO NOTHING，事务保持可用）——理由写在 0025 头注。
func (registrar *CatalogRegistrar) RegisterConflictSignalRule(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ConflictSignalRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register conflict signal rule: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.conflict_signal_rule
			(tenant_id, signal_kind, rule_version, confidence_ref, approved_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		registration.Kind.String(),
		registration.Rule.String(),
		registration.Confidence.String(),
		registration.ApprovedBy,
	)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register conflict signal rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CatalogVersionAlreadyRegistered, nil
	}
	return ports.CatalogVersionRegistered, nil
}
