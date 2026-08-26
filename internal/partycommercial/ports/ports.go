// Package ports 声明 party-commercial 自有的语义边界。PostgreSQL 适配器在
// adapters/postgres：发布登记册、已固定解析库与授权治理册。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CommercialAuthorityView 取一个范围下的当前权威商业视图。
//
// 按范围取而不是按对象取，是因为解析要回答的是「这个范围里有几个适用候选」。逐对象取
// 看不见新增的同范围候选，而那恰恰是提交前失效要抓的东西——已采用对象自身一字未动，
// 解析却已经不再唯一。
//
// 租户是显式入参，不从 context 里补：按 ADR-0003 运营集团租户是最高数据隔离边界，跨越
// 它必须在签名上看得见。
type CommercialAuthorityView interface {
	LoadScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
	) (*domain.CommercialRegistry, error)
}

// AsOfPolicyDeclaration 取某个接单规则包为各类下游判断声明的时点锚。
//
// 它与 CommercialAuthorityView 分开，因为两阶段机制的全部意义就在这个分界上：第一阶段用独立
// 于候选规则包的选择锚点选出规则包，第二阶段才由已选出的那个包声明各判断的 `asOf`。合成一个
// 端口，规则包就有机会参与决定它自己被选中的时间。
//
// 租户是显式入参：时点政策按租户登记为 `PAR-COM-14`，而按 ADR-0003 跨租户必须在签名上看得见。
//
// 没有租户时它必然交回空声明，编排据以停在`未配置`。这正是首发唯一走得到的真实分支——那组政策
// 属实例半边，本上下文不内置任何默认，尤其不拿系统当前时间顶替。
type AsOfPolicyDeclaration interface {
	LoadAsOfPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) ([]domain.AsOfPolicy, error)
}

// AcceptanceContentDeclaration 取已唯一选出对象为新委托声明的接受内容：规则包的适用校验组
// 与人工复核指令、服务产品的待路由许可（ADR-0042）。
//
// 它与 CommercialAuthorityView 分开，分界与 AsOfPolicyDeclaration 同一条：第一阶段用独立锚点
// 选出规则包与产品，内容声明只在唯一选出之后按已选对象读取。塞进解析结果，声明就有机会参与
// 决定它自己被谁采用。
//
// found=false 的含义按方法分，不共用一句：两个声明缺席时的安全方向相反，混说会引着读者
// 把适配器往错的方向「修」。共同点只有一条——声明属实例半边，本上下文不内置任何默认，
// 没有租户时两个方法必然都交回 found=false。
type AcceptanceContentDeclaration interface {
	// LoadAcceptanceRuleContent 的 found=false = 实例未配置。缺声明不等于没有组适用、也不
	// 等于免复核——无从知道该判哪些组；消费方据以停在未决，不是放行（不默认全组适用、
	// 不默认免复核）。
	LoadAcceptanceRuleContent(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) (domain.AcceptanceRuleContent, bool, error)

	// LoadPendingRoutingPermission 的 found=false = 未许可（零值语义），不是未决。待路由是
	// 例外许可，UC 只认「服务产品明确允许」；没有声明就是没有许可，消费方照常推进、只是
	// 不得走待路由接受。读取失败（error）才是未决——那是谁也没回答过，不得冒充「产品说了
	// 不许」。
	LoadPendingRoutingPermission(
		ctx context.Context,
		tenant domain.TenantID,
		serviceProduct domain.CommercialVersion,
	) (domain.PendingRoutingPermission, bool, error)
}

