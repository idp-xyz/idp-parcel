package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 结算政策册的持久化面（ADR-0044）。装载不在这里而在 CommercialPublications.LoadForScope。

type scannedSettlementPolicy struct {
	method       *string
	legalEntity  *string
	counterparty *string
	contract     *string
	chargeScope  *string
	currency     *string
	startsAt     *time.Time
	endsAt       *time.Time
}

func (row scannedSettlementPolicy) present() bool {
	return row.method != nil
}

func (row scannedSettlementPolicy) complete() bool {
	return row.method != nil && row.legalEntity != nil && row.counterparty != nil &&
		row.contract != nil && row.chargeScope != nil && row.currency != nil && row.startsAt != nil
}

// SaveSettlementPolicy 登记一份结算政策版本的方式与六维适用范围。撞键不覆盖。
func (repository *CommercialPublications) SaveSettlementPolicy(
	ctx context.Context,
	policy domain.SettlementPolicy,
) (ports.SettlementPolicySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SettlementPolicySaveOutcomeInvalid, fmt.Errorf("save settlement policy: %w", err)
	}

	version := policy.Version()
	method := policy.Method().String()
	applicability := policy.Applicability()
	if method == "" {
		return ports.SettlementPolicySaveOutcomeInvalid,
			fmt.Errorf("save settlement policy: 结算方式缺失，未经 NewSettlementPolicy 构造的政策不入册")
	}

	var endsAt *time.Time
	if end, bounded := applicability.Effective().EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}
	startsAt := applicability.Effective().StartsAt().UTC()

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_settlement_policy
			(tenant_id, object_kind, object_id, version_label,
			 method, legal_entity_ref, counterparty_ref, contract_label,
			 charge_scope_ref, currency_code, effective_starts_at, effective_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		method,
		applicability.LegalEntity().String(),
		applicability.Counterparty().String(),
		applicability.Contract().String(),
		applicability.ChargeScope().String(),
		applicability.Currency().String(),
		startsAt,
		endsAt,
	)
	if err != nil {
		return ports.SettlementPolicySaveOutcomeInvalid, fmt.Errorf("save settlement policy: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.SettlementPolicySaved, nil
	}

	var existingMethod, existingLegal, existingCounterparty, existingContract, existingCharge, existingCurrency string
	var existingStarts time.Time
	var existingEnds *time.Time
	err = executor.QueryRow(ctx,
		`SELECT method, legal_entity_ref, counterparty_ref, contract_label,
		        charge_scope_ref, currency_code, effective_starts_at, effective_ends_at
		   FROM party_commercial.commercial_settlement_policy
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existingMethod, &existingLegal, &existingCounterparty, &existingContract,
		&existingCharge, &existingCurrency, &existingStarts, &existingEnds)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SettlementPolicySaveOutcomeInvalid, fmt.Errorf("save settlement policy: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.SettlementPolicySaveOutcomeInvalid, fmt.Errorf("save settlement policy: %w", err)
	}
	if existingMethod == method &&
		existingLegal == applicability.LegalEntity().String() &&
		existingCounterparty == applicability.Counterparty().String() &&
		existingContract == applicability.Contract().String() &&
		existingCharge == applicability.ChargeScope().String() &&
		existingCurrency == applicability.Currency().String() &&
		existingStarts.Equal(startsAt) &&
		sameOptionalTime(existingEnds, endsAt) {
		return ports.SettlementPolicyAlreadyRegistered, nil
	}
	return ports.SettlementPolicyContentConflict, nil
}

func registerSettlementPolicy(
	registry *domain.CommercialRegistry,
	version domain.CommercialVersion,
	row scannedSettlementPolicy,
) error {
	if !row.present() {
		return nil
	}
	if !row.complete() {
		return fmt.Errorf("settlement policy row is incomplete")
	}
	method, err := settlementMethodFrom(*row.method)
	if err != nil {
		return err
	}
	legalEntity, err := domain.NewLegalEntityReference(*row.legalEntity)
	if err != nil {
		return err
	}
	counterparty, err := domain.NewCounterpartyReference(*row.counterparty)
	if err != nil {
		return err
	}
	contract, err := domain.NewCommercialVersionLabel(*row.contract)
	if err != nil {
		return err
	}
	chargeScope, err := domain.NewChargeScopeReference(*row.chargeScope)
	if err != nil {
		return err
	}
	currency, err := domain.NewCurrencyCode(*row.currency)
	if err != nil {
		return err
	}
	endsAt := time.Time{}
	if row.endsAt != nil {
		endsAt = *row.endsAt
	}
	interval, err := domain.NewEffectiveInterval(*row.startsAt, endsAt)
	if err != nil {
		return err
	}
	applicability, err := domain.NewSettlementApplicability(
		legalEntity, counterparty, contract, chargeScope, currency, interval)
	if err != nil {
		return err
	}
	if version.Status() != domain.CommercialVersionEffective {
		return nil
	}
	policy, err := domain.NewSettlementPolicy(version, method, applicability)
	if err != nil {
		return err
	}
	registry.RegisterSettlementPolicy(policy)
	return nil
}

func settlementMethodFrom(raw string) (domain.SettlementMethod, error) {
	switch raw {
	case domain.PrepaidMethod.String():
		return domain.PrepaidMethod, nil
	case domain.TermsMethod.String():
		return domain.TermsMethod, nil
	default:
		return domain.SettlementMethodInvalid, fmt.Errorf("unknown settlement method %q", raw)
	}
}
