package domain

import (
	"errors"
	"fmt"
	"time"
)

// 价卡换版时旧版只经显式收尾离开新的选择（ADR-0234）。按计价范围选卡的口（结算交来的评价请求、试算）在同一范围、方向与
// 目的上见到两版同时适用就答`适用冲突`，而已发布的版本不可原地覆盖（CONTEXT）——新的一版登进来，旧版不会自己退场。
// 登记方指名同一定价方案的旧版与后继，收尾有两种，由两版此刻的选用起点决定走哪一种、各走各的口：
//
//   - 截止：后继晚于旧版起点。旧版此刻的选用区间到后继起点为止，前一段照旧可选。截止点不由登记方给，取后继此刻的
//     选用起点：给了就多一处会与后继对不上的格。
//   - 退役：后继不晚于旧版起点（同一份依据重发，或连起点一起改正）。截在起点上旧版一刻不剩，区间表达不了；旧版自退役
//     登记起不再参与任何新的选择。
//
// 两种都要求后继接得住旧版此后的那一段，否则收尾会把原本唯一选中旧版的时段变成没有在用价卡。版本快照与发布时的
// 适用期一字不改：收尾是册上追加的事实，装载时施加到选择上。新版登记本身不收尾旧版。

var (
	// ErrInvalidPriceCardClosure：收尾缺格——租户、方案版本、后继、依据或登记时刻有一样立不住，或两版不在同一租户、
	// 同一定价方案下。命令的形状里方案只写一次，后者只可能是装配把两版读错了。
	ErrInvalidPriceCardClosure = errors.New("parcel pricing: invalid price card closure")
	// ErrPriceCardClosureSameVersion：后继就是旧版本身。
	ErrPriceCardClosureSameVersion = errors.New("parcel pricing: the successor names the same price card version")
	// ErrPriceCardClosureVersionNotRegistered：本租户的册上没有所指名的那一版（旧版或后继）。
	ErrPriceCardClosureVersionNotRegistered = errors.New("parcel pricing: the named price card version is not on the register")
	// ErrPriceCardClosureAcrossScopes：后继问的不是同一计价范围、方向与目的。按范围选卡的口只在这三格都相同时才会把两版
	// 读成适用冲突；换了范围的后继接不住旧版那一段，收尾会让旧版那一格变成没有在用价卡。
	ErrPriceCardClosureAcrossScopes = errors.New("parcel pricing: the successor is priced on another scope, direction or purpose")
	// ErrPriceCardClosureRetired：旧版或后继已经退役，此刻一刻也选不中。已退役的旧版没有可收的；已退役的后继接不住任何一段。
	ErrPriceCardClosureRetired = errors.New("parcel pricing: the predecessor or the successor is already retired")
	// ErrPriceCardSuccessorCutOutsideSelection：后继起点不严格落在旧版此刻的选用区间之内。不晚于旧版起点——截下来旧版一刻
	// 不剩，走退役；不早于旧版终点——两版本来不重叠，或旧版已在那一刻截止过。
	ErrPriceCardSuccessorCutOutsideSelection = errors.New("parcel pricing: the successor's start is not strictly inside the predecessor's selection")
	// ErrPriceCardRetirementSuccessorStartsLater：后继晚于旧版此刻的选用起点。退役让旧版整版不再参选，旧版起点到后继起点
	// 那一段会从唯一可选变成没有在用价卡；这一格走截止，前一段照旧由旧版答。
	ErrPriceCardRetirementSuccessorStartsLater = errors.New("parcel pricing: the successor starts after the predecessor; cut instead of retiring")
	// ErrPriceCardClosureLeavesGap：后继此刻的选用区间到不了旧版此刻的选用终点。
	ErrPriceCardClosureLeavesGap = errors.New("parcel pricing: the successor does not reach the end of the predecessor's selection")
	// ErrPriceCardRetiredOtherwise：旧版已经退役，凭的是另一个后继或另一份依据。一版只退一次；再登一次异内容的退役就是改写
	// 「何时、凭什么不再可选」这一段史实。
	ErrPriceCardRetiredOtherwise = errors.New("parcel pricing: the predecessor is already retired in favour of another successor or by another reference")
)

// PriceCardClosureReference 是一次收尾凭的依据：登记方给的决定或工单引用。同一后继、同一依据的再一次收尾是重放。
type PriceCardClosureReference struct{ identifier }

func NewPriceCardClosureReference(value string) (PriceCardClosureReference, error) {
	identifier, err := newIdentifier("price card closure reference", value)
	return PriceCardClosureReference{identifier: identifier}, err
}

// PriceCardCut 是册上的一次截止：哪一版、让位给同一方案的哪一版、选用截到哪一刻、凭什么、何时登记。
type PriceCardCut struct {
	tenant       TenantID
	plan         VersionReference
	successor    VersionReference
	endsAt       time.Time
	reference    PriceCardClosureReference
	registeredAt time.Time
}

