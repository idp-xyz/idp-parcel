package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
//
// 各册翻页、排序与筛选照 ADR-0144（票 catalogue-read-pagination-rollout/05）：每册一份落列，总数与取页出自同一组条件
// （readCataloguePage）；要另读一次的格（单据号）只对切出的本页现读。
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

// transportSchedulePages 把 ports.TransportSchedulePages 落到列上。q 在班次标识、连接与起讫节点、周期安排代码上做字面包含。
var transportSchedulePages = ports.TransportSchedulePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"departsAt": "departs_at", "scheduleId": "schedule_id"},
	Identity: []string{"schedule_id", "schedule_version"},
	Filters:  map[string]string{"origin": "origin"},
	Keyword:  []string{"schedule_id", "connection_code", "from_node", "to_node", "template_code"},
})

// currentScheduleCondition 只留每个班次的当前版：没有更高版本的那一版。写成反连接而不是先 DISTINCT ON 再排：键集比较与
// 排序才落得到（租户，出发时刻，标识）那条索引上，翻到深页也不必先把整个租户的班次去重一遍。
const currentScheduleCondition = `tenant_id = $1
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.transport_schedule AS later
		         WHERE later.tenant_id = current_schedule.tenant_id
		           AND later.schedule_id = current_schedule.schedule_id
		           AND later.schedule_version > current_schedule.schedule_version)`

// ListTransportSchedules 上列班次登记册：每个班次只列当前版，按查询的筛选、q 与排序取游标之后的一页；缺省晚出发的在前，
// 同刻按（班次，版本）倒序。
func (catalogue *ReviewCatalogue) ListTransportSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.TransportScheduleCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.TransportScheduleCatalogueRow], error) {
		return cataloguepage.Result[ports.TransportScheduleCatalogueRow]{}, fmt.Errorf("list transport schedules: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := transportSchedulePages.Plan(query, currentScheduleCondition, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.transport_schedule AS current_schedule"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`schedule_id, schedule_version, connection_code, from_node, to_node,
		 departs_at, arrives_at, service_scope_ref, origin,
		 COALESCE(template_code, ''), COALESCE(template_version, 0),
		 COALESCE(to_char(running_date, 'YYYY-MM-DD'), ''), recorded_at`, source),
		scanTransportScheduleCatalogueRow, transportSchedulePosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanTransportScheduleCatalogueRow(scanned pgx.Row) (ports.TransportScheduleCatalogueRow, error) {
	var row ports.TransportScheduleCatalogueRow
	if err := scanned.Scan(
		&row.ScheduleID, &row.Version, &row.Connection, &row.FromNode, &row.ToNode,
		&row.DepartsAt, &row.ArrivesAt, &row.ServiceScope, &row.Origin,
		&row.Template, &row.TemplateVersion, &row.RunningDate, &row.RecordedAt,
	); err != nil {
		return ports.TransportScheduleCatalogueRow{}, err
	}
	row.DepartsAt = row.DepartsAt.UTC()
	row.ArrivesAt = row.ArrivesAt.UTC()
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func transportSchedulePosition(row ports.TransportScheduleCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.DepartsAt)
	if sortField == "scheduleId" {
		value = row.ScheduleID
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.ScheduleID, cataloguepage.FormatInteger(int64(row.Version))}}
}

// scheduleTemplatePages 把 ports.ScheduleTemplatePages 落到列上。q 在周期安排代码、连接与起讫节点上做字面包含。
var scheduleTemplatePages = ports.ScheduleTemplatePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"template": "template_code", "recordedAt": "recorded_at"},
	Identity: []string{"template_code", "template_version"},
	Keyword:  []string{"template_code", "connection_code", "from_node", "to_node"},
})

// currentTemplateCondition 只留每份周期安排的当前版，写成反连接的理由同 currentScheduleCondition。
const currentTemplateCondition = `tenant_id = $1
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.schedule_template AS later
		         WHERE later.tenant_id = current_template.tenant_id
		           AND later.template_code = current_template.template_code
		           AND later.template_version > current_template.template_version)`

