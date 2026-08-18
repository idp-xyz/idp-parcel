package domain

import "errors"

// ErrAmbiguousParcelTarget 表示同一个租户下有两份当前已接受委托都声明了这件包裹。
// 机制拒绝自动采认，调用方不得按时间或行序挑一份。
var ErrAmbiguousParcelTarget = errors.New("parcel shipment: ambiguous current accepted parcel target")

// CurrentAcceptedParcelTarget 是按包裹反查命中的**当前已接受**委托目标。
// 它不是聚合本身：重建门只开到已提交（ADR-0030），已接受行不能经仓储读回；
// 下游要的是采用/履约编排的三重指名（来源身份 + 委托号 + 当前提交版本）。
type CurrentAcceptedParcelTarget struct {
	identity          SourceIdentity
	shipmentRequestID ShipmentRequestID
	submissionVersion SubmissionVersionID
}

func NewCurrentAcceptedParcelTarget(
	identity SourceIdentity,
	shipmentRequestID ShipmentRequestID,
	submissionVersion SubmissionVersionID,
) (CurrentAcceptedParcelTarget, error) {
	if !identity.valid() || !shipmentRequestID.valid() || !submissionVersion.valid() {
		return CurrentAcceptedParcelTarget{}, ErrInvalidShipmentRequest
	}
	return CurrentAcceptedParcelTarget{
		identity:          identity,
		shipmentRequestID: shipmentRequestID,
		submissionVersion: submissionVersion,
	}, nil
}

func (target CurrentAcceptedParcelTarget) Identity() SourceIdentity {
	return target.identity
}

func (target CurrentAcceptedParcelTarget) ShipmentRequestID() ShipmentRequestID {
	return target.shipmentRequestID
}

func (target CurrentAcceptedParcelTarget) SubmissionVersion() SubmissionVersionID {
	return target.submissionVersion
}
