package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// EvaluationCatalogue 实现计价评价登记册的列表读端口（票 admin-skeleton-closure-batch/03，
// 形状照 ADR-0077）。与 OperationsCatalogue 分立成型：那边上列的是主数据目录（价卡、
// 参考序列），这边上列的是业务事实登记册——评价由评价编排写入，目录侧没有登记 CLI，
// 两类册子的写入方、种子纪律（业务事实不造种子）都不同，读面各自成器不互相搭车。
//
// 只读：上列不重建领域对象、不触发判断。权威内容在 snapshot，本口不碰快照列——
// 快照读回必须经领域整图重验含摘要自校，那是 Evaluations.FindByID 的装载纪律，
// 详情读法届时按它另立。租户条件进语句（ADR-0003：作用域是身份的一部分）。
type EvaluationCatalogue struct {
	db *bentopg.DB
}

func NewEvaluationCatalogue(db *bentopg.DB) (*EvaluationCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &EvaluationCatalogue{db: db}, nil
}

var _ ports.EvaluationCatalogueRead = (*EvaluationCatalogue)(nil)

// evaluationPages 把 ports.EvaluationPages 落到评价表的列上。评价的主键只有评价标识（0001），行标识就是它这一列。
var evaluationPages = ports.EvaluationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"evaluation_id"},
	Filters:  map[string]string{"status": "status"},
	Keyword:  []string{"evaluation_id", "semantic_digest", "plan_content_digest"},
})

// ListEvaluations 上列评价登记册的检索列面，按查询的筛选、q 与排序取游标之后的一页；缺省是登记时间倒序、同刻按评价
// 标识倒序。空册如实答空页（ADR-0077 Decision 四）；limit 非正拒（Decision 五，判据与 catalogueLimit 同款同源）。
func (catalogue *EvaluationCatalogue) ListEvaluations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.EvaluationCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.EvaluationCatalogueRow], error) {
		return cataloguepage.Result[ports.EvaluationCatalogueRow]{}, fmt.Errorf("list evaluations: %w", err)
	}
	if err := catalogueLimit("list evaluations", limit); err != nil {
		return cataloguepage.Result[ports.EvaluationCatalogueRow]{}, err
	}
	plan, err := evaluationPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "parcel_pricing.evaluation"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`evaluation_id, status, semantic_digest, plan_content_digest, canonicalization, recorded_at`, source,
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.EvaluationCatalogueRow, 0, limit+1)
	for rows.Next() {
		var row ports.EvaluationCatalogueRow
		if err := rows.Scan(
			&row.EvaluationID, &row.Status, &row.SemanticDigest,
			&row.PlanContentDigest, &row.Canonicalization, &row.RecordedAt,
		); err != nil {
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}

	page, err := cataloguepage.Finish(query, fetched, limit, total, evaluationPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// evaluationPosition 的登记时间取自扫描出的 recorded_at 列，与 ORDER BY 比的是同一个值（cataloguepage.Finish 头注）。
func evaluationPosition(row ports.EvaluationCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(row.RecordedAt),
		Identity: []string{row.EvaluationID},
	}
}
