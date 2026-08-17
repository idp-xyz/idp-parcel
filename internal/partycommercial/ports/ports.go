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

// CommercialResolutionStore 按解析标识取回一次已固定的解析。
//
// 用例步骤 5 要求本上下文「固定解析标识、判断时间、锚点、版本、有效区间和当前修订」并「返回
// 不可覆盖解析结果」，`AT-PC-024` 又要求相同输入与修订「返回原解析语义」——两处都要求本上下文
// 对一次解析负有超出单次调用的责任。本端口就是那份责任的接口（[ADR-0027](../../../docs/adr/0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)）。
//
// 它的存在是为了让后续阶段只回指标识：调用方带着整个闭包回来，键就可以被替换，一次「校验」
// 便能拿另一个范围的视图去证明这份解析仍然成立。
//
// 租户是显式入参，与本包另外两个端口同理：按 ADR-0003 跨越租户必须在签名上看得见。取回后
// 调用方身份仍要与解析键比对——解析标识不是一张能力凭证。
type CommercialResolutionStore interface {
	LoadResolution(
		ctx context.Context,
		tenant domain.TenantID,
		resolution domain.ResolutionID,
	) (domain.CommercialClosure, bool, error)
	// Save 固定一次解析（ADR-0027 / UC-PC-002 步骤 5）。同标识同内容是重放，同标识
	// 异内容是冲突——两者都不是 error，且绝不覆盖（ADR-0031）。
	Save(ctx context.Context, closure domain.CommercialClosure) (ResolutionSaveOutcome, error)
}

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

// PublicationRegistry 是发布登记册的持久化面：已发布版本不可覆盖，键=租户+对象+
// 版本号。整册按（租户+范围）取回供解析选用——解析要的是候选集合与选用区间，逐条
// 查带不出「同范围有哪些并存版本」。
//
// 本口先只承载版本册；有效性更正册（ADR-0038）与价格/结算政策册（ADR-0034/0044）
// 的持久化面另票补，端口届时扩展而不是在这里预开空方法。
type PublicationRegistry interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
	) (*domain.CommercialRegistry, error)
	SaveVersion(
		ctx context.Context,
		version domain.CommercialVersion,
	) (PublicationSaveOutcome, error)
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
