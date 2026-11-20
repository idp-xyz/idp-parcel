package registrationjson

import (
	"encoding/json"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// warehouseMeasurementDocument 是仓测登记的线格式（UC-WF-002）。
// 租户、操作者与设备不在载荷里：前两格取自信封，设备取自作业事实请求头，键在场即坏载荷。
// 尺寸与重量数值沿用 JSON 字符串，避开二进制浮点；数值为零与目录外单位留给应用层答 NOT_ACCEPTED 并指出那一格。
type warehouseMeasurementDocument struct {
	carriedIdentity
	DeviceID       json.RawMessage          `json:"deviceId"`
	Operator       json.RawMessage          `json:"operator"`
	FactID         *string                  `json:"factId"`
	OccurredAt     *string                  `json:"occurredAt"`
	DeviceSequence *int64                   `json:"deviceSequence"`
	Warehouse      *string                  `json:"warehouse"`
	TargetKind     *string                  `json:"targetKind"`
	OwnerAccount   *string                  `json:"ownerAccount"`
	GoodsCode      *string                  `json:"goodsCode"`
	PackagingLevel *string                  `json:"packagingLevel"`
	CartonNumber   *string                  `json:"cartonNumber"`
	Dimensions     *goodsDimensionsDocument `json:"dimensions"`
	Weight         *goodsWeightDocument     `json:"weight"`
}

func (document warehouseMeasurementDocument) refuseCarriedActors() error {
	if err := document.refuseCarriedIdentity(); err != nil {
		return err
	}
	if len(document.DeviceID) > 0 {
		return malformed("deviceId must not be carried in the payload; the device comes from the work-fact headers, not from the request")
	}
	if len(document.Operator) > 0 {
		return malformed("operator must not be carried in the payload; the operator comes from the work-fact envelope, not from the request")
	}
	return nil
}

// WarehouseMeasurementCommand 把仓测登记载荷译成命令。租户、操作者与设备留空，由 Intake 在认证之后填上。
func WarehouseMeasurementCommand(reader io.Reader) (application.RecordWarehouseMeasurementCommand, error) {
	var none application.RecordWarehouseMeasurementCommand
	document, err := decodeDocument[warehouseMeasurementDocument](reader, "warehouse measurement")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedActors(); err != nil {
		return none, err
	}
	factID, err := requiredText("factId", document.FactID, domain.NewMeasurementFactID)
	if err != nil {
		return none, err
	}
	occurredAt, err := requiredInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	if document.DeviceSequence == nil {
		return none, malformed("deviceSequence is required")
	}
	if *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	targetKind, err := requiredClosedWord("targetKind", "measurement target kind", document.TargetKind, domain.NewMeasurementTargetKind)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	target, err := measurementTarget(targetKind, owner, document)
	if err != nil {
		return none, err
	}
	if document.Dimensions == nil {
		return none, malformed("dimensions is required")
	}
	dimensions, err := goodsDimensions("dimensions", document.Dimensions)
	if err != nil {
		return none, err
	}
	if document.Weight == nil {
		return none, malformed("weight is required")
	}
	weight, err := goodsWeight("weight", document.Weight)
	if err != nil {
		return none, err
	}
	return application.RecordWarehouseMeasurementCommand{
		FactID:         factID,
		OccurredAt:     occurredAt,
		DeviceSequence: *document.DeviceSequence,
		Warehouse:      warehouse,
		Target:         target,
		Dimensions:     dimensions,
		Weight:         weight,
	}, nil
}

func measurementTarget(
	kind domain.MeasurementTargetKind,
	owner domain.OwnerAccountID,
	document warehouseMeasurementDocument,
) (domain.MeasurementTarget, error) {
	switch kind {
	case domain.MeasurementTargetGoodsLevel:
		if document.CartonNumber != nil {
			return domain.MeasurementTarget{}, malformed("cartonNumber must not be given when targetKind is GOODS_LEVEL")
		}
		goods, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return domain.MeasurementTarget{}, err
		}
		level, err := requiredText("packagingLevel", document.PackagingLevel, domain.NewGoodsUnitName)
		if err != nil {
			return domain.MeasurementTarget{}, err
		}
		return domain.NewGoodsLevelMeasurementTarget(owner, goods, level)
	case domain.MeasurementTargetCarton:
		if document.GoodsCode != nil || document.PackagingLevel != nil {
			return domain.MeasurementTarget{}, malformed("goodsCode and packagingLevel must not be given when targetKind is CARTON")
		}
		carton, err := requiredText("cartonNumber", document.CartonNumber, domain.NewCartonNumber)
		if err != nil {
			return domain.MeasurementTarget{}, err
		}
		return domain.NewCartonMeasurementTarget(owner, carton)
	default:
		return domain.MeasurementTarget{}, malformed("targetKind is not a known measurement target kind")
	}
}
