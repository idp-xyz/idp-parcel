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

// SegmentCommissionAdoptions 实现 ports.SegmentCommissionAdoptionStore（迁移 TF 0035）。只有 INSERT 与按段读：采用之后不改、
// 不解除，这个适配器上没有任何 UPDATE 或 DELETE。
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

// FindBySegment 按（租户、段）取回那一份采用，经重建门复验各格成形。
func (repository *SegmentCommissionAdoptions) FindBySegment(
	ctx context.Context,
	key ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	var commissionID, provider, startBasis, digest string
	var startedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT commission_id, provider_ref, start_basis, started_at, content_digest, recorded_at
		   FROM transport_fulfillment.segment_commission_adoption
		  WHERE tenant_id = $1
		    AND segment_ref = $2`,
		key.TenantID.String(),
		key.Segment.String(),
	).Scan(&commissionID, &provider, &startBasis, &startedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SegmentCommissionAdoptionRecord{}, false, nil
	}
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}

	spec := domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:  key.TenantID,
		Segment:   key.Segment,
		StartedAt: startedAt,
	}
	if spec.Commission, err = domain.NewTransportCommissionReference(commissionID); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	if spec.Provider, err = domain.NewServiceProviderReference(provider); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	if spec.StartBasis, err = domain.NewParticipationBasisReference(startBasis); err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	adoption, err := domain.RehydrateSegmentCommissionAdoption(spec)
	if err != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, fmt.Errorf("find segment commission adoption: %w", err)
	}
	return ports.SegmentCommissionAdoptionRecord{
		Key:           key,
		ContentDigest: digest,
		Adoption:      adoption,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一份采用。同段已有采用时答`已登记`，不覆盖先到者。只认主键那一种冲突：外键不满足（段或委托不在册）是编排
// 核过之后的违约，照实报错，不吞成「已登记」。
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
			(tenant_id, segment_ref, commission_id, provider_ref, start_basis, started_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT ON CONSTRAINT segment_commission_adoption_pkey DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		record.Adoption.Commission().String(),
		record.Adoption.Provider().String(),
		startBasis.String(),
		startedAt.UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AdoptionSaveOutcomeInvalid, fmt.Errorf("save segment commission adoption: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AdoptionAlreadyRegistered, nil
	}
	return ports.AdoptionSaved, nil
}
