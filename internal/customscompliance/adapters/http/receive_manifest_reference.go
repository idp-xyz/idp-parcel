package customshttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// ManifestReferenceIntake 把一次已认证的通道请求翻译成外部监管舱单引用的首登或版本推进命令。租户与承运商责任都只取
// 认证结果：舱单由责任承运商在产品之外形成并提交，交引用来的客户端在册上绑定的来源身份就是它代表的承运商；载荷自报
// 承运商等于让持令牌的一方替别家报舱单。
type ManifestReferenceIntake interface {
	IntakeManifestReference(ctx context.Context, request *http.Request) (application.ReceiveManifestCommand, error)
	IntakeManifestRevision(ctx context.Context, request *http.Request) (application.ReviseManifestCommand, error)
}

// ManifestHandler 是本适配器转交的应用编排。
type ManifestHandler interface {
	Receive(ctx context.Context, command application.ReceiveManifestCommand) (application.ManifestResult, error)
	Revise(ctx context.Context, command application.ReviseManifestCommand) (application.ManifestResult, error)
}

// NewReceiveManifestReferenceEndpoint 交回外部监管舱单引用首登的 HTTP 入口（UC-CC-012）。
func NewReceiveManifestReferenceEndpoint(intake ManifestReferenceIntake, handler ManifestHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeManifestReference(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := handler.Receive(request.Context(), command)
		writeManifestOutcome(response, result, err)
	})
}

// NewReviseManifestReferenceEndpoint 交回来源版本推进的 HTTP 入口：承运商明确的更正、撤销、替代或范围变化。它与首登
// 分两口——同舱单异版本经首登口来是版本冲突，只有走这一口才留得下前版指回。
func NewReviseManifestReferenceEndpoint(intake ManifestReferenceIntake, handler ManifestHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeManifestRevision(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := handler.Revise(request.Context(), command)
		writeManifestOutcome(response, result, err)
	})
}

// manifestResponse 是这两口的封闭响应形状。`outcome` 取应用结果枚举的原名。在册引用的版本与关联只在本次形成或命中了
// 这家承运商自己的引用时交回；未受理与不在册不带任何在册内容。
type manifestResponse struct {
	Outcome          string `json:"outcome"`
	Version          string `json:"version,omitempty"`
	PriorVersion     string `json:"priorVersion,omitempty"`
	DeclarationUnit  string `json:"declarationUnit,omitempty"`
	HandoffReference string `json:"handoffReference,omitempty"`
}

func writeManifestOutcome(response http.ResponseWriter, result application.ManifestResult, err error) {
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 编排的「未决」只在读写引用册或盘候选失败时出现，是没形成答案而不是业务未决——待关联才是业务答案。答 5xx 让通道
	// 留队重发；答 200 的话来源会以为这份舱单已经有了着落。
	if result.Outcome() == application.ManifestUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := manifestResponse{Outcome: outcome, HandoffReference: result.HandoffReference()}
	if reference, has := result.Reference(); has {
		body.Version = reference.Version().String()
		if prior, revised := reference.PriorVersion(); revised {
			body.PriorVersion = prior.String()
		}
		if unit, associated := reference.Association(); associated {
			body.DeclarationUnit = unit.String()
		}
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.ManifestAssociated, application.ManifestPendingAssociation, application.ManifestRevised:
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
