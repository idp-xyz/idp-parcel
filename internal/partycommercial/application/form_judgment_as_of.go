package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// JudgmentAsOfOutcome 是第二阶段的封闭结果集合。
//
// 四种未成形分开，因为要采取的动作各不相同：`未配置`等 `PAR-COM-14` 落地，`未决`等依赖恢复，
// `值不合法`要调用方改这次请求，`依据未解析`要它先回到第一阶段。压成一个「失败」，调用方就只
// 能靠猜——而其中只有一种是它自己能修的。
type JudgmentAsOfOutcome uint8

const (
	JudgmentAsOfOutcomeInvalid JudgmentAsOfOutcome = iota
	JudgmentAsOfFormed
	JudgmentAsOfBasisNotResolved
	JudgmentAsOfNotConfigured
	JudgmentAsOfPending
	JudgmentAsOfValueInvalid
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

// FormJudgmentAsOfCommand 的输入是第一阶段的结果本身，不是调用方另给的规则包：规则包必须由
// 第一阶段用独立锚点选出，自带一个进来就等于让它决定自己被选中的时间。
type FormJudgmentAsOfCommand struct {
	Prior     domain.CommercialClosure
	Judgments []JudgmentAsOfRequest
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
	policies ports.AsOfPolicyDeclaration
}

func NewFormJudgmentAsOfHandler(policies ports.AsOfPolicyDeclaration) *FormJudgmentAsOfHandler {
	return &FormJudgmentAsOfHandler{policies: policies}
}

// Handle 执行用例第二阶段：由第一阶段选出的规则包声明各下游判断的时点锚，消费方给出值，两者
// 在这里对起来并逐项校验。
//
// 全有或全无，不逐项部分成功：缺一项时调用方拿着半套时点去推进判断，而缺的那一项等的可能是实例
// 参数落地，不是重试。与第一阶段的引用闭包同一条道理——把已经解出的成员交回去，等于引诱调用方
// 在一份判定为不成立的依据上继续往下走。
func (handler *FormJudgmentAsOfHandler) Handle(
	ctx context.Context,
	command FormJudgmentAsOfCommand,
) (FormJudgmentAsOfResult, error) {
	// 先看第一阶段成没成。没有已选规则包就去问政策，等于替一个尚未选出的包声明时点锚，而调用
	// 方拿到时点锚会以为商业依据已经定了。
	rulePackage, selected := command.Prior.AdoptedFor(domain.AcceptanceRulePackageObject)
	if command.Prior.Outcome() != domain.UniquelyResolved || !selected {
		return FormJudgmentAsOfResult{outcome: JudgmentAsOfBasisNotResolved}, nil
	}

	declared, err := handler.policies.LoadAsOfPolicies(
		ctx,
		command.Prior.ResolutionKey().TenantID,
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
