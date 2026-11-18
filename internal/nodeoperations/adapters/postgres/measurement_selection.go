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

// MeasurementSelectionAdoptions 是当前有效实测形态的采用登记册。
type MeasurementSelectionAdoptions struct {
	db *bentopg.DB
}

func NewMeasurementSelectionAdoptions(db *bentopg.DB) (*MeasurementSelectionAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &MeasurementSelectionAdoptions{db: db}, nil
}

func (repository *MeasurementSelectionAdoptions) FindAdoption(
	ctx context.Context,
	tenant domain.TenantID,
) (ports.MeasurementSelectionAdoption, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.MeasurementSelectionAdoption{}, false, fmt.Errorf("find measurement selection adoption: %w", err)
	}
	var (
		formName, basis string
		recordedAt      time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT form, basis, recorded_at
		   FROM node_operations.measurement_selection_adoption
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&formName, &basis, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MeasurementSelectionAdoption{}, false, nil
	}
	if err != nil {
		return ports.MeasurementSelectionAdoption{}, false, fmt.Errorf("find measurement selection adoption: %w", err)
	}
	form, err := domain.MeasurementSelectionFormFromName(formName)
	if err != nil {
		return ports.MeasurementSelectionAdoption{}, false, fmt.Errorf("find measurement selection adoption: %w", err)
	}
	return ports.MeasurementSelectionAdoption{
		TenantID: tenant, Form: form, Basis: basis, RecordedAt: recordedAt.UTC(),
	}, true, nil
}

func (repository *MeasurementSelectionAdoptions) SaveAdoption(
	ctx context.Context,
	adoption ports.MeasurementSelectionAdoption,
) (ports.MeasurementSelectionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MeasurementSelectionSaveOutcomeInvalid, fmt.Errorf("save measurement selection adoption: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.measurement_selection_adoption
			(tenant_id, form, basis, recorded_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT ON CONSTRAINT measurement_selection_adoption_pkey DO NOTHING`,
		adoption.TenantID.String(), adoption.Form.String(), adoption.Basis, adoption.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.MeasurementSelectionSaveOutcomeInvalid, fmt.Errorf("save measurement selection adoption: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.MeasurementSelectionSaved, nil
	}
	existing, found, err := repository.FindAdoption(ctx, adoption.TenantID)
	if err != nil {
		return ports.MeasurementSelectionSaveOutcomeInvalid, err
	}
	if found && existing.Form == adoption.Form && existing.Basis == adoption.Basis {
		return ports.MeasurementSelectionAlreadyRecorded, nil
	}
	return ports.MeasurementSelectionConflicts, nil
}
