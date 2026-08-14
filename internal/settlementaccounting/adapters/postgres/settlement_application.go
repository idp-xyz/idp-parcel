package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SettlementApplications 实现 ports.SettlementApplicationStore。Save 登记；Replace
// 只写撤销两列——分配在 INSERT 里冻结。
type SettlementApplications struct {
	db *bentopg.DB
}

func NewSettlementApplications(db *bentopg.DB) (*SettlementApplications, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SettlementApplications{db: db}, nil
}

type allocationRow struct {
	Mapping     string `json:"mapping"`
	TargetKind  string `json:"targetKind"`
	Target      string `json:"target"`
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amountMinor"`
}

// FindByKey 按（租户+核销）取回。否定结果只回 false。读回经重建门复验守恒与撤销两半，
// 不重审映射背书。
func (repository *SettlementApplications) FindByKey(
	ctx context.Context,
	key ports.SettlementApplicationKey,
) (ports.SettlementApplicationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SettlementApplicationRecord{}, false, fmt.Errorf("find settlement application: %w", err)
	}

	var factID, currency, basis, digest string
	var factMinor, appliedMinor int64
	var allocationsJSON []byte
	var appliedAt, recordedAt time.Time
	var reversalBasis *string
	var reversedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT fact_id, currency, fact_minor, applied_minor, allocations, basis,
		        applied_at, reversal_basis, reversed_at, content_digest, recorded_at
		   FROM settlement_accounting.settlement_application
		  WHERE tenant_id = $1
		    AND application_id = $2`,
		key.TenantID.String(),
		key.Application.String(),
	).Scan(&factID, &currency, &factMinor, &appliedMinor, &allocationsJSON, &basis,
		&appliedAt, &reversalBasis, &reversedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SettlementApplicationRecord{}, false, nil
	}
	if err != nil {
		return ports.SettlementApplicationRecord{}, false, fmt.Errorf("find settlement application: %w", err)
	}

	application, err := rebuildSettlementApplication(key.Application, factID, currency, factMinor, appliedMinor, allocationsJSON, basis, appliedAt, reversalBasis, reversedAt)
	if err != nil {
		return ports.SettlementApplicationRecord{}, false, fmt.Errorf("find settlement application: %w", err)
	}
	return ports.SettlementApplicationRecord{
		Key:           key,
		ContentDigest: digest,
		Application:   application,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次核销。同标识已有记录时答`已核销`，不覆盖先到者。
func (repository *SettlementApplications) Save(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
) (ports.SettlementApplicationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SettlementApplicationSaveOutcomeInvalid, fmt.Errorf("save settlement application: %w", err)
	}

	allocationsJSON, err := marshalAllocations(record.Application.Allocations())
	if err != nil {
		return ports.SettlementApplicationSaveOutcomeInvalid, fmt.Errorf("save settlement application: %w", err)
	}
	reversalBasis, reversedAt, reversed := record.Application.Reversed()

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, reversal_basis, reversed_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Application.String(),
		record.Application.Fact().String(),
		record.Application.Currency().String(),
		record.Application.AppliedMinor()+record.Application.RemainderMinor(),
		record.Application.AppliedMinor(),
		allocationsJSON,
		record.Application.Basis().String(),
		record.Application.AppliedAt().UTC(),
		optionalRef(reversalBasis, reversed),
		optionalTime(reversedAt, reversed),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.SettlementApplicationSaveOutcomeInvalid, fmt.Errorf("save settlement application: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SettlementApplicationAlreadyApplied, nil
	}
	return ports.SettlementApplicationSaved, nil
}

// Replace 只落撤销两列。WHERE 要求尚未撤销——并发第二撤与行不存在同样答 false。
func (repository *SettlementApplications) Replace(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace settlement application: %w", err)
	}

	reversalBasis, reversedAt, reversed := record.Application.Reversed()
	if !reversed {
		return false, fmt.Errorf("replace settlement application: replace requires a reversal")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.settlement_application
		    SET reversal_basis = $3, reversed_at = $4, recorded_at = $5
		  WHERE tenant_id = $1
		    AND application_id = $2
		    AND reversed_at IS NULL`,
		record.Key.TenantID.String(),
		record.Key.Application.String(),
		reversalBasis.String(),
		reversedAt.UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("replace settlement application: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func rebuildSettlementApplication(
	id domain.ApplicationReference,
	factID, currency string,
	factMinor, appliedMinor int64,
	allocationsJSON []byte,
	basis string,
	appliedAt time.Time,
	reversalBasis *string,
	reversedAt *time.Time,
) (domain.SettlementApplication, error) {
	factRef, err := domain.NewFundsFactReference(factID)
	if err != nil {
		return domain.SettlementApplication{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.SettlementApplication{}, err
	}
	basisRef, err := domain.NewApplicationBasisReference(basis)
	if err != nil {
		return domain.SettlementApplication{}, err
	}
	allocations, err := unmarshalAllocations(allocationsJSON)
	if err != nil {
		return domain.SettlementApplication{}, err
	}
	spec := domain.RehydrateSettlementApplicationSpec{
		Application:  id,
		Fact:         factRef,
		Currency:     currencyCode,
		FactMinor:    factMinor,
		Allocations:  allocations,
		AppliedMinor: appliedMinor,
		Basis:        basisRef,
		AppliedAt:    appliedAt,
	}
	if reversalBasis != nil {
		spec.ReversalBasis, err = domain.NewApplicationBasisReference(*reversalBasis)
		if err != nil {
			return domain.SettlementApplication{}, err
		}
	}
	if reversedAt != nil {
		spec.ReversedAt = *reversedAt
	}
	return domain.RehydrateSettlementApplication(spec)
}

func marshalAllocations(allocations []domain.SettlementAllocation) ([]byte, error) {
	rows := make([]allocationRow, 0, len(allocations))
	for _, allocation := range allocations {
		rows = append(rows, allocationRow{
			Mapping:     allocation.Mapping.String(),
			TargetKind:  allocation.TargetKind.String(),
			Target:      allocation.Target.String(),
			Direction:   allocation.Direction.String(),
			AmountMinor: allocation.AmountMinor,
		})
	}
	return json.Marshal(rows)
}

func unmarshalAllocations(raw []byte) ([]domain.SettlementAllocation, error) {
	var rows []allocationRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	allocations := make([]domain.SettlementAllocation, 0, len(rows))
	for _, row := range rows {
		mapping, err := domain.NewMappingReference(row.Mapping)
		if err != nil {
			return nil, err
		}
		targetKind, err := settlementTargetKindFrom(row.TargetKind)
		if err != nil {
			return nil, err
		}
		target, err := domain.NewSettlementTargetReference(row.Target)
		if err != nil {
			return nil, err
		}
		direction, err := allocationDirectionFrom(row.Direction)
		if err != nil {
			return nil, err
		}
		allocations = append(allocations, domain.SettlementAllocation{
			Mapping:     mapping,
			TargetKind:  targetKind,
			Target:      target,
			Direction:   direction,
			AmountMinor: row.AmountMinor,
		})
	}
	return allocations, nil
}

func allocationDirectionFrom(raw string) (domain.AllocationDirection, error) {
	switch raw {
	case domain.AllocationDebit.String():
		return domain.AllocationDebit, nil
	case domain.AllocationCredit.String():
		return domain.AllocationCredit, nil
	default:
		return 0, fmt.Errorf("unknown allocation direction %q", raw)
	}
}
