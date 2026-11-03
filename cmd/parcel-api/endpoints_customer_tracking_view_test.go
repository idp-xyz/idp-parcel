package main

import (
	"context"

	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

func init() {
	registerUnwired(customerTrackingViewWiring{trackingViews: unwiredTrackingViews{}, attributions: unwiredCustomerAttributions{}, shipments: unwiredCustomerShipments{}})
}

// unwiredCustomerAttributions 是归属反查口的未接线占位：答没接线的错误，落未形成那一格，绝不顶成零份——那会把一次未接线
// 说成「这件不归你」。装配测试里各行恒为未配置即拒，走不到它。
type unwiredCustomerAttributions struct{}

func (unwiredCustomerAttributions) FindCustomerAccount(
	context.Context,
	vedomain.TenantID,
	vedomain.TrackedParcelReference,
) (vedomain.CustomerAccountReference, bool, error) {
	return vedomain.CustomerAccountReference{}, false, errOrchestrationNotWired
}

// unwiredCustomerShipments 是整单读口的未接线占位：答没接线的错误，落未形成那一格，绝不顶成查无此单。装配测试里各行恒为未配置
// 即拒，走不到它。
type unwiredCustomerShipments struct{}

func (unwiredCustomerShipments) LocateCustomerShipment(
	context.Context,
	vedomain.TenantID,
	vedomain.CustomerAccountReference,
	vedomain.ShipmentDocumentNumberReference,
	veports.ShipmentParcelPage,
) (veports.LocatedCustomerShipment, error) {
	return veports.LocatedCustomerShipment{}, errOrchestrationNotWired
}
