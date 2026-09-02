package customshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 关务四类配置登记的在线登记口（ADR-0085 Decision 一；票 admin-write-faces/02 的关务
// 片按其范围裁定收敛为解释规则、门禁目录、候选口岸、申报路径）。同一受控 CLI 里那些
// 改「案上此刻的事实」的命令不在本端点族内，判据在那张票上，此处不复述。
//
// 四类分属两个登记用例 handler（案件配置面与口岸路径面），却交回同一套答案代数
// CaseConfigurationOutcome——端点体因此共用一份而命令类型逐类分开，见
// newConfigurationRegistrationEndpoint。
//
// 畸形请求不另立哨兵，复用本包的 ErrMalformedRequest：它在本包已由外部结果命令面与
// 目录查阅面共用，说的就是「这次请求构造不出命令且重发不会变」，登记面等的是同一件事。
// 分流助手则单立一份（见 writeRegistrationIntakeProblem），两者不矛盾——将来要加的
// 载荷规范化摘要一格（ADR-0055 Decision 五）落在命令面的分流里，不是落在哨兵上。

// InterpretationRuleRegistrationIntake 把一次已认证的接入请求翻译成解释规则登记命令。
//
// 四个 Intake 都是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：
// 登记快照本体的译装已在 registrationjson 落定一份（受控 CLI 的 -input 与本口同源），
// 但「渠道原始载荷 → 登记快照」的边界与操作者认证属渠道接入契约，`PAR-INT-01` 待提供；
// 采信报文自称的租户会穿透 ADR-0003 的隔离边界。未决期间本包不带任何实现，包括
// 「开发用」的采信头部版本。
//
// 逐类分设接口而不合成一个按种类分派的口子，判据同登记 CLI 的命令族：四类的命令类型
// 互不相同，合成一个就得在 Intake 里先认种类再定形状，装配点从此可以把一类的译装接到
// 另一类的端点上而编译仍绿。查阅面 /customs-ports-paths 折成 ?registry= 一口不构成
// 先例——那里两册共用同一个作用域与同一个读口，写面每一类各有自己的命令与登记册。
type InterpretationRuleRegistrationIntake interface {
	IntakeInterpretationRuleRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterInterpretationRuleCommand, error)
}

// GateCatalogRegistrationIntake 同上，翻译门禁前置条件目录登记。
type GateCatalogRegistrationIntake interface {
	IntakeGateCatalogRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterGateCatalogCommand, error)
}

// CandidatePortRegistrationIntake 同上，翻译口岸合规候选版本登记。
type CandidatePortRegistrationIntake interface {
	IntakeCandidatePortRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterCandidatePortCommand, error)
}

// DeclarationPathRegistrationIntake 同上，翻译申报路径版本登记。
type DeclarationPathRegistrationIntake interface {
	IntakeDeclarationPathRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterDeclarationPathCommand, error)
}

// InterpretationRuleRegistrar 是本端点转交的登记编排。事务边界在编排之外给出（登记
// 写口无环境事务即拒，形照登记 CLI 的 execute：一次调用一笔事务，登记与它的冲突判定
// 读回因此看同一份快照），适配器只转交与映射，不判断任何业务结果。
type InterpretationRuleRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterInterpretationRuleCommand,
	) (application.CaseConfigurationOutcome, error)
}

// GateCatalogRegistrar 同上，转交门禁目录登记编排。
type GateCatalogRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterGateCatalogCommand,
	) (application.CaseConfigurationOutcome, error)
}

// CandidatePortRegistrar 同上，转交口岸目录登记编排。
type CandidatePortRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterCandidatePortCommand,
	) (application.CaseConfigurationOutcome, error)
}

// DeclarationPathRegistrar 同上，转交申报路径登记编排。
type DeclarationPathRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterDeclarationPathCommand,
	) (application.CaseConfigurationOutcome, error)
}

// NewRegisterInterpretationRuleEndpoint 交回解释规则版本登记的 HTTP 入口（ADR-0085
// Decision 一：登记写面进端点表带未配置格；在线口与登记 CLI 消费同一登记用例，答案
// 代数一致）。它登的是「这个租户在某辖区、某结果层自某法定起点采用哪一版解释规则」，
// 终点不是输入——换版走登记一个更晚起点的新版本（ADR-0070）。
func NewRegisterInterpretationRuleEndpoint(
	intake InterpretationRuleRegistrationIntake,
	registrar InterpretationRuleRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeInterpretationRuleRegistration, registrar.Handle)
}

