package domain

import (
	"errors"
	"fmt"
	"time"
)

// 按后继收尾旧版（ADR-0233）：同一对象的两版都已生效、在同一范围里并存时，登记方显式指名后继，把旧版收尾。收尾有两种，
// 由两版此刻的选用起点决定能用哪一种，登记方各走各的口：
//
//   - 截止：后继晚于旧版起点。落一条区间更正（ADR-0038、ADR-0056），旧版的选用区间到后继生效那一刻为止，版本壳的
//     生命周期位置不动。
//   - 同起点退役：后继与旧版同一起点（同一份依据重发）。截在起点上旧版一刻不剩，区间表达不了；旧版按决定引用
//     `已生效 → 已退役`，此后不再参与新的选择。
//
// 两种都要求后继接得住旧版此后的那一段，否则收尾会把原本唯一选中旧版的时段变成一版都选不中。新版发布本身不收尾旧版。

var (
	// ErrClosureSameVersion：旧版与后继是同一版。
	ErrClosureSameVersion = errors.New("party commercial: the successor names the same version")
	// ErrClosureVersionNotRegistered：这一范围的册上没有所指名的那一版（旧版或后继）。另一对象、另一类或另一范围下
	// 同名的版本都不算——收尾只认同一对象。
	ErrClosureVersionNotRegistered = errors.New("party commercial: the named version is not registered in this scope")
	// ErrClosureNotEffective：旧版或后继不在`已生效`。已计划生效的后继在只增仓储里不会自己翻到已生效（发布用例到界才
	// 取效），收尾旧版会让后继生效时点之后一版都选不中；不在已生效的旧版本来就不参选，没有可收的。
	ErrClosureNotEffective = errors.New("party commercial: the predecessor and the successor must both be effective")
	// ErrClosureLeavesGap：后继接不住旧版此后的那一段——选用区间（自带区间的政策正文同样）到不了旧版此刻的选用终点，
	// 或同起点退役时后继正文起点晚于旧版的。收尾之后那一段原本由旧版唯一选中，会变成`无适用依据`。
	ErrClosureLeavesGap = errors.New("party commercial: the successor does not reach the end of the predecessor's selection")
	// ErrSuccessorCutOutsideInterval：后继的生效时点不严格落在旧版此刻的选用区间之内。不晚于旧版起点——截下来旧版
	// 一刻都不剩，同起点的那一格走同起点退役；不早于旧版终点——两版本来不重叠，或旧版已在那一刻截止过。
	ErrSuccessorCutOutsideInterval = errors.New("party commercial: the successor's start is not strictly inside the predecessor's selection interval")
	// ErrSuccessorCutBodyStartsElsewhere：后继自带区间的正文（价格、结算、信用政策）不从截止点开始。这几类按正文
	// 区间选候选，截在壳的生效时点上：正文起点更早，两版在那一段仍重叠成`适用冲突`；更晚，那一段原本唯一选中的
	// 旧版被截掉、后继还没开始。发布不要求正文与壳同区间，所以这一格要在收尾时判。
	ErrSuccessorCutBodyStartsElsewhere = errors.New("party commercial: the successor's policy body does not start at the cut")
	// ErrSameStartRetirementStartsDiffer：两版此刻的选用起点不同。后继晚于旧版起点的走截止，旧版前一段照旧可选；
	// 后继早于旧版起点的不是重发。
	ErrSameStartRetirementStartsDiffer = errors.New("party commercial: the predecessor and the successor do not start at the same instant")
	// ErrVersionRetiredOtherwise：旧版已经退役，凭的是另一个后继或另一份决定引用。退役是生命周期上的一次推进，
	// 一版只退一次；再登一次异内容的退役就是改写「何时、凭什么不再可选」这一段史实。
	ErrVersionRetiredOtherwise = errors.New("party commercial: the predecessor is already retired in favour of another successor or by another decision")
)

// VersionRetirement 是登记册上的一次同起点退役：哪一版、让位给哪个后继、凭哪份决定、何时起不再参与新的选择。
// 版本快照不被改写（ADR-0233）；装载时把它施加到版本上，得到`已退役`的那一版。
type VersionRetirement struct {
	tenant    TenantID
	kind      CommercialObjectKind
	objectID  CommercialObjectID
	version   CommercialVersionLabel
	successor CommercialVersionLabel
	reference RetirementReference
	at        time.Time
}

