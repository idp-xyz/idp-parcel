package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Charges 记下仓储作业收费发生项。表上没有金额列。
type Charges struct {
	db *bentopg.DB
}

func NewCharges(db *bentopg.DB) (*Charges, error) {
	if db == nil {
		return nil, errors.New("warehouse fulfillment postgres: charges need a db")
	}
	return &Charges{db: db}, nil
}

var _ ports.ChargeStore = (*Charges)(nil)

const chargeVersionColumns = `SELECT occurrence.occurrence_id, version.version, version.supersedes, version.correction, occurrence.kind,
		        occurrence.owner_account_id, occurrence.warehouse_code, occurrence.source_fact, version.business_time,
		        version.measures, version.attributes, version.lines, version.weight_unit, version.volume_unit`

func (store *Charges) LoadBySource(ctx context.Context, tenant domain.TenantID, kind, source string) (ports.ChargeVersion, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge occurrence: %w", err)
	}
	version, err := scanCharge(executor.QueryRow(ctx,
		chargeVersionColumns+`
		   FROM warehouse_fulfillment.charge_occurrence occurrence
		   JOIN warehouse_fulfillment.charge_occurrence_version version
		     ON version.tenant_id = occurrence.tenant_id AND version.occurrence_id = occurrence.occurrence_id
		    AND version.version = occurrence.head_version
		  WHERE occurrence.tenant_id = $1 AND occurrence.kind = $2 AND occurrence.source_fact = $3`,
		tenant.String(), kind, source))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeVersion{}, false, nil
	}
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge occurrence: %w", err)
	}
	return version, true, nil
}

func (store *Charges) LoadVersion(ctx context.Context, tenant domain.TenantID, id string, number int) (ports.ChargeVersion, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge version: %w", err)
	}
	version, err := scanCharge(executor.QueryRow(ctx,
		chargeVersionColumns+`
		   FROM warehouse_fulfillment.charge_occurrence occurrence
		   JOIN warehouse_fulfillment.charge_occurrence_version version
		     ON version.tenant_id = occurrence.tenant_id AND version.occurrence_id = occurrence.occurrence_id
		  WHERE occurrence.tenant_id = $1 AND occurrence.occurrence_id = $2 AND version.version = $3`,
		tenant.String(), id, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeVersion{}, false, nil
	}
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge version: %w", err)
	}
	return version, true, nil
}

func (store *Charges) Successor(ctx context.Context, tenant domain.TenantID, id string, number int) (ports.ChargeVersion, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge successor: %w", err)
	}
	version, err := scanCharge(executor.QueryRow(ctx,
		chargeVersionColumns+`
		   FROM warehouse_fulfillment.charge_occurrence occurrence
		   JOIN warehouse_fulfillment.charge_occurrence_version version
		     ON version.tenant_id = occurrence.tenant_id AND version.occurrence_id = occurrence.occurrence_id
		  WHERE occurrence.tenant_id = $1 AND occurrence.occurrence_id = $2 AND version.supersedes = $3`,
		tenant.String(), id, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeVersion{}, false, nil
	}
	if err != nil {
		return ports.ChargeVersion{}, false, fmt.Errorf("load charge successor: %w", err)
	}
	return version, true, nil
}

func (store *Charges) SaveOriginal(ctx context.Context, tenant domain.TenantID, version ports.ChargeVersion) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save charge occurrence: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.charge_occurrence (
		    tenant_id, occurrence_id, kind, owner_account_id, warehouse_code, source_fact, head_version)
		 VALUES ($1,$2,$3,$4,$5,$6,1)`,
		tenant.String(), version.ID, version.Kind, version.Owner, version.Warehouse, version.Source); err != nil {
		return fmt.Errorf("save charge occurrence: %w", err)
	}
	return insertChargeVersion(ctx, executor, tenant, version)
}

func (store *Charges) SaveCorrection(ctx context.Context, tenant domain.TenantID, version ports.ChargeVersion) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save charge correction: %w", err)
	}
	if err := insertChargeVersion(ctx, executor, tenant, version); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.charge_occurrence SET head_version = $3
		  WHERE tenant_id = $1 AND occurrence_id = $2`,
		tenant.String(), version.ID, version.Version); err != nil {
		return fmt.Errorf("save charge correction: %w", err)
	}
	return nil
}

func (store *Charges) LoadUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (string, string, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("load charge units: %w", err)
	}
	var weight, volume string
	err = executor.QueryRow(ctx,
		`SELECT weight_unit, volume_unit FROM warehouse_fulfillment.charge_measure_unit_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2
		  ORDER BY revision DESC LIMIT 1`,
		tenant.String(), warehouse.String()).Scan(&weight, &volume)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load charge units: %w", err)
	}
	return weight, volume, true, nil
}

