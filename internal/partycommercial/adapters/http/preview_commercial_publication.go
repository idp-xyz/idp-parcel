package commercialhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// CommercialPublicationPreviewIntake 把一次已认证的接入请求翻译成发布前预览命令。
//
// 预览不写库，却与录入口同族而不与目录查阅同族：拟录的壳要信封里的租户才立得住，租户不从载荷里取——
// 预览若采信自报租户，录入时换成信封里的那个，壳就是另一份（ADR-0101 决定四要防的正是「预览通过、
// 录入却不同」）。因此隔离读放行（ADR-0078）装不进本口，与录入口同一道编译期排除。
//
// 载荷形状由产品定义、属机制半边；预览与录入共用 CommercialPublicationPayload 同一段解码，解出同一组领域
// 值对象，摘要在领域一处算，两口自然逐字节同答（ADR-0126 Decision 四）。
type CommercialPublicationPreviewIntake interface {
	IntakeCommercialPublicationPreview(
		ctx context.Context,
		request *http.Request,
	) (application.PreviewCommercialPublicationCommand, error)
}

// CommercialPublicationPreviewer 是本端点转交的预览用例。没有事务包装：预览不读也不写。
type CommercialPublicationPreviewer interface {
	Handle(
		ctx context.Context,
		command application.PreviewCommercialPublicationCommand,
	) (application.CommercialPublicationPreview, error)
}

var _ CommercialPublicationPreviewer = (*application.PreviewCommercialPublicationHandler)(nil)

// NewPreviewCommercialPublicationEndpoint 交回商业发布前预览的 HTTP 入口（POST /commercial-publication-previews；
// ADR-0126 Decision 四）。
//
// 用 POST 而不是 GET：拟录的整份壳与正文在请求体里，它不是对册上某个资源的查阅，而是「把这份载荷过一遍
// 构造门，告诉我会得到什么」。路径叫 `-previews` 而不并进发布口加 dry-run 参数：并进去之后装配点可以把预览
// 编排接到发布端点上而编译仍绿，且「一个参数决定写不写库」正是最容易被顺手改错的那种格。
//
// 逐格问题在这里成为一格答案：Intake 解码收齐的 PublicationPayloadProblems 走 200 + NOT_ACCEPTED + problems——
// 预览的用途就是告诉表单哪几格不对，那是形成了的答案（ADR-0022），不是「请求畸形」。别的 Intake 失败照旧
// 分流（403 未配置 / 400 畸形 / 5xx 故障）。
func NewPreviewCommercialPublicationEndpoint(
	intake CommercialPublicationPreviewIntake,
	previewer CommercialPublicationPreviewer,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeCommercialPublicationPreview(request.Context(), request)
		if err != nil {
			var problems *PublicationPayloadProblems
			if errors.As(err, &problems) {
				writeJSON(response, http.StatusOK, publicationPreviewAnswer{
					Outcome:  application.CommercialPublicationPreviewNotAccepted.String(),
					Problems: payloadProblemAnswersOf(problems),
				})
				return
			}
			writeRegistrationIntakeProblem(response, err)
			return
		}

		preview, err := previewer.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := preview.Outcome().String()
		if name == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := publicationPreviewAnswer{Outcome: name}
		if cause := preview.RefusalCause(); cause != nil {
			answer.Cause = cause.Error()
		}
		if canonical, ok := preview.Canonical(); ok {
			answer.Canonicalization = canonical.Canonicalization()
			answer.ContentDigest = canonical.Digest().String()
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// publicationPreviewAnswer 是预览的封闭响应形状：算出来了带规范化版本与摘要；`未受理`带成因（构造门的原话）
// 或逐格问题（解码收齐的那几格）。不受理时没有摘要可透——免得一个空摘要看起来像算出来的。
type publicationPreviewAnswer struct {
	Outcome          string                 `json:"outcome"`
	Canonicalization string                 `json:"canonicalization,omitempty"`
	ContentDigest    string                 `json:"contentDigest,omitempty"`
	Cause            string                 `json:"cause,omitempty"`
	Problems         []payloadProblemAnswer `json:"problems,omitempty"`
}

type payloadProblemAnswer struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func payloadProblemAnswersOf(problems *PublicationPayloadProblems) []payloadProblemAnswer {
	answers := make([]payloadProblemAnswer, 0, len(problems.Problems))
	for _, problem := range problems.Problems {
		answers = append(answers, payloadProblemAnswer{Field: problem.Field, Problem: problem.Problem})
	}
	return answers
}
