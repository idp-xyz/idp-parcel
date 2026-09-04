package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ReferenceSeriesCoverage 实现 ports.ReferenceSeriesCoverageRead(票
// pricing-reference-series-operations/05 第 1 项):按租户交出每条序列的覆盖地平线摘要。
//
// 只读,没有任何写口——登记仍走 ReferenceSeriesVersions,复核仍走 ReferenceSeriesReviews。
// 另立一个类型而不是给 OperationsCatalogue 加方法:那两口只读版本表一张,本口要把版本
// 与复核两张连起来,而伴生读端口的立意就是不让一个接口随着新问法不断变宽(catalogue_read.go
// 的头注记过扩写侧接口会拆全部替身的那条代价)。
//
// **在用版本的选择规则不在本文件。** SQL 只把这条序列的全部版本与它们的每一条复核取成
// 候选,交 domain.SelectInForceSeriesVersion 挑——与 ReferenceSeriesReviews.ResolveInForce
// 同一条规则、同一处定义。在这里用 ORDER BY 排出「最新通过的那版」会让同一个问题有两套
// 答案,而它们只在恰好同时被改时才一致。
//
// 不读快照:权威内容在快照、读回须经领域整图重验,那是评价装载的纪律;本口只转写列面,
// 候选所需的版本引用由(标识、版本、内容摘要)三列构造,不必展开快照。
type ReferenceSeriesCoverage struct {
	db *bentopg.DB
}

func NewReferenceSeriesCoverage(db *bentopg.DB) (*ReferenceSeriesCoverage, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceSeriesCoverage{db: db}, nil
}

var _ ports.ReferenceSeriesCoverageRead = (*ReferenceSeriesCoverage)(nil)

// coverageWindow 是一版的适用期包络。用显式布尔而不是零值判断:零时刻是一个合法的绝对
// 时刻,拿它兼作「没有上界」会让补历史的区间读不出来(同 PriceCardCatalogueRow 那条)。
type coverageWindow struct {
	from  time.Time
	to    time.Time
	hasTo bool
}

// coverageGroup 攒一条序列(标识 + 种类)的中间态。版本与复核是一对多,连接后一版会出现
// 多行,所以逐版去重的账要单独记,不能拿行数当版本数。
type coverageGroup struct {
	seriesID   string
	kind       string
	order      int
	windows    map[string]coverageWindow
	reviewed   map[string]bool
	approved   map[string]bool
	candidates []domain.ReviewedSeriesVersion
	lastAt     time.Time
	lastCall   string
}

// ListReferenceSeriesCoverage 交出每条序列此刻的覆盖摘要。at 是「在用」所参照的时刻,
// 由调用方给:在用版本是该时刻之前复核通过的最新版本,读口自己取 now 会让同参数的两次
// 查询答不同的话。
//
// limit 数的是**序列条数**不是行数:一条序列有几版是登记方的事,拿版本数分页会让一条
// 多版序列把整页占满,而调用方要的是「我有几条序列、各自还剩多少」。
func (coverage *ReferenceSeriesCoverage) ListReferenceSeriesCoverage(
	ctx context.Context,
	tenant domain.TenantID,
	at time.Time,
	limit int,
) ([]ports.ReferenceSeriesCoverageRow, error) {
	if err := catalogueLimit("list reference series coverage", limit); err != nil {
		return nil, err
	}
	if tenant.String() == "" {
		return nil, fmt.Errorf("list reference series coverage: tenant is required")
	}
	// 零时刻在领域里恒答「没有在用版本」(SelectInForceSeriesVersion 首行就拒)。放它进来
	// 会让每一行都显示未在用,而那是调用方忘了传时刻,不是登记册的事实。
	if at.IsZero() {
		return nil, fmt.Errorf("list reference series coverage: moment is required")
	}
	querier, err := coverage.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reference series coverage: %w", err)
	}

	rows, err := querier.Query(ctx,
		`WITH picked AS (
		     SELECT series_id, kind
		       FROM parcel_pricing.reference_series_version
		      WHERE tenant_id = $1
		      GROUP BY series_id, kind
		      ORDER BY series_id, kind
		      LIMIT $2
		 )
		 SELECT v.series_id, v.kind, v.series_version, v.content_digest,
		        v.registered_at, v.effective_from, v.effective_to,
		        r.reviewed_at, r.decision
		   FROM parcel_pricing.reference_series_version v
		   JOIN picked p ON p.series_id = v.series_id AND p.kind = v.kind
		   LEFT JOIN parcel_pricing.reference_series_review r
		     ON r.tenant_id = v.tenant_id
		    AND r.series_id = v.series_id
		    AND r.series_version = v.series_version
		  WHERE v.tenant_id = $1
		  ORDER BY v.series_id, v.kind, v.series_version, r.reviewed_at, r.reviewer`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list reference series coverage: %w", err)
	}
	defer rows.Close()

	groups := make(map[string]*coverageGroup)
	order := 0
	for rows.Next() {
		var seriesID, kind, version, digest string
		var registeredAt, effectiveFrom time.Time
		var effectiveTo *time.Time
		var reviewedAt *time.Time
		var decision *string
		if err := rows.Scan(
			&seriesID, &kind, &version, &digest,
			&registeredAt, &effectiveFrom, &effectiveTo,
			&reviewedAt, &decision,
		); err != nil {
			return nil, fmt.Errorf("list reference series coverage: %w", err)
		}

		key := seriesID + "\x00" + kind
		group, seen := groups[key]
		if !seen {
			group = &coverageGroup{
				seriesID: seriesID,
				kind:     kind,
				order:    order,
				windows:  make(map[string]coverageWindow),
				reviewed: make(map[string]bool),
				approved: make(map[string]bool),
			}
			groups[key] = group
			order++
		}
		if _, known := group.windows[version]; !known {
			window := coverageWindow{from: effectiveFrom}
			if effectiveTo != nil {
				window.to = *effectiveTo
				window.hasTo = true
			}
			group.windows[version] = window
			group.reviewed[version] = false
			group.approved[version] = false
		}
		if reviewedAt == nil || decision == nil {
			continue
		}

		group.reviewed[version] = true
		if *decision == domain.SeriesReviewApproved.String() {
			group.approved[version] = true
		}
		// 最近一次复核取时刻最大者;同刻按 reviewer 由 ORDER BY 定序,取扫到的最后一条,
		// 所以同参数两次查询答同一条。这一格是「最近有人看过这条序列没有」,与在用无关
		// ——一条退回也是看过。
		if !reviewedAt.Before(group.lastAt) {
			group.lastAt = *reviewedAt
			group.lastCall = *decision
		}

		// 这条引用带的指纹是那一版登记的内容摘要（ADR-0108：有真摘要就带），不是身份的一部分。
		reference, err := domain.NewVersionReferenceWithFingerprint(domain.ArtifactReferenceSeries, seriesID, version, digest)
		if err != nil {
			return nil, fmt.Errorf("list reference series coverage: %s/%s：%w", seriesID, version, err)
		}
		candidate, err := domain.NewReviewedSeriesVersion(
			reference, registeredAt, *reviewedAt, domain.SeriesReviewDecision(*decision))
		if err != nil {
			return nil, fmt.Errorf("list reference series coverage: review row for %s/%s：%w", seriesID, version, err)
		}
		group.candidates = append(group.candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reference series coverage: %w", err)
	}

	ordered := make([]*coverageGroup, len(groups))
	for _, group := range groups {
		ordered[group.order] = group
	}
	summaries := make([]ports.ReferenceSeriesCoverageRow, 0, len(ordered))
	for _, group := range ordered {
		summaries = append(summaries, group.summarise(at))
	}
	return summaries, nil
}

// summarise 把一条序列的中间态折成一行。挑在用那一步交领域,本函数只负责把领域挑出来的
// 引用对回它的适用期,以及数那两笔各自等着不同人的欠账。
func (group *coverageGroup) summarise(at time.Time) ports.ReferenceSeriesCoverageRow {
	row := ports.ReferenceSeriesCoverageRow{
		SeriesID:               group.seriesID,
		Kind:                   group.kind,
		RegisteredVersionCount: len(group.windows),
	}
	for version, reviewed := range group.reviewed {
		switch {
		case !reviewed:
			row.UnreviewedVersionCount++
		case !group.approved[version]:
			row.ReturnedVersionCount++
		}
	}
	if !group.lastAt.IsZero() {
		row.LastReviewedAt = group.lastAt
		row.LastReviewDecision = group.lastCall
		row.HasReview = true
	}
	inForce, found := domain.SelectInForceSeriesVersion(group.candidates, at)
	if !found {
		return row
	}
	row.InForceVersion = inForce.Version()
	row.HasInForceVersion = true
	if window, known := group.windows[inForce.Version()]; known {
		row.InForceEffectiveFrom = window.from
		row.InForceEffectiveTo = window.to
		row.HasInForceEffectiveTo = window.hasTo
	}
	return row
}