// PreAcceptanceControlDeclarationView 取客户合同版本对「这个范围要不要接受前财务控制」
// 的声明（`PAR-COM-15`，列在合同版本下）。
//
// found=false = 实例未配置。CONTEXT 明写「合同明确无接受前财务控制时必须保存商业不适用
// 依据，不能用缺失结果或默认通过代替」——所以缺声明既不是`要求控制`也不是`不适用`，
// 消费方停在未决等租户把合同正文补齐。读取失败（error）是另一格，等依赖恢复。
//
// **它只答「要不要」。** 控制用什么方式、实际采用哪一版政策由结算政策的解析回答
// （ADR-0044），本口一概不碰。两层压成一层就会从结算方式倒推控制要不要，而
// `pn-02-w03` 明写账期不能推导无需信用校验——那条推导会让一个约定了账期的客户
// 静默跳过信用校验。
//
// 租户显式入参，同本包其余端口（ADR-0003）。
type PreAcceptanceControlDeclarationView interface {
	LoadPreAcceptanceControl(
		ctx context.Context,
		tenant domain.TenantID,
		contract domain.CommercialVersion,
	) (domain.PreAcceptanceControlDeclaration, bool, error)
}

// CustomerContractContentView 取已唯一选出的客户合同版本的正文：它引用哪个接单规则包，
// 以及按费用范围对财务控制作了什么约定。
//
// 它与 CommercialAuthorityView 分开，也与 PreAcceptanceControlDeclarationView 分开：
// 前者回答「这个范围有几个适用候选」，后者回答合同版本级「要不要」接受前控制；本口
// 回答范围级「适用哪份策略 / 显式不适用」。塞进 ViewRevision，一次与选择无关的绑定
// 改动会把该范围全部在途解析判成已失效（open-decisions D-4 的同一条纪律）。
//
// found=false = 正文未登记（无父行）。父行在场而零子行是另一件事：租户明确登记了
// 一份没有费用范围约定的合同，FinancialControlFor 对任何范围答「不存在」而不是
// 「不适用」。读取失败与坏数据（含版本壳与正文件规则包引用分歧）走 error，不得折成
// found=false——那会把一份损坏的正文伪装成从未登记。
//
// 租户显式入参，同本包其余端口（ADR-0003）。本口不提供默认内容。
type CustomerContractContentView interface {
	LoadCustomerContract(
		ctx context.Context,
		tenant domain.TenantID,
		contract domain.CommercialVersion,
	) (domain.CustomerContract, bool, error)
}

// IntakeQualificationView 取已唯一选出的接单规则包版本的收寄资格声明（PAR-COM-16）。
//
// 它与 CommercialAuthorityView 分开：前者回答「这个范围有几个适用候选」，本口回答已
// 选出规则包的阶段正文。塞进 ViewRevision，一次与选择无关的声明改动会把该范围全部
// 在途解析判成已失效（open-decisions D-4 的同一条纪律）。
//
// found=false = 声明未登记（无父行）。父行在场而子行空/坏走 error，不得折成
// found=false——领域要求至少一行允许来源，把损坏的正文伪装成从未登记会让消费方去催
// 一份其实已经写坏的配置。读取失败同样走 error。本上下文不提供默认内容。
//
// 租户显式入参，同本包其余端口（ADR-0003）。显式租户必须与拥有规则版本同一身份。
type IntakeQualificationView interface {
	LoadIntakeQualification(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) (domain.IntakeQualificationContent, bool, error)
}

// FinalRuleContentView 取已唯一选出的接单规则包版本的终局规则声明（PAR-COM-17）。
//
// 分界、三格含义与 IntakeQualificationView 相同：无父行 = 未配置；父行在场而零子行
// 是坏声明（NewFinalRuleContent 拒零行），走 error。产品与合同是采用方，不作为本口
// 的键（ADR-0058）。
type FinalRuleContentView interface {
	LoadFinalRule(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) (domain.FinalRuleContent, bool, error)
}

// CancellationAuthorityContentView 取已唯一选出的授权规则版本的取消授权目录
// （PAR-COM-17）。拥有对象是授权规则，不是产品或合同（ADR-0058）。
//
// 三格含义同上：无父行 = 未配置；父行在场而零子行走 error。本上下文不默认「客户可取消」。
type CancellationAuthorityContentView interface {
	LoadCancellationAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		authorizationRule domain.CommercialVersion,
	) (domain.CancellationAuthorityContent, bool, error)
}

