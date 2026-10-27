package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ETAs 实现 ports.ETAStore。版本行只增不改写：Save 追加版本行并把 eta_prediction_current 的显式标记指向它，
// 被接替的预测连同输入与区间留存、按版本读得回；读当前版走标记直取，不扫描历史。标记是唯一的可变处。
// 预测线的键是（租户+包裹+里程碑+来源），承运商提供的与运营企业派生的预测各是一条线，一路的写不触碰另一路。
type ETAs struct {
	db *bentopg.DB
}

func NewETAs(db *bentopg.DB) (*ETAs, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ETAs{db: db}, nil
}

var _ ports.ETAStore = (*ETAs)(nil)

const etaColumns = `v.version_id, v.source, v.inputs_ref, v.model_ref, v.range_from, v.range_to,
		        v.confidence_ref, v.predicted_at, v.prior_version`

// etaRowScanner 是 pgx.Row 与 pgx.Rows 共有的那一半。
type etaRowScanner interface {
	Scan(dest ...any) error
}

func (repository *ETAs) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
) (domain.ETAPrediction, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("find current eta: %w", err)
	}
	eta, found, err := scanETA(parcel, milestone, querier.QueryRow(ctx,
		`SELECT `+etaColumns+`
		   FROM visibility_exception.eta_prediction_current AS c
		   JOIN visibility_exception.eta_prediction_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.parcel_ref = c.parcel_ref
		    AND v.milestone_ref = c.milestone_ref
		    AND v.source = c.source
		    AND v.version_id = c.version_id
		  WHERE c.tenant_id = $1 AND c.parcel_ref = $2 AND c.milestone_ref = $3 AND c.source = $4`,
		tenant.String(), parcel.String(), milestone.String(), source.String(),
	))
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("find current eta: %w", err)
	}
	return eta, found, nil
}

// ListCurrent 用一条语句读出（租户+包裹+里程碑）下各来源线的当前版：两路并存时来自同一份快照。
func (repository *ETAs) ListCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
) ([]domain.ETAPrediction, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list current etas: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+etaColumns+`
		   FROM visibility_exception.eta_prediction_current AS c
		   JOIN visibility_exception.eta_prediction_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.parcel_ref = c.parcel_ref
		    AND v.milestone_ref = c.milestone_ref
		    AND v.source = c.source
		    AND v.version_id = c.version_id
		  WHERE c.tenant_id = $1 AND c.parcel_ref = $2 AND c.milestone_ref = $3
		  ORDER BY c.source`,
		tenant.String(), parcel.String(), milestone.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list current etas: %w", err)
	}
	defer rows.Close()

	var currents []domain.ETAPrediction
	for rows.Next() {
		eta, _, err := scanETA(parcel, milestone, rows)
		if err != nil {
			return nil, fmt.Errorf("list current etas: %w", err)
		}
		currents = append(currents, eta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list current etas: %w", err)
	}
	return currents, nil
}

// LockCurrent 对（租户+包裹+里程碑+来源）这条线持事务级咨询锁到环境事务结束，再读回当前版。写者据此
// 决定重放、刷新还是首版：同一条线上并发的写者在锁上排队，输家醒来读到赢家已提交的当前版。锁与读分两条
// 语句——读已提交下后一条语句取新快照，才看得见赢家刚提交的版本。首版之前线上没有标记行可锁，所以锁的是线的
// 键而不是某一行。锁随事务释放，走 RequireExecutor：没有环境事务时锁在取锁那条语句结束就放掉，静默放过
// 比报错更糟。锁键各维以 chr(31) 分隔后取 hashtext，口径同 TF 的 BookingAnswers.FindAnswersForUpdate，
// 哈希撞上只多等一会儿，不错判。
func (repository *ETAs) LockCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
) (domain.ETAPrediction, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("lock current eta: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('visibility_exception.eta_prediction.line'),
		                              hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		tenant.String(), parcel.String(), milestone.String(), source.String(),
	); err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("lock current eta: 取预测线锁：%w", err)
	}
	eta, found, err := scanETA(parcel, milestone, executor.QueryRow(ctx,
		`SELECT `+etaColumns+`
		   FROM visibility_exception.eta_prediction_current AS c
		   JOIN visibility_exception.eta_prediction_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.parcel_ref = c.parcel_ref
		    AND v.milestone_ref = c.milestone_ref
		    AND v.source = c.source
		    AND v.version_id = c.version_id
		  WHERE c.tenant_id = $1 AND c.parcel_ref = $2 AND c.milestone_ref = $3 AND c.source = $4`,
		tenant.String(), parcel.String(), milestone.String(), source.String(),
	))
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("lock current eta: %w", err)
	}
	return eta, found, nil
}

