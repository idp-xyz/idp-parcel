// Package pilotgovernance 是 parcel-shipment 对试点治理登记册的消费侧适配器
// （ADR-0025：只有本包可以同时导入两个上下文；只翻译不判断，翻译必须是全函数）。
//
// 它把治理侧已经登记的三样事实——生产权威区间、暂停决定、对象级接管记录——译成本
// 上下文的 `ProductionOwnershipDecision`。治理语义一概不在这里形成：区间归谁、暂停
// 何时生效、接管算不算走完，全部按登记的记录读，本包只做坐标换算与落点翻译。
package pilotgovernance

import (
	"context"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// ownershipRuleVersion 是本适配器所执行的那条归属规则的版本，不是租户参数。
//
// 规则本身是「按治理登记册的生产权威区间与暂停记录回答归属」——它属机制半边，此刻就
// 已确定；真实客户、线路与权威方的取值才在登记册里，那些经 GovernanceScopeDirectory
// 进来。把它做成装配参数会让「用了哪条规则」变成可被调用方改写的东西。
const ownershipRuleVersion = "PG-AUTHORITY-INTERVAL/v1"

// AuthorityIntervalSource 交回登记在册的全部生产权威区间。
//
// 它按 pilot-governance 的 AuthorityIntervalStore 原形取，不另开按范围过滤的窄口：
// 重叠检测要看的就是整册（同维两条区间本身是记录错误），过滤在本包做才看得见「命中
// 了几条」这件事。
type AuthorityIntervalSource interface {
	ListCurrent(ctx context.Context) ([]pgdomain.AuthorityInterval, error)
}

// AdmissionSuspensionSource 回答某个治理范围在某个业务时点是否处在暂停新准入之中。
//
// 第二个返回值为 false 即「该时点没有生效中的暂停」。依赖调不通作为错误返回——把它
// 读成「没暂停」是一次默认放行，一次故障会因此看起来像准入开放。
type AdmissionSuspensionSource interface {
	FindEffectiveSuspension(
		ctx context.Context,
		scope pgdomain.ScopeVersionReference,
		at time.Time,
	) (pgdomain.SuspensionDecision, bool, error)
}

// OwnershipHandoffSource 按权威区间找回对象级接管记录，即原权威停止写入的那份证据。
//
// `其他权威`归属决定在领域上必须携带交接确认引用（没有确认引用就不能判定为 OTHER
// 权威，见 production_ownership.go 的 validAuthorityDetails），而治理侧登记这份确认的
// 地方只有接管记录。取不到确认就不能答`其他权威`——那正是本口存在的理由。
type OwnershipHandoffSource interface {
	FindByInterval(
		ctx context.Context,
		interval pgdomain.AuthorityInterval,
	) (pgdomain.TakeoverRecord, bool, error)
}

// GovernanceScope 是一次拟受理范围在治理登记册里的坐标。
//
// 四样分开而不合成一个串：权威区间按（对象范围 × 能力 × 事实类型）三维定位，暂停按
// pilotScopeVersion 定位，两套坐标在治理侧本就不是同一个词（区间登的是「谁在写这类
// 事实」，暂停登的是「哪一版试点范围停止纳新」）。合成一个会让其中一边定位到错的行。
type GovernanceScope struct {
	ObjectScope string
	Capability  string
	FactKind    string
	PilotScope  pgdomain.ScopeVersionReference
}

func (scope GovernanceScope) complete() bool {
	return scope.ObjectScope != "" &&
		scope.Capability != "" &&
		scope.FactKind != "" &&
		scope.PilotScope.String() != ""
}

// GovernanceScopeDirectory 把本上下文的拟受理范围换成治理登记册的坐标。这是范围缝的
// 实例半边：哪份拟受理范围对应哪个对象范围、哪种能力与事实类型、哪一版试点范围，是
// 租户随试点登记的东西，没有租户就没有目录。
//
// 第二个返回值为 false 即「显式未配置」——停下，不代拟一个坐标去问登记册。代拟的后果
// 不是查不到而是查到别人那一行：对象范围写错一格，答的就是另一个范围归谁。
type GovernanceScopeDirectory interface {
	FindGovernanceScope(
		ctx context.Context,
		scope psdomain.AdmissionScope,
	) (GovernanceScope, bool, error)
}

// ProductionOwnershipAdapterDeps 是装配本适配器的全部输入。
//
// 三个读口与目录都允许缺席，缺席一律是「显式未配置」的诚实表达而不是构造错误：首发
// 期本来就没有租户，能装起来但答不出归属，正是要停的地方。唯独 AnswerValidity 在构造
// 期拒绝——见 NewProductionOwnershipAdapter。
type ProductionOwnershipAdapterDeps struct {
	Intervals   AuthorityIntervalSource
	Suspensions AdmissionSuspensionSource
	Handoffs    OwnershipHandoffSource
	Directory   GovernanceScopeDirectory
	// SelfAuthority 是登记册里代表本产品的那个权威串。它是全册一个约定而不是逐范围
	// 一份，所以不进 GovernanceScope。留空即未配置：届时无从判断命中的区间是不是自己
	// 的，答`权威未确定`而不是认领它。
	SelfAuthority string
	Clock         psports.Clock
	// AnswerValidity 是一份归属答复可被信任多久。
	//
	// 它由装配方说出，本包不挑一个数：`ProductionOwnershipDecision` 要求有效期间必须
	// 存在且包含判断时点，而治理登记册的开放区间（to_at 为 NULL）根本没有终点，随手
	// 补一个时长就是发明默认值。构造期拒绝非正值，因为这一格补不上就装不出诚实的答复。
	AnswerValidity time.Duration
}

// ProductionOwnershipAdapter 实现 psports.ProductionOwnershipAuthority。
type ProductionOwnershipAdapter struct {
	deps ProductionOwnershipAdapterDeps
}

// ErrAnswerValidityNotStated 说明装配方没有说出答复视界。它是装配错误不是业务答案：
// 没有它，本适配器答不出任何形状合法的归属决定。
var ErrAnswerValidityNotStated = fmt.Errorf(
	"parcel shipment pilotgovernance adapter: answer validity must be stated")

// ErrUntranslatableAnswer 与其余跨上下文适配器的同名哨兵同义：某一侧交出了词汇表之外
// 的内容，是编程错误不是业务答案。
var ErrUntranslatableAnswer = fmt.Errorf(
	"parcel shipment pilotgovernance adapter: untranslatable answer")

func NewProductionOwnershipAdapter(deps ProductionOwnershipAdapterDeps) (*ProductionOwnershipAdapter, error) {
	if deps.AnswerValidity <= 0 {
		return nil, ErrAnswerValidityNotStated
	}
	if deps.Clock == nil {
		return nil, fmt.Errorf("parcel shipment pilotgovernance adapter: clock is nil")
	}
	return &ProductionOwnershipAdapter{deps: deps}, nil
}

var _ psports.ProductionOwnershipAuthority = (*ProductionOwnershipAdapter)(nil)

// DecideProductionOwnership 按登记册回答这份拟受理范围当前由谁承接、以及本产品此刻是
// 否接纳新准入。
//
// 依赖调不通一律作为错误上抛，绝不折成某一格答复：把读不到登记册读成「没有登记」，
// 一次故障就会变成一次「权威未确定」，而两者的运维动作不同（ADR-0029）。真正的业务
// 答案只有三种权威身份加两种准入控制，全部经决定对象交回。
func (adapter *ProductionOwnershipAdapter) DecideProductionOwnership(
	ctx context.Context,
	scope psdomain.AdmissionScope,
) (psdomain.ProductionOwnershipDecision, error) {
	asOf := adapter.deps.Clock.Now().UTC()

	governance, configured, err := adapter.governanceScope(ctx, scope)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}
	if !configured {
		return adapter.unresolved(scope, asOf, psdomain.OwnershipUnresolvedRuleUnavailable,
			psdomain.AdmissionControlOpen, psdomain.OwnershipSuspensionReference{}, "")
	}

	matched, reason, err := adapter.matchInterval(ctx, governance, asOf)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}

	// 准入控制先于权威身份取。它是独立一维：`暂停`回答的是本产品此刻是否接纳新准入，
	// 与这个范围归谁无关（SubmitOutcome 的 OutcomeAdmissionPaused 注释是同一句话），
	// 因此权威已判未决时它照样要如实带出来。
	control, suspensionRef, err := adapter.admissionControl(ctx, governance, asOf)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}

	if reason != psdomain.OwnershipUnresolvedReasonInvalid {
		return adapter.unresolved(scope, asOf, reason, control, suspensionRef, "")
	}
	return adapter.resolved(ctx, scope, asOf, matched, control, suspensionRef)
}