func (retirement VersionRetirement) Tenant() TenantID                  { return retirement.tenant }
func (retirement VersionRetirement) Kind() CommercialObjectKind        { return retirement.kind }
func (retirement VersionRetirement) ObjectID() CommercialObjectID      { return retirement.objectID }
func (retirement VersionRetirement) Version() CommercialVersionLabel   { return retirement.version }
func (retirement VersionRetirement) Successor() CommercialVersionLabel { return retirement.successor }
func (retirement VersionRetirement) Reference() RetirementReference    { return retirement.reference }
func (retirement VersionRetirement) RetiredAt() time.Time              { return retirement.at }

// sameRetirementDecision 判两次退役是不是同一个决定：同一版、同一后继、同一决定引用。时刻不在内——退役时刻取登记
// 时刻，重放晚到时刻必然不同，而它说的是同一件事。
func sameRetirementDecision(left, right VersionRetirement) bool {
	return left.tenant == right.tenant && left.kind == right.kind && left.objectID == right.objectID &&
		left.version == right.version && left.successor == right.successor && left.reference == right.reference
}

// RetireInFavourOf 构造这一版让位给 successor 的退役。生命周期上的门（只有`已生效`可退役、时刻不早于取效时刻、要有
// 决定引用）问 Retire 本身，不在这里抄第二份。
func (version CommercialVersion) RetireInFavourOf(
	successor CommercialVersionLabel,
	reference RetirementReference,
	at time.Time,
) (VersionRetirement, error) {
	if !successor.valid() || successor == version.version {
		return VersionRetirement{}, ErrInvalidCommercialTransition
	}
	if _, err := version.Retire(reference, at); err != nil {
		return VersionRetirement{}, err
	}
	return VersionRetirement{
		tenant: version.tenant, kind: version.kind, objectID: version.objectID, version: version.version,
		successor: successor, reference: reference, at: at.UTC(),
	}, nil
}

// RegisterRetirement 把一次退役施加到册上那一版：它从此是`已退役`，不再参与新的选择；正文、批准与原区间不动。同一个
// 决定重放不推进视图；异内容答 ErrVersionRetiredOtherwise。
func (registry *CommercialRegistry) RegisterRetirement(retirement VersionRetirement) (AuthorityViewRevision, error) {
	if registry == nil {
		return AuthorityViewRevision{}, ErrInvalidCommercialTransition
	}
	key := commercialVersionKey{tenant: retirement.tenant, kind: retirement.kind, objectID: retirement.objectID, version: retirement.version}
	existing, found := registry.versions[key]
	if !found {
		return AuthorityViewRevision{}, ErrCommercialVersionNotPublished
	}
	if prior, retired := registry.retirements[key]; retired {
		if !sameRetirementDecision(prior, retirement) {
			return AuthorityViewRevision{}, ErrVersionRetiredOtherwise
		}
		return registry.ViewRevision(existing.tenant, existing.scope), nil
	}
	closed, err := existing.Retire(retirement.reference, retirement.at)
	if err != nil {
		return AuthorityViewRevision{}, err
	}
	registry.versions[key] = closed
	registry.retirements[key] = retirement
	return registry.ViewRevision(existing.tenant, existing.scope), nil
}

// RetirementOf 取回一版在册上的退役（若有）。
func (registry *CommercialRegistry) RetirementOf(
	tenant TenantID,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	version CommercialVersionLabel,
) (VersionRetirement, bool) {
	if registry == nil {
		return VersionRetirement{}, false
	}
	retirement, found := registry.retirements[commercialVersionKey{tenant: tenant, kind: kind, objectID: objectID, version: version}]
	return retirement, found
}