// ListScheduleTemplates 上列周期安排登记册（ADR-0201）：每份只列当前版；缺省按代码升序——周期安排没有业务时刻可排。
func (catalogue *ReviewCatalogue) ListScheduleTemplates(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ScheduleTemplateCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ScheduleTemplateCatalogueRow], error) {
		return cataloguepage.Result[ports.ScheduleTemplateCatalogueRow]{}, fmt.Errorf("list schedule templates: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := scheduleTemplatePages.Plan(query, currentTemplateCondition, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.schedule_template AS current_template"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`template_code, template_version, connection_code, from_node, to_node,
		 weekdays, departure_local_minute, duration_minutes, time_zone, service_scope_ref,
		 to_char(effective_from, 'YYYY-MM-DD'), COALESCE(to_char(effective_until, 'YYYY-MM-DD'), ''),
		 recorded_at,
		 (SELECT COALESCE(jsonb_agg(jsonb_build_object(
		             'dimension', capacity.dimension, 'unit', capacity.unit_ref, 'capacity', capacity.capacity)
		             ORDER BY capacity.dimension), '[]'::jsonb)
		    FROM transport_fulfillment.schedule_template_capacity AS capacity
		   WHERE capacity.tenant_id = current_template.tenant_id
		     AND capacity.template_code = current_template.template_code
		     AND capacity.template_version = current_template.template_version)`, source),
		scanScheduleTemplateRow, scheduleTemplatePosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanScheduleTemplateRow(scanned pgx.Row) (ports.ScheduleTemplateCatalogueRow, error) {
	var row ports.ScheduleTemplateCatalogueRow
	var capacities []storedCapacityDeclaration
	if err := scanned.Scan(
		&row.Template, &row.Version, &row.Connection, &row.FromNode, &row.ToNode,
		&row.Weekdays, &row.DepartureLocalMinute, &row.DurationMinutes, &row.TimeZone, &row.ServiceScope,
		&row.EffectiveFrom, &row.EffectiveUntil, &row.RecordedAt, &capacities,
	); err != nil {
		return ports.ScheduleTemplateCatalogueRow{}, err
	}
	row.RecordedAt = row.RecordedAt.UTC()
	row.Capacities = catalogueCapacities(capacities)
	return row, nil
}

func scheduleTemplatePosition(row ports.ScheduleTemplateCatalogueRow, sortField string) cataloguepage.Position {
	value := row.Template
	if sortField == "recordedAt" {
		value = cataloguepage.FormatInstant(row.RecordedAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.Template, cataloguepage.FormatInteger(int64(row.Version))}}
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

// capacityPoolPages 把 ports.CapacityPoolPages 落到列上。q 在池标识与班次上做字面包含。列都带表别名：取页那一句另接预占
// 子表的求和。
var capacityPoolPages = ports.CapacityPoolPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"poolId": "pool.pool_id"},
	Identity: []string{"pool.pool_id"},
	Filters:  map[string]string{"dimension": "pool.dimension"},
	Keyword:  []string{"pool.pool_id", "pool.schedule_id"},
})

// ListCapacityPools 上列容量池登记册：三个和按预占子表求和（bigint 求和在 PG 里是
// numeric，落回 bigint 由 SQL 显式转换），无预占的池三量为零。行上没有业务时刻可
// 排序（登记时刻是提交簿记），缺省按池标识升序。总数只数池，不连预占子表。
func (catalogue *ReviewCatalogue) ListCapacityPools(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CapacityPoolCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CapacityPoolCatalogueRow], error) {
		return cataloguepage.Result[ports.CapacityPoolCatalogueRow]{}, fmt.Errorf("list capacity pools: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := capacityPoolPages.Plan(query, "pool.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const pools = "transport_fulfillment.capacity_pool AS pool"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(pools), plan.PageSQL(
		`pool.pool_id, pool.schedule_id, pool.dimension, pool.unit_ref, pool.capacity,
		 reservations.reserved, reservations.released, reservations.consumed, pool.recorded_at`,
		pools+`
		 CROSS JOIN LATERAL (
		      SELECT COALESCE(SUM(reservation.quantity), 0)::bigint AS reserved,
		             COALESCE(SUM(reservation.released), 0)::bigint AS released,
		             COALESCE(SUM(reservation.consumed), 0)::bigint AS consumed
		        FROM transport_fulfillment.capacity_reservation AS reservation
		       WHERE reservation.tenant_id = pool.tenant_id
		         AND reservation.pool_id = pool.pool_id) AS reservations`),
		scanCapacityPoolRow, capacityPoolPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanCapacityPoolRow(scanned pgx.Row) (ports.CapacityPoolCatalogueRow, error) {
	var row ports.CapacityPoolCatalogueRow
	if err := scanned.Scan(
		&row.PoolID, &row.Schedule, &row.Dimension, &row.Unit, &row.Capacity,
		&row.Reserved, &row.Released, &row.Consumed, &row.RecordedAt,
	); err != nil {
		return ports.CapacityPoolCatalogueRow{}, err
	}
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func capacityPoolPosition(row ports.CapacityPoolCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: row.PoolID, Identity: []string{row.PoolID}}
}

// transportHandoverPages 把 ports.TransportHandoverPages 落到列上。q 在对象、范围与交出、接收两方上做字面包含。
var transportHandoverPages = ports.TransportHandoverPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"judgedAt": "judged_at", "object": "object_ref"},
	Identity: []string{"object_ref", "scope_ref", "handover_version"},
	Filters:  map[string]string{"verdict": "verdict"},
	Keyword:  []string{"object_ref", "scope_ref", "released_by", "received_by"},
})

