package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// CC 三类信封只投 VE 投影：
//   - CC 关务案件建立（不 FanOut：多成员信封，成员维与案件维一并进事实引用，ADR-0066）；
//   - CC 申报提交版本形成（不 FanOut：同为多成员信封，成员维进引用、提交版本走版本维，ADR-0066）；
//   - CC 外部结果已收（不 FanOut：放行门禁核对按动作请求驱动、不订阅到达；只派生这一次接收实际形成的那一层，沿结果范围
//     指名的申报单元逐成员展开，ADR-0197）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veCustomsCase, err := deriveCustomsCaseConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		veDeclarationSubmission, err := deriveDeclarationSubmissionConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		veExternalResult, err := deriveExternalResultConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.CustomsCaseEstablishedEventType, position: positionProjection, consumer: veCustomsCase,
				sentinels: veCustomsCaseUndecidedSentinels, label: "customs case projection",
			},
			{
				event: veinbox.DeclarationSubmissionFormedEventType, position: positionProjection, consumer: veDeclarationSubmission,
				sentinels: veDeclarationSubmissionUndecidedSentinels, label: "declaration submission projection",
			},
			{
				event: veinbox.ExternalResultReceivedEventType, position: positionProjection, consumer: veExternalResult,
				sentinels: veExternalResultUndecidedSentinels, label: "external result projection",
			},
		}, nil
	})
}
