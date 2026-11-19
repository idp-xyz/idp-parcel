package registrationjson

import (
	"errors"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// packagingMaterialDocument 是一笔包材修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`dimensions.length`、`weight.value`、
// `weight.unit` 等），登记方照答复里的格改报文，不必再做一次对照。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。四个数值是 JSON 字符串，不是
// JSON 数字：数字经 JSON 解码器会先走一遍二进制浮点，`0.1` 与 `0.10` 的写法在那里就丢了。
type packagingMaterialDocument struct {
	carriedIdentity
	PackagingMaterialCode *string             `json:"packagingMaterialCode"`
	Revision              *int                `json:"revision"`
	Name                  *string             `json:"name"`
	Dimensions            *dimensionsDocument `json:"dimensions"`
	Weight                *weightDocument     `json:"weight"`
	Deactivated           *bool               `json:"deactivated"`
}

type dimensionsDocument struct {
	Length *string `json:"length"`
	Width  *string `json:"width"`
	Height *string `json:"height"`
	Unit   *string `json:"unit"`
}

type weightDocument struct {
	Value *string `json:"value"`
	Unit  *string `json:"unit"`
}

// PackagingMaterialCommand 把一笔包材修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象，规矩同 WarehouseCommand。
//
// 包材编码与名称逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。四个数值只认 `12` 或 `12.50` 这种十进制数（见 domain.NewMeasure），
// 写法不对或位数超了是坏载荷，理由只点名那一格、不回显取值；单位给成空白也是坏载荷。数值是否为零、单位在不在计量单位目录里、类别对不对，
// 译装不判：那是应用层在判过重放、冲突与修订连续之后另判的事，译装提前拒掉，同一份内容在「原样重投」时就会先于「已登记（重放）」被拒。
// 是否停用缺失即不停用，不取任何默认。
func PackagingMaterialCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterPackagingMaterialCommand, error) {
	var none application.RegisterPackagingMaterialCommand
	document, err := decodeDocument[packagingMaterialDocument](reader, "packaging material revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}

	if document.PackagingMaterialCode == nil {
		return none, malformed("packagingMaterialCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.Name == nil {
		return none, malformed("name is required")
	}
	if document.Dimensions == nil {
		return none, malformed("dimensions is required")
	}
	if document.Weight == nil {
		return none, malformed("weight is required")
	}
	for _, cell := range []struct {
		path  string
		given *string
	}{
		{"dimensions.length", document.Dimensions.Length},
		{"dimensions.width", document.Dimensions.Width},
		{"dimensions.height", document.Dimensions.Height},
		{"dimensions.unit", document.Dimensions.Unit},
		{"weight.value", document.Weight.Value},
		{"weight.unit", document.Weight.Unit},
	} {
		if cell.given == nil {
			return none, malformed("%s is required", cell.path)
		}
	}

	code, err := textCell("packagingMaterialCode", *document.PackagingMaterialCode, domain.NewPackagingMaterialCode)
	if err != nil {
		return none, err
	}
	name, err := textCell("name", *document.Name, domain.NewPackagingMaterialName)
	if err != nil {
		return none, err
	}
	length, err := measureCell("dimensions.length", *document.Dimensions.Length)
	if err != nil {
		return none, err
	}
	width, err := measureCell("dimensions.width", *document.Dimensions.Width)
	if err != nil {
		return none, err
	}
	height, err := measureCell("dimensions.height", *document.Dimensions.Height)
	if err != nil {
		return none, err
	}
	dimensionsUnit, err := textCell("dimensions.unit", *document.Dimensions.Unit, domain.NewUnitCode)
	if err != nil {
		return none, err
	}
	weight, err := measureCell("weight.value", *document.Weight.Value)
	if err != nil {
		return none, err
	}
	weightUnit, err := textCell("weight.unit", *document.Weight.Unit, domain.NewUnitCode)
	if err != nil {
		return none, err
	}

	spec := domain.PackagingMaterialContentSpec{
		Name:           name,
		Length:         length,
		Width:          width,
		Height:         height,
		DimensionsUnit: dimensionsUnit,
		Weight:         weight,
		WeightUnit:     weightUnit,
	}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewPackagingMaterialContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the packaging material content is incomplete")
	}
	return application.RegisterPackagingMaterialCommand{
		Tenant:     tenant,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}

// measureCell 过一格数值的领域构造门。理由只点名 JSON 里的键路径与规则，不回显取值。
func measureCell(path, raw string) (domain.Measure, error) {
	measure, err := domain.NewMeasure(raw)
	switch {
	case err == nil:
		return measure, nil
	case errors.Is(err, domain.ErrMeasureTooLong):
		return measure, malformed("%s allows at most %d integer digits and %d fractional digits",
			path, domain.MeasureMaxIntegerDigits, domain.MeasureMaxFractionDigits)
	default:
		return measure, malformed("%s must be written as digits with an optional decimal point and digits on both sides of it, "+
			"without sign, exponent, thousands separator or whitespace", path)
	}
}
