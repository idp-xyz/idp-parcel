package main

import (
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapricing "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
)

// evaluationRequestUnformedUndecidedSentinels 是 PP「评价未形成」→ SA 记停因这一路（ADR-0229 决定三）的未决哨兵：
//   - sapricing.ErrEvaluationUnformedRecordingUndecided——请求册答不出，停因记没记下未知，整笔回滚等重投。
//
// 不在名单里、落 publish_failed 的：PP 的方向或停因词译不过来、引用坏了（sapricing.ErrUntranslatableAnswer、
// ErrUntranslatableReference）；请求不在册上或册上另有结论（两侧矛盾，重投不自愈）。
var evaluationRequestUnformedUndecidedSentinels = []error{
	sapricing.ErrEvaluationUnformedRecordingUndecided,
}

// evaluationResumptionUndecidedSentinels 是三扇续办门共用的未决哨兵（ADR-0229 决定五）：
//   - sainbox.ErrEvaluationResumptionUndecided——队列读不动，或至少一件续办停在依赖答不出上。整封重投时已续办的那几件
//     因为有了下一代或有了请求不再被取，只有没续办成的再来一次。
//
// 不在名单里、落 publish_failed 的：某一件续办撞上矛盾（队列交出的件回读不到、记下的输入重跑被发起编排拒收、信封被
// 框架确定性拒收）。三扇门一份名单：同一段续办代码、同一张未决面，各列一份就会有漂开的那一天。
var evaluationResumptionUndecidedSentinels = []error{
	sainbox.ErrEvaluationResumptionUndecided,
}

// PP 评价未形成 → SA 记停因；三类「参数已登记」→ SA 续办（ADR-0229）。「结算账户已登记」这一类信封 PS 那扇作用域续办门
// 也收（routes_acceptance_chain.go），本路排在计费名次：先让受理链续办委托，再续办停在发起之前的计费那一拍。另两类信封
// 只有本路收。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		unformed, err := recordEvaluationUnformedConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		legs := []routeLeg{{
			event: sainbox.EvaluationRequestUnformedEventType, position: positionPrimary, consumer: unformed,
			sentinels: evaluationRequestUnformedUndecidedSentinels, label: "evaluation request unformed",
		}}
		for _, registration := range []struct {
			registration sainbox.EvaluationParameterRegistration
			position     fanOutPosition
			label        string
		}{
			{sainbox.SettlementAccountRegisteredResumption, positionBilling, "evaluation resumption on settlement account"},
			{sainbox.EvaluationParameterRegisteredResumption, positionPrimary, "evaluation resumption on evaluation parameter"},
			{sainbox.PricingParameterRegisteredResumption, positionPrimary, "evaluation resumption on pricing parameter"},
		} {
			consumer, err := evaluationResumptionConsumer(
				registration.registration, sources.db, sources.outbox, sources.inbox, sources.clock)
			if err != nil {
				return nil, err
			}
			legs = append(legs, routeLeg{
				event: registration.registration.EventType, position: registration.position, consumer: consumer,
				sentinels: evaluationResumptionUndecidedSentinels, label: registration.label,
			})
		}
		return legs, nil
	})
}
