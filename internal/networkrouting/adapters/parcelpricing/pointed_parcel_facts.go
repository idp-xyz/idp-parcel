package parcelpricing

import (
	"context"
	"fmt"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件按判断点选包裹事实的来源（network-routing CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理
// 事实，到站复核使用当前有效物理实测」，ADR-0148 决定四第 7 条）。规则三条，全落在这里一处：
//
//   - 物理事实一项都没取得：退回客户声明出价，依据标 DECLARATION_FALLBACK，排序照常。复核一律排不了序的代价比一笔标明
//     来源的估价大，而「没称过」是收寄当时的常态。
//   - 物理事实在、却定不下用哪条（多于一条而没有已登记的取用规则）：不退回声明，各段待判断。客户声明不得顶替实测，
//     顶替之后的价看上去与正常出价没有区别，错得没有声响。
//   - 物理事实只有一部分（只称了重、没量外廓）：有的项取物理事实，其余项取声明，依据标 PHYSICAL_WITH_DECLARATION；
//     声明只补物理事实没有的项，不顶替任何一项已有的实测。

// PhysicalFactsAnswer 是物理事实读口的三格答复。三格的恢复动作各不相同（ADR-0029）：没取得的退回声明，定不下的等取用规则
// 登记，读不回的等依赖恢复（读口以错误作答，不在这三格里）。
type PhysicalFactsAnswer uint8

const (
	PhysicalFactsAnswerInvalid PhysicalFactsAnswer = iota
	// PhysicalFactsNotObtained 是截至该时点一项物理事实也没有取得。
	PhysicalFactsNotObtained
	// PhysicalFactsObtained 是取得了至少一项物理事实，且每项都定得下用哪一条。
	PhysicalFactsObtained
	// PhysicalFactsUnresolved 是物理事实在，某一项却有多条仍有效的实测而没有已登记的取用规则，机制不替租户挑「最近一次」。
	PhysicalFactsUnresolved
)

// PhysicalParcelFacts 是物理事实读口的答复。构造只走下面三个入口，让「取得了却什么都没带」立不起来。
type PhysicalParcelFacts struct {
	answer     PhysicalFactsAnswer
	weight     *ppdomain.Weight
	dimensions *ppdomain.Dimensions
	facts      []ppdomain.VersionedFactReference
}

func NoPhysicalFactsObtained() PhysicalParcelFacts {
	return PhysicalParcelFacts{answer: PhysicalFactsNotObtained}
}

func UnresolvedPhysicalFacts() PhysicalParcelFacts {
	return PhysicalParcelFacts{answer: PhysicalFactsUnresolved}
}

// ObtainedPhysicalFacts 立一份取得了的物理事实：重量与外廓至少有一项，且必带事实引用——没有引用就说不清价是按哪条实测出的。
func ObtainedPhysicalFacts(
	weight *ppdomain.Weight,
	dimensions *ppdomain.Dimensions,
	facts []ppdomain.VersionedFactReference,
) (PhysicalParcelFacts, error) {
	if weight == nil && dimensions == nil {
		return PhysicalParcelFacts{}, fmt.Errorf("network routing: obtained physical facts carry neither weight nor dimensions")
	}
	if len(facts) == 0 {
		return PhysicalParcelFacts{}, fmt.Errorf("network routing: obtained physical facts carry no fact reference")
	}
	return PhysicalParcelFacts{
		answer:     PhysicalFactsObtained,
		weight:     weight,
		dimensions: dimensions,
		facts:      append([]ppdomain.VersionedFactReference(nil), facts...),
	}, nil
}

func (facts PhysicalParcelFacts) Answer() PhysicalFactsAnswer { return facts.answer }

// PhysicalParcelFactsSource 交出一件包裹截至某一业务时点已经取得的物理事实（node-operations 的实际测量）。时点由判断点定：
// 首次收寄复核截到收寄的业务时间，到站复核与授权改路决定取判断此刻。
type PhysicalParcelFactsSource interface {
	PhysicalParcelFactsFor(
		ctx context.Context,
		key nrdomain.InitialRouteJudgmentKey,
		asOf time.Time,
	) (PhysicalParcelFacts, error)
}

// NoPhysicalParcelFacts 是没有物理事实来源的部署如实装的那一份：一项都没取得。它不是某个来源读空了，是根本没有来源，
// 所以复核恒退回客户声明、依据如实标 DECLARATION_FALLBACK，而不是假装读过。
type NoPhysicalParcelFacts struct{}

var _ PhysicalParcelFactsSource = NoPhysicalParcelFacts{}

func (NoPhysicalParcelFacts) PhysicalParcelFactsFor(
	context.Context, nrdomain.InitialRouteJudgmentKey, time.Time,
) (PhysicalParcelFacts, error) {
	return NoPhysicalFactsObtained(), nil
}

// PointedParcelFacts 按判断点把客户声明与物理事实两路来源合成一份包裹事实。
type PointedParcelFacts struct {
	declared DeclaredFactsSource
	physical PhysicalParcelFactsSource
	clock    Clock
}

func NewPointedParcelFacts(
	declared DeclaredFactsSource,
	physical PhysicalParcelFactsSource,
	clock Clock,
) (*PointedParcelFacts, error) {
	if declared == nil || physical == nil || clock == nil {
		return nil, fmt.Errorf("network routing: pointed parcel facts need a declared source, a physical source and a clock")
	}
	return &PointedParcelFacts{declared: declared, physical: physical, clock: clock}, nil
}

var _ ParcelFactsSource = (*PointedParcelFacts)(nil)

func (source *PointedParcelFacts) ParcelFactsFor(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	point nrdomain.ParcelFactsPoint,
) (ParcelFacts, bool, error) {
	if !point.Valid() {
		return ParcelFacts{}, false, fmt.Errorf("%w: parcel facts point is not one of the judgment points", errUntranslatableParcelFacts)
	}
	if !point.WantsPhysicalFacts() {
		return source.declaredAs(ctx, key, nrdomain.ParcelFactsDeclaration)
	}
	asOf, _ := point.PhysicalFactsAsOf(source.clock.Now())
	physical, err := source.physical.PhysicalParcelFactsFor(ctx, key, asOf)
	if err != nil {
		return ParcelFacts{}, false, fmt.Errorf("read physical parcel facts: %w", err)
	}
	switch physical.Answer() {
	case PhysicalFactsNotObtained:
		return source.declaredAs(ctx, key, nrdomain.ParcelFactsDeclarationFallback)
	case PhysicalFactsUnresolved:
		return ParcelFacts{Basis: nrdomain.ParcelFactsPhysicalUnresolved}, false, nil
	case PhysicalFactsObtained:
		return source.completeWithDeclaration(ctx, key, physical)
	default:
		return ParcelFacts{}, false, fmt.Errorf("%w: physical facts answer %d", errUntranslatableParcelFacts, physical.Answer())
	}
}

// declaredAs 以给定依据交出客户声明；声明也没有即一项事实都没有。
func (source *PointedParcelFacts) declaredAs(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	basis nrdomain.ParcelFactsBasis,
) (ParcelFacts, bool, error) {
	declared, found, err := source.declared.DeclaredFactsFor(ctx, key)
	if err != nil {
		return ParcelFacts{}, false, err
	}
	if !found {
		return ParcelFacts{Basis: nrdomain.ParcelFactsNone}, false, nil
	}
	declared.Basis = basis
	return declared, true, nil
}

// completeWithDeclaration 以物理事实为主，只在物理事实缺的项上补客户声明。两项物理事实都在时不读声明。
func (source *PointedParcelFacts) completeWithDeclaration(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	physical PhysicalParcelFacts,
) (ParcelFacts, bool, error) {
	weight, dimensions := physical.weight, physical.dimensions
	facts := append([]ppdomain.VersionedFactReference(nil), physical.facts...)
	basis := nrdomain.ParcelFactsPhysical
	if weight == nil || dimensions == nil {
		declared, found, err := source.declared.DeclaredFactsFor(ctx, key)
		if err != nil {
			return ParcelFacts{}, false, err
		}
		usedDeclaration := false
		if weight == nil {
			if !found {
				// 只量到了外廓，实重哪里都没有：不能凭一个外廓出价。
				return ParcelFacts{Basis: nrdomain.ParcelFactsNone}, false, nil
			}
			weight = &declared.Weight
			usedDeclaration = true
		}
		if dimensions == nil && found && declared.Dimensions != nil {
			dimensions = declared.Dimensions
			usedDeclaration = true
		}
		if usedDeclaration {
			facts = append(facts, declared.Facts...)
			basis = nrdomain.ParcelFactsPhysicalWithDeclaration
		}
	}
	return ParcelFacts{Weight: *weight, Dimensions: dimensions, Facts: facts, Basis: basis}, true, nil
}
