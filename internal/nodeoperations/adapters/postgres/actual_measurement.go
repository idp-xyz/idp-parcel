package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ActualMeasurements 是实际测量登记册。测量与作废分两张表：作废不回头改测量行。
type ActualMeasurements struct {
	db *bentopg.DB
}

func NewActualMeasurements(db *bentopg.DB) (*ActualMeasurements, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ActualMeasurements{db: db}, nil
}

// SerializeSourceIdentity 取（租户，来源身份）的事务级咨询锁。测量与作废是两张表、各有各的主键，库里没有任何一处挡「同一来源身份
// 既是测量又是作废」；挡它的是编排：先取这把锁，再读另一张表。锁名与类别无关，所以一条测量与一条作废带同一个来源身份并发到时
// 也排同一个队，后一条读得到前一条已提交的行。事务的隔离级别是读已提交，取锁之后的读才看得见对方的提交。
func (repository *ActualMeasurements) SerializeSourceIdentity(ctx context.Context, key ports.ActualMeasurementKey) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("serialize measurement source identity: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.measurement.source_identity'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.SourceID,
	); err != nil {
		return fmt.Errorf("serialize measurement source identity: 取来源身份锁：%w", err)
	}
	return nil
}

func (repository *ActualMeasurements) FindMeasurement(
	ctx context.Context,
	key ports.ActualMeasurementKey,
) (ports.ActualMeasurementRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ActualMeasurementRecord{}, false, fmt.Errorf("find actual measurement: %w", err)
	}
	record, found, err := scanMeasurement(querier.QueryRow(ctx,
		measurementSelect+` WHERE tenant_id = $1 AND source_id = $2`,
		key.TenantID.String(), key.SourceID,
	))
	if err != nil {
		return ports.ActualMeasurementRecord{}, false, fmt.Errorf("find actual measurement: %w", err)
	}
	if found {
		record.Key = key
	}
	return record, found, nil
}

