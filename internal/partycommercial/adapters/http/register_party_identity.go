package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 参与方身份与关系的在线登记口（ADR-0085 Decision 一；票 admin-write-faces/02 商业片）：
// 四类登记加一个停用，与受控 CLI 的 register-parties / deactivate-party-identity 同源。
//
// 停用进本端点族，而关务片那两个 Revoke 被排除在它那张票之外，两者不矛盾——判据是
// 「改的是这个租户怎么配置，还是账上/案上此刻的事实」。撤销就绪与撤销提交授权改的是
// 某个案件此刻的事实；停用改的是租户身份册上这个身份的生命周期，与登记它的那笔修订
// 同一本册、同一条修订链，落库形状也是插一笔新修订而不是改旧行。票 01 的红线把「停用
// 走状态推进」与「不开任何行级 UPDATE/DELETE 面」并列写在同一句里，说的正是这一格。

// 四类登记与停用各一个 Intake 接口，与受控登记口的命令族一一对应。
//
// 它们是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：领域门与
// 引用检查在用例侧已实现，但「渠道原始载荷 → 登记快照」的翻译与操作者认证属渠道接入
// 契约，`PAR-INT-01` 待提供。这里尤其不能照抄受控登记口的译装——那一份从批文里读
// tenantId，运维在库网内手跑时那是治理动作，搬到在线口上就是采信自报租户，穿透
// ADR-0003 的隔离边界。命令的租户格与行内容格本就分开（见 application 的五个命令
// 类型），真渠道 Intake 要把认证结果填进租户格、只从载荷取行内容。未决期间本包不带
// 任何实现，包括「开发用」的采信头部版本。
//
// 不合并成一个带种类参数的接口：五类的命令类型互不相同，合并之后 Intake 得先认种类
// 再定形状，装配点从此可以把一类的译装接到另一类的端点上而编译仍绿；且「某一类还没有
// 真 Intake」在装配点是看得见的一行，合并后会变成一个类型内部的分支。
type BusinessPartyRegistrationIntake interface {
	IntakeBusinessPartyRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterBusinessPartyCommand, error)
}

// LegalEntityRegistrationIntake 同上，翻译责任法人身份修订登记。
type LegalEntityRegistrationIntake interface {
	IntakeLegalEntityRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterLegalEntityCommand, error)
}

// CustomerAccountRegistrationIntake 同上，翻译货主客户账户修订登记。
type CustomerAccountRegistrationIntake interface {
	IntakeCustomerAccountRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterCustomerAccountCommand, error)
}

// PartyRelationshipRegistrationIntake 同上，翻译参与方关系修订登记。
type PartyRelationshipRegistrationIntake interface {
	IntakePartyRelationshipRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterPartyRelationshipCommand, error)
}

// PartyIdentityDeactivationIntake 同上，翻译身份停用。命令自带身份种类（封闭集的名称镜像，
// 关系不在内——关系的终止走撤销/到期/替代，不叫停用），因此各册共一个口。
type PartyIdentityDeactivationIntake interface {
	IntakePartyIdentityDeactivation(
		ctx context.Context,
		request *http.Request,
	) (application.DeactivatePartyIdentityCommand, error)
}

// PartyIdentityRegistrar 是五个端点转交的登记编排。五族共一个接口而 Intake 分五个，
// 因为两侧被谁实现不同：Intake 的实现方是渠道契约，逐类到位；编排的实现方是登记用例
// 本身，它对五类是同一个对象、同一套答案代数（`PartyRegistryResult`）。
//
// 事务边界在编排侧给出（登记写口无环境事务即拒，形照登记 CLI 的 execute：一次调用一笔
// 事务，登记与它的修订连续性读回因此看同一份快照），适配器只转交与映射，不判断任何
// 业务结果。方法各自具名而不是五个同名 `Handle`：接错一格在端点构造函数那一行就读得
// 出来。
type PartyIdentityRegistrar interface {
	RegisterBusinessParty(
		ctx context.Context,
		command application.RegisterBusinessPartyCommand,
	) (application.PartyRegistryResult, error)
	RegisterLegalEntity(
		ctx context.Context,
		command application.RegisterLegalEntityCommand,
	) (application.PartyRegistryResult, error)
	RegisterCustomerAccount(
		ctx context.Context,
		command application.RegisterCustomerAccountCommand,
	) (application.PartyRegistryResult, error)
	RegisterRelationship(
		ctx context.Context,
		command application.RegisterPartyRelationshipCommand,
	) (application.PartyRegistryResult, error)
	Deactivate(
		ctx context.Context,
		command application.DeactivatePartyIdentityCommand,
	) (application.PartyRegistryResult, error)
}

