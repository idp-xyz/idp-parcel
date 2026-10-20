package main

import (
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 客户视图两路（UC-VE-008）：PS 接受决定 → VE 客户归属确立补派生（AT-VE-169，与 NR 初始路由同扇出、排在它前面），
// VE 投影派生 → 客户视图（UC-VE-008 内部半边：账户维经 PS 按包裹反查填上，ADR-0060 三格）。
//
// `visibility-exception.tracking-projection.derived` 已登记（UC-VE-008）：早先不登记
// 的理由是 Customer 那一维填不上；ADR-0060 的按包裹反查把账户随来源身份一并交回之后
// 本进程真接得住它了——接得住才登记，正是 ADR-0049 决定三的判据。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veAcceptance, err := deriveCustomerViewOnAcceptanceConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		veCustomerView, err := deriveCustomerViewConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: nrinbox.AcceptedDecisionEventType, position: positionProjection, consumer: veAcceptance,
				sentinels: veAcceptanceRederiveUndecidedSentinels, label: "acceptance rederive",
			},
			{
				event: veinbox.TrackingProjectionDerivedEventType, position: positionProjection, consumer: veCustomerView,
				sentinels: veCustomerViewUndecidedSentinels, label: "customer view",
			},
		}, nil
	})
}
