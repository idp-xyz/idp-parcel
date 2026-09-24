package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// ReferenceSeriesRegistrationIntake 把一次已认证的接入请求翻译成序列登记命令。
// 接口而非实现的理由同 PriceCardRegistrationIntake：翻译与操作者认证归操作者渠道
// （ADR-0100，真 Intake 未就位），重建门在领域侧（RehydrateReferenceSeriesRegistration）。
type ReferenceSeriesRegistrationIntake interface {
	IntakeReferenceSeriesRegistration(ctx context.Context, request *http.Request) (application.RegisterReferenceSeriesCommand, error)
}

// ReferenceSeriesRegistrar 是本端点转交的登记编排，事务边界在编排侧给出。
type ReferenceSeriesRegistrar interface {
	Handle(ctx context.Context, command application.RegisterReferenceSeriesCommand) (application.RegisterReferenceSeriesOutcome, error)
}

// NewRegisterReferenceSeriesEndpoint 交回参考系列登记的 HTTP 入口（ADR-0085
// Decision 一；两类登记各立端点，与其 CLI 命令一一对应，不合并成一个「登记」口——
// 合并会让装配点看起来能只配一半）。
func NewRegisterReferenceSeriesEndpoint(intake ReferenceSeriesRegistrationIntake, registrar ReferenceSeriesRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeReferenceSeriesRegistration(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		outcome, err := registrar.Handle(request.Context(), command)
		if err != nil {
			// 判据同价卡登记口：依赖故障不是业务答案（ADR-0022）。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, outcome.String(), outcome == application.ReferenceSeriesRecorded)
	})
}
