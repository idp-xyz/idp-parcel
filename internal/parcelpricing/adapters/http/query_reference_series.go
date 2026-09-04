package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ReferenceSeriesCatalogueReader 是参考序列登记册端点消费的读口。
type ReferenceSeriesCatalogueReader interface {
	ListReferenceSeries(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ReferenceSeriesCatalogueRow, error)
}

// 编译期锁缝:读口形状与端口保持一致。
var _ ReferenceSeriesCatalogueReader = ports.ReferenceSeriesCatalogueRead(nil)

// outcomeReferenceSeriesListed 是本端点唯一的业务成格,判据同价卡目录。
const outcomeReferenceSeriesListed = "REFERENCE_SERIES_LISTED"

// NewQueryReferenceSeriesEndpoint 交回计价参考序列登记册查阅的 HTTP 入口
// (GET /pricing-reference-series,ADR-0077、票 master-data-wiring/02;最终路径归
// 装配票)。逐期取值不在响应里:期次的消费口是按计价基准时点的解析,目录只答
// 「登了哪些版本、什么来源、什么证据等级」。
func NewQueryReferenceSeriesEndpoint(
	intake PricingCatalogueIntake,
	reader ReferenceSeriesCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		rows, err := reader.ListReferenceSeries(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]referenceSeriesBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, referenceSeriesBodyOf(row))
		}
		writeJSON(response, http.StatusOK, referenceSeriesListResponse{
			Outcome: outcomeReferenceSeriesListed,
			Series:  bodies,
		})
	})
}

type referenceSeriesListResponse struct {
	Outcome string                `json:"outcome"`
	Series  []referenceSeriesBody `json:"series"`
}

// referenceSeriesBody 逐字段透出检索列面。口径两键与更正两键各自成对在场或成对
// 缺席(库上 CHECK 钉住,这里如实转写不补);evidenceGrade 照登记册汇总透出——
// 断言强度只准隔离验证,页面必须能看见这一格,不得粉饰成可复核。
//
// 票 pricing-reference-series-operations/08 加的两组:复核两计数**总在场**(无复核是 0 条,
// 不是缺键),最近一次复核两键成对在场或成对缺席;periods 逐期转写供「更正此版本」预填;
// referenceDigest 是登记时声明的引用 digest,与 contentDigest(PRS 内容摘要)分开两键。
// **没有任何「在用」键**:在用相对评价形成时刻而定,目录页没有那个时刻,归覆盖读口。
type referenceSeriesBody struct {
	SeriesID          string `json:"seriesId"`
	SeriesVersion     string `json:"seriesVersion"`
	Kind              string `json:"kind"`
	SourceIdentifier  string `json:"sourceIdentifier"`
	Registrant        string `json:"registrant"`
	QuoteBasisID      string `json:"quoteBasisId,omitempty"`
	QuoteBasisVersion string `json:"quoteBasisVersion,omitempty"`
	EffectiveFrom     string `json:"effectiveFrom"`
	EffectiveTo       string `json:"effectiveTo,omitempty"`
	EvidenceGrade     string `json:"evidenceGrade"`
	PriorVersion      string `json:"priorVersion,omitempty"`
	CorrectionBasis   string `json:"correctionBasis,omitempty"`
	Canonicalization  string `json:"canonicalization"`
	ContentDigest     string `json:"contentDigest"`
	ReferenceDigest   string `json:"referenceDigest"`
	RegisteredAt      string `json:"registeredAt"`

	ReviewCount         int    `json:"reviewCount"`
	ApprovedReviewCount int    `json:"approvedReviewCount"`
	LastReviewedAt      string `json:"lastReviewedAt,omitempty"`
	LastReviewDecision  string `json:"lastReviewDecision,omitempty"`

	Periods []referenceSeriesPeriodBody `json:"periods"`
}

// referenceSeriesPeriodBody 是一期取值:止点与凭证按在场与否给键——无上界不是「止点为空串」,
// 缺凭证不是「凭证为空串」,两处都用缺键表达「没有」。
type referenceSeriesPeriodBody struct {
	StartsAt    string `json:"startsAt"`
	EndsAt      string `json:"endsAt,omitempty"`
	Value       string `json:"value"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

func referenceSeriesBodyOf(row ports.ReferenceSeriesCatalogueRow) referenceSeriesBody {
	body := referenceSeriesBody{
		SeriesID:            row.SeriesID,
		SeriesVersion:       row.SeriesVersion,
		Kind:                row.Kind,
		SourceIdentifier:    row.SourceIdentifier,
		Registrant:          row.Registrant,
		EffectiveFrom:       rfc3339(row.EffectiveFrom),
		EvidenceGrade:       row.EvidenceGrade,
		Canonicalization:    row.Canonicalization,
		ContentDigest:       row.ContentDigest,
		ReferenceDigest:     row.ReferenceDigest,
		RegisteredAt:        rfc3339(row.RegisteredAt),
		ReviewCount:         row.ReviewCount,
		ApprovedReviewCount: row.ApprovedReviewCount,
		Periods:             make([]referenceSeriesPeriodBody, 0, len(row.Periods)),
	}
	if row.HasQuoteBasis {
		body.QuoteBasisID = row.QuoteBasisID
		body.QuoteBasisVersion = row.QuoteBasisVersion
	}
	if row.HasEffectiveTo {
		body.EffectiveTo = rfc3339(row.EffectiveTo)
	}
	if row.IsCorrection {
		body.PriorVersion = row.PriorVersion
		body.CorrectionBasis = row.CorrectionBasis
	}
	if row.HasReview {
		body.LastReviewedAt = rfc3339(row.LastReviewedAt)
		body.LastReviewDecision = row.LastReviewDecision
	}
	for _, period := range row.Periods {
		body.Periods = append(body.Periods, referenceSeriesPeriodBodyOf(period))
	}
	return body
}

func referenceSeriesPeriodBodyOf(period ports.ReferenceSeriesPeriodRow) referenceSeriesPeriodBody {
	body := referenceSeriesPeriodBody{
		StartsAt: rfc3339(period.StartsAt),
		Value:    period.Value,
	}
	if period.HasEndsAt {
		body.EndsAt = rfc3339(period.EndsAt)
	}
	if period.HasEvidence {
		body.EvidenceRef = period.EvidenceRef
	}
	return body
}
