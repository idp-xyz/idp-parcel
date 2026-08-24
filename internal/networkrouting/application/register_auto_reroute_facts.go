package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 自动改路四条件事实目录（审计票 05）的登记用例。它站在写入口之前只作一件事：把不
// 完整的陈述挡在库外并把缺处指名。改善阈值、改路条件与权限、冻结边界的取值属
// PAR-NET-14 实例半边，本用例一个默认值都不补——缺键就拒、缺版本就拒、缺折算依据
// 就拒；补一个占位值会把「还没人陈述过」变成一份能进改路评估的事实。
//
// 事务不由本层开：环境事务由进程级入口给出（先例：register_network_catalog.go）。
// 幂等与冲突分界在编排：写口交回`已登记`后读回既有版本逐字段比，同则`已存在`、
// 异则`内容冲突`——绝不覆盖（同键同版本重的内容之争没有「后到为准」）。

// RegisterAutoRerouteFactsOutcome 是一次事实登记的应用处理结果。没有`未决`：登记是
// 管理动作，依赖调不通时没有一个如实的中间答案可记——那一路交回错误，重试即可。
type RegisterAutoRerouteFactsOutcome uint8

const (
	RegisterAutoRerouteFactsOutcomeInvalid RegisterAutoRerouteFactsOutcome = iota
	AutoRerouteFactsAccepted
	AutoRerouteFactsAlreadyExists
	AutoRerouteFactsConflict
	AutoRerouteFactsRefused
)

func (outcome RegisterAutoRerouteFactsOutcome) String() string {
	switch outcome {
	case AutoRerouteFactsAccepted:
		return "REGISTERED"
	case AutoRerouteFactsAlreadyExists:
		return "ALREADY_EXISTS"
	case AutoRerouteFactsConflict:
		return "CONTENT_CONFLICT"
	case AutoRerouteFactsRefused:
		return "REFUSED"
	default:
		return ""
	}
}

// AutoRerouteFactsRefusalReason 指名这一笔登记差在哪一格。逐格分开是因为恢复动作
// 各不相同：键不完整要补判断范围，版本缺要登记方给一个，依据缺违背「事实要说得出
// 按什么折的」，清单元素空白说不出缺的是哪条限制/责任。
type AutoRerouteFactsRefusalReason uint8

const (
	AutoRerouteFactsRefusalReasonNone AutoRerouteFactsRefusalReason = iota
	AutoRerouteFactsKeyIncomplete
	AutoRerouteFactsVersionMissing
	AutoRerouteFactsBasisMissing
	AutoRerouteFactsRestrictionBlank
	AutoRerouteFactsResponsibilityBlank
)

func (reason AutoRerouteFactsRefusalReason) String() string {
	switch reason {
	case AutoRerouteFactsKeyIncomplete:
		return "JUDGMENT_KEY_INCOMPLETE"
	case AutoRerouteFactsVersionMissing:
		return "VERSION_MISSING"
	case AutoRerouteFactsBasisMissing:
		return "STRATEGY_BASIS_MISSING"
	case AutoRerouteFactsRestrictionBlank:
		return "RESTRICTION_BLANK"
	case AutoRerouteFactsResponsibilityBlank:
		return "RESPONSIBILITY_BLANK"
	default:
		return ""
	}
}

// RegisterAutoRerouteFactsCommand 携带一版事实陈述。判断键整体随命令到达（租户在键
// 内，ADR-0003 的显式跨界同款）；清单以原始字符串到达，翻译成领域引用是本用例的受理
// 动作之一——空白元素在这里指名被拒，不留给库或读口炸。
type RegisterAutoRerouteFactsCommand struct {
	Key                         domain.InitialRouteJudgmentKey
	Version                     int
	PolicyAllowsAutomatic       bool
	AtControlledNode            bool
	OnlyUnexecutedAffected      bool
	UnresolvedRestrictions      []string
	OutstandingResponsibilities []string
	StrategyBasis               string
}

type RegisterAutoRerouteFactsResult struct {
	outcome RegisterAutoRerouteFactsOutcome
	refusal AutoRerouteFactsRefusalReason
}

func (result RegisterAutoRerouteFactsResult) Outcome() RegisterAutoRerouteFactsOutcome {
	return result.outcome
}

// RefusalReason 只在被拒时有值。
func (result RegisterAutoRerouteFactsResult) RefusalReason() AutoRerouteFactsRefusalReason {
	return result.refusal
}

type RegisterAutoRerouteFactsDeps struct {
	Registry ports.AutoRerouteFactsRegistry
	Clock    ports.Clock
}

type RegisterAutoRerouteFactsHandler struct {
	deps RegisterAutoRerouteFactsDeps
}

func NewRegisterAutoRerouteFactsHandler(
	deps RegisterAutoRerouteFactsDeps,
) *RegisterAutoRerouteFactsHandler {
	return &RegisterAutoRerouteFactsHandler{deps: deps}
}

