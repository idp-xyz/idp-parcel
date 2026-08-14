package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedHandover 是交接重建入口因快照数据本身而拒绝时给出的理由。与
// ErrInvalidTransportHandover 分格的道理同交付侧：后者说「此刻要形成的这份不合规则」，
// 前者说「这份已经登记过的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的
// 适配器，不是改调用方的入参。
var ErrInvalidRehydratedHandover = errors.New("transport fulfillment: invalid rehydrated transport handover")

// RehydrateTransportHandoverSpec 是一份交接判断在库里的样子。版本链两字段独立收下，
// 因为 Correct 把它们写在未导出字段上——没有这个入口，一份更正版本读回来会退化成首登，
// 「新版回指前身」的链在重启后就断了。
type RehydrateTransportHandoverSpec struct {
	TenantID          TenantID
	Object            CarriedObjectReference
	Scope             HandoverScopeReference
	ReleasedBy        HandoverPartyReference
	ReceivedBy        HandoverPartyReference
	Verdict           HandoverVerdict
	ReleasingEvidence HandoverEvidenceReference
	ReceivingEvidence HandoverEvidenceReference
	Rule              HandoverRuleReference
	Basis             HandoverBasisReference
	Version           HandoverResultVersion
	JudgedAt          time.Time
	Corrects          HandoverResultVersion
	CorrectedAt       time.Time
}

// RehydrateTransportHandover 从库里读到的产物重建一份交接判断。
//
// 逐格完备性与 FormTransportHandover 同一套：`已交接`要双方证据加适用规则且不带依据，
// 拒收与待确认必须带依据。版本链要么整体缺席（首登），要么回指前版且不自指、更正时间
// 不早于裁决时间——与 Correct 立的三道门一一对应。
func RehydrateTransportHandover(spec RehydrateTransportHandoverSpec) (TransportHandover, error) {
	if !spec.TenantID.valid() || !spec.Object.valid() || !spec.Scope.valid() ||
		!spec.ReleasedBy.valid() || !spec.ReceivedBy.valid() ||
		!spec.Verdict.valid() || !spec.Version.valid() || spec.JudgedAt.IsZero() {
		return TransportHandover{}, rehydratedHandoverRefusal("交接身份、双方、裁决、版本或业务时间缺失")
	}
	if spec.Verdict == ObjectHandedOver {
		if !spec.ReleasingEvidence.valid() || !spec.ReceivingEvidence.valid() || !spec.Rule.valid() {
			return TransportHandover{}, rehydratedHandoverRefusal("已交接缺双方证据或适用规则")
		}
		if spec.Basis.valid() {
			return TransportHandover{}, rehydratedHandoverRefusal("已交接携带拒收/待确认依据")
		}
	} else if !spec.Basis.valid() {
		return TransportHandover{}, rehydratedHandoverRefusal("拒收或待确认缺依据——没有原因的拒收与数据丢失无从分辨")
	}
	if spec.Corrects.valid() != !spec.CorrectedAt.IsZero() {
		return TransportHandover{}, rehydratedHandoverRefusal("版本链半截——前版引用与更正时间必须同缺席或同在场")
	}
	if spec.Corrects.valid() {
		if spec.Corrects == spec.Version {
			return TransportHandover{}, rehydratedHandoverRefusal("前版引用指向版本自己")
		}
		if spec.CorrectedAt.Before(spec.JudgedAt) {
			return TransportHandover{}, rehydratedHandoverRefusal("更正时间早于裁决")
		}
	}
	return TransportHandover{
		tenantID:          spec.TenantID,
		object:            spec.Object,
		scope:             spec.Scope,
		releasedBy:        spec.ReleasedBy,
		receivedBy:        spec.ReceivedBy,
		verdict:           spec.Verdict,
		releasingEvidence: spec.ReleasingEvidence,
		receivingEvidence: spec.ReceivingEvidence,
		rule:              spec.Rule,
		basis:             spec.Basis,
		version:           spec.Version,
		judgedAt:          spec.JudgedAt.UTC(),
		corrects:          spec.Corrects,
		correctedAt:       spec.CorrectedAt.UTC(),
	}, nil
}

func rehydratedHandoverRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedHandover, reason)
}
