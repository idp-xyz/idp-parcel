package visibilityhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// ErrMalformedRegistration 表示这次请求构造不出目录登记命令，且重发同样的内容不会改变
// 结果。与查阅面的 ErrMalformedRequest、索赔提交面的 ErrMalformedClaim 分设，判据同后者
// ——各端点族的翻译各自演化，共享一个哨兵会让一边的收紧误伤另一边。
//
// 六类登记共用这一个而不再细分：它们消费同一份登记快照翻译（registrationjson），也交回
// 同一套答案代数，收紧必然同时发生在六处。
var ErrMalformedRegistration = errors.New("visibility exception http: malformed catalog registration")

// codeUnnamedRefusalReason 与 codeUnnamedOutcome 同属「应用层交回的答案没有名字」这一类
// 编程错误。目录登记面单立这一格是因为拒绝理由在这套答案代数里是答案的另一半——登记 CLI
// 的退出码正按它分治理答案与输入错误两路，一个没有名字的理由让登记方无从知道该改什么。
const codeUnnamedRefusalReason = "UNNAMED_REFUSAL_REASON"

// MilestoneMappingRegistrationIntake 把一次已认证的接入请求翻译成里程碑映射登记命令。
//
// 六个 Intake 都是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：
// 登记快照本体的翻译已在 registrationjson 落定一份，但「渠道原始载荷 → 登记快照」的边界
// 与操作者认证属渠道接入契约，`PAR-INT-01` 待提供；采信自报租户会穿透 ADR-0003 的隔离
// 边界。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
//
// 逐类分设接口而不合成一个按种类分派的口子，判据同登记 CLI 的命令族：六类的命令互不相同，
// 合成一个就得在 Intake 里先认种类再定形状，装配点从此可以把一类的翻译接到另一类的端点上
// 而编译仍绿。查阅面 /visibility-catalogues 折成 ?kind= 一口不构成先例——那里六种共用同一
// 个作用域与同一个读口，写面每一类各有自己的命令与授权对象。
type MilestoneMappingRegistrationIntake interface {
	IntakeMilestoneMappingRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterMilestoneMappingCommand, error)
}

// TriageRulesRegistrationIntake 同上，翻译分诊规则登记。
type TriageRulesRegistrationIntake interface {
	IntakeTriageRulesRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterTriageRulesCommand, error)
}

// NotificationPolicyRegistrationIntake 同上，翻译通知策略登记。
type NotificationPolicyRegistrationIntake interface {
	IntakeNotificationPolicyRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterNotificationPolicyCommand, error)
}

// ClaimEligibilityRegistrationIntake 同上，翻译索赔资格声明登记。
type ClaimEligibilityRegistrationIntake interface {
	IntakeClaimEligibilityRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterClaimEligibilityCommand, error)
}

// ClaimAuthorizationRegistrationIntake 同上，翻译申请人授权名单登记。
type ClaimAuthorizationRegistrationIntake interface {
	IntakeClaimAuthorizationRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterClaimAuthorizationCommand, error)
}

// DisclosurePolicyRegistrationIntake 同上，翻译披露策略登记。
type DisclosurePolicyRegistrationIntake interface {
	IntakeDisclosurePolicyRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterDisclosurePolicyCommand, error)
}

// MilestoneMappingRegistrar 是本端点转交的登记编排。事务边界在编排侧给出（目录写口无
// 环境事务即拒，先例同登记 CLI 的 execute），适配器只转交与映射，不判断任何业务结果。
type MilestoneMappingRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterMilestoneMappingCommand,
	) (application.RegisterCatalogResult, error)
}

// TriageRulesRegistrar 同上，转交分诊规则登记编排。
type TriageRulesRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterTriageRulesCommand,
	) (application.RegisterCatalogResult, error)
}

// NotificationPolicyRegistrar 同上，转交通知策略登记编排。
type NotificationPolicyRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterNotificationPolicyCommand,
	) (application.RegisterCatalogResult, error)
}

// ClaimEligibilityRegistrar 同上，转交索赔资格声明登记编排。
type ClaimEligibilityRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterClaimEligibilityCommand,
	) (application.RegisterCatalogResult, error)
}

// ClaimAuthorizationRegistrar 同上，转交申请人授权名单登记编排。
type ClaimAuthorizationRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterClaimAuthorizationCommand,
	) (application.RegisterCatalogResult, error)
}

// DisclosurePolicyRegistrar 同上，转交披露策略登记编排。
type DisclosurePolicyRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterDisclosurePolicyCommand,
	) (application.RegisterCatalogResult, error)
}

// NewRegisterMilestoneMappingEndpoint 交回里程碑映射登记的 HTTP 入口（ADR-0085
// Decision 一：登记写面进端点表带未配置格；在线口与登记 CLI 消费同一登记用例，答案代数
// 一致）。
//
// 里程碑映射改的是「这个租户怎么把各上下文的事实译成对客里程碑」，属运营配置册而非案上
// 事实，因而落在本票范围内（判据见票 admin-write-faces/02 的关务片范围实测那一节）。
func NewRegisterMilestoneMappingEndpoint(
	intake MilestoneMappingRegistrationIntake,
	registrar MilestoneMappingRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeMilestoneMappingRegistration, registrar.Handle)
}