// CutAtSuccessorStart 构造一条把 predecessor 截止在 successor 生效时点的区间更正：起点取旧版此刻的选用起点，终点取
// 后继此刻的选用起点。两个「此刻的选用」都问登记册（selectionInterval），与解析选候选看的是同一个区间——否则截出来的
// 两版在解析眼里仍可能重叠或留缝。
//
// replay 为真时交回的是册上已有的那一条：同一次截止重放不追加（ADR-0056 决定一）。重放判在区间检查之前，因为第一次
// 截止之后旧版的终点已经就是截止点，再按「严格落在区间之内」判会把重放读成越界。
//
// 引用与时刻由调用方给，不由时钟补：同内容重放靠它们逐字相同来认（ADR-0056 的唯一键含更正时刻）。
func (registry *CommercialRegistry) CutAtSuccessorStart(
	tenant TenantID,
	scope CommercialScopeReference,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	predecessor CommercialVersionLabel,
	successor CommercialVersionLabel,
	reference ValidityCorrectionReference,
	at time.Time,
) (correction ValidityCorrection, replay bool, err error) {
	if registry == nil || !reference.valid() || at.IsZero() {
		return ValidityCorrection{}, false, ErrValidityCorrectionInvalid
	}
	old, next, err := registry.closurePair(tenant, scope, kind, objectID, predecessor, successor)
	if err != nil {
		return ValidityCorrection{}, false, err
	}

	current := registry.selectionInterval(old)
	cut := registry.selectionInterval(next).StartsAt()
	if interval, intervalErr := NewEffectiveInterval(current.StartsAt(), cut); intervalErr == nil {
		candidate := ValidityCorrection{
			tenant: old.tenant, kind: old.kind, objectID: old.objectID, version: old.version,
			corrected: interval, reference: reference, at: at.UTC(),
		}
		for _, prior := range registry.corrections[keyOf(old)] {
			if sameValidityCorrection(prior, candidate) {
				return prior, true, nil
			}
		}
	}
	if err := requireEffective(old, next); err != nil {
		return ValidityCorrection{}, false, err
	}
	end, bounded := current.EndsAt()
	if !cut.After(current.StartsAt()) || (bounded && !cut.Before(end)) {
		return ValidityCorrection{}, false, fmt.Errorf("%w: successor %s starts at %s, predecessor %s selects from %s",
			ErrSuccessorCutOutsideInterval, successor, cut.Format(time.RFC3339), predecessor, describeInterval(current))
	}
	if body, carried, selects := registry.bodySelectionOf(next); carried && selects && !body.StartsAt().Equal(cut) {
		return ValidityCorrection{}, false, fmt.Errorf("%w: successor %s body starts at %s, its version at %s",
			ErrSuccessorCutBodyStartsElsewhere, successor, body.StartsAt().Format(time.RFC3339), cut.Format(time.RFC3339))
	}
	if err := registry.requireTakeOver(old, next, false); err != nil {
		return ValidityCorrection{}, false, err
	}
	interval, err := NewEffectiveInterval(current.StartsAt(), cut)
	if err != nil {
		return ValidityCorrection{}, false, err
	}
	correction, err = old.CorrectEffectiveInterval(interval, reference, at)
	if err != nil {
		return ValidityCorrection{}, false, err
	}
	return correction, false, nil
}

