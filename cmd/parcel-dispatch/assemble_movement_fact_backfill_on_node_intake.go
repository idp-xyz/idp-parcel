package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	venodeops "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// backfillMovementFactsOnNodeIntakeConsumer 接 NO 节点收寄形成 → VE 移动事实补派生（ADR-0299）：按信封键重读收寄，认出了包裹就向
// 节点作业问这件实物曾被装进哪几件单元，在每一件候选单元有参与的段上按段列出在册移动事实，凡事实那一刻这件包裹正是单元成员的就补
// 派生——收寄册、成员关系册、段册与移动事实册都只读，权威事实留在各自的上下文。展开成员与正常路径是同一个装配件
// （projectionConsolidationMembers），派生借投影那一份租户现绑的编排，与移动事实那一路同一个写入方。
func backfillMovementFactsOnNodeIntakeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill on node intake receptions: %w", err)
	}
	holders, err := nopostgres.NewConsolidationUnitsHoldingMember(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill on node intake unit holders: %w", err)
	}
	identified, err := venodeops.NewIdentifiedMemberView(receptions, holders)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill on node intake identified member view: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill on node intake segments: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill on node intake movement facts: %w", err)
	}
	members, err := projectionConsolidationMembers(db)
	if err != nil {
		return nil, err
	}
	processing, err := vetf.NewBackfillIdentifiedMemberAdapter(identified, segments, facts, members, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill identified member: %w", err)
	}
	consumer, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts on node intake consumer: %w", err)
	}
	return consumer, nil
}