// ListTransportHandovers 上列权威交接判断登记册：一行一版本、版本链完整可见；缺省新近裁决在前，同刻按（对象，范围，版本）
// 倒序。
func (catalogue *ReviewCatalogue) ListTransportHandovers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.TransportHandoverCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.TransportHandoverCatalogueRow], error) {
		return cataloguepage.Result[ports.TransportHandoverCatalogueRow]{}, fmt.Errorf("list transport handovers: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := transportHandoverPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.transport_handover"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`object_ref, scope_ref, handover_version,
		 released_by, received_by, verdict,
		 COALESCE(basis_ref, ''),
		 COALESCE(corrects_version, ''),
		 corrected_at, judged_at, recorded_at,
		 COALESCE(releasing_node, ''),
		 COALESCE(receiving_node, ''),
		 COALESCE(onward_node, ''),
		 COALESCE(gate_reading, '')`, source),
		scanTransportHandoverRow, transportHandoverPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanTransportHandoverRow(scanned pgx.Row) (ports.TransportHandoverCatalogueRow, error) {
	var (
		row         ports.TransportHandoverCatalogueRow
		correctedAt *time.Time
	)
	if err := scanned.Scan(
		&row.Object, &row.Scope, &row.Version,
		&row.ReleasedBy, &row.ReceivedBy, &row.Verdict,
		&row.Basis, &row.CorrectsVersion,
		&correctedAt, &row.JudgedAt, &row.RecordedAt,
		&row.ReleasingNode, &row.ReceivingNode, &row.OnwardNode,
		&row.GateReading,
	); err != nil {
		return ports.TransportHandoverCatalogueRow{}, err
	}
	if correctedAt != nil {
		utc := correctedAt.UTC()
		row.CorrectedAt = &utc
	}
	row.JudgedAt = row.JudgedAt.UTC()
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func transportHandoverPosition(row ports.TransportHandoverCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.JudgedAt)
	if sortField == "object" {
		value = row.Object
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.Object, row.Scope, row.Version}}
}

// effectiveDeliveryPages 把 ports.EffectiveDeliveryPages 落到列上。q 在对象、尝试、地点、收件方与交付证明上做字面包含。
var effectiveDeliveryPages = ports.EffectiveDeliveryPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"occurredAt": "occurred_at", "object": "object_ref"},
	Identity: []string{"object_ref", "attempt_ref", "delivery_version"},
	Filters:  map[string]string{"gateReading": "gate_reading"},
	Keyword:  []string{"object_ref", "attempt_ref", "place_ref", "recipient_ref", "proof_ref"},
})

// ListEffectiveDeliveries 上列有效交付结果册：只列当前版（is_current）；缺省新近交付在前，同刻按（对象，尝试，版本）倒序。
// 历史版本按键与版本走详情读口，不在列面展开。
func (catalogue *ReviewCatalogue) ListEffectiveDeliveries(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.EffectiveDeliveryCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.EffectiveDeliveryCatalogueRow], error) {
		return cataloguepage.Result[ports.EffectiveDeliveryCatalogueRow]{}, fmt.Errorf("list effective deliveries: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := effectiveDeliveryPages.Plan(query, "tenant_id = $1 AND is_current", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.effective_delivery"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`object_ref, attempt_ref, delivery_version,
		 place_ref, method_ref, recipient_ref, proof_ref,
		 COALESCE(corrects_version, ''),
		 corrected_at, occurred_at, recorded_at, gate_reading`, source),
		scanEffectiveDeliveryRow, effectiveDeliveryPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanEffectiveDeliveryRow(scanned pgx.Row) (ports.EffectiveDeliveryCatalogueRow, error) {
	var (
		row         ports.EffectiveDeliveryCatalogueRow
		correctedAt *time.Time
	)
	if err := scanned.Scan(
		&row.Object, &row.Attempt, &row.Version,
		&row.Place, &row.Method, &row.Recipient, &row.Proof,
		&row.CorrectsVersion,
		&correctedAt, &row.OccurredAt, &row.RecordedAt, &row.GateReading,
	); err != nil {
		return ports.EffectiveDeliveryCatalogueRow{}, err
	}
	if correctedAt != nil {
		utc := correctedAt.UTC()
		row.CorrectedAt = &utc
	}
	row.OccurredAt = row.OccurredAt.UTC()
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func effectiveDeliveryPosition(row ports.EffectiveDeliveryCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.OccurredAt)
	if sortField == "object" {
		value = row.Object
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.Object, row.Attempt, row.Version}}
}

// carrierMasterDocumentPages 把 ports.CarrierMasterDocumentPages 落到列上。q 在总单、签发方、范围与所引委托、订舱上做字面
// 包含。列都带表别名：取页那一句另接关联子表的计数。
var carrierMasterDocumentPages = ports.CarrierMasterDocumentPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"recordedAt": "master_document.recorded_at",
		"document":   "master_document.master_document_ref",
	},
	Identity: []string{"master_document.master_document_ref", "master_document.version"},
	Filters:  map[string]string{"standing": "master_document.standing"},
	Keyword: []string{
		"master_document.master_document_ref", "master_document.issuer_ref", "master_document.scope_ref",
		"master_document.commission_ref", "master_document.booking_ref",
	},
})

