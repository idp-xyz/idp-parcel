package registrationjson

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// preAdviceOnBehalfDocument 是一笔入库预报代录提交的线格式。货主客户账户、决定标识与依据引用是这一口的业务内容
// （ADR-0384 决定四）。租户与登记责任方不在载荷里。线索与参考号可缺。
type preAdviceOnBehalfDocument struct {
	carriedIdentity
	OwnerAccount      *string                      `json:"ownerAccount"`
	DecisionID        *string                      `json:"decisionId"`
	EvidenceReference *string                      `json:"evidenceReference"`
	WarehouseCode     *string                      `json:"warehouseCode"`
	ArrivalMode       *string                      `json:"arrivalMode"`
	InboundMode       *string                      `json:"inboundMode"`
	ExpectedAt        *string                      `json:"expectedAt"`
	ArrivalClue       *string                      `json:"arrivalClue"`
	OwnerReference    *string                      `json:"ownerReference"`
	GoodsLines        []preAdviceGoodsLineDocument `json:"goodsLines"`
	Cartons           []preAdviceCartonDocument    `json:"cartons"`
}

type preAdviceGoodsLineDocument struct {
	GoodsCode   *string `json:"goodsCode"`
	Unit        *string `json:"unit"`
	Quantity    *int64  `json:"quantity"`
	BatchNumber *string `json:"batchNumber"`
	ExpiryDate  *string `json:"expiryDate"`
	CartonCount *int64  `json:"cartonCount"`
}

type preAdviceCartonDocument struct {
	CartonNumber *string                       `json:"cartonNumber"`
	Lines        []preAdviceCartonLineDocument `json:"lines"`
}

type preAdviceCartonLineDocument struct {
	GoodsCode *string `json:"goodsCode"`
	Quantity  *int64  `json:"quantity"`
}

// refusedOwner 只为拒而存在：告知更新与撤回的货主取自预报，载荷里再带即坏载荷（ADR-0384 决定四）。
// RawMessage 认得出键在场（含 null）；指针认不出 null。
type refusedOwner struct {
	OwnerAccount json.RawMessage `json:"ownerAccount"`
}

func (refused refusedOwner) refuseOwner() error {
	if len(refused.OwnerAccount) > 0 {
		return malformed("ownerAccount must not be carried in the payload; the owner is taken from the pre-advice")
	}
	return nil
}

// PreAdviceOnBehalfCommand 把一笔代录提交译成受理命令。不认识的到仓方式或入库方式是坏载荷，理由不回显取值。
// 数量是否为正、商品是否已登记留给受理用例，否则同一份内容原样重投会先于重放被拒。
func PreAdviceOnBehalfCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.AcceptPreAdviceCommand, error) {
	var none application.AcceptPreAdviceCommand
	document, err := decodeDocument[preAdviceOnBehalfDocument](reader, "inbound pre-advice recorded on behalf")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	decisionID, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	evidence, err := requiredText("evidenceReference", document.EvidenceReference, domain.NewEvidenceReference)
	if err != nil {
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
	submission, err := domain.NewOnBehalfPreAdviceSubmission(decisionID, evidence)
	if err != nil {
		return none, err
	}
	return application.AcceptPreAdviceCommand{
		Tenant: tenant, Owner: owner, Warehouse: warehouse, Arrival: arrival, Inbound: inbound,
		GoodsLines: goodsLines, Cartons: cartons, ExpectedAt: expectedAt, Clue: clue, Reference: reference,
		Submission: submission, Registrant: registrant,
	}, nil
}

type preAdviceNoticeDocument struct {
	carriedIdentity
	refusedOwner
	PreAdviceID       *string `json:"preAdviceId"`
	DecisionID        *string `json:"decisionId"`
	EvidenceReference *string `json:"evidenceReference"`
	ExpectedAt        *string `json:"expectedAt"`
	ArrivalClue       *string `json:"arrivalClue"`
	OwnerReference    *string `json:"ownerReference"`
}

// PreAdviceNoticeOnBehalfCommand 把一笔代录告知更新译成命令。没带的线索或参考号就是清空。带了货主客户账户即坏载荷。
func PreAdviceNoticeOnBehalfCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.UpdatePreAdviceNoticeCommand, error) {
	var none application.UpdatePreAdviceNoticeCommand
	document, err := decodeDocument[preAdviceNoticeDocument](reader, "inbound pre-advice notice recorded on behalf")
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
	decisionID, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	evidence, err := requiredText("evidenceReference", document.EvidenceReference, domain.NewEvidenceReference)
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
	submission, err := domain.NewOnBehalfPreAdviceSubmission(decisionID, evidence)
	if err != nil {
		return none, err
	}
	return application.UpdatePreAdviceNoticeCommand{
		Tenant: tenant, ID: id, ExpectedAt: expectedAt, Clue: clue, Reference: reference,
		Submission: submission, Registrant: registrant,
	}, nil
}

