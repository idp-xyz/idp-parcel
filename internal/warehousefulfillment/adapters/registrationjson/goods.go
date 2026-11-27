package registrationjson

import (
	"errors"
	"io"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// goodsOnBehalfDocument 是一笔商品代录的线格式。键名与登记口答复里指出的格同一套（`packagingLevels[].factor`、
// `declaration.unitPrice` 等）。必填格用指针，缺失与零值才分得开：基准修订 0 是给了基准 0，不是没给。
// 数值是 JSON 字符串，不是 JSON 数字：数字经解码器会先走一遍二进制浮点。
//
// 货主客户账户、决定标识与依据引用是这一口的业务内容（ADR-0384）。租户与登记责任方不在载荷里。
type goodsOnBehalfDocument struct {
	carriedIdentity
	OwnerAccount         *string                    `json:"ownerAccount"`
	DecisionID           *string                    `json:"decisionId"`
	EvidenceReference    *string                    `json:"evidenceReference"`
	GoodsCode            *string                    `json:"goodsCode"`
	BasisRevision        *int                       `json:"basisRevision"`
	Name                 *string                    `json:"name"`
	BaseUnit             *string                    `json:"baseUnit"`
	PackagingLevels      []goodsLevelDocument       `json:"packagingLevels"`
	TrackBatch           *bool                      `json:"trackBatch"`
	TrackExpiry          *bool                      `json:"trackExpiry"`
	TrackSerial          *bool                      `json:"trackSerial"`
	Aliases              []goodsAliasDocument       `json:"aliases"`
	Composition          *goodsCompositionDocument  `json:"composition"`
	Declaration          *goodsDeclarationDocument  `json:"declaration"`
	DeclaredMeasurements []goodsMeasurementDocument `json:"declaredMeasurements"`
}

type goodsLevelDocument struct {
	Name   *string `json:"name"`
	Factor *int64  `json:"factor"`
}

type goodsAliasDocument struct {
	Code     *string                `json:"code"`
	Type     *string                `json:"type"`
	Platform *string                `json:"platform"`
	Validity *goodsValidityDocument `json:"validity"`
}

type goodsValidityDocument struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

type goodsCompositionDocument struct {
	OutboundMode *string                  `json:"outboundMode"`
	Components   []goodsComponentDocument `json:"components"`
}

type goodsComponentDocument struct {
	Code     *string `json:"code"`
	Quantity *int64  `json:"quantity"`
}

type goodsDeclarationDocument struct {
	Name          *string `json:"name"`
	HSCode        *string `json:"hsCode"`
	OriginCountry *string `json:"originCountry"`
	UnitPrice     *string `json:"unitPrice"`
	Currency      *string `json:"currency"`
}

type goodsMeasurementDocument struct {
	Level      *string                  `json:"level"`
	Dimensions *goodsDimensionsDocument `json:"dimensions"`
	Weight     *goodsWeightDocument     `json:"weight"`
}

type goodsDimensionsDocument struct {
	Length *string `json:"length"`
	Width  *string `json:"width"`
	Height *string `json:"height"`
	Unit   *string `json:"unit"`
}

type goodsWeightDocument struct {
	Value *string `json:"value"`
	Unit  *string `json:"unit"`
}

// GoodsOnBehalfCommand 把一笔商品代录的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 没带依据引用、不认识的别名类型或出库方式，是坏载荷，不是资料内容不合：封闭集之外的词到不了领域。
// 折合数是否为正、区间是否颠倒、组件是否已登记，译装不判，留给登记用例，否则同一份内容原样重投会先于重放被拒。
func GoodsOnBehalfCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterGoodsCommand, error) {
	var none application.RegisterGoodsCommand
	document, err := decodeDocument[goodsOnBehalfDocument](reader, "goods revision recorded on behalf")
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
	code, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return none, err
	}
	name, err := requiredText("name", document.Name, domain.NewGoodsName)
	if err != nil {
		return none, err
	}
	baseUnit, err := requiredText("baseUnit", document.BaseUnit, domain.NewGoodsUnitName)
	if err != nil {
		return none, err
	}
	levels, err := goodsLevels(document.PackagingLevels)
	if err != nil {
		return none, err
	}
	aliases, err := goodsAliases(document.Aliases)
	if err != nil {
		return none, err
	}
	composite, outbound, components, err := goodsComposition(document.Composition)
	if err != nil {
		return none, err
	}
	declaration, err := goodsDeclaration(document.Declaration)
	if err != nil {
		return none, err
	}
	measurements, err := goodsMeasurements(document.DeclaredMeasurements)
	if err != nil {
		return none, err
	}
	content, err := domain.NewGoodsContent(domain.GoodsContentSpec{
		Name: name, BaseUnit: baseUnit, PackagingLevels: levels,
		TrackBatch: boolOrFalse(document.TrackBatch), TrackExpiry: boolOrFalse(document.TrackExpiry), TrackSerial: boolOrFalse(document.TrackSerial),
		Aliases: aliases, Composite: composite, OutboundMode: outbound, Components: components,
		Declaration: declaration, DeclaredMeasurements: measurements,
	})
	if err != nil {
		var field *domain.GoodsContentFieldError
		if errors.As(err, &field) {
			return none, malformed("%s is required", field.Cell.String())
		}
		return none, err
	}
	submission, err := domain.NewOnBehalfSubmission(decisionID, evidence)
	if err != nil {
		return none, err
	}
	return application.RegisterGoodsCommand{
		Tenant: tenant, Owner: owner, Code: code, Basis: goodsBasis(document.BasisRevision),
		Content: content, Submission: submission, Registrant: registrant,
	}, nil
}