// NewRegisterGateCatalogEndpoint 交回门禁前置条件目录登记的 HTTP 入口。
//
// 它登的是某（范围，动作，边界）这本前置条件目录在场，没有可比内容——「目录登了而
// 清单空」是必须登得出来的一格，含义与「未登记」相反（后者是未决）。目录里的逐项判断
// 是另一个命令，不在本端点族内。
func NewRegisterGateCatalogEndpoint(
	intake GateCatalogRegistrationIntake,
	registrar GateCatalogRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeGateCatalogRegistration, registrar.Handle)
}

// NewRegisterCandidatePortEndpoint 交回口岸合规候选版本登记的 HTTP 入口。它登的是
// 「这个租户自某起点把某口岸视为合规候选」，区间约定同解释规则。
func NewRegisterCandidatePortEndpoint(
	intake CandidatePortRegistrationIntake,
	registrar CandidatePortRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeCandidatePortRegistration, registrar.Handle)
}

// NewRegisterDeclarationPathEndpoint 交回申报路径版本登记的 HTTP 入口。它登的是某
// 申报路径自某起点的三维内容（口岸、方向、申报模式），区间约定同上。
func NewRegisterDeclarationPathEndpoint(
	intake DeclarationPathRegistrationIntake,
	registrar DeclarationPathRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeDeclarationPathRegistration, registrar.Handle)
}

// newConfigurationRegistrationEndpoint 是四类共用的端点体。
//
// 共用一份而不是各抄一遍：四类交回同一个 CaseConfigurationOutcome，方法门、Intake
// 分流与答案转写因而逐字相同，抄四遍等于把同一条转写规则摊到四处，收紧时改一处漏
// 三处。类型仍逐类分开——泛型按命令类型实例化后互不相容，把一类的编排接到另一类的
// 端点上编译期就红。
func newConfigurationRegistrationEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	register func(context.Context, Command) (application.CaseConfigurationOutcome, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		outcome, err := register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案：状态码只报「没形成答案」
			// （ADR-0022），调用方据以重试或转人工，成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeConfigurationRegistrationAnswer(response, outcome)
	})
}

// configurationRegistrationResponse 是四个登记端点的封闭响应形状。`outcome` 取应用
// 结果枚举原名——与登记 CLI 的答案代数同源：幂等重放与内容冲突是治理答案不是失败，
// 传输层不合并、不改名，也没有「其他」这一格。
type configurationRegistrationResponse struct {
	Outcome string `json:"outcome"`
}

// writeRegistrationIntakeProblem 与查阅侧的分流判据相同（403 未配置 / 400 畸形 /
// 5xx Intake 故障），单立一份不复用查阅侧助手：两侧各随自己的端点族演化，命令面的
// 分流将来要加载荷规范化摘要一格（ADR-0055 Decision 五），不该牵动查阅行。
func writeRegistrationIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// writeConfigurationRegistrationAnswer 把用例的登记册答案逐名转写，两格例外各有判据。
//
// `UNDECIDED` 不作为业务答案上线：本上下文的登记用例把依赖故障折成这一格**值**而不是
// error，可它说的正是「登记与否未知」——按 ADR-0022 那是没有形成答案，与编排交回
// error 落在同一格，登记 CLI 的退出码也正是这么并的（两条路同为未决那一个码）。折成
// 200 会让它挤进「登记册治理答案」那一栏，而治理答案的续办是改内容、重试没有用，未决
// 的续办恰恰是重跑同一份。
//
// 受理拒绝与内容冲突走 200 而不是 4xx：它们是登记册对这次登记作出的判断，与「请求根本
// 构造不出命令」的恢复动作不同——前者要登记方改内容或换生效起点，后者要接入方改请求
// 形状，压成一格登记方就不知道该改哪一样。
//
// 其余具名格一律原名过线，不逐格枚举：撤销那几格属就绪与授权两条撤销命令，本端点族
// 不接，写出来就是一段走不到的分派；而枚举写死之后，用例日后多一格会掉进「不认识」的
// 兜底，原名过线对新格仍然诚实（管理台对未收录的 outcome 按原名示出，不归进某个既有
// 中文说法）。
func writeConfigurationRegistrationAnswer(
	response http.ResponseWriter,
	outcome application.CaseConfigurationOutcome,
) {
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if outcome == application.ConfigurationUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	status := http.StatusOK
	if outcome == application.ConfigurationRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, configurationRegistrationResponse{Outcome: name})
}
