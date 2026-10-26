package main

import (
	"context"

	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

func init() {
	registerUnwired(customerTrackingViewWiring{trackingViews: unwiredTrackingViews{}, attributions: unwiredCustomerAttributions{}})
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