// FindVersion 按版本读回该预测线名下留存的任一版，当前版或已被接替的皆可。否定结果只回 false，不区分
// 「不存在」「属于另一条线」与「属于另一个租户」（ADR-0029）。
func (repository *ETAs) FindVersion(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
	version domain.ETAVersionID,
) (domain.ETAPrediction, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("find eta by version: %w", err)
	}
	eta, found, err := scanETA(parcel, milestone, querier.QueryRow(ctx,
		`SELECT `+etaColumns+`
		   FROM visibility_exception.eta_prediction_version AS v
		  WHERE v.tenant_id = $1 AND v.parcel_ref = $2 AND v.milestone_ref = $3
		    AND v.source = $4 AND v.version_id = $5`,
		tenant.String(), parcel.String(), milestone.String(), source.String(), version.String(),
	))
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("find eta by version: %w", err)
	}
	return eta, found, nil
}

func scanETA(
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	row etaRowScanner,
) (domain.ETAPrediction, bool, error) {
	var (
		versionID, source, inputs, model, confidence string
		rangeFrom, rangeTo, predictedAt              time.Time
		priorVersion                                 *string
	)
	err := row.Scan(&versionID, &source, &inputs, &model, &rangeFrom, &rangeTo,
		&confidence, &predictedAt, &priorVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ETAPrediction{}, false, nil
	}
	if err != nil {
		return domain.ETAPrediction{}, false, err
	}

	spec := domain.ETAPredictionSpec{
		Parcel:      parcel,
		Milestone:   milestone,
		RangeFrom:   rangeFrom,
		RangeTo:     rangeTo,
		PredictedAt: predictedAt,
	}
	if spec.Version, err = domain.NewETAVersionID(versionID); err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
	}
	if spec.Source, err = etaSourceFrom(source); err != nil {
		return domain.ETAPrediction{}, false, err
	}
	if spec.Inputs, err = domain.NewPredictionInputsReference(inputs); err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
	}
	if spec.Model, err = domain.NewPredictionModelReference(model); err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
	}
	if spec.Confidence, err = domain.NewConfidenceReference(confidence); err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
	}
	var prior domain.ETAVersionID
	if priorVersion != nil {
		if prior, err = domain.NewETAVersionID(*priorVersion); err != nil {
			return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
		}
	}

	eta, err := domain.RehydrateETAPrediction(spec, prior)
	if err != nil {
		return domain.ETAPrediction{}, false, fmt.Errorf("rebuild eta: %w", err)
	}
	return eta, true, nil
}

