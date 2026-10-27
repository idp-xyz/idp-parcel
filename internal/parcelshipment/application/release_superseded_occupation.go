package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrControlReleaseNotAccepted 说明接受链判当前版本之前放被取代各版时，提供方确定不受理其中一笔的释放。它不进未决：
// 重投同一封信封得到同一个答复，折进内部重试会一直重投到失败预算耗尽（ADR-0081 决定三，ADR-0282 决定三）；本步也
// 不记释放未受理册——这一轮整笔回滚，记下的会随之撤回。上抛之后消费门落 `dispatch.publish_failed`，要人来看这一笔。
var ErrControlReleaseNotAccepted = errors.New("parcel shipment: control release was not accepted by the provider")

// releaseSuperseded 在判当前版本之前，按原业务关联放掉被取代各版已成立的占用。
//
// 被取代的那一版接受已确定不成立：新版本成为唯一待判断版本（ADR-0045），它不会再被判，占着客户的资金或额度等
// 一份不会到来的接受没有依据——ADR-0132 决定一给`交客户补充`的理由，换代同理。CONTEXT 词条「控制项结果」因此要求
// 释放，不论那一版停在哪个等待态上被取代。
//
// 为什么落在链的开头而不是形成新版本那一笔：
//   - 先放后判。新版本的控制若先于释放，客户的余额或额度被旧版本那一笔吃着，新版本可能因此受限、按策略被拒——
//     拒绝不可覆盖，一次可续办的补偿就被改写成了业务结果。放在这里，同一轮里释放与新版本的控制先后分明。
//   - 有人续办。放不掉时整轮停在本步、恢复动作是内部重试，消费门回滚重投，下一轮再放；形成新版本那一笔是客户的
//     一次请求，失败了只能交回一条补偿引用，之后没有任何东西会去推它。
//   - 释放出口只此一处装配。链的形成决定那一步本就持有判断读口与释放端口（拒绝时放本版本那一笔），这里用同一对，
//     不为换代另接第二个释放端口，两处也就不会各认各的账本。
//
// 释放随整轮一起提交：它与新版本的判断同在消费门那一笔事务里，这一轮因别的原因回滚，释放也一起回滚。所以新版本
// 那一轮能提交之前，被取代那一版一直占着——先放后判守住的是新版本的控制看不到旧占用，不是旧占用尽早腾出来。
//
// 每一轮都问，不记「放过没有」，只问「有没有成立的项」，不看结论：逐版怎么认、发不发归 occupationRelease。
//
// 提供方确定不受理某一笔（或释放口形不成三格之一）时响亮上抛，不停在本步内部重试：重投推不动它，新版本却一直被挡着
// （ADR-0282 决定三）。可续办的缺口与拒收同时出现时也上抛——拒收不解开，续办成功了也判不了新版本。人已处理的那一笔除外
// （unhandledRefusals）：处理动作同笔交了续办信封，这一步就是被那封信封再驱时放行的地方（ADR-0293 决定四）。
//
// 不论委托此刻是否已形成决定都照放：被取代的版本从来不是接受所在的那一版（决定形成后不再换代），放它碰不到合法
// 占用。
func (handler *AdvanceAcceptanceChainHandler) releaseSuperseded(
	ctx context.Context,
	command AdvanceAcceptanceChainCommand,
	request domain.ShipmentRequest,
) (AdvanceAcceptanceChainResult, bool, error) {
	priors := versionsOtherThan(request, command.SubmissionVersion)
	if len(priors) == 0 {
		return AdvanceAcceptanceChainResult{}, false, nil
	}
	decision := handler.deps.Decision.deps
	if decision.Judgments == nil || decision.Release == nil {
		return AdvanceAcceptanceChainResult{}, false, ErrAcceptanceChainNotAssembled
	}

	scope := occupationRelease{
		judgments: decision.Judgments,
		release:   decision.Release,
		identity:  command.Identity,
		request:   command.ShipmentRequestID,
	}
	outcome := scope.releaseRecorded(ctx, priors)
	refusals, unreadable := handler.unhandledRefusals(ctx, command.Identity.TenantID(), command.ShipmentRequestID, outcome.refusals)
	if len(refusals) > 0 {
		return AdvanceAcceptanceChainResult{}, false, refusedSupersededRelease(refusals[0])
	}
	// 读不回某一版的判断也停：不知道旧版本形成过什么就发，settlement-accounting 无从认领；跳过它去判新版本，又会
	// 让一笔可能还在的占用压着新版本的控制。
	if gap := outcome.pending.or(unreadable); gap.open() {
		return AdvanceAcceptanceChainResult{
			outcome:      AcceptanceChainUndecided,
			stage:        AcceptanceChainSupersededReleaseStage,
			state:        request.State(),
			reason:       gap.reason,
			continuation: scope.reference(gap),
		}, true, nil
	}
	return AdvanceAcceptanceChainResult{}, false, nil
}

// unhandledRefusals 滤掉人已处理的拒收（ADR-0293 决定四）：提供方对某一笔确定不受理、而释放未受理册上那一笔在处理留痕里有行，说明人已经
// 核对过它——在结算侧放掉了，或标记了资金已在本产品之外处置——链凭留痕认出这一笔并放行，接着判新版本。提供方先被问：人放掉的，提供方答已收口，
// 走不到这里；走到这里又处理过的，是标记已处理的存量行。
//
// 只认处理留痕里（版本、控制关联）逐段对得上的那一笔，别的版本的拒收上抛。留痕读不回既不当成处理过、也不当成没处理过：那一笔记进缺口，
// 停在本步内部重试（ADR-0029：读不动才是未决）。读口没接上（nil）时一概不滤，退回 ADR-0282 决定三原样。
func (handler *AdvanceAcceptanceChainHandler) unhandledRefusals(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.ShipmentRequestID,
	refusals []occupationRefusal,
) ([]occupationRefusal, occupationReleaseGap) {
	view := handler.deps.Decision.deps.RefusalHandlings
	if view == nil {
		return refusals, occupationReleaseGap{}
	}
	var open []occupationRefusal
	var unreadable occupationReleaseGap
	for _, refusal := range refusals {
		_, handled, err := view.FindControlReleaseRefusalHandling(ctx, ports.ControlReleaseRefusalKey{
			TenantID:          tenant,
			ShipmentRequestID: request,
			SubmissionVersion: refusal.version,
			ControlResultID:   refusal.control.ResultID(),
		})
		switch {
		case err != nil:
			unreadable = unreadable.or(occupationReleaseGap{
				reason: ControlReleasePending,
				scope:  []string{refusal.version.String(), refusal.control.ResultID().String()},
			})
		case !handled:
			open = append(open, refusal)
		}
	}
	return open, unreadable
}

// refusedSupersededRelease 把第一笔拒收译成上抛的错误。版本、控制关联与原因都写进正文：路由条目把它记成
// publish_failed，原错误原样留在链上供运维读。释放口形不成三格之一时带着原错误，提供方拒收时带本包的哨兵。
func refusedSupersededRelease(refusal occupationRefusal) error {
	if refusal.cause != nil {
		return fmt.Errorf("release superseded version %s, control %s: %w",
			refusal.version, refusal.control.ResultID(), refusal.cause)
	}
	return fmt.Errorf("%w: superseded version %s, control %s, reason %s",
		ErrControlReleaseNotAccepted, refusal.version, refusal.control.ResultID(), refusal.reason)
}