// RetireAtSameStartSuccessor 构造 predecessor 让位给同起点后继的退役。at 是登记时刻，由调用方的时钟给：退役只管此后的
// 新选择（CONTEXT「自退役生效时间起不再供新的商业选择」），倒填一个更早的时刻改变不了已经作出的决定，填一个更晚的
// 时刻又会让它在那之前就已落选——两者都不是真话。
//
// replay 为真时交回的是册上那一次退役：同一个决定（同一后继、同一决定引用）重放不重登，交回原来的时刻。
func (registry *CommercialRegistry) RetireAtSameStartSuccessor(
	tenant TenantID,
	scope CommercialScopeReference,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	predecessor CommercialVersionLabel,
	successor CommercialVersionLabel,
	reference RetirementReference,
	at time.Time,
) (retirement VersionRetirement, replay bool, err error) {
	if registry == nil || !reference.valid() || at.IsZero() {
		return VersionRetirement{}, false, ErrInvalidCommercialTransition
	}
	old, next, err := registry.closurePair(tenant, scope, kind, objectID, predecessor, successor)
	if err != nil {
		return VersionRetirement{}, false, err
	}
	if prior, retired := registry.retirements[keyOf(old)]; retired {
		requested := VersionRetirement{
			tenant: old.tenant, kind: old.kind, objectID: old.objectID, version: old.version,
			successor: successor, reference: reference,
		}
		if !sameRetirementDecision(prior, requested) {
			return VersionRetirement{}, false, ErrVersionRetiredOtherwise
		}
		return prior, true, nil
	}
	if err := requireEffective(old, next); err != nil {
		return VersionRetirement{}, false, err
	}
	oldStart := registry.selectionInterval(old).StartsAt()
	nextStart := registry.selectionInterval(next).StartsAt()
	if !nextStart.Equal(oldStart) {
		return VersionRetirement{}, false, fmt.Errorf("%w: successor %s starts at %s, predecessor %s at %s",
			ErrSameStartRetirementStartsDiffer, successor, nextStart.Format(time.RFC3339), predecessor, oldStart.Format(time.RFC3339))
	}
	if err := registry.requireTakeOver(old, next, true); err != nil {
		return VersionRetirement{}, false, err
	}
	retirement, err = old.RetireInFavourOf(successor, reference, at)
	if err != nil {
		return VersionRetirement{}, false, err
	}
	return retirement, false, nil
}

// closurePair 取回两种收尾共用的那一对：同一对象、同一范围、不是同一版，都在册。生命周期位置另判（requireEffective），
// 因为重放要在它之前认——第一次退役之后旧版就不在已生效了。
func (registry *CommercialRegistry) closurePair(
	tenant TenantID,
	scope CommercialScopeReference,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	predecessor CommercialVersionLabel,
	successor CommercialVersionLabel,
) (old, next CommercialVersion, err error) {
	if !tenant.valid() || !scope.valid() || !kind.valid() || !objectID.valid() || !predecessor.valid() || !successor.valid() {
		return CommercialVersion{}, CommercialVersion{}, ErrInvalidCommercialVersion
	}
	if predecessor == successor {
		return CommercialVersion{}, CommercialVersion{}, ErrClosureSameVersion
	}
	old, found := registry.lookupInScope(tenant, scope, kind, objectID, predecessor)
	if !found {
		return CommercialVersion{}, CommercialVersion{}, fmt.Errorf("%w: predecessor %s/%s", ErrClosureVersionNotRegistered, objectID, predecessor)
	}
	next, found = registry.lookupInScope(tenant, scope, kind, objectID, successor)
	if !found {
		return CommercialVersion{}, CommercialVersion{}, fmt.Errorf("%w: successor %s/%s", ErrClosureVersionNotRegistered, objectID, successor)
	}
	return old, next, nil
}

func requireEffective(old, next CommercialVersion) error {
	if old.status != CommercialVersionEffective || next.status != CommercialVersionEffective {
		return ErrClosureNotEffective
	}
	return nil
}

// requireTakeOver 判后继接不接得住旧版此后的那一段：壳的选用终点，以及自带区间的政策正文——两边都按解析此刻实际看到的
// 那一段比（bodySelectionOf），不按发布时的原区间：旧版若先被截止过，原区间比它此刻还能被选中的那一段长，按原区间比会
// 把一个正好接得住的后继误拒。旧版登了正文而后继没登、或后继的正文此刻一刻也选不中，后继在这几类里根本接不住。
//
// fromStart 为真（同起点退役）时另要求后继的正文起点不晚于旧版的：退役让旧版整版不再参选，壳同起点而后继正文起点更晚，
// 旧版正文起点到后继正文起点那一段就会从唯一可选变成无依据。截止不要这一格：截止点之前旧版照旧可选。
func (registry *CommercialRegistry) requireTakeOver(old, next CommercialVersion, fromStart bool) error {
	oldSelection, nextSelection := registry.selectionInterval(old), registry.selectionInterval(next)
	if !reachesEndOf(nextSelection, oldSelection) {
		return fmt.Errorf("%w: successor %s selects %s, predecessor %s %s",
			ErrClosureLeavesGap, next.version, describeInterval(nextSelection), old.version, describeInterval(oldSelection))
	}
	oldBody, oldCarried, oldSelects := registry.bodySelectionOf(old)
	if !oldCarried || !oldSelects {
		return nil
	}
	nextBody, nextCarried, nextSelects := registry.bodySelectionOf(next)
	switch {
	case !nextCarried || !nextSelects:
		return fmt.Errorf("%w: the successor %s carries no selectable policy body", ErrClosureLeavesGap, next.version)
	case fromStart && nextBody.StartsAt().After(oldBody.StartsAt()):
		return fmt.Errorf("%w: the successor's policy body starts at %s, the predecessor's at %s",
			ErrClosureLeavesGap, nextBody.StartsAt().Format(time.RFC3339), oldBody.StartsAt().Format(time.RFC3339))
	case !reachesEndOf(nextBody, oldBody):
		return fmt.Errorf("%w: the successor's policy body does not reach the end of the predecessor's", ErrClosureLeavesGap)
	}
	return nil
}

