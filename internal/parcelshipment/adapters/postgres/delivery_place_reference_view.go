package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.DeliveryPlaceReferenceView（票 ps-port-remainder/06；ADR-0130 决定二）：
// 写侧仓储与「收件地点引用」读口是同一只适配器，读的是同一张表的同一份快照（判据同 ChannelSelectionDecisions
// 兼 ChannelSelectionDecisionRead）。
//
// 一条按（租户，包裹）的查询走 `declared_parcel_ids` 上的部分 GIN（迁移 0006，只认已接受）反查委托——**它只是
// 找到那一行的路，不是成员集合的权威**：命中的行整份读回、过重建门（ADR-0028），成员归属由聚合上的接受基线
// 自己回答（AcceptanceBaseline.covers），当前采用判断由 CurrentSourceDataAdoption 派生，四格由 domain 的
// DeliveryPlaceReferenceFor 译出。SQL 里没有第二套派生规则，也不解释四格里的任何一格。不加迁移：既有索引够用，
// 没有新列。
//
// 与 FindCurrentAcceptedByParcel 同一条歧义纪律：LIMIT 2 只为分辨「恰一」与「多于一」，多于一行是读面坏了
// （ADR-0060），上抛而不挑一份——四格里没有一格是「不知道」，也没有一格容得下「两份里的某一份」。

var _ ports.DeliveryPlaceReferenceView = (*ShipmentRequests)(nil)

// LoadDeliveryPlaceReference 按（租户，声明包裹身份）答收件地点引用的封闭四格。
//
// 零行命中直接答「没有收件地点」——按统一不可见结果，不区分不存在、他租户与未授权（租户维在 SQL 条件上，
// 别的租户拿同一个包裹身份得到的与真不存在同答）；仅`已提交`的委托不在部分索引里，其成员同样落这一格：责任
// 起点在接受之后。
func (repository *ShipmentRequests) LoadDeliveryPlaceReference(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.DeliveryPlaceResolution, error) {
	none := domain.DeliveryPlaceResolution{}
	if tenant.String() == "" || parcel.String() == "" {
		return none, fmt.Errorf("load delivery place reference: tenant and parcel identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}

	// ORDER BY 故意没有：有序会引诱人把第一行当「最新接受者」，而两行同时声明一件包裹没有正确的挑法。
	rows, err := querier.Query(ctx,
		`SELECT revision, state, snapshot
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
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	defer rows.Close()

	type hit struct {
		revision int64
		state    uint8
		raw      []byte
	}
	var hits []hit
	for rows.Next() {
		var row hit
		if err := rows.Scan(&row.revision, &row.state, &row.raw); err != nil {
			return none, fmt.Errorf("load delivery place reference: %w", err)
		}
		hits = append(hits, row)
	}
	if err := rows.Err(); err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	if len(hits) == 0 {
		return domain.NoDeliveryPlaceResolution(), nil
	}
	if len(hits) > 1 {
		return none, fmt.Errorf("load delivery place reference: %w", domain.ErrAmbiguousParcelTarget)
	}

	var document requestDocument
	if err := json.Unmarshal(hits[0].raw, &document); err != nil {
		return none, fmt.Errorf("load delivery place reference: 快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(hits[0].revision, domain.ShipmentRequestState(hits[0].state))
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	resolution, err := request.DeliveryPlaceReferenceFor(parcel)
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	return resolution, nil
}
