package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// CustomerSupplementGap 是信封上的一条缺口：哪一判组、哪一件包裹（委托级为空）、什么原因。
type CustomerSupplementGap struct {
	Group  string
	Parcel string
	Reason string
}

// CustomerSupplementHandoffIntent 是「委托待受控补充」交给下游的引用。发出方不看来源。
type CustomerSupplementHandoffIntent struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Gaps              []CustomerSupplementGap
}

// CustomerSupplementHandoff 在等待态落库的同一事务里把信封入队。信封标识取提交版本。
type CustomerSupplementHandoff interface {
	HandOffCustomerSupplement(ctx context.Context, intent CustomerSupplementHandoffIntent) error
}
