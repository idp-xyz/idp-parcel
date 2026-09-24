package main

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// buildPriceCardImportPreview 装配 `/pricing-price-card-previews` 的真编排（ADR-0101 决定四；票 price-card-import/02）。
// 预览只读上传的字节、不碰库，依赖只有模板读口；录入口（票 03）要拿同一个读口，同一份字节过两口才逐字节同摘要。
func buildPriceCardImportPreview() (*pricingapp.PreviewPriceCardImportHandler, error) {
	handler, err := pricingapp.NewPreviewPriceCardImportHandler(pricecardtemplate.Reader{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: price card import preview: %w", err)
	}
	return handler, nil
}
