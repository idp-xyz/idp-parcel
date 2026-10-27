package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.AcceptanceBaselineGeoProjectionView（ADR-0301，票 demo-chain-gaps/143）。
//
// **不走 findAcceptedRequestCoveringParcel**：那条路把整份快照读回、过重建门，其余按（租户，包裹）答问的读口用它是因为
// 它们要在聚合上问问题。本口存在的理由恰恰是让消费方不经手整份委托——声明测量、接受决定与商业依据、接受后的资料修订
// 都不该为了取两端的国家与邮编被读进来——所以 SQL 只从快照里取这几格：接受基线（`baseline`），当前提交版本的标识、
// 成员与地址要素子段（`currentVersion` 下的 `versionId`、`declaredParcelIds`、`elements`），与接受决定的时刻
// （`decision.decidedAt`）。按段取快照有先例（ShipmentRequestViews.ListAwaitingManualReview 只取 `acceptanceTask`）；
// 基线与要素子段仍用重建那条路上的同一组文档类型解码（acceptanceBaselineDocument、elementsDocument.declaredElements），
// 这两段怎么读只有一处定义。
//
// 不过重建门，就要自己守重建门里关于接受基线的那几条（domain 的 acceptedProductsValidForRehydration）：基线在、指当前
// 提交版本、有固定时刻、成员与当前提交版本一致、时刻等于接受决定的时刻。哪一条不成立，重建门都会拒这份委托，本口答
// domain.ErrAcceptanceBaselineInconsistent，不把重建门会拒的委托当好答案交出。重建门其余各条（接受决定是否为接受、商业
// 依据与预计承诺、判断任务、资料版本、撤回）本口不读也不判，那是 FindBySourceIdentity 的事（ADR-0301 越权风险点 1）。
//
// 取行的谓词与 findAcceptedRequestCoveringParcel 逐字相同，走同一个部分 GIN（迁移 0006，只认已接受）；一条 SQL 取完，
// 没有「反查一次、读聚合一次」之间委托在动的窗口。不加列不加迁移。

var _ ports.AcceptanceBaselineGeoProjectionView = (*ShipmentRequests)(nil)

