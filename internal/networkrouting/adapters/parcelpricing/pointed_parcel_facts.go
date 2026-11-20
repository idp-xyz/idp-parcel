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
//   - 物理事实取得了：重量与外廓整份取用，依据标 PHYSICAL，客户声明一格都不读、不补。物理事实的来源（node-operations 的
//     实际测量）一条同时带毛重与长宽高，所以取得了的物理事实重量与外廓必然都在；拿声明去补其中一项，会造出重量是实测、
//     外廓是客户声明的混合快照，与计价「实测整份取用」的口径相反。
//   - 物理事实一项都没取得：退回客户声明出价，依据标 DECLARATION_FALLBACK，排序照常。复核一律排不了序的代价比一笔标明
//     来源的估价大，而「没称过」是收寄当时的常态。
//   - 物理事实在、却定不下用哪条：不退回声明，各段待判断。客户声明不得顶替实测，
//     顶替之后的价看上去与正常出价没有区别，错得没有声响。

// PhysicalFactsAnswer 是物理事实读口的三格答复。三格的恢复动作各不相同（ADR-0029）：没取得的退回声明，定不下的等取用规则
// 登记，读不回的等依赖恢复（读口以错误作答，不在这三格里）。
type PhysicalFactsAnswer uint8

const (
	PhysicalFactsAnswerInvalid PhysicalFactsAnswer = iota
	// PhysicalFactsNotObtained 是截至该时点一项物理事实也没有取得。
	PhysicalFactsNotObtained
	// PhysicalFactsObtained 是取得了物理事实，且定得下用哪一条：重量与外廓都在，整份取用。
	PhysicalFactsObtained
	// PhysicalFactsUnresolved 是物理事实在，却定不下用哪一条。计价读口答输入不可得时落这一格：多件实物、多条无采用、
	// 发生时间并列、单位或数值译不出。机制不替租户挑一条，也不退回客户声明。
	PhysicalFactsUnresolved
)

// PhysicalParcelFacts 是物理事实读口的答复。构造只走下面三个入口，让「取得了却什么都没带」立不起来：取得了的一份，
// 重量与外廓是值而不是指针，没有「只带一样」的写法。
type PhysicalParcelFacts struct {
	answer     PhysicalFactsAnswer
	weight     ppdomain.Weight
	dimensions ppdomain.Dimensions
	facts      []ppdomain.VersionedFactReference
}

func NoPhysicalFactsObtained() PhysicalParcelFacts {
	return PhysicalParcelFacts{answer: PhysicalFactsNotObtained}
}

func UnresolvedPhysicalFacts() PhysicalParcelFacts {
	return PhysicalParcelFacts{answer: PhysicalFactsUnresolved}
}

// ObtainedPhysicalFacts 立一份取得了的物理事实：重量与外廓都必带，且必带事实引用——没有引用就说不清价是按哪条实测出的。
// 重量与外廓取值而不取指针：来源的一条实测同时带毛重与长宽高，「只带一样」的物理事实无从产生，签名上也就没有写法；
// 没经 ppdomain 构造门的零值同样当作没带，报错。
func ObtainedPhysicalFacts(
	weight ppdomain.Weight,
	dimensions ppdomain.Dimensions,
	facts []ppdomain.VersionedFactReference,
) (PhysicalParcelFacts, error) {
	// ppdomain 不导出合法性判断；两者的字段都只经构造门填，合法值的单位必非空，零值的单位必为空。
	if weight.Unit() == "" {
		return PhysicalParcelFacts{}, fmt.Errorf("network routing: obtained physical facts carry no weight")
	}
	if dimensions.Unit() == "" {
		return PhysicalParcelFacts{}, fmt.Errorf("network routing: obtained physical facts carry no dimensions")
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

// PointedParcelFacts 按判断点在客户声明与物理事实两路来源里整份选一路作包裹事实，两路不混用。
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
		dimensions := physical.dimensions
		return ParcelFacts{
			Weight:     physical.weight,
			Dimensions: &dimensions,
			Facts:      append([]ppdomain.VersionedFactReference(nil), physical.facts...),
			Basis:      nrdomain.ParcelFactsPhysical,
		}, true, nil
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
