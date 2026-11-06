package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// ReviewCatalogue 实现 ports.ReviewCatalogueRead：节点作业查阅页的列表读面
// （ADR-0077，票 admin-skeleton-closure-batch/05）。
//
// 目录上列是照实转写，不经领域重建门：重建是写路与按键读回的纪律（坏行要在那里
// 响亮），检索列面把登记的字段原样透出。jsonb 字段在 SQL 里就地展开成列
// （intake/control 的键形状由本包写侧的 intakeRow/controlRow 固定），时刻经
// ::timestamptz 还原——写侧以 RFC 3339 序列化，两侧共一份形状，不在 Go 里二次解码。
// 例外在 ListConsolidationMembers：方向依据记录经领域重建门读回，理由在 ports.ReviewCatalogueRead 所在文件的头注。
//
// 各册按 ports 上同名声明取页（ADR-0144）：落列在各册的 *Pages 变量里，总数与本页出自同一组条件；游标位置取自本页扫描回来的
// 排序列与标识列，不取 jsonb 里的副本。
//
// 所有语句显式携带租户条件（ADR-0003）；读走 ReadExecutor，事务外用显式注入的
// 连接池。
type ReviewCatalogue struct {
	db *bentopg.DB
}

var _ ports.ReviewCatalogueRead = (*ReviewCatalogue)(nil)

func NewReviewCatalogue(db *bentopg.DB) (*ReviewCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ReviewCatalogue{db: db}, nil
}

const receptionRegister = "node_operations.reception"

// receptionPages 把 ports.ReceptionPages 落到收寄登记的列上；q 在来源标识与收寄里的作业实物、节点、交付方上做字面包含。
var receptionPages = ports.ReceptionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"source_id"},
	Filters:  map[string]string{"kind": "kind"},
	Keyword:  []string{"source_id", "intake ->> 'unit'", "intake ->> 'node'", "intake ->> 'deliveredBy'"},
})

// listedReceptions 是收寄册的定义：只列带收寄与控制在场的两格（kind 封闭集见端口注释）。部分索引 reception_catalogue_by_recorded
// 的谓词与它的种类条件逐字同形，规划器才认得出这一读可以走它。
const listedReceptions = `tenant_id = $1 AND kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION')`