func (cut PriceCardCut) Tenant() TenantID                     { return cut.tenant }
func (cut PriceCardCut) Plan() VersionReference               { return cut.plan }
func (cut PriceCardCut) Successor() VersionReference          { return cut.successor }
func (cut PriceCardCut) SelectionEndsAt() time.Time           { return cut.endsAt }
func (cut PriceCardCut) Reference() PriceCardClosureReference { return cut.reference }
func (cut PriceCardCut) RegisteredAt() time.Time              { return cut.registeredAt }

// RehydratePriceCardCut 从册上一行重建一次截止。只核形状，不重判截止成立与否：那是登记那一刻在锁下判过的，此后后继或
// 旧版的状态变了也不追溯改写这条事实。
func RehydratePriceCardCut(
	tenant TenantID,
	plan, successor VersionReference,
	endsAt time.Time,
	reference PriceCardClosureReference,
	registeredAt time.Time,
) (PriceCardCut, error) {
	cut := PriceCardCut{
		tenant: tenant, plan: withoutFingerprint(plan), successor: withoutFingerprint(successor), endsAt: endsAt.UTC(),
		reference: reference, registeredAt: registeredAt.UTC(),
	}
	if !closureShapeHolds(tenant, plan, successor, reference) || endsAt.IsZero() || registeredAt.IsZero() {
		return PriceCardCut{}, ErrInvalidPriceCardClosure
	}
	return cut, nil
}

// PriceCardRetirement 是册上的一次退役：哪一版、让位给同一方案的哪一版、凭什么、何时起不再参与新的选择。
type PriceCardRetirement struct {
	tenant    TenantID
	plan      VersionReference
	successor VersionReference
	reference PriceCardClosureReference
	retiredAt time.Time
}

func (retirement PriceCardRetirement) Tenant() TenantID            { return retirement.tenant }
func (retirement PriceCardRetirement) Plan() VersionReference      { return retirement.plan }
func (retirement PriceCardRetirement) Successor() VersionReference { return retirement.successor }
func (retirement PriceCardRetirement) Reference() PriceCardClosureReference {
	return retirement.reference
}
func (retirement PriceCardRetirement) RetiredAt() time.Time { return retirement.retiredAt }

// RehydratePriceCardRetirement 从册上一行重建一次退役，判据同 RehydratePriceCardCut。
func RehydratePriceCardRetirement(
	tenant TenantID,
	plan, successor VersionReference,
	reference PriceCardClosureReference,
	retiredAt time.Time,
) (PriceCardRetirement, error) {
	retirement := PriceCardRetirement{
		tenant: tenant, plan: withoutFingerprint(plan), successor: withoutFingerprint(successor), reference: reference,
		retiredAt: retiredAt.UTC(),
	}
	if !closureShapeHolds(tenant, plan, successor, reference) || retiredAt.IsZero() {
		return PriceCardRetirement{}, ErrInvalidPriceCardClosure
	}
	return retirement, nil
}

func closureShapeHolds(tenant TenantID, plan, successor VersionReference, reference PriceCardClosureReference) bool {
	return tenant.valid() && reference.valid() &&
		plan.kind == ArtifactPricingPlan && plan.valid() && successor.kind == ArtifactPricingPlan && successor.valid() &&
		plan.id == successor.id && plan.version != successor.version
}

// SelectablePriceCardPeriod 是一版卡此刻还能被新的选择选中的那一段：发布时的适用期被册上最新一条截止的终点收窄，退役了就
// 一刻不剩（selects 为假）。按范围选卡的各口与收尾的判据都问这一处，两边看的才是同一个区间——否则截出来的两版在选卡
// 眼里仍可能重叠或留缝。latestCutEnd 为零即没有截止。
//
// 取「最新一条」而不是最早的终点：截止只增不改，选用按登记顺序取最后一条（ADR-0056 决定一的同一读法）；而每一次截止都
// 要求截止点落在当时的选用区间之内，于是最后一条也总是最短的那一条，两种读法今天答同一个区间。
func SelectablePriceCardPeriod(published EffectivePeriod, latestCutEnd time.Time, retired bool) (EffectivePeriod, bool) {
	if retired || !published.valid() {
		return EffectivePeriod{}, false
	}
	if latestCutEnd.IsZero() {
		return published, true
	}
	if !latestCutEnd.After(published.startsAt) {
		return EffectivePeriod{}, false
	}
	if !published.endsAt.IsZero() && !published.endsAt.After(latestCutEnd) {
		return published, true
	}
	return EffectivePeriod{startsAt: published.startsAt, endsAt: latestCutEnd.UTC()}, true
}

