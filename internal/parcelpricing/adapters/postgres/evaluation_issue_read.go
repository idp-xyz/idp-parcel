package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PendingSeriesEvaluations 实现 ports.PendingSeriesEvaluationRead（ADR-0105 Decision 五）。只读子表与父表的
// 列，不读快照；另立一个类型而不给 EvaluationCatalogue 加方法——伴生读端口的立意就是不让一个接口随新问法
// 变宽。
type PendingSeriesEvaluations struct {
	db *bentopg.DB
}

func NewPendingSeriesEvaluations(db *bentopg.DB) (*PendingSeriesEvaluations, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &PendingSeriesEvaluations{db: db}, nil
}

var _ ports.PendingSeriesEvaluationRead = (*PendingSeriesEvaluations)(nil)

// CountPendingSeriesEvaluations 按序列种类数待判断的评价。数的是评价不是问题项（COUNT(DISTINCT evaluation_id)）；
// 两个原因码在 SQL 里以字面量钉住，与领域两处产出点同一组词——这里不解释别的原因码。
func (read *PendingSeriesEvaluations) CountPendingSeriesEvaluations(
	ctx context.Context,
	tenant domain.TenantID,
) ([]ports.PendingSeriesEvaluationCount, error) {
	if tenant.String() == "" {
		return nil, fmt.Errorf("count pending series evaluations: tenant is required")
	}
	querier, err := read.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("count pending series evaluations: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT i.series_kind, COUNT(DISTINCT i.evaluation_id)
		   FROM parcel_pricing.evaluation_issue i
		   JOIN parcel_pricing.evaluation e ON e.evaluation_id = i.evaluation_id
		  WHERE e.tenant_id = $1
		    AND e.status = $2
		    AND i.code IN ('REFERENCE_SERIES_UNRESOLVED', 'EXCHANGE_RATE_UNRESOLVED')
		    AND i.series_kind IS NOT NULL
		  GROUP BY i.series_kind
		  ORDER BY i.series_kind`,
		tenant.String(), string(domain.EvaluationPending),
	)
	if err != nil {
		return nil, fmt.Errorf("count pending series evaluations: %w", err)
	}
	defer rows.Close()

	counts := make([]ports.PendingSeriesEvaluationCount, 0)
	for rows.Next() {
		var count ports.PendingSeriesEvaluationCount
		if err := rows.Scan(&count.Kind, &count.EvaluationCount); err != nil {
			return nil, fmt.Errorf("count pending series evaluations: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count pending series evaluations: %w", err)
	}
	return counts, nil
}
