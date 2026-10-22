package shipmenthttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// codeRequestNotYetAccepted：资料修订指名的委托还没接受。它与 `outcome` 里的`请求未受理`不是一回事：这里没有业务答案，只说
// 这条命令对一份尚未接受的委托不成立，决定前的纠错走受控补充。状态与改载荷那一格同为 400，两格靠码分开——ADR-0022 否决的
// 正是按语义逐例挑状态码（409 / 422）那条路。
const codeRequestNotYetAccepted = "SHIPMENT_REQUEST_NOT_YET_ACCEPTED"

// writeCommandProblem 答指名一份委托的命令编排交回的错（ADR-0022）。委托看不见答 404，码与委托查阅同一枚：`统一不可见结果`
// 在本上下文只有一个传输层写法，404 本身就是「不存在」与「属于别人」的同答，不带 detail。只认那一个具名错误——别的错误里
// 碰巧也说了「not found」不算；其余一律是没形成答案。
func writeCommandProblem(response http.ResponseWriter, err error) {
	answerCommandFailure(legacyProblems(response), err)
}

// answerCommandFailure 是 writeCommandProblem 的映射本身，客户面命令口以问题详情的写法调它。
func answerCommandFailure(problem problemWriter, err error) {
	if errors.Is(err, application.ErrShipmentRequestNotVisible) {
		problem(http.StatusNotFound, codeRequestNotVisible, "")
		return
	}
	problem(http.StatusInternalServerError, codeNoAnswerFormed, "")
}

// answerAmendmentFailure 先答资料修订受理阶段的三种调用方的错，其余交 answerCommandFailure。意图与内容两格改请求才会好，带
// 只谈调用方自己载荷的 detail（ADR-0140 决定三）；委托未接受要换一条路而不是改这一份，同答 400 而码不同、不带 detail。
func answerAmendmentFailure(problem problemWriter, err error) {
	switch {
	case errors.Is(err, application.ErrAmendmentIntentNotDeclared):
		problem(http.StatusBadRequest, codeMalformedRequest, "intent: the amendment intent is not declared")
	case errors.Is(err, application.ErrAmendmentContentUnfit):
		problem(http.StatusBadRequest, codeMalformedRequest, "content: does not fit the declared scope and intent")
	case errors.Is(err, application.ErrAmendedRequestNotAccepted):
		problem(http.StatusBadRequest, codeRequestNotYetAccepted, "")
	default:
		answerCommandFailure(problem, err)
	}
}
