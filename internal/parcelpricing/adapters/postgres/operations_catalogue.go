package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// OperationsCatalogue 实现两个主数据目录查阅读端口(ADR-0077,票 master-data-wiring/02):
// 价卡目录与计价参考序列登记册。只读——目录上列不形成判断、不做评价、不择优,所以
// 这里只有 SELECT,没有任何 Register;登记仍走 PriceCards / ReferenceSeriesVersions
// 与 cmd/parcel-pricing-register 的受控通道。
//
// 上列检索列面,不读快照:权威内容在快照、读回须经领域整图重验,那是评价装载
// (LoadApplicable / ResolveAt)的纪律;目录照列转写,列与登记字段对齐,不发明列。
//
// 每条语句显式携带租户条件(ADR-0003:作用域是身份的一部分,不是过滤器)。排序以
// 登记时间倒序、同刻按标识与版本正序收尾,保证分页可重复。
type OperationsCatalogue struct {
	db *bentopg.DB
}

func NewOperationsCatalogue(db *bentopg.DB) (*OperationsCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &OperationsCatalogue{db: db}, nil
}

var _ ports.PriceCardCatalogueRead = (*OperationsCatalogue)(nil)
var _ ports.ReferenceSeriesCatalogueRead = (*OperationsCatalogue)(nil)

// catalogueLimit 把「忘了传页大小」挡在读口上:静默答一页会把缺参变成一个没人决定
// 过的页大小(ADR-0077 Decision 五,判据与运营追踪读口同款)。
func catalogueLimit(operation string, limit int) error {
	if limit < 1 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// ListPriceCards 上列价卡版本的检索列面。方向不设默认过滤:目录答「登了哪些卡」,
// 方向隔离是**评价装载**的谓词(BUY 的册面不进 SELL 的答案),上列两向并见不穿它
// ——运营查阅的授权边界只有租户,方向列照实透出供页面分栏。
func (catalogue *OperationsCatalogue) ListPriceCards(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PriceCardCatalogueRow, error) {
	if err := catalogueLimit("list price cards", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price cards: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT plan_id, plan_version, direction, purpose, scope,
		        rate_table_id, rate_table_version, effective_from, effective_to,
		        canonicalization, content_digest, source_file_name, source_file_sha256,
		        authorization_id, authorization_version, publication_approver, registered_at
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1
		  ORDER BY registered_at DESC, plan_id, plan_version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list price cards: %w", err)
	}
	defer rows.Close()

	cards := make([]ports.PriceCardCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.PriceCardCatalogueRow
		var effectiveTo *time.Time
		if err := rows.Scan(
			&row.PlanID, &row.PlanVersion, &row.Direction, &row.Purpose, &row.Scope,
			&row.RateTableID, &row.RateTableVersion, &row.EffectiveFrom, &effectiveTo,
			&row.Canonicalization, &row.ContentDigest, &row.SourceFileName, &row.SourceFileSHA256,
			&row.AuthorizationID, &row.AuthorizationVersion, &row.PublicationApprover, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list price cards: %w", err)
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		cards = append(cards, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list price cards: %w", err)
	}
	return cards, nil
}

// ListReferenceSeries 上列参考序列版本的检索列面。逐期取值在快照内,不上列——期次
// 的消费口是按计价基准时点的 ResolveAt,目录只答「登了哪些版本」。
func (catalogue *OperationsCatalogue) ListReferenceSeries(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ReferenceSeriesCatalogueRow, error) {
	if err := catalogueLimit("list reference series", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reference series: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT series_id, series_version, kind, source_identifier, registrant,
		        quote_basis_id, quote_basis_version, effective_from, effective_to,
		        evidence_grade, prior_version, correction_basis,
		        canonicalization, content_digest, registered_at
		   FROM parcel_pricing.reference_series_version
		  WHERE tenant_id = $1
		  ORDER BY registered_at DESC, series_id, series_version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list reference series: %w", err)
	}
	defer rows.Close()

	series := make([]ports.ReferenceSeriesCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.ReferenceSeriesCatalogueRow
		var quoteBasisID, quoteBasisVersion *string
		var effectiveTo *time.Time
		var priorVersion, correctionBasis *string
		if err := rows.Scan(
			&row.SeriesID, &row.SeriesVersion, &row.Kind, &row.SourceIdentifier, &row.Registrant,
			&quoteBasisID, &quoteBasisVersion, &row.EffectiveFrom, &effectiveTo,
			&row.EvidenceGrade, &priorVersion, &correctionBasis,
			&row.Canonicalization, &row.ContentDigest, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list reference series: %w", err)
		}
		// 口径两列成对、更正两件成对,库上 CHECK 钉住;这里按在场与否翻译,不补半边。
		if quoteBasisID != nil && quoteBasisVersion != nil {
			row.QuoteBasisID = *quoteBasisID
			row.QuoteBasisVersion = *quoteBasisVersion
			row.HasQuoteBasis = true
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		if priorVersion != nil && correctionBasis != nil {
			row.PriorVersion = *priorVersion
			row.CorrectionBasis = *correctionBasis
			row.IsCorrection = true
		}
		series = append(series, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reference series: %w", err)
	}
	return series, nil
}
