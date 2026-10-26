package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DepartureGateReasks 实现 ports.DepartureGateReaskRegistry：一次补问一行（迁移 0052，ADR-0305）。
//
// **只插不改。** 本类型上没有任何 UPDATE：出发首版那一份读数留在移动事实那一行上，补问问来的每一份各占一行。读数两列与移动事实
// 那一行同一个形状，装回走同一份翻译（departureGateReadingDetailOf / departureGateReadingFrom）：两份各写一遍，就会有一份先漏掉
// 日后加的那一位。
type DepartureGateReasks struct {
	db *bentopg.DB
}

func NewDepartureGateReasks(db *bentopg.DB) (*DepartureGateReasks, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DepartureGateReasks{db: db}, nil
}

var _ ports.DepartureGateReaskRegistry = (*DepartureGateReasks)(nil)

const departureGateReaskColumns = `fact_ref, version, object_ref, sequence, gate_reading, gate_reading_detail, trigger_ref, recorded_at`

// FindByKey 按键取回一次补问。否定结果只回 false。
func (repository *DepartureGateReasks) FindByKey(
	ctx context.Context,
	key ports.DepartureGateReaskKey,
) (ports.DepartureGateReaskRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DepartureGateReaskRecord{}, false, fmt.Errorf("find departure gate reask: %w", err)
	}
	var row departureGateReaskRow
	err = querier.QueryRow(ctx,
		`SELECT `+departureGateReaskColumns+`
		   FROM transport_fulfillment.departure_gate_reask
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3 AND object_ref = $4 AND sequence = $5`,
		key.TenantID.String(), key.Fact.String(), key.Version.String(), key.Object.String(), key.Sequence,
	).Scan(row.targets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DepartureGateReaskRecord{}, false, nil
	}
	if err != nil {
		return ports.DepartureGateReaskRecord{}, false, fmt.Errorf("find departure gate reask: %w", err)
	}
	record, err := row.record(key.TenantID)
	if err != nil {
		return ports.DepartureGateReaskRecord{}, false, fmt.Errorf("find departure gate reask: %w: %w",
			ports.ErrDepartureGateReaskRecordUnreadable, err)
	}
	return record, true, nil
}

// FindByDeparture 列出一条出发这一版上的全部补问，按落册时刻、对象、次序排（端口头注的排序契约）。一行装不回整次报错，不跳过：
// 跳过会让调用方把「漏了一次」当成「列全了」。
func (repository *DepartureGateReasks) FindByDeparture(
	ctx context.Context,
	departure ports.MovementFactKey,
) ([]ports.DepartureGateReaskRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find departure gate reasks: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+departureGateReaskColumns+`
		   FROM transport_fulfillment.departure_gate_reask
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3
		  ORDER BY recorded_at, object_ref, sequence`,
		departure.TenantID.String(), departure.Fact.String(), departure.Version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find departure gate reasks: %w", err)
	}
	defer rows.Close()
	records := []ports.DepartureGateReaskRecord{}
	for rows.Next() {
		var row departureGateReaskRow
		if err := rows.Scan(row.targets()...); err != nil {
			return nil, fmt.Errorf("find departure gate reasks: %w", err)
		}
		record, err := row.record(departure.TenantID)
		if err != nil {
			return nil, fmt.Errorf("find departure gate reasks: %w: %w", ports.ErrDepartureGateReaskRecordUnreadable, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find departure gate reasks: %w", err)
	}
	return records, nil
}

// Save 追加一次补问。撞键答`已登记`（ADR-0031）：另一方先补了这一次，不覆盖先到者。写口要求环境事务——补问那一行与它的信封
// 同一笔入队（ADR-0305）。
func (repository *DepartureGateReasks) Save(
	ctx context.Context,
	record ports.DepartureGateReaskRecord,
) (ports.DepartureGateReaskSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DepartureGateReaskSaveOutcomeInvalid, fmt.Errorf("save departure gate reask: %w", err)
	}
	if record.Key.Sequence < 1 || record.Reading.Answer() == domain.DepartureGateAnswerInvalid ||
		record.Trigger == "" || record.RecordedAt.IsZero() {
		return ports.DepartureGateReaskSaveOutcomeInvalid, fmt.Errorf("save departure gate reask: incomplete record")
	}
	detail, err := json.Marshal(departureGateReadingDetailOf(record.Reading))
	if err != nil {
		return ports.DepartureGateReaskSaveOutcomeInvalid, fmt.Errorf("save departure gate reask: gate reading detail: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.departure_gate_reask
		     (tenant_id, fact_ref, version, object_ref, sequence, gate_reading, gate_reading_detail, trigger_ref, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Key.Version.String(),
		record.Key.Object.String(),
		record.Key.Sequence,
		record.Reading.Answer().String(),
		detail,
		record.Trigger,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.DepartureGateReaskSaveOutcomeInvalid, fmt.Errorf("save departure gate reask: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DepartureGateReaskAlreadyRegistered, nil
	}
	return ports.DepartureGateReaskSaved, nil
}

// departureGateReaskRow 是一行装回之前的原样各列，次序与 departureGateReaskColumns 一一对应。租户不在其中，由调用方给。
type departureGateReaskRow struct {
	factRef, version, objectRef, gateReading, trigger string
	sequence                                          int
	gateReadingDetail                                 []byte
	recordedAt                                        time.Time
}

func (row *departureGateReaskRow) targets() []any {
	return []any{
		&row.factRef, &row.version, &row.objectRef, &row.sequence, &row.gateReading, &row.gateReadingDetail, &row.trigger,
		&row.recordedAt,
	}
}

// record 逐列走各自的构造门装回。
func (row departureGateReaskRow) record(tenant domain.TenantID) (ports.DepartureGateReaskRecord, error) {
	none := ports.DepartureGateReaskRecord{}
	fact, err := domain.NewMovementFactReference(row.factRef)
	if err != nil {
		return none, err
	}
	version, err := domain.NewMovementFactVersion(row.version)
	if err != nil {
		return none, err
	}
	object, err := domain.NewCarriedObjectReference(row.objectRef)
	if err != nil {
		return none, err
	}
	if row.sequence < 1 || row.trigger == "" {
		return none, fmt.Errorf("departure gate reask: sequence %d trigger %q", row.sequence, row.trigger)
	}
	reading, err := departureGateReadingFrom(row.gateReading, row.gateReadingDetail)
	if err != nil {
		return none, err
	}
	return ports.DepartureGateReaskRecord{
		Key: ports.DepartureGateReaskKey{
			TenantID: tenant, Fact: fact, Version: version, Object: object, Sequence: row.sequence,
		},
		Reading:    reading,
		Trigger:    row.trigger,
		RecordedAt: row.recordedAt.UTC(),
	}, nil
}
