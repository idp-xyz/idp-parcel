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

// CostAllocations 实现 ports.CostAllocationStore。Save 登记；Replace 换份额/规则/
// 版本回指，不改来源金额与身份。
type CostAllocations struct {
	db *bentopg.DB
}

func NewCostAllocations(db *bentopg.DB) (*CostAllocations, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CostAllocations{db: db}, nil
}

type portionRow struct {
	Target      string `json:"target"`
	AmountMinor int64  `json:"amountMinor"`
}

func (repository *CostAllocations) FindByKey(
	ctx context.Context,
	key ports.AllocationKey,
) (ports.AllocationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AllocationRecord{}, false, fmt.Errorf("find cost allocation: %w", err)
	}

	var source, currency, rule, version, digest string
	var sourceMinor, unallocated int64
	var portionsJSON []byte
	var allocatedAt, recordedAt time.Time
	var corrects *string
	err = querier.QueryRow(ctx,
		`SELECT source_ref, source_minor, currency, rule_ref, portions, unallocated_minor,
		        version, allocated_at, corrects, content_digest, recorded_at
		   FROM settlement_accounting.cost_allocation
		  WHERE tenant_id = $1
		    AND allocation_id = $2`,
		key.TenantID.String(),
		key.Allocation.String(),
	).Scan(&source, &sourceMinor, &currency, &rule, &portionsJSON, &unallocated,
		&version, &allocatedAt, &corrects, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AllocationRecord{}, false, nil
	}
	if err != nil {
		return ports.AllocationRecord{}, false, fmt.Errorf("find cost allocation: %w", err)
	}

	allocation, err := rebuildCostAllocation(key.Allocation, source, sourceMinor, currency, rule, portionsJSON, version, allocatedAt, corrects)
	if err != nil {
		return ports.AllocationRecord{}, false, fmt.Errorf("find cost allocation: %w", err)
	}
	if allocation.UnallocatedMinor() != unallocated {
		return ports.AllocationRecord{}, false, fmt.Errorf("find cost allocation: unallocated mismatch")
	}
	return ports.AllocationRecord{
		Key:           key,
		ContentDigest: digest,
		Allocation:    allocation,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

func (repository *CostAllocations) Save(
	ctx context.Context,
	record ports.AllocationRecord,
) (ports.AllocationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AllocationSaveOutcomeInvalid, fmt.Errorf("save cost allocation: %w", err)
	}
	currency, sourceMinor := record.Allocation.SourceAmount()
	portionsJSON, err := marshalPortions(record.Allocation.Portions())
	if err != nil {
		return ports.AllocationSaveOutcomeInvalid, fmt.Errorf("save cost allocation: %w", err)
	}
	corrects, hasCorrects := record.Allocation.Corrects()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.cost_allocation
			(tenant_id, allocation_id, source_ref, source_minor, currency, rule_ref,
			 portions, unallocated_minor, version, allocated_at, corrects,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Allocation.String(),
		record.Allocation.Source().String(),
		sourceMinor,
		currency.String(),
		record.Allocation.Rule().String(),
		portionsJSON,
		record.Allocation.UnallocatedMinor(),
		record.Allocation.Version().String(),
		record.Allocation.AllocatedAt().UTC(),
		optionalRef(corrects, hasCorrects),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AllocationSaveOutcomeInvalid, fmt.Errorf("save cost allocation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AllocationAlreadyFormed, nil
	}
	return ports.AllocationSaved, nil
}

func (repository *CostAllocations) Replace(
	ctx context.Context,
	record ports.AllocationRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace cost allocation: %w", err)
	}
	corrects, hasCorrects := record.Allocation.Corrects()
	if !hasCorrects {
		return false, fmt.Errorf("replace cost allocation: replace requires a reallocation")
	}
	currency, sourceMinor := record.Allocation.SourceAmount()
	portionsJSON, err := marshalPortions(record.Allocation.Portions())
	if err != nil {
		return false, fmt.Errorf("replace cost allocation: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.cost_allocation
		    SET rule_ref = $3, portions = $4, unallocated_minor = $5, version = $6,
		        allocated_at = $7, corrects = $8, content_digest = $9, recorded_at = $10
		  WHERE tenant_id = $1
		    AND allocation_id = $2
		    AND source_minor = $11
		    AND currency = $12`,
		record.Key.TenantID.String(),
		record.Key.Allocation.String(),
		record.Allocation.Rule().String(),
		portionsJSON,
		record.Allocation.UnallocatedMinor(),
		record.Allocation.Version().String(),
		record.Allocation.AllocatedAt().UTC(),
		corrects.String(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
		sourceMinor,
		currency.String(),
	)
	if err != nil {
		return false, fmt.Errorf("replace cost allocation: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func rebuildCostAllocation(
	id domain.AllocationID,
	source string,
	sourceMinor int64,
	currency, rule string,
	portionsJSON []byte,
	version string,
	allocatedAt time.Time,
	corrects *string,
) (domain.CostAllocation, error) {
	sourceRef, err := domain.NewAllocationSourceReference(source)
	if err != nil {
		return domain.CostAllocation{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.CostAllocation{}, err
	}
	ruleRef, err := domain.NewAllocationRuleVersionReference(rule)
	if err != nil {
		return domain.CostAllocation{}, err
	}
	versionRef, err := domain.NewAllocationVersion(version)
	if err != nil {
		return domain.CostAllocation{}, err
	}
	portions, err := unmarshalPortions(portionsJSON)
	if err != nil {
		return domain.CostAllocation{}, err
	}
	spec := domain.RehydrateCostAllocationSpec{
		ID:          id,
		Source:      sourceRef,
		SourceMinor: sourceMinor,
		Currency:    currencyCode,
		Rule:        ruleRef,
		Portions:    portions,
		Version:     versionRef,
		AllocatedAt: allocatedAt,
	}
	if corrects != nil {
		spec.Corrects, err = domain.NewAllocationVersion(*corrects)
		if err != nil {
			return domain.CostAllocation{}, err
		}
	}
	return domain.RehydrateCostAllocation(spec)
}

func marshalPortions(portions []domain.AllocationPortion) ([]byte, error) {
	rows := make([]portionRow, 0, len(portions))
	for _, portion := range portions {
		rows = append(rows, portionRow{Target: portion.Target.String(), AmountMinor: portion.AmountMinor})
	}
	return json.Marshal(rows)
}

func unmarshalPortions(raw []byte) ([]domain.AllocationPortion, error) {
	var rows []portionRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	portions := make([]domain.AllocationPortion, 0, len(rows))
	for _, row := range rows {
		target, err := domain.NewAllocationTargetReference(row.Target)
		if err != nil {
			return nil, err
		}
		portions = append(portions, domain.AllocationPortion{Target: target, AmountMinor: row.AmountMinor})
	}
	return portions, nil
}
