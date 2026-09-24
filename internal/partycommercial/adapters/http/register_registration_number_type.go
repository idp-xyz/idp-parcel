package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 注册号类型目录的在线登记口（ADR-0145 决定一；登记写面通例同 ADR-0085 Decision 一），与受控 CLI
// 的 register-registration-number-types 同源。登记修订与停用各一个端点：停用是往修订链上插一笔
// 新修订，不是改旧行，所以没有 PATCH/DELETE，两口方法门都只放 POST。

// RegistrationNumberTypeRegistrationIntake 把一次已认证的接入请求翻译成类型修订登记命令。
//
// 两个 Intake 都是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：操作者认证
// 属渠道接入契约，`PAR-INT-01` 待提供；采信请求自称的 tenantId 会穿透 ADR-0003 的隔离边界。未决
// 期间本包不带任何实现，装配点挂 UnconfiguredIntake。
type RegistrationNumberTypeRegistrationIntake interface {
	IntakeRegistrationNumberTypeRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterRegistrationNumberTypeCommand, error)
}

// RegistrationNumberTypeDeactivationIntake 同上，翻译类型停用。
type RegistrationNumberTypeDeactivationIntake interface {
	IntakeRegistrationNumberTypeDeactivation(
		ctx context.Context,
		request *http.Request,
	) (application.DeactivateRegistrationNumberTypeCommand, error)
}

// RegistrationNumberTypeRegistrar 是两个端点转交的登记编排。事务边界在编排侧给出（登记写口无环境
// 事务即拒），适配器只转交与映射，不判断任何业务结果。
type RegistrationNumberTypeRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterRegistrationNumberTypeCommand,
	) (application.RegistrationNumberTypeResult, error)
	Deactivate(
		ctx context.Context,
		command application.DeactivateRegistrationNumberTypeCommand,
	) (application.RegistrationNumberTypeResult, error)
}

// 编译期锁缝：本端点族不新造登记语义，只消费 application 里那一个登记用例。
var _ RegistrationNumberTypeRegistrar = (*application.RegisterRegistrationNumberTypeHandler)(nil)

// NewRegisterRegistrationNumberTypeEndpoint 交回类型修订登记的 HTTP 入口：修订 1 是登记，其后是
// 内容更正；改层不受理由用例侧把门。
func NewRegisterRegistrationNumberTypeEndpoint(
	intake RegistrationNumberTypeRegistrationIntake,
	registrar RegistrationNumberTypeRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeRegistrationNumberTypeRegistration,
		registrar.Register,
		writeRegistrationNumberTypeAnswer,
	)
}

// NewDeactivateRegistrationNumberTypeEndpoint 交回类型停用的 HTTP 入口。
func NewDeactivateRegistrationNumberTypeEndpoint(
	intake RegistrationNumberTypeDeactivationIntake,
	registrar RegistrationNumberTypeRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeRegistrationNumberTypeDeactivation,
		registrar.Deactivate,
		writeRegistrationNumberTypeAnswer,
	)
}

// writeRegistrationNumberTypeAnswer 把目录登记用例的答案逐名转写，判据与身份族 writePartyRegistryAnswer
// 相同：`已登记`与`已停用`取 201，其余取 200（`未找到`同样不取 404——能力在、册也在，只是册上没有
// 这一个类型）；散文原因随 `cause` 过线，具名格原名过线。两族各写一份而不共用：答案代数是两个
// 类型，共用就得在转写里判一个对方永远取不到的值。
func writeRegistrationNumberTypeAnswer(
	response http.ResponseWriter,
	result application.RegistrationNumberTypeResult,
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
	switch outcome {
	case application.RegistrationNumberTypeRegistered, application.RegistrationNumberTypeDeactivated:
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