// ListReceptions 上列收寄登记册的一页：缺省新近登记在前，同刻按来源标识倒序。
func (catalogue *ReviewCatalogue) ListReceptions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ReceptionCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ReceptionCatalogueRow], error) {
		return cataloguepage.Result[ports.ReceptionCatalogueRow]{}, fmt.Errorf("list receptions: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := receptionPages.Plan(query, listedReceptions, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(receptionRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	rows, err := querier.Query(ctx, plan.PageSQL(
		`source_id, kind,
		 intake->>'unit',
		 intake->>'node',
		 intake->>'deliveredBy',
		 (intake->>'receivedAt')::timestamptz,
		 control->>'kind',
		 (control->>'establishedAt')::timestamptz,
		 COALESCE(`+foldedControl+`->>'releasedBy', ''),
		 (`+foldedControl+`->>'releasedAt')::timestamptz,
		 service_markers,
		 service_result_check,
		 recorded_at,
		 identity_check,
		 delivery_scope_check, delivery_scope_unverified_reason, delivery_scope_inapplicability`,
		receptionRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.ReceptionCatalogueRow, 0, limit+1)
	for rows.Next() {
		var (
			row         ports.ReceptionCatalogueRow
			releasedAt  *time.Time
			markersJSON []byte
			checkName   *string
			identity    *string
			scope       [3]*string
		)
		if err := rows.Scan(
			&row.SourceID, &row.Kind,
			&row.Unit, &row.Node, &row.DeliveredBy, &row.ReceivedAt,
			&row.ControlKind, &row.ControlEstablishedAt,
			&row.ControlReleasedBy, &releasedAt,
			&markersJSON, &checkName, &row.RecordedAt, &identity,
			&scope[0], &scope[1], &scope[2],
		); err != nil {
			return fail(err)
		}
		if releasedAt != nil {
			utc := releasedAt.UTC()
			row.ControlReleasedAt = &utc
		}
		if err := json.Unmarshal(markersJSON, &row.ServiceMarkers); err != nil {
			return fail(err)
		}
		// 核对态与按键读回共用 serviceResultCheckFrom：词表只认一份，集外取值让整页答不出，不悄悄折成「不带」——
		// 形成行缺了这一格，审阅者又分不出标记的空是查过了还是没读到。
		check, err := serviceResultCheckFrom(checkName)
		if err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		row.ServiceResultCheck = check
		// 身份核对态同理，与按键读回共用 identityCheckFrom；交付范围核对态与按键读回共用 deliveryScopeCheckFrom。
		if row.IdentityCheck, err = identityCheckFrom(identity); err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		if row.DeliveryScopeCheck, err = deliveryScopeCheckFrom(scope[0], scope[1], scope[2]); err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		row.ReceivedAt = row.ReceivedAt.UTC()
		row.ControlEstablishedAt = row.ControlEstablishedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	// 登记时刻与来源标识都是从 recorded_at、source_id 两列原样扫回的值，与 ORDER BY 用的是同一份。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.ReceptionCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.RecordedAt), Identity: []string{row.SourceID}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// unidentifiedItemPages 把 ports.UnidentifiedItemPages 落到收寄登记（别名 r）的列上；q 在来源标识、作业实物、节点与来源文档里的
// 外部标识观察上做字面包含（来源未保全的行没有那一格，只靠另几列命中）。
var unidentifiedItemPages = ports.UnidentifiedItemPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "r.recorded_at"},
	Identity: []string{"r.source_id"},
	Filters:  map[string]string{"identityCheck": "r.identity_check"},
	Keyword:  []string{"r.source_id", "r.intake ->> 'unit'", "r.intake ->> 'node'", "r.source_document ->> 'mark'"},
})

// pendingIdentification 是待识别册的定义；种类条件与部分索引 reception_pending_catalogue_by_recorded 的谓词逐字同形。
const pendingIdentification = `r.tenant_id = $1 AND r.kind = 'PENDING_IDENTIFICATION'`

