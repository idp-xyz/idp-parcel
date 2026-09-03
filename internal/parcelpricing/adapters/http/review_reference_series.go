package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// ReferenceSeriesReviewIntake 把一次已认证的接入请求翻译成序列版本复核命令。
//
// 接口而非实现的理由同两个登记 Intake：翻译与操作者认证属渠道接入契约（`PAR-INT-01` 待
// 提供）。**复核这一口尤其不能带「开发用」实现**——复核责任方是四眼门的一半（领域拒绝
// 复核责任方等于登记责任方），从请求内容里铸一个出来就等于把那道门拆了。
type ReferenceSeriesReviewIntake interface {
	IntakeReferenceSeriesReview(ctx context.Context, request *http.Request) (application.ReviewReferenceSeriesCommand, error)
}

// ReferenceSeriesReviewer 是本端点转交的复核编排，事务边界在编排侧给出。
type ReferenceSeriesReviewer interface {
	Handle(ctx context.Context, command application.ReviewReferenceSeriesCommand) (application.ReviewReferenceSeriesOutcome, error)
}

// NewReviewReferenceSeriesEndpoint 交回序列版本复核的 HTTP 入口
// （POST /pricing-reference-series-reviews；ADR-0085 决定一、ADR-0099 决定二）。
//
// 复核不与登记合成一个口：它是往复核册上追加一条治理动作，与登记一版取值是两种命令、
// 两套答案代数，而**四眼门只在复核那一侧**。合成一口之后装配点可以只配一半而编译仍绿。
//
// 路径不带 `-registrations` 后缀而叫 `-reviews`：本上下文这一格的动词是复核，答案代数说的
// 也是复核（`已追加`/`需换人复核`/`版本不在册`）；叫成登记会与序列登记那一族的答案混为一谈。
func NewReviewReferenceSeriesEndpoint(intake ReferenceSeriesReviewIntake, reviewer ReferenceSeriesReviewer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeReferenceSeriesReview(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		outcome, err := reviewer.Handle(request.Context(), command)
		if err != nil {
			// 依赖故障不是业务答案（ADR-0022）。复核用例把`未决`连同错误一起交回，
			// 这里只认错误——「记没记上未知」不该带着一个业务 outcome 上线。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		// `已追加`是新增一行，取 201；其余（幂等重放、冲突、版本不在册、需换人复核、
		// 未受理）都是形成了的答案，取 200——状态码不替 outcome 说话。
		writeRegistrationAnswer(response, outcome.String(), outcome == application.SeriesReviewRecorded)
	})
}
