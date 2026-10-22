package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// occupationRelease 是「按原业务关联放掉某几版已成立的占用」这件事的唯一一处定义。凡是接受已确定不成立、要放占用
// 的路都经它发；各路只决定放哪几版、没收口时怎么办——停下等重投，还是留补偿续办。怎么认、发不发、续办引用怎么
// 派生，在这里：分在各路各写一遍，一处改了「看成立项而不看结论」或引用范围、另几处照旧，同一笔占用就会因停在
// 哪条路上而得到两种处置、两个引用。
type occupationRelease struct {
	judgments ports.RecordedJudgmentReader
	release   ports.PreAcceptanceControlRelease
	identity  domain.SourceIdentity
	request   domain.ShipmentRequestID
}

// occupationReleaseGap 是这一轮没能收口的那一处：原因，与续办范围里跟在委托之后的那几段（版本；放不掉时再带控制
// 关联）。零值表示要放的各版都已收口。
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

// releaseFormed 按原业务关联放掉一版已读回的控制结果占下的各项。
//
// 看「有没有成立的项」，不看结论（ADR-0125 决定四）：账期额度占用与「第一项冻结成立、第二项受限」那笔冻结都在
// 提供方账本上，按结论只放 `HELD` 会把它们留成孤儿。反过来，什么都没占下（从未形成控制、明确无控制、唯一一项
// 受限）就没有可释放的关联——凭空发一次释放会让 settlement-accounting 去认领一笔不存在的占用。
//
// 不记「放过没有」：认领与幂等归提供方账本（ADR-0122 决定三），已释放的再请求一次答原释放；本方另存一份释放
// 状态，就是同一事实的第二处定义。
func (scope occupationRelease) releaseFormed(
	ctx context.Context,
	version domain.SubmissionVersionID,
	control domain.FinancialControlResult,
) occupationReleaseGap {
	if !control.OccupationFormed() {
		return occupationReleaseGap{}
	}
	if err := scope.release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          scope.identity,
		ShipmentRequestID: scope.request,
		SubmissionVersion: version,
		ControlResultID:   control.ResultID(),
	}); err != nil {
		return occupationReleaseGap{
			reason: ControlReleasePending,
			scope:  []string{version.String(), control.ResultID().String()},
		}
	}
	return occupationReleaseGap{}
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
) occupationReleaseGap {
	var first occupationReleaseGap
	for _, version := range versions {
		recorded, err := scope.judgments.LoadRecordedJudgments(ctx, scope.identity.TenantID(), scope.request, version)
		if err != nil {
			first = first.or(occupationReleaseGap{reason: RecordedJudgmentsUnavailable, scope: []string{version.String()}})
			continue
		}
		first = first.or(scope.releaseFormed(ctx, version, recorded.FinancialControl))
	}
	return first
}

// releaseEvery 放一份走到终局的委托各版已成立的占用：命令指名那一版在前，其余各版随后。
func (scope occupationRelease) releaseEvery(
	ctx context.Context,
	request domain.ShipmentRequest,
	named domain.SubmissionVersionID,
) occupationReleaseGap {
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
