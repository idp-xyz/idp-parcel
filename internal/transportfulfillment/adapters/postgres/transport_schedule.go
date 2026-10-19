package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// TransportSchedules 实现 ports.TransportScheduleStore（写入代数同 ADR-0031）。一个班次一版一行，只插不改；当前版是
// 版本号最大的那一行（迁移 0024）。班次行不承载容量或订舱。
type TransportSchedules struct {
	db *bentopg.DB
}

var (
	_ ports.TransportScheduleStore  = (*TransportSchedules)(nil)
	_ ports.TransportScheduleReader = (*TransportSchedules)(nil)
)

func NewTransportSchedules(db *bentopg.DB) (*TransportSchedules, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &TransportSchedules{db: db}, nil
}

const transportScheduleColumns = `tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
		        departs_at, arrives_at, service_scope_ref, origin, template_code, template_version, running_date,
		        content_digest, recorded_at`

// FindByKey 按（租户+班次）取回当前版。否定结果只回 false。读回经重建门。
func (repository *TransportSchedules) FindByKey(
	ctx context.Context,
	key ports.ScheduleKey,
) (ports.ScheduleRecord, bool, error) {
	records, err := queryTransportSchedules(ctx, repository.db, "find transport schedule",
		`SELECT `+transportScheduleColumns+`
		   FROM transport_fulfillment.transport_schedule
		  WHERE tenant_id = $1
		    AND schedule_id = $2
		  ORDER BY schedule_version DESC
		  LIMIT 1`,
		key.TenantID.String(),
		key.Schedule.String(),
	)
	if err != nil || len(records) == 0 {
		return ports.ScheduleRecord{}, false, err
	}
	return records[0], true, nil
}

// Save 落记录上那一版。同一（班次，版本）已有行时答`已建立`，不覆盖先到者——撞的是另一方先落的同一版本号，
// 重放还是另一份内容由编排读回当前版比对。
func (repository *TransportSchedules) Save(
	ctx context.Context,
	record ports.ScheduleRecord,
) (ports.ScheduleSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ScheduleSaveOutcomeInvalid, fmt.Errorf("save transport schedule: %w", err)
	}

	schedule := record.Schedule
	var templateCode *string
	var templateVersion *int
	var runningDate *string
	if code, version, date, materialized := schedule.Origin().Template(); materialized {
		codeText, versionNumber, dateText := code.String(), int(version), date.String()
		templateCode, templateVersion, runningDate = &codeText, &versionNumber, &dateText
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
			 departs_at, arrives_at, service_scope_ref, origin, template_code, template_version, running_date,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::date, $14, $15)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Schedule.String(),
		int(schedule.Version()),
		schedule.Direction().Connection().String(),
		schedule.Direction().From().String(),
		schedule.Direction().To().String(),
		schedule.DepartsAt().UTC(),
		schedule.ArrivesAt().UTC(),
		schedule.ServiceScope().String(),
		schedule.Origin().Kind().String(),
		templateCode,
		templateVersion,
		runningDate,
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

func queryTransportSchedules(
	ctx context.Context,
	db *bentopg.DB,
	verb string,
	sql string,
	args ...any,
) ([]ports.ScheduleRecord, error) {
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	defer rows.Close()

	var records []ports.ScheduleRecord
	for rows.Next() {
		record, err := scanTransportSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	return records, nil
}

// scanTransportSchedule 把一行装回一版班次：逐格过构造门，再交重建门核身份与来历——不按列直接拼结构体（ADR-0028）。
func scanTransportSchedule(rows pgx.Rows) (ports.ScheduleRecord, error) {
	var (
		tenantID, scheduleID, connection, from, to, serviceScope, originText, digest string
		version                                                                      int
		departsAt, arrivesAt, recordedAt                                             time.Time
		templateCode                                                                 *string
		templateVersion                                                              *int
		runningDate                                                                  *time.Time
	)
	if err := rows.Scan(
		&tenantID, &scheduleID, &version, &connection, &from, &to,
		&departsAt, &arrivesAt, &serviceScope, &originText, &templateCode, &templateVersion, &runningDate,
		&digest, &recordedAt,
	); err != nil {
		return ports.ScheduleRecord{}, err
	}

	spec := domain.RehydrateTransportScheduleSpec{Version: domain.ScheduleVersion(version)}
	var err error
	if spec.Schedule.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return ports.ScheduleRecord{}, err
	}
	if spec.Schedule.Schedule, err = domain.NewScheduleReference(scheduleID); err != nil {
		return ports.ScheduleRecord{}, err
	}
	if spec.Schedule.Direction, err = domain.NewScheduleDirection(connection, from, to); err != nil {
		return ports.ScheduleRecord{}, err
	}
	if spec.Schedule.ServiceScope, err = domain.NewScheduleServiceScopeReference(serviceScope); err != nil {
		return ports.ScheduleRecord{}, err
	}
	spec.Schedule.DepartsAt = departsAt
	spec.Schedule.ArrivesAt = arrivesAt
	if spec.OriginKind, err = domain.ParseScheduleOriginKind(originText); err != nil {
		return ports.ScheduleRecord{}, err
	}
	if templateCode != nil {
		if spec.Template, err = domain.NewScheduleTemplateCode(*templateCode); err != nil {
			return ports.ScheduleRecord{}, err
		}
	}
	if templateVersion != nil {
		spec.TemplateVersion = domain.ScheduleTemplateVersion(*templateVersion)
	}
	if runningDate != nil {
		if spec.RunningDate, err = domain.ParseRunningDate(runningDate.Format("2006-01-02")); err != nil {
			return ports.ScheduleRecord{}, err
		}
	}

	schedule, err := domain.RehydrateTransportSchedule(spec)
	if err != nil {
		return ports.ScheduleRecord{}, err
	}
	return ports.ScheduleRecord{
		Key:           ports.ScheduleKey{TenantID: spec.Schedule.TenantID, Schedule: spec.Schedule.Schedule},
		ContentDigest: digest,
		Schedule:      schedule,
		RecordedAt:    recordedAt.UTC(),
	}, nil
}
