package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type nodeCollaborationWiring struct {
	handler   nodeopshttp.CollaborationHandler
	execution *nodeopshttp.WorkFactCollaborationIntake
}

// executionIntake 在 nil 时交回字面量未配置：不把一个带类型的空指针塞进接口——那样的 Intake 不等于 nil，第一次请求才会崩。
func (wiring nodeCollaborationWiring) executionIntake() nodeopshttp.CollaborationExecutionIntake {
	if wiring.execution == nil {
		return nodeopshttp.UnconfiguredIntake{}
	}
	return wiring.execution
}

func init() {
	registerEndpoints(endpointFeature[nodeCollaborationWiring]{
		lines: map[string]endpointLine{
			"/node-operations/collaboration-acceptances": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionNodeCollaborationAcceptance)},
			"/node-operations/collaboration-executions":  {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode), ownIntake: true},
		},
		build: func(sources endpointSources) (nodeCollaborationWiring, error) {
			// 承接与执行事实共用一只编排；谁持有「节点监管协作承接」授予、哪些设备被登记在哪个节点都是租户取值，生产装配不配任何默认。
			handler, err := buildNodeCollaboration(sources.db)
			if err != nil {
				return nodeCollaborationWiring{}, err
			}
			execution, err := buildNodeCollaborationExecutionIntake(sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry, sources.admission)
			return nodeCollaborationWiring{handler: handler, execution: execution}, err
		},
		mount: func(channels endpointChannels, wiring nodeCollaborationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 节点监管协作承接口（UC-NO-001，ADR-0333、ADR-0338）：节点作业责任角色对关务协作事项形成承接决定（已承接、部分承接、待补充、
				// 拒绝），挂操作者渠道「运营决定」能力面的节点监管协作承接一种；待补充之后补齐的决定是下一轮，回指它。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/node-operations/collaboration-acceptances", Handler: nodeopshttp.NewAcceptCollaborationEndpoint(channels.operatorDecisions.nodeOperations, wiring.handler)},
				// 协作执行事实登记口（UC-NO-001，ADR-0338 决定二）：节点现场登记已做了什么，挂「作业事实登记」能力面、判这一种事实的准入
				// 范围；拒接或待补充的事项、越出承接范围与别的节点的设备登记都不落事实。
				{Pattern: "/node-operations/collaboration-executions", Handler: nodeopshttp.NewRecordCollaborationExecutionEndpoint(wiring.executionIntake(), wiring.handler)},
			}
		},
	})
}