// ListCarrierMasterDocuments 上列总单登记册（ADR-0113 决定六）：一行一版本、版本链完整可见；缺省新近登记在前，同刻按
// （总单，版本）倒序。关联数按关联子表逐版计数——计数是派生不是判断，与容量四量的求和同一条纪律；关联本体不在列面展开。
func (catalogue *ReviewCatalogue) ListCarrierMasterDocuments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CarrierMasterDocumentCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CarrierMasterDocumentCatalogueRow], error) {
		return cataloguepage.Result[ports.CarrierMasterDocumentCatalogueRow]{}, fmt.Errorf("list carrier master documents: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := carrierMasterDocumentPages.Plan(query, "master_document.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const documents = "transport_fulfillment.carrier_master_document AS master_document"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(documents), plan.PageSQL(
		`master_document.master_document_ref, master_document.version, master_document.issuer_ref, master_document.scope_ref,
		 COALESCE(master_document.commission_ref, ''), COALESCE(master_document.booking_ref, ''),
		 master_document.standing,
		 COALESCE(master_document.supersedes_version, ''), COALESCE(master_document.replaced_by_document, ''),
		 associations.counted,
		 master_document.changed_at, master_document.recorded_at`,
		documents+`
		 CROSS JOIN LATERAL (
		      SELECT count(*)::bigint AS counted
		        FROM transport_fulfillment.carrier_master_document_association AS association
		       WHERE association.tenant_id = master_document.tenant_id
		         AND association.master_document_ref = master_document.master_document_ref
		         AND association.version = master_document.version) AS associations`),
		scanCarrierMasterDocumentRow, carrierMasterDocumentPosition)
	if err != nil {
		return fail(err)
	}
	// 所引委托、订舱的单据号格另读一次（ADR-0355 决定十一），只读本页这几行引到的：号格的读法只在
	// referencedDocumentNumberings 一处，不在这里再连一次表。
	var commissions, bookings []string
	for _, row := range page.Rows {
		commissions = append(commissions, row.Commission)
		bookings = append(bookings, row.Booking)
	}
	commissionNumbers, err := commissionDocumentNumberings(ctx, querier, tenant, commissions)
	if err != nil {
		return fail(err)
	}
	bookingNumbers, err := bookingDocumentNumberings(ctx, querier, tenant, bookings)
	if err != nil {
		return fail(err)
	}
	for index := range page.Rows {
		page.Rows[index].CommissionDocumentNumber = referencedNumbering(commissionNumbers, page.Rows[index].Commission)
		page.Rows[index].BookingDocumentNumber = referencedNumbering(bookingNumbers, page.Rows[index].Booking)
	}
	return page, nil
}

func scanCarrierMasterDocumentRow(scanned pgx.Row) (ports.CarrierMasterDocumentCatalogueRow, error) {
	var (
		row       ports.CarrierMasterDocumentCatalogueRow
		changedAt *time.Time
	)
	if err := scanned.Scan(
		&row.Document, &row.Version, &row.Issuer, &row.Scope,
		&row.Commission, &row.Booking,
		&row.Standing,
		&row.Supersedes, &row.ReplacedBy,
		&row.AssociationCount,
		&changedAt, &row.RecordedAt,
	); err != nil {
		return ports.CarrierMasterDocumentCatalogueRow{}, err
	}
	if changedAt != nil {
		utc := changedAt.UTC()
		row.ChangedAt = &utc
	}
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func carrierMasterDocumentPosition(row ports.CarrierMasterDocumentCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.RecordedAt)
	if sortField == "document" {
		value = row.Document
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.Document, row.Version}}
}

// transportPreparationPages 把 ports.TransportPreparationPages 落到列上。q 在段、对象、连接与起讫节点、选中的班次上做字面包含。
var transportPreparationPages = ports.TransportPreparationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"judgedAt": "judged_at", "object": "object_ref"},
	Identity: []string{"planned_leg_ref", "object_ref", "rule_version"},
	Filters:  map[string]string{"outcome": "outcome"},
	Keyword:  []string{"planned_leg_ref", "object_ref", "connection_code", "from_node", "to_node", "selected_schedule"},
})

