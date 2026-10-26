package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReviewCatalogue 实现 ports.ReviewCatalogueRead：运输履约查阅页的列表读面
// （ADR-0077，票 admin-skeleton-closure-batch/05）。
//
// 目录上列是照实转写，不经领域重建门：重建是写路与按键读回的纪律（坏行要在那里
// 响亮），检索列面把登记的字段原样透出。例外是订舱册：状态词要从当前那一份答复读出，
// 而当前那一份由领域选（bookingStanding），答复行因此先经重建门读回；转写的各列照旧
// 原样透出。容量四量在 SQL 里按预占子表求和——求和的三个口径（预占量、已释放、
// 实际使用）逐维独立，读口不互相抵扣，也不代算可用量。
//
// 所有语句显式携带租户条件（作用域不是过滤器而是身份的一部分）；读走 ReadExecutor，
// 事务外用显式注入的连接池。
type ReviewCatalogue struct {
	db *bentopg.DB
}

var _ ports.ReviewCatalogueRead = (*ReviewCatalogue)(nil)

func NewReviewCatalogue(db *bentopg.DB) (*ReviewCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ReviewCatalogue{db: db}, nil
}

// ListTransportSchedules 上列班次登记册：每个班次只列当前版，晚出发的在前，同刻按班次标识稳定排序。
func (catalogue *ReviewCatalogue) ListTransportSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TransportScheduleCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list transport schedules: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT schedule_id, schedule_version, connection_code, from_node, to_node,
		        departs_at, arrives_at, service_scope_ref, origin,
		        COALESCE(template_code, ''), COALESCE(template_version, 0),
		        COALESCE(to_char(running_date, 'YYYY-MM-DD'), ''), recorded_at
		   FROM (SELECT DISTINCT ON (schedule_id) *
		           FROM transport_fulfillment.transport_schedule
		          WHERE tenant_id = $1
		          ORDER BY schedule_id, schedule_version DESC) AS current_schedule
		  ORDER BY departs_at DESC, schedule_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}
	defer rows.Close()

	list := make([]ports.TransportScheduleCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.TransportScheduleCatalogueRow
		if err := rows.Scan(
			&row.ScheduleID, &row.Version, &row.Connection, &row.FromNode, &row.ToNode,
			&row.DepartsAt, &row.ArrivesAt, &row.ServiceScope, &row.Origin,
			&row.Template, &row.TemplateVersion, &row.RunningDate, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list transport schedules: %w", err)
		}
		row.DepartsAt = row.DepartsAt.UTC()
		row.ArrivesAt = row.ArrivesAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}
	return list, nil
}

// ListScheduleTemplates 上列周期安排登记册（ADR-0201）：每份只列当前版，按代码稳定排序——周期安排没有业务时刻可排。
func (catalogue *ReviewCatalogue) ListScheduleTemplates(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ScheduleTemplateCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list schedule templates: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedule templates: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT template_code, template_version, connection_code, from_node, to_node,
		        weekdays, departure_local_minute, duration_minutes, time_zone, service_scope_ref,
		        to_char(effective_from, 'YYYY-MM-DD'), COALESCE(to_char(effective_until, 'YYYY-MM-DD'), ''),
		        recorded_at,
		        (SELECT COALESCE(jsonb_agg(jsonb_build_object(
		                    'dimension', capacity.dimension, 'unit', capacity.unit_ref, 'capacity', capacity.capacity)
		                    ORDER BY capacity.dimension), '[]'::jsonb)
		           FROM transport_fulfillment.schedule_template_capacity AS capacity
		          WHERE capacity.tenant_id = current_template.tenant_id
		            AND capacity.template_code = current_template.template_code
		            AND capacity.template_version = current_template.template_version)
		   FROM (SELECT DISTINCT ON (template_code) *
		           FROM transport_fulfillment.schedule_template
		          WHERE tenant_id = $1
		          ORDER BY template_code, template_version DESC) AS current_template
		  ORDER BY template_code
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list schedule templates: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ScheduleTemplateCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.ScheduleTemplateCatalogueRow
		var capacities []storedCapacityDeclaration
		if err := rows.Scan(
			&row.Template, &row.Version, &row.Connection, &row.FromNode, &row.ToNode,
			&row.Weekdays, &row.DepartureLocalMinute, &row.DurationMinutes, &row.TimeZone, &row.ServiceScope,
			&row.EffectiveFrom, &row.EffectiveUntil, &row.RecordedAt, &capacities,
		); err != nil {
			return nil, fmt.Errorf("list schedule templates: %w", err)
		}
		row.RecordedAt = row.RecordedAt.UTC()
		row.Capacities = catalogueCapacities(capacities)
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schedule templates: %w", err)
	}
	return list, nil
}