func (adapter *ProductionOwnershipAdapter) governanceScope(
	ctx context.Context,
	scope psdomain.AdmissionScope,
) (GovernanceScope, bool, error) {
	if adapter.deps.Directory == nil ||
		adapter.deps.Intervals == nil ||
		adapter.deps.Suspensions == nil ||
		adapter.deps.SelfAuthority == "" {
		return GovernanceScope{}, false, nil
	}
	governance, found, err := adapter.deps.Directory.FindGovernanceScope(ctx, scope)
	if err != nil {
		return GovernanceScope{}, false, fmt.Errorf("find governance scope: %w", err)
	}
	if !found || !governance.complete() {
		return GovernanceScope{}, false, nil
	}
	return governance, true, nil
}

// matchInterval 找出在该时点覆盖这个治理坐标的权威区间。命中恰一条才有权威可答；零条
// 是「这个范围此刻没人被登记为写入方」，多于一条是登记本身就冲突——后者不按时间或行序
// 任选一条，那正是治理侧 DetectAuthorityConflicts 要拦的事。
func (adapter *ProductionOwnershipAdapter) matchInterval(
	ctx context.Context,
	governance GovernanceScope,
	asOf time.Time,
) (pgdomain.AuthorityInterval, psdomain.OwnershipUnresolvedReason, error) {
	registered, err := adapter.deps.Intervals.ListCurrent(ctx)
	if err != nil {
		return pgdomain.AuthorityInterval{}, 0, fmt.Errorf("list authority intervals: %w", err)
	}

	var matched []pgdomain.AuthorityInterval
	for _, interval := range registered {
		if interval.ObjectScope != governance.ObjectScope ||
			interval.Capability != governance.Capability ||
			interval.FactKind != governance.FactKind {
			continue
		}
		if asOf.Before(interval.From) {
			continue
		}
		if !interval.To.IsZero() && !asOf.Before(interval.To) {
			continue
		}
		matched = append(matched, interval)
	}

	switch len(matched) {
	case 0:
		return pgdomain.AuthorityInterval{}, psdomain.OwnershipUnresolvedRuleUnavailable, nil
	case 1:
		return matched[0], psdomain.OwnershipUnresolvedReasonInvalid, nil
	default:
		return pgdomain.AuthorityInterval{}, psdomain.OwnershipUnresolvedAuthorityNotUnique, nil
	}
}

