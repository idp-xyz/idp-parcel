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

// AdvanceRecoveries 实现 ports.AdvanceRecoveryStore（写入代数同 ADR-0031）。
// 同一回收标识只形成一次。
type AdvanceRecoveries struct {
	db *bentopg.DB
}

func NewAdvanceRecoveries(db *bentopg.DB) (*AdvanceRecoveries, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AdvanceRecoveries{db: db}, nil
}

// FindByKey 按（租户+回收）取回。否定结果只回 false。读回经重建门复验回收自身形状，
// 不重审评估是否成立。
func (repository *AdvanceRecoveries) FindByKey(
	ctx context.Context,
	key ports.AdvanceRecoveryKey,
) (ports.AdvanceRecoveryRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}

	var assessmentID, customer, contractBasis, account, currency, digest string
	var amount int64
	var formedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT assessment_id, customer_ref, contract_basis, account_id, currency,
		        amount_minor, formed_at, content_digest, recorded_at
		   FROM settlement_accounting.advance_recovery
		  WHERE tenant_id = $1
		    AND recovery_id = $2`,
		key.TenantID.String(),
		key.Recovery.String(),
	).Scan(&assessmentID, &customer, &contractBasis, &account, &currency,
		&amount, &formedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AdvanceRecoveryRecord{}, false, nil
	}
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}

	assessmentRef, err := domain.NewAdvanceAssessmentID(assessmentID)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	customerRef, err := domain.NewRecoveryCustomerReference(customer)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	contractRef, err := domain.NewContractResponsibilityReference(contractBasis)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	accountRef, err := domain.NewSettlementAccountID(account)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	recovery, err := domain.RehydrateCustomerAdvanceRecovery(domain.RehydrateCustomerAdvanceRecoverySpec{
		ID:            key.Recovery,
		Assessment:    assessmentRef,
		Customer:      customerRef,
		ContractBasis: contractRef,
		Account:       accountRef,
		Currency:      currencyCode,
		AmountMinor:   amount,
		FormedAt:      formedAt,
	})
	if err != nil {
		return ports.AdvanceRecoveryRecord{}, false, fmt.Errorf("find advance recovery: %w", err)
	}
	return ports.AdvanceRecoveryRecord{
		Key:           key,
		ContentDigest: digest,
		Recovery:      recovery,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次客户代垫回收。同标识已有记录时答`已形成`，不覆盖先到者。
func (repository *AdvanceRecoveries) Save(
	ctx context.Context,
	record ports.AdvanceRecoveryRecord,
) (ports.AdvanceRecoverySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AdvanceRecoverySaveOutcomeInvalid, fmt.Errorf("save advance recovery: %w", err)
	}

	currency, amount := record.Recovery.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.advance_recovery
			(tenant_id, recovery_id, assessment_id, customer_ref, contract_basis,
			 account_id, currency, amount_minor, formed_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Recovery.String(),
		record.Recovery.Assessment().String(),
		record.Recovery.Customer().String(),
		record.Recovery.ContractBasis().String(),
		record.Recovery.Account().String(),
		currency.String(),
		amount,
		record.Recovery.FormedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AdvanceRecoverySaveOutcomeInvalid, fmt.Errorf("save advance recovery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AdvanceRecoveryAlreadyFormed, nil
	}
	return ports.AdvanceRecoverySaved, nil
}