// ListTransportPreparations 上列运输准备结果册（ADR-0201 决定七）：缺省新判的在前，同刻按（段，对象，规则）倒序。需求量
// 与候选两列快照照写下的样子转写，不经领域重建门——那是写路与按键读回的纪律。
func (catalogue *ReviewCatalogue) ListTransportPreparations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.TransportPreparationCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.TransportPreparationCatalogueRow], error) {
		return cataloguepage.Result[ports.TransportPreparationCatalogueRow]{}, fmt.Errorf("list transport preparations: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := transportPreparationPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.transport_preparation"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`planned_leg_ref, object_ref, rule_version, outcome, connection_code, from_node, to_node,
		 window_start, window_end, demand_source, COALESCE(demand_anchor, ''), demand,
		 COALESCE(selected_schedule, ''), candidates, judged_at, recorded_at`, source),
		scanTransportPreparationRow, transportPreparationPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanTransportPreparationRow(scanned pgx.Row) (ports.TransportPreparationCatalogueRow, error) {
	var row ports.TransportPreparationCatalogueRow
	var demand, candidates []byte
	if err := scanned.Scan(
		&row.PlannedLeg, &row.Object, &row.Rule, &row.Outcome, &row.Connection, &row.FromNode, &row.ToNode,
		&row.WindowStart, &row.WindowEnd, &row.DemandSource, &row.DemandAnchor, &demand,
		&row.Selected, &candidates, &row.JudgedAt, &row.RecordedAt,
	); err != nil {
		return ports.TransportPreparationCatalogueRow{}, err
	}
	var err error
	if row.Demand, err = preparationDemandRows(demand); err != nil {
		return ports.TransportPreparationCatalogueRow{}, err
	}
	if row.Candidates, err = preparationCandidateRows(candidates); err != nil {
		return ports.TransportPreparationCatalogueRow{}, err
	}
	row.WindowStart = row.WindowStart.UTC()
	row.WindowEnd = row.WindowEnd.UTC()
	row.JudgedAt = row.JudgedAt.UTC()
	row.RecordedAt = row.RecordedAt.UTC()
	return row, nil
}

func transportPreparationPosition(row ports.TransportPreparationCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.JudgedAt)
	if sortField == "object" {
		value = row.Object
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.PlannedLeg, row.Object, row.Rule}}
}

// deliveryDispatchPendingPages 把 ports.DeliveryDispatchPendingPages 落到列上。q 在段、对象与入场依据上做字面包含。
var deliveryDispatchPendingPages = ports.DeliveryDispatchPendingPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"lastAskedAt": "last_asked_at", "firstAnsweredAt": "first_answered_at"},
	Identity: []string{"segment_ref", "object_ref"},
	Keyword:  []string{"segment_ref", "object_ref", "entry_basis"},
})

// ListDeliveryDispatchPendings 上列派送待形成册里待形成的那几件（ADR-0230 决定三）。筛法与按册重问的队列共用
// deliveryDispatchPendingOpen 一处；缺省序与队列同为最近一次问最早的在前、同刻按（段，对象）升序，翻页另走本册的 Plan——
// 队列口不收游标，也不该为查阅页长出一个。
func (catalogue *ReviewCatalogue) ListDeliveryDispatchPendings(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DeliveryDispatchPendingRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.DeliveryDispatchPendingRecord], error) {
		return cataloguepage.Result[ports.DeliveryDispatchPendingRecord]{}, fmt.Errorf("list delivery dispatch pendings: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := deliveryDispatchPendingPages.Plan(query, deliveryDispatchPendingOpen, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.delivery_dispatch_pending"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source),
		plan.PageSQL(deliveryDispatchPendingColumns, source), scanDeliveryDispatchPending, deliveryDispatchPendingPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// deliveryDispatchPendingPosition 取装回的记录：两个时刻只换了时区，段与对象的构造门原样存值，与列逐位相同。
func deliveryDispatchPendingPosition(record ports.DeliveryDispatchPendingRecord, sortField string) cataloguepage.Position {
	at := record.LastAskedAt
	if sortField == "firstAnsweredAt" {
		at = record.FirstAnsweredAt
	}
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(at),
		Identity: []string{record.Key.Segment.String(), record.Key.Object.String()},
	}
}

// bookingRequestPages 把 ports.BookingRequestPages 落到列上。q 在订舱、所属委托、班次与订舱的单据号上做字面包含。列都带
// 别名 booking：取页先在订舱表上截出一页，外层再按同一个 ORDER BY 排一次，两层的 booking 是同一组列。
var bookingRequestPages = ports.BookingRequestPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"requestedAt": "booking.requested_at", "booking": "booking.booking_id"},
	Identity: []string{"booking.booking_id"},
	Keyword:  []string{"booking.booking_id", "booking.commission_id", "booking.schedule_id", "booking.document_number"},
})