func (adapter *ProductionOwnershipAdapter) admissionControl(
	ctx context.Context,
	governance GovernanceScope,
	asOf time.Time,
) (psdomain.AdmissionControl, psdomain.OwnershipSuspensionReference, error) {
	suspension, paused, err := adapter.deps.Suspensions.FindEffectiveSuspension(ctx, governance.PilotScope, asOf)
	if err != nil {
		return 0, psdomain.OwnershipSuspensionReference{}, fmt.Errorf("find effective suspension: %w", err)
	}
	if !paused {
		return psdomain.AdmissionControlOpen, psdomain.OwnershipSuspensionReference{}, nil
	}
	reference, err := psdomain.NewOwnershipSuspensionReference(suspension.ID().String())
	if err != nil {
		return 0, psdomain.OwnershipSuspensionReference{}, fmt.Errorf(
			"%w: suspension reference: %v", ErrUntranslatableAnswer, err)
	}
	return psdomain.AdmissionControlPaused, reference, nil
}

// resolved 把命中的那条区间译成权威身份。自己那一格直接成立；他方那一格必须先取到原
// 权威停止写入的证据，取不到就退回未决——`其他权威`是一个中性但确定的答案，用没有证据
// 的猜测填它，等于替对方宣布交接已经完成。
func (adapter *ProductionOwnershipAdapter) resolved(
	ctx context.Context,
	scope psdomain.AdmissionScope,
	asOf time.Time,
	matched pgdomain.AuthorityInterval,
	control psdomain.AdmissionControl,
	suspensionRef psdomain.OwnershipSuspensionReference,
) (psdomain.ProductionOwnershipDecision, error) {
	if matched.Authority == adapter.deps.SelfAuthority {
		return adapter.decision(scope, asOf, matched, control, suspensionRef, func(
			spec *psdomain.ProductionOwnershipDecisionSpec,
		) error {
			spec.Authority = psdomain.ProductionAuthorityIDPParcel
			return nil
		})
	}

	if adapter.deps.Handoffs == nil {
		return adapter.unresolved(scope, asOf, psdomain.OwnershipUnresolvedHandoffUnavailable,
			control, suspensionRef, intervalIdentity(matched))
	}
	takeover, found, err := adapter.deps.Handoffs.FindByInterval(ctx, matched)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf("find takeover record: %w", err)
	}
	if !found {
		return adapter.unresolved(scope, asOf, psdomain.OwnershipUnresolvedHandoffIncomplete,
			control, suspensionRef, intervalIdentity(matched))
	}

	return adapter.decision(scope, asOf, matched, control, suspensionRef, func(
		spec *psdomain.ProductionOwnershipDecisionSpec,
	) error {
		authorityRef, err := psdomain.NewProductionAuthorityReference(matched.Authority)
		if err != nil {
			return fmt.Errorf("%w: other authority reference: %v", ErrUntranslatableAnswer, err)
		}
		handoffRef, err := psdomain.NewHandoffConfirmationReference(takeover.StopEvidence())
		if err != nil {
			return fmt.Errorf("%w: handoff confirmation reference: %v", ErrUntranslatableAnswer, err)
		}
		spec.Authority = psdomain.ProductionAuthorityOther
		spec.OtherAuthorityRef = authorityRef
		spec.HandoffRef = handoffRef
		return nil
	})
}

