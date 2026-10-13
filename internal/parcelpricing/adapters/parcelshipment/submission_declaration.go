// Package parcelshipment 是 parcel-pricing 消费 parcel-shipment 的一侧（ADR-0025 消费方侧）：受理前试算按提交版本读申报
// （ADR-0179 决定四）。只翻译不判断——单位符号按原词进计价单位表、两段邮编拼成邮编路线；哪张卡要哪一格、缺了怎么停，归入口。
package parcelshipment

import (
	"context"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmissionDeclarations 实现 ppports.SubmissionDeclarationView。
type SubmissionDeclarations struct {
	declarations psports.SubmissionVersionDeclarationView
}

// NewSubmissionDeclarations 构造期拒 nil：漏装读口要在启动时炸。
func NewSubmissionDeclarations(declarations psports.SubmissionVersionDeclarationView) (*SubmissionDeclarations, error) {
	if declarations == nil {
		return nil, fmt.Errorf("parcel pricing parcelshipment adapter: submission version declaration view is nil")
	}
	return &SubmissionDeclarations{declarations: declarations}, nil
}

var _ ppports.SubmissionDeclarationView = (*SubmissionDeclarations)(nil)

// LoadSubmissionDeclaration 按提交版本逐件译申报。三个引用在提供方的构造门上立不起来时答不可见：立不起来的引用指不到那一侧的
// 任何一份委托，与「这个租户下没有」是同一个答案。
func (adapter *SubmissionDeclarations) LoadSubmissionDeclaration(
	ctx context.Context,
	tenant ppdomain.TenantID,
	shipmentRequest string,
	submissionVersion string,
) (ppports.SubmissionDeclaration, bool, error) {
	none := ppports.SubmissionDeclaration{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, nil
	}
	request, err := psdomain.NewShipmentRequestID(shipmentRequest)
	if err != nil {
		return none, false, nil
	}
	version, err := psdomain.NewSubmissionVersionID(submissionVersion)
	if err != nil {
		return none, false, nil
	}
	declaration, found, err := adapter.declarations.LoadSubmissionVersionDeclaration(ctx, psTenant, request, version)
	if err != nil {
		return none, false, fmt.Errorf("load submission declaration: %w", err)
	}
	if !found {
		return none, false, nil
	}

	parcels := declaration.Parcels()
	translated := ppports.SubmissionDeclaration{
		Parcels: make([]ppports.DeclaredParcel, 0, len(parcels)),
		Route:   postalRouteOf(declaration),
	}
	for _, parcel := range parcels {
		declared := ppports.DeclaredParcel{Parcel: parcel.Parcel().String()}
		if measurement, present := parcel.Measurement(); present {
			declared.Weight, declared.Dimensions = measurementOf(measurement)
		}
		translated.Parcels = append(translated.Parcels, declared)
	}
	return translated, true, nil
}

// measurementOf 把一份申报测量译成计价的重量与外廓。单位符号按原词进计价的封闭单位表，不折大小写：正式 SELL 评价的输入解析
// （transportfulfillment 适配器的 weightUnit / lengthUnit）按原词认，受理前若折叠，同一份申报会在受理前估出冻结额、受理后却评不出
// 客户费用，那笔冻结就没有费用可转。表外的单位不猜，整件当没有测量交回——猜错单位估错的是要占用客户的那笔钱。
func measurementOf(measurement psdomain.DeclaredMeasurement) (*ppdomain.Weight, *ppdomain.Dimensions) {
	declared := measurement.Weight()
	unit, err := ppdomain.NewWeightUnit(declared.Unit().String())
	if err != nil {
		return nil, nil
	}
	weight, err := ppdomain.NewWeightFromString(declared.Value().String(), unit)
	if err != nil {
		return nil, nil
	}
	sides, present := measurement.Dimensions()
	if !present {
		return &weight, nil
	}
	lengthUnit, err := ppdomain.NewLengthUnit(sides.Unit().String())
	if err != nil {
		return nil, nil
	}
	length, lengthErr := ppdomain.ParseDecimal(sides.Length().String())
	width, widthErr := ppdomain.ParseDecimal(sides.Width().String())
	height, heightErr := ppdomain.ParseDecimal(sides.Height().String())
	if lengthErr != nil || widthErr != nil || heightErr != nil {
		return nil, nil
	}
	dimensions, err := ppdomain.NewDimensions(length, width, height, lengthUnit)
	if err != nil {
		return nil, nil
	}
	return &weight, &dimensions
}

// postalRouteOf 把两段邮编拼成邮编路线；收件邮编没报即没有路线。始发邮编过不了路线的构造门时只按目的邮编拼——与正式评价的输入
// 解析器（transportfulfillment 适配器）同一处置，同一件包裹在受理前后不该因此拿到两种路线。
func postalRouteOf(declaration psdomain.SubmissionVersionDeclaration) *ppdomain.PostalRoute {
	destination, declared := declaration.DestinationPostalCode()
	if !declared || destination == "" {
		return nil
	}
	origin, _ := declaration.OriginPostalCode()
	route, err := ppdomain.NewPostalRoute(origin, destination)
	if err != nil && origin != "" {
		route, err = ppdomain.NewPostalRoute("", destination)
	}
	if err != nil {
		return nil
	}
	return &route
}
