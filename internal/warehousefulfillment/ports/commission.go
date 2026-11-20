package ports

import "context"

// 出库委托提交的答复词。未配置是代办授予没登记。已有结果是同一来源请求键的重放。
const (
	CommissionSubmittedWord       = "SUBMITTED"
	CommissionExistingWord        = "EXISTING_RESULT"
	CommissionNotConfiguredWord   = "NOT_CONFIGURED"
	CommissionUnresolvedWord      = "OWNERSHIP_UNRESOLVED"
	CommissionPausedWord          = "ADMISSION_PAUSED"
	CommissionOtherAuthorityWord  = "OTHER_PRODUCTION_AUTHORITY"
	CommissionInputRejectedWord   = "INPUT_NOT_ACCEPTED"
	CommissionIngressConflictWord = "INGRESS_CONFLICT"
)

// CommissionParcel 是要提交的一个包裹。来源请求键由出库请求与包裹序号组成，不在这里另带。
type CommissionParcel struct {
	Sequence   int
	Weight     string
	WeightUnit string
	Length     string
	Width      string
	Height     string
	Dimension  string
	Goods      string
	Unit       string
	Quantity   int64
}

// CommissionSubmission 是一份以货主名义提交的出库委托。一个包裹一份。
type CommissionSubmission struct {
	Tenant    string
	Owner     string
	Request   string
	Sequence  int
	Service   string
	Recipient string
	Address   string
	Parcel    CommissionParcel
}

// CommissionAnswer 是小包托运对这一来源请求的答复。ShipmentRequestID 只在已提交或已有结果时在场。
type CommissionAnswer struct {
	Outcome           string
	ShipmentRequestID string
	DeclaredParcelID  string
}

// CommissionRevision 是待货主补正之后，以货主名义形成的同一委托新提交版本。
type CommissionRevision struct {
	CommissionSubmission
	RequestKey       string
	BasisVersion     string
	ShipmentRequest  string
	DeclaredParcelID string
}

// OutboundCommissioner 以货主名义提交来源为仓配出库的委托。
type OutboundCommissioner interface {
	Submit(ctx context.Context, submission CommissionSubmission) (CommissionAnswer, error)
}
