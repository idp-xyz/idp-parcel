package warehousehttp

import (
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// registrationAnswerHead 是各册登记口答复体的头四格：`outcome` 取应用结果枚举原名，业务判别只看它：重放与内容冲突是治理答案
// 不是失败，传输层不合并、不改名。`refusal` 只随 NOT_ACCEPTED，`field` 只随 IMMUTABLE_FIELD、CONTENT_INVALID 与
// REFERENCE_NOT_FOUND，`latestRevision` 只随 REVISION_OUT_OF_SEQUENCE——册上没有这个对象时是 0，所以用指针：0 要出现，别的结果上
// 整键缺席。回显在册那一笔的那一格各册键名不同（仓是 `warehouse`，库区是 `zone`），由各册的答复体接在头后面。
type registrationAnswerHead struct {
	Outcome        string `json:"outcome"`
	Refusal        string `json:"refusal,omitempty"`
	Field          string `json:"field,omitempty"`
	LatestRevision *int   `json:"latestRevision,omitempty"`
}

// writeRegistrationAnswer 按结果代数答各册登记口：新建了对象（REGISTERED）答 201，其余业务答复（重放、内容冲突、未受理）答 200，
// 业务判别只在 `outcome`（ADR-0022）。落册或在册的结果一定带着那一笔，没带是编排与传输层对结果的理解漂开了，不能答一个空的成功；
// 没有名字的结果不能带空 `outcome` 上线。answerOf 拿到答复头与（有回显时）在册那一笔，拼成这一册的答复体。
func writeRegistrationAnswer[Revision any, Field fmt.Stringer](
	response http.ResponseWriter,
	result application.RegistrationResult[Revision, Field],
	answerOf func(head registrationAnswerHead, echoed *Revision) any,
) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	head := registrationAnswerHead{Outcome: name}
	status := http.StatusOK
	var echoed *Revision

	switch outcome {
	case application.RegistrationRegistered, application.RegistrationAlreadyRegistered:
		revision, present := result.Revision()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		echoed = &revision
		if outcome == application.RegistrationRegistered {
			status = http.StatusCreated
		}
	case application.RegistrationNotAccepted:
		refusal := result.Refusal()
		if refusal.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		head.Refusal = refusal.String()
		switch refusal {
		case application.RegistrationRefusalImmutableField,
			application.RegistrationRefusalContentInvalid,
			application.RegistrationRefusalReferenceNotFound:
			head.Field = result.Field().String()
		case application.RegistrationRefusalRevisionOutOfSequence:
			latest := result.LatestRevision()
			head.LatestRevision = &latest
		}
	}
	writeJSON(response, status, answerOf(head, echoed))
}
