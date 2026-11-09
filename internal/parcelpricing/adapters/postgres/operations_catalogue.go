package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// OperationsCatalogue 实现两个主数据目录查阅读端口(ADR-0077,票 master-data-wiring/02):
// 价卡目录与计价参考序列登记册。只读——目录上列不形成判断、不做评价、不择优,所以
// 这里只有 SELECT,没有任何 Register;登记仍走 PriceCards / ReferenceSeriesVersions
// 与 cmd/parcel-pricing-register 的受控通道。
//
// 上列检索列面,不读快照:权威内容在快照、读回须经领域整图重验,那是评价装载
// (LoadApplicable / ResolveAt)的纪律;目录照列转写,列与登记字段对齐,不发明列。
//
// 每条语句显式携带租户条件(ADR-0003:作用域是身份的一部分,不是过滤器)。翻页、排序
// 与筛选照 ADR-0144 由查询对象说;缺省登记时间倒序、同刻按标识与版本倒序收尾。
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
//
// 收尾事实从截止册与退役册连过来(ADR-0234):截止取最新一条,与选卡读的是同一条;发布时的适用期
// 两格照列,不被截止点覆盖——目录要让人看得出「发布时登的是什么、后来被截到哪」两件事。两处连接
// 各至多一行(截止取最新一条、退役以版本为主键),不会把一版摊成几行,所以总数只数版本表。
func (catalogue *OperationsCatalogue) ListPriceCards(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PriceCardCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.PriceCardCatalogueRow], error) {
		return cataloguepage.Result[ports.PriceCardCatalogueRow]{}, fmt.Errorf("list price cards: %w", err)
	}
	if err := catalogueLimit("list price cards", limit); err != nil {
		return cataloguepage.Result[ports.PriceCardCatalogueRow]{}, err
	}
	plan, err := priceCardPages.Plan(query, "v.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL("parcel_pricing.price_card_version v"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`v.plan_id, v.plan_version, v.direction, v.purpose, v.scope,
		 v.rate_table_id, v.rate_table_version, v.effective_from, v.effective_to,
		 v.canonicalization, v.content_digest, v.source_file_name, v.source_file_sha256,
		 v.authorization_id, v.authorization_version, v.publication_approver, v.registered_at,
		 latest_cut.selection_ends_at, latest_cut.successor_version,
		 retirement.retired_at, retirement.successor_version`,
		`parcel_pricing.price_card_version v
		 LEFT JOIN LATERAL (
		      SELECT c.selection_ends_at, c.successor_version
		        FROM parcel_pricing.price_card_version_cut c
		       WHERE c.tenant_id = v.tenant_id AND c.plan_id = v.plan_id AND c.plan_version = v.plan_version
		       ORDER BY c.registration_id DESC
		       LIMIT 1) latest_cut ON true
		 LEFT JOIN parcel_pricing.price_card_version_retirement retirement
		        ON retirement.tenant_id = v.tenant_id AND retirement.plan_id = v.plan_id
		       AND retirement.plan_version = v.plan_version`,
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.PriceCardCatalogueRow, 0, limit+1)
	for rows.Next() {
		var row ports.PriceCardCatalogueRow
		var effectiveTo, cutEnd, retiredAt *time.Time
		var cutSuccessor, retirementSuccessor *string
		if err := rows.Scan(
			&row.PlanID, &row.PlanVersion, &row.Direction, &row.Purpose, &row.Scope,
			&row.RateTableID, &row.RateTableVersion, &row.EffectiveFrom, &effectiveTo,
			&row.Canonicalization, &row.ContentDigest, &row.SourceFileName, &row.SourceFileSHA256,
			&row.AuthorizationID, &row.AuthorizationVersion, &row.PublicationApprover, &row.RegisteredAt,
			&cutEnd, &cutSuccessor, &retiredAt, &retirementSuccessor,
		); err != nil {
			return fail(err)
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		if cutEnd != nil && cutSuccessor != nil {
			row.SelectionEndsAt, row.HasSelectionEnd, row.SuccessorVersion = *cutEnd, true, *cutSuccessor
		}
		if retiredAt != nil && retirementSuccessor != nil {
			row.RetiredAt, row.HasRetirement, row.SuccessorVersion = *retiredAt, true, *retirementSuccessor
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}

	page, err := cataloguepage.Finish(query, fetched, limit, total, priceCardPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// priceCardPages 把 ports.PriceCardPages 落到价卡版本表(别名 v)的列上。
//
// retired 落在「退役册上有这一版那一行」的文本形上('true' / 'false',与词表同词)。总数那一句只从版本表取,取页那一句左连的
// 退役行要等页切出来才在,所以这一维落成 EXISTS 而不是连接列;两处读的是同一行——退役以版本为主键,ListPriceCards 据它填
// HasRetirement。子查询走退役册主键 price_card_version_retirement_pkey,不另建索引。
var priceCardPages = ports.PriceCardPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"registeredAt": "v.registered_at", "planId": "v.plan_id"},
	Identity: []string{"v.plan_id", "v.plan_version"},
	Filters: map[string]string{
		"direction": "v.direction",
		"retired": `(EXISTS (
			SELECT 1 FROM parcel_pricing.price_card_version_retirement AS r
			 WHERE r.tenant_id = v.tenant_id AND r.plan_id = v.plan_id AND r.plan_version = v.plan_version))::text`,
	},
	Keyword: []string{"v.plan_id", "v.plan_version", "v.scope"},
})

// priceCardPosition 的登记时间取自扫描出的 registered_at 列,与 ORDER BY 比的是同一个值(cataloguepage.Finish 头注)。
func priceCardPosition(row ports.PriceCardCatalogueRow, sortField string) cataloguepage.Position {
	value := row.PlanID
	if sortField == "registeredAt" {
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.PlanID, row.PlanVersion}}
}

// ListReferenceSeries 上列参考序列版本的检索列面,连同两组不在列面上的事实(票
// pricing-reference-series-operations/08 件①):
//
//   - 期次与登记时声明的引用 digest 只在快照里。文件头那句「不读快照」在这里收窄为**不绕过
//     领域重建门读快照**:本页每一版经 rehydrateRegisteredSeries(整版重验 + 比对列交叉核)读回
//     再转写,与 LoadVersion 同一道门;不在 SQL 里展开 JSON。代价是每版一次重建,而一页有
//     limit 封顶;为判有没有下一页多取的那一行不重建。
//   - 复核事实从复核册按版本折叠。版本与复核是一对多,所以分两句:先按查询取一页版本,再只为
//     本页这几版取复核——连在一句里,一版多条复核会把一页撑满、把别的版本挤出去,总数也会数成
//     复核行。**最近一次复核**取时刻最大者,同刻按 reviewer 由 ORDER BY 定序取扫到的最后
//     一条——同参数两次查询答同一条。
//
// 这里没有「在用」,也不做任何排序裁决:在用是相对评价形成时刻的结论,规则只在
// domain.SelectInForceSeriesVersion 一处,目录页没有那个时刻(票 04 的 owner 裁决)。
func (catalogue *OperationsCatalogue) ListReferenceSeries(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ReferenceSeriesCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ReferenceSeriesCatalogueRow], error) {
		return cataloguepage.Result[ports.ReferenceSeriesCatalogueRow]{}, fmt.Errorf("list reference series: %w", err)
	}
	if err := catalogueLimit("list reference series", limit); err != nil {
		return cataloguepage.Result[ports.ReferenceSeriesCatalogueRow]{}, err
	}
	plan, err := referenceSeriesPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "parcel_pricing.reference_series_version"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`series_id, series_version, kind, source_identifier, registrant,
		 quote_basis_id, quote_basis_version, effective_from, effective_to,
		 evidence_grade, prior_version, correction_basis,
		 canonicalization, content_digest, snapshot, registered_at`, source,
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedSeriesVersion, 0, limit+1)
	for rows.Next() {
		var scanned scannedSeriesVersion
		row := &scanned.row
		var quoteBasisID, quoteBasisVersion *string
		var effectiveTo *time.Time
		var priorVersion, correctionBasis *string
		if err := rows.Scan(
			&row.SeriesID, &row.SeriesVersion, &row.Kind, &row.SourceIdentifier, &row.Registrant,
			&quoteBasisID, &quoteBasisVersion, &row.EffectiveFrom, &effectiveTo,
			&row.EvidenceGrade, &priorVersion, &correctionBasis,
			&row.Canonicalization, &row.ContentDigest, &scanned.snapshot, &row.RegisteredAt,
		); err != nil {
			return fail(err)
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
		fetched = append(fetched, scanned)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()

	page, err := cataloguepage.Finish(query, fetched, limit, total, seriesVersionPosition)
	if err != nil {
		return fail(err)
	}

	series := make([]ports.ReferenceSeriesCatalogueRow, 0, len(page.Rows))
	index := make(map[[2]string]int, len(page.Rows))
	seriesIDs := make([]string, 0, len(page.Rows))
	seriesVersions := make([]string, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		row := scanned.row
		registration, err := rehydrateRegisteredSeries(scanned.snapshot, registeredSeriesColumns{
			tenant: tenant, seriesID: row.SeriesID, seriesVersion: row.SeriesVersion,
			kind: row.Kind, grade: row.EvidenceGrade, canonicalization: row.Canonicalization, digest: row.ContentDigest,
		})
		if err != nil {
			return fail(err)
		}
		row.ReferenceDigest = registration.Reference().Digest()
		row.Periods = transcribeSeriesPeriods(registration)
		index[[2]string{row.SeriesID, row.SeriesVersion}] = len(series)
		series = append(series, row)
		seriesIDs = append(seriesIDs, row.SeriesID)
		seriesVersions = append(seriesVersions, row.SeriesVersion)
	}
	if len(series) > 0 {
		if err := foldSeriesReviews(ctx, querier, tenant, seriesIDs, seriesVersions, series, index); err != nil {
			return fail(err)
		}
	}
	return cataloguepage.Result[ports.ReferenceSeriesCatalogueRow]{Rows: series, Next: page.Next, Total: page.Total}, nil
}

// referenceSeriesPages 把 ports.ReferenceSeriesPages 落到序列版本表的列上。
var referenceSeriesPages = ports.ReferenceSeriesPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"registeredAt": "registered_at", "seriesId": "series_id"},
	Identity: []string{"series_id", "series_version"},
	Filters:  map[string]string{"kind": "kind", "seriesId": "series_id"},
	Keyword:  []string{"series_id", "source_identifier", "registrant"},
})

// scannedSeriesVersion 是取页读回的一版:列面已按在场与否翻译好,快照留到切出本页之后才经重建门读。
type scannedSeriesVersion struct {
	row      ports.ReferenceSeriesCatalogueRow
	snapshot []byte
}

// seriesVersionPosition 的登记时间取自扫描出的 registered_at 列,与 ORDER BY 比的是同一个值(cataloguepage.Finish 头注)。
func seriesVersionPosition(scanned scannedSeriesVersion, sortField string) cataloguepage.Position {
	value := scanned.row.SeriesID
	if sortField == "registeredAt" {
		value = cataloguepage.FormatInstant(scanned.row.RegisteredAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{scanned.row.SeriesID, scanned.row.SeriesVersion}}
}

// foldSeriesReviews 只为本页这几版取复核,按版本折进 series:条数、通过条数与最近一次复核。
func foldSeriesReviews(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	seriesIDs, seriesVersions []string,
	series []ports.ReferenceSeriesCatalogueRow,
	index map[[2]string]int,
) error {
	rows, err := querier.Query(ctx,
		`SELECT series_id, series_version, reviewed_at, decision
		   FROM parcel_pricing.reference_series_review
		  WHERE tenant_id = $1
		    AND (series_id, series_version) IN (SELECT * FROM unnest($2::text[], $3::text[]))
		  ORDER BY reviewed_at, reviewer`,
		tenant.String(), seriesIDs, seriesVersions,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var seriesID, seriesVersion, decision string
		var reviewedAt time.Time
		if err := rows.Scan(&seriesID, &seriesVersion, &reviewedAt, &decision); err != nil {
			return err
		}
		position, listed := index[[2]string{seriesID, seriesVersion}]
		if !listed {
			return fmt.Errorf("复核册答回了本页之外的版本 %s/%s", seriesID, seriesVersion)
		}
		version := &series[position]
		version.ReviewCount++
		if decision == domain.SeriesReviewApproved.String() {
			version.ApprovedReviewCount++
		}
		if !version.HasReview || !reviewedAt.Before(version.LastReviewedAt) {
			version.LastReviewedAt = reviewedAt
			version.LastReviewDecision = decision
			version.HasReview = true
		}
	}
	return rows.Err()
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
