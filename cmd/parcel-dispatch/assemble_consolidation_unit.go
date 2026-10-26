package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
)

// carriedObjectUnitView 装小包托运认「运输履约交来的载运对象是不是集运单元」的那一口：场外揽收采用、有效交付采用与面单终局判断
// （经首次有效收寄）三路各装一份，问的都是节点作业的集运单元册，不另立串形或前缀规则。读面只读，各路自己的实例之间不共享状态。
func carriedObjectUnitView(db *bentopg.DB) (psnodeops.ConsolidationUnitView, error) {
	memberships, err := nopostgres.NewConsolidationMembershipsAt(db)
	if err != nil {
		return psnodeops.ConsolidationUnitView{}, fmt.Errorf("parcel-dispatch: carried object consolidation memberships: %w", err)
	}
	units, err := psnodeops.NewConsolidationUnitView(memberships)
	if err != nil {
		return psnodeops.ConsolidationUnitView{}, fmt.Errorf("parcel-dispatch: carried object consolidation unit view: %w", err)
	}
	return units, nil
}
