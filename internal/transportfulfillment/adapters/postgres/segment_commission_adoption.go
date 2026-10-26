package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// SegmentCommissionAdoptions 实现 ports.SegmentCommissionAdoptionStore（迁移 TF 0035、0049）与解除册的几口。采用与解除都只有
// INSERT 与读：解除另记一行、不改采用那一行，这个适配器上没有任何 UPDATE 或 DELETE。
type SegmentCommissionAdoptions struct {
	db *bentopg.DB
}

var _ ports.SegmentCommissionAdoptionStore = (*SegmentCommissionAdoptions)(nil)

func NewSegmentCommissionAdoptions(db *bentopg.DB) (*SegmentCommissionAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &SegmentCommissionAdoptions{db: db}, nil
}

// currentAdoptionCondition 是「没有解除的那一代」的读法，按段读当前采用与按委托列当前采用共用这一句。
const currentAdoptionCondition = `NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.segment_commission_adoption_withdrawal AS withdrawal
		         WHERE withdrawal.tenant_id = adoption.tenant_id
		           AND withdrawal.segment_ref = adoption.segment_ref
		           AND withdrawal.generation = adoption.generation)`

// FindBySegment 按（租户、段）取回当前采用：最后一代、且没有解除，经重建门复验各格成形。各代排成一条线、只有最后一代可能没有解除
// （约束 segment_commission_adoption_generation_chained 与外键 segment_commission_adoption_previous_withdrawn），按代次倒序取第一条
// 没解除的就是它。
func (repository *SegmentCommissionAdoptions) FindBySegment(
	ctx context.Context,
	key ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	row := querier.QueryRow(ctx,
		`SELECT adoption.segment_ref, adoption.generation, adoption.commission_id, adoption.provider_ref, adoption.start_basis,
		        adoption.started_at, adoption.content_digest, adoption.recorded_at, adoption.adopted_by
		   FROM transport_fulfillment.segment_commission_adoption AS adoption
		  WHERE adoption.tenant_id = $1
		    AND adoption.segment_ref = $2
		    AND `+currentAdoptionCondition+`
		  ORDER BY adoption.generation DESC
		  LIMIT 1`,
		key.TenantID.String(),
		key.Segment.String(),
	)
	record, err := scanAdoption(key.TenantID, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SegmentCommissionAdoptionRecord{}, false, nil
	}
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	return record, true, nil
}

// scanAdoption 把一行采用折回记录。
func scanAdoption(tenant domain.TenantID, row pgx.Row) (ports.SegmentCommissionAdoptionRecord, error) {
	var segmentRef, commissionID, provider, startBasis, digest string
	var generation int
	var startedAt, recordedAt time.Time
	var adoptedBy *string
	if err := row.Scan(&segmentRef, &generation, &commissionID, &provider, &startBasis, &startedAt, &digest, &recordedAt, &adoptedBy); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	spec := domain.RehydrateSegmentCommissionAdoptionSpec{TenantID: tenant, StartedAt: startedAt}
	var err error
	if spec.Segment, err = domain.NewFulfillmentSegmentReference(segmentRef); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	if spec.Commission, err = domain.NewTransportCommissionReference(commissionID); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	if spec.Provider, err = domain.NewServiceProviderReference(provider); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	if spec.StartBasis, err = domain.NewParticipationBasisReference(startBasis); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	adoption, err := domain.RehydrateSegmentCommissionAdoption(spec)
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	adopter, err := registrantFromColumn(adoptedBy)
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, err
	}
	return ports.SegmentCommissionAdoptionRecord{
		Key:           ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: spec.Segment},
		ContentDigest: digest,
		Adoption:      adoption,
		RecordedAt:    recordedAt.UTC(),
		Generation:    generation,
		AdoptedBy:     adopter,
	}, nil
}

// Save 写下这一段的下一代采用：代次取最后一代加一，且只在最后一代已解除（或这一段从没采用过）时落行；record 带了代次时还要恰是这一代，
// 编排据最近一次解除算出的代次与此刻的册对不上（另一笔并发采用先落了），同样不落。这一段还有当前采用、或另一笔并发首交先落了同一代，
// 都答`已登记`，不覆盖先到者。只认主键那一种冲突：外键不满足（段或委托不在册）是编排核过之后的违约，照实报错，
// 不吞成「已登记」。
func (repository *SegmentCommissionAdoptions) Save(
	ctx context.Context,
	record ports.SegmentCommissionAdoptionRecord,
) (ports.AdoptionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AdoptionSaveOutcomeInvalid, fmt.Errorf("save segment commission adoption: %w", err)
	}
	startBasis, startedAt := record.Adoption.Started()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.segment_commission_adoption
			(tenant_id, segment_ref, generation, previous_generation,
			 commission_id, provider_ref, start_basis, started_at, content_digest, recorded_at, adopted_by)
		 SELECT $1::text, $2::text, COALESCE(latest.generation, 0) + 1, latest.generation,
		        $3::text, $4::text, $5::text, $6::timestamptz, $7::text, $8::timestamptz, $10::text
		   FROM (SELECT 1) AS anchor
		   LEFT JOIN LATERAL (
		        SELECT adoption.generation, NOT `+currentAdoptionCondition+` AS withdrawn
		          FROM transport_fulfillment.segment_commission_adoption AS adoption
		         WHERE adoption.tenant_id = $1
		           AND adoption.segment_ref = $2
		         ORDER BY adoption.generation DESC
		         LIMIT 1) AS latest ON true
		  WHERE (latest.generation IS NULL OR latest.withdrawn)
		    AND ($9::integer = 0 OR COALESCE(latest.generation, 0) + 1 = $9::integer)
		 ON CONFLICT ON CONSTRAINT segment_commission_adoption_pkey DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		record.Adoption.Commission().String(),
		record.Adoption.Provider().String(),
		startBasis.String(),
		startedAt.UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
		record.Generation,
		optionalRegistrant(record.AdoptedBy),
	)
	if err != nil {
		return ports.AdoptionSaveOutcomeInvalid, fmt.Errorf("save segment commission adoption: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AdoptionAlreadyRegistered, nil
	}
	return ports.AdoptionSaved, nil
}
