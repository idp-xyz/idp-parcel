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

// AuditEscalationCeilings 是越权升级金额上限的登记册。空册答 found=false，不默认放行。
type AuditEscalationCeilings struct {
	db *bentopg.DB
}

func NewAuditEscalationCeilings(db *bentopg.DB) (*AuditEscalationCeilings, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AuditEscalationCeilings{db: db}, nil
}

func (repository *AuditEscalationCeilings) SaveAuditEscalationCeiling(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AuditEscalationCeilingRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save audit escalation ceiling: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT audit_escalation_ceiling"); err != nil {
		return 0, fmt.Errorf("save audit escalation ceiling: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.audit_escalation_ceiling (
			tenant_id, supplier_id, legal_entity_id, currency, ceiling_minor, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(),
		registration.Supplier().String(),
		registration.LegalEntity().String(),
		registration.Currency().String(),
		registration.Ceiling().LimitMinor(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save audit escalation ceiling: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT audit_escalation_ceiling"); err != nil {
		return 0, fmt.Errorf("save audit escalation ceiling: %w", err)
	}
	var limit int64
	err = executor.QueryRow(ctx, `
		SELECT ceiling_minor
		  FROM settlement_accounting.audit_escalation_ceiling
		 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3 AND currency = $4`,
		tenant.String(), registration.Supplier().String(), registration.LegalEntity().String(), registration.Currency().String(),
	).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CatalogueConflict, nil
	}
	if err != nil {
		return 0, fmt.Errorf("save audit escalation ceiling: %w", err)
	}
	if limit != registration.Ceiling().LimitMinor() {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *AuditEscalationCeilings) LoadAuditEscalationCeiling(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	legalEntity domain.LegalEntityReference,
	currency domain.CurrencyCode,
) (domain.AuditEscalationCeiling, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.AuditEscalationCeiling{}, false, fmt.Errorf("load audit escalation ceiling: %w", err)
	}
	var limit int64
	err = querier.QueryRow(ctx, `
		SELECT ceiling_minor
		  FROM settlement_accounting.audit_escalation_ceiling
		 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3 AND currency = $4`,
		tenant.String(), supplier.String(), legalEntity.String(), currency.String(),
	).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuditEscalationCeiling{}, false, nil
	}
	if err != nil {
		return domain.AuditEscalationCeiling{}, false, fmt.Errorf("load audit escalation ceiling: %w", err)
	}
	ceiling, err := domain.NewAuditEscalationCeiling(limit)
	if err != nil {
		return domain.AuditEscalationCeiling{}, false, fmt.Errorf("load audit escalation ceiling: %w", err)
	}
	return ceiling, true, nil
}