// LoadAcceptanceBaselineGeoProjection 按（租户，声明包裹身份）答接受基线那一版提交的地理投影；各格答法见端口头注。
//
// 零行答 found=false——按统一不可见结果，不区分不存在、他租户与未授权；仅`已提交`的委托不在部分索引里，其成员同答。多于
// 一行是歧义（ADR-0060），上抛而不挑一份；LIMIT 2 只为分辨「恰一」与「多于一」，ORDER BY 故意没有。
func (repository *ShipmentRequests) LoadAcceptanceBaselineGeoProjection(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.AcceptanceBaselineGeoProjection, bool, error) {
	none := domain.AcceptanceBaselineGeoProjection{}
	if tenant.String() == "" || parcel.String() == "" {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: tenant and parcel identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT customer_account_id,
		        shipment_request_id,
		        snapshot -> 'baseline',
		        snapshot #>> '{currentVersion,versionId}',
		        snapshot #> '{currentVersion,declaredParcelIds}',
		        snapshot #> '{currentVersion,elements}',
		        snapshot #> '{decision,decidedAt}'
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND state = $2
		    AND declared_parcel_ids @> ARRAY[$3]::text[]
		  LIMIT 2`,
		tenant.String(),
		uint8(domain.ShipmentRequestAccepted),
		parcel.String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	defer rows.Close()

	type hit struct {
		customer, requestID string
		baseline            []byte
		currentVersion      *string
		currentMembers      []byte
		elements            []byte
		decidedAt           []byte
	}
	var hits []hit
	for rows.Next() {
		var row hit
		if err := rows.Scan(&row.customer, &row.requestID, &row.baseline,
			&row.currentVersion, &row.currentMembers, &row.elements, &row.decidedAt); err != nil {
			return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
		}
		hits = append(hits, row)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	if len(hits) == 0 {
		return none, false, nil
	}
	if len(hits) > 1 {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", domain.ErrAmbiguousParcelTarget)
	}
	row := hits[0]

	var baseline *acceptanceBaselineDocument
	var currentMembers []string
	var decidedAt *time.Time
	var elements *elementsDocument
	for _, segment := range []struct {
		raw    []byte
		target any
	}{
		{row.baseline, &baseline},
		{row.currentMembers, &currentMembers},
		{row.decidedAt, &decidedAt},
		{row.elements, &elements},
	} {
		if len(segment.raw) == 0 {
			continue
		}
		if err := json.Unmarshal(segment.raw, segment.target); err != nil {
			return none, false, fmt.Errorf("load acceptance baseline geo projection: 快照不是本适配器写下的形状：%w", err)
		}
	}

	// 先判整份委托的基线立不立得住，再看问的这件在不在基线里：重建门拒的是整份委托，与问的是哪一件无关。各条的先后与
	// 重建门相同。
	inconsistent := func(why string) error {
		return fmt.Errorf("load acceptance baseline geo projection: accepted request %q: %w: %s",
			row.requestID, domain.ErrAcceptanceBaselineInconsistent, why)
	}
	switch {
	case baseline == nil:
		return none, false, inconsistent("no acceptance baseline")
	case row.currentVersion == nil || baseline.SubmissionVersion != *row.currentVersion:
		return none, false, inconsistent(fmt.Sprintf("baseline is fixed on version %q, not on the current submission version",
			baseline.SubmissionVersion))
	case baseline.FixedAt.IsZero():
		return none, false, inconsistent("baseline has no fixed time")
	case !sameMembers(baseline.DeclaredParcelIDs, currentMembers):
		return none, false, inconsistent("baseline members differ from the current submission version")
	case decidedAt == nil || !baseline.FixedAt.Equal(*decidedAt):
		return none, false, inconsistent("baseline fixed time differs from the acceptance decision time")
	}
	// 基线与当前提交版本成员一致之后，部分索引上的 `declared_parcel_ids` 命中而基线里没有它，只剩投影列与快照不一致这一种。
	// 那是一件不在基线里的对象，照其余按包裹读口的答法答「不是」：成员归属以基线为准。
	if !baselineDocumentCovers(*baseline, parcel) {
		return none, false, nil
	}

	version, err := domain.NewSubmissionVersionID(baseline.SubmissionVersion)
	if err != nil {
		return none, false, inconsistent(fmt.Sprintf("baseline version: %v", err))
	}
	declared, err := elements.declaredElements()
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	customer, err := domain.NewCustomerAccountID(row.customer)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	requestID, err := domain.NewShipmentRequestID(row.requestID)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	projection, err := domain.NewAcceptanceBaselineGeoProjection(domain.AcceptanceBaselineGeoProjectionSpec{
		ShipmentRequestID: requestID,
		CustomerAccountID: customer,
		BaselineVersion:   version,
		FixedAt:           baseline.FixedAt,
		Geo:               domain.GeoProjectionOf(declared),
	})
	if err != nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: %w", err)
	}
	return projection, true, nil
}

// baselineDocumentCovers 照 domain.AcceptanceBaseline 的成员判法比原串：基线是成员集合的权威。
func baselineDocumentCovers(baseline acceptanceBaselineDocument, parcel domain.DeclaredParcelID) bool {
	for _, member := range baseline.DeclaredParcelIDs {
		if member == parcel.String() {
			return true
		}
	}
	return false
}

// sameMembers 照重建门的 sameDeclaredParcels 比两份成员清单：按多重集合比，与先后无关。
func sameMembers(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, member := range left {
		counts[member]++
	}
	for _, member := range right {
		counts[member]--
		if counts[member] < 0 {
			return false
		}
	}
	return true
}