// ListBookingRequests 上列订舱册（票 demo-chain-gaps/48）：缺省新申请的在前，同刻按订舱标识倒序。先按页截出订舱，再把每一次
// 订舱名下的全部承运答复聚合进同一行，一条语句读出，订舱与答复取自同一个快照。答复按答复时刻、落库先后排，与答复编排判先后
// （BookingAnswers.FindAnswersForUpdate）同一个次序；排序键末尾的答复标识只为让几列聚合逐位对齐。
//
// 订舱与它所属委托的单据号格（号或无号原因）各随一行带出（ADR-0355 决定十一）：委托以左连接取，委托行读不回时那一格缺席，
// 不拿委托标识顶。
func (catalogue *ReviewCatalogue) ListBookingRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.BookingRequestCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.BookingRequestCatalogueRow], error) {
		return cataloguepage.Result[ports.BookingRequestCatalogueRow]{}, fmt.Errorf("list booking requests: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := bookingRequestPages.Plan(query, "booking.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const bookings = "transport_fulfillment.booking_request AS booking"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(bookings),
		`SELECT booking.booking_id, booking.commission_id, booking.schedule_id, booking.quantity, booking.unit_ref,
		        booking.requested_at, booking.cancelled_at, booking.recorded_at, booking.submitted_by, booking.cancelled_by,
		        booking.document_number, booking.document_number_absence,
		        commission.commission_id IS NOT NULL, commission.document_number, commission.document_number_absence,
		        answers.ids, answers.outcomes, answers.quantities, answers.bases, answers.decided, answers.recorded,
		        answers.manual
		   FROM (`+plan.PageSQL("booking.*", bookings)+`) AS booking
		   LEFT JOIN transport_fulfillment.transport_commission AS commission
		     ON commission.tenant_id = booking.tenant_id
		    AND commission.commission_id = booking.commission_id
		   LEFT JOIN LATERAL (
		        SELECT array_agg(answer.acceptance_id ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS ids,
		               array_agg(answer.outcome ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS outcomes,
		               array_agg(answer.quantity ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS quantities,
		               array_agg(COALESCE(answer.basis, '') ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS bases,
		               array_agg(answer.decided_at ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS decided,
		               array_agg(answer.recorded_at ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS recorded,
		               array_agg((SELECT json_build_object(
		                                 'enteredBy', entry.entered_by, 'original', entry.original_ref, 'enteredAt', entry.entered_at)::text
		                            FROM transport_fulfillment.booking_answer_manual_entry AS entry
		                           WHERE entry.tenant_id = answer.tenant_id AND entry.booking_id = answer.booking_id
		                             AND entry.acceptance_ref = answer.acceptance_id)
		                         ORDER BY answer.decided_at, answer.inserted_at, answer.acceptance_id) AS manual
		          FROM transport_fulfillment.booking_answer AS answer
		         WHERE answer.tenant_id = booking.tenant_id
		           AND answer.booking_id = booking.booking_id) AS answers ON true
		  ORDER BY `+plan.OrderBy,
		bookingRequestRowScanner(tenant), bookingRequestPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// bookingRequestRowScanner 读回订舱册的一行并派生状态词（bookingStanding）。
func bookingRequestRowScanner(tenant domain.TenantID) func(pgx.Row) (ports.BookingRequestCatalogueRow, error) {
	return func(scanned pgx.Row) (ports.BookingRequestCatalogueRow, error) {
		var (
			row                                   ports.BookingRequestCatalogueRow
			cancelledAt                           *time.Time
			submittedBy, cancelledBy              *string
			documentNumber, documentNumberAbsence *string
			commissionFound                       bool
			commissionNumber, commissionAbsence   *string
			ids, outcomes, bases                  []string
			quantities                            []int64
			decidedTimes, recordedAt              []time.Time
			manualEntries                         []*string
		)
		if err := scanned.Scan(
			&row.Booking, &row.Commission, &row.Schedule, &row.Quantity, &row.Unit,
			&row.RequestedAt, &cancelledAt, &row.RecordedAt, &submittedBy, &cancelledBy,
			&documentNumber, &documentNumberAbsence, &commissionFound, &commissionNumber, &commissionAbsence,
			&ids, &outcomes, &quantities, &bases, &decidedTimes, &recordedAt, &manualEntries,
		); err != nil {
			return ports.BookingRequestCatalogueRow{}, err
		}
		numbering, err := documentNumberingFromColumns(documentNumber, documentNumberAbsence)
		if err != nil {
			return ports.BookingRequestCatalogueRow{}, fmt.Errorf("booking %q: %w", row.Booking, err)
		}
		row.DocumentNumber = numbering
		if commissionFound {
			commissionNumbering, err := documentNumberingFromColumns(commissionNumber, commissionAbsence)
			if err != nil {
				return ports.BookingRequestCatalogueRow{}, fmt.Errorf("commission %q: %w", row.Commission, err)
			}
			row.CommissionDocumentNumber = &commissionNumbering
		}
		if cancelledAt != nil {
			utc := cancelledAt.UTC()
			row.CancelledAt = &utc
		}
		row.RequestedAt = row.RequestedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		submitter, err := registrantFromColumn(submittedBy)
		if err != nil {
			return ports.BookingRequestCatalogueRow{}, fmt.Errorf("booking %q: %w", row.Booking, err)
		}
		canceller, err := registrantFromColumn(cancelledBy)
		if err != nil {
			return ports.BookingRequestCatalogueRow{}, fmt.Errorf("booking %q: %w", row.Booking, err)
		}
		row.SubmittedBy, row.CancelledBy = submitter.String(), canceller.String()
		row.Answers = make([]ports.BookingAnswerCatalogueRow, 0, len(ids))
		for index := range ids {
			var note *ports.ManualEntryNote
			if raw := manualEntries[index]; raw != nil {
				if note, err = manualEntryNoteFromColumn([]byte(*raw)); err != nil {
					return ports.BookingRequestCatalogueRow{}, fmt.Errorf("booking %q answer %q: %w", row.Booking, ids[index], err)
				}
			}
			row.Answers = append(row.Answers, ports.BookingAnswerCatalogueRow{
				Answer:      ids[index],
				Outcome:     outcomes[index],
				Quantity:    quantities[index],
				Basis:       bases[index],
				DecidedAt:   decidedTimes[index].UTC(),
				RecordedAt:  recordedAt[index].UTC(),
				ManualEntry: note,
			})
		}
		standing, current, err := bookingStanding(tenant, row.Booking, row.Quantity, row.Answers)
		if err != nil {
			return ports.BookingRequestCatalogueRow{}, fmt.Errorf("booking %q: %w", row.Booking, err)
		}
		row.Standing = standing
		if current >= 0 {
			row.Current = &row.Answers[current]
		}
		return row, nil
	}
}

func bookingRequestPosition(row ports.BookingRequestCatalogueRow, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.RequestedAt)
	if sortField == "booking" {
		value = row.Booking
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.Booking}}
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

// bookingAnswerBouncePages 把 ports.BookingAnswerBouncePages 落到列上。basis 落在存定宽码的列上，词经 bounceBasisCodeOf 换码；
// q 在答复方、订舱与答复标识上做字面包含。
var bookingAnswerBouncePages = ports.BookingAnswerBouncePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"lastBouncedAt": "last_bounced_at", "count": "bounce_count"},
	Identity: []string{"source_ref", "booking_id", "acceptance_id", "basis"},
	Filters: map[string]string{
		ports.BookingAnswerBounceBookingFilter: "booking_id",
		ports.BookingAnswerBounceSourceFilter:  "source_ref",
		"basis":                                "basis",
	},
	Codes:   map[string]func(string) (int64, error){"basis": bounceBasisCodeOf},
	Keyword: []string{"source_ref", "booking_id", "acceptance_id"},
})