// Handle 把一版事实陈述推进到目录：受理门 → 写入 → 幂等/冲突分界。
func (handler *RegisterAutoRerouteFactsHandler) Handle(
	ctx context.Context,
	command RegisterAutoRerouteFactsCommand,
) (RegisterAutoRerouteFactsResult, error) {
	refused := func(reason AutoRerouteFactsRefusalReason) RegisterAutoRerouteFactsResult {
		return RegisterAutoRerouteFactsResult{outcome: AutoRerouteFactsRefused, refusal: reason}
	}
	if !command.Key.MinimumIdentityEstablished() {
		return refused(AutoRerouteFactsKeyIncomplete), nil
	}
	if command.Version < 1 {
		return refused(AutoRerouteFactsVersionMissing), nil
	}
	if command.StrategyBasis == "" {
		return refused(AutoRerouteFactsBasisMissing), nil
	}

	restrictions := make([]domain.RestrictionReference, 0, len(command.UnresolvedRestrictions))
	for _, raw := range command.UnresolvedRestrictions {
		reference, err := domain.NewRestrictionReference(raw)
		if err != nil {
			return refused(AutoRerouteFactsRestrictionBlank), nil
		}
		restrictions = append(restrictions, reference)
	}
	responsibilities := make([]domain.ResponsibilityReference, 0, len(command.OutstandingResponsibilities))
	for _, raw := range command.OutstandingResponsibilities {
		reference, err := domain.NewResponsibilityReference(raw)
		if err != nil {
			return refused(AutoRerouteFactsResponsibilityBlank), nil
		}
		responsibilities = append(responsibilities, reference)
	}

	record := ports.AutoRerouteFactsRecord{
		Key:     command.Key,
		Version: command.Version,
		Facts: domain.AutoRerouteFacts{
			PolicyAllowsAutomatic:       command.PolicyAllowsAutomatic,
			AtControlledNode:            command.AtControlledNode,
			OnlyUnexecutedAffected:      command.OnlyUnexecutedAffected,
			UnresolvedRestrictions:      restrictions,
			OutstandingResponsibilities: responsibilities,
		},
		StrategyBasis: command.StrategyBasis,
		RegisteredAt:  handler.deps.Clock.Now(),
	}

	saved, err := handler.deps.Registry.RegisterAutoRerouteFacts(ctx, record)
	if err != nil {
		return RegisterAutoRerouteFactsResult{}, err
	}
	switch saved {
	case ports.AutoRerouteFactsRegistered:
		return RegisterAutoRerouteFactsResult{outcome: AutoRerouteFactsAccepted}, nil
	case ports.AutoRerouteFactsAlreadyRegistered:
		existing, found, err := handler.deps.Registry.FindAutoRerouteFacts(ctx, command.Key, command.Version)
		if err != nil {
			return RegisterAutoRerouteFactsResult{}, err
		}
		if !found {
			// 写口答已在册却读不回那一版：仓储不变量已破，响亮报错不折成业务答案。
			return RegisterAutoRerouteFactsResult{}, fmt.Errorf(
				"network routing: auto reroute facts reported registered but version %d not found",
				command.Version)
		}
		if sameAutoRerouteStatement(existing, record) {
			return RegisterAutoRerouteFactsResult{outcome: AutoRerouteFactsAlreadyExists}, nil
		}
		return RegisterAutoRerouteFactsResult{outcome: AutoRerouteFactsConflict}, nil
	default:
		return RegisterAutoRerouteFactsResult{}, fmt.Errorf(
			"network routing: unexpected auto reroute facts save outcome %d", saved)
	}
}

// sameAutoRerouteStatement 比两版陈述的内容：三个结论、两份清单（逐位，顺序也是内容
// ——重放同一份命令顺序不变）与折算依据。登记时刻不参与比对：重放的时钟不同，幂等
// 按内容认。
func sameAutoRerouteStatement(existing, incoming ports.AutoRerouteFactsRecord) bool {
	if existing.Facts.PolicyAllowsAutomatic != incoming.Facts.PolicyAllowsAutomatic ||
		existing.Facts.AtControlledNode != incoming.Facts.AtControlledNode ||
		existing.Facts.OnlyUnexecutedAffected != incoming.Facts.OnlyUnexecutedAffected ||
		existing.StrategyBasis != incoming.StrategyBasis {
		return false
	}
	if len(existing.Facts.UnresolvedRestrictions) != len(incoming.Facts.UnresolvedRestrictions) {
		return false
	}
	for i := range existing.Facts.UnresolvedRestrictions {
		if existing.Facts.UnresolvedRestrictions[i] != incoming.Facts.UnresolvedRestrictions[i] {
			return false
		}
	}
	if len(existing.Facts.OutstandingResponsibilities) != len(incoming.Facts.OutstandingResponsibilities) {
		return false
	}
	for i := range existing.Facts.OutstandingResponsibilities {
		if existing.Facts.OutstandingResponsibilities[i] != incoming.Facts.OutstandingResponsibilities[i] {
			return false
		}
	}
	return true
}