// AcceptanceRulePackageContentView 取已唯一选出的接单规则包版本的正文：五维适用性
// 与按分类归档的规则引用。
//
// 它与 CommercialAuthorityView 分开：前者回答「这个范围有几个适用候选」，本口回答
// 已选出规则包的正文。五维适用性照存，但不参与选择、不进 CommercialRegistry /
// ViewRevision（open-decisions D-3）。塞进视图，一次与选择无关的正文改动会把该范围
// 全部在途解析判成已失效。
//
// found=false = 正文未登记（无父行）。父行在场而零子行是坏数据：领域要求至少一条
// 规则，空包等于无条件接受（NewAcceptanceRulePackage 拒空）。读取失败与坏数据走
// error，不得折成 found=false——那会把一份损坏的正文伪装成从未登记。本口不提供
// 默认正文，也不提供 Save。
//
// 租户显式入参，同本包其余端口（ADR-0003）。显式租户必须与拥有规则版本同一身份。
type AcceptanceRulePackageContentView interface {
	LoadAcceptanceRulePackage(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) (domain.AcceptanceRulePackage, bool, error)
}

// CommercialResolutionView 是解析固定的只读半边：按（租户+解析标识）取回一次已固定的闭包。
//
// 它是 CommercialResolutionStore 的读口。消费方只要回指标识（ADR-0027 / ADR-0062），不该
// 持有 Save——写口与读口分开的理由与 CommercialPublicationView 同形（F-2）：解析与采用
// 回指只要候选闭包，把 Save 交给它们等于让消费侧适配器拿到覆盖一次解析的能力。
type CommercialResolutionView interface {
	LoadResolution(
		ctx context.Context,
		tenant domain.TenantID,
		resolution domain.ResolutionID,
	) (domain.CommercialClosure, bool, error)
}

// CommercialResolutionStore 按解析标识取回并固定一次已解析的闭包。
//
// 用例步骤 5 要求本上下文「固定解析标识、判断时间、锚点、版本、有效区间和当前修订」并「返回
// 不可覆盖解析结果」，`AT-PC-024` 又要求相同输入与修订「返回原解析语义」——两处都要求本上下文
// 对一次解析负有超出单次调用的责任。本端口就是那份责任的接口（[ADR-0027](../../../docs/adr/0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)）。
//
// 本口内嵌 CommercialResolutionView，再叠加 Save。写侧调用方依赖本口；只需读的消费方
// 依赖内嵌的只读口，不持有 Save。
//
// 它的存在是为了让后续阶段只回指标识：调用方带着整个闭包回来，键就可以被替换，一次「校验」
// 便能拿另一个范围的视图去证明这份解析仍然成立。
//
// 租户是显式入参，与本包另外两个端口同理：按 ADR-0003 跨越租户必须在签名上看得见。取回后
// 调用方身份仍要与解析键比对——解析标识不是一张能力凭证。
type CommercialResolutionStore interface {
	CommercialResolutionView
	// Save 固定一次解析（ADR-0027 / UC-PC-002 步骤 5）。同标识同内容是重放，同标识
	// 异内容是冲突——两者都不是 error，且绝不覆盖（ADR-0031）。
	Save(ctx context.Context, closure domain.CommercialClosure) (ResolutionSaveOutcome, error)
}

// 只读口必须是写口的真子集：写口当只读口接线时，消费方看不到 Save。
var _ CommercialResolutionView = CommercialResolutionStore(nil)

// ResolutionSaveOutcome 是一次解析固定在持久化面的落点封闭代数（ADR-0031）：
// `已记录`是重放（同标识同内容），`内容冲突`是同标识携带不同闭包——需要查库，
// 绝不静默覆盖；两者都不是 error，事务保持可用。
type ResolutionSaveOutcome uint8

const (
	ResolutionSaveOutcomeInvalid ResolutionSaveOutcome = iota
	ResolutionSaved
	ResolutionAlreadyRecorded
	ResolutionContentConflict
)

