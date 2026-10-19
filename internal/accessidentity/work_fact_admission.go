package accessidentity

import (
	"errors"
	"slices"
)

// ErrWorkFactKindUnknown 表示给出的作业事实类型不在铸造侧判准入的那几种里。
var ErrWorkFactKindUnknown = errors.New("access identity: work fact kind is unknown")

// WorkFactKind 是「作业事实登记」能力面在治理登记册上的事实类型（ADR-0180 决定四末项「一个能力词，事实类型逐口取」）。能力词就是
// 能力面 CapabilityWorkFactRegistration 那一个串，与运营决定口取 CapabilityOperationDecision 同一个取法。
//
// 词表只收在铸造侧判准入的作业事实：送站交接单的开立、归入、关闭（ADR-0212 决定三；票 ps-external-mark-relations/12）。节点收寄与
// 运输履约各口今天不在铸造侧判准入，它们的词随把它们接上准入的那一票加进来——先登进对照却没有一口会问，等于让阶段治理去登一个
// 不起作用的区间。
type WorkFactKind string

const (
	WorkFactDeliveryHandoverNoteOpening    WorkFactKind = "DELIVERY_HANDOVER_NOTE_OPENING"
	WorkFactDeliveryHandoverNoteAssignment WorkFactKind = "DELIVERY_HANDOVER_NOTE_ASSIGNMENT"
	WorkFactDeliveryHandoverNoteClosure    WorkFactKind = "DELIVERY_HANDOVER_NOTE_CLOSURE"
)

// workFactKindsAdmittedAtMint 是准入范围对照收哪几个词、认证方问哪一对准入要求的唯一来源：两处各抄一份，改了一份，登在旧词下的
// 区间就对另一边无声失效（同 customerCapabilitiesAdmittedAtMint 的理由）。
var workFactKindsAdmittedAtMint = []WorkFactKind{
	WorkFactDeliveryHandoverNoteOpening,
	WorkFactDeliveryHandoverNoteAssignment,
	WorkFactDeliveryHandoverNoteClosure,
}

// ParseWorkFactKind 按字面取值认作业事实类型，大小写不折叠：对照册里存的就是这些字面量。
func ParseWorkFactKind(value string) (WorkFactKind, error) {
	if kind := WorkFactKind(value); slices.Contains(workFactKindsAdmittedAtMint, kind) {
		return kind, nil
	}
	return "", ErrWorkFactKindUnknown
}

func (kind WorkFactKind) String() string { return string(kind) }

// WorkFactAdmission 交回这一种作业事实在铸造侧判的准入要求；词表外的交回 nil。
func WorkFactAdmission(kind WorkFactKind) *AdmissionRequirement {
	if !slices.Contains(workFactKindsAdmittedAtMint, kind) {
		return nil
	}
	return &AdmissionRequirement{Capability: CapabilityWorkFactRegistration.String(), FactKind: kind.String()}
}
