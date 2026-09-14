// form_evaluation_from_request.go 编排 UC-SA-002 步 2 后半在 parcel-pricing 这一侧的「按评价请求形成评价」
// （票 sa-cc/11 裁决 1）：settlement-accounting 登记一份评价请求并经信封交出引用，本上下文按引用取回主要范围、
// 计算目的、合格来源引用与计价基准时点，三步形成评价——① 解析在用价卡版本 → ② 造不可变计价输入快照 →
// ③ 造 domain.EvaluationRequest 交**既有** EvaluatePricingHandler.Handle。幂等 / 入册 / 交付照旧由它承担，本文件
// 不改它、不绕它。这是 PP 第一条生产形成路：此前评价只由测试与 HTTP 回放门形成。
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrNilDependency 是构造期漏装一口的哨兵（形照 SA application 同名哨兵）：装配方按 errors.Is 能把「装配漏了」
// 与别的构造错误分开。
var ErrNilDependency = errors.New("parcel pricing: nil dependency")

// ErrUnexpectedFormationOutcome 说明既有评价编排交回了本入口结构上到不了的结果——新铸的标识不该在册，所以
// `已存在` 与 `冲突` 两格在这里只可能是标识撞号或替身装错；静默入账等于替编排作判断，响亮报错。
var ErrUnexpectedFormationOutcome = errors.New("parcel pricing: unexpected evaluation formation outcome")

// FormEvaluationFromRequestOutcome 是「按评价请求形成评价」的封闭结果。按恢复动作分格（ADR-0029）：已形成 /
// 已存在是答案；未配置要去登记价卡；适用冲突要人裁；输入不可得要等提供方那一侧的读口；未受理要改请求；
// 未决要等依赖。
type FormEvaluationFromRequestOutcome uint8

const (
	FormEvaluationFromRequestOutcomeInvalid FormEvaluationFromRequestOutcome = iota
	// RequestEvaluationFormed：评价已形成并入册，回指在评价上。
	RequestEvaluationFormed
	// RequestEvaluationExisting：同一请求已有评价（按回指命中）——同一请求不形成第二份（票面做法 3）。
	RequestEvaluationExisting
	// RequestPriceCardNotConfigured：（租户、范围、方向、目的、时点）下没有价卡。缺的是价卡。
	RequestPriceCardNotConfigured
	// RequestPriceCardApplicabilityConflict：多于一版价卡同时适用，候选随结果交人裁；不挑。
	RequestPriceCardApplicabilityConflict
	// RequestPricingInputUnavailable：造不出计价输入快照——缺哪几只读口随结果点名（裁决 4）。
	RequestPricingInputUnavailable
	// RequestEvaluationNotAccepted：命令不成形。
	RequestEvaluationNotAccepted
	// RequestEvaluationUndecided：依赖故障，形成与否未知，停在哪一口见 Reason。
	RequestEvaluationUndecided
)

