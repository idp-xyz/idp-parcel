package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// JudgmentAsOfOutcome 是第二阶段的封闭结果集合。
//
// 五种未成形分开，因为要采取的动作各不相同：`未配置`等 `PAR-COM-14` 落地，`未决`等依赖恢复，
// `值不合法`要调用方改这次请求，`依据未解析`要它先回到第一阶段，`输入未受理`说的是它问了一份
// 不属于自己的解析。压成一个「失败」，调用方就只能靠猜——而其中只有一种是它自己能修的。
type JudgmentAsOfOutcome uint8

const (
	JudgmentAsOfOutcomeInvalid JudgmentAsOfOutcome = iota
	JudgmentAsOfFormed
	JudgmentAsOfBasisNotResolved
	JudgmentAsOfNotConfigured
	JudgmentAsOfPending
	JudgmentAsOfValueInvalid
	JudgmentAsOfInputNotAccepted
)

func (outcome JudgmentAsOfOutcome) String() string {
	switch outcome {
	case JudgmentAsOfFormed:
		return "FORMED"
	case JudgmentAsOfBasisNotResolved:
		return "BASIS_NOT_RESOLVED"
	case JudgmentAsOfNotConfigured:
		return "NOT_CONFIGURED"
	case JudgmentAsOfPending:
		return "PENDING"
	case JudgmentAsOfValueInvalid:
		return "VALUE_INVALID"
	case JudgmentAsOfInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// JudgmentAsOfRequest 是消费方为一项判断形成的时点值。值由消费方给，语义由规则包声明——用例
// 步骤 6 把这两半分给了不同责任方，本上下文只负责把它们对起来并校验。
type JudgmentAsOfRequest struct {
	Judgment domain.JudgmentType
	At       time.Time
}

// CallerScope 是调用方自称的身份。它与解析标识一起进来，因为标识不是一张能力凭证：只凭标识
// 就交回闭包，任何拿到标识的人都能读走另一个客户的商业依据，而 `AT-PC-028` 要挡的正是这个。
type CallerScope struct {
	TenantID          domain.TenantID
	CustomerAccountID domain.CustomerAccountID
}

// FormJudgmentAsOfCommand 只回指第一阶段的解析标识，不收调用方带回来的闭包，也不收调用方
// 另给的规则包。
//
// 不收规则包：它必须由第一阶段用独立锚点选出，自带一个进来就等于让它决定自己被选中的时间。
// 不收闭包：闭包里带着形成它的那次查询，调用方能替换它，一次「校验」就能拿另一个范围的视图
// 去证明这份解析仍然成立。中间状态因此由本上下文按标识保留（ADR-0027）。
type FormJudgmentAsOfCommand struct {
	Caller     CallerScope
	Resolution domain.ResolutionID
	Judgments  []JudgmentAsOfRequest
}

type FormJudgmentAsOfResult struct {
	outcome JudgmentAsOfOutcome
	formed  map[domain.JudgmentType]domain.JudgmentAsOf
}

func (result FormJudgmentAsOfResult) Outcome() JudgmentAsOfOutcome {
	return result.outcome
}

func (result FormJudgmentAsOfResult) AsOfFor(judgment domain.JudgmentType) (domain.JudgmentAsOf, bool) {
	asOf, formed := result.formed[judgment]
	return asOf, formed
}

type FormJudgmentAsOfHandler struct {
	resolutions ports.CommercialResolutionStore
	policies    ports.AsOfPolicyDeclaration
}

func NewFormJudgmentAsOfHandler(
	resolutions ports.CommercialResolutionStore,
	policies ports.AsOfPolicyDeclaration,
) *FormJudgmentAsOfHandler {
	return &FormJudgmentAsOfHandler{resolutions: resolutions, policies: policies}
}

// loadPrior 按标识取回第一阶段的结果，并核对它确实属于这个调用方。
//
// 三种取不到分开：读不回是`未决`（等依赖恢复），查无此解析是`依据未解析`（要回第一阶段），
// 范围不符是`输入未受理`（`AT-PC-028`：不泄露候选，也不告诉对方这份解析存不存在）。合成一格，
// 一次越权探测就与一次依赖抖动分不开，而前者不该被重试。
func (handler *FormJudgmentAsOfHandler) loadPrior(
	ctx context.Context,
	caller CallerScope,
	resolution domain.ResolutionID,
) (domain.CommercialClosure, JudgmentAsOfOutcome) {
	// 身份或标识缺失时不查询：一次已经发出的查询本身就回答了「这份解析存不存在」，而用例
	// 要求最小身份不成立时不查询、不泄露候选。
	if caller.TenantID.String() == "" ||
		caller.CustomerAccountID.String() == "" ||
		resolution.String() == "" {
		return domain.CommercialClosure{}, JudgmentAsOfInputNotAccepted
	}

	prior, found, err := handler.resolutions.LoadResolution(ctx, caller.TenantID, resolution)
	if err != nil {
		return domain.CommercialClosure{}, JudgmentAsOfPending
	}
	if !found {
		return domain.CommercialClosure{}, JudgmentAsOfBasisNotResolved
	}

	// 取回之后仍要比对：端口按租户取，但同一租户下的另一个客户账户同样不该读到这份解析。
	key := prior.ResolutionKey()
	if key.TenantID != caller.TenantID || key.CustomerAccountID != caller.CustomerAccountID {
		return domain.CommercialClosure{}, JudgmentAsOfInputNotAccepted
	}
	return prior, JudgmentAsOfFormed
}

// Handle 执行用例第二阶段：由第一阶段选出的规则包声明各下游判断的时点锚，消费方给出值，两者
// 在这里对起来并逐项校验。
//
// 一次调用之内全有或全无，不逐项部分成功：一次请求里缺一项时，已形成的那些也不交回。与第一阶段
// 的引用闭包同一条道理——把已经解出的成员交回去，等于引诱调用方在一份判定为不成立的依据上继续
// 往下走。
//
// 这条**只管一次调用**，不要读成「调用方不可能拿着半套时点往下走」。调用方完全可以分两次调用、
// 各要一项，其中一项形成、另一项停在`未配置`——本上下文看不见那个局面，也拦不到它。真正拦住它
// 的在消费方：接受决定要求每个适用校验组都已判断，缺一组就不形成决定。把这里写成一条全局担保，
// 就是声称代码做不到的事。
func (handler *FormJudgmentAsOfHandler) Handle(
	ctx context.Context,
	command FormJudgmentAsOfCommand,
) (FormJudgmentAsOfResult, error) {
	prior, loaded := handler.loadPrior(ctx, command.Caller, command.Resolution)
	if loaded != JudgmentAsOfFormed {
		return FormJudgmentAsOfResult{outcome: loaded}, nil
	}

	// 先看第一阶段成没成。没有已选规则包就去问政策，等于替一个尚未选出的包声明时点锚，而调用
	// 方拿到时点锚会以为商业依据已经定了。
	rulePackage, selected := prior.AdoptedFor(domain.AcceptanceRulePackageObject)
	if prior.Outcome() != domain.UniquelyResolved || !selected {
		return FormJudgmentAsOfResult{outcome: JudgmentAsOfBasisNotResolved}, nil
	}

	declared, err := handler.policies.LoadAsOfPolicies(
		ctx,
		prior.ResolutionKey().TenantID,
		rulePackage.Version(),
	)
	if err != nil {
		// 读不回与「规则包没声明」是两回事：前者等依赖恢复，后者等 `PAR-COM-14` 落地。合成
		// 一格，调用方就不知道该重试还是该催人去登记。
		return FormJudgmentAsOfResult{outcome: JudgmentAsOfPending}, nil
	}

	declaration, err := domain.DeclareAsOfPolicies(rulePackage.Version(), declared)
	if err != nil {
		// 一个空声明与一个不可用的规则包都落在这里，两者都是`未配置`：没有租户时端口必然交回
		// 空声明，而那正是首发要停下的地方，不是要绕过的地方。
		return FormJudgmentAsOfResult{outcome: JudgmentAsOfNotConfigured}, nil
	}

	formed := make(map[domain.JudgmentType]domain.JudgmentAsOf, len(command.Judgments))
	for _, requested := range command.Judgments {
		// 先问在不在，再形成。FormAsOf 对「没声明」与「值不合法」交回两个不同的错误，但都
		// 落在同一个 err 上；不在这里分开，一次未登记会被报成「你给的值不合法」，把人打发去
		// 改自己的请求，而真答案是等 `PAR-COM-14` 登记。
		if _, present := declaration.PolicyFor(requested.Judgment); !present {
			return FormJudgmentAsOfResult{outcome: JudgmentAsOfNotConfigured}, nil
		}
		asOf, err := declaration.FormAsOf(requested.Judgment, requested.At)
		if err != nil {
			// 政策在场却形不成，只剩值本身的问题——零值时点尤其不能被读成「此刻」。
			return FormJudgmentAsOfResult{outcome: JudgmentAsOfValueInvalid}, nil
		}
		formed[requested.Judgment] = asOf
	}

	return FormJudgmentAsOfResult{outcome: JudgmentAsOfFormed, formed: formed}, nil
}