// catalogueCapacities 照实转写一版的容量声明，次序按封闭词（领域的比较）定，与登记答复同一次序。SQL 里按维度词排
// 是字母序，WEIGHT 会落到 PIECES 之后，那一层只为让结果稳定。维度词只为排序而认：认不出的词照实列出、排在最前，
// 列面不当重建门（见本文件头注）。
func catalogueCapacities(stored []storedCapacityDeclaration) []ports.ScheduleTemplateCapacityRow {
	rows := make([]ports.ScheduleTemplateCapacityRow, 0, len(stored))
	for _, capacity := range stored {
		rows = append(rows, ports.ScheduleTemplateCapacityRow{
			Dimension: capacity.Dimension,
			Unit:      capacity.Unit,
			Capacity:  capacity.Capacity,
		})
	}
	slices.SortStableFunc(rows, func(left, right ports.ScheduleTemplateCapacityRow) int {
		leftDimension, _ := domain.ParseCapacityDimension(left.Dimension)
		rightDimension, _ := domain.ParseCapacityDimension(right.Dimension)
		return domain.CompareCapacityDimensions(leftDimension, rightDimension)
	})
	return rows
}

// ListCapacityPools 上列容量池登记册：三个和按预占子表求和（bigint 求和在 PG 里是
// numeric，落回 bigint 由 SQL 显式转换），无预占的池三量为零。行上没有业务时刻可
// 排序（登记时刻是提交簿记），按池标识稳定排序。
func (catalogue *ReviewCatalogue) ListCapacityPools(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CapacityPoolCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list capacity pools: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT p.pool_id, p.schedule_id, p.dimension, p.unit_ref, p.capacity,
		        COALESCE(SUM(r.quantity), 0)::bigint,
		        COALESCE(SUM(r.released), 0)::bigint,
		        COALESCE(SUM(r.consumed), 0)::bigint,
		        p.recorded_at
		   FROM transport_fulfillment.capacity_pool p
		   LEFT JOIN transport_fulfillment.capacity_reservation r
		     ON r.tenant_id = p.tenant_id
		    AND r.pool_id = p.pool_id
		  WHERE p.tenant_id = $1
		  GROUP BY p.pool_id, p.schedule_id, p.dimension, p.unit_ref, p.capacity, p.recorded_at
		  ORDER BY p.pool_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}
	defer rows.Close()

	list := make([]ports.CapacityPoolCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.CapacityPoolCatalogueRow
		if err := rows.Scan(
			&row.PoolID, &row.Schedule, &row.Dimension, &row.Unit, &row.Capacity,
			&row.Reserved, &row.Released, &row.Consumed, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list capacity pools: %w", err)
		}
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}
	return list, nil
}

// ListTransportHandovers 上列权威交接判断登记册：一行一版本、版本链完整可见，新近
// 裁决在前，同刻按（对象，范围，版本）稳定排序。
func (catalogue *ReviewCatalogue) ListTransportHandovers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TransportHandoverCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list transport handovers: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_ref, scope_ref, handover_version,
		        released_by, received_by, verdict,
		        COALESCE(basis_ref, ''),
		        COALESCE(corrects_version, ''),
		        corrected_at, judged_at, recorded_at,
		        COALESCE(releasing_node, ''),
		        COALESCE(receiving_node, ''),
		        COALESCE(onward_node, ''),
		        COALESCE(gate_reading, '')
		   FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1
		  ORDER BY judged_at DESC, object_ref, scope_ref, handover_version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}
	defer rows.Close()

	list := make([]ports.TransportHandoverCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row         ports.TransportHandoverCatalogueRow
			correctedAt *time.Time
		)
		if err := rows.Scan(
			&row.Object, &row.Scope, &row.Version,
			&row.ReleasedBy, &row.ReceivedBy, &row.Verdict,
			&row.Basis, &row.CorrectsVersion,
			&correctedAt, &row.JudgedAt, &row.RecordedAt,
			&row.ReleasingNode, &row.ReceivingNode, &row.OnwardNode,
			&row.GateReading,
		); err != nil {
			return nil, fmt.Errorf("list transport handovers: %w", err)
		}
		if correctedAt != nil {
			utc := correctedAt.UTC()
			row.CorrectedAt = &utc
		}
		row.JudgedAt = row.JudgedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}
	return list, nil
}