func (store *Charges) LoadUnitsAt(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, at time.Time) (string, string, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("load charge units: %w", err)
	}
	var weight, volume string
	err = executor.QueryRow(ctx,
		`SELECT weight_unit, volume_unit FROM warehouse_fulfillment.charge_measure_unit_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND registered_at < $3
		  ORDER BY revision DESC LIMIT 1`,
		tenant.String(), warehouse.String(), at).Scan(&weight, &volume)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load charge units: %w", err)
	}
	return weight, volume, true, nil
}

func (store *Charges) SaveUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, weight, volume string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save charge units: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.charge_measure_unit_revision (
		    tenant_id, warehouse_code, revision, weight_unit, volume_unit, registered_at)
		 SELECT $1, $2, COALESCE(MAX(revision), 0) + 1, $3, $4, $5
		   FROM warehouse_fulfillment.charge_measure_unit_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2`,
		tenant.String(), warehouse.String(), weight, volume, at); err != nil {
		return fmt.Errorf("save charge units: %w", err)
	}
	return nil
}

func (store *Charges) LoadAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (string, time.Time, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("load aging basis: %w", err)
	}
	var basis string
	var at time.Time
	err = executor.QueryRow(ctx,
		`SELECT basis, registered_at FROM warehouse_fulfillment.charge_aging_basis
		  WHERE tenant_id = $1 AND warehouse_code = $2`,
		tenant.String(), warehouse.String()).Scan(&basis, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("load aging basis: %w", err)
	}
	return basis, at, true, nil
}

func (store *Charges) SaveAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, basis string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save aging basis: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.charge_aging_basis (tenant_id, warehouse_code, basis, registered_at)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), warehouse.String(), basis, at); err != nil {
		return fmt.Errorf("save aging basis: %w", err)
	}
	return nil
}

func (store *Charges) LoadProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string) (bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load charge project: %w", err)
	}
	var found int
	err = executor.QueryRow(ctx,
		`SELECT 1 FROM warehouse_fulfillment.charge_service_project
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND project_name = $3`,
		tenant.String(), warehouse.String(), name).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load charge project: %w", err)
	}
	return true, nil
}

func (store *Charges) SaveProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save charge project: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.charge_service_project (tenant_id, warehouse_code, project_name, registered_at)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), warehouse.String(), name, at); err != nil {
		return fmt.Errorf("save charge project: %w", err)
	}
	return nil
}

type chargeExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertChargeVersion(ctx context.Context, executor chargeExec, tenant domain.TenantID, version ports.ChargeVersion) error {
	if version.Measures == nil {
		version.Measures = []ports.ChargeMeasure{}
	}
	if version.Attributes == nil {
		version.Attributes = []ports.ChargeAttribute{}
	}
	if version.Lines == nil {
		version.Lines = []ports.ChargeLine{}
	}
	measures, err := json.Marshal(version.Measures)
	if err != nil {
		return err
	}
	attributes, err := json.Marshal(version.Attributes)
	if err != nil {
		return err
	}
	lines, err := json.Marshal(version.Lines)
	if err != nil {
		return err
	}
	var supersedes *int
	if version.Supersedes > 0 {
		supersedes = &version.Supersedes
	}
	weight := nullableText(version.WeightUnit)
	volume := nullableText(version.VolumeUnit)
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.charge_occurrence_version (
		    tenant_id, occurrence_id, version, supersedes, correction, business_time, measures, attributes, lines,
		    weight_unit, volume_unit)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		tenant.String(), version.ID, version.Version, supersedes, version.Correction, version.Occurred, measures, attributes, lines, weight, volume); err != nil {
		return fmt.Errorf("save charge version: %w", err)
	}
	return nil
}

type chargeScanner interface {
	Scan(dest ...any) error
}

func scanCharge(row chargeScanner) (ports.ChargeVersion, error) {
	var version ports.ChargeVersion
	var supersedes *int
	var weight, volume *string
	var measures, attributes, lines []byte
	err := row.Scan(&version.ID, &version.Version, &supersedes, &version.Correction, &version.Kind, &version.Owner, &version.Warehouse, &version.Source, &version.Occurred, &measures, &attributes, &lines, &weight, &volume)
	if err != nil {
		return ports.ChargeVersion{}, err
	}
	if supersedes != nil {
		version.Supersedes = *supersedes
	}
	if weight != nil {
		version.WeightUnit = *weight
	}
	if volume != nil {
		version.VolumeUnit = *volume
	}
	if err := json.Unmarshal(measures, &version.Measures); err != nil {
		return ports.ChargeVersion{}, err
	}
	if err := json.Unmarshal(attributes, &version.Attributes); err != nil {
		return ports.ChargeVersion{}, err
	}
	if err := json.Unmarshal(lines, &version.Lines); err != nil {
		return ports.ChargeVersion{}, err
	}
	return version, nil
}
