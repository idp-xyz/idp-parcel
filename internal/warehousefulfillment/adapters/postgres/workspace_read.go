package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WorkspaceReads 是仓配工作台与各业务模块详情/台账读适配器，只读不写。
type WorkspaceReads struct {
	db *bentopg.DB
}

func NewWorkspaceReads(db *bentopg.DB) (*WorkspaceReads, error) {
	if db == nil {
		return nil, errors.New("warehouse fulfillment: workspace reads require a db")
	}
	return &WorkspaceReads{db: db}, nil
}

func dateOrEmpty(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

// SettingsOverview 读取本租户全部八项仓配规则的当前生效配置。未配置项如实留空/nil。
func (store *WorkspaceReads) SettingsOverview(ctx context.Context, tenant domain.TenantID) (ports.SettingsOverview, error) {
	const op = "warehouse settings overview"
	if tenant.String() == "" {
		return ports.SettingsOverview{}, fmt.Errorf("%s: 缺租户", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SettingsOverview{}, fmt.Errorf("%s: %w", op, err)
	}
	out := ports.SettingsOverview{}

	// 1. 参考分配策略最新修订
	{
		var item ports.AllocationStrategyOverview
		err := querier.QueryRow(ctx,
			`SELECT revision, operator_id, registered_at
			   FROM warehouse_fulfillment.allocation_strategy
			  WHERE tenant_id = $1
			  ORDER BY revision DESC LIMIT 1`,
			tenant.String(),
		).Scan(&item.Revision, &item.Operator, &item.RegisteredAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, fmt.Errorf("%s: allocation strategy: %w", op, err)
		}
		if err == nil {
			out.AllocationStrategy = &item
		}
	}

	// 2. 各货主缺货处置规则最新修订
	{
		rows, err := querier.Query(ctx,
			`SELECT DISTINCT ON (owner_account_id) owner_account_id, revision, coalesce(kind, ''), coalesce(partial_shipment, ''), operator_id, registered_at
			   FROM warehouse_fulfillment.shortage_policy
			  WHERE tenant_id = $1
			  ORDER BY owner_account_id, revision DESC`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: shortage policies: %w", op, err)
		}
		for rows.Next() {
			var item ports.ShortagePolicyOverview
			if err := rows.Scan(&item.OwnerAccount, &item.Revision, &item.Kind, &item.PartialShipment, &item.Operator, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: shortage policies: %w", op, err)
			}
			out.ShortagePolicies = append(out.ShortagePolicies, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return out, fmt.Errorf("%s: shortage policies: %w", op, err)
		}
		rows.Close()
	}

	// 3. 库存调整批准职责最新修订
	{
		var item ports.ApprovalDutyOverview
		err := querier.QueryRow(ctx,
			`SELECT revision, distinct_subjects, operator_id, registered_at
			   FROM warehouse_fulfillment.approval_duty
			  WHERE tenant_id = $1
			  ORDER BY revision DESC LIMIT 1`,
			tenant.String(),
		).Scan(&item.Revision, &item.DistinctSubjects, &item.Operator, &item.RegisteredAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, fmt.Errorf("%s: approval duty: %w", op, err)
		}
		if err == nil {
			out.ApprovalDuty = &item
		}
	}

	// 4. 各仓待认领处置规则最新修订及允许处置方式
	{
		rows, err := querier.Query(ctx,
			`SELECT DISTINCT ON (warehouse_code) warehouse_code, revision, claim_window_seconds, registrant, registered_at
			   FROM warehouse_fulfillment.unclaimed_disposal_rule_revision
			  WHERE tenant_id = $1
			  ORDER BY warehouse_code, revision DESC`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: disposal rules: %w", op, err)
		}
		for rows.Next() {
			var item ports.DisposalRuleOverview
			if err := rows.Scan(&item.WarehouseCode, &item.Revision, &item.ClaimWindowSeconds, &item.Registrant, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: disposal rules: %w", op, err)
			}
			out.DisposalRules = append(out.DisposalRules, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return out, fmt.Errorf("%s: disposal rules: %w", op, err)
		}
		rows.Close()
		for i := range out.DisposalRules {
			mRows, err := querier.Query(ctx,
				`SELECT method FROM warehouse_fulfillment.unclaimed_disposal_rule_method
				  WHERE tenant_id = $1 AND warehouse_code = $2 AND revision = $3
				  ORDER BY position`,
				tenant.String(), out.DisposalRules[i].WarehouseCode, out.DisposalRules[i].Revision,
			)
			if err != nil {
				return out, fmt.Errorf("%s: disposal methods: %w", op, err)
			}
			for mRows.Next() {
				var method string
				if err := mRows.Scan(&method); err != nil {
					mRows.Close()
					return out, fmt.Errorf("%s: disposal methods: %w", op, err)
				}
				out.DisposalRules[i].DisposalMethods = append(out.DisposalRules[i].DisposalMethods, method)
			}
			mRows.Close()
		}
	}

	// 5. 各仓计费重量与体积单位最新修订
	{
		rows, err := querier.Query(ctx,
			`SELECT DISTINCT ON (warehouse_code) warehouse_code, revision, weight_unit, volume_unit, registered_at
			   FROM warehouse_fulfillment.charge_measure_unit_revision
			  WHERE tenant_id = $1
			  ORDER BY warehouse_code, revision DESC`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: charge units: %w", op, err)
		}
		for rows.Next() {
			var item ports.ChargeUnitsOverview
			if err := rows.Scan(&item.WarehouseCode, &item.Revision, &item.WeightUnit, &item.VolumeUnit, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: charge units: %w", op, err)
			}
			out.ChargeUnits = append(out.ChargeUnits, item)
		}
		rows.Close()
	}

	// 6. 各仓库龄口径
	{
		rows, err := querier.Query(ctx,
			`SELECT warehouse_code, basis, registered_at
			   FROM warehouse_fulfillment.charge_aging_basis
			  WHERE tenant_id = $1
			  ORDER BY warehouse_code`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: charge aging: %w", op, err)
		}
		for rows.Next() {
			var item ports.ChargeAgingOverview
			if err := rows.Scan(&item.WarehouseCode, &item.Basis, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: charge aging: %w", op, err)
			}
			out.ChargeAging = append(out.ChargeAging, item)
		}
		rows.Close()
	}

	// 7. 各仓增值服务项目
	{
		rows, err := querier.Query(ctx,
			`SELECT warehouse_code, project_name, registered_at
			   FROM warehouse_fulfillment.charge_service_project
			  WHERE tenant_id = $1
			  ORDER BY warehouse_code, project_name`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: charge projects: %w", op, err)
		}
		for rows.Next() {
			var item ports.ChargeProjectOverview
			if err := rows.Scan(&item.WarehouseCode, &item.ProjectName, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: charge projects: %w", op, err)
			}
			out.ChargeProjects = append(out.ChargeProjects, item)
		}
		rows.Close()
	}

	// 8. 代理仓连接器
	{
		rows, err := querier.Query(ctx,
			`SELECT warehouse_code, registered_at
			   FROM warehouse_fulfillment.agent_connector
			  WHERE tenant_id = $1
			  ORDER BY warehouse_code`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: agent connectors: %w", op, err)
		}
		for rows.Next() {
			var item ports.AgentConnectorOverview
			if err := rows.Scan(&item.WarehouseCode, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: agent connectors: %w", op, err)
			}
			out.AgentConnectors = append(out.AgentConnectors, item)
		}
		rows.Close()
	}

	return out, nil
}

// OutboundRequestDetail 读取单张出库请求的完整 360° 明细、预占、拣货、复核、包裹面单、确认与回库任务。
func (store *WorkspaceReads) OutboundRequestDetail(
	ctx context.Context,
	tenant domain.TenantID,
	requestID string,
) (ports.OutboundRequestDetail, bool, error) {
	const op = "outbound request detail"
	if tenant.String() == "" || requestID == "" {
		return ports.OutboundRequestDetail{}, false, fmt.Errorf("%s: 缺租户或出库请求号", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OutboundRequestDetail{}, false, fmt.Errorf("%s: %w", op, err)
	}
	var (
		out                                               ports.OutboundRequestDetail
		service, basis, operator, number, absence, parent *string
		prefetchShipment, prefetchDeclared, partial       *string
	)
	err = querier.QueryRow(ctx,
		`SELECT `+outboundRequestListColumns+`
		   FROM warehouse_fulfillment.outbound_request
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), requestID,
	).Scan(
		&out.Request.RequestID, &out.Request.OwnerAccount, &out.Request.WarehouseCode, &out.Request.Origin, &out.Request.RequestKey,
		&out.Request.Dispatch, &service, &out.Request.Recipient, &out.Request.Address, &basis, &operator, &out.Request.Status,
		&number, &absence, &out.Request.ReceivedAt, &parent, &out.Request.PlatformStocking, &prefetchShipment, &prefetchDeclared, &partial,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OutboundRequestDetail{}, false, nil
	}
	if err != nil {
		return ports.OutboundRequestDetail{}, false, fmt.Errorf("%s: %w", op, err)
	}
	out.Request.Service = valueOrEmpty(service)
	out.Request.Basis = valueOrEmpty(basis)
	out.Request.Operator = valueOrEmpty(operator)
	out.Request.DocumentNumber = valueOrEmpty(number)
	out.Request.NumberingAbsence = valueOrEmpty(absence)
	out.Request.ParentRequestID = valueOrEmpty(parent)
	out.Request.PrefetchShipmentID = valueOrEmpty(prefetchShipment)
	out.Request.PrefetchDeclaredParcelID = valueOrEmpty(prefetchDeclared)
	out.Request.PartialShipment = valueOrEmpty(partial)
	if out.PartialShipment, err = partialShipmentDetail(ctx, querier, tenant, out.Request); err != nil {
		return out, false, fmt.Errorf("%s: partial shipment: %w", op, err)
	}

	// 明细行
	{
		rows, err := querier.Query(ctx,
			`SELECT line_index, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date, quantity
			   FROM warehouse_fulfillment.outbound_request_line
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY line_index`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: lines: %w", op, err)
		}
		for rows.Next() {
			var (
				line         ports.OutboundRequestDetailLine
				alias, batch *string
				expiry       *time.Time
			)
			if err := rows.Scan(&line.LineIndex, &line.GoodsCode, &line.Unit, &line.StockState, &alias, &batch, &expiry, &line.Quantity); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: lines: %w", op, err)
			}
			line.LabelAlias = valueOrEmpty(alias)
			line.BatchNumber = valueOrEmpty(batch)
			line.ExpiryDate = dateOrEmpty(expiry)
			out.Lines = append(out.Lines, line)
		}
		rows.Close()
	}

	// 预占行
	{
		rows, err := querier.Query(ctx,
			`SELECT line_index, goods_code, stock_state, label_alias, batch_number, expiry_date, quantity, from_unlabeled, wanted
			   FROM warehouse_fulfillment.outbound_reservation
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY line_index`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: reservations: %w", op, err)
		}
		for rows.Next() {
			var (
				res          ports.OutboundReservationDetail
				alias, batch *string
				expiry       *time.Time
			)
			if err := rows.Scan(&res.LineIndex, &res.GoodsCode, &res.StockState, &alias, &batch, &expiry, &res.Quantity, &res.FromUnlabeled, &res.Wanted); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: reservations: %w", op, err)
			}
			res.LabelAlias = valueOrEmpty(alias)
			res.BatchNumber = valueOrEmpty(batch)
			res.ExpiryDate = dateOrEmpty(expiry)
			out.Request.ReservedQuantity += res.Quantity
			out.Request.WantedQuantity += res.Wanted
			out.Reservations = append(out.Reservations, res)
		}
		rows.Close()
	}

	// 关联波次
	{
		rows, err := querier.Query(ctx,
			`SELECT wave_id FROM warehouse_fulfillment.wave_request
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY wave_id`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: waves: %w", op, err)
		}
		for rows.Next() {
			var waveID string
			if err := rows.Scan(&waveID); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: waves: %w", op, err)
			}
			out.Waves = append(out.Waves, waveID)
		}
		rows.Close()
	}

	// 拣货任务
	{
		rows, err := querier.Query(ctx,
			`SELECT task_id, wave_id, location_code, staging_location, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id
			   FROM warehouse_fulfillment.pick_task
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY task_id`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: pick tasks: %w", op, err)
		}
		for rows.Next() {
			var (
				task            ports.OutboundPickTaskDetail
				staging, factID *string
			)
			if err := rows.Scan(&task.TaskID, &task.WaveID, &task.LocationCode, &staging, &task.GoodsCode, &task.Unit, &task.StockState, &task.InboundBatchID, &task.Quantity, &task.Picked, &task.Status, &factID); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: pick tasks: %w", op, err)
			}
			task.StagingLocation = valueOrEmpty(staging)
			task.FactID = valueOrEmpty(factID)
			out.PickTasks = append(out.PickTasks, task)
		}
		rows.Close()
	}

	// 复核记录
	{
		rows, err := querier.Query(ctx,
			`SELECT fact_id, observed, matched, checked_at
			   FROM warehouse_fulfillment.outbound_check
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY checked_at DESC, fact_id`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: checks: %w", op, err)
		}
		for rows.Next() {
			var chk ports.OutboundCheckDetail
			if err := rows.Scan(&chk.FactID, &chk.Observed, &chk.Matched, &chk.CheckedAt); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: checks: %w", op, err)
			}
			out.Checks = append(out.Checks, chk)
		}
		rows.Close()
	}

	// 装箱包裹 + 货主面单 + 包裹明细行
	{
		rows, err := querier.Query(ctx,
			`SELECT p.parcel_sequence, p.packaging_material, p.weight_value, p.weight_unit,
			        p.length_value, p.width_value, p.height_value, p.dimension_unit,
			        p.shipment_request_id, p.confirmation_state, p.cutoff_state, l.external_number
			   FROM warehouse_fulfillment.outbound_parcel p
			   LEFT JOIN warehouse_fulfillment.outbound_owner_label l
			     ON l.tenant_id = p.tenant_id AND l.request_id = p.request_id AND l.parcel_sequence = p.parcel_sequence
			  WHERE p.tenant_id = $1 AND p.request_id = $2
			  ORDER BY p.parcel_sequence`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: parcels: %w", op, err)
		}
		for rows.Next() {
			var (
				parcel                        ports.OutboundParcelDetail
				shipID, confState, cut, label *string
			)
			if err := rows.Scan(
				&parcel.ParcelSequence, &parcel.PackagingMaterial, &parcel.WeightValue, &parcel.WeightUnit,
				&parcel.LengthValue, &parcel.WidthValue, &parcel.HeightValue, &parcel.DimensionUnit,
				&shipID, &confState, &cut, &label,
			); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: parcels: %w", op, err)
			}
			parcel.ShipmentRequestID = valueOrEmpty(shipID)
			parcel.ConfirmationState = valueOrEmpty(confState)
			parcel.CutoffState = valueOrEmpty(cut)
			parcel.OwnerLabel = valueOrEmpty(label)
			out.Parcels = append(out.Parcels, parcel)
		}
		rows.Close()

		for i := range out.Parcels {
			lRows, err := querier.Query(ctx,
				`SELECT goods_code, unit_name, quantity
				   FROM warehouse_fulfillment.outbound_parcel_line
				  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3
				  ORDER BY line_index`,
				tenant.String(), requestID, out.Parcels[i].ParcelSequence,
			)
			if err != nil {
				return out, false, fmt.Errorf("%s: parcel lines: %w", op, err)
			}
			for lRows.Next() {
				var line ports.OutboundParcelLineDetail
				if err := lRows.Scan(&line.GoodsCode, &line.Unit, &line.Quantity); err != nil {
					lRows.Close()
					return out, false, fmt.Errorf("%s: parcel lines: %w", op, err)
				}
				out.Parcels[i].Lines = append(out.Parcels[i].Lines, line)
			}
			lRows.Close()
		}
	}

	// 出库确认与更正态
	{
		rows, err := querier.Query(ctx,
			`SELECT c.fact_id, c.parcel_sequence, c.kind, c.offline, c.missing,
			        c.external_number, c.pickup_party, c.pickup_credential, c.occurred_at,
			        cor.kind, cor.status
			   FROM warehouse_fulfillment.outbound_confirmation c
			   LEFT JOIN warehouse_fulfillment.outbound_confirmation_correction cor
			     ON cor.tenant_id = c.tenant_id AND cor.fact_id = c.fact_id
			  WHERE c.tenant_id = $1 AND c.request_id = $2
			  ORDER BY c.occurred_at DESC, c.fact_id`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: confirmations: %w", op, err)
		}
		for rows.Next() {
			var (
				item                                   ports.OutboundConfirmationDetail
				ext, party, cred, corrKind, corrStatus *string
			)
			if err := rows.Scan(
				&item.FactID, &item.ParcelSequence, &item.Kind, &item.Offline, &item.Missing,
				&ext, &party, &cred, &item.OccurredAt, &corrKind, &corrStatus,
			); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: confirmations: %w", op, err)
			}
			item.ExternalNumber = valueOrEmpty(ext)
			item.PickupParty = valueOrEmpty(party)
			item.PickupCredential = valueOrEmpty(cred)
			item.CorrectionKind = valueOrEmpty(corrKind)
			item.CorrectionStatus = valueOrEmpty(corrStatus)
			out.Confirmations = append(out.Confirmations, item)
		}
		rows.Close()
	}

	// 截单回库任务
	{
		rows, err := querier.Query(ctx,
			`SELECT return_id, parcel_sequence, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, status, fact_id
			   FROM warehouse_fulfillment.return_task
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY return_id`,
			tenant.String(), requestID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: return tasks: %w", op, err)
		}
		for rows.Next() {
			var (
				task   ports.OutboundReturnTaskDetail
				seq    *int
				factID *string
			)
			if err := rows.Scan(&task.ReturnID, &seq, &task.LocationCode, &task.GoodsCode, &task.Unit, &task.StockState, &task.InboundBatchID, &task.Quantity, &task.Status, &factID); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: return tasks: %w", op, err)
			}
			if seq != nil {
				task.ParcelSequence = *seq
			}
			task.FactID = valueOrEmpty(factID)
			out.ReturnTasks = append(out.ReturnTasks, task)
		}
		rows.Close()
	}

	return out, true, nil
}

const wsFreezeColumns = `
	freeze_id, decision_id, owner_account_id, warehouse_code, target,
	location_code, goods_code, unit_name, stock_state, label_alias,
	inbound_batch_id, batch_number, expiry_date, carton_number,
	quantity, reason, operator_id, frozen_at, status,
	unfreeze_operator_id, unfrozen_at, unfreeze_note`

const freezeListed = `WITH listed AS (
	SELECT ` + wsFreezeColumns + `
	  FROM warehouse_fulfillment.stock_freeze
	 WHERE tenant_id = $1
)`

var freezeBinding = ports.StockFreezePages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"frozenAt": "frozen_at",
		"freezeId": "freeze_id",
	},
	Identity: []string{"freeze_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"target":        "target",
		"status":        "status",
	},
	Keyword: []string{"freeze_id", "owner_account_id", "warehouse_code", "goods_code", "location_code", "carton_number", "reason"},
})

func (store *WorkspaceReads) ListFreezes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.StockFreezeListRow], error) {
	const op = "list freezes"
	fail := func(err error) (cataloguepage.Result[ports.StockFreezeListRow], error) {
		return cataloguepage.Result[ports.StockFreezeListRow]{}, fmt.Errorf("%s: %w", op, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := freezeBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, freezeListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, freezeListed+" "+plan.PageSQL(wsFreezeColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.StockFreezeListRow
	for rows.Next() {
		var (
			item                                                            ports.StockFreezeListRow
			loc, gcode, unit, state, alias, ib, batch, carton, unOp, unNote *string
			expiry                                                          *time.Time
		)
		if err := rows.Scan(
			&item.FreezeID, &item.DecisionID, &item.OwnerAccount, &item.WarehouseCode, &item.Target,
			&loc, &gcode, &unit, &state, &alias, &ib, &batch, &expiry, &carton,
			&item.Quantity, &item.Reason, &item.Operator, &item.FrozenAt, &item.Status,
			&unOp, &item.UnfrozenAt, &unNote,
		); err != nil {
			return fail(err)
		}
		item.LocationCode = valueOrEmpty(loc)
		item.GoodsCode = valueOrEmpty(gcode)
		item.Unit = valueOrEmpty(unit)
		item.StockState = valueOrEmpty(state)
		item.LabelAlias = valueOrEmpty(alias)
		item.InboundBatchID = valueOrEmpty(ib)
		item.BatchNumber = valueOrEmpty(batch)
		item.ExpiryDate = dateOrEmpty(expiry)
		item.CartonNumber = valueOrEmpty(carton)
		item.UnfreezeOperator = valueOrEmpty(unOp)
		item.UnfreezeNote = valueOrEmpty(unNote)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.StockFreezeListRow, sortField string) cataloguepage.Position {
		if sortField == "freezeId" {
			return cataloguepage.Position{Value: item.FreezeID, Identity: []string{item.FreezeID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.FrozenAt), Identity: []string{item.FreezeID}}
	})
}

const countColumns = `
	count_id, decision_id, warehouse_code, owner_account_id, note, operator_id, created_at,
	status, started_by, started_at, completed_by, completed_at, voided_by, voided_at, void_reason`

const countListed = `WITH listed AS (
	SELECT ` + countColumns + `
	  FROM warehouse_fulfillment.stock_count
	 WHERE tenant_id = $1
)`

var countBinding = ports.StockCountPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"createdAt": "created_at",
		"countId":   "count_id",
	},
	Identity: []string{"count_id"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"ownerAccount":  "owner_account_id",
		"status":        "status",
	},
	Keyword: []string{"count_id", "warehouse_code", "owner_account_id", "note", "operator_id"},
})

func (store *WorkspaceReads) ListCounts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.StockCountRow], error) {
	const op = "list counts"
	fail := func(err error) (cataloguepage.Result[ports.StockCountRow], error) {
		return cataloguepage.Result[ports.StockCountRow]{}, fmt.Errorf("%s: %w", op, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := countBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, countListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, countListed+" "+plan.PageSQL(countColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.StockCountRow
	for rows.Next() {
		var (
			item                                                      ports.StockCountRow
			owner, note, startedBy, completedBy, voidedBy, voidReason *string
		)
		if err := rows.Scan(
			&item.CountID, &item.DecisionID, &item.WarehouseCode, &owner, &note, &item.Operator, &item.CreatedAt,
			&item.Status, &startedBy, &item.StartedAt, &completedBy, &item.CompletedAt, &voidedBy, &item.VoidedAt, &voidReason,
		); err != nil {
			return fail(err)
		}
		item.OwnerAccount = valueOrEmpty(owner)
		item.Note = valueOrEmpty(note)
		item.StartedBy = valueOrEmpty(startedBy)
		item.CompletedBy = valueOrEmpty(completedBy)
		item.VoidedBy = valueOrEmpty(voidedBy)
		item.VoidReason = valueOrEmpty(voidReason)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for i := range items {
		if err := store.fillCountMeta(ctx, querier, tenant.String(), &items[i]); err != nil {
			return fail(err)
		}
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.StockCountRow, sortField string) cataloguepage.Position {
		if sortField == "countId" {
			return cataloguepage.Position{Value: item.CountID, Identity: []string{item.CountID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.CreatedAt), Identity: []string{item.CountID}}
	})
}

func (store *WorkspaceReads) fillCountMeta(ctx context.Context, querier bentopg.Querier, tenantID string, item *ports.StockCountRow) error {
	locRows, err := querier.Query(ctx,
		`SELECT location_code FROM warehouse_fulfillment.stock_count_location WHERE tenant_id = $1 AND count_id = $2 ORDER BY location_code`,
		tenantID, item.CountID,
	)
	if err != nil {
		return err
	}
	for locRows.Next() {
		var code string
		if err := locRows.Scan(&code); err != nil {
			locRows.Close()
			return err
		}
		item.Locations = append(item.Locations, code)
	}
	locRows.Close()

	gRows, err := querier.Query(ctx,
		`SELECT goods_code FROM warehouse_fulfillment.stock_count_goods WHERE tenant_id = $1 AND count_id = $2 ORDER BY goods_code`,
		tenantID, item.CountID,
	)
	if err != nil {
		return err
	}
	for gRows.Next() {
		var code string
		if err := gRows.Scan(&code); err != nil {
			gRows.Close()
			return err
		}
		item.Goods = append(item.Goods, code)
	}
	gRows.Close()

	if err := querier.QueryRow(ctx,
		`SELECT count(*)::int FROM warehouse_fulfillment.count_entry WHERE tenant_id = $1 AND count_id = $2`,
		tenantID, item.CountID,
	).Scan(&item.EntryCount); err != nil {
		return err
	}
	if err := querier.QueryRow(ctx,
		`SELECT count(*)::int, count(*) FILTER (WHERE status = 'PENDING')::int
		   FROM warehouse_fulfillment.count_difference
		  WHERE tenant_id = $1 AND count_id = $2`,
		tenantID, item.CountID,
	).Scan(&item.DifferenceCount, &item.PendingDifferences); err != nil {
		return err
	}
	return nil
}

func (store *WorkspaceReads) CountDetail(ctx context.Context, tenant domain.TenantID, countID string) (ports.StockCountDetail, bool, error) {
	const op = "count detail"
	if tenant.String() == "" || countID == "" {
		return ports.StockCountDetail{}, false, fmt.Errorf("%s: 缺租户或盘点单号", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StockCountDetail{}, false, fmt.Errorf("%s: %w", op, err)
	}
	var (
		out                                                       ports.StockCountDetail
		owner, note, startedBy, completedBy, voidedBy, voidReason *string
	)
	err = querier.QueryRow(ctx,
		`SELECT `+countColumns+` FROM warehouse_fulfillment.stock_count WHERE tenant_id = $1 AND count_id = $2`,
		tenant.String(), countID,
	).Scan(
		&out.Count.CountID, &out.Count.DecisionID, &out.Count.WarehouseCode, &owner, &note, &out.Count.Operator, &out.Count.CreatedAt,
		&out.Count.Status, &startedBy, &out.Count.StartedAt, &completedBy, &out.Count.CompletedAt, &voidedBy, &out.Count.VoidedAt, &voidReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StockCountDetail{}, false, nil
	}
	if err != nil {
		return ports.StockCountDetail{}, false, fmt.Errorf("%s: %w", op, err)
	}
	out.Count.OwnerAccount = valueOrEmpty(owner)
	out.Count.Note = valueOrEmpty(note)
	out.Count.StartedBy = valueOrEmpty(startedBy)
	out.Count.CompletedBy = valueOrEmpty(completedBy)
	out.Count.VoidedBy = valueOrEmpty(voidedBy)
	out.Count.VoidReason = valueOrEmpty(voidReason)
	if err := store.fillCountMeta(ctx, querier, tenant.String(), &out.Count); err != nil {
		return out, false, fmt.Errorf("%s: %w", op, err)
	}

	// 清点记录
	{
		rows, err := querier.Query(ctx,
			`SELECT fact_id, location_code, occurred_at, device_sequence, device_id, operator_id, received_at
			   FROM warehouse_fulfillment.count_entry
			  WHERE tenant_id = $1 AND count_id = $2
			  ORDER BY occurred_at DESC, fact_id`,
			tenant.String(), countID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: entries: %w", op, err)
		}
		for rows.Next() {
			var entry ports.StockCountEntryDetail
			if err := rows.Scan(&entry.FactID, &entry.LocationCode, &entry.OccurredAt, &entry.DeviceSequence, &entry.DeviceID, &entry.OperatorID, &entry.ReceivedAt); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: entries: %w", op, err)
			}
			out.Entries = append(out.Entries, entry)
		}
		rows.Close()
		for i := range out.Entries {
			lRows, err := querier.Query(ctx,
				`SELECT owner_account_id, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date, quantity, observed
				   FROM warehouse_fulfillment.count_entry_line
				  WHERE tenant_id = $1 AND fact_id = $2
				  ORDER BY line_index`,
				tenant.String(), out.Entries[i].FactID,
			)
			if err != nil {
				return out, false, fmt.Errorf("%s: entry lines: %w", op, err)
			}
			for lRows.Next() {
				var (
					line                        ports.StockCountEntryLineDetail
					ow, gc, un, st, al, bt, obs *string
					exp                         *time.Time
				)
				if err := lRows.Scan(&ow, &gc, &un, &st, &al, &bt, &exp, &line.Quantity, &obs); err != nil {
					lRows.Close()
					return out, false, fmt.Errorf("%s: entry lines: %w", op, err)
				}
				line.OwnerAccount = valueOrEmpty(ow)
				line.GoodsCode = valueOrEmpty(gc)
				line.Unit = valueOrEmpty(un)
				line.StockState = valueOrEmpty(st)
				line.LabelAlias = valueOrEmpty(al)
				line.BatchNumber = valueOrEmpty(bt)
				line.ExpiryDate = dateOrEmpty(exp)
				line.Observed = valueOrEmpty(obs)
				out.Entries[i].Lines = append(out.Entries[i].Lines, line)
			}
			lRows.Close()
		}
	}

	// 盘点差额
	{
		rows, err := querier.Query(ctx,
			`SELECT difference_id, count_id, owner_account_id, warehouse_code, location_code, goods_code, unit_name, stock_state,
			        label_alias, batch_number, expiry_date, book_quantity, counted_quantity, status,
			        approved_by, approved_at, approval_note, rejected_by, rejected_at, reject_reason
			   FROM warehouse_fulfillment.count_difference
			  WHERE tenant_id = $1 AND count_id = $2
			  ORDER BY difference_id`,
			tenant.String(), countID,
		)
		if err != nil {
			return out, false, fmt.Errorf("%s: differences: %w", op, err)
		}
		for rows.Next() {
			var (
				diff                                     ports.StockCountDifferenceDetail
				al, bt, appBy, appNote, rejBy, rejReason *string
				exp                                      *time.Time
			)
			if err := rows.Scan(
				&diff.DifferenceID, &diff.CountID, &diff.OwnerAccount, &diff.WarehouseCode, &diff.LocationCode,
				&diff.GoodsCode, &diff.Unit, &diff.StockState, &al, &bt, &exp, &diff.BookQuantity, &diff.CountedQuantity,
				&diff.Status, &appBy, &diff.ApprovedAt, &appNote, &rejBy, &diff.RejectedAt, &rejReason,
			); err != nil {
				rows.Close()
				return out, false, fmt.Errorf("%s: differences: %w", op, err)
			}
			diff.LabelAlias = valueOrEmpty(al)
			diff.BatchNumber = valueOrEmpty(bt)
			diff.ExpiryDate = dateOrEmpty(exp)
			diff.ApprovedBy = valueOrEmpty(appBy)
			diff.ApprovalNote = valueOrEmpty(appNote)
			diff.RejectedBy = valueOrEmpty(rejBy)
			diff.RejectReason = valueOrEmpty(rejReason)
			out.Differences = append(out.Differences, diff)
		}
		rows.Close()
	}

	return out, true, nil
}

const adjustmentColumns = `
	adjustment_id, owner_account_id, warehouse_code, reason, operator_id, submitted_at, status,
	approved_by, approved_at, approval_note, rejected_by, rejected_at, reject_reason`

const adjustmentListed = `WITH listed AS (
	SELECT ` + adjustmentColumns + `
	  FROM warehouse_fulfillment.stock_adjustment
	 WHERE tenant_id = $1
)`

var adjustmentBinding = ports.StockAdjustmentPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"submittedAt":  "submitted_at",
		"adjustmentId": "adjustment_id",
	},
	Identity: []string{"adjustment_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"status":        "status",
	},
	Keyword: []string{"adjustment_id", "owner_account_id", "warehouse_code", "reason", "operator_id"},
})

func (store *WorkspaceReads) ListAdjustments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.StockAdjustmentRow], error) {
	const op = "list adjustments"
	fail := func(err error) (cataloguepage.Result[ports.StockAdjustmentRow], error) {
		return cataloguepage.Result[ports.StockAdjustmentRow]{}, fmt.Errorf("%s: %w", op, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := adjustmentBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, adjustmentListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, adjustmentListed+" "+plan.PageSQL(adjustmentColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.StockAdjustmentRow
	for rows.Next() {
		var (
			item                             ports.StockAdjustmentRow
			appBy, appNote, rejBy, rejReason *string
		)
		if err := rows.Scan(
			&item.AdjustmentID, &item.OwnerAccount, &item.WarehouseCode, &item.Reason, &item.Operator, &item.SubmittedAt, &item.Status,
			&appBy, &item.ApprovedAt, &appNote, &rejBy, &item.RejectedAt, &rejReason,
		); err != nil {
			return fail(err)
		}
		item.ApprovedBy = valueOrEmpty(appBy)
		item.ApprovalNote = valueOrEmpty(appNote)
		item.RejectedBy = valueOrEmpty(rejBy)
		item.RejectReason = valueOrEmpty(rejReason)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for i := range items {
		lRows, err := querier.Query(ctx,
			`SELECT line_index, kind, location_code, goods_code, unit_name, stock_state,
			        label_alias, batch_number, expiry_date, inbound_batch_id, quantity, basis
			   FROM warehouse_fulfillment.stock_adjustment_line
			  WHERE tenant_id = $1 AND adjustment_id = $2
			  ORDER BY line_index`,
			tenant.String(), items[i].AdjustmentID,
		)
		if err != nil {
			return fail(err)
		}
		for lRows.Next() {
			var (
				line              ports.StockAdjustmentLineDetail
				al, bt, ib, basis *string
				exp               *time.Time
			)
			if err := lRows.Scan(&line.LineIndex, &line.Kind, &line.LocationCode, &line.GoodsCode, &line.Unit, &line.StockState, &al, &bt, &exp, &ib, &line.Quantity, &basis); err != nil {
				lRows.Close()
				return fail(err)
			}
			line.LabelAlias = valueOrEmpty(al)
			line.BatchNumber = valueOrEmpty(bt)
			line.ExpiryDate = dateOrEmpty(exp)
			line.InboundBatchID = valueOrEmpty(ib)
			line.Basis = valueOrEmpty(basis)
			items[i].Lines = append(items[i].Lines, line)
		}
		lRows.Close()
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.StockAdjustmentRow, sortField string) cataloguepage.Position {
		if sortField == "adjustmentId" {
			return cataloguepage.Position{Value: item.AdjustmentID, Identity: []string{item.AdjustmentID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.SubmittedAt), Identity: []string{item.AdjustmentID}}
	})
}

const jobDiscrepancyColumns = `
	discrepancy_id, fact_id, line_position, owner_account_id, warehouse_code,
	source_location, location_code, goods_code, unit_name, stock_state,
	label_alias, batch_number, expiry_date, inbound_batch_id, carton_number,
	note, quantity, short_on_book, held_quantity, carton_missing, status,
	approved_by, approved_at, approval_note, rejected_by, rejected_at, reject_reason`

const jobDiscrepancyListed = `WITH listed AS (
	SELECT ` + jobDiscrepancyColumns + `
	  FROM warehouse_fulfillment.stock_discrepancy
	 WHERE tenant_id = $1
)`

var jobDiscrepancyBinding = ports.JobDiscrepancyPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"discrepancyId": "discrepancy_id",
	},
	Identity: []string{"discrepancy_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"status":        "status",
	},
	Keyword: []string{"discrepancy_id", "fact_id", "owner_account_id", "warehouse_code", "goods_code", "location_code", "carton_number"},
})

func (store *WorkspaceReads) ListJobDiscrepancies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.JobDiscrepancyRow], error) {
	const op = "list job discrepancies"
	fail := func(err error) (cataloguepage.Result[ports.JobDiscrepancyRow], error) {
		return cataloguepage.Result[ports.JobDiscrepancyRow]{}, fmt.Errorf("%s: %w", op, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := jobDiscrepancyBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, jobDiscrepancyListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, jobDiscrepancyListed+" "+plan.PageSQL(jobDiscrepancyColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.JobDiscrepancyRow
	for rows.Next() {
		var (
			item                                                                           ports.JobDiscrepancyRow
			srcLoc, loc, gc, un, st, al, bt, ib, carton, note, appBy, appNote, rejBy, rejR *string
			exp                                                                            *time.Time
		)
		if err := rows.Scan(
			&item.DiscrepancyID, &item.FactID, &item.LinePosition, &item.OwnerAccount, &item.WarehouseCode,
			&srcLoc, &loc, &gc, &un, &st, &al, &bt, &exp, &ib, &carton, &note,
			&item.Quantity, &item.ShortOnBook, &item.HeldQuantity, &item.CartonMissing, &item.Status,
			&appBy, &item.ApprovedAt, &appNote, &rejBy, &item.RejectedAt, &rejR,
		); err != nil {
			return fail(err)
		}
		item.SourceLocation = valueOrEmpty(srcLoc)
		item.LocationCode = valueOrEmpty(loc)
		item.GoodsCode = valueOrEmpty(gc)
		item.Unit = valueOrEmpty(un)
		item.StockState = valueOrEmpty(st)
		item.LabelAlias = valueOrEmpty(al)
		item.BatchNumber = valueOrEmpty(bt)
		item.ExpiryDate = dateOrEmpty(exp)
		item.InboundBatchID = valueOrEmpty(ib)
		item.CartonNumber = valueOrEmpty(carton)
		item.Note = valueOrEmpty(note)
		item.ApprovedBy = valueOrEmpty(appBy)
		item.ApprovalNote = valueOrEmpty(appNote)
		item.RejectedBy = valueOrEmpty(rejBy)
		item.RejectReason = valueOrEmpty(rejR)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.JobDiscrepancyRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: item.DiscrepancyID, Identity: []string{item.DiscrepancyID}}
	})
}

const wsInstructionColumns = `
	instruction_id, owner_account_id, warehouse_code, kind, status, origin,
	goods_code, unit_name, quantity, done_quantity, from_alias, to_alias,
	batch_number, expiry_date, reason, basis_reference, operator_id,
	fixed_revision, void_reason, voided_at, received_at`

const instructionListed = `WITH listed AS (
	SELECT ` + wsInstructionColumns + `
	  FROM warehouse_fulfillment.instruction
	 WHERE tenant_id = $1
)`

var instructionBinding = ports.InstructionPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"receivedAt":    "received_at",
		"instructionId": "instruction_id",
	},
	Identity: []string{"instruction_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"kind":          "kind",
		"status":        "status",
	},
	Keyword: []string{"instruction_id", "owner_account_id", "warehouse_code", "goods_code", "to_alias", "reason"},
})

func (store *WorkspaceReads) ListInstructions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.InstructionRow], error) {
	const op = "list instructions"
	fail := func(err error) (cataloguepage.Result[ports.InstructionRow], error) {
		return cataloguepage.Result[ports.InstructionRow]{}, fmt.Errorf("%s: %w", op, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := instructionBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, instructionListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, instructionListed+" "+plan.PageSQL(wsInstructionColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.InstructionRow
	for rows.Next() {
		var (
			item                                                ports.InstructionRow
			unit, fromA, toA, batch, reason, basis, opID, voidR *string
			exp                                                 *time.Time
		)
		if err := rows.Scan(
			&item.InstructionID, &item.OwnerAccount, &item.WarehouseCode, &item.Kind, &item.Status, &item.Origin,
			&item.GoodsCode, &unit, &item.Quantity, &item.DoneQuantity, &fromA, &toA,
			&batch, &exp, &reason, &basis, &opID, &item.FixedRevision, &voidR, &item.VoidedAt, &item.ReceivedAt,
		); err != nil {
			return fail(err)
		}
		item.Unit = valueOrEmpty(unit)
		item.FromAlias = valueOrEmpty(fromA)
		item.ToAlias = valueOrEmpty(toA)
		item.BatchNumber = valueOrEmpty(batch)
		item.ExpiryDate = dateOrEmpty(exp)
		item.Reason = valueOrEmpty(reason)
		item.Basis = valueOrEmpty(basis)
		item.Operator = valueOrEmpty(opID)
		item.VoidReason = valueOrEmpty(voidR)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for i := range items {
		cRows, err := querier.Query(ctx,
			`SELECT goods_code, quantity FROM warehouse_fulfillment.instruction_component
			  WHERE tenant_id = $1 AND instruction_id = $2 ORDER BY position`,
			tenant.String(), items[i].InstructionID,
		)
		if err != nil {
			return fail(err)
		}
		for cRows.Next() {
			var comp ports.InstructionComponentDetail
			if err := cRows.Scan(&comp.GoodsCode, &comp.Quantity); err != nil {
				cRows.Close()
				return fail(err)
			}
			items[i].Components = append(items[i].Components, comp)
		}
		cRows.Close()

		eRows, err := querier.Query(ctx,
			`SELECT fact_id, device_id, operator_id, occurred_at, device_sequence, done_quantity, movement_id
			   FROM warehouse_fulfillment.instruction_execution
			  WHERE tenant_id = $1 AND instruction_id = $2 ORDER BY occurred_at DESC, fact_id`,
			tenant.String(), items[i].InstructionID,
		)
		if err != nil {
			return fail(err)
		}
		for eRows.Next() {
			var ex ports.InstructionExecutionDetail
			if err := eRows.Scan(&ex.FactID, &ex.DeviceID, &ex.OperatorID, &ex.OccurredAt, &ex.DeviceSequence, &ex.DoneQuantity, &ex.MovementID); err != nil {
				eRows.Close()
				return fail(err)
			}
			items[i].Executions = append(items[i].Executions, ex)
		}
		eRows.Close()
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.InstructionRow, sortField string) cataloguepage.Position {
		if sortField == "instructionId" {
			return cataloguepage.Position{Value: item.InstructionID, Identity: []string{item.InstructionID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReceivedAt), Identity: []string{item.InstructionID}}
	})
}

// ReturnsOverview 读取退货预报（含明细行）与退件到仓记录（含回库处置状态）。
func (store *WorkspaceReads) ReturnsOverview(ctx context.Context, tenant domain.TenantID) (ports.ReturnsOverview, error) {
	const op = "returns overview"
	if tenant.String() == "" {
		return ports.ReturnsOverview{}, fmt.Errorf("%s: 缺租户", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnsOverview{}, fmt.Errorf("%s: %w", op, err)
	}
	out := ports.ReturnsOverview{}

	// 退货预报
	{
		rows, err := querier.Query(ctx,
			`SELECT advice_id, document_number, numbering_absence, owner_account_id, warehouse_code,
			        origin, tracking_number, basis, operator_id, received_at
			   FROM warehouse_fulfillment.return_pre_advice
			  WHERE tenant_id = $1
			  ORDER BY received_at DESC, advice_id`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: pre-advices: %w", op, err)
		}
		for rows.Next() {
			var (
				row                   ports.ReturnPreAdviceRow
				num, abs, basis, opID *string
			)
			if err := rows.Scan(&row.AdviceID, &num, &abs, &row.OwnerAccount, &row.WarehouseCode, &row.Origin, &row.TrackingNumber, &basis, &opID, &row.ReceivedAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: pre-advices: %w", op, err)
			}
			row.DocumentNumber = valueOrEmpty(num)
			row.NumberingAbsence = valueOrEmpty(abs)
			row.Basis = valueOrEmpty(basis)
			row.Operator = valueOrEmpty(opID)
			out.PreAdvices = append(out.PreAdvices, row)
		}
		rows.Close()

		for i := range out.PreAdvices {
			lRows, err := querier.Query(ctx,
				`SELECT goods_code, unit_name, quantity
				   FROM warehouse_fulfillment.return_pre_advice_line
				  WHERE tenant_id = $1 AND advice_id = $2 ORDER BY line_index`,
				tenant.String(), out.PreAdvices[i].AdviceID,
			)
			if err != nil {
				return out, fmt.Errorf("%s: pre-advice lines: %w", op, err)
			}
			for lRows.Next() {
				var line ports.ReturnPreAdviceLineRow
				if err := lRows.Scan(&line.GoodsCode, &line.Unit, &line.Quantity); err != nil {
					lRows.Close()
					return out, fmt.Errorf("%s: pre-advice lines: %w", op, err)
				}
				out.PreAdvices[i].Lines = append(out.PreAdvices[i].Lines, line)
			}
			lRows.Close()
		}
	}

	// 退件到仓及回库处置
	{
		rows, err := querier.Query(ctx,
			`SELECT r.fact_id, r.warehouse_code, r.location_code, r.tracking_number, r.owner_account_id,
			        r.advice_id, r.arrival_id, r.goods_code, r.unit_name, r.stock_state, r.quantity, r.outcome, r.occurred_at,
			        d.decision_id, d.destination, d.basis, d.operator_id, d.origin, d.decided_at
			   FROM warehouse_fulfillment.return_receipt r
			   LEFT JOIN LATERAL (
			     SELECT decision_id, destination, basis, operator_id, origin, decided_at
			       FROM warehouse_fulfillment.return_disposition
			      WHERE tenant_id = r.tenant_id AND fact_id = r.fact_id
			      ORDER BY decided_at DESC LIMIT 1
			   ) d ON true
			  WHERE r.tenant_id = $1
			  ORDER BY r.occurred_at DESC, r.fact_id`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: receipts: %w", op, err)
		}
		for rows.Next() {
			var (
				row                                            ports.ReturnReceiptRow
				ow, adv, arr, decID, dest, basis, opID, origin *string
				decidedAt                                      *time.Time
			)
			if err := rows.Scan(
				&row.FactID, &row.WarehouseCode, &row.LocationCode, &row.TrackingNumber, &ow, &adv, &arr,
				&row.GoodsCode, &row.Unit, &row.StockState, &row.Quantity, &row.Outcome, &row.OccurredAt,
				&decID, &dest, &basis, &opID, &origin, &decidedAt,
			); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: receipts: %w", op, err)
			}
			row.OwnerAccount = valueOrEmpty(ow)
			row.AdviceID = valueOrEmpty(adv)
			row.ArrivalID = valueOrEmpty(arr)
			if decID != nil && *decID != "" {
				disp := &ports.ReturnDispositionRow{
					DecisionID:  *decID,
					Destination: valueOrEmpty(dest),
					Basis:       valueOrEmpty(basis),
					Operator:    valueOrEmpty(opID),
					Origin:      valueOrEmpty(origin),
				}
				if decidedAt != nil {
					disp.DecidedAt = *decidedAt
				}
				row.Disposition = disp
			}
			out.Receipts = append(out.Receipts, row)
		}
		rows.Close()
	}

	return out, nil
}

// AgentOverview 读取代理仓连接器、库存声明与回执。声明新鲜度与 AgentHandler.ReadStatement 同一套，见 statementFreshness。
func (store *WorkspaceReads) AgentOverview(ctx context.Context, tenant domain.TenantID, now time.Time) (ports.AgentOverview, error) {
	const op = "agent overview"
	if tenant.String() == "" {
		return ports.AgentOverview{}, fmt.Errorf("%s: 缺租户", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AgentOverview{}, fmt.Errorf("%s: %w", op, err)
	}
	out := ports.AgentOverview{}

	// 连接器
	{
		rows, err := querier.Query(ctx,
			`SELECT warehouse_code, registered_at FROM warehouse_fulfillment.agent_connector
			  WHERE tenant_id = $1 ORDER BY warehouse_code`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: connectors: %w", op, err)
		}
		for rows.Next() {
			var item ports.AgentConnectorOverview
			if err := rows.Scan(&item.WarehouseCode, &item.RegisteredAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: connectors: %w", op, err)
			}
			out.Connectors = append(out.Connectors, item)
		}
		rows.Close()
	}

	// 库存声明
	{
		rows, err := querier.Query(ctx,
			`SELECT s.source_id, s.stated_at, s.valid_until, s.warehouse_code, s.owner_account_id, s.goods_code, s.unit_name, s.quantity,
			        EXISTS (
			          SELECT 1 FROM warehouse_fulfillment.agent_statement later
			           WHERE later.tenant_id = s.tenant_id AND later.source_id = s.source_id
			             AND later.warehouse_code = s.warehouse_code AND later.owner_account_id = s.owner_account_id
			             AND later.goods_code = s.goods_code AND later.stated_at > s.stated_at
			        )
			   FROM warehouse_fulfillment.agent_statement s
			  WHERE s.tenant_id = $1
			  ORDER BY s.stated_at DESC, s.source_id`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: statements: %w", op, err)
		}
		for rows.Next() {
			var item ports.AgentStatementRow
			var superseded bool
			if err := rows.Scan(&item.SourceID, &item.StatedAt, &item.ValidUntil, &item.WarehouseCode, &item.OwnerAccount, &item.GoodsCode, &item.Unit, &item.Quantity, &superseded); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: statements: %w", op, err)
			}
			item.Freshness = statementFreshness(now, item.ValidUntil, superseded)
			out.Statements = append(out.Statements, item)
		}
		rows.Close()
	}

	// 代理仓回执结果
	{
		rows, err := querier.Query(ctx,
			`SELECT result_id, request_id, warehouse_code, kind,
			        weight_value, weight_unit, length_value, width_value, height_value, dimension_unit, reported_at
			   FROM warehouse_fulfillment.agent_result
			  WHERE tenant_id = $1
			  ORDER BY reported_at DESC, result_id`,
			tenant.String(),
		)
		if err != nil {
			return out, fmt.Errorf("%s: results: %w", op, err)
		}
		for rows.Next() {
			var (
				item                    ports.AgentResultRow
				wv, wu, lv, wdv, hv, du *string
			)
			if err := rows.Scan(&item.ResultID, &item.RequestID, &item.WarehouseCode, &item.Kind, &wv, &wu, &lv, &wdv, &hv, &du, &item.ReportedAt); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s: results: %w", op, err)
			}
			item.WeightValue = valueOrEmpty(wv)
			item.WeightUnit = valueOrEmpty(wu)
			item.LengthValue = valueOrEmpty(lv)
			item.WidthValue = valueOrEmpty(wdv)
			item.HeightValue = valueOrEmpty(hv)
			item.DimensionUnit = valueOrEmpty(du)
			out.Results = append(out.Results, item)
		}
		rows.Close()
	}

	return out, nil
}

// statementFreshness 与 AgentHandler.ReadStatement 同一套。被取代只看同一来源、仓、货主、商品且 stated_at 更晚；否则当前时刻不早于 validUntil 就是 UNKNOWN。
func statementFreshness(now, validUntil time.Time, superseded bool) string {
	if superseded {
		return "SUPERSEDED"
	}
	if !now.Before(validUntil) {
		return "UNKNOWN"
	}
	return "VALID"
}

// partialShipmentDetail 把一张请求三层各自写的部分发货规则读出来，按 domain.EffectivePartialShipment 定此刻适用哪一层。
func partialShipmentDetail(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, request ports.OutboundRequestRow) (ports.PartialShipmentDetail, error) {
	detail := ports.PartialShipmentDetail{Order: request.PartialShipment}
	var consentAt time.Time
	err := querier.QueryRow(ctx,
		`SELECT partial_shipment, basis, operator_id, recorded_at
		   FROM warehouse_fulfillment.outbound_partial_shipment_consent
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY recorded_at DESC, basis DESC
		  LIMIT 1`,
		tenant.String(), request.RequestID).Scan(&detail.Consent, &detail.ConsentBasis, &detail.ConsentOperator, &consentAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		detail.ConsentAt = &consentAt
	}
	var owner *string
	err = querier.QueryRow(ctx,
		`SELECT partial_shipment
		   FROM warehouse_fulfillment.shortage_policy
		  WHERE tenant_id = $1 AND owner_account_id = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), request.OwnerAccount).Scan(&owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return detail, err
	}
	detail.Owner = valueOrEmpty(owner)
	parse := func(word string) (domain.PartialShipment, error) {
		if word == "" {
			return domain.PartialShipmentUnset, nil
		}
		return domain.NewPartialShipment(word)
	}
	consentRule, err := parse(detail.Consent)
	if err != nil {
		return detail, err
	}
	orderRule, err := parse(detail.Order)
	if err != nil {
		return detail, err
	}
	ownerRule, err := parse(detail.Owner)
	if err != nil {
		return detail, err
	}
	rule, source := domain.EffectivePartialShipment(consentRule, orderRule, ownerRule)
	detail.Rule, detail.Source = rule.String(), source.String()
	return detail, nil
}