func (outcome ResolutionSaveOutcome) String() string {
	switch outcome {
	case ResolutionSaved:
		return "SAVED"
	case ResolutionAlreadyRecorded:
		return "ALREADY_RECORDED"
	case ResolutionContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// PublicationSaveOutcome 是一次版本登记在持久化面的落点封闭代数（ADR-0031 同款）：
// `已登记`是重放（同键同内容），`内容冲突`是同版本号携带不同内容——需要商业责任方
// 修正，绝不静默覆盖；两者都不是 error，事务保持可用。
type PublicationSaveOutcome uint8

const (
	PublicationSaveOutcomeInvalid PublicationSaveOutcome = iota
	PublicationSaved
	PublicationAlreadyRegistered
	PublicationContentConflict
)

func (outcome PublicationSaveOutcome) String() string {
	switch outcome {
	case PublicationSaved:
		return "SAVED"
	case PublicationAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case PublicationContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ServiceProductSaveOutcome 是一次服务产品形态登记在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放（同一产品版本同一形态），`内容冲突`是同一产品版本被登记成另一种形态
// ——同一次发布不可能既是这种形态又是那种，需要商业责任方修正，绝不覆盖；两者都不是
// error，事务保持可用。
type ServiceProductSaveOutcome uint8

const (
	ServiceProductSaveOutcomeInvalid ServiceProductSaveOutcome = iota
	ServiceProductSaved
	ServiceProductAlreadyRegistered
	ServiceProductContentConflict
)

func (outcome ServiceProductSaveOutcome) String() string {
	switch outcome {
	case ServiceProductSaved:
		return "SAVED"
	case ServiceProductAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ServiceProductContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ValidityCorrectionSaveOutcome 是一次区间更正登记在持久化面的落点。
// 更正只增（D-5）：同一版本可有多条，同内容重放答`已登记`；不同内容是一条新更正，
// 答`已保存`。这里没有「内容冲突」格——异更正不是冲突，是合法的下一条。
type ValidityCorrectionSaveOutcome uint8

const (
	ValidityCorrectionSaveOutcomeInvalid ValidityCorrectionSaveOutcome = iota
	ValidityCorrectionSaved
	ValidityCorrectionAlreadyRegistered
)

func (outcome ValidityCorrectionSaveOutcome) String() string {
	switch outcome {
	case ValidityCorrectionSaved:
		return "SAVED"
	case ValidityCorrectionAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// PricePolicySaveOutcome 是一次商业价格政策登记在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放，`内容冲突`是同一价格规则版本被登记成另一份政策正文——含发布期
// 保全的方案方向与转换（ADR-0057）。两者都不是 error，绝不覆盖。
type PricePolicySaveOutcome uint8

const (
	PricePolicySaveOutcomeInvalid PricePolicySaveOutcome = iota
	PricePolicySaved
	PricePolicyAlreadyRegistered
	PricePolicyContentConflict
)

func (outcome PricePolicySaveOutcome) String() string {
	switch outcome {
	case PricePolicySaved:
		return "SAVED"
	case PricePolicyAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case PricePolicyContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// SettlementPolicySaveOutcome 是一次结算政策登记在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放，`内容冲突`是同一结算政策版本被登记成另一种方式或另一份六维适用范围。
// 两者都不是 error，绝不覆盖。
type SettlementPolicySaveOutcome uint8

const (
	SettlementPolicySaveOutcomeInvalid SettlementPolicySaveOutcome = iota
	SettlementPolicySaved
	SettlementPolicyAlreadyRegistered
	SettlementPolicyContentConflict
)

func (outcome SettlementPolicySaveOutcome) String() string {
	switch outcome {
	case SettlementPolicySaved:
		return "SAVED"
	case SettlementPolicyAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case SettlementPolicyContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// DeclarationSaveOutcome 是一份版本化声明正文在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放（同拥有版本同正文），`内容冲突`是同拥有版本携带不同正文——声明随
// 发布固定，改声明必须发新版本，绝不覆盖也绝不并写；两者都不是 error，事务保持可用。
type DeclarationSaveOutcome uint8

const (
	DeclarationSaveOutcomeInvalid DeclarationSaveOutcome = iota
	DeclarationSaved
	DeclarationAlreadyRegistered
	DeclarationContentConflict
)

func (outcome DeclarationSaveOutcome) String() string {
	switch outcome {
	case DeclarationSaved:
		return "SAVED"
	case DeclarationAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case DeclarationContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// CommercialPublicationView 按（租户+范围）取回该范围已发布的整册。
//
// 它是 PublicationRegistry 的只读半边。解析与权威视图只要候选集合，不该持有
// SaveVersion 及各通道的具名 Save——两端口分开的理由写在 PublicationRegistry
// 的注释里；CommercialAuthority 依赖本口，兑现那条理由（F-2）。
//
// LoadForScope 一次交回该范围**已落库的全部通道**，而不是逐通道各取一次：ViewRevision
// 由各通道的内容共同派生，两次取回之间视图一变，派生出的修订就不再对应任何一个真实时刻。
// 通道增多时扩的是那一次取回的内容，不是取回的次数。
type CommercialPublicationView interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
	) (*domain.CommercialRegistry, error)
}

// PublicationRegistry 是发布登记册的持久化面：已发布版本不可覆盖，键=租户+对象+
// 版本号。整册按（租户+范围）取回供解析选用——解析要的是候选集合与选用区间，逐条
// 查带不出「同范围有哪些并存版本」。
//
// 本口内嵌 CommercialPublicationView，再叠加各通道的具名 Save。写侧调用方依赖本口；
// 只需读的解析与权威视图依赖内嵌的只读口，不持有任何 Save。
//
// 本口今天承载版本册、服务产品形态册（ADR-0050）、有效性更正册（ADR-0038）以及
// 价格与结算政策册（ADR-0034/0044/0057）。端口按具名 Save 扩展，不开通用口。
type PublicationRegistry interface {
	CommercialPublicationView
	SaveVersion(
		ctx context.Context,
		version domain.CommercialVersion,
	) (PublicationSaveOutcome, error)
	// SaveServiceProduct 登记一份服务产品版本的服务形态。它不代替 SaveVersion：
	// 产品对象本身仍须按版本通道入册，这里只登记「它是哪种形态」（ADR-0050）。
	SaveServiceProduct(
		ctx context.Context,
		product domain.ServiceProduct,
	) (ServiceProductSaveOutcome, error)
	// SaveValidityCorrection 登记一条区间更正。它不代替 SaveVersion，也不改写原版本
	// 键下的正文、批准与原区间（ADR-0038）。同一版本已有更正时，不同内容追加为新行。
	SaveValidityCorrection(
		ctx context.Context,
		correction domain.ValidityCorrection,
	) (ValidityCorrectionSaveOutcome, error)
	// SavePricePolicy 登记一份价格规则版本的计价正文。它不代替 SaveVersion。
	// planDirection 与 conversion 是发布当时 parcel-pricing 的答复与当时声明的转换，
	// 必须显式交出（ADR-0057）；结构体上没有这两项。
	SavePricePolicy(
		ctx context.Context,
		policy domain.CommercialPricePolicy,
		planDirection domain.PriceDirection,
		conversion domain.PlanBindingConversion,
	) (PricePolicySaveOutcome, error)
	// SaveSettlementPolicy 登记一份结算政策版本的方式与六维适用范围。它不代替 SaveVersion。
	SaveSettlementPolicy(
		ctx context.Context,
		policy domain.SettlementPolicy,
	) (SettlementPolicySaveOutcome, error)

	// 以下是六族声明表的具名 Save（syn-wall-door-audit 票 03 的写入半边）。声明正文
	// 随其拥有版本的发布一并登记，键=拥有版本完整身份；按拥有对象挂、不合并
	// （ADR-0042/0058 的归属纪律）。它们都不代替 SaveVersion：拥有版本自身仍须按版本
	// 通道入册。声明的**读**口仍在各自的只读端口上（族 B 点读，不进 ViewRevision），
	// 消费方不经本口取声明——写与读分开的理由与 CommercialPublicationView 同源（F-2）。
	SaveAsOfPolicies(
		ctx context.Context,
		declaration domain.AsOfDeclaration,
	) (DeclarationSaveOutcome, error)
	SaveAcceptanceRuleContent(
		ctx context.Context,
		content domain.AcceptanceRuleContent,
	) (DeclarationSaveOutcome, error)
	SavePendingRoutingPermission(
		ctx context.Context,
		permission domain.PendingRoutingPermission,
	) (DeclarationSaveOutcome, error)
	SavePreAcceptanceControl(
		ctx context.Context,
		declaration domain.PreAcceptanceControlDeclaration,
	) (DeclarationSaveOutcome, error)
	SaveCustomerContractContent(
		ctx context.Context,
		contract domain.CustomerContract,
	) (DeclarationSaveOutcome, error)
	SaveIntakeQualification(
		ctx context.Context,
		content domain.IntakeQualificationContent,
	) (DeclarationSaveOutcome, error)
	SaveFinalRule(
		ctx context.Context,
		content domain.FinalRuleContent,
	) (DeclarationSaveOutcome, error)
	SaveCancellationAuthority(
		ctx context.Context,
		content domain.CancellationAuthorityContent,
	) (DeclarationSaveOutcome, error)
	SaveAcceptanceRulePackage(
		ctx context.Context,
		pack domain.AcceptanceRulePackage,
	) (DeclarationSaveOutcome, error)
}

type Clock interface {
	Now() time.Time
}

// GrantSaveOutcome 是一次授权规则登记在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放，`内容冲突`是同版本号携带不同授权内容——绝不覆盖。
type GrantSaveOutcome uint8

const (
	GrantSaveOutcomeInvalid GrantSaveOutcome = iota
	GrantSaved
	GrantAlreadyRegistered
	GrantContentConflict
)

func (outcome GrantSaveOutcome) String() string {
	switch outcome {
	case GrantSaved:
		return "SAVED"
	case GrantAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case GrantContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// AuthorityGrantStore 是授权治理册的持久化面，也是裁定编排的内部协作者。
//
// LoadEffectiveGrants 按（租户+范围+业务时点）装载当时管得着的授权。空切片交给
// domain.Authorize 译`未配置`，本口不把空折成不允许。读取失败上抛，不得折成空切片。
//
// 公开裁定口是应用层的 AdjudicateCommercialAuthorization，不在本接口上——消费方
// 不得绕过 Authorize 四格直接拿切片自己判。
type AuthorityGrantStore interface {
	LoadEffectiveGrants(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
		at time.Time,
	) ([]domain.AuthorityGrant, error)
	SaveGrant(ctx context.Context, grant domain.AuthorityGrant) (GrantSaveOutcome, error)
}

// ServiceProductCatalogueRow 是服务产品目录上列的一行:一份已入册的服务产品版本,
// 连同它登记过的服务形态。
//
// 上列对象是**版本壳**而不是形态行(ADR-0077 通例下本上下文的对照结论):CONTEXT 把
// 目录对象定义为「服务产品版本——具有独立身份和适用范围的商业定义版本」,身份、范围、
// 区间与状态都在版本壳上;形态册只答「它是哪种服务形态」,且缺席是合法的(ADR-0050:
// 产品缺席不使解析退化)。只列形态行会让未登形态的已发布产品从目录上消失——目录以
// 缺席说谎。装载方向与 LoadForScope 同派:版本侧驱动,形态左连接。
//
// HasEffectiveEnd 为假即开放结束。用显式布尔而不是零值判断:零时刻是一个合法的
// 绝对时刻,拿它兼作「没有终点」会让补历史的区间读不出来。HasForm 同理:形态未登记
// 与登记了空形态必须可分辨,后者在库上进不来,前者是本行的常态。
type ServiceProductCatalogueRow struct {
	ObjectID          string
	VersionLabel      string
	Scope             string
	Status            string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	PublishedAt       time.Time
	Form              string
	HasForm           bool
}

// ServiceProductCatalogueRead 是服务产品目录的伴生列表读端口(ADR-0077):管理台
// service-products 页的供数面。它不拓宽 PublicationRegistry——扩写侧接口会拆全部
// 写侧测试替身,伴生读端口另立(与 OperationsProjectionRead 不并进 ProjectionStore
// 同一条理由)。
//
// 租户在方法签名上(ADR-0077 Decision 五):目录是租户内部对象,运营查阅的授权边界
// 只有租户。Limit 必须为正;每页多大由接入面按渠道契约裁决,读口只拒绝无意义的取值。
// 空目录如实交回空列表(ADR-0077 Decision 四):空表本身就是内容,上列不形成判断。
type ServiceProductCatalogueRead interface {
	ListServiceProducts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ServiceProductCatalogueRow, error)
}

// AssembledRuleRow 是规则包正文里一条按分类归档的规则引用的上列转写。
type AssembledRuleRow struct {
	Category  string
	Reference string
}

// AcceptanceRulePackageRow 是接单规则包正文册(0014 父子两表)上列的一行:五维适用
// 性与按分类归档的规则引用。正文照上列,不参与选择——选包仍走版本壳(ADR-0059,
// 五维不进 ViewRevision),目录读它不改变这一点。
type AcceptanceRulePackageRow struct {
	ObjectID          string
	VersionLabel      string
	ServiceProduct    string
	Contract          string
	LegalEntity       string
	Scope             string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	DeclaredAt        time.Time
	Rules             []AssembledRuleRow
}

// PreAcceptanceControlRow 是接受前财务控制声明册上列的一行。拥有对象是**客户合同
// 版本**而不是第 5 类策略对象(`PAR-COM-15` 列在合同版本下,库上 object_kind CHECK
// 钉在 2)——ContractObjectID/ContractVersion 因此指名合同。NotApplicableBasis 只在
// `不适用`时携带,库上 CHECK 与 requirement 绑定,这里如实转写不补。
type PreAcceptanceControlRow struct {
	ContractObjectID   string
	ContractVersion    string
	Requirement        string
	NotApplicableBasis string
	DeclaredAt         time.Time
}

// PricePolicyRow 是商业价格政策册上列的一行:方向、方案绑定与政策自己的适用范围。
// PlanDirection 与 BindingConversion 是发布当时保全的答复与声明(ADR-0057),照列
// 转写。
type PricePolicyRow struct {
	ObjectID          string
	VersionLabel      string
	Direction         string
	PlanRef           string
	PlanDirection     string
	BindingConversion string
	PolicyScope       string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	RegisteredAt      time.Time
}

// SettlementPolicyRow 是结算政策册上列的一行:方式与六维适用范围平铺(ADR-0044)。
type SettlementPolicyRow struct {
	ObjectID          string
	VersionLabel      string
	Method            string
	LegalEntity       string
	Counterparty      string
	ContractLabel     string
	ChargeScope       string
	Currency          string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	RegisteredAt      time.Time
}

// AsOfPolicyRow 是时点锚声明册上列的一行:某接单规则包版本为某类下游判断声明的
// 时点语义与政策版本。它不存时点值本身——取值由消费方逐项形成,目录照实转写。
type AsOfPolicyRow struct {
	RulePackageObjectID string
	RulePackageVersion  string
	JudgmentType        string
	SemanticsRef        string
	PolicyVersion       string
	DeclaredAt          time.Time
}

// CommercialPolicyCatalogueRead 是商业策略目录的伴生列表读端口(ADR-0077):管理台
// commercial-policies 页的供数面,策略种类是封闭集,每种一个方法。
//
// 五种册子:接单规则包正文(0014)、接受前财务控制声明(0007)、商业价格政策(0010)、
// 结算政策(0011)、时点锚声明(0005)。CONTEXT 词条里的**信用政策**没有独立正文表
// (版本壳可入册,正文册未建),如实不列——预留一个空方法就是替租户拟一种它还没有
// 的册子;正文表落库时按封闭集扩方法,不开通用口。
//
// 租户在签名上、Limit 非正拒、空册答空列表,判据同 ServiceProductCatalogueRead。
// 各册行内自带的对象/版本标识只是引用转写,读口不跨表拼接版本壳——策略种类间不串,
// 每个方法只读自己那张册子。
type CommercialPolicyCatalogueRead interface {
	ListAcceptanceRulePackages(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]AcceptanceRulePackageRow, error)
	ListPreAcceptanceControls(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]PreAcceptanceControlRow, error)
	ListPricePolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]PricePolicyRow, error)
	ListSettlementPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]SettlementPolicyRow, error)
	ListAsOfPolicyDeclarations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]AsOfPolicyRow, error)
}

// ControlBindingRow 是一份客户合同正文里对某个费用范围的财务控制约定的上列转写。
// 指名策略与显式不适用恰有一个在场(库上 CHECK 钉住,两列同空的行进不来),因此这里
// 不设「两者皆无」的第三态:读回两空即坏数据,由装载方上抛。
type ControlBindingRow struct {
	ChargeScope          string
	PolicyID             string
	InapplicabilityBasis string
}

// CustomerContractCatalogueRow 是客户与合同目录上列的一行:一份已入册的客户合同
// 版本壳,连同它登记过的正文与按费用范围的控制约定。
//
// 上列对象是版本壳,判据同 ServiceProductCatalogueRow:身份、范围、区间与状态都在
// 壳上,正文缺席是合法的。
//
// HasContent 不能省,也不能拿 len(Bindings) 兼作它。0012 迁移把这条写进了表形:
// **无正文行 = 正文未登记**,**有正文行零绑定 = 明确的空约定**——后者是合同已登记
// 且对任何费用范围都没作约定,与前者的恢复动作完全不同(前者去登记正文,后者无事
// 可做)。两态在「零绑定」上撞成同一个可观察签名,只有这个布尔分得开。
// RulePackageID 同理只在 HasContent 为真时有意义:它必存于正文行(库上 NOT NULL)。
type CustomerContractCatalogueRow struct {
	ObjectID          string
	VersionLabel      string
	Scope             string
	Status            string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	PublishedAt       time.Time
	RulePackageID     string
	DeclaredAt        time.Time
	HasContent        bool
	Bindings          []ControlBindingRow
}

// SupplierAgreementCatalogueRow 是供应商协议目录上列的一行:一份已入册的供应商
// 商业协议版本壳。
//
// **只有壳**。领域的 SupplierAgreement 还携供应商、采购定价方案与方向,但那些今天
// 没有正文表——与 CommercialPolicyCatalogueRead 注释里信用政策那一格同形:版本壳
// 可入册,正文册未建。如实只列壳,不从别处拼一份看起来完整的行;正文表落库时在本
// 结构上扩字段,那时才谈得上列它们。
type SupplierAgreementCatalogueRow struct {
	ObjectID          string
	VersionLabel      string
	Scope             string
	Status            string
	EffectiveStartsAt time.Time
	EffectiveEndsAt   time.Time
	HasEffectiveEnd   bool
	PublishedAt       time.Time
}

// CommercialRelationCatalogueRead 是商业关系载体目录的伴生列表读端口(ADR-0077):
// 管理台 party-contracts 与 supplier-agreements 两页的供数面。
//
// 它与 CommercialPolicyCatalogueRead 分开而不并入,因为两者装的不是一类东西:那边
// 是**策略**(接单规则包、财务控制、价格、结算、时点锚),这边是**商业关系的载体**
// (客户合同、供应商协议)。合同不是一种策略——把它并进去,那个读口连同它对外的
// `kind` 参数就开始说谎,而端点路径是对外契约的一部分,日后改的代价比现在分开大。
//
// 两类各一个方法,不开按 object_kind 传参的通用口:通用口会让「本上下文支持哪几类
// 目录查阅」从代码里读不出来,而那正是封闭集要表达的东西(判据同
// CommercialPolicyCatalogueRead 的「正文表落库时按封闭集扩方法」)。
//
// 租户在签名上、Limit 非正拒、空册答空列表,判据同 ServiceProductCatalogueRead。
type CommercialRelationCatalogueRead interface {
	ListCustomerContracts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CustomerContractCatalogueRow, error)
	ListSupplierAgreements(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]SupplierAgreementCatalogueRow, error)
}
