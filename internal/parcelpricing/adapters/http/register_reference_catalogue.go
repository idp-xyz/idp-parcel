package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// ReferenceCatalogueRegistrationIntake 把一次已认证的接入请求翻译成目录登记命令。接口而非实现的理由与
// 复核口同源：登记责任方的身份来自 ADR-0100 的操作者信封，不从载荷里取；载荷形状由产品定义（模板导入，
// ADR-0101 决定八），解码在 DecodeReferenceCataloguePayload。
type ReferenceCatalogueRegistrationIntake interface {
	IntakeReferenceCatalogueRegistration(ctx context.Context, request *http.Request) (application.RegisterReferenceCatalogueCommand, error)
}

// ReferenceCatalogueRegistrar 是本端点转交的登记编排，事务边界在编排侧给出。
type ReferenceCatalogueRegistrar interface {
	Handle(ctx context.Context, command application.RegisterReferenceCatalogueCommand) (application.RegisterReferenceCatalogueOutcome, error)
}

// NewRegisterReferenceCatalogueEndpoint 交回计价参考目录登记的 HTTP 入口
// （POST /pricing-reference-catalogue-registrations；ADR-0085 决定一、ADR-0109 Decision 二）。
// 不与序列登记合口：两种命令、两套答案代数，合口后装配点可以只配一半而编译仍绿。
func NewRegisterReferenceCatalogueEndpoint(intake ReferenceCatalogueRegistrationIntake, registrar ReferenceCatalogueRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeReferenceCatalogueRegistration(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		outcome, err := registrar.Handle(request.Context(), command)
		if err != nil {
			// 依赖故障不是业务答案（ADR-0022）。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, outcome.String(), outcome == application.ReferenceCatalogueRecorded)
	})
}
