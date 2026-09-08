package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 待批准发布载体的三个命令口（ADR-0126 Decision 三；票 admin-write-faces/08）：录入、批准、发布。
//
// 三口分设而不合成一个带动作字段的口：录入收整份壳与正文，批准与发布只收载体引用，三者的载荷形状、身份要求
// （录入者 / 批准者主体 / 只要租户）与答案代数各不相同；合成一口之后装配点可以只配一半而编译仍绿，且载荷里
// 会同时躺着动作与正文，等于把「批准者不得从载荷里来」那道门要挡的机会又递回给调用方。
//
// 三口都是运营操作者面，身份从 ADR-0100 的 `OperatorEnvelope` 来。Intake 是接口而不是实现：理由与 PP 复核口
// 同源——不是「等 PAR-INT-01 契约」，是「去接信封」。批准那一口尤其松不得：批准者是审批职责规则的一半，从
// 请求内容里铸一个出来就等于把那道门拆了。

// PublicationDraftSubmissionIntake 把一次已认证的接入请求翻译成录入命令：载荷 + 信封里的租户与录入者。
type PublicationDraftSubmissionIntake interface {
	IntakePublicationDraftSubmission(
		ctx context.Context,
		request *http.Request,
	) (application.SubmitPublicationDraftCommand, error)
}

// PublicationDraftApprovalIntake 把一次已认证的接入请求翻译成批准命令：载体引用 + 信封里的租户与批准者主体
// （引用 + 授予集）。
type PublicationDraftApprovalIntake interface {
	IntakePublicationDraftApproval(
		ctx context.Context,
		request *http.Request,
	) (application.ApprovePublicationDraftCommand, error)
}

// PublicationDraftPublicationIntake 把一次已认证的接入请求翻译成发布命令：载体引用 + 信封里的租户。
type PublicationDraftPublicationIntake interface {
	IntakePublicationDraftPublication(
		ctx context.Context,
		request *http.Request,
	) (application.PublishPublicationDraftCommand, error)
}

// PublicationDraftOperator 是三口转交的编排：一族一个契约，方法各自具名。事务边界在编排侧给出（录入 / 批准 /
// 发布各一笔，发布那一笔与受控发布用例同事务），适配器只转交与映射。
type PublicationDraftOperator interface {
	Submit(ctx context.Context, command application.SubmitPublicationDraftCommand) (application.SubmitPublicationDraftResult, error)
	Approve(ctx context.Context, command application.ApprovePublicationDraftCommand) (application.ApprovePublicationDraftResult, error)
	Publish(ctx context.Context, command application.PublishPublicationDraftCommand) (application.PublishPublicationDraftResult, error)
}

// NewSubmitPublicationDraftEndpoint 交回录入口（POST /commercial-publication-drafts）。
func NewSubmitPublicationDraftEndpoint(intake PublicationDraftSubmissionIntake, operator PublicationDraftOperator) http.Handler {
	return newRegistrationEndpoint(intake.IntakePublicationDraftSubmission, operator.Submit, writeDraftSubmissionAnswer)
}

// NewApprovePublicationDraftEndpoint 交回批准口（POST /commercial-publication-draft-approvals）。路径按动词叫
// `-approvals`，判据同 PP 复核口叫 `-reviews`：本口的动词是批准，答案代数说的也是批准。
func NewApprovePublicationDraftEndpoint(intake PublicationDraftApprovalIntake, operator PublicationDraftOperator) http.Handler {
	return newRegistrationEndpoint(intake.IntakePublicationDraftApproval, operator.Approve, writeDraftApprovalAnswer)
}

// NewPublishPublicationDraftEndpoint 交回发布口（POST /commercial-publication-draft-publications）。它不并进
// `/commercial-publications`：那一口收的是受控批文（JSON 镜像，声明摘要与批准三格），这一口收的是载体引用；
// 两口消费的用例也不同（这一口经载体用例转交发布用例），合一口就得在 Intake 里先认形状再分派。
func NewPublishPublicationDraftEndpoint(intake PublicationDraftPublicationIntake, operator PublicationDraftOperator) http.Handler {
	return newRegistrationEndpoint(intake.IntakePublicationDraftPublication, operator.Publish, writeDraftPublicationAnswer)
}

// draftAnswer 是录入口与批准口共用的封闭响应形状：`outcome` 取应用结果枚举原名；载体在场时带它的规范化版本、
// 摘要与状态（表单据此呈现「服务端算出的摘要」——它不算，只显）；`cause` 只随录入的`未受理`在场。
type draftAnswer struct {
	Outcome          string `json:"outcome"`
	Canonicalization string `json:"canonicalization,omitempty"`
	ContentDigest    string `json:"contentDigest,omitempty"`
	Status           string `json:"status,omitempty"`
	Cause            string `json:"cause,omitempty"`
}

// writeDraftSubmissionAnswer：`已录入`与`已修订`是本次落库取 201；重放、内容已固定与未受理是形成了的答案取 200
// ——判据同发布口（ADR-0022：状态码只说答案有没有形成）。
func writeDraftSubmissionAnswer(response http.ResponseWriter, result application.SubmitPublicationDraftResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := draftAnswer{Outcome: name}
	if cause := result.RefusalCause(); cause != nil {
		answer.Cause = cause.Error()
	}
	if draft, ok := result.Draft(); ok {
		answer.Canonicalization = draft.Canonical().Canonicalization()
		answer.ContentDigest = draft.Canonical().Digest().String()
		answer.Status = draft.Status().String()
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.PublicationDraftSubmitted, application.PublicationDraftRevised:
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}

// writeDraftApprovalAnswer：批准是往载体上推进一格，`已批准`取 200 而不是 201——没有新资源形成，改的是载体的
// 状态。其余各格（未配置、需换人、不合格、已批准、已发布、已被替换、未找到）都是形成了的答案，同取 200。
func writeDraftApprovalAnswer(response http.ResponseWriter, result application.ApprovePublicationDraftResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := draftAnswer{Outcome: name}
	if draft, ok := result.Draft(); ok {
		answer.Canonicalization = draft.Canonical().Canonicalization()
		answer.ContentDigest = draft.Canonical().Digest().String()
		answer.Status = draft.Status().String()
	}
	writeJSON(response, http.StatusOK, answer)
}

// draftPublicationAnswer 是发布口的封闭响应形状：载体侧的 `outcome`，加受控发布用例的整份答案（`publication`）
// ——它只在载体真交给了发布用例时在场（`已发布`与`发布未落定`两格），形状与 /commercial-publications 的答案
// 同一个 publicationAnswer：同一个用例的答案在两口不换形。
type draftPublicationAnswer struct {
	Outcome     string             `json:"outcome"`
	Publication *publicationAnswer `json:"publication,omitempty"`
}

// writeDraftPublicationAnswer：`已发布`取 201——受控发布落库形成了版本；其余取 200。
func writeDraftPublicationAnswer(response http.ResponseWriter, result application.PublishPublicationDraftResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := draftPublicationAnswer{Outcome: name}
	if publication, ok := result.Publication(); ok {
		body, err := publicationAnswerOf(publication)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer.Publication = &body
	}
	status := http.StatusOK
	if result.Outcome() == application.PublicationDraftPublished {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
