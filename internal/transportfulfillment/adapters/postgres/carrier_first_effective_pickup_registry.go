package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CarrierFirstEffectivePickups 实现 ports.CarrierFirstEffectivePickupRegistry：一行一版本、依据另表逐条，只插不改
// （迁移 transport_fulfillment/0020）。
//
// 「链尾」按「未被同一事实的任何版本回指」派生，没有 current 列——与 ExternalTrackingFacts 同一条纪律；FindByKey 交回
// 指名那一代，不问它是不是链尾（lc/24 教训：消费方每份信封代表一代）。撞键（同版本重放、同对象第二条首登、同一前版
// 被回指两次）都由唯一约束拦下、译成已登记（ADR-0031），事务保持可用让编排读回链尾再答。
type CarrierFirstEffectivePickups struct {
	db *bentopg.DB
}

func NewCarrierFirstEffectivePickups(db *bentopg.DB) (*CarrierFirstEffectivePickups, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &CarrierFirstEffectivePickups{db: db}, nil
}

var _ ports.CarrierFirstEffectivePickupRegistry = (*CarrierFirstEffectivePickups)(nil)

const selectCarrierPickupColumns = `SELECT tenant_id, fact_ref, version, object_ref, result, carrier_kind, carrier_ref,
	        occurred_at, pending_reason, claimed_material, judged_at, supersedes_version, recorded_at
	   FROM transport_fulfillment.carrier_first_effective_pickup p`

// carrierPickupNotSuperseded 是「链尾」谓词：同一（租户，事实）下没有别的版本回指它。
const carrierPickupNotSuperseded = ` AND NOT EXISTS (
	        SELECT 1 FROM transport_fulfillment.carrier_first_effective_pickup later
	         WHERE later.tenant_id = p.tenant_id AND later.fact_ref = p.fact_ref AND later.supersedes_version = p.version)`

type carrierPickupRow struct {
	tenant, fact, version, object, result                 string
	carrierKind, carrierRef, reason, material, supersedes *string
	occurredAt                                            *time.Time
	judgedAt, recordedAt                                  time.Time
}

func scanCarrierPickupRow(scanner interface{ Scan(dest ...any) error }) (carrierPickupRow, error) {
	var row carrierPickupRow
	err := scanner.Scan(&row.tenant, &row.fact, &row.version, &row.object, &row.result, &row.carrierKind, &row.carrierRef,
		&row.occurredAt, &row.reason, &row.material, &row.judgedAt, &row.supersedes, &row.recordedAt)
	return row, err
}

// FindByKey 按（租户，事实，版本）取回指名那一代。否定结果只回 false，不区分「不存在」与「属于另一个租户」。
func (repository *CarrierFirstEffectivePickups) FindByKey(
	ctx context.Context,
	key ports.CarrierFirstEffectivePickupKey,
) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find carrier first effective pickup: %w", err)
	}
	row, err := scanCarrierPickupRow(querier.QueryRow(ctx,
		selectCarrierPickupColumns+` WHERE p.tenant_id = $1 AND p.fact_ref = $2 AND p.version = $3`,
		key.TenantID.String(), key.Fact.String(), key.Version.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CarrierFirstEffectivePickupRecord{}, false, nil
	}
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find carrier first effective pickup: %w", err)
	}
	record, err := repository.rehydrate(ctx, querier, row)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find carrier first effective pickup: %w", err)
	}
	return record, true, nil
}

// FindCurrentByObject 交回该对象那条链的链尾。一对象至多一条链，所以最多一行。
func (repository *CarrierFirstEffectivePickups) FindCurrentByObject(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find current carrier first effective pickup: %w", err)
	}
	row, err := scanCarrierPickupRow(querier.QueryRow(ctx,
		selectCarrierPickupColumns+` WHERE p.tenant_id = $1 AND p.object_ref = $2`+carrierPickupNotSuperseded,
		tenant.String(), object.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CarrierFirstEffectivePickupRecord{}, false, nil
	}
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find current carrier first effective pickup: %w", err)
	}
	record, err := repository.rehydrate(ctx, querier, row)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, fmt.Errorf("find current carrier first effective pickup: %w", err)
	}
	return record, true, nil
}