// ListUnidentifiedItems 上列待识别实物册的一页：收寄登记里 PENDING_IDENTIFICATION 那一格的身份视角，缺省新近登记在前、同刻按来源
// 标识倒序。来源文档里的号码照行转写（来源文档为空即来源未保全）；直接接续这一版的那一版按（租户，前版）取，走
// reception_predecessor_unique。来源文档里的交付范围依据两键与实物最后归入的交接单经 unitScopeBasisFrom 还原；后者与交接单册
// FindLastAssignedHandoverNote 同一问法（只看归入，业务时间倒序、同刻按落册先后），走同一本部分索引。
func (catalogue *ReviewCatalogue) ListUnidentifiedItems(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.UnidentifiedItemCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.UnidentifiedItemCatalogueRow], error) {
		return cataloguepage.Result[ports.UnidentifiedItemCatalogueRow]{}, fmt.Errorf("list unidentified items: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := unidentifiedItemPages.Plan(query, pendingIdentification, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	const source = receptionRegister + " AS r"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	rows, err := querier.Query(ctx, plan.PageSQL(
		`r.source_id,
		 r.intake->>'unit',
		 r.intake->>'node',
		 r.candidates,
		 r.identity_conflict,
		 (r.intake->>'receivedAt')::timestamptz,
		 COALESCE(r.intake->>'association', ''),
		 r.recorded_at,
		 COALESCE(r.judgment_version, ''),
		 r.source_document IS NOT NULL,
		 COALESCE(r.source_document->>'mark', ''),
		 COALESCE((SELECT successor.judgment_version
		             FROM node_operations.reception AS successor
		            WHERE successor.tenant_id = r.tenant_id
		              AND successor.predecessor_version = r.judgment_version), ''),
		 r.identity_check,
		 r.identity_conflict_cause,
		 r.delivery_scope_check, r.delivery_scope_unverified_reason, r.delivery_scope_inapplicability,
		 COALESCE(r.source_document->>'scope_basis_kind', ''),
		 COALESCE(r.source_document->>'scope_basis_id', ''),
		 COALESCE((SELECT member.note_id
		             FROM node_operations.delivery_handover_member AS member
		            WHERE member.tenant_id = r.tenant_id
		              AND member.unit_id = r.intake->>'unit'
		              AND member.membership_kind = 'ASSIGNMENT'
		            ORDER BY member.formed_at DESC, member.member_sequence DESC
		            LIMIT 1), '')`,
		source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.UnidentifiedItemCatalogueRow, 0, limit+1)
	for rows.Next() {
		var (
			row            ports.UnidentifiedItemCatalogueRow
			candidatesJSON []byte
			identity       *string
			conflictCause  *string
			scope          [3]*string
			basis          [3]string
		)
		if err := rows.Scan(
			&row.SourceID, &row.Unit, &row.Node,
			&candidatesJSON, &row.IdentityConflict,
			&row.ReceivedAt, &row.Association, &row.RecordedAt,
			&row.JudgmentVersion, &row.SourcePreserved, &row.Mark, &row.SucceededBy,
			&identity, &conflictCause,
			&scope[0], &scope[1], &scope[2],
			&basis[0], &basis[1], &basis[2],
		); err != nil {
			return fail(err)
		}
		check, err := identityCheckFrom(identity)
		if err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		row.IdentityCheck = check
		if row.IdentityConflictCause, err = identityConflictCauseFrom(conflictCause); err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		if row.DeliveryScopeCheck, err = deliveryScopeCheckFrom(scope[0], scope[1], scope[2]); err != nil {
			return fail(fmt.Errorf("source %q: %w", row.SourceID, err))
		}
		if row.DeliveryScopeBasis, err = unitScopeBasisFrom(basis[0], basis[1], basis[2]); err != nil {
			return fail(fmt.Errorf("source %q: delivery scope basis: %w", row.SourceID, err))
		}
		if err := json.Unmarshal(candidatesJSON, &row.Candidates); err != nil {
			return fail(err)
		}
		row.ReceivedAt = row.ReceivedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.UnidentifiedItemCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.RecordedAt), Identity: []string{row.SourceID}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

const consolidationUnitRegister = "node_operations.consolidation_unit"

// consolidationUnitPages 把 ports.ConsolidationUnitPages 落到集运单元行的列上：标识既是唯一的可排维，也是行标识；q 在单元标识、
// 载具与最近一份快照的封签上做字面包含（尚未封装过的单元没有那一格，只靠另两列命中）。
var consolidationUnitPages = ports.ConsolidationUnitPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"unitId": "unit_id"},
	Identity: []string{"unit_id"},
	Filters:  map[string]string{"phase": "phase"},
	Keyword:  []string{"unit_id", "asset_ref", "snapshots -> -1 ->> 'seal'"},
})