// scannedBookingAnswerBounce 是挡回册取页读回的一行。依据另留库里的定宽码：游标位置要与 ORDER BY 用的是同一个值，行上只有
// 译回的词。
type scannedBookingAnswerBounce struct {
	row  ports.BookingAnswerBounceCatalogueRow
	code int16
}

// ListBookingAnswerBounces 上列承运答复挡回册（ADR-0239 决定二，票 demo-chain-gaps/167）：缺省最近一次挡回的在前，同刻按
// （答复方、订舱、答复标识、依据）倒序。依据码经 bounceBasisWord 译回词。
func (catalogue *ReviewCatalogue) ListBookingAnswerBounces(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.BookingAnswerBounceCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.BookingAnswerBounceCatalogueRow], error) {
		return cataloguepage.Result[ports.BookingAnswerBounceCatalogueRow]{}, fmt.Errorf("list booking answer bounces: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := bookingAnswerBouncePages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.booking_answer_bounce"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source), plan.PageSQL(
		`source_ref, booking_id, acceptance_id, basis, first_bounced_at, last_bounced_at, bounce_count`, source),
		scanBookingAnswerBounce, bookingAnswerBouncePosition)
	if err != nil {
		return fail(err)
	}
	rows := make([]ports.BookingAnswerBounceCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		rows = append(rows, scanned.row)
	}
	return cataloguepage.Result[ports.BookingAnswerBounceCatalogueRow]{Rows: rows, Next: page.Next, Total: page.Total}, nil
}

func scanBookingAnswerBounce(scanned pgx.Row) (scannedBookingAnswerBounce, error) {
	var bounce scannedBookingAnswerBounce
	row := &bounce.row
	if err := scanned.Scan(
		&row.Source, &row.Booking, &row.Answer, &bounce.code, &row.FirstBouncedAt, &row.LastBouncedAt, &row.Count,
	); err != nil {
		return scannedBookingAnswerBounce{}, err
	}
	word, known := bounceBasisWord(bounce.code)
	if !known {
		return scannedBookingAnswerBounce{}, fmt.Errorf("stored basis code %d is not a basis", bounce.code)
	}
	row.Basis = word
	row.FirstBouncedAt = row.FirstBouncedAt.UTC()
	row.LastBouncedAt = row.LastBouncedAt.UTC()
	return bounce, nil
}

