package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// stockImportDocument 是一份建账导入的线格式。租户与提交操作者取自信封，载荷里再带即坏载荷。
// 没带原因、一行都没有，是坏载荷。数量不为正与箱单为空留给应用层答内容不合。
type stockImportDocument struct {
	carriedIdentity
	ImportID      *string                  `json:"importId"`
	OwnerAccount  *string                  `json:"ownerAccount"`
	WarehouseCode *string                  `json:"warehouseCode"`
	Reason        *string                  `json:"reason"`
	GoodsLines    *[]stockImportGoodsLine  `json:"goodsLines"`
	CartonLines   *[]stockImportCartonLine `json:"cartonLines"`
}

type stockImportGoodsLine struct {
	LocationCode      *string `json:"locationCode"`
	GoodsCode         *string `json:"goodsCode"`
	Unit              *string `json:"unit"`
	StockState        *string `json:"stockState"`
	Quantity          *int64  `json:"quantity"`
	BatchNumber       *string `json:"batchNumber"`
	ExpiryDate        *string `json:"expiryDate"`
	LabelAlias        *string `json:"labelAlias"`
	DeclaredInboundAt *string `json:"declaredInboundAt"`
}

type stockImportCartonLine struct {
	LocationCode      *string                     `json:"locationCode"`
	CartonNumber      *string                     `json:"cartonNumber"`
	Contents          *[]stockImportCartonContent `json:"contents"`
	DeclaredInboundAt *string                     `json:"declaredInboundAt"`
}

type stockImportCartonContent struct {
	GoodsCode *string `json:"goodsCode"`
	Quantity  *int64  `json:"quantity"`
}

// StockImportCommand 把一份建账导入的 JSON 译成提交命令。
func StockImportCommand(reader io.Reader, tenant domain.TenantID, submitter domain.Registrant) (application.SubmitStockImportCommand, error) {
	var none application.SubmitStockImportCommand
	document, err := decodeDocument[stockImportDocument](reader, "stock import")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("importId", document.ImportID, domain.NewStockImportID)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouseCode", document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	reason, err := requiredText("reason", document.Reason, domain.NewStockImportReason)
	if err != nil {
		return none, err
	}
	goods, err := stockImportGoods(document.GoodsLines)
	if err != nil {
		return none, err
	}
	cartons, err := stockImportCartons(document.CartonLines)
	if err != nil {
		return none, err
	}
	if len(goods) == 0 && len(cartons) == 0 {
		return none, malformed("at least one line is required")
	}
	return application.SubmitStockImportCommand{
		Tenant: tenant, ID: id, Owner: owner, Warehouse: warehouse, Reason: reason,
		Goods: goods, Cartons: cartons, Submitter: submitter,
	}, nil
}

func stockImportGoods(raw *[]stockImportGoodsLine) ([]domain.StockImportGoodsLine, error) {
	if raw == nil {
		return nil, nil
	}
	lines := make([]domain.StockImportGoodsLine, 0, len(*raw))
	for index, document := range *raw {
		path := "goodsLines"
		location, err := requiredText(path+".locationCode", document.LocationCode, domain.NewLocationCode)
		if err != nil {
			return nil, err
		}
		goods, err := requiredText(path+".goodsCode", document.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return nil, err
		}
		unit, err := requiredText(path+".unit", document.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return nil, err
		}
		if document.StockState == nil {
			return nil, malformed("%s.stockState is required", path)
		}
		state, err := domain.NewStockState(*document.StockState)
		if err != nil {
			return nil, malformed("%s.stockState is not a stock state", path)
		}
		if document.Quantity == nil {
			return nil, malformed("%s.quantity is required", path)
		}
		var batch domain.BatchNumber
		if document.BatchNumber != nil {
			batch, err = domain.NewBatchNumber(*document.BatchNumber)
			if err != nil {
				return nil, malformed("%s.batchNumber is blank", path)
			}
		}
		var expiry domain.ExpiryDate
		if document.ExpiryDate != nil {
			expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
			if err != nil {
				return nil, malformed("%s.expiryDate is not a calendar day", path)
			}
		}
		var alias domain.GoodsAliasCode
		if document.LabelAlias != nil {
			alias, err = domain.NewGoodsAliasCode(*document.LabelAlias)
			if err != nil {
				return nil, malformed("%s.labelAlias is blank", path)
			}
		}
		declared, err := optionalInstant(path+".declaredInboundAt", document.DeclaredInboundAt)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewStockImportGoodsLine(location, goods, unit, state, *document.Quantity, batch, expiry, alias, declared)
		if err != nil {
			return nil, malformed("%s is not a goods line", path)
		}
		_ = index
		lines = append(lines, line)
	}
	return lines, nil
}

func stockImportCartons(raw *[]stockImportCartonLine) ([]domain.StockImportCartonLine, error) {
	if raw == nil {
		return nil, nil
	}
	lines := make([]domain.StockImportCartonLine, 0, len(*raw))
	for _, document := range *raw {
		path := "cartonLines"
		location, err := requiredText(path+".locationCode", document.LocationCode, domain.NewLocationCode)
		if err != nil {
			return nil, err
		}
		number, err := requiredText(path+".cartonNumber", document.CartonNumber, domain.NewCartonNumber)
		if err != nil {
			return nil, err
		}
		var contents []domain.StockImportCartonContent
		if document.Contents != nil {
			for _, item := range *document.Contents {
				goods, err := requiredText(path+".contents.goodsCode", item.GoodsCode, domain.NewGoodsCode)
				if err != nil {
					return nil, err
				}
				if item.Quantity == nil {
					return nil, malformed("%s.contents.quantity is required", path)
				}
				content, err := domain.NewStockImportCartonContent(goods, *item.Quantity)
				if err != nil {
					return nil, err
				}
				contents = append(contents, content)
			}
		}
		declared, err := optionalInstant(path+".declaredInboundAt", document.DeclaredInboundAt)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewStockImportCartonLine(location, number, contents, declared)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

type stockImportDecisionDocument struct {
	carriedIdentity
	refusedOwner
	ImportID *string `json:"importId"`
	Note     *string `json:"note"`
	Reason   *string `json:"reason"`
}

// ApproveStockImportCommand 把批准一份建账导入译成命令。货主与内容取自册上那一份，载荷里再带货主即坏载荷。
func ApproveStockImportCommand(reader io.Reader) (application.PostStockImportCommand, error) {
	id, _, err := readStockImportDecision(reader, false)
	if err != nil {
		return application.PostStockImportCommand{}, err
	}
	return application.PostStockImportCommand{ID: id}, nil
}

// RejectStockImportCommand 把驳回一份建账导入译成命令。原因必带。
func RejectStockImportCommand(reader io.Reader) (application.RejectStockImportCommand, error) {
	id, reason, err := readStockImportDecision(reader, true)
	if err != nil {
		return application.RejectStockImportCommand{}, err
	}
	return application.RejectStockImportCommand{ID: id, Note: reason}, nil
}

func readStockImportDecision(reader io.Reader, reject bool) (domain.StockImportID, string, error) {
	var none domain.StockImportID
	document, err := decodeDocument[stockImportDecisionDocument](reader, "stock import decision")
	if err != nil {
		return none, "", err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, "", err
	}
	if err := document.refuseOwner(); err != nil {
		return none, "", err
	}
	id, err := requiredText("importId", document.ImportID, domain.NewStockImportID)
	if err != nil {
		return none, "", err
	}
	reason := ""
	if document.Reason != nil {
		reason = *document.Reason
	}
	if reject && reason == "" {
		return none, "", malformed("reason is required")
	}
	return id, reason, nil
}