// ListConsolidationUnits 上列集运单元登记册的一页。行上没有业务时刻可排序（形成时间无
// 登记格，库面簿记时刻不是业务事实），按实例标识稳定排序——同一册两次读回同一个序。
// 最近封签在 SQL 里取快照数组末元素：写侧按封装次序追加快照，末元素即最近一次。
func (catalogue *ReviewCatalogue) ListConsolidationUnits(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ConsolidationUnitCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ConsolidationUnitCatalogueRow], error) {
		return cataloguepage.Result[ports.ConsolidationUnitCatalogueRow]{}, fmt.Errorf("list consolidation units: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := consolidationUnitPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(consolidationUnitRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	rows, err := querier.Query(ctx, plan.PageSQL(
		`unit_id, asset_ref, phase,
		 jsonb_array_length(members),
		 jsonb_array_length(snapshots),
		 opened_source ->> 'sourceId',
		 opened_source ->> 'performedBy',
		 COALESCE(snapshots -> -1 ->> 'seal', ''),
		 COALESCE(snapshots -> -1 -> 'source' ->> 'sourceId', ''),
		 COALESCE(snapshots -> -1 -> 'source' ->> 'performedBy', ''),
		 (snapshots -> -1 ->> 'sealedAt')::timestamptz,
		 closed_at`,
		consolidationUnitRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.ConsolidationUnitCatalogueRow, 0, limit+1)
	for rows.Next() {
		var (
			row            ports.ConsolidationUnitCatalogueRow
			latestSealedAt *time.Time
			closedAt       *time.Time
		)
		if err := rows.Scan(
			&row.UnitID, &row.Asset, &row.Phase,
			&row.MemberCount, &row.SealCount,
			&row.OpenedSourceID, &row.OpenedBy,
			&row.LatestSeal, &row.LatestSealSourceID, &row.LatestSealPerformedBy,
			&latestSealedAt, &closedAt,
		); err != nil {
			return fail(err)
		}
		if latestSealedAt != nil {
			utc := latestSealedAt.UTC()
			row.LatestSealedAt = &utc
		}
		if closedAt != nil {
			utc := closedAt.UTC()
			row.ClosedAt = &utc
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.ConsolidationUnitCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.UnitID, Identity: []string{row.UnitID}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

const consolidationFactRegister = "node_operations.consolidation_fact"

// consolidationMemberPages 把 ports.ConsolidationMemberPages 落到集运作业事实登记的列上；q 在来源标识、单元与成员上做字面包含。
var consolidationMemberPages = ports.ConsolidationMemberPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"source_id"},
	Filters:  map[string]string{"action": "action"},
	Keyword:  []string{"source_id", "unit_id", "member_id"},
})

// listedConsolidationMembers 是成员关系册的定义：只列移入与移出两种来源事实；种类条件与部分索引
// consolidation_fact_membership_by_recorded 的谓词逐字同形。
const listedConsolidationMembers = `tenant_id = $1 AND action IN ('ADD_MEMBER', 'REMOVE_MEMBER')`

// ListConsolidationMembers 上列「集运成员关系」册的一页：移入与移出两种来源事实，缺省按落册时刻倒序（同刻按来源标识倒序）。
// 方向依据记录经领域重建门读回，与按键读回走同一份行模型；一行坏记录让整页答不出，而不是悄悄显示成依据齐全。
func (catalogue *ReviewCatalogue) ListConsolidationMembers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ConsolidationMemberCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ConsolidationMemberCatalogueRow], error) {
		return cataloguepage.Result[ports.ConsolidationMemberCatalogueRow]{}, fmt.Errorf("list consolidation members: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := consolidationMemberPages.Plan(query, listedConsolidationMembers, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(consolidationFactRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	rows, err := querier.Query(ctx, plan.PageSQL(
		`source_id, action, unit_id, member_id, performed_by, evidence,
		 occurred_at, recorded_at, directional_basis`,
		consolidationFactRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.ConsolidationMemberCatalogueRow, 0, limit+1)
	for rows.Next() {
		var (
			row   ports.ConsolidationMemberCatalogueRow
			basis []byte
		)
		if err := rows.Scan(
			&row.SourceID, &row.Action, &row.UnitID, &row.Member, &row.PerformedBy, &row.Evidence,
			&row.OccurredAt, &row.RecordedAt, &basis,
		); err != nil {
			return fail(err)
		}
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		if basis != nil {
			record, err := rebuildDirectionalBasis(basis)
			if err != nil {
				return fail(fmt.Errorf("source %q: directional basis: %w", row.SourceID, err))
			}
			row.DirectionalBasis = record
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.ConsolidationMemberCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.RecordedAt), Identity: []string{row.SourceID}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}