// PriceCardStanding 是一版卡此刻在册上的样子：发布的那一版，加上册上追加给它的截止（按登记顺序）与退役。收尾的判据只读它，
// 不读别的版本：「接得住」看的是指名的后继，不是同一方案的全部版本。
type PriceCardStanding struct {
	tenant     TenantID
	plan       PricingPlanVersion
	cuts       []PriceCardCut
	retirement PriceCardRetirement
	retired    bool
}

// NewPriceCardStanding 把一版卡与它的收尾事实拼起来。截止与退役必须都是这一版的；retirement 为 nil 即没退役。
func NewPriceCardStanding(
	tenant TenantID,
	plan PricingPlanVersion,
	cuts []PriceCardCut,
	retirement *PriceCardRetirement,
) (PriceCardStanding, error) {
	if !tenant.valid() || !plan.valid() {
		return PriceCardStanding{}, ErrInvalidPriceCardClosure
	}
	standing := PriceCardStanding{tenant: tenant, plan: plan, cuts: append([]PriceCardCut(nil), cuts...)}
	for _, cut := range standing.cuts {
		if cut.tenant != tenant || !cut.plan.SameIdentity(plan.reference) {
			return PriceCardStanding{}, fmt.Errorf("%w: cut of %s/%s on the standing of %s/%s", ErrInvalidPriceCardClosure,
				cut.plan.id, cut.plan.version, plan.reference.id, plan.reference.version)
		}
	}
	if retirement != nil {
		if retirement.tenant != tenant || !retirement.plan.SameIdentity(plan.reference) {
			return PriceCardStanding{}, fmt.Errorf("%w: retirement of %s/%s on the standing of %s/%s", ErrInvalidPriceCardClosure,
				retirement.plan.id, retirement.plan.version, plan.reference.id, plan.reference.version)
		}
		standing.retirement, standing.retired = *retirement, true
	}
	return standing, nil
}

func (standing PriceCardStanding) Tenant() TenantID         { return standing.tenant }
func (standing PriceCardStanding) Plan() PricingPlanVersion { return standing.plan }

// Selection 是这一版此刻还能被选中的那一段（SelectablePriceCardPeriod）。
func (standing PriceCardStanding) Selection() (EffectivePeriod, bool) {
	var latest time.Time
	if count := len(standing.cuts); count > 0 {
		latest = standing.cuts[count-1].endsAt
	}
	return SelectablePriceCardPeriod(standing.plan.period, latest, standing.retired)
}

// Retirement 交回这一版的退役（若有）。
func (standing PriceCardStanding) Retirement() (PriceCardRetirement, bool) {
	return standing.retirement, standing.retired
}

// LatestCut 交回这一版册上最新的那一条截止（若有）。
func (standing PriceCardStanding) LatestCut() (PriceCardCut, bool) {
	if count := len(standing.cuts); count > 0 {
		return standing.cuts[count-1], true
	}
	return PriceCardCut{}, false
}

// CutForSuccessor 把这一版截止在 successor 此刻的选用起点。replay 为真时交回的是册上已有的那一条：同一后继、同一依据的
// 再一次截止不追加。重放判在区间检查之前，因为第一次截止之后旧版的终点已经就是截止点，再按「严格落在区间之内」判会把
// 重放读成越界；也判在退役检查之前——截止之后又退役的那一版，重放那次截止仍是同一件已登记的事。
//
// at 是登记时刻，由调用方的时钟给，不进重放的比对：截止管的是此后的新选择从哪一刻起换成后继，登记时刻只是审计那一格。
func (standing PriceCardStanding) CutForSuccessor(
	successor PriceCardStanding,
	reference PriceCardClosureReference,
	at time.Time,
) (cut PriceCardCut, replay bool, err error) {
	if err := standing.closurePair(successor, reference, at); err != nil {
		return PriceCardCut{}, false, err
	}
	for _, prior := range standing.cuts {
		if prior.successor.SameIdentity(successor.plan.reference) && prior.reference == reference {
			return prior, true, nil
		}
	}
	current, selects := standing.Selection()
	next, nextSelects := successor.Selection()
	if !selects || !nextSelects {
		return PriceCardCut{}, false, ErrPriceCardClosureRetired
	}
	point := next.startsAt
	if !point.After(current.startsAt) || (!current.endsAt.IsZero() && !point.Before(current.endsAt)) {
		return PriceCardCut{}, false, fmt.Errorf("%w: successor %s starts at %s, predecessor %s selects %s",
			ErrPriceCardSuccessorCutOutsideSelection, successor.plan.reference.version, point.Format(time.RFC3339),
			standing.plan.reference.version, describePeriod(current))
	}
	if !reachesEnd(next, current) {
		return PriceCardCut{}, false, fmt.Errorf("%w: successor %s selects %s, predecessor %s %s", ErrPriceCardClosureLeavesGap,
			successor.plan.reference.version, describePeriod(next), standing.plan.reference.version, describePeriod(current))
	}
	return PriceCardCut{
		tenant: standing.tenant, plan: withoutFingerprint(standing.plan.reference), successor: withoutFingerprint(successor.plan.reference),
		endsAt: point, reference: reference, registeredAt: at.UTC(),
	}, false, nil
}