// bodySelectionOf 取一版自带区间的政策正文此刻在解析里还能被选中的那一段：正文区间被壳上最新一条区间更正的终点收窄，
// 判法与 bodyExcludedAt 同一条（只截终点）。carried 为假是这一版没登正文或不是这几类；selects 为假是收窄之后一刻也不剩。
func (registry *CommercialRegistry) bodySelectionOf(version CommercialVersion) (interval EffectiveInterval, carried, selects bool) {
	body, carried := registry.policyBodyIntervalOf(version)
	if !carried {
		return EffectiveInterval{}, false, false
	}
	end, cut := registry.correctionEndOf(keyOf(version))
	if !cut {
		return body, true, true
	}
	if !end.After(body.StartsAt()) {
		return EffectiveInterval{}, true, false
	}
	if bodyEnd, bounded := body.EndsAt(); bounded && !bodyEnd.After(end) {
		return body, true, true
	}
	narrowed, err := NewEffectiveInterval(body.StartsAt(), end)
	if err != nil {
		return EffectiveInterval{}, true, false
	}
	return narrowed, true, true
}

// reachesEndOf 答 outer 是否一直延续到 inner 的终点（含开放结束）。
func reachesEndOf(outer, inner EffectiveInterval) bool {
	outerEnd, outerBounded := outer.EndsAt()
	if !outerBounded {
		return true
	}
	innerEnd, innerBounded := inner.EndsAt()
	return innerBounded && !outerEnd.Before(innerEnd)
}

// lookupInScope 按版本键取回一版，且只认登在这一范围的那一版：登记册可以装着多个范围，入口指名的是这一范围的册。
func (registry *CommercialRegistry) lookupInScope(
	tenant TenantID,
	scope CommercialScopeReference,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	label CommercialVersionLabel,
) (CommercialVersion, bool) {
	version, found := registry.Lookup(tenant, kind, objectID, label)
	if !found || version.scope != scope {
		return CommercialVersion{}, false
	}
	return version, true
}

// policyBodyIntervalOf 取一版登在册上的、自带区间的政策正文发布时的原区间；此刻还能被选中的那一段问 bodySelectionOf。
// 别的类别、或这一版没登正文，carried 为假。
func (registry *CommercialRegistry) policyBodyIntervalOf(version CommercialVersion) (EffectiveInterval, bool) {
	for _, policy := range registry.policies {
		if policy.version.SameVersionAs(version) {
			return policy.effective, true
		}
	}
	for _, policy := range registry.settlementPolicies {
		if policy.version.SameVersionAs(version) {
			return policy.applicability.effective, true
		}
	}
	for _, policy := range registry.creditPolicies {
		if policy.version.SameVersionAs(version) {
			return policy.effective, true
		}
	}
	return EffectiveInterval{}, false
}

func keyOf(version CommercialVersion) commercialVersionKey {
	return commercialVersionKey{tenant: version.tenant, kind: version.kind, objectID: version.objectID, version: version.version}
}

func describeInterval(interval EffectiveInterval) string {
	end, bounded := interval.EndsAt()
	if !bounded {
		return interval.StartsAt().Format(time.RFC3339) + " (open-ended)"
	}
	return interval.StartsAt().Format(time.RFC3339) + " until " + end.Format(time.RFC3339)
}