func (repository *ActualMeasurements) SaveMeasurement(
	ctx context.Context,
	record ports.ActualMeasurementRecord,
) (ports.ActualMeasurementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ActualMeasurementSaveOutcomeInvalid, fmt.Errorf("save actual measurement: %w", err)
	}
	measurement := record.Measurement
	weightValue, weightUnit := measurement.Weight()
	length, width, height, dimensionUnit := measurement.Dimensions()
	enteredBy, document, manual := measurement.ManualEntry()
	var original *string
	if manual {
		original = optionalReference(document.String())
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.actual_measurement
			(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
			 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
			 performed_by, evidence, occurred_at, recorded_at, entered_by, original_document)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		 ON CONFLICT ON CONSTRAINT actual_measurement_pkey DO NOTHING`,
		record.Key.TenantID.String(),
		measurement.Source().SourceID(),
		record.ContentDigest,
		measurement.ObjectKind().String(),
		measurement.ObjectID(),
		measurement.Node().String(),
		measurement.Location().String(),
		measurement.Basis().String(),
		weightValue,
		weightUnit,
		length,
		width,
		height,
		dimensionUnit,
		measurement.Source().PerformedBy().String(),
		measurement.Source().Evidence().String(),
		measurement.Source().OccurredAt().UTC(),
		record.RecordedAt.UTC(),
		optionalReference(enteredBy),
		original,
	)
	if err != nil {
		return ports.ActualMeasurementSaveOutcomeInvalid, fmt.Errorf("save actual measurement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ActualMeasurementAlreadyRecorded, nil
	}
	return ports.ActualMeasurementSaved, nil
}

func (repository *ActualMeasurements) ListStillValid(
	ctx context.Context,
	tenant domain.TenantID,
	objectKind domain.MeasurementObjectKind,
	objectID string,
) ([]ports.ActualMeasurementRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list actual measurements: %w", err)
	}
	rows, err := querier.Query(ctx,
		measurementSelect+`
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3
		    AND NOT EXISTS (
		        SELECT 1 FROM node_operations.measurement_invalidation AS invalidation
		         WHERE invalidation.tenant_id = actual_measurement.tenant_id
		           AND invalidation.target_source_id = actual_measurement.source_id
		    )
		  ORDER BY occurred_at, source_id`,
		tenant.String(), objectKind.String(), objectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list actual measurements: %w", err)
	}
	defer rows.Close()
	var listed []ports.ActualMeasurementRecord
	for rows.Next() {
		record, _, err := scanMeasurement(rows)
		if err != nil {
			return nil, fmt.Errorf("list actual measurements: %w", err)
		}
		record.Key.TenantID = tenant
		listed = append(listed, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list actual measurements: %w", err)
	}
	return listed, nil
}

func (repository *ActualMeasurements) FindInvalidation(
	ctx context.Context,
	key ports.ActualMeasurementKey,
) (ports.MeasurementInvalidationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.MeasurementInvalidationRecord{}, false, fmt.Errorf("find measurement invalidation: %w", err)
	}
	record, found, err := scanInvalidation(querier.QueryRow(ctx,
		invalidationSelect+` WHERE tenant_id = $1 AND source_id = $2`,
		key.TenantID.String(), key.SourceID,
	))
	if err != nil {
		return ports.MeasurementInvalidationRecord{}, false, fmt.Errorf("find measurement invalidation: %w", err)
	}
	if found {
		record.Key = key
	}
	return record, found, nil
}

// SaveInvalidation 先锁住被作废的那条测量，再看它有没有已经被指过。锁住再看，是为了
// 两次作废不会都以为自己是第一次；唯一约束是这道锁没盖住时的后背。
func (repository *ActualMeasurements) SaveInvalidation(
	ctx context.Context,
	record ports.MeasurementInvalidationRecord,
) (ports.MeasurementInvalidationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MeasurementInvalidationSaveOutcomeInvalid, fmt.Errorf("save measurement invalidation: %w", err)
	}
	invalidation := record.Invalidation
	var locked string
	err = executor.QueryRow(ctx,
		`SELECT source_id FROM node_operations.actual_measurement
		  WHERE tenant_id = $1 AND source_id = $2
		  FOR UPDATE`,
		record.Key.TenantID.String(), invalidation.TargetSourceID(),
	).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MeasurementInvalidationTargetMissing, nil
	}
	if err != nil {
		return ports.MeasurementInvalidationSaveOutcomeInvalid, fmt.Errorf("save measurement invalidation: %w", err)
	}
	var existingSource string
	err = executor.QueryRow(ctx,
		`SELECT source_id FROM node_operations.measurement_invalidation
		  WHERE tenant_id = $1 AND target_source_id = $2`,
		record.Key.TenantID.String(), invalidation.TargetSourceID(),
	).Scan(&existingSource)
	if err == nil {
		if existingSource == invalidation.Source().SourceID() {
			return ports.MeasurementInvalidationAlreadyRecorded, nil
		}
		return ports.MeasurementInvalidationAlreadyVoid, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ports.MeasurementInvalidationSaveOutcomeInvalid, fmt.Errorf("save measurement invalidation: %w", err)
	}
	enteredBy, document, manual := invalidation.ManualEntry()
	var original *string
	if manual {
		original = optionalReference(document.String())
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.measurement_invalidation
			(tenant_id, source_id, content_digest, target_source_id, reason, performed_by, evidence,
			 occurred_at, recorded_at, entered_by, original_document)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT ON CONSTRAINT measurement_invalidation_pkey DO NOTHING`,
		record.Key.TenantID.String(),
		invalidation.Source().SourceID(),
		record.ContentDigest,
		invalidation.TargetSourceID(),
		invalidation.Reason(),
		invalidation.Source().PerformedBy().String(),
		invalidation.Source().Evidence().String(),
		invalidation.Source().OccurredAt().UTC(),
		record.RecordedAt.UTC(),
		optionalReference(enteredBy),
		original,
	)
	if err != nil {
		return ports.MeasurementInvalidationSaveOutcomeInvalid, fmt.Errorf("save measurement invalidation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MeasurementInvalidationAlreadyRecorded, nil
	}
	return ports.MeasurementInvalidationSaved, nil
}

const measurementSelect = `
	SELECT source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
	       weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
	       performed_by, evidence, occurred_at, recorded_at, entered_by, original_document
	  FROM node_operations.actual_measurement`

const invalidationSelect = `
	SELECT source_id, content_digest, target_source_id, reason, performed_by, evidence,
	       occurred_at, recorded_at, entered_by, original_document
	  FROM node_operations.measurement_invalidation`

type measurementScanner interface {
	Scan(dest ...any) error
}