// refusedDecision 只为拒而存在：代录撤回的键是预报标识，不从载荷收决定标识（ADR-0384 决定二）。
type refusedDecision struct {
	DecisionID json.RawMessage `json:"decisionId"`
}

func (refused refusedDecision) refuseDecision() error {
	if len(refused.DecisionID) > 0 {
		return malformed("decisionId must not be carried in the payload; the withdrawal is keyed by the pre-advice")
	}
	return nil
}

type preAdviceWithdrawalDocument struct {
	carriedIdentity
	refusedOwner
	refusedDecision
	PreAdviceID       *string `json:"preAdviceId"`
	EvidenceReference *string `json:"evidenceReference"`
}

// PreAdviceWithdrawalOnBehalfCommand 把一笔代录撤回译成命令。键用预报标识构造，不从载荷收决定标识。依据引用必带。
func PreAdviceWithdrawalOnBehalfCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.WithdrawPreAdviceCommand, error) {
	var none application.WithdrawPreAdviceCommand
	document, err := decodeDocument[preAdviceWithdrawalDocument](reader, "inbound pre-advice withdrawal recorded on behalf")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	if err := document.refuseDecision(); err != nil {
		return none, err
	}
	id, err := requiredText("preAdviceId", document.PreAdviceID, domain.NewPreAdviceID)
	if err != nil {
		return none, err
	}
	evidence, err := requiredText("evidenceReference", document.EvidenceReference, domain.NewEvidenceReference)
	if err != nil {
		return none, err
	}
	key, err := domain.NewRequestKey(id.String())
	if err != nil {
		return none, err
	}
	submission, err := domain.NewOnBehalfPreAdviceSubmission(key, evidence)
	if err != nil {
		return none, err
	}
	return application.WithdrawPreAdviceCommand{
		Tenant: tenant, ID: id, Submission: submission, Registrant: registrant,
	}, nil
}

func preAdviceLines(
	inbound domain.InboundMode,
	goodsDocuments []preAdviceGoodsLineDocument,
	cartonDocuments []preAdviceCartonDocument,
) ([]domain.GoodsInboundLine, []domain.Carton, error) {
	switch inbound {
	case domain.InboundByGoods:
		if len(cartonDocuments) > 0 {
			return nil, nil, malformed("cartons is not part of a goods inbound pre-advice")
		}
		lines, err := preAdviceGoodsLines(goodsDocuments)
		return lines, nil, err
	case domain.InboundByCarton:
		if len(goodsDocuments) > 0 {
			return nil, nil, malformed("goodsLines is not part of a carton inbound pre-advice")
		}
		cartons, err := preAdviceCartons(cartonDocuments)
		return nil, cartons, err
	default:
		return nil, nil, malformed("inboundMode is not a known inbound mode")
	}
}