// Save 追加预测版本并移当前标记：首版只在线上无当前标记时立标记（ON CONFLICT DO NOTHING），刷新只在标记恰指
// 新版本所指前版时移标记（UPDATE ... WHERE version_id = prior 的乐观条件）。零行命中都是
// ports.ErrETACurrentChanged；已写下的版本行随同一事务回滚，不留一版从未成为当前的预测。
//
// 走 RequireExecutor：预测版本与同一步交出的 Outbox 意图必须同生共死。
func (repository *ETAs) Save(
	ctx context.Context,
	tenant domain.TenantID,
	eta domain.ETAPrediction,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save eta: %w", err)
	}

	rangeFrom, rangeTo := eta.Range()
	prior, refreshing := eta.PriorVersion()
	var priorColumn *string
	if refreshing {
		priorColumn = stringPointer(prior.String())
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.eta_prediction_version
			(tenant_id, parcel_ref, milestone_ref, source, version_id, inputs_ref,
			 model_ref, range_from, range_to, confidence_ref, predicted_at, prior_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		tenant.String(),
		eta.Parcel().String(),
		eta.Milestone().String(),
		eta.Source().String(),
		eta.Version().String(),
		eta.Inputs().String(),
		eta.Model().String(),
		rangeFrom,
		rangeTo,
		eta.Confidence().String(),
		eta.PredictedAt(),
		priorColumn,
	); err != nil {
		return fmt.Errorf("save eta version: %w", err)
	}

	if !refreshing {
		tag, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.eta_prediction_current
				(tenant_id, parcel_ref, milestone_ref, source, version_id)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT DO NOTHING`,
			tenant.String(), eta.Parcel().String(), eta.Milestone().String(),
			eta.Source().String(), eta.Version().String(),
		)
		if err != nil {
			return fmt.Errorf("save eta current marker: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("save eta: %w", ports.ErrETACurrentChanged)
		}
		return nil
	}

	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.eta_prediction_current
		    SET version_id = $5
		  WHERE tenant_id = $1
		    AND parcel_ref = $2
		    AND milestone_ref = $3
		    AND source = $4
		    AND version_id = $6`,
		tenant.String(), eta.Parcel().String(), eta.Milestone().String(),
		eta.Source().String(), eta.Version().String(), prior.String(),
	)
	if err != nil {
		return fmt.Errorf("save eta current marker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("save eta: %w", ports.ErrETACurrentChanged)
	}
	return nil
}

func etaSourceFrom(value string) (domain.ETASourceKind, error) {
	switch value {
	case "CARRIER_PROVIDED":
		return domain.CarrierProvidedETA, nil
	case "OPERATOR_DERIVED":
		return domain.OperatorDerivedETA, nil
	default:
		return domain.ETASourceKindInvalid,
			fmt.Errorf("visibility exception postgres: unknown eta source %q", value)
	}
}

// VisibilityGaps 实现 ports.VisibilityGapStore。键含窗口规则版本：新窗口是另一次
// 判断，不覆盖原行。缺口只增——同键重写走 ON CONFLICT DO NOTHING。
type VisibilityGaps struct {
	db *bentopg.DB
}

func NewVisibilityGaps(db *bentopg.DB) (*VisibilityGaps, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &VisibilityGaps{db: db}, nil
}

func (repository *VisibilityGaps) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	expectation domain.ExpectedObservationReference,
	windowRule domain.ObservationWindowReference,
) (domain.VisibilityGap, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.VisibilityGap{}, false, fmt.Errorf("find visibility gap: %w", err)
	}

	var windowEnd, formedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT window_end, formed_at
		   FROM visibility_exception.visibility_gap
		  WHERE tenant_id = $1 AND parcel_ref = $2
		    AND expectation_ref = $3 AND window_rule_ref = $4`,
		tenant.String(), parcel.String(), expectation.String(), windowRule.String(),
	).Scan(&windowEnd, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VisibilityGap{}, false, nil
	}
	if err != nil {
		return domain.VisibilityGap{}, false, fmt.Errorf("find visibility gap: %w", err)
	}

	gap, err := domain.FormVisibilityGap(parcel, expectation, windowRule, windowEnd, formedAt)
	if err != nil {
		return domain.VisibilityGap{}, false, fmt.Errorf("rebuild visibility gap: %w", err)
	}
	return gap, true, nil
}

// Save 写下已成立的缺口。同键已有记录时静默收下原判断——新窗口是另一键，不会走到
// 这里覆盖；同键重放不是第二份缺口。
func (repository *VisibilityGaps) Save(
	ctx context.Context,
	tenant domain.TenantID,
	gap domain.VisibilityGap,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save visibility gap: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.visibility_gap
			(tenant_id, parcel_ref, expectation_ref, window_rule_ref, window_end, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, parcel_ref, expectation_ref, window_rule_ref) DO NOTHING`,
		tenant.String(),
		gap.Parcel().String(),
		gap.Expectation().String(),
		gap.WindowRule().String(),
		gap.WindowEnd(),
		gap.FormedAt(),
	)
	if err != nil {
		return fmt.Errorf("save visibility gap: %w", err)
	}
	return nil
}

// CustomerNotifications 实现 ports.CustomerNotificationStore。按披露身份三维定位；
// 过程节点 jsonb 数组随 Save UPSERT 增长，历史节点不覆盖。
type CustomerNotifications struct {
	db *bentopg.DB
}

func NewCustomerNotifications(db *bentopg.DB) (*CustomerNotifications, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &CustomerNotifications{db: db}, nil
}

