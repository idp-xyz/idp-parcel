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

// ExternalFundsFacts 实现 ports.ExternalFundsFactStore（写入代数同 ADR-0031）。
// 同一事实引用只采用一次。
type ExternalFundsFacts struct {
	db *bentopg.DB
}

func NewExternalFundsFacts(db *bentopg.DB) (*ExternalFundsFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ExternalFundsFacts{db: db}, nil
}

// FindByKey 按（租户+事实）取回。否定结果只回 false。读回经重建门复验更正两半。
func (repository *ExternalFundsFacts) FindByKey(
	ctx context.Context,
	key ports.FundsFactKey,
) (ports.FundsFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}

	var source, kindName, currency, version, digest string
	var amount int64
	var occurredAt, recordedAt time.Time
	var corrects *string
	var correctedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT source_ref, kind, currency, amount_minor, version, occurred_at,
		        corrects, corrected_at, content_digest, recorded_at
		   FROM settlement_accounting.external_funds_fact
		  WHERE tenant_id = $1
		    AND fact_id = $2`,
		key.TenantID.String(),
		key.Fact.String(),
	).Scan(&source, &kindName, &currency, &amount, &version, &occurredAt,
		&corrects, &correctedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FundsFactRecord{}, false, nil
	}
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}

	sourceRef, err := domain.NewFundsSourceRegistrationReference(source)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	kind, err := fundsFactKindFrom(kindName)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	versionRef, err := domain.NewFundsFactVersion(version)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	spec := domain.RehydrateExternalFundsFactSpec{
		Fact:        key.Fact,
		Source:      sourceRef,
		Kind:        kind,
		Currency:    currencyCode,
		AmountMinor: amount,
		Version:     versionRef,
		OccurredAt:  occurredAt,
	}
	if corrects != nil {
		spec.Corrects, err = domain.NewFundsFactVersion(*corrects)
		if err != nil {
			return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = *correctedAt
	}
	fact, err := domain.RehydrateExternalFundsFact(spec)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	return ports.FundsFactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次资金事实采用。同引用已有记录时答`已采用`，不覆盖先到者。
func (repository *ExternalFundsFacts) Save(
	ctx context.Context,
	record ports.FundsFactRecord,
) (ports.FundsFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FundsFactSaveOutcomeInvalid, fmt.Errorf("save external funds fact: %w", err)
	}

	currency, amount := record.Fact.Amount()
	corrects, hasCorrects := record.Fact.Corrects()
	correctedAt, _ := record.Fact.CorrectedAt()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, corrects, corrected_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Fact.Source().String(),
		record.Fact.Kind().String(),
		currency.String(),
		amount,
		record.Fact.Version().String(),
		record.Fact.OccurredAt().UTC(),
		optionalRef(corrects, hasCorrects),
		optionalTime(correctedAt, hasCorrects),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.FundsFactSaveOutcomeInvalid, fmt.Errorf("save external funds fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FundsFactAlreadyAdopted, nil
	}
	return ports.FundsFactSaved, nil
}

func fundsFactKindFrom(raw string) (domain.FundsFactKind, error) {
	switch raw {
	case domain.FundsReceiptConfirmed.String():
		return domain.FundsReceiptConfirmed, nil
	case domain.FundsPaymentFailed.String():
		return domain.FundsPaymentFailed, nil
	case domain.FundsReturned.String():
		return domain.FundsReturned, nil
	default:
		return 0, fmt.Errorf("unknown funds fact kind %q", raw)
	}
}

func optionalTime(value time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	utc := value.UTC()
	return &utc
}