func bookingAnswerBouncePosition(bounce scannedBookingAnswerBounce, sortField string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(bounce.row.LastBouncedAt)
	if sortField == "count" {
		value = cataloguepage.FormatInteger(bounce.row.Count)
	}
	return cataloguepage.Position{Value: value, Identity: []string{
		bounce.row.Source, bounce.row.Booking, bounce.row.Answer, cataloguepage.FormatInteger(int64(bounce.code)),
	}}
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

// bounceBasisCodeOf 把筛选维 basis 的词换成库里的定宽码，码表同样只问 bounceBasisCode。
func bounceBasisCodeOf(word string) (int64, error) {
	for basis := ports.BookingAnswerBounceBasisInvalid + 1; basis.String() != ""; basis++ {
		if basis.String() != word {
			continue
		}
		if stored, known := bounceBasisCode(basis); known {
			return int64(stored), nil
		}
	}
	return 0, fmt.Errorf("%q is not a bounce basis", word)
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

// pendingAttemptResultPages 把 ports.PendingAttemptResultPages 落到列上：列是两侧合并之后导出的那几列。q 在尝试、对象、任务与
// 揽收一侧的来源身份上做字面包含。
var pendingAttemptResultPages = ports.PendingAttemptResultPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"occurredAt": "occurred_at"},
	Identity: []string{"side", "attempt_ref", "object_ref", "source_id"},
	Filters:  map[string]string{"side": "side"},
	Keyword:  []string{"attempt_ref", "object_ref", "task_ref", "source_id"},
})

// pendingAttemptResults 是两侧还在待确认的逐对象结果合并成的一张导出表，租户条件在两侧各自里面（$1）。**还在待确认**的才列——
// 已凭后补来源更正成确定结果的那一格，靠更正册里有没有行判（反连接），首次提交的待确认行本身永不改写。
const pendingAttemptResults = `(
		        SELECT 'PICKUP' AS side, r.source_id, a.attempt_ref, r.object_ref, a.task_ref, a.executed_by, a.place_ref,
		               r.source_ref, r.basis, r.occurred_at
		          FROM transport_fulfillment.pickup_attempt_result AS r
		          JOIN transport_fulfillment.pickup_attempt AS a
		            ON a.tenant_id = r.tenant_id AND a.source_id = r.source_id
		         WHERE r.tenant_id = $1
		           AND r.outcome = 'PENDING_CONFIRMATION'
		           AND NOT EXISTS (SELECT 1 FROM transport_fulfillment.pickup_attempt_result_correction AS c
		                            WHERE c.tenant_id = r.tenant_id AND c.source_id = r.source_id AND c.object_ref = r.object_ref)
		        UNION ALL
		        SELECT 'DELIVERY' AS side, '' AS source_id, a.attempt_ref, r.object_ref, a.task_ref, a.executed_by, a.place_ref,
		               r.source_ref, r.basis, r.occurred_at
		          FROM transport_fulfillment.delivery_attempt_result AS r
		          JOIN transport_fulfillment.delivery_attempt AS a
		            ON a.tenant_id = r.tenant_id AND a.attempt_ref = r.attempt_ref
		         WHERE r.tenant_id = $1
		           AND r.outcome = 'PENDING_CONFIRMATION'
		           AND NOT EXISTS (SELECT 1 FROM transport_fulfillment.delivery_attempt_result_correction AS c
		                            WHERE c.tenant_id = r.tenant_id AND c.attempt_ref = r.attempt_ref AND c.object_ref = r.object_ref)
		        ) AS pending`

// ListPendingAttemptResults 上列还在待确认的逐对象尝试结果（票 docs-consistency/code/90）：揽收与派送两侧合并；缺省待确认登记得
// 最早的在前，同刻按（一侧、尝试、对象、来源身份）升序。揽收一侧带尝试提交的来源身份：更正口按它指名被更正的那一格。
func (catalogue *ReviewCatalogue) ListPendingAttemptResults(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PendingAttemptResultRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.PendingAttemptResultRow], error) {
		return cataloguepage.Result[ports.PendingAttemptResultRow]{}, fmt.Errorf("list pending attempt results: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := pendingAttemptResultPages.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(pendingAttemptResults), plan.PageSQL(
		`side, source_id, attempt_ref, object_ref, task_ref, executed_by, place_ref, source_ref, basis, occurred_at`,
		pendingAttemptResults),
		scanPendingAttemptResultRow, pendingAttemptResultPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanPendingAttemptResultRow(scanned pgx.Row) (ports.PendingAttemptResultRow, error) {
	var row ports.PendingAttemptResultRow
	if err := scanned.Scan(&row.Side, &row.SourceID, &row.Attempt, &row.Object, &row.Task, &row.ExecutedBy, &row.Place,
		&row.Source, &row.Gap, &row.OccurredAt); err != nil {
		return ports.PendingAttemptResultRow{}, err
	}
	row.OccurredAt = row.OccurredAt.UTC()
	return row, nil
}

func pendingAttemptResultPosition(row ports.PendingAttemptResultRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(row.OccurredAt),
		Identity: []string{row.Side, row.Attempt, row.Object, row.SourceID},
	}
}