type notificationMilestoneRow struct {
	Milestone  string    `json:"milestone"`
	RecordedAt time.Time `json:"recordedAt"`
}

func (repository *CustomerNotifications) FindByDisclosure(
	ctx context.Context,
	tenant domain.TenantID,
	disclosure domain.DisclosureDecision,
) (*domain.CustomerNotification, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find customer notification: %w", err)
	}

	var (
		notificationID, policy, conclusion, content, channel, obligation string
		deadline                                                         time.Time
		milestonesRaw                                                    []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT notification_id, disclosure_policy_ref, disclosure_conclusion,
		        content_ref, deadline, channel_ref, obligation_ref, milestones
		   FROM visibility_exception.customer_notification
		  WHERE tenant_id = $1 AND customer_ref = $2
		    AND episode_id = $3 AND decided_at = $4`,
		tenant.String(),
		disclosure.Customer().String(),
		disclosure.Episode().String(),
		disclosure.DecidedAt(),
	).Scan(&notificationID, &policy, &conclusion, &content, &deadline,
		&channel, &obligation, &milestonesRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find customer notification: %w", err)
	}

	notification, err := rebuildCustomerNotification(
		notificationID, disclosure, policy, conclusion, content,
		deadline, channel, obligation, milestonesRaw,
	)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild customer notification: %w", err)
	}
	return notification, true, nil
}

// Save 落通知的当前过程历史。过程节点回填按主键 UPSERT；首发走披露身份三维唯一
// 约束的 ON CONFLICT DO NOTHING：并发第二份不同 notification_id、同一披露三维交回
// AlreadyRecorded，事务保持可用。两条冲突路径不压成一个 ON CONFLICT。
func (repository *CustomerNotifications) Save(
	ctx context.Context,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) (ports.NotificationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.NotificationSaveOutcomeInvalid, fmt.Errorf("save customer notification: %w", err)
	}
	if notification == nil {
		return ports.NotificationSaveOutcomeInvalid, fmt.Errorf("save customer notification: notification is nil")
	}

	found, err := repository.notificationExists(ctx, tenant, notification.ID())
	if err != nil {
		return ports.NotificationSaveOutcomeInvalid, err
	}
	if found {
		if err := repository.upsertByPrimaryKey(ctx, executor, tenant, notification); err != nil {
			return ports.NotificationSaveOutcomeInvalid, err
		}
		return ports.NotificationSaved, nil
	}

	tag, err := repository.insertByDisclosure(ctx, executor, tenant, notification)
	if err != nil {
		return ports.NotificationSaveOutcomeInvalid, err
	}
	if tag == 0 {
		return ports.NotificationAlreadyRecorded, nil
	}
	return ports.NotificationSaved, nil
}

func (repository *CustomerNotifications) notificationExists(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.NotificationID,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("find customer notification: %w", err)
	}
	var exists bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM visibility_exception.customer_notification
			 WHERE tenant_id = $1 AND notification_id = $2)`,
		tenant.String(), id.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("find customer notification: %w", err)
	}
	return exists, nil
}

func (repository *CustomerNotifications) upsertByPrimaryKey(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) error {
	args, err := notificationArgs(tenant, notification)
	if err != nil {
		return fmt.Errorf("upsert customer notification: %w", err)
	}
	_, err = executor.Exec(ctx,
		notificationInsertSQL+`
		 ON CONFLICT (tenant_id, notification_id) DO UPDATE SET
			milestones = EXCLUDED.milestones,
			deadline = EXCLUDED.deadline,
			channel_ref = EXCLUDED.channel_ref,
			obligation_ref = EXCLUDED.obligation_ref`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("upsert customer notification: %w", err)
	}
	return nil
}

func (repository *CustomerNotifications) insertByDisclosure(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) (int64, error) {
	args, err := notificationArgs(tenant, notification)
	if err != nil {
		return 0, fmt.Errorf("insert customer notification: %w", err)
	}
	tag, err := executor.Exec(ctx,
		notificationInsertSQL+`
		 ON CONFLICT (tenant_id, customer_ref, episode_id, decided_at) DO NOTHING`,
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("insert customer notification: %w", err)
	}
	return tag.RowsAffected(), nil
}

