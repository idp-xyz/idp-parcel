package parcelpricing

import (
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// pendingIssueCauses 把提供方`待判断`评价的问题项原词译成本上下文的停因细分。按缺的是哪一类东西分，分法就是续办的分法：
// 分区与偏远档位出自目录，参考序列与汇率出自序列，价表上没有这一段出自卡，尺寸与包裹特征出自包裹那一侧的事实。
var pendingIssueCauses = map[string]sadomain.EvaluationStopCause{
	"ZONE_UNRESOLVED":              sadomain.StopEvaluationCatalogueUnresolved,
	"REMOTE_TIER_UNRESOLVED":       sadomain.StopEvaluationCatalogueUnresolved,
	"REFERENCE_SERIES_UNRESOLVED":  sadomain.StopEvaluationSeriesUnresolved,
	"EXCHANGE_RATE_UNRESOLVED":     sadomain.StopEvaluationSeriesUnresolved,
	"RATE_NOT_FOUND":               sadomain.StopEvaluationRateNotFound,
	"DIMENSIONS_REQUIRED":          sadomain.StopEvaluationInputIncomplete,
	"PACKAGE_FEATURES_UNAVAILABLE": sadomain.StopEvaluationInputIncomplete,
}

// pendingCauseOf 交回一份`待判断`评价缺的是哪一类。问题项为空、分属不止一类、或带本表没有的原词，落笼统的待判断：笼统那
// 一格卡、序列、目录三类登记都续办，分错一格却会让它等一类永远不来的登记。
func pendingCauseOf(issues []ppdomain.EvaluationIssue) sadomain.EvaluationStopCause {
	cause := sadomain.EvaluationStopCauseInvalid
	for _, issue := range issues {
		classified, known := pendingIssueCauses[issue.Code()]
		if !known || (cause != sadomain.EvaluationStopCauseInvalid && classified != cause) {
			return sadomain.StopEvaluationPendingJudgment
		}
		cause = classified
	}
	if cause == sadomain.EvaluationStopCauseInvalid {
		return sadomain.StopEvaluationPendingJudgment
	}
	return cause
}