func goodsBasis(number *int) domain.BasisRevision {
	if number == nil {
		return domain.BasisRevision{}
	}
	return domain.NewBasisRevision(*number)
}

func boolOrFalse(value *bool) bool {
	return value != nil && *value
}

func requiredText[Value any](path string, raw *string, construct func(string) (Value, error)) (Value, error) {
	var none Value
	if raw == nil {
		return none, malformed("%s is required", path)
	}
	return textCell(path, *raw, construct)
}

func goodsLevels(documents []goodsLevelDocument) ([]domain.PackagingLevel, error) {
	levels := make([]domain.PackagingLevel, 0, len(documents))
	for index, document := range documents {
		path := "packagingLevels[" + strconv.Itoa(index) + "]"
		name, err := requiredText(path+".name", document.Name, domain.NewGoodsUnitName)
		if err != nil {
			return nil, err
		}
		if document.Factor == nil {
			return nil, malformed("%s.factor is required", path)
		}
		levels = append(levels, domain.NewPackagingLevel(name, *document.Factor))
	}
	return levels, nil
}

func goodsAliases(documents []goodsAliasDocument) ([]domain.GoodsAlias, error) {
	aliases := make([]domain.GoodsAlias, 0, len(documents))
	for index, document := range documents {
		path := "aliases[" + strconv.Itoa(index) + "]"
		code, err := requiredText(path+".code", document.Code, domain.NewGoodsAliasCode)
		if err != nil {
			return nil, err
		}
		if document.Type == nil {
			return nil, malformed("%s.type is required", path)
		}
		kind, err := domain.NewGoodsAliasKind(*document.Type)
		if err != nil {
			return nil, malformed("%s.type is not a known alias type", path)
		}
		var platform domain.PlatformName
		if document.Platform != nil {
			platform, err = textCell(path+".platform", *document.Platform, domain.NewPlatformName)
			if err != nil {
				return nil, err
			}
		}
		validity, err := goodsValidity(path+".validity", document.Validity)
		if err != nil {
			return nil, err
		}
		aliases = append(aliases, domain.NewGoodsAlias(code, kind, platform, validity))
	}
	return aliases, nil
}

func goodsValidity(path string, document *goodsValidityDocument) (domain.AliasValidity, error) {
	if document == nil {
		return domain.NewAliasValidity(time.Time{}, time.Time{}), nil
	}
	from, err := optionalInstant(path+".from", document.From)
	if err != nil {
		return domain.AliasValidity{}, err
	}
	to, err := optionalInstant(path+".to", document.To)
	if err != nil {
		return domain.AliasValidity{}, err
	}
	return domain.NewAliasValidity(from, to), nil
}

func optionalInstant(path string, raw *string) (time.Time, error) {
	if raw == nil {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339Nano, *raw)
	if err != nil {
		return time.Time{}, malformed("%s must be an RFC3339 timestamp", path)
	}
	return at, nil
}

