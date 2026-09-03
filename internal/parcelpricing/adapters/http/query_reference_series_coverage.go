package pricinghttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ReferenceSeriesCoverageReader 是覆盖地平线端点消费的读口。
type ReferenceSeriesCoverageReader interface {
	ListReferenceSeriesCoverage(
		ctx context.Context,
		tenant domain.TenantID,
		at time.Time,
		limit int,
	) ([]ports.ReferenceSeriesCoverageRow, error)
}

// 编译期锁缝:读口形状与端口保持一致。
var _ ReferenceSeriesCoverageReader = ports.ReferenceSeriesCoverageRead(nil)

// CoverageClock 交出解析「在用」所参照的那一刻。
//
// 端口把 at 放在签名上是为了可重放;而 HTTP 面这一层的正当时刻源就是服务端的 now
// ——这一页问的本来就是「从现在算还盖得住多久」。**不开 `?at=` 查询参数**:历史问法
// 今天没有调用方,现在加就是发明一个没人要的形状;端口侧已留口,要时加得上。
type CoverageClock interface {
	Now() time.Time
}

// outcomeReferenceSeriesCoverageListed 是本端点唯一的业务成格,判据同两个目录读口。
const outcomeReferenceSeriesCoverageListed = "REFERENCE_SERIES_COVERAGE_LISTED"

// NewQueryReferenceSeriesCoverageEndpoint 交回参考序列覆盖地平线查阅的 HTTP 入口
// (GET /pricing-reference-series-coverage,ADR-0077 读面通例;票
// pricing-reference-series-operations/05 第 1 项;最终路径归装配票)。
//
// 一条序列一行,不是一版一行——一版一行的问法归 /pricing-reference-series。
func NewQueryReferenceSeriesCoverageEndpoint(
	intake PricingCatalogueIntake,
	reader ReferenceSeriesCoverageReader,
	clock CoverageClock,
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

		at := clock.Now().UTC()
		rows, err := reader.ListReferenceSeriesCoverage(request.Context(), query.Scope.Tenant(), at, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]referenceSeriesCoverageBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, referenceSeriesCoverageBodyOf(row))
		}
		writeJSON(response, http.StatusOK, referenceSeriesCoverageListResponse{
			Outcome: outcomeReferenceSeriesCoverageListed,
			AsOf:    rfc3339(at),
			Series:  bodies,
		})
	})
}

// referenceSeriesCoverageListResponse 顶层带 asOf。
//
// **它不是装饰。** 「在用」是只对某一刻成立的结论,不回显那一刻,页面上就会出现一个看起来
// 永久的权威答案,而它其实只对服务端那一瞬成立。同一条判据下,目录页的状态列被裁为不含
// 「在用」——那一页没有正当的时刻源;本端点有,代价是必须把它说出来。
type referenceSeriesCoverageListResponse struct {
	Outcome string                        `json:"outcome"`
	AsOf    string                        `json:"asOf"`
	Series  []referenceSeriesCoverageBody `json:"series"`
}

// referenceSeriesCoverageBody 逐字段透出一条序列的覆盖摘要。
//
// **在用那一组做成子对象而不是平铺,是为了防一处折叠**:若把止点平铺成一个可缺席的键,
// 「这条序列没有在用版本」与「有在用版本但它没有上界」都表现为止点缺席,而两者一个是
// 「今天没得用」、一个是「用着且不会到期」,续办动作相反。子对象在场与否只答第一问,
// 子对象里的 openEnded 只答第二问。inForceResolved 与子对象成对,理由同 party-commercial
// 的 caliberDeclared:布尔让调用方分得开「服务端说没有」与「这个键没序列化出来」。
//
// 没有「剩余多少天」这一格:无上界时那个数既不是 0 也不是无穷,是「没有终点」;算差值要挑
// 时区与舍入口径,那属于呈现面。页面拿 asOf 与 effectiveTo 自己算。
type referenceSeriesCoverageBody struct {
	SeriesID               string                      `json:"seriesId"`
	Kind                   string                      `json:"kind"`
	RegisteredVersionCount int                         `json:"registeredVersionCount"`
	InForceResolved        bool                        `json:"inForceResolved"`
	InForce                *referenceSeriesInForceBody `json:"inForce,omitempty"`
	LastReviewedAt         string                      `json:"lastReviewedAt,omitempty"`
	LastReviewDecision     string                      `json:"lastReviewDecision,omitempty"`
	UnreviewedVersionCount int                         `json:"unreviewedVersionCount"`
	ReturnedVersionCount   int                         `json:"returnedVersionCount"`
}

// referenceSeriesInForceBody 是在用那一版的引用与它的适用期。OpenEnded 为真时 EffectiveTo
// 缺席,那是「没有终点」不是「终点未知」。
type referenceSeriesInForceBody struct {
	Version       string `json:"version"`
	EffectiveFrom string `json:"effectiveFrom"`
	EffectiveTo   string `json:"effectiveTo,omitempty"`
	OpenEnded     bool   `json:"openEnded"`
}

func referenceSeriesCoverageBodyOf(row ports.ReferenceSeriesCoverageRow) referenceSeriesCoverageBody {
	body := referenceSeriesCoverageBody{
		SeriesID:               row.SeriesID,
		Kind:                   row.Kind,
		RegisteredVersionCount: row.RegisteredVersionCount,
		InForceResolved:        row.HasInForceVersion,
		UnreviewedVersionCount: row.UnreviewedVersionCount,
		ReturnedVersionCount:   row.ReturnedVersionCount,
	}
	if row.HasReview {
		body.LastReviewedAt = rfc3339(row.LastReviewedAt)
		body.LastReviewDecision = row.LastReviewDecision
	}
	if row.HasInForceVersion {
		inForce := referenceSeriesInForceBody{
			Version:       row.InForceVersion,
			EffectiveFrom: rfc3339(row.InForceEffectiveFrom),
			OpenEnded:     !row.HasInForceEffectiveTo,
		}
		if row.HasInForceEffectiveTo {
			inForce.EffectiveTo = rfc3339(row.InForceEffectiveTo)
		}
		body.InForce = &inForce
	}
	return body
}