// 编译期锁缝：登记编排的形状与真用例保持一致——本端点族不新造登记语义，只消费
// application 里那一个登记用例，签名漂移在编译期暴露。判据同查阅侧锁到 ports 读面。
var _ PartyIdentityRegistrar = (*application.RegisterPartyIdentityHandler)(nil)

// NewRegisterBusinessPartyEndpoint 交回业务参与方身份修订登记的 HTTP 入口（ADR-0085
// Decision 一：登记写面进端点表带未配置格；在线口与受控登记 CLI 消费同一登记用例，
// 答案代数一致）。它登的是「这个租户认下这个角色中立的参与方身份，自某时点起生效」；
// 改名或改依据翻旧插新走下一笔修订，不覆盖已登行。
func NewRegisterBusinessPartyEndpoint(
	intake BusinessPartyRegistrationIntake,
	registrar PartyIdentityRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeBusinessPartyRegistration,
		registrar.RegisterBusinessParty,
		writePartyRegistryAnswer,
	)
}

// NewRegisterLegalEntityEndpoint 交回责任法人身份修订登记的 HTTP 入口。法人钉在已登记
// 且届时已生效的参与方上，参与方内容在参与方册，不随法人重复登记（用例侧的引用检查）。
func NewRegisterLegalEntityEndpoint(
	intake LegalEntityRegistrationIntake,
	registrar PartyIdentityRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeLegalEntityRegistration,
		registrar.RegisterLegalEntity,
		writePartyRegistryAnswer,
	)
}

// NewRegisterCustomerAccountEndpoint 交回货主客户账户修订登记的 HTTP 入口，引用判据
// 同法人登记；跨租户绑定由领域构造门拒绝（ADR-0041）。
func NewRegisterCustomerAccountEndpoint(
	intake CustomerAccountRegistrationIntake,
	registrar PartyIdentityRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeCustomerAccountRegistration,
		registrar.RegisterCustomerAccount,
		writePartyRegistryAnswer,
	)
}

// NewRegisterPartyRelationshipEndpoint 交回参与方关系修订登记的 HTTP 入口。带批准事实
// 的登记在用例侧走真转换（候选 → 批准），本端点不构造任何领域对象。
func NewRegisterPartyRelationshipEndpoint(
	intake PartyRelationshipRegistrationIntake,
	registrar PartyIdentityRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakePartyRelationshipRegistration,
		registrar.RegisterRelationship,
		writePartyRegistryAnswer,
	)
}

// NewDeactivatePartyIdentityEndpoint 交回身份停用的 HTTP 入口。停用是往修订链上插一笔
// 新修订，不是改旧行——本端点族因此没有 PATCH/DELETE，方法门只放 POST。
func NewDeactivatePartyIdentityEndpoint(
	intake PartyIdentityDeactivationIntake,
	registrar PartyIdentityRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakePartyIdentityDeactivation,
		registrar.Deactivate,
		writePartyRegistryAnswer,
	)
}

// writePartyRegistryAnswer 把身份登记用例的答案逐名转写。
//
// `已登记`与`已停用`取 201，其余取 200：前两格是本次落库，其余是登记册对这次登记作出
// 的判断。治理答案走 200 而不是 4xx，判据同关务片——重放、内容冲突与受理门拒绝要登记
// 方对着册面修正内容，与「请求根本构造不出命令」的恢复动作不同，压成一格登记方就不
// 知道该改哪一样。
//
// `未找到`（停用一个从未登记的身份）也走 200 而不是 404：404 说的是「这个产品没有这条
// 能力」，而这里能力在、册也在，只是册上没有这一个身份——登记方要去查的是册面不是
// 路由，两者的续办动作不同。
//
// 具名格不逐个枚举，原名过线：用例日后多一格时，枚举写死的转写会把新格掉进兜底，而
// 原名过线对新格仍然诚实（管理台对未收录的 outcome 按原名示出，不归进某个既有说法）。
func writePartyRegistryAnswer(
	response http.ResponseWriter,
	result application.PartyRegistryResult,
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
	case application.PartyIdentityRegistered, application.PartyIdentityDeactivated:
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