func preAdviceGoodsLines(documents []preAdviceGoodsLineDocument) ([]domain.GoodsInboundLine, error) {
	lines := make([]domain.GoodsInboundLine, 0, len(documents))
	for index, document := range documents {
		path := "goodsLines[" + strconv.Itoa(index) + "]"
		goods, err := requiredText(path+".goodsCode", document.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return nil, err
		}
		unit, err := requiredText(path+".unit", document.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return nil, err
		}
		if document.Quantity == nil {
			return nil, malformed("%s.quantity is required", path)
		}
		batch, err := optionalBatch(path+".batchNumber", document.BatchNumber)
		if err != nil {
			return nil, err
		}
		expiry, err := optionalExpiry(path+".expiryDate", document.ExpiryDate)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewGoodsInboundLine(goods, unit, *document.Quantity, batch, expiry, optionalCartonCount(document.CartonCount))
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func preAdviceCartons(documents []preAdviceCartonDocument) ([]domain.Carton, error) {
	cartons := make([]domain.Carton, 0, len(documents))
	for index, document := range documents {
		path := "cartons[" + strconv.Itoa(index) + "]"
		number, err := requiredText(path+".cartonNumber", document.CartonNumber, domain.NewCartonNumber)
		if err != nil {
			return nil, err
		}
		content := make([]domain.CartonContentLine, 0, len(document.Lines))
		for lineIndex, lineDocument := range document.Lines {
			linePath := path + ".lines[" + strconv.Itoa(lineIndex) + "]"
			goods, err := requiredText(linePath+".goodsCode", lineDocument.GoodsCode, domain.NewGoodsCode)
			if err != nil {
				return nil, err
			}
			if lineDocument.Quantity == nil {
				return nil, malformed("%s.quantity is required", linePath)
			}
			line, err := domain.NewCartonContentLine(goods, *lineDocument.Quantity)
			if err != nil {
				return nil, err
			}
			content = append(content, line)
		}
		carton, err := domain.NewCarton(number, content)
		if err != nil {
			return nil, err
		}
		cartons = append(cartons, carton)
	}
	return cartons, nil
}

func requiredClosedWord[Value any](path, kind string, raw *string, construct func(string) (Value, error)) (Value, error) {
	var none Value
	if raw == nil {
		return none, malformed("%s is required", path)
	}
	value, err := construct(*raw)
	if err != nil {
		if errors.Is(err, domain.ErrUnstorableValue) {
			return none, malformed("%s carries a character that cannot be stored (NUL or invalid UTF-8)", path)
		}
		return none, malformed("%s is not a known %s", path, kind)
	}
	return value, nil
}

func requiredInstant(path string, raw *string) (time.Time, error) {
	if raw == nil {
		return time.Time{}, malformed("%s is required", path)
	}
	at, err := time.Parse(time.RFC3339Nano, *raw)
	if err != nil {
		return time.Time{}, malformed("%s must be an RFC3339 timestamp", path)
	}
	return at, nil
}

func optionalClue(path string, raw *string) (domain.ArrivalClue, error) {
	if raw == nil {
		return domain.ArrivalClue{}, nil
	}
	return textCell(path, *raw, domain.NewArrivalClue)
}

func optionalReference(path string, raw *string) (domain.OwnerReference, error) {
	if raw == nil {
		return domain.OwnerReference{}, nil
	}
	return textCell(path, *raw, domain.NewOwnerReference)
}

func optionalBatch(path string, raw *string) (domain.BatchNumber, error) {
	if raw == nil {
		return domain.BatchNumber{}, nil
	}
	return textCell(path, *raw, domain.NewBatchNumber)
}

func optionalExpiry(path string, raw *string) (domain.ExpiryDate, error) {
	var none domain.ExpiryDate
	if raw == nil {
		return none, nil
	}
	date, err := domain.NewExpiryDate(*raw)
	switch {
	case err == nil:
		return date, nil
	case errors.Is(err, domain.ErrBlankValue):
		return none, malformed("%s must not be blank", path)
	case errors.Is(err, domain.ErrUnstorableValue):
		return none, malformed("%s carries a character that cannot be stored (NUL or invalid UTF-8)", path)
	default:
		return none, malformed("%s must be a calendar date", path)
	}
}

func optionalCartonCount(raw *int64) domain.CartonCount {
	if raw == nil {
		return domain.CartonCount{}
	}
	return domain.GivenCartonCount(*raw)
}
