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

// CommissionParcel 是要提交的一个包裹。未标平台备货时来源请求键由出库请求与包裹序号组成；标了时由出库请求与提交轮次组成。
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

// CommissionSubmission 是一份以货主名义提交的出库委托。
// Round 为 0 时按 Sequence 各交一份。Round 从 1 起是平台备货的提交轮次，Parcels 是这一轮的全部包裹。
type CommissionSubmission struct {
	Tenant    string
	Owner     string
	Request   string
	Sequence  int
	Round     int
	Service   string
	Recipient string
	Address   string
	Parcel    CommissionParcel
	Parcels   []CommissionParcel
}

// CommissionMember 是一份委托里一个包裹对上的已签发成员。
type CommissionMember struct {
	Sequence         int
	DeclaredParcelID string
}

// CommissionAnswer 是小包托运对这一来源请求的答复。ShipmentRequestID 只在已提交或已有结果时在场。
type CommissionAnswer struct {
	Outcome           string
	ShipmentRequestID string
	DeclaredParcelID  string
	Members           []CommissionMember
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

// 预取委托打包后称重与估算不同时，声明测量资料修订的答案词。
const (
	MeasurementAdoptedWord        = "ADOPTED"
	MeasurementAwaitingReviewWord = "AWAITING_REVIEW"
	MeasurementDisallowedWord     = "DISALLOWED"
	MeasurementNotAcceptedWord    = "NOT_ACCEPTED"
	MeasurementNotConfiguredWord  = "NOT_CONFIGURED"
)

// MeasurementAmendment 是预取委托打包称重后，以货主名义提出的声明测量资料修订。
type MeasurementAmendment struct {
	Tenant           string
	Owner            string
	Request          string
	Sequence         int
	ShipmentRequest  string
	DeclaredParcelID string
	Weight           string
	WeightUnit       string
	Length           string
	Width            string
	Height           string
	Dimension        string
}

// MeasurementAmendmentAnswer 是小包托运对声明测量资料修订的答复。
type MeasurementAmendmentAnswer struct {
	Outcome string
}

// GoodsAmendment 是预取委托的包裹打包后件数与申报的不同时，以货主名义提出的货物明细资料修订。
// 预取时按分到的全量申报了件数；拣不齐照货主的规则少发、拆出了缺货部分，或一张单打成了几个包裹，
// 那一个包裹里实际装的就不是申报的那么多。修订交的是整组货物明细：这份委托只有这一个包裹、一种货。
type GoodsAmendment struct {
	Tenant           string
	Owner            string
	Request          string
	Sequence         int
	ShipmentRequest  string
	DeclaredParcelID string
	Goods            string
	Unit             string
	Quantity         int64
}

// GoodsAmender 以货主名义把打包后的实际件数交成已接受委托上的资料修订。答案词与声明测量修订是同一组。
type GoodsAmender interface {
	AmendGoods(ctx context.Context, amendment GoodsAmendment) (MeasurementAmendmentAnswer, error)
}

// MeasurementAmender 以货主名义把打包后的实际测量交成已接受委托上的资料修订。
type MeasurementAmender interface {
	AmendMeasurement(ctx context.Context, amendment MeasurementAmendment) (MeasurementAmendmentAnswer, error)
}
