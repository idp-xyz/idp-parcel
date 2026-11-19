package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// PackagingMaterialRegistrar 是包材登记口转交的登记编排，理由同 WarehouseRegistrar：事务边界在编排侧给出，适配器只转交与映射，不判断任何业务结果。
type PackagingMaterialRegistrar interface {
	Register(ctx context.Context, command application.RegisterPackagingMaterialCommand) (application.RegisterPackagingMaterialResult, error)
}

// 编译期锁缝：本端点不新造登记语义，只消费 application 里那一个登记用例。
var _ PackagingMaterialRegistrar = (*application.RegisterPackagingMaterialHandler)(nil)

// registerPackagingMaterialAnswer 是包材登记口的答复体：各册共用的答复头（registrationAnswerHead）加 `packagingMaterial`，后者只随 REGISTERED 与
// ALREADY_REGISTERED，回显在册的那一笔（含登记责任方与登记时刻）。
type registerPackagingMaterialAnswer struct {
	registrationAnswerHead
	PackagingMaterial *packagingMaterialBody `json:"packagingMaterial,omitempty"`
}

// NewRegisterPackagingMaterialEndpoint 交回包材修订登记的 HTTP 入口（POST /warehouse-fulfillment-packaging-material-registrations）：修订 1 是
// 首次登记，其后是改名称、改尺寸与重量、停用与再启用，各占下一个修订号，所以没有 PATCH 与 DELETE。状态码与答复体的形状同仓的登记口（ADR-0022）：
// 新建答 201，其余业务答复（含本租户一个仓都没登记时的 NOT_CONFIGURED、数值为零或单位不合的 NOT_ACCEPTED/CONTENT_INVALID）答 200，业务判别只在
// `outcome`；没形成答案（编排交回 error）答 500 NO_ANSWER_FORMED。登记口不判准入范围：登记一个修订不形成生产事实（ADR-0100 决定四）。
func NewRegisterPackagingMaterialEndpoint(intake PackagingMaterialRegistrationIntake, registrar PackagingMaterialRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}

		command, err := intake.IntakePackagingMaterialRegistration(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案。成因不外泄，日志只带编码与修订号：错误原文里可能带着在途那一行的内容
			// （UC-WF-001「权限、隔离与审计」）。
			slog.ErrorContext(request.Context(), "packaging material registration did not form an answer",
				"packagingMaterialCode", command.Code.String(), "revision", command.Revision)
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, result, func(head registrationAnswerHead, echoed *domain.PackagingMaterialRevision) any {
			answer := registerPackagingMaterialAnswer{registrationAnswerHead: head}
			if echoed != nil {
				body := packagingMaterialBodyOfRevision(*echoed)
				answer.PackagingMaterial = &body
			}
			return answer
		})
	})
}