// ListEffectiveDeliveries 上列有效交付结果册：只列当前版（is_current），新近交付在
// 前，同刻按（对象，尝试）稳定排序。历史版本按键与版本走详情读口，不在列面展开。
func (catalogue *ReviewCatalogue) ListEffectiveDeliveries(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.EffectiveDeliveryCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list effective deliveries: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_ref, attempt_ref, delivery_version,
		        place_ref, method_ref, recipient_ref, proof_ref,
		        COALESCE(corrects_version, ''),
		        corrected_at, occurred_at, recorded_at, gate_reading
		   FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = $1
		    AND is_current
		  ORDER BY occurred_at DESC, object_ref, attempt_ref
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}
	defer rows.Close()

	list := make([]ports.EffectiveDeliveryCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row         ports.EffectiveDeliveryCatalogueRow
			correctedAt *time.Time
		)
		if err := rows.Scan(
			&row.Object, &row.Attempt, &row.Version,
			&row.Place, &row.Method, &row.Recipient, &row.Proof,
			&row.CorrectsVersion,
			&correctedAt, &row.OccurredAt, &row.RecordedAt, &row.GateReading,
		); err != nil {
			return nil, fmt.Errorf("list effective deliveries: %w", err)
		}
		if correctedAt != nil {
			utc := correctedAt.UTC()
			row.CorrectedAt = &utc
		}
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}
	return list, nil
}

// ListCarrierMasterDocuments 上列总单登记册（ADR-0113 决定六）：一行一版本、版本链完整
// 可见，新近登记在前，同刻按（总单，版本）稳定排序。关联数按关联子表逐版计数——计数是
// 派生不是判断，与容量四量的求和同一条纪律；关联本体不在列面展开。
func (catalogue *ReviewCatalogue) ListCarrierMasterDocuments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CarrierMasterDocumentCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list carrier master documents: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list carrier master documents: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT d.master_document_ref, d.version, d.issuer_ref, d.scope_ref,
		        COALESCE(d.commission_ref, ''), COALESCE(d.booking_ref, ''),
		        d.standing,
		        COALESCE(d.supersedes_version, ''), COALESCE(d.replaced_by_document, ''),
		        COUNT(a.associated_ref)::bigint,
		        d.changed_at, d.recorded_at
		   FROM transport_fulfillment.carrier_master_document d
		   LEFT JOIN transport_fulfillment.carrier_master_document_association a
		     ON a.tenant_id = d.tenant_id
		    AND a.master_document_ref = d.master_document_ref
		    AND a.version = d.version
		  WHERE d.tenant_id = $1
		  GROUP BY d.tenant_id, d.master_document_ref, d.version, d.issuer_ref, d.scope_ref,
		           d.commission_ref, d.booking_ref, d.standing, d.supersedes_version,
		           d.replaced_by_document, d.changed_at, d.recorded_at
		  ORDER BY d.recorded_at DESC, d.master_document_ref, d.version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list carrier master documents: %w", err)
	}
	defer rows.Close()

	list := make([]ports.CarrierMasterDocumentCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row       ports.CarrierMasterDocumentCatalogueRow
			changedAt *time.Time
		)
		if err := rows.Scan(
			&row.Document, &row.Version, &row.Issuer, &row.Scope,
			&row.Commission, &row.Booking,
			&row.Standing,
			&row.Supersedes, &row.ReplacedBy,
			&row.AssociationCount,
			&changedAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list carrier master documents: %w", err)
		}
		if changedAt != nil {
			utc := changedAt.UTC()
			row.ChangedAt = &utc
		}
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list carrier master documents: %w", err)
	}
	return list, nil
}

