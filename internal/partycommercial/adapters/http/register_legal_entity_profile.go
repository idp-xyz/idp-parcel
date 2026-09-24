package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 法人资料的在线登记口（ADR-0145 决定三；登记写面通例同 ADR-0085 Decision 一），与受控 CLI 的
// register-legal-entity-profiles 同源。资料没有停用这一步，只有登记修订一个端点：新修订是往修订链上插一笔，
// 不是改旧行，所以没有 PATCH/DELETE，方法门只放 POST。

// LegalEntityProfileRegistrationIntake 把一次已认证的接入请求翻译成资料修订登记命令。它是接口而不是本包内的
// 解析代码，理由同 RegistrationNumberTypeRegistrationIntake：操作者认证属渠道接入契约，未决期间装配点挂
// UnconfiguredIntake，未配置即拒。
type LegalEntityProfileRegistrationIntake interface {
	IntakeLegalEntityProfileRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterLegalEntityProfileCommand, error)
}

// LegalEntityProfileRegistrar 是端点转交的登记编排。事务边界在编排侧给出，适配器只转交与映射。
type LegalEntityProfileRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterLegalEntityProfileCommand,
	) (application.LegalEntityProfileResult, error)
}

// 编译期锁缝：本端点不新造登记语义，只消费 application 里那一个登记用例。
var _ LegalEntityProfileRegistrar = (*application.RegisterLegalEntityProfileHandler)(nil)

// NewRegisterLegalEntityProfileEndpoint 交回资料修订登记的 HTTP 入口。
func NewRegisterLegalEntityProfileEndpoint(
	intake LegalEntityProfileRegistrationIntake,
	registrar LegalEntityProfileRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeLegalEntityProfileRegistration,
		registrar.Register,
		writeLegalEntityProfileAnswer,
	)
}

// writeLegalEntityProfileAnswer 把资料登记用例的答案逐名转写，判据同 writeRegistrationNumberTypeAnswer：`已登记`
// 取 201，其余取 200；散文原因随 `cause` 过线。答案代数是另一个类型，所以另写一份。
func writeLegalEntityProfileAnswer(
	response http.ResponseWriter,
	result application.LegalEntityProfileResult,
) {
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
	if outcome == application.LegalEntityProfileRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
