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

// ListReferenceSeries 上列参考序列版本的检索列面,连同两组不在列面上的事实(票
// pricing-reference-series-operations/08 件①):
//
//   - 期次与登记时声明的引用 digest 只在快照里。文件头那句「不读快照」在这里收窄为**不绕过
//     领域重建门读快照**:每一版经 rehydrateRegisteredSeries(整版重验 + 比对列交叉核)读回
//     再转写,与 LoadVersion 同一道门;不在 SQL 里展开 JSON。代价是每版一次重建,而目录有
//     limit 封顶。
//   - 复核事实从复核册连过来按版本折叠。版本与复核是一对多,limit 因此套在版本上(CTE 先
//     挑版本再连复核),不然一版多条复核会把一页撑满、把别的版本挤出去。**最近一次复核**取
//     时刻最大者,同刻按 reviewer 由 ORDER BY 定序取扫到的最后一条——同参数两次查询答同一条。
//
// 这里没有「在用」,也不做任何排序裁决:在用是相对评价形成时刻的结论,规则只在
// domain.SelectInForceSeriesVersion 一处,目录页没有那个时刻(票 04 的 owner 裁决)。
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
		`WITH picked AS (
		     SELECT series_id, series_version, kind, source_identifier, registrant,
		            quote_basis_id, quote_basis_version, effective_from, effective_to,
		            evidence_grade, prior_version, correction_basis,
		            canonicalization, content_digest, snapshot, registered_at
		       FROM parcel_pricing.reference_series_version
		      WHERE tenant_id = $1
		      ORDER BY registered_at DESC, series_id, series_version
		      LIMIT $2
		 )
		 SELECT p.series_id, p.series_version, p.kind, p.source_identifier, p.registrant,
		        p.quote_basis_id, p.quote_basis_version, p.effective_from, p.effective_to,
		        p.evidence_grade, p.prior_version, p.correction_basis,
		        p.canonicalization, p.content_digest, p.snapshot, p.registered_at,
		        r.reviewed_at, r.decision
		   FROM picked p
		   LEFT JOIN parcel_pricing.reference_series_review r
		     ON r.tenant_id = $1
		    AND r.series_id = p.series_id
		    AND r.series_version = p.series_version
		  ORDER BY p.registered_at DESC, p.series_id, p.series_version, r.reviewed_at, r.reviewer`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list reference series: %w", err)
	}
	defer rows.Close()

	series := make([]ports.ReferenceSeriesCatalogueRow, 0, limit)
	// 连接后一版会出现多行(每条复核一行),按(标识、版本)折叠;版本列面只在首行转写一次。
	index := make(map[[2]string]int)
	for rows.Next() {
		var row ports.ReferenceSeriesCatalogueRow
		var quoteBasisID, quoteBasisVersion *string
		var effectiveTo *time.Time
		var priorVersion, correctionBasis *string
		var snapshot []byte
		var reviewedAt *time.Time
		var decision *string
		if err := rows.Scan(
			&row.SeriesID, &row.SeriesVersion, &row.Kind, &row.SourceIdentifier, &row.Registrant,
			&quoteBasisID, &quoteBasisVersion, &row.EffectiveFrom, &effectiveTo,
			&row.EvidenceGrade, &priorVersion, &correctionBasis,
			&row.Canonicalization, &row.ContentDigest, &snapshot, &row.RegisteredAt,
			&reviewedAt, &decision,
		); err != nil {
			return nil, fmt.Errorf("list reference series: %w", err)
		}

		key := [2]string{row.SeriesID, row.SeriesVersion}
		position, seen := index[key]
		if !seen {
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
			registration, err := rehydrateRegisteredSeries(snapshot, registeredSeriesColumns{
				tenant: tenant, seriesID: row.SeriesID, seriesVersion: row.SeriesVersion,
				kind: row.Kind, grade: row.EvidenceGrade, canonicalization: row.Canonicalization, digest: row.ContentDigest,
			})
			if err != nil {
				return nil, fmt.Errorf("list reference series: %w", err)
			}
			row.ReferenceDigest = registration.Reference().Digest()
			row.Periods = transcribeSeriesPeriods(registration)
			position = len(series)
			index[key] = position
			series = append(series, row)
		}
		if reviewedAt == nil || decision == nil {
			continue
		}
		listed := &series[position]
		listed.ReviewCount++
		if *decision == domain.SeriesReviewApproved.String() {
			listed.ApprovedReviewCount++
		}
		if !listed.HasReview || !reviewedAt.Before(listed.LastReviewedAt) {
			listed.LastReviewedAt = *reviewedAt
			listed.LastReviewDecision = *decision
			listed.HasReview = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reference series: %w", err)
	}
	return series, nil
}

// transcribeSeriesPeriods 把重建后的期次表逐期照实转写:无上界与缺凭证各以布尔说出「没有」,
// 取值取规范十进制文本——它是读面,不折算不舍入。
func transcribeSeriesPeriods(registration domain.ReferenceSeriesRegistration) []ports.ReferenceSeriesPeriodRow {
	periods := registration.Periods()
	transcribed := make([]ports.ReferenceSeriesPeriodRow, 0, len(periods))
	for _, period := range periods {
		row := ports.ReferenceSeriesPeriodRow{
			StartsAt: period.StartsAt(),
			Value:    period.Value().String(),
		}
		if endsAt, bounded := period.EndsAt(); bounded {
			row.EndsAt = endsAt
			row.HasEndsAt = true
		}
		if evidence, verifiable := period.Evidence(); verifiable {
			row.EvidenceRef = evidence
			row.HasEvidence = true
		}
		transcribed = append(transcribed, row)
	}
	return transcribed
}