// ListTransportPreparations 上列运输准备结果册（ADR-0201 决定七）：新判的在前，同刻按（段，对象，规则）稳定排序。需求量
// 与候选两列快照照写下的样子转写，不经领域重建门——那是写路与按键读回的纪律。
func (catalogue *ReviewCatalogue) ListTransportPreparations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TransportPreparationCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list transport preparations: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transport preparations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT planned_leg_ref, object_ref, rule_version, outcome, connection_code, from_node, to_node,
		        window_start, window_end, demand_source, COALESCE(demand_anchor, ''), demand,
		        COALESCE(selected_schedule, ''), candidates, judged_at, recorded_at
		   FROM transport_fulfillment.transport_preparation
		  WHERE tenant_id = $1
		  ORDER BY judged_at DESC, planned_leg_ref, object_ref, rule_version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list transport preparations: %w", err)
	}
	defer rows.Close()

	list := make([]ports.TransportPreparationCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.TransportPreparationCatalogueRow
		var demand, candidates []byte
		if err := rows.Scan(
			&row.PlannedLeg, &row.Object, &row.Rule, &row.Outcome, &row.Connection, &row.FromNode, &row.ToNode,
			&row.WindowStart, &row.WindowEnd, &row.DemandSource, &row.DemandAnchor, &demand,
			&row.Selected, &candidates, &row.JudgedAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list transport preparations: %w", err)
		}
		if row.Demand, err = preparationDemandRows(demand); err != nil {
			return nil, fmt.Errorf("list transport preparations: %w", err)
		}
		if row.Candidates, err = preparationCandidateRows(candidates); err != nil {
			return nil, fmt.Errorf("list transport preparations: %w", err)
		}
		row.WindowStart = row.WindowStart.UTC()
		row.WindowEnd = row.WindowEnd.UTC()
		row.JudgedAt = row.JudgedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list transport preparations: %w", err)
	}
	return list, nil
}

// ListDeliveryDispatchPendings 上列派送待形成册里待形成的那几件（ADR-0230 决定三）。不另写一条查询：清单就是按册重问的那个队列，
// 筛法与排序只在 DeliveryDispatchPendings.ListPending 一处。
func (catalogue *ReviewCatalogue) ListDeliveryDispatchPendings(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DeliveryDispatchPendingRecord, error) {
	return (&DeliveryDispatchPendings{db: catalogue.db}).ListPending(ctx, tenant, time.Time{}, limit)
}

