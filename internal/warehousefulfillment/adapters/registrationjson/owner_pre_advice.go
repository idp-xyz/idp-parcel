package registrationjson

import (
	"io"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件是入库预报货主口的线格式。租户、货主客户账户与登记责任方只取自信封，载荷里再带即坏载荷。
// 接入请求标识原样交回：去空白、拒保留前缀与加前缀都在首方族的推导口，不在这里做第二遍。

// OwnerPreAdviceSubmissionFields 是货主提交口解出来的业务内容，还没有来源请求键。
type OwnerPreAdviceSubmissionFields struct {
	AccessRequestID string
	Warehouse       domain.WarehouseCode
	Arrival         domain.ArrivalMode
	Inbound         domain.InboundMode
	GoodsLines      []domain.GoodsInboundLine
	Cartons         []domain.Carton
	ExpectedAt      time.Time
	Clue            domain.ArrivalClue
	Reference       domain.OwnerReference
}

type ownerPreAdviceSubmissionDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string                      `json:"accessRequestId"`
	WarehouseCode   *string                      `json:"warehouseCode"`
	ArrivalMode     *string                      `json:"arrivalMode"`
	InboundMode     *string                      `json:"inboundMode"`
	ExpectedAt      *string                      `json:"expectedAt"`
	ArrivalClue     *string                      `json:"arrivalClue"`
	OwnerReference  *string                      `json:"ownerReference"`
	GoodsLines      []preAdviceGoodsLineDocument `json:"goodsLines"`
	Cartons         []preAdviceCartonDocument    `json:"cartons"`
}

// ReadOwnerPreAdviceSubmission 把货主提交译成字段。不认识的到仓方式或入库方式是坏载荷。数量是否为正留给受理用例，
// 否则同一份内容原样重投会先于重放被拒。
func ReadOwnerPreAdviceSubmission(reader io.Reader) (OwnerPreAdviceSubmissionFields, error) {
	var none OwnerPreAdviceSubmissionFields
	document, err := decodeDocument[ownerPreAdviceSubmissionDocument](reader, "inbound pre-advice")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouseCode", document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	arrival, err := requiredClosedWord("arrivalMode", "arrival mode", document.ArrivalMode, domain.NewArrivalMode)
	if err != nil {
		return none, err
	}
	inbound, err := requiredClosedWord("inboundMode", "inbound mode", document.InboundMode, domain.NewInboundMode)
	if err != nil {
		return none, err
	}
	expectedAt, err := requiredInstant("expectedAt", document.ExpectedAt)
	if err != nil {
		return none, err
	}
	clue, err := optionalClue("arrivalClue", document.ArrivalClue)
	if err != nil {
		return none, err
	}
	reference, err := optionalReference("ownerReference", document.OwnerReference)
	if err != nil {
		return none, err
	}
	goodsLines, cartons, err := preAdviceLines(inbound, document.GoodsLines, document.Cartons)
	if err != nil {
		return none, err
	}
	return OwnerPreAdviceSubmissionFields{
		AccessRequestID: rawAccessRequestID(document.AccessRequestID),
		Warehouse:       warehouse, Arrival: arrival, Inbound: inbound,
		GoodsLines: goodsLines, Cartons: cartons, ExpectedAt: expectedAt, Clue: clue, Reference: reference,
	}, nil
}

// OwnerPreAdviceNoticeFields 是货主告知更新口解出来的一组告知。没带的线索或参考号就是清空，由零值表达。
type OwnerPreAdviceNoticeFields struct {
	AccessRequestID string
	ID              domain.PreAdviceID
	ExpectedAt      time.Time
	Clue            domain.ArrivalClue
	Reference       domain.OwnerReference
}

type ownerPreAdviceNoticeDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	PreAdviceID     *string `json:"preAdviceId"`
	ExpectedAt      *string `json:"expectedAt"`
	ArrivalClue     *string `json:"arrivalClue"`
	OwnerReference  *string `json:"ownerReference"`
}

// ReadOwnerPreAdviceNotice 把货主告知更新译成字段。带了明细、到仓方式或货主客户账户都是坏载荷：前两样是不认识的键，货主是点名拒的键。
func ReadOwnerPreAdviceNotice(reader io.Reader) (OwnerPreAdviceNoticeFields, error) {
	var none OwnerPreAdviceNoticeFields
	document, err := decodeDocument[ownerPreAdviceNoticeDocument](reader, "inbound pre-advice arrival notice")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	id, err := requiredText("preAdviceId", document.PreAdviceID, domain.NewPreAdviceID)
	if err != nil {
		return none, err
	}
	expectedAt, err := requiredInstant("expectedAt", document.ExpectedAt)
	if err != nil {
		return none, err
	}
	clue, err := optionalClue("arrivalClue", document.ArrivalClue)
	if err != nil {
		return none, err
	}
	reference, err := optionalReference("ownerReference", document.OwnerReference)
	if err != nil {
		return none, err
	}
	return OwnerPreAdviceNoticeFields{
		AccessRequestID: rawAccessRequestID(document.AccessRequestID),
		ID:              id, ExpectedAt: expectedAt, Clue: clue, Reference: reference,
	}, nil
}

// OwnerPreAdviceWithdrawalFields 是货主撤回口解出来的指名。键是来源请求键，不在这份字段里。
type OwnerPreAdviceWithdrawalFields struct {
	AccessRequestID string
	ID              domain.PreAdviceID
}

type ownerPreAdviceWithdrawalDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	PreAdviceID     *string `json:"preAdviceId"`
}

// ReadOwnerPreAdviceWithdrawal 把货主撤回译成字段。
func ReadOwnerPreAdviceWithdrawal(reader io.Reader) (OwnerPreAdviceWithdrawalFields, error) {
	var none OwnerPreAdviceWithdrawalFields
	document, err := decodeDocument[ownerPreAdviceWithdrawalDocument](reader, "inbound pre-advice withdrawal")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	id, err := requiredText("preAdviceId", document.PreAdviceID, domain.NewPreAdviceID)
	if err != nil {
		return none, err
	}
	return OwnerPreAdviceWithdrawalFields{AccessRequestID: rawAccessRequestID(document.AccessRequestID), ID: id}, nil
}

func rawAccessRequestID(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}
