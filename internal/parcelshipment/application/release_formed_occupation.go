package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// occupationRelease 是「按原业务关联放掉某几版已成立的占用」这件事的唯一一处定义。凡是接受已确定不成立、要放占用
// 的路都经它发；各路只决定放哪几版、没收口时怎么办——停下等重投、留补偿续办，还是记进释放未受理册。怎么认、发不发、
// 释放的答复落哪一格、续办引用怎么派生，在这里：分在各路各写一遍，一处改了「看成立项而不看结论」或引用范围、另几处
// 照旧，同一笔占用就会因停在哪条路上而得到两种处置、两个引用。
type occupationRelease struct {
	judgments ports.RecordedJudgmentReader
	release   ports.PreAcceptanceControlRelease
	identity  domain.SourceIdentity
	request   domain.ShipmentRequestID
	// refusals 与 clock 只有终局各路与暂停那一路用：释放确定未受理时记进释放未受理册（ADR-0282 决定二）。接受链开头那一步
	// 不记册，遇到它响亮停下（决定三）。
	refusals ports.ControlReleaseRefusalRegister
	clock    ports.Clock
	// scopeLock 只有保存委托的各路用：保存之前先取作用域锁（lockScopesBeforeSave）。接受链开头放被取代各版那一步不用它，
	// 链里取锁的次序由施加与释放自己的取锁决定。
	scopeLock ports.PreAcceptanceControlScopeLock
}

// occupationReleaseGap 是这一轮没能收口、可以续办的那一处：原因，与续办范围里跟在委托之后的那几段（版本；放不掉时再带
// 控制关联）。零值表示没有可续办的缺口。
type occupationReleaseGap struct {
	reason JudgmentPendingReason
	scope  []string
}

func (gap occupationReleaseGap) open() bool {
	return gap.reason != PendingReasonNone
}

// or 保留先出现的那一处。只交回一处，是因为各路的结果都只有一格续办或补偿引用；交第一处，是因为终局各路把命令
// 指名那一版排在最前，那一笔正是调用方此刻在办的那一份。
func (gap occupationReleaseGap) or(later occupationReleaseGap) occupationReleaseGap {
	if gap.open() {
		return gap
	}
	return later
}

// occupationRefusal 是一版释放确定未受理的那一笔：提供方拒收了请求，或释放口形不成三格之一（cause 在场）。重放同一
// 请求得到同一个答复，补偿续办推不动它（ADR-0282 决定一）。
type occupationRefusal struct {
	version domain.SubmissionVersionID
	control domain.FinancialControlResult
	reason  domain.CheckReason
	cause   error
}

// occupationReleaseOutcome 是放几版占用的结局：第一处可续办的缺口，加上每一笔确定未受理的占用。确定未受理的逐笔留着
// 而不只留第一笔：每一笔都要各自记进册里交人处理，册上少一笔就是少放一笔钱。
type occupationReleaseOutcome struct {
	pending  occupationReleaseGap
	refusals []occupationRefusal
}

func (outcome occupationReleaseOutcome) and(later occupationReleaseOutcome) occupationReleaseOutcome {
	return occupationReleaseOutcome{
		pending:  outcome.pending.or(later.pending),
		refusals: append(append([]occupationRefusal(nil), outcome.refusals...), later.refusals...),
	}
}

// reasonControlReleaseAnswerUntranslatable 是释放口形不成三格之一时记进册里的原因。本上下文自有：那一格没有提供方给的
// 原因可抄，而册上每一行都要有一个稳定原因。
const reasonControlReleaseAnswerUntranslatable = "CONTROL_RELEASE_ANSWER_UNTRANSLATABLE"

