package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// GoodsRegistrar 是商品代录口转交的登记编排。事务边界在编排侧给出。
type GoodsRegistrar interface {
	Register(ctx context.Context, command application.RegisterGoodsCommand) (application.RegisterGoodsResult, error)
}

var _ GoodsRegistrar = (*application.RegisterGoodsHandler)(nil)

type registerGoodsAnswer struct {
	Outcome string          `json:"outcome"`
	Field   string          `json:"field,omitempty"`
	Fields  []string        `json:"fields,omitempty"`
	Current *goodsBasisBody `json:"current,omitempty"`
	Goods   *goodsBody      `json:"goods,omitempty"`
}

type goodsBasisBody struct {
	Registered bool `json:"registered"`
	Revision   *int `json:"revision,omitempty"`
}

// NewRegisterGoodsOnBehalfEndpoint 交回商品代录的 HTTP 入口（POST /warehouse-fulfillment-goods-on-behalf）。
// 新建答 201，其余业务答复答 200，判别只在 outcome。货主客户账户在本租户查不到答 404，与越权探针同答，不带 outcome。
// 没形成答案答 500。日志不带依据引用。
func NewRegisterGoodsOnBehalfEndpoint(intake GoodsOnBehalfIntake, registrar GoodsRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeGoodsOnBehalf(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := registrar.Register(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			slog.ErrorContext(request.Context(), "goods registration on behalf did not form an answer",
				"ownerAccount", command.Owner.String(), "goodsCode", command.Code.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeGoodsAnswer(response, result)
	})
}

func writeGoodsAnswer(response http.ResponseWriter, result application.RegisterGoodsResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := registerGoodsAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.GoodsOutcomeRegistered, application.GoodsOutcomeExistingResult:
		revision, present := result.Revision()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := goodsBodyOfRevision(revision)
		answer.Goods = &body
		if result.Outcome() == application.GoodsOutcomeRegistered {
			status = http.StatusCreated
		}
	case application.GoodsOutcomeBasisStale:
		current := result.CurrentBasis()
		basis := goodsBasisBody{Registered: current.Registered()}
		if current.Registered() {
			revision := current.Revision()
			basis.Revision = &revision
		}
		answer.Current = &basis
	case application.GoodsOutcomeImmutableField:
		answer.Field = result.ImmutableCell().String()
	case application.GoodsOutcomeIdentifierConflict:
		code, present := result.ConflictingCode()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Field = code.String()
	case application.GoodsOutcomeContentInvalid:
		cells := result.InvalidCells()
		answer.Fields = make([]string, 0, len(cells))
		for _, cell := range cells {
			answer.Fields = append(answer.Fields, cell.String())
		}
	}
	writeJSON(response, status, answer)
}