func (outcome FormEvaluationFromRequestOutcome) String() string {
	switch outcome {
	case RequestEvaluationFormed:
		return "FORMED"
	case RequestEvaluationExisting:
		return "EXISTING"
	case RequestPriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case RequestPriceCardApplicabilityConflict:
		return "PRICE_CARD_APPLICABILITY_CONFLICT"
	case RequestPricingInputUnavailable:
		return "PRICING_INPUT_UNAVAILABLE"
	case RequestEvaluationNotAccepted:
		return "NOT_ACCEPTED"
	case RequestEvaluationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// FormEvaluationFromRequestUndecidedReason 指名未决停在哪一口。
type FormEvaluationFromRequestUndecidedReason uint8

const (
	FormEvaluationFromRequestUndecidedReasonNone FormEvaluationFromRequestUndecidedReason = iota
	RequestEvaluationRegistryUnavailable
	RequestPriceCardResolutionUnavailable
	RequestPricingInputResolutionUnavailable
	RequestEvaluationIdentityUnavailable
	RequestEvaluationFormationUndecided
)

func (reason FormEvaluationFromRequestUndecidedReason) String() string {
	switch reason {
	case RequestEvaluationRegistryUnavailable:
		return "EVALUATION_REGISTRY_UNAVAILABLE"
	case RequestPriceCardResolutionUnavailable:
		return "PRICE_CARD_RESOLUTION_UNAVAILABLE"
	case RequestPricingInputResolutionUnavailable:
		return "PRICING_INPUT_RESOLUTION_UNAVAILABLE"
	case RequestEvaluationIdentityUnavailable:
		return "EVALUATION_IDENTITY_UNAVAILABLE"
	case RequestEvaluationFormationUndecided:
		return "EVALUATION_FORMATION_UNDECIDED"
	default:
		return ""
	}
}

// FormEvaluationFromRequestCommand 是一份 SA 评价请求在本上下文词汇里的转述：全部来自 SA 评价请求读口，由消费侧
// 适配器按信封所指那一份取回并译成这里的类型。方向与目的成对交进来（SA 的 BUY_SUPPLIER_COST 译成 BUY +
// SUPPLIER_COST）；计价基准时点是发生项的业务时间——它既定在用价卡的选版，也进快照的 businessAt。证据层级由
// 装配方声明、不给默认：它是来源数据的属性，编排不知道来源是什么。金额、币种、换算一格都没有（ADR-0107）。
type FormEvaluationFromRequestCommand struct {
	Tenant    domain.TenantID
	Request   domain.EvaluationRequestReference
	Scope     domain.PricingScopeID
	Direction domain.PricingDirection
	Purpose   domain.PricingPurpose
	BasisAt   time.Time
	Sources   ports.EligibleSourceReferences
	Evidence  domain.EvidenceKind
}

// FormEvaluationFromRequestResult 是入口的答复。字段导出、不设访问器，形状随 ReplayPricingEvaluationResult：
// 消费侧要把它落成消费两格，测试要能造出任一格。
type FormEvaluationFromRequestResult struct {
	Outcome FormEvaluationFromRequestOutcome
	// Reason 只在 RequestEvaluationUndecided 时非零。
	Reason FormEvaluationFromRequestUndecidedReason
	// Evaluation 在已形成 / 已存在两格有值：刚入册那一份，或按回指读回的先到者。
	Evaluation    domain.PricingEvaluation
	HasEvaluation bool
	// HandoffReference 非空说明评价已入册但意图还没交出去（照 EvaluatePricingResult 那一格转述）。
	HandoffReference string
	// Candidates 只在适用冲突时有值：同时适用的每一版价卡的引用。
	Candidates []domain.VersionReference
	// Missing 只在输入不可得时有值：缺的读口或事实，一条一件，文字给人读。
	Missing []string
}

// FormEvaluationFromRequestDeps 是入口的依赖。四口必备：回指读口守「同一请求一份评价」、解析读口是第 ① 步、
// 铸造口签评价标识、既有评价编排是第 ③ 步。Inputs 可缺席，是有意的：造快照所需的三只跨上下文读口今天不存在
// （裁决 4），生产装配不接任何实现，入口对缺席答「输入不可得」并点名它们——不用一个永远答「不在」的替身顶上。
type FormEvaluationFromRequestDeps struct {
	Evaluations ports.EvaluationByRequestView
	PriceCards  ports.PriceCardInForceResolver
	Identity    ports.EvaluationIdentityFactory
	Evaluate    *EvaluatePricingHandler
	Inputs      ports.PricingInputResolver
}

// FormEvaluationFromRequestHandler 是「按评价请求形成评价」的编排。计算一步都不在这里；它只做解析、造快照、
// 铸标识、带回指、转交，与把既有编排的答案折成本入口的格。
type FormEvaluationFromRequestHandler struct {
	deps FormEvaluationFromRequestDeps
}

// NewFormEvaluationFromRequestHandler 构造期拒 nil：漏装一口在这里就报出来，而不是等第一封信封到达时在解引用处
// panic。Evaluate 收具体类型而不收接口，是让「交既有 EvaluatePricingHandler、不绕它」在结构上成立。
func NewFormEvaluationFromRequestHandler(deps FormEvaluationFromRequestDeps) (*FormEvaluationFromRequestHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"evaluation by request view", deps.Evaluations == nil},
		{"price card in-force resolver", deps.PriceCards == nil},
		{"evaluation identity factory", deps.Identity == nil},
		{"evaluate pricing handler", deps.Evaluate == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &FormEvaluationFromRequestHandler{deps: deps}, nil
}

// missingInputReadPorts 是今天造不出快照的原因（裁决 4 的量）：快照要的四样里只有业务时点在 SA 请求上，其余三样
// 各归一只本上下文没有的读口。写成三条是让「等谁」可读——三只读口各归不同上下文、各是一张票。
var missingInputReadPorts = []string{
	"transport-fulfillment: no read-only view of the charge occurrence's member carried objects (evaluation subject)",
	"node-operations / parcel-shipment: no read port for measured or declared actual weight and dimensions (pricing weight)",
	"parcel-shipment: no read port for the origin / destination postal route (zone)",
}

// Handle 把一份评价请求推进到本入口的答案：受理 → 按回指找先到者（有则 `已存在`）→ ① 解析在用价卡（未配置 /
// 适用冲突各自成格）→ ② 造快照（没有读口或读口答不可得都是 `输入不可得`）→ 铸标识 → ③ 造评价请求、带回指、
// 交 EvaluatePricingHandler。铸标识放在造快照之后：停在前几格的请求不该消耗一个谁都不会引用的标识。
//
// 依赖故障不上抛而答 `未决` 并指名停在哪一口：消费者据此重投，与 EvaluatePricingHandler 对依赖故障的处置同形；
// 只有结构上到不了的答案才是 error。
func (handler *FormEvaluationFromRequestHandler) Handle(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
) (FormEvaluationFromRequestResult, error) {
	if !command.accepted() {
		return FormEvaluationFromRequestResult{Outcome: RequestEvaluationNotAccepted}, nil
	}

	existing, found, err := handler.deps.Evaluations.FindByRequestReference(ctx, command.Tenant, command.Request)
	if err != nil {
		return undecidedFormation(RequestEvaluationRegistryUnavailable), nil
	}
	if found {
		return FormEvaluationFromRequestResult{Outcome: RequestEvaluationExisting, Evaluation: existing, HasEvaluation: true}, nil
	}

	resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, command.Scope, command.Direction, command.Purpose, command.BasisAt)
	if err != nil {
		return undecidedFormation(RequestPriceCardResolutionUnavailable), nil
	}
	switch resolution.Outcome {
	case ports.PriceCardVersionInForce:
	case ports.PriceCardNotConfigured:
		return FormEvaluationFromRequestResult{Outcome: RequestPriceCardNotConfigured}, nil
	case ports.PriceCardApplicabilityConflict:
		return FormEvaluationFromRequestResult{
			Outcome:    RequestPriceCardApplicabilityConflict,
			Candidates: append([]domain.VersionReference(nil), resolution.Candidates...),
		}, nil
	default:
		return FormEvaluationFromRequestResult{}, fmt.Errorf("%w: price card in-force outcome %d", ErrUnexpectedInForceOutcome, resolution.Outcome)
	}

	input, result, stopped := handler.resolveInput(ctx, command)
	if stopped {
		return result, nil
	}

	id, err := handler.deps.Identity.MintEvaluationID(ctx)
	if err != nil {
		return undecidedFormation(RequestEvaluationIdentityUnavailable), nil
	}
	request, err := domain.NewEvaluationRequest(id, resolution.Plan, input, command.Evidence)
	if err != nil {
		return FormEvaluationFromRequestResult{Outcome: RequestEvaluationNotAccepted}, nil
	}
	request, err = request.WithRequestReference(command.Request)
	if err != nil {
		return FormEvaluationFromRequestResult{Outcome: RequestEvaluationNotAccepted}, nil
	}

	formed, err := handler.deps.Evaluate.Handle(ctx, EvaluatePricingCommand{Request: request})
	if err != nil {
		return FormEvaluationFromRequestResult{}, err
	}
	switch formed.Outcome() {
	case EvaluationRecorded:
		evaluation, _ := formed.Evaluation()
		return FormEvaluationFromRequestResult{
			Outcome:          RequestEvaluationFormed,
			Evaluation:       evaluation,
			HasEvaluation:    true,
			HandoffReference: formed.HandoffReference(),
		}, nil
	case EvaluationUndecided:
		// 含「同租户同回指第二份撞迁移 0010 的唯一索引、按新标识读不回先到者」那一格：编排答未决，重投时
		// 按回指命中 `已存在`。
		return undecidedFormation(RequestEvaluationFormationUndecided), nil
	default:
		return FormEvaluationFromRequestResult{}, fmt.Errorf("%w: %s", ErrUnexpectedFormationOutcome, formed.Outcome())
	}
}

// resolveInput 是第 ② 步。没接读口与读口答不可得落同一格，缺项来路不同：前者由本入口点名三只读口，后者照读口
// 的话转述、不替它补。stopped 为真时 result 就是入口的答案。
func (handler *FormEvaluationFromRequestHandler) resolveInput(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
) (domain.PricingInputSnapshot, FormEvaluationFromRequestResult, bool) {
	if handler.deps.Inputs == nil {
		return domain.PricingInputSnapshot{}, FormEvaluationFromRequestResult{
			Outcome: RequestPricingInputUnavailable,
			Missing: append([]string(nil), missingInputReadPorts...),
		}, true
	}
	resolved, err := handler.deps.Inputs.ResolvePricingInput(ctx, ports.PricingInputQuery{
		Tenant:  command.Tenant,
		Scope:   command.Scope,
		Purpose: command.Purpose,
		BasisAt: command.BasisAt,
		Sources: command.Sources,
	})
	if err != nil {
		return domain.PricingInputSnapshot{}, undecidedFormation(RequestPricingInputResolutionUnavailable), true
	}
	switch resolved.Outcome {
	case ports.PricingInputResolved:
		return resolved.Input, FormEvaluationFromRequestResult{}, false
	case ports.PricingInputUnavailable:
		return domain.PricingInputSnapshot{}, FormEvaluationFromRequestResult{
			Outcome: RequestPricingInputUnavailable,
			Missing: append([]string(nil), resolved.Missing...),
		}, true
	default:
		// 封闭集之外照未决处置——读口装错了，形成与否未知，不静默当成可得。
		return domain.PricingInputSnapshot{}, undecidedFormation(RequestPricingInputResolutionUnavailable), true
	}
}

// accepted 是受理门：每一格都要在，方向与目的要配对——配错的一对不该被译成「未配置」（那会让人去登记一张本就
// 不该存在的卡），配对表由领域一处持有。三件来源引用里只查发生项（身份 + 版本）：它是步 ② 唯一能拿去键
// 事实的引用，业务时点也从它上面来；费用项目与供应商协议是 SA 那一侧的钥匙，本入口不读它们的内容，缺了由
// 输入读口按自己的需要点名。
func (command FormEvaluationFromRequestCommand) accepted() bool {
	if command.Tenant.String() == "" || command.Request.String() == "" || command.Scope.String() == "" ||
		command.BasisAt.IsZero() {
		return false
	}
	if !command.Sources.OccurrenceReferenced() {
		return false
	}
	if !command.Purpose.PairsWithDirection(command.Direction) {
		return false
	}
	return command.Evidence.Declared()
}

func undecidedFormation(reason FormEvaluationFromRequestUndecidedReason) FormEvaluationFromRequestResult {
	return FormEvaluationFromRequestResult{Outcome: RequestEvaluationUndecided, Reason: reason}
}
