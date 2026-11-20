package domain

import (
	"errors"
	"fmt"
)

// 本文件是商品这一册的取值与封闭集。取值都逐字保存（CONTEXT「主数据登记」：货主给的码逐字保存，不折算），构造器只管「给了且存得下」；
// 零值一律是「没给」。可缺的格由持有它们的结构交回 (值, 是否给了)，取值类型本身不另带「给没给」的方法。

// OwnerAccountID 是货主客户账户标识，商品身份（租户，货主，商品编码）的一段。货主就是货主客户账户（CONTEXT「货主、仓与商品」）：
// 客户面取自接入信封，代录时是载荷里的业务内容。
type OwnerAccountID struct{ requiredValue }

func NewOwnerAccountID(value string) (OwnerAccountID, error) {
	required, err := newRequiredValue("owner account ID", value)
	return OwnerAccountID{required}, err
}

// GoodsCode 是商品编码，商品身份的一段，登记后不改。逐字保存：同一货主内 `SKU-1` 与 `sku-1` 是两个商品，别的货主的同一编码是另一个商品。
type GoodsCode struct{ requiredValue }

func NewGoodsCode(value string) (GoodsCode, error) {
	required, err := newRequiredValue("goods code", value)
	return GoodsCode{required}, err
}

type GoodsName struct{ requiredValue }

func NewGoodsName(value string) (GoodsName, error) {
	required, err := newRequiredValue("goods name", value)
	return GoodsName{required}, err
}

// GoodsUnitName 是商品包装层级的单位名，也是基本单位的写法（件、内盒、箱之类）。单位名是货主自己的词，不取产品内置的计量单位目录——
// 目录只管尺寸与重量带的单位。
type GoodsUnitName struct{ requiredValue }

func NewGoodsUnitName(value string) (GoodsUnitName, error) {
	required, err := newRequiredValue("goods unit name", value)
	return GoodsUnitName{required}, err
}

// GoodsAliasCode 是商品别名的码（商品条码、平台商品码），逐字保存。
type GoodsAliasCode struct{ requiredValue }

func NewGoodsAliasCode(value string) (GoodsAliasCode, error) {
	required, err := newRequiredValue("goods alias code", value)
	return GoodsAliasCode{required}, err
}

// IdentifierCode 是同一货主内识别空间里的一个码：商品编码或商品别名的码。商品编码与各别名同在一个识别空间（CONTEXT「主数据登记」），
// 识别码冲突与按码找商品都以它说话，不分这个码是编码还是别名。
type IdentifierCode struct{ requiredValue }

func NewIdentifierCode(value string) (IdentifierCode, error) {
	required, err := newRequiredValue("identifier code", value)
	return IdentifierCode{required}, err
}

// PlatformName 是平台商品码可带的平台或渠道名，逐字保存；产品不内置平台词表（CONTEXT「商品别名」）。
type PlatformName struct{ requiredValue }

func NewPlatformName(value string) (PlatformName, error) {
	required, err := newRequiredValue("platform name", value)
	return PlatformName{required}, err
}

// RequestKey 是判重空间里的键：货主口是来源请求键，代录口是决定标识。
type RequestKey struct{ requiredValue }

func NewRequestKey(value string) (RequestKey, error) {
	required, err := newRequiredValue("request key", value)
	return RequestKey{required}, err
}

// SubmissionSource 是货主口的来源（客户渠道来源）；代录口没有来源。
type SubmissionSource struct{ requiredValue }

func NewSubmissionSource(value string) (SubmissionSource, error) {
	required, err := newRequiredValue("submission source", value)
	return SubmissionSource{required}, err
}

// EvidenceReference 是代录的依据引用：货主授权的凭据在别处的引用，逐字保存，不判真伪。
type EvidenceReference struct{ requiredValue }

func NewEvidenceReference(value string) (EvidenceReference, error) {
	required, err := newRequiredValue("evidence reference", value)
	return EvidenceReference{required}, err
}

// DeclaredGoodsName 是商品申报资料里的申报品名，逐字保存，不判归类。
type DeclaredGoodsName struct{ requiredValue }

func NewDeclaredGoodsName(value string) (DeclaredGoodsName, error) {
	required, err := newRequiredValue("declared goods name", value)
	return DeclaredGoodsName{required}, err
}

