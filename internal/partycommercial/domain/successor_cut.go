package domain

import (
	"errors"
	"fmt"
	"time"
)

// 按后继截止：把一版商业版本的选用区间截止在同一对象另一版的生效时点。它落下的仍是一条区间更正（ADR-0038、
// ADR-0056），不是新的事实种类，也不是替代或退役——版本壳的生命周期位置不动，旧版照旧是`已生效`，只是选用区间
// 到后继生效那一刻为止。这里只规定受控入口能写出哪一种更正：同一对象、后继已生效、截止点严格落在旧版此刻的选用
// 区间之内。任意改区间（含往后延）不经这里。

var (
	// ErrSuccessorCutKindReadByVersion：这一类对象的消费方按指名的版本点读，并在点读里按这一版自己的区间判它此刻
	// 能不能用，却不折区间更正（SupplierAgreementVersionView 的注释）。截止它只会让闭包与点读对同一版给出两种答案：
	// 闭包不再选它，点读照旧收它。
	ErrSuccessorCutKindReadByVersion = errors.New("party commercial: this kind is judged by its named-version reads, which do not fold validity corrections")
	// ErrSuccessorCutSameVersion：旧版与后继是同一版。
	ErrSuccessorCutSameVersion = errors.New("party commercial: the successor names the same version")
	// ErrSuccessorCutVersionNotRegistered：这一范围的册上没有所指名的那一版（旧版或后继）。另一对象、另一类或另一
	// 范围下同名的版本都不算——入口只认同一对象，跨对象的收尾不是截止。
	ErrSuccessorCutVersionNotRegistered = errors.New("party commercial: the named version is not registered in this scope")
	// ErrSuccessorCutNotEffective：旧版或后继不在`已生效`。已计划生效的后继在只增仓储里不会自己翻到已生效（发布用例
	// 到界才取效），截止旧版会让后继生效时点之后一版都选不中；已计划生效的旧版本来就不参选，没有可截的。
	ErrSuccessorCutNotEffective = errors.New("party commercial: the predecessor and the successor must both be effective")
	// ErrSuccessorCutOutsideInterval：后继的生效时点不严格落在旧版此刻的选用区间之内。不晚于旧版起点——截下来旧版
	// 一刻都不剩，那是替代而不是截止；不早于旧版终点——两版本来不重叠，或旧版已在那一刻截止过。
	ErrSuccessorCutOutsideInterval = errors.New("party commercial: the successor's start is not strictly inside the predecessor's selection interval")
	// ErrSuccessorCutBodyStartsElsewhere：后继自带区间的正文（价格、结算、信用政策）不从截止点开始。这几类按正文
	// 区间选候选，截在壳的生效时点上：正文起点更早，两版在那一段仍重叠成`适用冲突`；更晚，那一段原本唯一选中的
	// 旧版被截掉、后继还没开始，答`无适用依据`。发布不要求正文与壳同区间，所以这一格要在入口判。
	ErrSuccessorCutBodyStartsElsewhere = errors.New("party commercial: the successor's policy body does not start at the cut")
)

// readByVersion 是受控截止入口不收的那几类，判据见 ErrSuccessorCutKindReadByVersion。点读却根本不按区间判的（只看
// 生命周期位置）不在其中：区间更正在那里与原区间一样不起作用，两种读法不会因截止而分岔。
func (kind CommercialObjectKind) readByVersion() bool {
	return kind == SupplierAgreementObject
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
	if registry == nil || !tenant.valid() || !scope.valid() || !kind.valid() || !objectID.valid() ||
		!predecessor.valid() || !successor.valid() || !reference.valid() || at.IsZero() {
		return ValidityCorrection{}, false, ErrValidityCorrectionInvalid
	}
	if kind.readByVersion() {
		return ValidityCorrection{}, false, ErrSuccessorCutKindReadByVersion
	}
	if predecessor == successor {
		return ValidityCorrection{}, false, ErrSuccessorCutSameVersion
	}
	old, found := registry.lookupInScope(tenant, scope, kind, objectID, predecessor)
	if !found {
		return ValidityCorrection{}, false, fmt.Errorf("%w: predecessor %s/%s", ErrSuccessorCutVersionNotRegistered, objectID, predecessor)
	}
	next, found := registry.lookupInScope(tenant, scope, kind, objectID, successor)
	if !found {
		return ValidityCorrection{}, false, fmt.Errorf("%w: successor %s/%s", ErrSuccessorCutVersionNotRegistered, objectID, successor)
	}
	if old.status != CommercialVersionEffective || next.status != CommercialVersionEffective {
		return ValidityCorrection{}, false, ErrSuccessorCutNotEffective
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
	end, bounded := current.EndsAt()
	if !cut.After(current.StartsAt()) || (bounded && !cut.Before(end)) {
		return ValidityCorrection{}, false, fmt.Errorf("%w: successor %s starts at %s, predecessor %s selects from %s",
			ErrSuccessorCutOutsideInterval, successor, cut.Format(time.RFC3339), predecessor, describeInterval(current))
	}
	if body, carried := registry.policyBodyIntervalOf(next); carried && !body.StartsAt().Equal(cut) {
		return ValidityCorrection{}, false, fmt.Errorf("%w: successor %s body starts at %s, its version at %s",
			ErrSuccessorCutBodyStartsElsewhere, successor, body.StartsAt().Format(time.RFC3339), cut.Format(time.RFC3339))
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

// policyBodyIntervalOf 取一版登在册上的、自带区间的政策正文的区间：这几类的选候选看的是它（correctionExcludes 只在
// 其上收窄）。别的类别、或这一版没登正文，carried 为假。
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