func scanMeasurement(row measurementScanner) (ports.ActualMeasurementRecord, bool, error) {
	var (
		sourceID, digest, objectKindName, objectID, nodeRef, locationRef, basisRef string
		weightValue, weightUnit, length, width, height, dimensionUnit              string
		performedBy, evidence                                                      string
		occurredAt, recordedAt                                                     time.Time
		enteredBy, originalDocument                                                *string
	)
	err := row.Scan(
		&sourceID, &digest, &objectKindName, &objectID, &nodeRef, &locationRef, &basisRef,
		&weightValue, &weightUnit, &length, &width, &height, &dimensionUnit,
		&performedBy, &evidence, &occurredAt, &recordedAt, &enteredBy, &originalDocument,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ActualMeasurementRecord{}, false, nil
	}
	if err != nil {
		return ports.ActualMeasurementRecord{}, false, err
	}
	measurement, err := rebuildMeasurement(
		sourceID, objectKindName, objectID, nodeRef, locationRef, basisRef,
		weightValue, weightUnit, length, width, height, dimensionUnit,
		performedBy, evidence, occurredAt, enteredBy, originalDocument,
	)
	if err != nil {
		return ports.ActualMeasurementRecord{}, false, err
	}
	if measurement.ContentDigest() != digest {
		return ports.ActualMeasurementRecord{}, false, fmt.Errorf("actual measurement %s digest does not match the row", sourceID)
	}
	return ports.ActualMeasurementRecord{
		Key:           ports.ActualMeasurementKey{SourceID: sourceID},
		Measurement:   measurement,
		ContentDigest: digest,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

func scanInvalidation(row measurementScanner) (ports.MeasurementInvalidationRecord, bool, error) {
	var (
		sourceID, digest, targetSourceID, reason, performedBy, evidence string
		occurredAt, recordedAt                                          time.Time
		enteredBy, originalDocument                                     *string
	)
	err := row.Scan(
		&sourceID, &digest, &targetSourceID, &reason, &performedBy, &evidence,
		&occurredAt, &recordedAt, &enteredBy, &originalDocument,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MeasurementInvalidationRecord{}, false, nil
	}
	if err != nil {
		return ports.MeasurementInvalidationRecord{}, false, err
	}
	invalidation, err := rebuildInvalidation(
		sourceID, targetSourceID, reason, performedBy, evidence, occurredAt, enteredBy, originalDocument,
	)
	if err != nil {
		return ports.MeasurementInvalidationRecord{}, false, err
	}
	if invalidation.ContentDigest() != digest {
		return ports.MeasurementInvalidationRecord{}, false, fmt.Errorf("measurement invalidation %s digest does not match the row", sourceID)
	}
	return ports.MeasurementInvalidationRecord{
		Key:           ports.ActualMeasurementKey{SourceID: sourceID},
		Invalidation:  invalidation,
		ContentDigest: digest,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

func rebuildMeasurement(
	sourceID, objectKindName, objectID, nodeRef, locationRef, basisRef string,
	weightValue, weightUnit, length, width, height, dimensionUnit string,
	performedBy, evidence string,
	occurredAt time.Time,
	enteredBy, originalDocument *string,
) (domain.ActualMeasurement, error) {
	source, err := measurementSource(sourceID, performedBy, evidence, occurredAt)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	objectKind, err := domain.MeasurementObjectKindFromName(objectKindName)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	node, err := domain.NewNodeReference(nodeRef)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	location, err := domain.NewWorkLocationReference(locationRef)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	basis, err := domain.NewWorkBasisReference(basisRef)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	spec := domain.ActualMeasurementSpec{
		Source: source, ObjectKind: objectKind, ObjectID: objectID,
		Node: node, Location: location, Basis: basis,
		WeightValue: weightValue, WeightUnit: weightUnit,
		Length: length, Width: width, Height: height, DimensionUnit: dimensionUnit,
		EnteredBy: textOf(enteredBy),
	}
	if originalDocument != nil {
		spec.OriginalDocument, err = domain.NewMeasurementOriginalDocument(*originalDocument)
		if err != nil {
			return domain.ActualMeasurement{}, err
		}
	}
	return domain.NewActualMeasurement(spec)
}

func rebuildInvalidation(
	sourceID, targetSourceID, reason, performedBy, evidence string,
	occurredAt time.Time,
	enteredBy, originalDocument *string,
) (domain.MeasurementInvalidation, error) {
	source, err := measurementSource(sourceID, performedBy, evidence, occurredAt)
	if err != nil {
		return domain.MeasurementInvalidation{}, err
	}
	spec := domain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: targetSourceID, Reason: reason, EnteredBy: textOf(enteredBy),
	}
	if originalDocument != nil {
		spec.OriginalDocument, err = domain.NewMeasurementOriginalDocument(*originalDocument)
		if err != nil {
			return domain.MeasurementInvalidation{}, err
		}
	}
	return domain.NewMeasurementInvalidation(spec)
}

func measurementSource(sourceID, performedBy, evidence string, occurredAt time.Time) (domain.WorkFactSource, error) {
	party, err := domain.NewPerformingPartyReference(performedBy)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	proof, err := domain.NewExecutionEvidenceReference(evidence)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	return domain.NewWorkFactSource(sourceID, party, proof, occurredAt)
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
