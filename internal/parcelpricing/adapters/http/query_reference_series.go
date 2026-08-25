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
	RegisteredAt      string `json:"registeredAt"`
}

func referenceSeriesBodyOf(row ports.ReferenceSeriesCatalogueRow) referenceSeriesBody {
	body := referenceSeriesBody{
		SeriesID:         row.SeriesID,
		SeriesVersion:    row.SeriesVersion,
		Kind:             row.Kind,
		SourceIdentifier: row.SourceIdentifier,
		Registrant:       row.Registrant,
		EffectiveFrom:    rfc3339(row.EffectiveFrom),
		EvidenceGrade:    row.EvidenceGrade,
		Canonicalization: row.Canonicalization,
		ContentDigest:    row.ContentDigest,
		RegisteredAt:     rfc3339(row.RegisteredAt),
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
	return body
}