// reference 派生这一处的续办引用：原因 + 租户、客户账户、委托 + 版本（及控制关联）。范围与停在哪条路上无关——同一
// 笔占用因同一原因没放掉，无论停在哪条路上，续办方都拿到同一个引用，不必先知道这一次是谁停的才查得回原次尝试。
func (scope occupationRelease) reference(gap occupationReleaseGap) domain.OwnershipContinuationReference {
	if !gap.open() {
		return domain.OwnershipContinuationReference{}
	}
	return judgmentContinuation(
		gap.reason,
		append([]string{
			scope.identity.TenantID().String(),
			scope.identity.CustomerAccountID().String(),
			scope.request.String(),
		}, gap.scope...)...,
	)
}

// releaseFormed 按原业务关联放掉一版已读回的控制结果占下的各项，把答复落进三格之一（ADR-0282 决定一）。
//
// 看「有没有成立的项」，不看结论（ADR-0125 决定四）：账期额度占用与「第一项冻结成立、第二项受限」那笔冻结都在
// 提供方账本上，按结论只放 `HELD` 会把它们留成孤儿。反过来，什么都没占下（从未形成控制、明确无控制、唯一一项
// 受限）就没有可释放的关联——凭空发一次释放会让 settlement-accounting 去认领一笔不存在的占用。
//
// 不记「放过没有」：认领与幂等归提供方账本（ADR-0122 决定三），已释放的再请求一次答原释放；本方另存一份释放
// 状态，就是同一事实的第二处定义。作用域取结果上存下的那一份（ADR-0282 决定四），不在这里重新推导。
//
// 逐格落点，不留兜底：`可重试未形成`进续办；`确定未受理`与形不成三格之一都是确定性的，交给调用的那一路按自己的落法
// 处置——并进续办，补偿续办就会对着一个重放一万次也是同一答复的请求一直挂着。
func (scope occupationRelease) releaseFormed(
	ctx context.Context,
	version domain.SubmissionVersionID,
	control domain.FinancialControlResult,
) occupationReleaseOutcome {
	if !control.OccupationFormed() {
		return occupationReleaseOutcome{}
	}
	answer, err := scope.release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          scope.identity,
		ShipmentRequestID: scope.request,
		SubmissionVersion: version,
		ControlResultID:   control.ResultID(),
		Scope:             control.Scope(),
	})
	if err != nil {
		return scope.untranslatable(version, control, err)
	}
	switch answer.Outcome {
	case ports.ControlReleaseSettled:
		return occupationReleaseOutcome{}
	case ports.ControlReleaseNotFormed:
		return occupationReleaseOutcome{pending: occupationReleaseGap{
			reason: ControlReleasePending,
			scope:  []string{version.String(), control.ResultID().String()},
		}}
	case ports.ControlReleaseNotAccepted:
		return occupationReleaseOutcome{refusals: []occupationRefusal{
			{version: version, control: control, reason: answer.Reason},
		}}
	default:
		return scope.untranslatable(version, control,
			fmt.Errorf("%w: control release outcome %d", ports.ErrUntranslatableProviderAnswer, answer.Outcome))
	}
}

func (scope occupationRelease) untranslatable(
	version domain.SubmissionVersionID,
	control domain.FinancialControlResult,
	cause error,
) occupationReleaseOutcome {
	reason, _ := domain.NewCheckReason(reasonControlReleaseAnswerUntranslatable)
	return occupationReleaseOutcome{refusals: []occupationRefusal{
		{version: version, control: control, reason: reason, cause: cause},
	}}
}

// releaseRecorded 逐版读回判断，再按 releaseFormed 放。
//
// 读不回某一版的判断时不发那一版：不知道它形成过什么就发，settlement-accounting 无从认领哪一笔。
//
// 一版没收口不停下，接着放其余各版：终局路径之后再没有轮次会回来放它们，停在第一处，后面那几笔就只能等第一处
// 续办成功才有人问；接受链没收口时整轮回滚，多放的几笔随之撤回，不改变它停下的结论。
func (scope occupationRelease) releaseRecorded(
	ctx context.Context,
	versions []domain.SubmissionVersionID,
) occupationReleaseOutcome {
	var outcome occupationReleaseOutcome
	for _, version := range versions {
		recorded, err := scope.judgments.LoadRecordedJudgments(ctx, scope.identity.TenantID(), scope.request, version)
		if err != nil {
			outcome = outcome.and(occupationReleaseOutcome{
				pending: occupationReleaseGap{reason: RecordedJudgmentsUnavailable, scope: []string{version.String()}},
			})
			continue
		}
		outcome = outcome.and(scope.releaseFormed(ctx, version, recorded.FinancialControl))
	}
	return outcome
}

