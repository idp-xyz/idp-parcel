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

// ScheduleTemplates 实现 ports.ScheduleTemplateRegistry：周期安排一版一行，只插不改，当前版是版本号最大的那一行
// （迁移 0024，ADR-0201）。
type ScheduleTemplates struct {
	db *bentopg.DB
}

var _ ports.ScheduleTemplateRegistry = (*ScheduleTemplates)(nil)

func NewScheduleTemplates(db *bentopg.DB) (*ScheduleTemplates, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ScheduleTemplates{db: db}, nil
}

const scheduleTemplateColumns = `tenant_id, template_code, template_version, connection_code, from_node, to_node,
		        weekdays, departure_local_minute, duration_minutes, time_zone, service_scope_ref,
		        effective_from, effective_until, content_digest, recorded_at`

// FindCurrentTemplate 按（租户+代码）取回当前版。否定结果只回 false。
func (repository *ScheduleTemplates) FindCurrentTemplate(
	ctx context.Context,
	key ports.ScheduleTemplateKey,
) (ports.ScheduleTemplateRecord, bool, error) {
	records, err := queryScheduleTemplates(ctx, repository.db, "find current schedule template",
		`SELECT `+scheduleTemplateColumns+`
		   FROM transport_fulfillment.schedule_template
		  WHERE tenant_id = $1
		    AND template_code = $2
		  ORDER BY template_version DESC
		  LIMIT 1`,
		key.TenantID.String(),
		key.Code.String(),
	)
	if err != nil || len(records) == 0 {
		return ports.ScheduleTemplateRecord{}, false, err
	}
	return records[0], true, nil
}

// SaveTemplateVersion 落记录上那一版。同一（代码，版本）已有行时答`版本已占`，不覆盖先到者。
func (repository *ScheduleTemplates) SaveTemplateVersion(
	ctx context.Context,
	record ports.ScheduleTemplateRecord,
) (ports.ScheduleTemplateSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ScheduleTemplateSaveOutcomeInvalid, fmt.Errorf("save schedule template: %w", err)
	}

	template := record.Template
	content := template.Content()
	weekdays := make([]string, 0, len(content.Weekdays))
	for _, weekday := range content.Weekdays {
		weekdays = append(weekdays, domain.ScheduleWeekdayWord(weekday))
	}
	var effectiveUntil *string
	if until := content.EffectiveUntil.String(); until != "" {
		effectiveUntil = &until
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.schedule_template
			(tenant_id, template_code, template_version, connection_code, from_node, to_node,
			 weekdays, departure_local_minute, duration_minutes, time_zone, service_scope_ref,
			 effective_from, effective_until, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::date, $13::date, $14, $15)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Code.String(),
		int(template.Version()),
		content.Direction.Connection().String(),
		content.Direction.From().String(),
		content.Direction.To().String(),
		weekdays,
		content.DepartureLocalMinute,
		int64(content.Duration/time.Minute),
		content.TimeZone,
		content.ServiceScope.String(),
		content.EffectiveFrom.String(),
		effectiveUntil,
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ScheduleTemplateSaveOutcomeInvalid, fmt.Errorf("save schedule template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ScheduleTemplateVersionTaken, nil
	}
	return ports.ScheduleTemplateSaved, nil
}

// ScheduleCandidates 实现 ports.ScheduleCandidateSource：准备评估候选时读周期安排与具体班次两本册子（ADR-0201
// 决定五、六）。两问都只看当前版——被后一版顶掉的那一版不再是任何候选的来源。
type ScheduleCandidates struct {
	db *bentopg.DB
}

var _ ports.ScheduleCandidateSource = (*ScheduleCandidates)(nil)

func NewScheduleCandidates(db *bentopg.DB) (*ScheduleCandidates, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ScheduleCandidates{db: db}, nil
}

// currentTemplates 与 currentSchedules 取每个身份的当前版；连接代码按当前版筛——一份周期安排改登到别的连接上，
// 就不再是原连接的候选来源。
const (
	currentTemplates = `SELECT DISTINCT ON (template_code) ` + scheduleTemplateColumns + `
		   FROM transport_fulfillment.schedule_template
		  WHERE tenant_id = $1
		  ORDER BY template_code, template_version DESC`
	currentSchedules = `SELECT DISTINCT ON (schedule_id) ` + transportScheduleColumns + `
		   FROM transport_fulfillment.transport_schedule
		  WHERE tenant_id = $1
		  ORDER BY schedule_id, schedule_version DESC`
)

