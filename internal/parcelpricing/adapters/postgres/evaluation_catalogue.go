package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
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

// ListEvaluations 上列评价登记册的检索列面。排序以登记时间倒序、同刻按评价标识正序
// 收尾，保证分页可重复；空册如实答空列表（ADR-0077 Decision 四）；limit 非正拒
// （Decision 五，判据与 catalogueLimit 同款同源）。
func (catalogue *EvaluationCatalogue) ListEvaluations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.EvaluationCatalogueRow, error) {
	if err := catalogueLimit("list evaluations", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list evaluations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT evaluation_id, status, semantic_digest, plan_content_digest,
		        canonicalization, recorded_at
		   FROM parcel_pricing.evaluation
		  WHERE tenant_id = $1
		  ORDER BY recorded_at DESC, evaluation_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list evaluations: %w", err)
	}
	defer rows.Close()

	evaluations := make([]ports.EvaluationCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.EvaluationCatalogueRow
		if err := rows.Scan(
			&row.EvaluationID, &row.Status, &row.SemanticDigest,
			&row.PlanContentDigest, &row.Canonicalization, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list evaluations: %w", err)
		}
		evaluations = append(evaluations, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list evaluations: %w", err)
	}
	return evaluations, nil
}