// RetireForCoveringSuccessor 让这一版让位给起点不晚于它的 successor：自退役登记起，这一版不再参与任何新的选择。replay 为真
// 时交回的是册上那一次退役：同一后继、同一依据重放不重登，交回原来的时刻。
//
// at 是登记时刻，由调用方的时钟给：退役只管此后的新选择，倒填一个更早的时刻改变不了已经作出的选择，填一个更晚的时刻
// 又会让这一版在那之前就已落选——只增的册上没有到点翻状态的口，两者都不是真话。
func (standing PriceCardStanding) RetireForCoveringSuccessor(
	successor PriceCardStanding,
	reference PriceCardClosureReference,
	at time.Time,
) (retirement PriceCardRetirement, replay bool, err error) {
	if err := standing.closurePair(successor, reference, at); err != nil {
		return PriceCardRetirement{}, false, err
	}
	if standing.retired {
		if standing.retirement.successor.SameIdentity(successor.plan.reference) && standing.retirement.reference == reference {
			return standing.retirement, true, nil
		}
		return PriceCardRetirement{}, false, ErrPriceCardRetiredOtherwise
	}
	current, _ := standing.Selection()
	next, nextSelects := successor.Selection()
	if !nextSelects {
		return PriceCardRetirement{}, false, ErrPriceCardClosureRetired
	}
	if next.startsAt.After(current.startsAt) {
		return PriceCardRetirement{}, false, fmt.Errorf("%w: successor %s starts at %s, predecessor %s at %s",
			ErrPriceCardRetirementSuccessorStartsLater, successor.plan.reference.version, next.startsAt.Format(time.RFC3339),
			standing.plan.reference.version, current.startsAt.Format(time.RFC3339))
	}
	if !reachesEnd(next, current) {
		return PriceCardRetirement{}, false, fmt.Errorf("%w: successor %s selects %s, predecessor %s %s", ErrPriceCardClosureLeavesGap,
			successor.plan.reference.version, describePeriod(next), standing.plan.reference.version, describePeriod(current))
	}
	return PriceCardRetirement{
		tenant: standing.tenant, plan: withoutFingerprint(standing.plan.reference), successor: withoutFingerprint(successor.plan.reference),
		reference: reference, retiredAt: at.UTC(),
	}, false, nil
}

// closurePair 是两种收尾共用的门：同一租户、同一定价方案的两个不同版本，问同一计价范围、方向与目的。生命周期位置（退役与否）
// 另判，因为重放要在它之前认。
func (standing PriceCardStanding) closurePair(successor PriceCardStanding, reference PriceCardClosureReference, at time.Time) error {
	if !standing.tenant.valid() || !standing.plan.valid() || !successor.plan.valid() || !reference.valid() || at.IsZero() ||
		standing.tenant != successor.tenant || standing.plan.reference.id != successor.plan.reference.id {
		return ErrInvalidPriceCardClosure
	}
	if standing.plan.reference.version == successor.plan.reference.version {
		return ErrPriceCardClosureSameVersion
	}
	if standing.plan.scope != successor.plan.scope || standing.plan.direction != successor.plan.direction ||
		standing.plan.purpose != successor.plan.purpose {
		return fmt.Errorf("%w: predecessor %s/%s/%s, successor %s/%s/%s", ErrPriceCardClosureAcrossScopes,
			standing.plan.scope, standing.plan.direction, standing.plan.purpose,
			successor.plan.scope, successor.plan.direction, successor.plan.purpose)
	}
	return nil
}

// withoutFingerprint 只留引用的三元：收尾事实落库时不带指纹，读回再比时也就不会因指纹差一格而认不出同一版。
func withoutFingerprint(reference VersionReference) VersionReference {
	return VersionReference{kind: reference.kind, id: reference.id, version: reference.version}
}

// reachesEnd 答 outer 是否一直延续到 inner 的终点（含无上界）。
func reachesEnd(outer, inner EffectivePeriod) bool {
	if outer.endsAt.IsZero() {
		return true
	}
	return !inner.endsAt.IsZero() && !outer.endsAt.Before(inner.endsAt)
}

func describePeriod(period EffectivePeriod) string {
	if period.endsAt.IsZero() {
		return "from " + period.startsAt.Format(time.RFC3339) + " (open-ended)"
	}
	return period.startsAt.Format(time.RFC3339) + " until " + period.endsAt.Format(time.RFC3339)
}
