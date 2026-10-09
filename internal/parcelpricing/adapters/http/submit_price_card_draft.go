package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 价卡草稿录入口（ADR-0101 决定三；票 price-card-import/03）：上传一份模板文件，读法与预览口同一段，读得出方案身份
// 就落进草稿册。租户与录入者来自操作者信封，载荷里没有身份格。

// PriceCardDraftSubmissionIntake 把一次已认证的接入请求翻译成录入命令：上传的文件 + 信封里的租户与录入者。
type PriceCardDraftSubmissionIntake interface {
	IntakePriceCardDraftSubmission(ctx context.Context, request *http.Request) (application.SubmitPriceCardDraftCommand, error)
}

// PriceCardDraftSubmitter 是本端点转交的录入编排。
type PriceCardDraftSubmitter interface {
	Handle(ctx context.Context, command application.SubmitPriceCardDraftCommand) (application.SubmitPriceCardDraftResult, error)
}

// NewSubmitPriceCardDraftEndpoint 交回录入口（POST /pricing-price-card-drafts）。
func NewSubmitPriceCardDraftEndpoint(intake PriceCardDraftSubmissionIntake, submitter PriceCardDraftSubmitter) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePriceCardDraftSubmission(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := submitter.Handle(request.Context(), command)
		if err != nil {
			// 落没落行未知（草稿册读写失败，或读口交出了领域不认的读法）：没形成答案，不折成某一格落点。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := priceCardDraftSubmissionResponse{
			Outcome: result.Outcome.String(),
			Reading: priceCardPreviewResponseOf(result.Reading),
		}
		status := http.StatusOK
		if wroteTheRow(result.Outcome) && result.HasDraft {
			row := priceCardDraftBodyOf(result.Draft)
			answer.Draft, status = &row, http.StatusCreated
		}
		writeJSON(response, status, answer)
	})
}

// wroteTheRow 答这一落点是不是本次写下了册上那一行。重放可以落在已批准乃至已发布的行上，内容已固定的行已批准，
// 两格里应用层交回的草稿是这一次拟录的那份而不是册上的；把它当册上的行交出去，一行已批准的草稿就被答成了
// `已校验`。所以只有这两格带行、取 201（ADR-0022：状态码只说答案有没有形成、有没有新落一行）。
func wroteTheRow(outcome application.SubmitPriceCardDraftOutcome) bool {
	return outcome == application.PriceCardDraftSubmitted || outcome == application.PriceCardDraftRevised
}

// priceCardDraftSubmissionResponse 是录入口的答复：`outcome` 是落点，`reading` 与预览口的答复逐字段同形——同一份
// 字节过两口答同一份读法；`draft` 只在本次写下了那一行时在场，册上此刻是什么由查阅读口答。
type priceCardDraftSubmissionResponse struct {
	Outcome string                   `json:"outcome"`
	Reading priceCardPreviewResponse `json:"reading"`
	Draft   *priceCardDraftBody      `json:"draft,omitempty"`
}

// priceCardDraftBody 是草稿册上的一行，录入口与查阅读口同形。录入者与批准者并排（ADR-0101 Consequences）；
// 批准与发布两格的痕迹缺席即还没走到那一步，内容概要自`已校验`起在场，逐格问题只有`草稿`有。
type priceCardDraftBody struct {
	Plan        planReferenceBody      `json:"plan"`
	Status      string                 `json:"status"`
	SourceFile  sourceFileBody         `json:"sourceFile"`
	Submitter   string                 `json:"submitter"`
	SubmittedAt string                 `json:"submittedAt"`
	Approver    string                 `json:"approver,omitempty"`
	ApprovedAt  string                 `json:"approvedAt,omitempty"`
	PublishedAt string                 `json:"publishedAt,omitempty"`
	Content     *priceCardContentBody  `json:"content,omitempty"`
	Problems    []priceCardProblemBody `json:"problems"`
}

func priceCardDraftBodyOf(draft domain.PriceCardDraft) priceCardDraftBody {
	body := priceCardDraftBody{
		Plan:        planReferenceBodyOf(draft.Plan()),
		Status:      draft.Status().String(),
		SourceFile:  sourceFileBody{Name: draft.SourceFile().Name(), SHA256: draft.SourceFile().SHA256()},
		Submitter:   draft.Submitter(),
		SubmittedAt: rfc3339(draft.SubmittedAt()),
		Problems:    []priceCardProblemBody{},
	}
	if approver, ok := draft.Approver(); ok {
		body.Approver = approver
	}
	if approvedAt, ok := draft.ApprovedAt(); ok {
		body.ApprovedAt = rfc3339(approvedAt)
	}
	if publishedAt, ok := draft.PublishedAt(); ok {
		body.PublishedAt = rfc3339(publishedAt)
	}
	if content, ok := draft.Content(); ok {
		summary := priceCardContentBodyOf(content.Plan)
		summary.DirectionAuthorization = planReferenceBodyOf(content.DirectionAuthorization)
		body.Content = &summary
	}
	for _, problem := range draft.Problems() {
		body.Problems = append(body.Problems, priceCardProblemBody{
			Sheet: problem.Sheet(), Row: problem.Row(), Column: problem.Column(), Code: problem.Code(), Message: problem.Message(),
		})
	}
	return body
}

var _ PriceCardDraftSubmissionIntake = (*OperatorRegistryIntake)(nil)

// IntakePriceCardDraftSubmission 是操作者渠道对录入的译法：先认证、后解上传，与预览口同一段解码与上限。租户与
// 录入者都取认证出的身份——录入者是 ADR-0101 决定三记在草稿上的那一格，审批职责规则（票 04）拿它与批准者比。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftSubmission(ctx context.Context, request *http.Request) (application.SubmitPriceCardDraftCommand, error) {
	operator, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.SubmitPriceCardDraftCommand{}, err
	}
	upload, err := DecodePriceCardUpload(request)
	if err != nil {
		return application.SubmitPriceCardDraftCommand{}, err
	}
	return application.SubmitPriceCardDraftCommand{
		Tenant: operator.Tenant, Submitter: operator.Operator, FileName: upload.FileName, Raw: upload.Raw,
	}, nil
}
