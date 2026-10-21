package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.SegmentCommissionAdoptionsByCommission = (*SegmentCommissionAdoptions)(nil)

// ListByCommission 按（租户、委托）列出采用了它的各段，按段排序（迁移 0035 的按委托索引）。一行装不回就整次报错，不跳过它：跳过一段，
// 换版之后那一段的补形成就无声地缺了。
func (repository *SegmentCommissionAdoptions) ListByCommission(
	ctx context.Context,
	key ports.TransportCommissionKey,
) ([]ports.SegmentCommissionAdoptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT segment_ref, provider_ref, start_basis, started_at, content_digest, recorded_at
		   FROM transport_fulfillment.segment_commission_adoption
		  WHERE tenant_id = $1
		    AND commission_id = $2
		  ORDER BY segment_ref`,
		key.TenantID.String(), key.Commission.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	defer rows.Close()
	var records []ports.SegmentCommissionAdoptionRecord
	for rows.Next() {
		var segmentRef, provider, startBasis, digest string
		var startedAt, recordedAt time.Time
		if err := rows.Scan(&segmentRef, &provider, &startBasis, &startedAt, &digest, &recordedAt); err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
		}
		spec := domain.RehydrateSegmentCommissionAdoptionSpec{TenantID: key.TenantID, Commission: key.Commission, StartedAt: startedAt}
		if spec.Segment, err = domain.NewFulfillmentSegmentReference(segmentRef); err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
		}
		if spec.Provider, err = domain.NewServiceProviderReference(provider); err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
		}
		if spec.StartBasis, err = domain.NewParticipationBasisReference(startBasis); err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
		}
		adoption, err := domain.RehydrateSegmentCommissionAdoption(spec)
		if err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: segment %s: %w", segmentRef, err)
		}
		records = append(records, ports.SegmentCommissionAdoptionRecord{
			Key:           ports.SegmentCommissionAdoptionKey{TenantID: key.TenantID, Segment: spec.Segment},
			ContentDigest: digest,
			Adoption:      adoption,
			RecordedAt:    recordedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	return records, nil
}
