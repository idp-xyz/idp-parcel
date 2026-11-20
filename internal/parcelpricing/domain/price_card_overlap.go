package domain

import "time"

// 价卡预览、批准与发布要告诉操作者：这个范围、方向与目的上，折完收尾之后还有哪些在册版本会和这张卡同时适用。
// 判据必须与选卡同一套，否则提示里的版本集合会和随后 ResolveInForce 答的适用冲突对不上。
// 这里只分类、不拦：新版先入册，旧版才经指名后继收尾（ADR-0234）。把重叠收成拒绝，后继就登不进去。

// PriceCardOverlapResolution 是一条重叠该怎么往下走。零值不是一种收尾。
type PriceCardOverlapResolution uint8

const (
	PriceCardOverlapResolutionInvalid PriceCardOverlapResolution = iota
	// PriceCardOverlapSuccessorCut：同一方案，后继晚于旧版选用起点，并且接得住旧版的尾。发布后对旧版走后继截止。
	PriceCardOverlapSuccessorCut
	// PriceCardOverlapRetirePredecessor：同一方案，后继起点不晚于旧版选用起点，并且接得住旧版的尾。截止会把旧版截没，发布后走退役。
	PriceCardOverlapRetirePredecessor
	// PriceCardOverlapGovernanceDecision：收尾口解决不了。不同方案，或同方案但后继盖不住旧版此刻的选用终点（再收尾会留空档）。
	PriceCardOverlapGovernanceDecision
)

func (resolution PriceCardOverlapResolution) String() string {
	switch resolution {
	case PriceCardOverlapSuccessorCut:
		return "SUCCESSOR_CUT"
	case PriceCardOverlapRetirePredecessor:
		return "RETIRE_PREDECESSOR"
	case PriceCardOverlapGovernanceDecision:
		return "GOVERNANCE_DECISION"
	default:
		return ""
	}
}

// Intersect 求两个合法半开区间的交。无上界用零值上界表示。
// 旧版截止在 T、新版恰好从 T 起时交是空的：半开区间首尾相接并不同时适用，选卡也不会把它们读成适用冲突。
func (period EffectivePeriod) Intersect(other EffectivePeriod) (EffectivePeriod, bool) {
	if !period.valid() || !other.valid() {
		return EffectivePeriod{}, false
	}
	start := period.startsAt
	if other.startsAt.After(start) {
		start = other.startsAt
	}
	var end time.Time
	switch {
	case period.endsAt.IsZero() && other.endsAt.IsZero():
		end = time.Time{}
	case period.endsAt.IsZero():
		end = other.endsAt
	case other.endsAt.IsZero():
		end = period.endsAt
	case !period.endsAt.After(other.endsAt):
		end = period.endsAt
	default:
		end = other.endsAt
	}
	if end.IsZero() || start.Before(end) {
		return EffectivePeriod{startsAt: start, endsAt: end}, true
	}
	return EffectivePeriod{}, false
}

// ServiceProductsCollide 答两张卡会不会在同一次选卡里落到同一层。
// 与 ResolveInForce 取层的口径相同：两边都不声明，同在不声明层，会一起变成适用冲突；一边指名、一边不声明，指名层优先，
// 两张卡分属不同层，永远不会互相报成适用冲突；两边都指名时，至少有一个共同产品才同层。
func ServiceProductsCollide(candidate, registered []ServiceProductReference) bool {
	if len(candidate) == 0 || len(registered) == 0 {
		return len(candidate) == 0 && len(registered) == 0
	}
	named := make(map[string]struct{}, len(registered))
	for _, product := range registered {
		named[product.String()] = struct{}{}
	}
	for _, product := range candidate {
		if _, found := named[product.String()]; found {
			return true
		}
	}
	return false
}

// PriceCardOverlap 是一条仍可选的在册版本与本卡适用期的交，以及下一步该怎么收尾。
// Plan 只留方案标识与版本，不带指纹：提示指的是册上那一版，不是某一次声明时手上的摘要。
type PriceCardOverlap struct {
	plan       VersionReference
	selection  EffectivePeriod
	overlap    EffectivePeriod
	samePlan   bool
	resolution PriceCardOverlapResolution
}

func (item PriceCardOverlap) Plan() VersionReference                 { return item.plan }
func (item PriceCardOverlap) Selection() EffectivePeriod             { return item.selection }
func (item PriceCardOverlap) Overlap() EffectivePeriod               { return item.overlap }
func (item PriceCardOverlap) SamePlan() bool                         { return item.samePlan }
func (item PriceCardOverlap) Resolution() PriceCardOverlapResolution { return item.resolution }

// ClassifyPriceCardOverlap 把「算不算同时适用的另一版」和「算的话下一步走哪一种」一次判完。
// 同方案同版本直接跳过：那是幂等重放或内容冲突，不是另一版。适用期没有交、或产品层根本不会撞，也不是重叠。
func ClassifyPriceCardOverlap(
	candidatePlan VersionReference,
	candidatePeriod EffectivePeriod,
	candidateProducts []ServiceProductReference,
	registeredPlan VersionReference,
	registeredSelection EffectivePeriod,
	registeredProducts []ServiceProductReference,
) (PriceCardOverlap, bool) {
	if candidatePlan.ID() == registeredPlan.ID() && candidatePlan.Version() == registeredPlan.Version() {
		return PriceCardOverlap{}, false
	}
	overlap, ok := candidatePeriod.Intersect(registeredSelection)
	if !ok || !ServiceProductsCollide(candidateProducts, registeredProducts) {
		return PriceCardOverlap{}, false
	}
	samePlan := candidatePlan.ID() == registeredPlan.ID()
	return PriceCardOverlap{
		plan:       withoutFingerprint(registeredPlan),
		selection:  registeredSelection,
		overlap:    overlap,
		samePlan:   samePlan,
		resolution: overlapResolution(samePlan, candidatePeriod, registeredSelection),
	}, true
}

// overlapResolution 与 CutForSuccessor、RetireForCoveringSuccessor 用同一对条件：接得住尾，再看起点谁早。
// 接不住（后继的选用终点到不了旧版的选用终点）时两口都会留空档，这里不假装能自动收尾。
func overlapResolution(samePlan bool, candidate, registered EffectivePeriod) PriceCardOverlapResolution {
	if !samePlan || !reachesEnd(candidate, registered) {
		return PriceCardOverlapGovernanceDecision
	}
	if candidate.StartsAt().After(registered.StartsAt()) {
		return PriceCardOverlapSuccessorCut
	}
	return PriceCardOverlapRetirePredecessor
}