// NewRegisterTriageRulesEndpoint 交回分诊规则登记的 HTTP 入口。分诊规则改的是「这个租户
// 把哪类信号按哪种可信度归到哪个走向」，属配置。
func NewRegisterTriageRulesEndpoint(
	intake TriageRulesRegistrationIntake,
	registrar TriageRulesRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeTriageRulesRegistration, registrar.Handle)
}

// NewRegisterNotificationPolicyEndpoint 交回通知策略登记的 HTTP 入口。通知策略改的是
// 「这个租户按哪个渠道、多长时限履行哪项披露义务」，属配置。
func NewRegisterNotificationPolicyEndpoint(
	intake NotificationPolicyRegistrationIntake,
	registrar NotificationPolicyRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeNotificationPolicyRegistration, registrar.Handle)
}

// NewRegisterClaimEligibilityEndpoint 交回索赔资格声明登记的 HTTP 入口。
//
// 它登的是一份合同责任范围覆盖哪些索赔类型，不是某一笔索赔够不够资格——后者是案上判断，
// 由 HandleClaimHandler.ScreenClaim 按这份声明核出。同一个词在两处指两件事，分界就在
// 「改的是租户的合同口径」还是「改的是这一笔的事实」。
func NewRegisterClaimEligibilityEndpoint(
	intake ClaimEligibilityRegistrationIntake,
	registrar ClaimEligibilityRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeClaimEligibilityRegistration, registrar.Handle)
}

// NewRegisterClaimAuthorizationEndpoint 交回申请人授权名单登记的 HTTP 入口。
//
// 这一类最像案上事实而实际不是，理由要写明：它以货主客户账户为键，登的是「这个租户为该
// 账户声明了谁可以代提索赔」这份带版本与发布批准责任的名单册，换名单走版本链而没有撤销
// 命令；关务片里被判为案件事实的那些提交授权则是对某一个案件的授权，且成对带 Revoke。
// 两者的分界不在「有没有指名对象」，在改的是租户的配置还是某一笔案上此刻的事实。
func NewRegisterClaimAuthorizationEndpoint(
	intake ClaimAuthorizationRegistrationIntake,
	registrar ClaimAuthorizationRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeClaimAuthorizationRegistration, registrar.Handle)
}

// NewRegisterDisclosurePolicyEndpoint 交回披露策略登记的 HTTP 入口。披露策略改的是「这个
// 租户对哪个客户展示哪几维、哪几维待确认或不展示」，属配置。
func NewRegisterDisclosurePolicyEndpoint(
	intake DisclosurePolicyRegistrationIntake,
	registrar DisclosurePolicyRegistrar,
) http.Handler {
	return newCatalogRegistrationEndpoint(intake.IntakeDisclosurePolicyRegistration, registrar.Handle)
}

// newCatalogRegistrationEndpoint 是六类共用的端点体。
//
// 共用一份而不是各抄一遍：六类交回的是同一个 RegisterCatalogResult，方法门、Intake 分流、
// 答案转写因而逐字相同，抄六遍等于把同一条转写规则摊到六处，收紧时改一处漏五处。类型仍
// 逐类分开——泛型按命令类型实例化后互不相容，把一类的编排接到另一类的端点上编译期就红，
// 价卡与序列两个包装不合并所要守的正是这一格。
func newCatalogRegistrationEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	register func(context.Context, Command) (application.RegisterCatalogResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake(request.Context(), request)
		if err != nil {
			writeCatalogRegistrationIntakeProblem(response, err)
			return
		}

		result, err := register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案：状态码只报「没形成答案」（ADR-0022），
			// 客户端据以重试或转人工，成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCatalogRegistrationAnswer(response, result)
	})
}

// catalogRegistrationResponse 是六个登记端点的封闭响应形状。`outcome` 与 `refusalReason`
// 都取应用结果枚举原名——与登记 CLI 的答案代数同源：版本不可覆盖与区间重叠是治理答案不是
// 失败，传输层不合并、不改名，也没有「其他」这一格。
type catalogRegistrationResponse struct {
	Outcome       string `json:"outcome"`
	RefusalReason string `json:"refusalReason,omitempty"`
}

// writeCatalogRegistrationIntakeProblem 与查阅侧的分流判据相同（403 未配置 / 400 畸形 /
// 5xx Intake 故障），单立一份不复用查阅侧助手：两侧各随自己的端点族演化，命令面的分流
// 将来要加载荷规范化摘要一格（ADR-0055 Decision 五），不该牵动查阅行。
func writeCatalogRegistrationIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRegistration) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// writeCatalogRegistrationAnswer 把用例答案逐名转写。
//
// 拒绝走 200 而不是 4xx：缺版本号、条目撞键、版本不可覆盖都是登记册对这次登记作出的判断，
// 与价卡登记面的 NOT_ACCEPTED 同类。4xx 会把它们与「请求根本构造不出命令」压成一格，而
// 那两件的恢复动作不同——前者要登记方改内容或换版本号，后者要接入方改请求形状。
func writeCatalogRegistrationAnswer(response http.ResponseWriter, result application.RegisterCatalogResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.CatalogRegistrationRefused {
		reason := result.RefusalReason().String()
		if reason == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedRefusalReason)
			return
		}
		writeJSON(response, http.StatusOK, catalogRegistrationResponse{Outcome: outcome, RefusalReason: reason})
		return
	}
	writeJSON(response, http.StatusCreated, catalogRegistrationResponse{Outcome: outcome})
}