// ListBookingRequests 上列订舱册（票 demo-chain-gaps/48）：新申请的在前，同刻按订舱标识稳定排序。先按页截出订舱，再把每一次
// 订舱名下的全部承运答复聚合进同一行，一条语句读出，订舱与答复取自同一个快照。答复按答复时刻、落库先后排，与答复编排判先后
// （BookingAnswers.FindAnswersForUpdate）同一个次序；排序键末尾的答复标识只为让几列聚合逐位对齐。
func (catalogue *ReviewCatalogue) ListBookingRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.BookingRequestCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list booking requests: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list booking requests: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT booking.booking_id, booking.commission_id, booking.schedule_id, booking.quantity, booking.unit_ref,
		        booking.requested_at, booking.cancelled_at, booking.recorded_at, booking.submitted_by, booking.cancelled_by,
		        answers.ids, answers.outcomes, answers.quantities, answers.bases, answers.decided, answers.recorded
		   FROM (SELECT *
		           FROM transport_fulfillment.booking_request
		          WHERE tenant_id = $1
		          ORDER BY requested_at DESC, booking_id
		          LIMIT $2) AS booking
		   LEFT JOIN LATERAL (
		        SELECT array_agg(answer.acceptance_id ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS ids,
		               array_agg(answer.outcome ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS outcomes,
		               array_agg(answer.quantity ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS quantities,
		               array_agg(COALESCE(answer.basis, '') ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS bases,
		               array_agg(answer.decided_at ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS decided,
		               array_agg(answer.recorded_at ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS recorded
		          FROM transport_fulfillment.booking_answer AS answer
		         WHERE answer.tenant_id = booking.tenant_id
		           AND answer.booking_id = booking.booking_id) AS answers ON true
		  ORDER BY booking.requested_at DESC, booking.booking_id`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list booking requests: %w", err)
	}
	defer rows.Close()

	list := make([]ports.BookingRequestCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row                      ports.BookingRequestCatalogueRow
			cancelledAt              *time.Time
			submittedBy, cancelledBy *string
			ids, outcomes, bases     []string
			quantities               []int64
			decidedTimes, recordedAt []time.Time
		)
		if err := rows.Scan(
			&row.Booking, &row.Commission, &row.Schedule, &row.Quantity, &row.Unit,
			&row.RequestedAt, &cancelledAt, &row.RecordedAt, &submittedBy, &cancelledBy,
			&ids, &outcomes, &quantities, &bases, &decidedTimes, &recordedAt,
		); err != nil {
			return nil, fmt.Errorf("list booking requests: %w", err)
		}
		if cancelledAt != nil {
			utc := cancelledAt.UTC()
			row.CancelledAt = &utc
		}
		row.RequestedAt = row.RequestedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		submitter, err := registrantFromColumn(submittedBy)
		if err != nil {
			return nil, fmt.Errorf("list booking requests: booking %q: %w", row.Booking, err)
		}
		canceller, err := registrantFromColumn(cancelledBy)
		if err != nil {
			return nil, fmt.Errorf("list booking requests: booking %q: %w", row.Booking, err)
		}
		row.SubmittedBy, row.CancelledBy = submitter.String(), canceller.String()
		row.Answers = make([]ports.BookingAnswerCatalogueRow, 0, len(ids))
		for index := range ids {
			row.Answers = append(row.Answers, ports.BookingAnswerCatalogueRow{
				Answer:     ids[index],
				Outcome:    outcomes[index],
				Quantity:   quantities[index],
				Basis:      bases[index],
				DecidedAt:  decidedTimes[index].UTC(),
				RecordedAt: recordedAt[index].UTC(),
			})
		}
		standing, current, err := bookingStanding(tenant, row.Booking, row.Quantity, row.Answers)
		if err != nil {
			return nil, fmt.Errorf("list booking requests: booking %q: %w", row.Booking, err)
		}
		row.Standing = standing
		if current >= 0 {
			row.Current = &row.Answers[current]
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list booking requests: %w", err)
	}
	return list, nil
}

// bookingStanding 是订舱状态词的唯一派生处（读法见 ports 那组状态词的注释）：交回状态词与当前那一份在 answers 里的位置，没有
// 答复时位置为 -1。名下答复经重建门读回，当前那一份由 domain.CurrentCarrierAcceptance 选——与收费发生项读订舱当前答复的
// BookingAnswers.FindByKey 同一处定义，这里不另选。
//
// 读不出一个状态词就答错误，不挑一份凑一个词：答复凑不成一段答复史（错误包着 domain.ErrBookingAnswersInconsistent）、答复行
// 过不了重建门、接受量超过申请量，都只可能是绕过答复编排落的行。
func bookingStanding(
	tenant domain.TenantID,
	booking string,
	requested int64,
	answers []ports.BookingAnswerCatalogueRow,
) (string, int, error) {
	bookingRef, err := domain.NewBookingReference(booking)
	if err != nil {
		return "", -1, err
	}
	key := ports.BookingKey{TenantID: tenant, Booking: bookingRef}
	acceptances := make([]domain.CarrierAcceptance, 0, len(answers))
	for _, answer := range answers {
		var basis *string
		if answer.Basis != "" {
			present := answer.Basis
			basis = &present
		}
		acceptance, err := rehydrateCarrierAcceptance(key, answer.Answer, answer.Outcome, answer.Quantity, basis, answer.DecidedAt)
		if err != nil {
			return "", -1, fmt.Errorf("answer %q: %w", answer.Answer, err)
		}
		acceptances = append(acceptances, acceptance)
	}
	current, err := domain.CurrentCarrierAcceptance(acceptances)
	if err != nil {
		return "", -1, err
	}
	if current < 0 {
		return ports.BookingStandingPendingConfirmation, -1, nil
	}
	answer := acceptances[current]
	if !answer.Settles() {
		return ports.BookingStandingWaitlisted, current, nil
	}
	switch answer.Outcome() {
	case domain.BookingAccepted:
		switch accepted := answer.AcceptedQuantity(); {
		case accepted == requested:
			return ports.BookingStandingConfirmed, current, nil
		case accepted < requested:
			return ports.BookingStandingPartiallyConfirmed, current, nil
		default:
			return "", -1, fmt.Errorf("answer %q accepts %d of %d requested", answer.Acceptance(), accepted, requested)
		}
	case domain.BookingRefused:
		return ports.BookingStandingRefused, current, nil
	case domain.BookingExpired:
		return ports.BookingStandingExpired, current, nil
	case domain.BookingWithdrawn:
		return ports.BookingStandingWithdrawn, current, nil
	default:
		return "", -1, fmt.Errorf("answer %q settles with outcome %d that has no standing", answer.Acceptance(), answer.Outcome())
	}
}

// ListBookingAnswerBounces 上列承运答复挡回册（ADR-0239 决定二，票 demo-chain-gaps/167）：最近一次挡回的在前，同刻按（答复方、
// 订舱、答复标识、依据）稳定排序；筛选的两维各自空串即不筛。依据码经 bounceBasisWord 译回词。
func (catalogue *ReviewCatalogue) ListBookingAnswerBounces(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.BookingAnswerBounceFilter,
	limit int,
) ([]ports.BookingAnswerBounceCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list booking answer bounces: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list booking answer bounces: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT source_ref, booking_id, acceptance_id, basis, first_bounced_at, last_bounced_at, bounce_count
		   FROM transport_fulfillment.booking_answer_bounce
		  WHERE tenant_id = $1
		    AND ($2 = '' OR booking_id = $2)
		    AND ($3 = '' OR source_ref = $3)
		  ORDER BY last_bounced_at DESC, source_ref, booking_id, acceptance_id, basis
		  LIMIT $4`,
		tenant.String(),
		filter.Booking,
		filter.Source,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list booking answer bounces: %w", err)
	}
	defer rows.Close()

	list := make([]ports.BookingAnswerBounceCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.BookingAnswerBounceCatalogueRow
		var code int16
		if err := rows.Scan(
			&row.Source, &row.Booking, &row.Answer, &code, &row.FirstBouncedAt, &row.LastBouncedAt, &row.Count,
		); err != nil {
			return nil, fmt.Errorf("list booking answer bounces: %w", err)
		}
		word, known := bounceBasisWord(code)
		if !known {
			return nil, fmt.Errorf("list booking answer bounces: stored basis code %d is not a basis", code)
		}
		row.Basis = word
		row.FirstBouncedAt = row.FirstBouncedAt.UTC()
		row.LastBouncedAt = row.LastBouncedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list booking answer bounces: %w", err)
	}
	return list, nil
}

// bounceBasisWord 是 bounceBasisCode 的逆：逐个依据问它的码，码表只在 bounceBasisCode 一处。依据从 1 起连号，String 答空串
// 即过了最后一个。
func bounceBasisWord(code int16) (string, bool) {
	for basis := ports.BookingAnswerBounceBasisInvalid + 1; basis.String() != ""; basis++ {
		if stored, known := bounceBasisCode(basis); known && stored == code {
			return basis.String(), true
		}
	}
	return "", false
}

func preparationDemandRows(raw []byte) ([]ports.PreparationDemandRow, error) {
	var stored []storedDemandQuantity
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	rows := make([]ports.PreparationDemandRow, 0, len(stored))
	for _, quantity := range stored {
		rows = append(rows, ports.PreparationDemandRow{
			Dimension: quantity.Dimension,
			Amount:    quantity.Amount,
			Unit:      quantity.Unit,
		})
	}
	return rows, nil
}

func preparationCandidateRows(raw []byte) ([]ports.PreparationCandidateRow, error) {
	var stored []storedCandidate
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	rows := make([]ports.PreparationCandidateRow, 0, len(stored))
	for _, candidate := range stored {
		row := ports.PreparationCandidateRow{
			Schedule:  candidate.Schedule,
			Version:   candidate.Version,
			DepartsAt: candidate.DepartsAt.UTC(),
			ArrivesAt: candidate.ArrivesAt.UTC(),
			Verdict:   candidate.Verdict,
			Pools:     make([]ports.PreparationPoolRow, 0, len(candidate.Pools)),
		}
		for _, pool := range candidate.Pools {
			row.Pools = append(row.Pools, ports.PreparationPoolRow{
				Pool:      pool.Pool,
				Dimension: pool.Dimension,
				Unit:      pool.Unit,
				Available: pool.Available,
				Requested: pool.Requested,
				Verdict:   pool.Verdict,
			})
		}
		rows = append(rows, row)
	}
	return rows, nil
}
