package main

import (
	"context"

	nodeopsdomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	nodeopsports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ListPhysicalControlTransitions 让未接线的节点作业查阅读口也答节点控制转移册那一问（ADR-0214 决定三），与它别的几问同一个答法。
func (unwiredNodeOperationsRecords) ListPhysicalControlTransitions(
	context.Context,
	nodeopsdomain.TenantID,
	int,
) ([]nodeopsports.PhysicalControlTransitionCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}
