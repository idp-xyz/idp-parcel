package registrationjson

import (
	"errors"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OwnerGoodsFields 是货主商品维护口（POST /v1/warehouse/goods）解出来的业务内容，还没有来源请求键。
// 租户、货主客户账户与登记责任方只取自信封，载荷里再带即坏载荷；接入请求标识原样交给首方族推导口（前缀 goods:）。
type OwnerGoodsFields struct {
	AccessRequestID string
	Code            domain.GoodsCode
	Basis           domain.BasisRevision
	Content         domain.GoodsContent
}

type ownerGoodsDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID      *string                    `json:"accessRequestId"`
	GoodsCode            *string                    `json:"goodsCode"`
	BasisRevision        *int                       `json:"basisRevision"`
	Name                 *string                    `json:"name"`
	BaseUnit             *string                    `json:"baseUnit"`
	PackagingLevels      []goodsLevelDocument       `json:"packagingLevels"`
	TrackBatch           *bool                      `json:"trackBatch"`
	TrackExpiry          *bool                      `json:"trackExpiry"`
	Aliases              []goodsAliasDocument       `json:"aliases"`
	Composition          *goodsCompositionDocument  `json:"composition"`
	Declaration          *goodsDeclarationDocument  `json:"declaration"`
	DeclaredMeasurements []goodsMeasurementDocument `json:"declaredMeasurements"`
}

// ReadOwnerGoods 把货主商品维护的 JSON 载荷译成字段。
// 自带 tenantId、registrant、ownerAccount 或不认识的字段（含代录用的 decisionId、evidenceReference）都是坏载荷。
// 折合数是否为正、区间是否颠倒、组件是否已登记留给登记用例，以免重放先于判重被拒。
func ReadOwnerGoods(reader io.Reader) (OwnerGoodsFields, error) {
	var none OwnerGoodsFields
	document, err := decodeDocument[ownerGoodsDocument](reader, "owner goods revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload; the owner comes from the customer access channel")
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
		TrackBatch: boolOrFalse(document.TrackBatch), TrackExpiry: boolOrFalse(document.TrackExpiry),
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
	return OwnerGoodsFields{
		AccessRequestID: rawAccessRequestID(document.AccessRequestID),
		Code:            code,
		Basis:           goodsBasis(document.BasisRevision),
		Content:         content,
	}, nil
}
