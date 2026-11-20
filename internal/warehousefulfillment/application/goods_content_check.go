package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// contentDefects 是判定次序里资料内容那一格：逐格指出全部不合的格，不只第一格。次序固定，同一份资料有几处不合时答出的列表才是确定的：
//
//   - 必填格没给（名称、基本单位与各列表项里的必填格）：有就只答这些，后面的核对对着缺了格的资料没有意义；
//   - 基本单位、包装层级整体，再是各层（层名重复、折合数不是正整数），位置按报文里交来的先后；
//   - 各别名（平台或渠道名只有平台商品码可以带；有效区间止不晚于起）；
//   - 组成：非组合带了组成，或组合没有组件、没有出库方式，再是各组件（编码重复或就是自己、没登记、是组合商品；数量不是正整数）；
//   - 申报资料：原产国、申报单价、币种；
//   - 声明的尺寸重量：层级，再是长、宽、高、尺寸单位、重量数值、重量单位。
//
// 目录里的码（国家、币种、计量单位）与「为正」是只拦新修订的登记门，在这里判，不在领域构造器里：读回的在册行不受目录更新牵连（ADR-0335、ADR-0345）。
// 组件读的是别的商品的当前修订；读不动走 error，不折成「没登记」。
func (handler *RegisterGoodsHandler) contentDefects(
	ctx context.Context,
	command RegisterGoodsCommand,
) ([]domain.GoodsCell, error) {
	content := command.Content
	if missing := content.MissingCells(); len(missing) > 0 {
		return missing, nil
	}

	var cells []domain.GoodsCell
	flag := func(field domain.GoodsField, index int) {
		cells = append(cells, domain.NewGoodsCell(field, index))
	}

	levels := content.PackagingLevels()
	unitary, baseUnitDeclared := 0, false
	for _, level := range levels {
		if level.Factor() == 1 {
			unitary++
			baseUnitDeclared = baseUnitDeclared || level.Name() == content.BaseUnit()
		}
	}
	if !baseUnitDeclared {
		flag(domain.GoodsFieldBaseUnit, 0)
	}
	if unitary != 1 {
		flag(domain.GoodsFieldPackagingLevels, 0)
	}
	levelNames := make(map[domain.GoodsUnitName]bool, len(levels))
	for index, level := range levels {
		if levelNames[level.Name()] {
			flag(domain.GoodsFieldPackagingLevelName, index)
		}
		levelNames[level.Name()] = true
		if level.Factor() < 1 {
			flag(domain.GoodsFieldPackagingLevelFactor, index)
		}
	}

	for index, alias := range content.Aliases() {
		if _, named := alias.Platform(); named && alias.Kind() != domain.GoodsAliasPlatformCode {
			flag(domain.GoodsFieldAliasPlatform, index)
		}
		if alias.Validity().Inverted() {
			flag(domain.GoodsFieldAliasValidity, index)
		}
	}

	compositionCells, err := handler.compositionDefects(ctx, command)
	if err != nil {
		return nil, err
	}
	cells = append(cells, compositionCells...)

	declaration := content.Declaration()
	if country, given := declaration.OriginCountry(); given && refdata.CheckCountry(country.String()) != nil {
		flag(domain.GoodsFieldDeclarationOriginCountry, 0)
	}
	if price, currency, given := declaration.UnitPrice(); given {
		if price.IsZero() {
			flag(domain.GoodsFieldDeclarationUnitPrice, 0)
		}
		if refdata.CheckCurrency(currency.String()) != nil {
			flag(domain.GoodsFieldDeclarationCurrency, 0)
		}
	}

	measuredLevels := make(map[domain.GoodsUnitName]bool)
	for index, measurement := range content.DeclaredMeasurements() {
		// 声明的尺寸重量按包装层级给：层级须是本份资料声明过的，同一层至多一条。
		if !levelNames[measurement.Level()] || measuredLevels[measurement.Level()] {
			flag(domain.GoodsFieldMeasurementLevel, index)
		}
		measuredLevels[measurement.Level()] = true
		if dimensions, declared := measurement.Dimensions(); declared {
			if dimensions.Length().IsZero() {
				flag(domain.GoodsFieldMeasurementLength, index)
			}
			if dimensions.Width().IsZero() {
				flag(domain.GoodsFieldMeasurementWidth, index)
			}
			if dimensions.Height().IsZero() {
				flag(domain.GoodsFieldMeasurementHeight, index)
			}
			if !isUnitOfCategory(dimensions.Unit().String(), refdata.UnitCategoryLength) {
				flag(domain.GoodsFieldMeasurementDimensionsUnit, index)
			}
		}
		if weight, declared := measurement.Weight(); declared {
			if weight.Value().IsZero() {
				flag(domain.GoodsFieldMeasurementWeightValue, index)
			}
			if !isUnitOfCategory(weight.Unit().String(), refdata.UnitCategoryWeight) {
				flag(domain.GoodsFieldMeasurementWeightUnit, index)
			}
		}
	}
	return cells, nil
}

// compositionDefects 判组成是否成立。非组合商品不带组成（组件与出库方式）；组合商品至少一个组件、带出库方式，每个组件的商品编码在本货主名下
// 已登记且是非组合商品、不重复、不是自己，数量是正整数。组件按落册这一刻别的商品的当前修订判。
func (handler *RegisterGoodsHandler) compositionDefects(
	ctx context.Context,
	command RegisterGoodsCommand,
) ([]domain.GoodsCell, error) {
	content := command.Content
	components := content.Components()
	_, modeGiven := content.OutboundMode()

	var cells []domain.GoodsCell
	flag := func(field domain.GoodsField, index int) {
		cells = append(cells, domain.NewGoodsCell(field, index))
	}

	if !content.Composite() {
		if len(components) > 0 {
			flag(domain.GoodsFieldCompositionComponents, 0)
		}
		if modeGiven {
			flag(domain.GoodsFieldCompositionOutboundMode, 0)
		}
		return cells, nil
	}
	if len(components) == 0 {
		flag(domain.GoodsFieldCompositionComponents, 0)
	}
	if !modeGiven {
		flag(domain.GoodsFieldCompositionOutboundMode, 0)
	}

	seen := make(map[domain.GoodsCode]bool, len(components))
	for index, component := range components {
		usable := !seen[component.Code()] && component.Code() != command.Code
		seen[component.Code()] = true
		if usable {
			registered, found, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, component.Code())
			if err != nil {
				return nil, fmt.Errorf("register goods: 读组件 %q：%w", component.Code().String(), err)
			}
			usable = found && !registered.Content().Composite()
		}
		if !usable {
			flag(domain.GoodsFieldComponentCode, index)
		}
		if component.Quantity() < 1 {
			flag(domain.GoodsFieldComponentQuantity, index)
		}
	}
	return cells, nil
}
