package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 参与方资料的在线登记口（ADR-0369 决定一；登记写面通例同 ADR-0085 Decision 一）。资料没有停用这一步，只有登记修订一个端点，
// 方法门只放 POST；判据与法人资料口一致。

// BusinessPartyProfileRegistrationIntake 把一次已认证的接入请求翻译成参与方资料修订登记命令；租户格只能来自认证结果。
type BusinessPartyProfileRegistrationIntake interface {
	IntakeBusinessPartyProfileRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterBusinessPartyProfileCommand, error)
}

// BusinessPartyProfileRegistrar 是端点转交的登记编排。事务边界在编排侧给出。
type BusinessPartyProfileRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterBusinessPartyProfileCommand,
	) (application.BusinessPartyProfileResult, error)
}

var _ BusinessPartyProfileRegistrar = (*application.RegisterBusinessPartyProfileHandler)(nil)

// NewRegisterBusinessPartyProfileEndpoint 交回参与方资料修订登记的 HTTP 入口（POST /commercial-business-party-profile-registrations）。
func NewRegisterBusinessPartyProfileEndpoint(
	intake BusinessPartyProfileRegistrationIntake,
	registrar BusinessPartyProfileRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeBusinessPartyProfileRegistration,
		registrar.Register,
		writeBusinessPartyProfileAnswer,
	)
}

// writeBusinessPartyProfileAnswer 逐名转写答案：`已登记`取 201，其余取 200；散文原因随 `cause` 过线。
func writeBusinessPartyProfileAnswer(response http.ResponseWriter, result application.BusinessPartyProfileResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := registrationAnswer{Outcome: name}
	if cause := result.Cause(); cause != nil {
		answer.Cause = cause.Error()
	}
	status := http.StatusOK
	if outcome == application.BusinessPartyProfileRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