// HSCode 是商品申报资料里的 HS 编码，逐字保存，不判归类、不判位数（归 customs-compliance）。
type HSCode struct{ requiredValue }

func NewHSCode(value string) (HSCode, error) {
	required, err := newRequiredValue("HS code", value)
	return HSCode{required}, err
}

// CurrencyCode 是申报单价的币种码。构造器只管「给了且存得下」；是不是产品内置币种目录里的码，由登记口在判过重放、冲突、基准、不可改的格
// 与识别码之后另判（ADR-0345 决定五），读回的在册行因此不受目录更新牵连。
type CurrencyCode struct{ requiredValue }

func NewCurrencyCode(value string) (CurrencyCode, error) {
	required, err := newRequiredValue("currency code", value)
	return CurrencyCode{required}, err
}

// GoodsAliasKind 是别名类型，产品内置的封闭两种：商品条码与平台商品码。封闭集之外的词是接入层的坏载荷，到不了领域。
type GoodsAliasKind uint8

const (
	GoodsAliasKindInvalid GoodsAliasKind = iota
	GoodsAliasBarcode
	GoodsAliasPlatformCode
)

func (kind GoodsAliasKind) String() string {
	switch kind {
	case GoodsAliasBarcode:
		return "BARCODE"
	case GoodsAliasPlatformCode:
		return "PLATFORM_CODE"
	default:
		return ""
	}
}

// ErrUnknownGoodsWord 是封闭集的词不在集合里：别名类型、出库方式与提交途径都按字面认。
var ErrUnknownGoodsWord = errors.New("warehouse fulfillment: unknown closed-set word")

// NewGoodsAliasKind 按字面认别名类型，只认 `BARCODE` 与 `PLATFORM_CODE`。
func NewGoodsAliasKind(word string) (GoodsAliasKind, error) {
	for _, kind := range []GoodsAliasKind{GoodsAliasBarcode, GoodsAliasPlatformCode} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return GoodsAliasKindInvalid, fmt.Errorf("%w: alias kind", ErrUnknownGoodsWord)
}

// GoodsOutboundMode 是组合商品的出库方式，产品内置封闭两种：按组件出（出库请求里它的行在受理时展开成组件行）与按成品出
// （只出加工好的成品库存）。零值是没给；非组合商品不带它。
type GoodsOutboundMode uint8

const (
	GoodsOutboundModeInvalid GoodsOutboundMode = iota
	GoodsOutboundByComponent
	GoodsOutboundAsFinishedGoods
)

func (mode GoodsOutboundMode) String() string {
	switch mode {
	case GoodsOutboundByComponent:
		return "BY_COMPONENT"
	case GoodsOutboundAsFinishedGoods:
		return "AS_FINISHED_GOODS"
	default:
		return ""
	}
}

// NewGoodsOutboundMode 按字面认出库方式，只认 `BY_COMPONENT` 与 `AS_FINISHED_GOODS`。
func NewGoodsOutboundMode(word string) (GoodsOutboundMode, error) {
	for _, mode := range []GoodsOutboundMode{GoodsOutboundByComponent, GoodsOutboundAsFinishedGoods} {
		if mode.String() == word {
			return mode, nil
		}
	}
	return GoodsOutboundModeInvalid, fmt.Errorf("%w: outbound mode", ErrUnknownGoodsWord)
}

// SubmissionPath 是一笔商品修订的提交途径：货主经客户渠道，或运营按货主授权代录。两条途径写的是同一条修订序列，判重空间却各自独立。
type SubmissionPath uint8

const (
	SubmissionPathInvalid SubmissionPath = iota
	SubmissionByOwner
	SubmissionOnBehalf
)

func (path SubmissionPath) String() string {
	switch path {
	case SubmissionByOwner:
		return "OWNER"
	case SubmissionOnBehalf:
		return "ON_BEHALF"
	default:
		return ""
	}
}

// NewSubmissionPath 按字面认提交途径，只认 `OWNER` 与 `ON_BEHALF`。
func NewSubmissionPath(word string) (SubmissionPath, error) {
	for _, path := range []SubmissionPath{SubmissionByOwner, SubmissionOnBehalf} {
		if path.String() == word {
			return path, nil
		}
	}
	return SubmissionPathInvalid, fmt.Errorf("%w: submission path", ErrUnknownGoodsWord)
}