func goodsComposition(document *goodsCompositionDocument) (bool, domain.GoodsOutboundMode, []domain.GoodsComponent, error) {
	if document == nil {
		return false, domain.GoodsOutboundModeInvalid, nil, nil
	}
	if document.OutboundMode == nil {
		return false, domain.GoodsOutboundModeInvalid, nil, malformed("composition.outboundMode is required")
	}
	mode, err := domain.NewGoodsOutboundMode(*document.OutboundMode)
	if err != nil {
		return false, domain.GoodsOutboundModeInvalid, nil, malformed("composition.outboundMode is not a known outbound mode")
	}
	if document.Components == nil {
		return false, domain.GoodsOutboundModeInvalid, nil, malformed("composition.components is required")
	}
	components := make([]domain.GoodsComponent, 0, len(document.Components))
	for index, item := range document.Components {
		path := "composition.components[" + strconv.Itoa(index) + "]"
		code, err := requiredText(path+".code", item.Code, domain.NewGoodsCode)
		if err != nil {
			return false, domain.GoodsOutboundModeInvalid, nil, err
		}
		if item.Quantity == nil {
			return false, domain.GoodsOutboundModeInvalid, nil, malformed("%s.quantity is required", path)
		}
		components = append(components, domain.NewGoodsComponent(code, *item.Quantity))
	}
	return true, mode, components, nil
}

func goodsDeclaration(document *goodsDeclarationDocument) (domain.GoodsDeclaration, error) {
	if document == nil {
		return domain.GoodsDeclaration{}, nil
	}
	spec := domain.GoodsDeclarationSpec{}
	var err error
	if document.Name != nil {
		spec.Name, err = textCell("declaration.name", *document.Name, domain.NewDeclaredGoodsName)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if document.HSCode != nil {
		spec.HSCode, err = textCell("declaration.hsCode", *document.HSCode, domain.NewHSCode)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if document.OriginCountry != nil {
		spec.OriginCountry, err = textCell("declaration.originCountry", *document.OriginCountry, domain.NewCountryCode)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if document.UnitPrice != nil {
		spec.UnitPrice, err = measureCell("declaration.unitPrice", *document.UnitPrice)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if document.Currency != nil {
		spec.Currency, err = textCell("declaration.currency", *document.Currency, domain.NewCurrencyCode)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	declaration, err := domain.NewGoodsDeclaration(spec)
	if err != nil {
		return domain.GoodsDeclaration{}, malformed("declaration.unitPrice and declaration.currency must be given together")
	}
	return declaration, nil
}

func goodsMeasurements(documents []goodsMeasurementDocument) ([]domain.DeclaredMeasurement, error) {
	measurements := make([]domain.DeclaredMeasurement, 0, len(documents))
	for index, document := range documents {
		path := "declaredMeasurements[" + strconv.Itoa(index) + "]"
		level, err := requiredText(path+".level", document.Level, domain.NewGoodsUnitName)
		if err != nil {
			return nil, err
		}
		spec := domain.DeclaredMeasurementSpec{Level: level}
		if document.Dimensions != nil {
			dimensions, err := goodsDimensions(path+".dimensions", document.Dimensions)
			if err != nil {
				return nil, err
			}
			spec.Dimensions = &dimensions
		}
		if document.Weight != nil {
			weight, err := goodsWeight(path+".weight", document.Weight)
			if err != nil {
				return nil, err
			}
			spec.Weight = &weight
		}
		measurement, err := domain.NewDeclaredMeasurement(spec)
		if err != nil {
			return nil, malformed("%s must carry dimensions or weight", path)
		}
		measurements = append(measurements, measurement)
	}
	return measurements, nil
}

func goodsDimensions(path string, document *goodsDimensionsDocument) (domain.DeclaredDimensions, error) {
	length, err := requiredMeasure(path+".length", document.Length)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	width, err := requiredMeasure(path+".width", document.Width)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	height, err := requiredMeasure(path+".height", document.Height)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	if document.Unit == nil {
		return domain.DeclaredDimensions{}, malformed("%s.unit is required", path)
	}
	unit, err := textCell(path+".unit", *document.Unit, domain.NewUnitCode)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	dimensions, err := domain.NewDeclaredDimensions(length, width, height, unit)
	if err != nil {
		return domain.DeclaredDimensions{}, malformed("%s is incomplete", path)
	}
	return dimensions, nil
}

func goodsWeight(path string, document *goodsWeightDocument) (domain.DeclaredWeight, error) {
	value, err := requiredMeasure(path+".value", document.Value)
	if err != nil {
		return domain.DeclaredWeight{}, err
	}
	if document.Unit == nil {
		return domain.DeclaredWeight{}, malformed("%s.unit is required", path)
	}
	unit, err := textCell(path+".unit", *document.Unit, domain.NewUnitCode)
	if err != nil {
		return domain.DeclaredWeight{}, err
	}
	weight, err := domain.NewDeclaredWeight(value, unit)
	if err != nil {
		return domain.DeclaredWeight{}, malformed("%s is incomplete", path)
	}
	return weight, nil
}

func requiredMeasure(path string, raw *string) (domain.Measure, error) {
	if raw == nil {
		return domain.Measure{}, malformed("%s is required", path)
	}
	return measureCell(path, *raw)
}
