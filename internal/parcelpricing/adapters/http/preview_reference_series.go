package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ReferenceSeriesPreviewIntake 把一次已认证的接入请求翻译成登记前预览命令。
//
// 预览不写库，却与两个登记口同族而不与目录查阅同族：拟登的登记本体要租户与登记责任方才立得住
// （CONTEXT 硬句），两样都来自 ADR-0100 的 `OperatorEnvelope`，不从载荷里取——预览若采信自报的
// 登记责任方，登记时换成信封里的那个，摘要就变了，「预览通过、登记却答内容冲突」那一格正是
// ADR-0101 决定四要防的。因此隔离读放行（ADR-0078）装不进本口，与登记口同一道编译期排除。
//
// 载荷形状按 ADR-0101 决定一由产品定义、属机制半边；预览与登记口共用同一份载荷解码，解出同一个
// 领域登记对象——摘要只在领域一处算，两口自然逐字节同答。
type ReferenceSeriesPreviewIntake interface {
	IntakeReferenceSeriesPreview(ctx context.Context, request *http.Request) (application.PreviewReferenceSeriesCommand, error)
}

// ReferenceSeriesPreviewer 是本端点转交的预览用例。没有事务包装：预览只读版本读口，不写。
type ReferenceSeriesPreviewer interface {
	Handle(ctx context.Context, command application.PreviewReferenceSeriesCommand) (application.ReferenceSeriesPreview, error)
}

// NewPreviewReferenceSeriesEndpoint 交回序列登记前预览的 HTTP 入口
// （POST /pricing-reference-series-previews；票 pricing-reference-series-operations/08）。
//
// 用 POST 而不是 GET：拟登的整版期次表在请求体里，它不是对册上某个资源的查阅，而是「把这份
// 载荷过一遍构造门，告诉我会得到什么」——与登记口收同一份载荷、答不同的东西。路径叫 `-previews`
// 而不并进 `-registrations` 加一个 dry-run 参数：并进去之后装配点可以把预览编排接到登记端点上而
// 编译仍绿，且「一个参数决定写不写库」正是最容易被顺手改错的那种格。
func NewPreviewReferenceSeriesEndpoint(intake ReferenceSeriesPreviewIntake, previewer ReferenceSeriesPreviewer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeReferenceSeriesPreview(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		preview, err := previewer.Handle(request.Context(), command)
		if err != nil {
			// 依赖故障不是业务答案（ADR-0022）：取对照版本失败时没有任何一格算得完整。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := preview.Outcome.String()
		if name == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		if preview.Outcome != application.ReferenceSeriesPreviewed {
			// 不受理没有摘要可透：只交 outcome，免得一个空摘要看起来像算出来的。
			writeJSON(response, http.StatusOK, registrationResponse{Outcome: name})
			return
		}
		writeJSON(response, http.StatusOK, referenceSeriesPreviewResponseOf(preview))
	})
}

// referenceSeriesPreviewResponse 是预览的封闭响应形状。等级、规范化版本与摘要原样透出；对照
// 一节总在场——没要求比也要说「没要求」，读的人才分得开「没比」与「比了没差异」。
type referenceSeriesPreviewResponse struct {
	Outcome          string                          `json:"outcome"`
	EvidenceGrade    string                          `json:"evidenceGrade"`
	Canonicalization string                          `json:"canonicalization"`
	ContentDigest    string                          `json:"contentDigest"`
	Comparison       referenceSeriesComparisonResult `json:"comparison"`
}

type referenceSeriesComparisonResult struct {
	Outcome     string                      `json:"outcome"`
	BaseVersion string                      `json:"baseVersion,omitempty"`
	Changes     []referenceSeriesChangeBody `json:"changes"`
}

// referenceSeriesChangeBody 是一期的比对结果：两侧期次按在场与否给键（新增无 base、移除无
// proposed），三个细项布尔总在场——它们只在 CHANGED 上为真，但缺键会让人猜是「没变」还是「没算」。
type referenceSeriesChangeBody struct {
	StartsAt        string                     `json:"startsAt"`
	Kind            string                     `json:"kind"`
	ValueChanged    bool                       `json:"valueChanged"`
	EndChanged      bool                       `json:"endChanged"`
	EvidenceChanged bool                       `json:"evidenceChanged"`
	Base            *referenceSeriesPeriodBody `json:"base,omitempty"`
	Proposed        *referenceSeriesPeriodBody `json:"proposed,omitempty"`
}

func referenceSeriesPreviewResponseOf(preview application.ReferenceSeriesPreview) referenceSeriesPreviewResponse {
	comparison := referenceSeriesComparisonResult{
		Outcome:     preview.Comparison.String(),
		BaseVersion: preview.BaseVersion,
		Changes:     make([]referenceSeriesChangeBody, 0, len(preview.Changes)),
	}
	for _, change := range preview.Changes {
		comparison.Changes = append(comparison.Changes, referenceSeriesChangeBodyOf(change))
	}
	return referenceSeriesPreviewResponse{
		Outcome:          preview.Outcome.String(),
		EvidenceGrade:    preview.EvidenceGrade.String(),
		Canonicalization: preview.Canonicalization,
		ContentDigest:    preview.ContentDigest,
		Comparison:       comparison,
	}
}

func referenceSeriesChangeBodyOf(change domain.SeriesPeriodChange) referenceSeriesChangeBody {
	body := referenceSeriesChangeBody{
		StartsAt:        rfc3339(change.StartsAt()),
		Kind:            change.Kind().String(),
		ValueChanged:    change.ValueChanged(),
		EndChanged:      change.EndChanged(),
		EvidenceChanged: change.EvidenceChanged(),
	}
	if base, ok := change.Base(); ok {
		period := periodBodyOfValue(base)
		body.Base = &period
	}
	if proposed, ok := change.Proposed(); ok {
		period := periodBodyOfValue(proposed)
		body.Proposed = &period
	}
	return body
}

// periodBodyOfValue 把领域期次转成与目录行同一形状的期次体：预览里的期次与目录里的期次是同一
// 种东西，两处一个形状，页面才能拿同一段呈现代码画它们。
func periodBodyOfValue(period domain.SeriesPeriodValue) referenceSeriesPeriodBody {
	body := referenceSeriesPeriodBody{
		StartsAt: rfc3339(period.StartsAt()),
		Value:    period.Value().String(),
	}
	if endsAt, bounded := period.EndsAt(); bounded {
		body.EndsAt = rfc3339(endsAt)
	}
	if evidence, verifiable := period.Evidence(); verifiable {
		body.EvidenceRef = evidence
	}
	return body
}
