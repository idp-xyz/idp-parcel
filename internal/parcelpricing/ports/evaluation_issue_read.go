package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是被挂起评价联动的伴生读端口（ADR-0105 Decision 五；票 pricing-reference-series-operations/05
// 第 2 项）。不拓宽 EvaluationCatalogueRead——扩写既有接口会拆全部替身（evaluation_read.go 头注那条）。
//
// 它只读问题项子表与父表的列（状态、租户），按（状态 = 待判断，原因码 ∈ REFERENCE_SERIES_UNRESOLVED /
// EXCHANGE_RATE_UNRESOLVED）筛、按序列种类分组数**评价**（不是问题项——一次评价同一种类可能带多条）。
// 不扫 JSONB、不解释原因码之外的任何语义、不开「按原因码任意筛」的通用口（Decision 六）。
//
// 计数覆盖的是子表有行的评价（Decision 四不回填）；租户内没有一条挂起评价时交回空列表，那是正常答案。

// PendingSeriesEvaluationCount 是一种序列下被挂起的评价数。
type PendingSeriesEvaluationCount struct {
	Kind            string
	EvaluationCount int
}

// PendingSeriesEvaluationRead 按租户数因序列未解析而待判断的评价，按序列种类分组。
type PendingSeriesEvaluationRead interface {
	CountPendingSeriesEvaluations(ctx context.Context, tenant domain.TenantID) ([]PendingSeriesEvaluationCount, error)
}
