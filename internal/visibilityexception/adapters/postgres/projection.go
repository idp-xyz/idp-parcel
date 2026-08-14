package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Projections 实现 ports.ProjectionStore。键是（租户+包裹）；Save 整行 UPSERT——
// 库只管当前版，重派生改同一行，历史由 prior_version 指回。
type Projections struct {
	db *bentopg.DB
}

func NewProjections(db *bentopg.DB) (*Projections, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &Projections{db: db}, nil
}

type projectionEntryRow struct {
	Source      string    `json:"source"`
	Parcel      string    `json:"parcel"`
	Fact        string    `json:"fact"`
	Version     string    `json:"version"`
	OccurredAt  time.Time `json:"occurredAt"`
	EffectiveAt time.Time `json:"effectiveAt"`
	ReceivedAt  time.Time `json:"receivedAt"`
	Mapping     string    `json:"mapping"`
	Milestone   string    `json:"milestone,omitempty"`
}

func (repository *Projections) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) (domain.TrackingProjection, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("find current projection: %w", err)
	}

	var versionID string
	var derivedAt time.Time
	var priorVersion *string
	var entriesRaw []byte
	err = querier.QueryRow(ctx,
		`SELECT version_id, derived_at, prior_version, entries
		   FROM visibility_exception.tracking_projection
		  WHERE tenant_id = $1 AND parcel_ref = $2`,
		tenant.String(), parcel.String(),
	).Scan(&versionID, &derivedAt, &priorVersion, &entriesRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TrackingProjection{}, false, nil
	}
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("find current projection: %w", err)
	}

	projection, err := rebuildProjection(versionID, parcel, derivedAt, priorVersion, entriesRaw)
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("rebuild projection: %w", err)
	}
	return projection, true, nil
}

func (repository *Projections) Save(
	ctx context.Context,
	tenant domain.TenantID,
	projection domain.TrackingProjection,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save projection: %w", err)
	}

	entriesRaw, err := marshalProjectionEntries(projection.Entries())
	if err != nil {
		return fmt.Errorf("save projection: %w", err)
	}
	var prior *string
	if version, ok := projection.PriorVersion(); ok {
		prior = stringPointer(version.String())
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.tracking_projection
			(tenant_id, parcel_ref, version_id, derived_at, prior_version, entries)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, parcel_ref) DO UPDATE SET
			version_id = EXCLUDED.version_id,
			derived_at = EXCLUDED.derived_at,
			prior_version = EXCLUDED.prior_version,
			entries = EXCLUDED.entries`,
		tenant.String(),
		projection.Parcel().String(),
		projection.Version().String(),
		projection.DerivedAt(),
		prior,
		entriesRaw,
	)
	if err != nil {
		return fmt.Errorf("save projection: %w", err)
	}
	return nil
}

func marshalProjectionEntries(entries []domain.MilestoneClassification) ([]byte, error) {
	rows := make([]projectionEntryRow, 0, len(entries))
	for _, entry := range entries {
		fact := entry.Fact()
		row := projectionEntryRow{
			Source:      fact.Source().String(),
			Parcel:      fact.Parcel().String(),
			Fact:        fact.Fact().String(),
			Version:     fact.Version().String(),
			OccurredAt:  fact.OccurredAt(),
			EffectiveAt: fact.EffectiveAt(),
			ReceivedAt:  fact.ReceivedAt(),
			Mapping:     entry.MappingVersion().String(),
		}
		if milestone, classified := entry.Milestone(); classified {
			row.Milestone = milestone.String()
		}
		rows = append(rows, row)
	}
	return json.Marshal(rows)
}

func rebuildProjection(
	versionID string,
	parcel domain.TrackedParcelReference,
	derivedAt time.Time,
	priorVersion *string,
	entriesRaw []byte,
) (domain.TrackingProjection, error) {
	version, err := domain.NewProjectionVersionID(versionID)
	if err != nil {
		return domain.TrackingProjection{}, err
	}
	var rows []projectionEntryRow
	if err := json.Unmarshal(entriesRaw, &rows); err != nil {
		return domain.TrackingProjection{}, fmt.Errorf("entries: %w", err)
	}
	entries := make([]domain.MilestoneClassification, 0, len(rows))
	for _, row := range rows {
		source, err := sourceContextFrom(row.Source)
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		factParcel, err := domain.NewTrackedParcelReference(row.Parcel)
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		factRef, err := domain.NewSourceFactReference(row.Fact)
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		factVersion, err := domain.NewSourceFactVersion(row.Version)
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
			Source:      source,
			Parcel:      factParcel,
			Fact:        factRef,
			Version:     factVersion,
			OccurredAt:  row.OccurredAt,
			EffectiveAt: row.EffectiveAt,
			ReceivedAt:  row.ReceivedAt,
		})
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		mapping, err := domain.NewMappingVersionReference(row.Mapping)
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		var classification domain.MilestoneClassification
		if row.Milestone == "" {
			classification, err = domain.LeaveUnclassified(fact, mapping)
		} else {
			var milestone domain.MilestoneReference
			milestone, err = domain.NewMilestoneReference(row.Milestone)
			if err != nil {
				return domain.TrackingProjection{}, err
			}
			classification, err = domain.ClassifyMilestone(fact, milestone, mapping)
		}
		if err != nil {
			return domain.TrackingProjection{}, err
		}
		entries = append(entries, classification)
	}
	var prior domain.ProjectionVersionID
	if priorVersion != nil {
		if prior, err = domain.NewProjectionVersionID(*priorVersion); err != nil {
			return domain.TrackingProjection{}, err
		}
	}
	return domain.RehydrateTrackingProjection(version, parcel, entries, derivedAt, prior)
}
