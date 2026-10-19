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

// DepartureGateSignalRules 实现 ports.DepartureGateSignalRuleView：本租户的出发门禁阻断按哪条已登记的规则形成信号（0027）。
//
// 规则内容属租户取值（`PAR-VIS-04` 待提供），实现不属：空表时如实交回「未配置」，阻断那一件照常进投影、不形成信号，不虚构
// 一种异常类型。依赖调不通才作为 error 返回。租户在装配期固定，理由同本包其余只读视图。
type DepartureGateSignalRules struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewDepartureGateSignalRules(db *bentopg.DB, tenant domain.TenantID) (*DepartureGateSignalRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &DepartureGateSignalRules{db: db, tenant: tenant}, nil
}

var _ ports.DepartureGateSignalRuleView = (*DepartureGateSignalRules)(nil)

func (view *DepartureGateSignalRules) DepartureGateSignalRule(ctx context.Context) (ports.DepartureGateSignalRule, bool, error) {
	if view.tenant.String() == "" {
		return ports.DepartureGateSignalRule{}, false, nil
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DepartureGateSignalRule{}, false, fmt.Errorf("departure gate signal rule: %w", err)
	}

	var kindRaw, ruleRaw, confidenceRaw string
	err = querier.QueryRow(ctx,
		`SELECT signal_kind, rule_version, confidence_ref
		   FROM visibility_exception.departure_gate_signal_rule
		  WHERE tenant_id = $1`,
		view.tenant.String(),
	).Scan(&kindRaw, &ruleRaw, &confidenceRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DepartureGateSignalRule{}, false, nil
	}
	if err != nil {
		return ports.DepartureGateSignalRule{}, false, fmt.Errorf("departure gate signal rule: %w", err)
	}

	var rule ports.DepartureGateSignalRule
	if rule.Kind, err = domain.NewExceptionSignalKindReference(kindRaw); err != nil {
		return ports.DepartureGateSignalRule{}, false, fmt.Errorf("departure gate signal rule: %w", err)
	}
	if rule.Rule, err = domain.NewSignalRuleVersionReference(ruleRaw); err != nil {
		return ports.DepartureGateSignalRule{}, false, fmt.Errorf("departure gate signal rule: %w", err)
	}
	if rule.Confidence, err = domain.NewConfidenceReference(confidenceRaw); err != nil {
		return ports.DepartureGateSignalRule{}, false, fmt.Errorf("departure gate signal rule: %w", err)
	}
	return rule, true, nil
}

var _ ports.DepartureGateSignalRuleRegistry = (*CatalogRegistrar)(nil)

// RegisterDepartureGateSignalRule 登记本租户的出发门禁阻断信号规则。一租户一条、不可覆盖：撞既有行交回 AlreadyRegistered
// （ON CONFLICT DO NOTHING，事务保持可用）——理由写在 0027 头注。
func (registrar *CatalogRegistrar) RegisterDepartureGateSignalRule(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.DepartureGateSignalRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register departure gate signal rule: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.departure_gate_signal_rule
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
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register departure gate signal rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CatalogVersionAlreadyRegistered, nil
	}
	return ports.CatalogVersionRegistered, nil
}
