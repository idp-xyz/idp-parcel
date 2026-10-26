package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.AcceptanceBaselineGeoProjectionView（ADR-0301，票 demo-chain-gaps/143）。
//
// **不走 findAcceptedRequestCoveringParcel**：那条路把整份快照读回、过重建门，其余按（租户，包裹）答问的读口用它是因为
// 它们要在聚合上问问题。本口存在的理由恰恰是让消费方不经手整份委托——申报测量、接受决定与商业依据、接受后的资料修订
// 都不该为了取两端的国家与邮编被读进来——所以 SQL 只从快照里取两段：接受基线（`baseline`），与基线所指那一版提交的
// 地址要素子段（`elements`）。按段取快照有先例（ShipmentRequestViews.ListAwaitingManualReview 只取 `acceptanceTask`）；
// 两段仍用重建那条路上的同一组文档类型解码（acceptanceBaselineDocument、elementsDocument.declaredElements），这两段
// 怎么读只有一处定义。
//
// 不过重建门就要自己守它在这两段上的那几条：基线缺席、基线没有固定时刻、基线所指的版本不在本委托里，答
// domain.ErrAcceptanceBaselineInconsistent。整份聚合是否完好不归本口担保，那是 FindBySourceIdentity 的事。
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

	// 基线所指的版本可能是当前版本，也可能在历史版本里（与 domain 的 SubmissionVersionByID 同一找法），所以把两处摊成
	// 一列再按版本标识找。found 只在找到时为真：要素子段缺席时 elements 是 NULL，与「版本找不到」得分开。
	rows, err := querier.Query(ctx,
		`SELECT request.customer_account_id,
		        request.shipment_request_id,
		        request.snapshot -> 'baseline',
		        version.found,
		        version.elements
		   FROM parcel_shipment.shipment_request AS request
		   LEFT JOIN LATERAL (
		        SELECT true AS found, candidate -> 'elements' AS elements
		          FROM jsonb_array_elements(
		                 jsonb_build_array(request.snapshot -> 'currentVersion')
		                 || COALESCE(request.snapshot -> 'priorVersions', '[]'::jsonb)
		               ) AS candidate
		         WHERE candidate ->> 'versionId' = request.snapshot #>> '{baseline,submissionVersion}'
		         LIMIT 1
		   ) AS version ON true
		  WHERE request.tenant_id = $1
		    AND request.state = $2
		    AND request.declared_parcel_ids @> ARRAY[$3]::text[]
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
		versionFound        *bool
		elements            []byte
	}
	var hits []hit
	for rows.Next() {
		var row hit
		if err := rows.Scan(&row.customer, &row.requestID, &row.baseline, &row.versionFound, &row.elements); err != nil {
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

	// 先有基线，再看成员，再看时刻与版本：基线里没有它就是答案（found=false），时刻与版本是那份答案的内容，没有答案
	// 就轮不到它们。
	var baseline *acceptanceBaselineDocument
	if len(row.baseline) > 0 {
		if err := json.Unmarshal(row.baseline, &baseline); err != nil {
			return none, false, fmt.Errorf("load acceptance baseline geo projection: 快照不是本适配器写下的形状：%w", err)
		}
	}
	if baseline == nil {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: accepted request %q: %w: no acceptance baseline",
			row.requestID, domain.ErrAcceptanceBaselineInconsistent)
	}
	if !baselineDocumentCovers(*baseline, parcel) {
		return none, false, nil
	}
	if baseline.FixedAt.IsZero() {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: accepted request %q: %w: baseline has no fixed time",
			row.requestID, domain.ErrAcceptanceBaselineInconsistent)
	}
	version, err := domain.NewSubmissionVersionID(baseline.SubmissionVersion)
	if err != nil || row.versionFound == nil || !*row.versionFound {
		return none, false, fmt.Errorf("load acceptance baseline geo projection: accepted request %q: %w: baseline version %q is not among its versions",
			row.requestID, domain.ErrAcceptanceBaselineInconsistent, baseline.SubmissionVersion)
	}

	var elements *elementsDocument
	if len(row.elements) > 0 {
		if err := json.Unmarshal(row.elements, &elements); err != nil {
			return none, false, fmt.Errorf("load acceptance baseline geo projection: 快照不是本适配器写下的形状：%w", err)
		}
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

// baselineDocumentCovers 照 domain.AcceptanceBaseline 的成员判法比原串：基线是成员集合的权威，部分索引上的
// `declared_parcel_ids` 是当前提交版本的成员，只用来找行。
func baselineDocumentCovers(baseline acceptanceBaselineDocument, parcel domain.DeclaredParcelID) bool {
	for _, member := range baseline.DeclaredParcelIDs {
		if member == parcel.String() {
			return true
		}
	}
	return false
}
