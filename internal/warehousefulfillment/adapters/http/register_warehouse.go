package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// WarehouseRegistrar 是登记口转交的登记编排。事务边界在编排侧给出（登记册的保存在没有环境事务时拒绝，
// application.RegisterWarehouseHandler 自己不开事务），适配器只转交与映射，不判断任何业务结果。
type WarehouseRegistrar interface {
	Register(ctx context.Context, command application.RegisterWarehouseCommand) (application.RegisterWarehouseResult, error)
}

// 编译期锁缝：本端点不新造登记语义，只消费 application 里那一个登记用例。
var _ WarehouseRegistrar = (*application.RegisterWarehouseHandler)(nil)

// registerAnswer 是登记口的答复体。`outcome` 取应用结果枚举原名，业务判别只看它：重放与内容冲突是治理答案不是失败，传输层
// 不合并、不改名。`refusal` 只随 NOT_ACCEPTED，`field` 只随 IMMUTABLE_FIELD 与 CONTENT_INVALID，`latestRevision` 只随
// REVISION_OUT_OF_SEQUENCE——册上没有这个仓时是 0，所以用指针：0 要出现，别的结果上整键缺席。`warehouse` 只随 REGISTERED 与
// ALREADY_REGISTERED，回显在册的那一笔（含登记责任方与登记时刻）。
type registerAnswer struct {
	Outcome        string         `json:"outcome"`
	Refusal        string         `json:"refusal,omitempty"`
	Field          string         `json:"field,omitempty"`
	LatestRevision *int           `json:"latestRevision,omitempty"`
	Warehouse      *warehouseBody `json:"warehouse,omitempty"`
}

// NewRegisterWarehouseEndpoint 交回仓修订登记的 HTTP 入口（POST /warehouse-fulfillment-warehouse-registrations）：修订 1 是
// 首次登记，其后是改资料、停用与再启用，各占下一个修订号，所以没有 PATCH 与 DELETE。
//
// 状态码照 ADR-0022：答案形成了，新建了对象（REGISTERED）答 201，其余业务答复（重放、内容冲突、未受理）答 200，业务判别只在
// `outcome`；没形成答案（编排交回 error）答 500 NO_ANSWER_FORMED，调用侧据以重试或转人工。登记口不判准入范围：登记一个修订
// 不形成生产事实（ADR-0100 决定四）。
func NewRegisterWarehouseEndpoint(intake WarehouseRegistrationIntake, registrar WarehouseRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}

		command, err := intake.IntakeWarehouseRegistration(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案。成因不外泄，日志只带仓编码与修订号：错误原文里可能带着在途那一行的内容，
			// 地址正文不进日志（UC-WF-001「权限、隔离与审计」）。
			slog.ErrorContext(request.Context(), "warehouse registration did not form an answer",
				"warehouseCode", command.Code.String(), "revision", command.Revision)
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegisterAnswer(response, result)
	})
}

func writeRegisterAnswer(response http.ResponseWriter, result application.RegisterWarehouseResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := registerAnswer{Outcome: name}
	status := http.StatusOK

	switch outcome {
	case application.WarehouseRegistered, application.WarehouseAlreadyRegistered:
		revision, present := result.Revision()
		if !present {
			// 落册或在册的结果一定带着那一笔；没带是编排与传输层对结果的理解漂开了，不能答一个空的成功。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := warehouseBodyOfRevision(revision)
		answer.Warehouse = &body
		if outcome == application.WarehouseRegistered {
			status = http.StatusCreated
		}
	case application.WarehouseNotAccepted:
		refusal := result.Refusal()
		if refusal.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer.Refusal = refusal.String()
		switch refusal {
		case application.WarehouseRefusalImmutableField, application.WarehouseRefusalContentInvalid:
			answer.Field = result.Field().String()
		case application.WarehouseRefusalRevisionOutOfSequence:
			latest := result.LatestRevision()
			answer.LatestRevision = &latest
		}
	}
	writeJSON(response, status, answer)
}