// unresolved 形成`权威未确定`那一格。续办引用一定给得出：它由停摆原因与范围派生，说的
// 是「回去补哪一件」，因此不依赖任何尚未取到的登记内容。
func (adapter *ProductionOwnershipAdapter) unresolved(
	scope psdomain.AdmissionScope,
	asOf time.Time,
	reason psdomain.OwnershipUnresolvedReason,
	control psdomain.AdmissionControl,
	suspensionRef psdomain.OwnershipSuspensionReference,
	intervalRef string,
) (psdomain.ProductionOwnershipDecision, error) {
	continuation := "CONT-PG-OWN/" + reason.String() + "/" + scope.Reference().String()
	if intervalRef != "" {
		continuation += "/" + intervalRef
	}
	continuationRef, err := psdomain.NewOwnershipContinuationReference(continuation)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf(
			"%w: continuation reference: %v", ErrUntranslatableAnswer, err)
	}
	return adapter.decision(scope, asOf, pgdomain.AuthorityInterval{}, control, suspensionRef, func(
		spec *psdomain.ProductionOwnershipDecisionSpec,
	) error {
		spec.Authority = psdomain.ProductionAuthorityUnresolved
		spec.UnresolvedReason = reason
		spec.ContinuationRef = continuationRef
		return nil
	})
}

