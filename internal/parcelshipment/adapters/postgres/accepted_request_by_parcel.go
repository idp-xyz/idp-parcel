package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// findAcceptedRequestCoveringParcel 是按（租户，包裹）答问的各读口（DeliveryPlaceReferenceView、
// CommercialResolutionReferenceView、DeclaredMeasurementView、AddressElementsView）共用的那一段路：走
// `declared_parcel_ids` 上的部分 GIN（迁移 0006，只认已接受）
// 找到那一行，整份读回过重建门（ADR-0028）。**它只是找到那一行的路，不是成员集合的权威**——成员归属由聚合上的接受
// 基线自己回答，各读口在读回的聚合上各问各的；端口那一侧仍是一口一问（ADR-0130 / 0133：读口不预设按委托或按引用
// 反查），共用的只有取行。
//
// 三种答法：零行 → found=false（按统一不可见结果，不区分不存在、他租户与未授权；仅`已提交`的委托不在部分索引里，
// 其成员同答）；恰一行 → 重建后的聚合；多于一行 → domain.ErrAmbiguousParcelTarget（ADR-0060），不挑一份——
// LIMIT 2 只为分辨「恰一」与「多于一」，ORDER BY 故意没有，有序会引诱人把第一行当「最新接受者」。
func (repository *ShipmentRequests) findAcceptedRequestCoveringParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.ShipmentRequest, bool, error) {
	none := domain.ShipmentRequest{}
	if tenant.String() == "" || parcel.String() == "" {
		return none, false, fmt.Errorf("tenant and parcel identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, err
	}

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
		return none, false, err
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
			return none, false, err
		}
		hits = append(hits, row)
	}
	if err := rows.Err(); err != nil {
		return none, false, err
	}
	if len(hits) == 0 {
		return none, false, nil
	}
	if len(hits) > 1 {
		return none, false, domain.ErrAmbiguousParcelTarget
	}

	var document requestDocument
	if err := json.Unmarshal(hits[0].raw, &document); err != nil {
		return none, false, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(hits[0].revision, domain.ShipmentRequestState(hits[0].state))
	if err != nil {
		return none, false, err
	}
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		return none, false, err
	}
	return request, true, nil
}
