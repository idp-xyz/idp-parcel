package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardClosureOutcome 是价卡收尾两口（截止与退役，ADR-0234）共用的结果代数：`已登记`是写下了一次新的截止或退役，
// `已登记（重放）`是同一次收尾再来一遍、一行不写，`未受理`是这一次收尾不成立、一行不写——原因随 Cause 交回。
//
// 没有「内容冲突」格：截止那一侧，同一版上依据不同的截止是合法的下一条，入口拒掉的只是截止本身不成立；退役那一侧，旧版
// 已凭别的依据退役，登记方要做的是改请求，与其余不成立的收尾同一种续办。
type PriceCardClosureOutcome uint8

const (
	PriceCardClosureOutcomeInvalid PriceCardClosureOutcome = iota
	PriceCardClosureRecorded
	PriceCardClosureAlreadyOnRegister
	PriceCardClosureNotAccepted
)

func (outcome PriceCardClosureOutcome) String() string {
	switch outcome {
	case PriceCardClosureRecorded:
		return "REGISTERED"
	case PriceCardClosureAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case PriceCardClosureNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// PriceCardClosureResult 携带落点、被拒的原因，以及落下（或重放认出）的那一次收尾：截止交回截止点，退役交回退役时刻——登记
// 方据它看到旧版此后选到哪一刻、或从哪一刻起不再参选。
type PriceCardClosureResult struct {
	outcome       PriceCardClosureOutcome
	cause         error
	cut           domain.PriceCardCut
	hasCut        bool
	retirement    domain.PriceCardRetirement
	hasRetirement bool
}

func (result PriceCardClosureResult) Outcome() PriceCardClosureOutcome { return result.outcome }

func (result PriceCardClosureResult) Cause() error { return result.cause }

func (result PriceCardClosureResult) Cut() (domain.PriceCardCut, bool) {
	return result.cut, result.hasCut
}

func (result PriceCardClosureResult) Retirement() (domain.PriceCardRetirement, bool) {
	return result.retirement, result.hasRetirement
}

func priceCardClosureNotAccepted(cause error) PriceCardClosureResult {
	return PriceCardClosureResult{outcome: PriceCardClosureNotAccepted, cause: cause}
}

// PriceCardClosureTarget 是两口共用的那一半：本租户哪一定价方案的哪一版，让位给同一方案的哪一版。方案只写一次，命令的
// 形状里装不下「后继是另一个方案」。
type PriceCardClosureTarget struct {
	Tenant    domain.TenantID
	PlanID    string
	Version   string
	Successor string
}

// CutPriceCardVersionCommand 把旧版截止在后继的起点。没有截止点一格：截止点取后继此刻的选用起点，给了就多一处会与后继对
// 不上的格。Reference 是登记方给的依据。
type CutPriceCardVersionCommand struct {
	PriceCardClosureTarget
	Reference string
}

// RetirePriceCardVersionCommand 让旧版整段让位给起点不晚于它的后继。没有退役时刻一格：退役时刻取登记时刻。
type RetirePriceCardVersionCommand struct {
	PriceCardClosureTarget
	Reference string
}

// ClosePriceCardVersionDeps 是价卡收尾的依赖。Registered 交「参数已登记」：后继登记时交的那一封续办出来的请求，在两版同时
// 适用时只会停在适用冲突上，能把它们续办到后继上的是收尾这一封（ADR-0234 决定四），所以必备。
type ClosePriceCardVersionDeps struct {
	Registry   ports.PriceCardClosureRegistry
	Registered ports.PricingParameterRegisteredHandoff
	Clock      ports.Clock
}

// ClosePriceCardVersionHandler 是价卡收尾的写侧编排：锁旧版行 → 读回两版此刻在册上的样子 → 领域判收尾 → 写截止或退役 →
// 同笔交「参数已登记」。调用方逐命令各起事务；写口无事务即拒。
type ClosePriceCardVersionHandler struct {
	deps ClosePriceCardVersionDeps
}

// NewClosePriceCardVersionHandler 构造期拒漏装：缺了交接口收尾照样落库，停着的请求却再也等不来续办。
func NewClosePriceCardVersionHandler(deps ClosePriceCardVersionDeps) (*ClosePriceCardVersionHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"price card closure registry", deps.Registry == nil},
		{"pricing parameter registered handoff", deps.Registered == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &ClosePriceCardVersionHandler{deps: deps}, nil
}

// Cut 落一次截止。领域交回的每一种拒绝都答`未受理`：它们都是输入或册面状态不允许这一次收尾，重试同一份请求不会变好，要
// 登记方改请求或先去登后继；读写与交信封失败才是 error，整笔回滚，端点按「没形成答案」答。
func (handler *ClosePriceCardVersionHandler) Cut(
	ctx context.Context,
	command CutPriceCardVersionCommand,
) (PriceCardClosureResult, error) {
	const operation = "cut price card version at successor"
	pair, refused, err := handler.lockAndLoad(ctx, command.PriceCardClosureTarget, command.Reference)
	if err != nil {
		return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if refused != nil {
		return priceCardClosureNotAccepted(refused), nil
	}
	cut, replay, err := pair.old.CutForSuccessor(pair.next, pair.reference, handler.deps.Clock.Now())
	if err != nil {
		return priceCardClosureNotAccepted(err), nil
	}
	result := PriceCardClosureResult{outcome: PriceCardClosureAlreadyOnRegister, cut: cut, hasCut: true}
	if !replay {
		saved, err := handler.deps.Registry.SavePriceCardCut(ctx, cut)
		if err != nil {
			return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
		}
		switch saved {
		case ports.PriceCardClosureSaved:
			result.outcome = PriceCardClosureRecorded
		case ports.PriceCardClosureAlreadyRegistered:
			// 锁之后读回的那一版里本该已认出重放；走到这里说明锁与读之间的约定被打破了，仍按库答，时刻却是这一次的——
			// 不交回一条册上没有的截止。
			result = PriceCardClosureResult{outcome: PriceCardClosureAlreadyOnRegister}
		default:
			return PriceCardClosureResult{}, fmt.Errorf("%s: unexpected save outcome %d", operation, saved)
		}
	}
	occasion := ports.PriceCardClosureOccasion{Kind: ports.PriceCardCutClosure, Successor: command.Successor, Reference: command.Reference}
	if err := handler.announce(ctx, pair.old, occasion); err != nil {
		return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return result, nil
}

// RetireAtCoveringSuccessor 落一次退役。拒绝的分格同 Cut。
func (handler *ClosePriceCardVersionHandler) RetireAtCoveringSuccessor(
	ctx context.Context,
	command RetirePriceCardVersionCommand,
) (PriceCardClosureResult, error) {
	const operation = "retire price card version for a covering successor"
	pair, refused, err := handler.lockAndLoad(ctx, command.PriceCardClosureTarget, command.Reference)
	if err != nil {
		return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if refused != nil {
		return priceCardClosureNotAccepted(refused), nil
	}
	retirement, replay, err := pair.old.RetireForCoveringSuccessor(pair.next, pair.reference, handler.deps.Clock.Now())
	if err != nil {
		return priceCardClosureNotAccepted(err), nil
	}
	result := PriceCardClosureResult{outcome: PriceCardClosureAlreadyOnRegister, retirement: retirement, hasRetirement: true}
	if !replay {
		saved, err := handler.deps.Registry.SavePriceCardRetirement(ctx, retirement)
		if err != nil {
			return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
		}
		switch saved {
		case ports.PriceCardClosureSaved:
			result.outcome = PriceCardClosureRecorded
		case ports.PriceCardClosureAlreadyRegistered:
			// 判据同 Cut 那一格。
			result = PriceCardClosureResult{outcome: PriceCardClosureAlreadyOnRegister}
		case ports.PriceCardRetiredOtherwise:
			return priceCardClosureNotAccepted(domain.ErrPriceCardRetiredOtherwise), nil
		default:
			return PriceCardClosureResult{}, fmt.Errorf("%s: unexpected save outcome %d", operation, saved)
		}
	}
	occasion := ports.PriceCardClosureOccasion{Kind: ports.PriceCardRetirementClosure, Successor: command.Successor, Reference: command.Reference}
	if err := handler.announce(ctx, pair.old, occasion); err != nil {
		return PriceCardClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return result, nil
}

// closurePair 是两口在判之前取齐的那一对：两版此刻在册上的样子与依据。
type closurePair struct {
	old, next domain.PriceCardStanding
	reference domain.PriceCardClosureReference
}

// lockAndLoad 是两口共用的前半段：先锁旧版那一行，再读回两版（顺序的理由在 ports.PriceCardClosureRegistry）。refused 非空是
// 这一次收尾不受理的原因——命令缺格、旧版或后继不在本租户的册上——不是故障。
func (handler *ClosePriceCardVersionHandler) lockAndLoad(
	ctx context.Context,
	target PriceCardClosureTarget,
	rawReference string,
) (pair closurePair, refused error, err error) {
	if target.Tenant.String() == "" {
		return closurePair{}, fmt.Errorf("%w: tenant", domain.ErrInvalidPriceCardClosure), nil
	}
	old, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, target.PlanID, target.Version)
	if err != nil {
		return closurePair{}, fmt.Errorf("%w: predecessor: %v", domain.ErrInvalidPriceCardClosure, err), nil
	}
	next, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, target.PlanID, target.Successor)
	if err != nil {
		return closurePair{}, fmt.Errorf("%w: successor: %v", domain.ErrInvalidPriceCardClosure, err), nil
	}
	reference, err := domain.NewPriceCardClosureReference(rawReference)
	if err != nil {
		return closurePair{}, fmt.Errorf("%w: reference: %v", domain.ErrInvalidPriceCardClosure, err), nil
	}
	if target.Version == target.Successor {
		return closurePair{}, domain.ErrPriceCardClosureSameVersion, nil
	}

	locked, err := handler.deps.Registry.LockPriceCardVersionForClosure(ctx, target.Tenant, old)
	if err != nil {
		return closurePair{}, nil, err
	}
	if !locked {
		return closurePair{}, fmt.Errorf("%w: predecessor %s/%s", domain.ErrPriceCardClosureVersionNotRegistered, target.PlanID, target.Version), nil
	}
	oldStanding, found, err := handler.deps.Registry.LoadPriceCardStanding(ctx, target.Tenant, old)
	if err != nil {
		return closurePair{}, nil, err
	}
	if !found {
		return closurePair{}, nil, fmt.Errorf("predecessor %s/%s is locked but cannot be read back", target.PlanID, target.Version)
	}
	nextStanding, found, err := handler.deps.Registry.LoadPriceCardStanding(ctx, target.Tenant, next)
	if err != nil {
		return closurePair{}, nil, err
	}
	if !found {
		return closurePair{}, fmt.Errorf("%w: successor %s/%s", domain.ErrPriceCardClosureVersionNotRegistered, target.PlanID, target.Successor), nil
	}
	return closurePair{old: oldStanding, next: nextStanding, reference: reference}, nil, nil
}

// announce 在收尾落库（或重放认出）之后交「参数已登记」，种类是价卡，带旧版的计价范围与方向——后继与它同一范围、同一方向，
// 停着的请求按这一面续办。重放重发同一封，由交接口按同一个标识去重。交接失败上抛，信封与收尾行同生共死。
func (handler *ClosePriceCardVersionHandler) announce(
	ctx context.Context,
	old domain.PriceCardStanding,
	occasion ports.PriceCardClosureOccasion,
) error {
	plan := old.Plan()
	return announceOnScopes(ctx, handler.deps.Registered, ports.PricingParameterRegisteredIntent{
		Tenant:       old.Tenant(),
		Kind:         ports.PricingParameterPriceCard,
		Registration: plan.Reference(),
		Closure:      occasion,
	}, []ports.PricedScope{{Scope: plan.Scope(), Direction: plan.Direction()}})
}