// settle 是终局各路与暂停那一路的收口：确定未受理的各笔记进释放未受理册、交人处理，业务事实不因此回滚（ADR-0282
// 决定二；UC-PS-005「不得把释放失败改写为撤回失败」）；交回补偿续办引用与「有一笔落进册里」两样，各路原样放进结果。
//
// 记不下册（册没接上或写不进）时那一笔改留补偿续办，原因 `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`：既没放掉也没交到人
// 手里，只剩重放能补——重放会再请求一次释放、再记一次册。它与`释放未收口`分开，是因为续办引用由原因派生：同一笔
// 占用放不掉与拒收了却没记下是两种缺口，共用一条引用，查回来的会是另一种。
func (scope occupationRelease) settle(
	ctx context.Context,
	outcome occupationReleaseOutcome,
) (domain.OwnershipContinuationReference, bool) {
	pending := outcome.pending
	recorded := false
	for _, refusal := range outcome.refusals {
		if scope.record(ctx, refusal) {
			recorded = true
			continue
		}
		pending = pending.or(occupationReleaseGap{
			reason: ControlReleaseRefusalNotRecorded,
			scope:  []string{refusal.version.String(), refusal.control.ResultID().String()},
		})
	}
	return scope.reference(pending), recorded
}

func (scope occupationRelease) record(ctx context.Context, refusal occupationRefusal) bool {
	if scope.refusals == nil || scope.clock == nil {
		return false
	}
	saved, err := scope.refusals.Record(ctx, ports.ControlReleaseRefusal{
		TenantID:          scope.identity.TenantID(),
		CustomerAccountID: scope.identity.CustomerAccountID(),
		ShipmentRequestID: scope.request,
		SubmissionVersion: refusal.version,
		ControlResultID:   refusal.control.ResultID(),
		Scope:             refusal.control.Scope(),
		Reason:            refusal.reason,
		RecordedAt:        scope.clock.Now(),
	})
	if err != nil {
		return false
	}
	switch saved {
	case ports.ControlReleaseRefusalRecorded, ports.ControlReleaseRefusalAlreadyRecorded:
		return true
	default:
		return false
	}
}

// releaseEvery 放一份走到终局的委托各版已成立的占用：命令指名那一版在前，其余各版随后。
func (scope occupationRelease) releaseEvery(
	ctx context.Context,
	request domain.ShipmentRequest,
	named domain.SubmissionVersionID,
) occupationReleaseOutcome {
	return scope.releaseRecorded(ctx, append([]domain.SubmissionVersionID{named}, versionsOtherThan(request, named)...))
}

// versionsOtherThan 交回委托除 named 之外的各版：当前版本在前，被取代各版按形成顺序随后。
//
// 终局路径要放的是这份委托已成立的占用，不只是命令指名那一版（UC-PS-005 步骤 6）：被取代那一版若在新版本那一轮链
// 提交之前就走到终局，它那一笔没有别的轮次去放。当前版本也列进来：撤回与拒绝的命令不核对版本，指名一版旧版本时，
// 只放它就把当前版本那一笔留成了孤儿。
func versionsOtherThan(request domain.ShipmentRequest, named domain.SubmissionVersionID) []domain.SubmissionVersionID {
	priors := request.PriorSubmissionVersions()
	versions := make([]domain.SubmissionVersionID, 0, len(priors)+1)
	if current := request.CurrentSubmissionVersion().VersionID(); current != named {
		versions = append(versions, current)
	}
	for _, prior := range priors {
		if prior.VersionID() != named {
			versions = append(versions, prior.VersionID())
		}
	}
	return versions
}
