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

// FundsMappings 实现 ports.FundsMappingStore（写入代数同 ADR-0031）。
// 同一映射标识只登一次。
type FundsMappings struct {
	db *bentopg.DB
}

func NewFundsMappings(db *bentopg.DB) (*FundsMappings, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &FundsMappings{db: db}, nil
}

// FindByKey 按（租户+映射）取回。否定结果只回 false。读回经重建门，不重审事实种类。
func (repository *FundsMappings) FindByKey(
	ctx context.Context,
	key ports.FundsMappingKey,
) (ports.FundsMappingRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}

	var factID, targetKindName, target, basis, digest string
	var mappedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT fact_id, target_kind, target_ref, basis, mapped_at, content_digest, recorded_at
		   FROM settlement_accounting.funds_mapping
		  WHERE tenant_id = $1
		    AND mapping_id = $2`,
		key.TenantID.String(),
		key.Mapping.String(),
	).Scan(&factID, &targetKindName, &target, &basis, &mappedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FundsMappingRecord{}, false, nil
	}
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}

	factRef, err := domain.NewFundsFactReference(factID)
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}
	targetKind, err := settlementTargetKindFrom(targetKindName)
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}
	targetRef, err := domain.NewSettlementTargetReference(target)
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}
	basisRef, err := domain.NewMappingBasisReference(basis)
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}
	mapping, err := domain.RehydrateFundsMapping(domain.RehydrateFundsMappingSpec{
		Mapping:    key.Mapping,
		Fact:       factRef,
		TargetKind: targetKind,
		Target:     targetRef,
		Basis:      basisRef,
		MappedAt:   mappedAt,
	})
	if err != nil {
		return ports.FundsMappingRecord{}, false, fmt.Errorf("find funds mapping: %w", err)
	}
	return ports.FundsMappingRecord{
		Key:           key,
		ContentDigest: digest,
		Mapping:       mapping,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次资金映射。同标识已有记录时答`已有记录`，不覆盖先到者。
func (repository *FundsMappings) Save(
	ctx context.Context,
	record ports.FundsMappingRecord,
) (ports.FundsMappingSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FundsMappingSaveOutcomeInvalid, fmt.Errorf("save funds mapping: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.funds_mapping
			(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis,
			 mapped_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Mapping.String(),
		record.Mapping.Fact().String(),
		record.Mapping.TargetKind().String(),
		record.Mapping.Target().String(),
		record.Mapping.Basis().String(),
		record.Mapping.MappedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.FundsMappingSaveOutcomeInvalid, fmt.Errorf("save funds mapping: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FundsMappingAlreadyRecorded, nil
	}
	return ports.FundsMappingSaved, nil
}

func settlementTargetKindFrom(raw string) (domain.SettlementTargetKind, error) {
	switch raw {
	case domain.TargetStatement.String():
		return domain.TargetStatement, nil
	case domain.TargetPayable.String():
		return domain.TargetPayable, nil
	case domain.TargetCreditNote.String():
		return domain.TargetCreditNote, nil
	default:
		return 0, fmt.Errorf("unknown settlement target kind %q", raw)
	}
}