// decision 拼出决定共有的那几件：标识、有效期间、修订与规则版本。
//
// 标识与修订都由内容派生而不是签发——本适配器不持久化决定，重复问同一份登记册必须得到
// 同一个标识，否则调用方每问一次都拿到「新的一次决定」。
func (adapter *ProductionOwnershipAdapter) decision(
	scope psdomain.AdmissionScope,
	asOf time.Time,
	matched pgdomain.AuthorityInterval,
	control psdomain.AdmissionControl,
	suspensionRef psdomain.OwnershipSuspensionReference,
	fill func(*psdomain.ProductionOwnershipDecisionSpec) error,
) (psdomain.ProductionOwnershipDecision, error) {
	validity, err := adapter.validity(matched, asOf)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}

	spec := psdomain.ProductionOwnershipDecisionSpec{
		Scope:            scope,
		AdmissionControl: control,
		SuspensionRef:    suspensionRef,
		AsOf:             asOf,
		Validity:         validity,
		DecisionAt:       asOf,
	}
	if err := fill(&spec); err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}

	revision, err := psdomain.NewProductionOwnershipRevision(revisionFor(spec, matched))
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf(
			"%w: ownership revision: %v", ErrUntranslatableAnswer, err)
	}
	decisionID, err := psdomain.NewProductionOwnershipDecisionID(
		"PG-OWN-DEC/" + scope.Digest().String() + "/" + revision.String())
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf(
			"%w: ownership decision ID: %v", ErrUntranslatableAnswer, err)
	}
	ruleVersion, err := psdomain.NewProductionOwnershipRuleVersion(ownershipRuleVersion)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf(
			"%w: ownership rule version: %v", ErrUntranslatableAnswer, err)
	}
	spec.DecisionID = decisionID
	spec.Revision = revision
	spec.RuleVersion = ruleVersion

	decision, err := psdomain.NewProductionOwnershipDecision(spec)
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, fmt.Errorf(
			"%w: production ownership decision: %v", ErrUntranslatableAnswer, err)
	}
	return decision, nil
}

// validity 形成答复的有效期间。上界一律以装配方声明的答复视界为准，命中的区间若已登记
// 终点则再往内收——登记的终点是事实，视界是本方愿意信多久，取两者更早的那个，答复就既
// 不会活过登记册也不会活过声明。
func (adapter *ProductionOwnershipAdapter) validity(
	matched pgdomain.AuthorityInterval,
	asOf time.Time,
) (psdomain.OwnershipValidityInterval, error) {
	from := asOf
	if !matched.From.IsZero() {
		from = matched.From.UTC()
	}
	until := asOf.Add(adapter.deps.AnswerValidity)
	if !matched.To.IsZero() && matched.To.UTC().Before(until) {
		until = matched.To.UTC()
	}
	validity, err := psdomain.NewOwnershipValidityInterval(from, until)
	if err != nil {
		return psdomain.OwnershipValidityInterval{}, fmt.Errorf(
			"%w: ownership validity: %v", ErrUntranslatableAnswer, err)
	}
	return validity, nil
}

// revisionFor 派生修订号：它必须在登记册变一处时就变一次，否则调用方带着旧修订来问，
// 门禁不会判`决定已过期`。因此权威身份、区间身份与准入控制三样全部进串。
//
// 停写证据不进串，靠的是治理侧「治理记录不可覆盖」那条不变式：区间身份定住之后，它的
// 接管记录就不会再换内容。那条不变式若哪天松掉，这里会漏掉一次变更而不报。
func revisionFor(
	spec psdomain.ProductionOwnershipDecisionSpec,
	matched pgdomain.AuthorityInterval,
) string {
	revision := "PG-OWN-REV/" + spec.Authority.String()
	switch spec.Authority {
	case psdomain.ProductionAuthorityUnresolved:
		revision += "/" + spec.UnresolvedReason.String()
	default:
		revision += "/" + intervalIdentity(matched)
	}
	revision += "/" + spec.AdmissionControl.String()
	if reference, paused := spec.SuspensionRef, spec.AdmissionControl == psdomain.AdmissionControlPaused; paused {
		revision += "/" + reference.String()
	}
	return revision
}

// intervalIdentity 是权威区间的四维身份加边界，与治理侧的唯一约束同一组列。开放区间
// 的终点写成 OPEN 而不是空串：空串与「终点碰巧格式化成空」分不开。
func intervalIdentity(interval pgdomain.AuthorityInterval) string {
	to := "OPEN"
	if !interval.To.IsZero() {
		to = interval.To.UTC().Format(time.RFC3339)
	}
	return interval.ObjectScope + "|" + interval.Capability + "|" + interval.FactKind +
		"|" + interval.Authority + "|" + interval.From.UTC().Format(time.RFC3339) + "|" + to
}
