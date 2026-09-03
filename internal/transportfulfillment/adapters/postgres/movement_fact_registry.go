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

// MovementFacts 实现 ports.MovementFactRegistry：一个版本一行。
//
// **只插不改。** 移动是已经发生的事实——它可以被更正（新版本回指前身），但不可回写；本类型上
// 没有任何 UPDATE。
type MovementFacts struct {
	db *bentopg.DB
}

func NewMovementFacts(db *bentopg.DB) (*MovementFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &MovementFacts{db: db}, nil
}

var _ ports.MovementFactRegistry = (*MovementFacts)(nil)

// FindByKey 按（租户+事实+版本）取回一个版本。否定结果只回 false。
func (repository *MovementFacts) FindByKey(
	ctx context.Context,
	key ports.MovementFactKey,
) (ports.MovementFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}

	var scheduleRef, kindText, locationRef, sourceRef string
	var occurredAt, recordedAt time.Time
	var gateClearance, correctsVersion *string
	var correctedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT schedule_ref, kind, location_ref, source_ref, occurred_at,
		        gate_clearance, corrects_version, corrected_at, recorded_at
		   FROM transport_fulfillment.transport_movement_fact
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Fact.String(), key.Version.String(),
	).Scan(&scheduleRef, &kindText, &locationRef, &sourceRef, &occurredAt,
		&gateClearance, &correctsVersion, &correctedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MovementFactRecord{}, false, nil
	}
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}

	spec := domain.RehydrateMovementFactSpec{
		TenantID:   key.TenantID,
		Fact:       key.Fact,
		Version:    key.Version,
		OccurredAt: occurredAt.UTC(),
	}
	// 逐列走各自的构造门装回，不按列直接拼结构体。
	if spec.Kind, err = movementFactKindFrom(kindText); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Schedule, err = domain.NewScheduleReference(scheduleRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Location, err = domain.NewMovementLocationReference(locationRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Source, err = domain.NewMovementSourceReference(sourceRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if gateClearance != nil {
		if spec.GateClearance, err = domain.NewGateClearanceReference(*gateClearance); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if correctsVersion != nil {
		if spec.Corrects, err = domain.NewMovementFactVersion(*correctsVersion); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = correctedAt.UTC()
	}

	fact, err := domain.RehydrateMovementFact(spec)
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	return ports.MovementFactRecord{Key: key, Fact: fact, RecordedAt: recordedAt.UTC()}, true, nil
}

// Save 登记一个版本。撞键答`已登记`（ADR-0031），编排据此读回原版本。
func (repository *MovementFacts) Save(
	ctx context.Context,
	record ports.MovementFactRecord,
) (ports.MovementFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}
	if err := assertMovementFactKeyAgrees(record); err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}

	var clearanceText, correctsText *string
	if clearance, gated := record.Fact.GateClearance(); gated {
		text := clearance.String()
		clearanceText = &text
	}
	if corrects, has := record.Fact.Corrects(); has {
		text := corrects.String()
		correctsText = &text
	}
	correctedAt, corrected := record.Fact.CorrectedAt()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_movement_fact
		     (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref,
		      occurred_at, gate_clearance, corrects_version, corrected_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Key.Version.String(),
		record.Fact.Schedule().String(),
		record.Fact.Kind().String(),
		record.Fact.Location().String(),
		record.Fact.Source().String(),
		record.Fact.OccurredAt().UTC(),
		clearanceText,
		correctsText,
		nullableTime(correctedAt, corrected),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MovementFactVersionAlreadyRegistered, nil
	}
	return ports.MovementFactSaved, nil
}

// assertMovementFactKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertMovementFactKeyAgrees(record ports.MovementFactRecord) error {
	if record.Key.TenantID != record.Fact.TenantID() ||
		record.Key.Fact != record.Fact.Fact() ||
		record.Key.Version != record.Fact.Version() {
		return fmt.Errorf("movement fact key disagrees with the aggregate")
	}
	return nil
}

func movementFactKindFrom(raw string) (domain.MovementFactKind, error) {
	switch raw {
	case domain.DepartureFact.String():
		return domain.DepartureFact, nil
	case domain.InTransitFact.String():
		return domain.InTransitFact, nil
	case domain.ArrivalFact.String():
		return domain.ArrivalFact, nil
	default:
		return domain.MovementFactKindInvalid, fmt.Errorf("unknown movement fact kind %q", raw)
	}
}
