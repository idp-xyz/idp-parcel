package nodeopshttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是节点作业查阅页的节点控制转移册（registry=physical-control，ADR-0214 决定三）：权威运输交接推进节点控制时留下的各行，转出、
// 转入与入账不推进同册列出。撤回控制的更正入账「控制来源链待重算」、转入对不上作业实物，人要在这里看得见。

const (
	registryPhysicalControl                 = "physical-control"
	outcomePhysicalControlTransitionsListed = "PHYSICAL_CONTROL_TRANSITIONS_LISTED"
)

func servePhysicalControlTransitions(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListPhysicalControlTransitions(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]physicalControlTransitionBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, physicalControlTransitionBody{
			HandoverObject:          row.HandoverObject,
			HandoverScope:           row.HandoverScope,
			HandoverVersion:         row.HandoverVersion,
			Direction:               row.Direction,
			Outcome:                 row.Outcome,
			Reason:                  row.Reason,
			Unit:                    row.Unit,
			Node:                    row.Node,
			ControlKind:             row.ControlKind,
			ControlBasis:            row.ControlBasis,
			ControlEstablishedAt:    optionalInstant(row.ControlEstablishedAt),
			TransferReference:       row.TransferReference,
			TransferredAt:           optionalInstant(row.TransferredAt),
			MismatchedReleasingNode: row.MismatchedReleasingNode,
			OccurredAt:              rfc3339(row.OccurredAt),
			RecordedAt:              rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, physicalControlTransitionListResponse{
		Outcome:     outcomePhysicalControlTransitionsListed,
		Transitions: bodies,
	})
}

type physicalControlTransitionListResponse struct {
	Outcome     string                          `json:"outcome"`
	Transitions []physicalControlTransitionBody `json:"transitions"`
}

// physicalControlTransitionBody 逐字段透出一行控制转移。
//
// handover 三件是 transport-fulfillment 的交接键，原样透出：人要拿它去交接查阅册对那一版。direction（OUT / IN）、outcome（ADVANCED /
// NOT_ADVANCED）、reason 与 controlKind 是封闭词原样透出，不折成布尔或中文——「没推进」的几格各有各的去查处。reason 只在没推进时
// 出现；unit 只在对上了一件作业实物时出现；控制四件只在这一行带控制时出现；transfer 两件只在推进了的转出与转出一侧的待重算上出现；
// mismatchedReleasingNode 只在推进了的转出、且交接声明的交出节点不是被转出那一份控制的节点时出现，值是声明的那一个。缺席用整键不
// 出现表达，不用空串或 null 兼表，惯例同收寄册的 controlReleased* 两件。occurredAt 是交接的业务时间，recordedAt 是本上下文的记录时刻。
type physicalControlTransitionBody struct {
	HandoverObject          string `json:"handoverObject"`
	HandoverScope           string `json:"handoverScope"`
	HandoverVersion         string `json:"handoverVersion"`
	Direction               string `json:"direction"`
	Outcome                 string `json:"outcome"`
	Reason                  string `json:"reason,omitempty"`
	Unit                    string `json:"unit,omitempty"`
	Node                    string `json:"node,omitempty"`
	ControlKind             string `json:"controlKind,omitempty"`
	ControlBasis            string `json:"controlBasis,omitempty"`
	ControlEstablishedAt    string `json:"controlEstablishedAt,omitempty"`
	TransferReference       string `json:"transferReference,omitempty"`
	TransferredAt           string `json:"transferredAt,omitempty"`
	MismatchedReleasingNode string `json:"mismatchedReleasingNode,omitempty"`
	OccurredAt              string `json:"occurredAt"`
	RecordedAt              string `json:"recordedAt"`
}
