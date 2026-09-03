package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是序列版本复核的写口与在用版本的读口（ADR-0099 决定二、三；票
// pricing-reference-series-operations/02）。两口都**不拓宽** ReferenceSeriesRegister——
// 扩写侧接口会拆全部写侧测试替身（catalogue_read.go 记过这条），复核与在用各自另立。
//
// 复核是追加在版本之旁的独立事实；在用版本是从复核记录按评价形成时刻派生的结论，不是
// 一个可维护的指针。读口只取候选、不在 SQL 里裁决——「按复核时刻、同刻按登记时刻、再按
// 版本号」这条判定在 domain.SelectInForceSeriesVersion 一处，SQL 若也排一遍就是两处口径。

// ReferenceSeriesReviewOutcome 是记录一条复核的封闭结果。
type ReferenceSeriesReviewOutcome uint8

const (
	ReferenceSeriesReviewOutcomeInvalid ReferenceSeriesReviewOutcome = iota
	// ReferenceSeriesReviewRecorded：复核已追加。
	ReferenceSeriesReviewRecorded
	// ReferenceSeriesReviewAlreadyRecorded：同（版本、复核时刻、复核责任方）且同结论同依据
	// 已在册——重复记录是幂等重放，不是错误。
	ReferenceSeriesReviewAlreadyRecorded
	// ReferenceSeriesReviewConflict：同键在册而结论或依据不同。复核记录只增不改，原行不被
	// 顶替；改主意就再追加一条新时刻的复核。
	ReferenceSeriesReviewConflict
	// ReferenceSeriesReviewVersionUnknown：被复核的序列版本不在册。消费方的恢复动作是先
	// 登记那一版，所以它是一格答案不是 error。
	ReferenceSeriesReviewVersionUnknown
)

// ReferenceSeriesReviewRegister 是序列版本复核的追加写口。
type ReferenceSeriesReviewRegister interface {
	// Record 追加一条复核。行只增不改；同键重复按（结论 + 依据）比对译成结果代数。
	Record(ctx context.Context, review domain.SeriesReview) (ReferenceSeriesReviewOutcome, error)
}

// InForceOutcome 是在用版本解析的封闭结果。三个「没有」按消费方的恢复动作分格（ADR-0029
// 同一判据）：没登记要去登记，登了没复核要去复核，种类不合要改绑定或改登记——三个都是
// 登记侧的一次动作，评价侧一律落待判断，但原因文字要分得开。
type InForceOutcome uint8

const (
	InForceOutcomeInvalid InForceOutcome = iota
	// SeriesVersionInForce：解析到在用版本，引用随答案交回。
	SeriesVersionInForce
	// SeriesHasNoRegisteredVersion：该租户内没有这条序列的任何版本。
	SeriesHasNoRegisteredVersion
	// SeriesHasNoApprovedVersion：有版本，但在该时刻之前没有任何一版复核通过。
	SeriesHasNoApprovedVersion
	// SeriesKindDisagrees：这条序列在册的种类与问的种类不同——方案绑错了序列，或序列登
	// 错了种类，两者都不是「再登一版」能修的。
	SeriesKindDisagrees
)

// ReferenceSeriesInForceResolver 按（租户、种类、序列标识、评价形成时刻）解析在用版本。
type ReferenceSeriesInForceResolver interface {
	ResolveInForce(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.ReferenceSeriesKind,
		seriesID string,
		at time.Time,
	) (domain.VersionReference, InForceOutcome, error)
}