// ConnectionHasSchedules 答该连接上有没有任何当前版的周期安排或具体班次，不问生效区间也不问时刻：过期的周期安排
// 照样说明「这条连接配过」，评估方据此答`无当前运输机会`而不是`未配置`。
func (source *ScheduleCandidates) ConnectionHasSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	connection domain.NetworkConnectionReference,
) (bool, error) {
	querier, err := source.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("connection has schedules: %w", err)
	}
	var configured bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		        SELECT 1 FROM (`+currentTemplates+`) AS current_template WHERE current_template.connection_code = $2)
		     OR EXISTS (
		        SELECT 1 FROM (`+currentSchedules+`) AS current_schedule WHERE current_schedule.connection_code = $2)`,
		tenant.String(),
		connection.String(),
	).Scan(&configured)
	if err != nil {
		return false, fmt.Errorf("connection has schedules: %w", err)
	}
	return configured, nil
}

// ListCurrentTemplatesOnConnection 交回该连接上全部周期安排的当前版，按代码排。
func (source *ScheduleCandidates) ListCurrentTemplatesOnConnection(
	ctx context.Context,
	tenant domain.TenantID,
	connection domain.NetworkConnectionReference,
) ([]ports.ScheduleTemplateRecord, error) {
	return queryScheduleTemplates(ctx, source.db, "list current schedule templates on connection",
		`SELECT * FROM (`+currentTemplates+`) AS current_template
		  WHERE current_template.connection_code = $2
		  ORDER BY current_template.template_code`,
		tenant.String(),
		connection.String(),
	)
}

// ListCurrentSchedulesDepartingWithin 交回该连接上出发时刻落在 [from, to] 的具体班次当前版，按出发先后、同刻按
// 班次标识排。
func (source *ScheduleCandidates) ListCurrentSchedulesDepartingWithin(
	ctx context.Context,
	tenant domain.TenantID,
	connection domain.NetworkConnectionReference,
	from, to time.Time,
) ([]ports.ScheduleRecord, error) {
	return queryTransportSchedules(ctx, source.db, "list current schedules departing within",
		`SELECT * FROM (`+currentSchedules+`) AS current_schedule
		  WHERE current_schedule.connection_code = $2
		    AND current_schedule.departs_at >= $3
		    AND current_schedule.departs_at <= $4
		  ORDER BY current_schedule.departs_at, current_schedule.schedule_id`,
		tenant.String(),
		connection.String(),
		from.UTC(),
		to.UTC(),
	)
}

// FindCurrentSchedules 按身份交回已登记班次的当前版，不问连接与时刻；没登过的身份不出现在结果里。
func (source *ScheduleCandidates) FindCurrentSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.ScheduleRecord, error) {
	if len(schedules) == 0 {
		return nil, nil
	}
	identities := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		identities = append(identities, schedule.String())
	}
	return queryTransportSchedules(ctx, source.db, "find current schedules",
		`SELECT DISTINCT ON (schedule_id) `+transportScheduleColumns+`
		   FROM transport_fulfillment.transport_schedule
		  WHERE tenant_id = $1
		    AND schedule_id = ANY($2)
		  ORDER BY schedule_id, schedule_version DESC`,
		tenant.String(),
		identities,
	)
}

func queryScheduleTemplates(
	ctx context.Context,
	db *bentopg.DB,
	verb string,
	sql string,
	args ...any,
) ([]ports.ScheduleTemplateRecord, error) {
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	defer rows.Close()

	var records []ports.ScheduleTemplateRecord
	for rows.Next() {
		record, err := scanScheduleTemplate(rows)
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

// scanScheduleTemplate 把一行装回一版周期安排：逐格过构造门，再交重建门（ADR-0028）。
func scanScheduleTemplate(rows pgx.Rows) (ports.ScheduleTemplateRecord, error) {
	var (
		tenantID, code, connection, from, to, timeZone, serviceScope, digest string
		version, departureMinute                                             int
		durationMinutes                                                      int64
		weekdayWords                                                         []string
		effectiveFrom, recordedAt                                            time.Time
		effectiveUntil                                                       *time.Time
	)
	if err := rows.Scan(
		&tenantID, &code, &version, &connection, &from, &to,
		&weekdayWords, &departureMinute, &durationMinutes, &timeZone, &serviceScope,
		&effectiveFrom, &effectiveUntil, &digest, &recordedAt,
	); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}

	spec := domain.ScheduleTemplateSpec{}
	var err error
	if spec.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	if spec.Code, err = domain.NewScheduleTemplateCode(code); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	if spec.Content.Direction, err = domain.NewScheduleDirection(connection, from, to); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	if spec.Content.ServiceScope, err = domain.NewScheduleServiceScopeReference(serviceScope); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	for _, word := range weekdayWords {
		weekday, err := domain.ParseScheduleWeekday(word)
		if err != nil {
			return ports.ScheduleTemplateRecord{}, err
		}
		spec.Content.Weekdays = append(spec.Content.Weekdays, weekday)
	}
	spec.Content.DepartureLocalMinute = departureMinute
	spec.Content.Duration = time.Duration(durationMinutes) * time.Minute
	spec.Content.TimeZone = timeZone
	if spec.Content.EffectiveFrom, err = domain.ParseRunningDate(effectiveFrom.Format("2006-01-02")); err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	if effectiveUntil != nil {
		if spec.Content.EffectiveUntil, err = domain.ParseRunningDate(effectiveUntil.Format("2006-01-02")); err != nil {
			return ports.ScheduleTemplateRecord{}, err
		}
	}

	template, err := domain.RehydrateScheduleTemplate(spec, domain.ScheduleTemplateVersion(version))
	if err != nil {
		return ports.ScheduleTemplateRecord{}, err
	}
	return ports.ScheduleTemplateRecord{
		Key:           ports.ScheduleTemplateKey{TenantID: spec.TenantID, Code: spec.Code},
		ContentDigest: digest,
		Template:      template,
		RecordedAt:    recordedAt.UTC(),
	}, nil
}