// ListByObject 交回该对象整条链，按判断形成时间、版本升序。
func (repository *CarrierFirstEffectivePickups) ListByObject(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.CarrierFirstEffectivePickupRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list carrier first effective pickups: %w", err)
	}
	rows, err := querier.Query(ctx,
		selectCarrierPickupColumns+` WHERE p.tenant_id = $1 AND p.object_ref = $2 ORDER BY p.judged_at, p.version`,
		tenant.String(), object.String())
	if err != nil {
		return nil, fmt.Errorf("list carrier first effective pickups: %w", err)
	}
	var scanned []carrierPickupRow
	for rows.Next() {
		row, err := scanCarrierPickupRow(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("list carrier first effective pickups: %w", err)
		}
		scanned = append(scanned, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list carrier first effective pickups: %w", err)
	}
	records := make([]ports.CarrierFirstEffectivePickupRecord, 0, len(scanned))
	for _, row := range scanned {
		record, err := repository.rehydrate(ctx, querier, row)
		if err != nil {
			return nil, fmt.Errorf("list carrier first effective pickups: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

// Save 追加一版连同它的依据。撞任一唯一约束译已登记（ON CONFLICT DO NOTHING 保事务可用）；依据表撞键不可能先于
// 版本表撞键发生，所以依据只在版本行真插进去之后才插。
func (repository *CarrierFirstEffectivePickups) Save(
	ctx context.Context,
	record ports.CarrierFirstEffectivePickupRecord,
) (ports.CarrierPickupSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CarrierPickupSaveOutcomeInvalid, fmt.Errorf("save carrier first effective pickup: %w", err)
	}
	pickup := record.Pickup
	var carrierKind, carrierRef, reason, material, supersedes *string
	var occurredAt *time.Time
	if carrier, identified := pickup.Carrier(); identified {
		kind, reference := carrier.Kind().String(), carrier.Reference()
		carrierKind, carrierRef = &kind, &reference
	}
	if at, present := pickup.OccurredAt(); present {
		utc := at.UTC()
		occurredAt = &utc
	}
	if pendingReason, pending := pickup.PendingReason(); pending {
		word := pendingReason.String()
		reason = &word
		claimed := pickup.Material()
		material = &claimed
	}
	if prior, has := pickup.Supersedes(); has {
		value := prior.String()
		supersedes = &value
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.carrier_first_effective_pickup
		     (tenant_id, fact_ref, version, object_ref, result, carrier_kind, carrier_ref, occurred_at,
		      pending_reason, claimed_material, judged_at, supersedes_version, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.Fact.String(), record.Key.Version.String(), pickup.Object().String(),
		pickup.Result().String(), carrierKind, carrierRef, occurredAt, reason, material,
		pickup.JudgedAt().UTC(), supersedes, record.RecordedAt.UTC())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ports.CarrierPickupAlreadyRegistered, nil
		}
		return ports.CarrierPickupSaveOutcomeInvalid, fmt.Errorf("save carrier first effective pickup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CarrierPickupAlreadyRegistered, nil
	}
	for ordinal, basis := range pickup.Bases() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.carrier_first_effective_pickup_basis
			     (tenant_id, fact_ref, version, ordinal, source, evidence_ref, source_version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			record.Key.TenantID.String(), record.Key.Fact.String(), record.Key.Version.String(), ordinal+1,
			basis.Source().String(), basis.Reference().String(), basis.SourceVersion()); err != nil {
			return ports.CarrierPickupSaveOutcomeInvalid, fmt.Errorf("save carrier first effective pickup basis: %w", err)
		}
	}
	return ports.CarrierPickupSaved, nil
}

// rehydrate 读回一版的依据并过领域重建门——坏行在这里暴露，不流到编排里。
func (repository *CarrierFirstEffectivePickups) rehydrate(
	ctx context.Context,
	querier interface {
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	},
	row carrierPickupRow,
) (ports.CarrierFirstEffectivePickupRecord, error) {
	basisRows, err := querier.Query(ctx,
		`SELECT source, evidence_ref, source_version
		   FROM transport_fulfillment.carrier_first_effective_pickup_basis
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3
		  ORDER BY ordinal`,
		row.tenant, row.fact, row.version)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	defer basisRows.Close()
	var bases []domain.CarrierPickupBasis
	for basisRows.Next() {
		var sourceWord, reference, sourceVersion string
		if err := basisRows.Scan(&sourceWord, &reference, &sourceVersion); err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
		source, err := domain.ParseCarrierEvidenceSource(sourceWord)
		if err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
		basis, err := domain.NewCarrierPickupBasis(source, reference, sourceVersion)
		if err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
		bases = append(bases, basis)
	}
	if err := basisRows.Err(); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}

	spec := domain.RehydrateCarrierFirstEffectivePickupSpec{JudgedAt: row.judgedAt, Bases: bases}
	if spec.TenantID, err = domain.NewTenantID(row.tenant); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	if spec.Object, err = domain.NewCarriedObjectReference(row.object); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	if spec.Fact, err = domain.NewCarrierFirstEffectivePickupReference(row.fact); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	if spec.Version, err = domain.NewCarrierFirstEffectivePickupVersion(row.version); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	if spec.Result, err = domain.ParseCarrierPickupResult(row.result); err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	if row.carrierKind != nil && row.carrierRef != nil {
		kind, err := domain.ParseCarrierSubjectKind(*row.carrierKind)
		if err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
		if spec.Carrier, err = domain.NewCarrierSubject(kind, *row.carrierRef); err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
	}
	if row.occurredAt != nil {
		spec.OccurredAt = *row.occurredAt
	}
	if row.reason != nil {
		if spec.Reason, err = domain.ParsePendingPickupReason(*row.reason); err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
	}
	if row.material != nil {
		spec.Material = *row.material
	}
	if row.supersedes != nil {
		if spec.Supersedes, err = domain.NewCarrierFirstEffectivePickupVersion(*row.supersedes); err != nil {
			return ports.CarrierFirstEffectivePickupRecord{}, err
		}
	}
	pickup, err := domain.RehydrateCarrierFirstEffectivePickup(spec)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	return ports.CarrierFirstEffectivePickupRecord{
		Key:        ports.CarrierFirstEffectivePickupKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Pickup:     pickup,
		RecordedAt: row.recordedAt,
	}, nil
}
