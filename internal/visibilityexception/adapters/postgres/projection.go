package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Projections 实现 ports.ProjectionStore。版本行只增不改写（ADR-0065）：Save 追加
// 版本行并把 tracking_projection_current 的显式标记指向它，原版本连同条目、映射
// 版本与派生时间留存，按版本读得回；读当前版走标记直取，不扫描历史。
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
	Kind        string    `json:"kind"`
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
		`SELECT v.version_id, v.derived_at, v.prior_version, v.entries
		   FROM visibility_exception.tracking_projection_current AS c
		   JOIN visibility_exception.tracking_projection_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.parcel_ref = c.parcel_ref
		    AND v.version_id = c.version_id
		  WHERE c.tenant_id = $1 AND c.parcel_ref = $2`,
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

// FindByVersion 按版本读回留存的任一版——当前版或已被更新的历史版皆可（ADR-0065）。
func (repository *Projections) FindByVersion(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.ProjectionVersionID,
) (domain.TrackingProjection, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("find projection by version: %w", err)
	}

	var parcelRef string
	var derivedAt time.Time
	var priorVersion *string
	var entriesRaw []byte
	err = querier.QueryRow(ctx,
		`SELECT parcel_ref, derived_at, prior_version, entries
		   FROM visibility_exception.tracking_projection_version
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant.String(), version.String(),
	).Scan(&parcelRef, &derivedAt, &priorVersion, &entriesRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TrackingProjection{}, false, nil
	}
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("find projection by version: %w", err)
	}

	parcel, err := domain.NewTrackedParcelReference(parcelRef)
	if err != nil {
		return domain.TrackingProjection{}, false, fmt.Errorf("rebuild projection: %w", err)
	}
	projection, err := rebuildProjection(version.String(), parcel, derivedAt, priorVersion, entriesRaw)
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

	// 版本只增：不带 ON CONFLICT，同版本号重写以主键冲突报错暴露，不静默覆盖
	// （ADR-0065「不得覆盖」）。
	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.tracking_projection_version
			(tenant_id, parcel_ref, version_id, derived_at, prior_version, entries)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(),
		projection.Parcel().String(),
		projection.Version().String(),
		projection.DerivedAt(),
		prior,
		entriesRaw,
	)
	if err != nil {
		return fmt.Errorf("save projection version: %w", err)
	}

	// 标记是唯一的可变处：当前版由它指名。并发重派生时后写者赢标记（与旧整行
	// UPSERT 同一竞态语义），但两个版本行都留存，输掉标记的那一版仍按版本读得回。
	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.tracking_projection_current
			(tenant_id, parcel_ref, version_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, parcel_ref) DO UPDATE SET
			version_id = EXCLUDED.version_id`,
		tenant.String(),
		projection.Parcel().String(),
		projection.Version().String(),
	)
	if err != nil {
		return fmt.Errorf("save projection current marker: %w", err)
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
			Kind:        fact.Kind().String(),
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
		if strings.TrimSpace(row.Kind) == "" {
			return domain.TrackingProjection{}, fmt.Errorf("projection entry missing source fact kind")
		}
		factKind, err := domain.NewSourceFactKind(row.Kind)
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
			Kind:        factKind,
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
