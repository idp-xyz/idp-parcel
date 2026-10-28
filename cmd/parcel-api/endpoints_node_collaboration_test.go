package main

import (
	"context"

	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

func init() {
	registerUnwired(nodeCollaborationWiring{handler: unwiredNodeCollaboration{}})
}

// unwiredNodeCollaboration 是两口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredNodeCollaboration struct{}

func (unwiredNodeCollaboration) Accept(context.Context, nodeopsapp.AcceptCollaborationCommand) (nodeopsapp.CollaborationResult, error) {
	return nodeopsapp.CollaborationResult{}, errOrchestrationNotWired
}

func (unwiredNodeCollaboration) RecordExecution(context.Context, nodeopsapp.RecordExecutionCommand) (nodeopsapp.CollaborationResult, error) {
	return nodeopsapp.CollaborationResult{}, errOrchestrationNotWired
}
