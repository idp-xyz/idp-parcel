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

// TransportSchedules 实现 ports.TransportScheduleStore（写入代数同 ADR-0031）。
// 同一班次标识只建一次；班次行不承载容量或订舱。
type TransportSchedules struct {
	db *bentopg.DB
}

func NewTransportSchedules(db *bentopg.DB) (*TransportSchedules, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &TransportSchedules{db: db}, nil
}

// FindByKey 按（租户+班次）取回已建立班次。否定结果只回 false。读回经公开构造门。
func (repository *TransportSchedules) FindByKey(
	ctx context.Context,
	key ports.ScheduleKey,
) (ports.ScheduleRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ScheduleRecord{}, false, fmt.Errorf("find transport schedule: %w", err)
	}

	var direction, digest string
	var departsAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT direction, departs_at, content_digest, recorded_at
		   FROM transport_fulfillment.transport_schedule
		  WHERE tenant_id = $1
		    AND schedule_id = $2`,
		key.TenantID.String(),
		key.Schedule.String(),
	).Scan(&direction, &departsAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ScheduleRecord{}, false, nil
	}
	if err != nil {
		return ports.ScheduleRecord{}, false, fmt.Errorf("find transport schedule: %w", err)
	}

	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:  key.TenantID,
		Schedule:  key.Schedule,
		Direction: direction,
		DepartsAt: departsAt,
	})
	if err != nil {
		return ports.ScheduleRecord{}, false, fmt.Errorf("find transport schedule: %w", err)
	}
	return ports.ScheduleRecord{
		Key:           key,
		ContentDigest: digest,
		Schedule:      schedule,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次班次建立。同标识已有记录时答`已建立`，不覆盖先到者。
func (repository *TransportSchedules) Save(
	ctx context.Context,
	record ports.ScheduleRecord,
) (ports.ScheduleSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ScheduleSaveOutcomeInvalid, fmt.Errorf("save transport schedule: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, direction, departs_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Schedule.String(),
		record.Schedule.Direction(),
		record.Schedule.DepartsAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ScheduleSaveOutcomeInvalid, fmt.Errorf("save transport schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ScheduleAlreadyEstablished, nil
	}
	return ports.ScheduleSaved, nil
}
