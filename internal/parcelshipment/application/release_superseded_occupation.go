package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

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
	// 读不回某一版的判断也停：不知道旧版本形成过什么就发，settlement-accounting 无从认领；跳过它去判新版本，又会
	// 让一笔可能还在的占用压着新版本的控制。
	if gap := scope.releaseRecorded(ctx, priors); gap.open() {
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