const notificationInsertSQL = `INSERT INTO visibility_exception.customer_notification
			(tenant_id, notification_id, customer_ref, episode_id, decided_at,
			 disclosure_policy_ref, disclosure_conclusion, content_ref, deadline,
			 channel_ref, obligation_ref, milestones)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

func notificationArgs(tenant domain.TenantID, notification *domain.CustomerNotification) ([]any, error) {
	milestonesRaw, err := marshalNotificationMilestones(notification)
	if err != nil {
		return nil, err
	}
	disclosure := notification.Disclosure()
	return []any{
		tenant.String(),
		notification.ID().String(),
		disclosure.Customer().String(),
		disclosure.Episode().String(),
		disclosure.DecidedAt(),
		disclosure.Policy().String(),
		disclosure.Conclusion().String(),
		notification.Content().String(),
		notification.Deadline(),
		notification.Channel().String(),
		notification.Obligation().String(),
		milestonesRaw,
	}, nil
}

func marshalNotificationMilestones(notification *domain.CustomerNotification) ([]byte, error) {
	milestones := notification.Milestones()
	recorded := notification.RecordedAt()
	if len(milestones) != len(recorded) {
		return nil, fmt.Errorf("milestone count %d disagrees with recorded-at count %d",
			len(milestones), len(recorded))
	}
	rows := make([]notificationMilestoneRow, 0, len(milestones))
	for i, milestone := range milestones {
		rows = append(rows, notificationMilestoneRow{
			Milestone:  milestone.String(),
			RecordedAt: recorded[i],
		})
	}
	return json.Marshal(rows)
}

func rebuildCustomerNotification(
	notificationID string,
	disclosure domain.DisclosureDecision,
	policy, conclusion, content string,
	deadline time.Time,
	channel, obligation string,
	milestonesRaw []byte,
) (*domain.CustomerNotification, error) {
	if conclusion != domain.DiscloseToCustomer.String() {
		return nil, fmt.Errorf("notification conclusion %q is not a disclose decision", conclusion)
	}
	if disclosure.Policy().String() != policy {
		return nil, fmt.Errorf("stored disclosure policy disagrees with the lookup key")
	}
	storedContent, disclosed := disclosure.Content()
	if !disclosed || storedContent.String() != content {
		return nil, fmt.Errorf("stored disclosure content disagrees with the lookup key")
	}

	id, err := domain.NewNotificationID(notificationID)
	if err != nil {
		return nil, err
	}
	channelRef, err := domain.NewNotificationChannelReference(channel)
	if err != nil {
		return nil, err
	}
	obligationRef, err := domain.NewDisclosurePolicyReference(obligation)
	if err != nil {
		return nil, err
	}

	var rows []notificationMilestoneRow
	if err := json.Unmarshal(milestonesRaw, &rows); err != nil {
		return nil, fmt.Errorf("milestones: %w", err)
	}
	if len(rows) == 0 || rows[0].Milestone != domain.NotificationGenerated.String() {
		return nil, fmt.Errorf("notification must start with a generated milestone")
	}

	notification, err := domain.GenerateNotification(
		id, disclosure, deadline, channelRef, obligationRef, rows[0].RecordedAt,
	)
	if err != nil {
		return nil, err
	}
	for _, row := range rows[1:] {
		milestone, err := notificationMilestoneFrom(row.Milestone)
		if err != nil {
			return nil, err
		}
		if err := notification.RecordMilestone(milestone, row.RecordedAt); err != nil {
			return nil, err
		}
	}
	return notification, nil
}

func notificationMilestoneFrom(value string) (domain.NotificationMilestone, error) {
	switch value {
	case "GENERATED":
		return domain.NotificationGenerated, nil
	case "SUBMITTED_TO_CHANNEL":
		return domain.NotificationSubmittedToChannel, nil
	case "CHANNEL_ACCEPTED":
		return domain.NotificationChannelAccepted, nil
	case "DELIVERED":
		return domain.NotificationDelivered, nil
	case "FAILED":
		return domain.NotificationFailed, nil
	case "CUSTOMER_CONFIRMED":
		return domain.CustomerConfirmed, nil
	default:
		return domain.NotificationMilestoneInvalid,
			fmt.Errorf("visibility exception postgres: unknown notification milestone %q", value)
	}
}
