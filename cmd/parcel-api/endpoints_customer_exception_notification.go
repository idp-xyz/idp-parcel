package main

import (
	"net/http"
	"os"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type customerExceptionNotificationWiring struct {
	notifier visibilityhttp.CustomerExceptionNotifier
}

func init() {
	registerEndpoints(endpointFeature[customerExceptionNotificationWiring]{
		lines: map[string]endpointLine{
			"/customer-exception-notifications": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomerExceptionNotification)},
		},
		build: func(sources endpointSources) (customerExceptionNotificationWiring, error) {
			face, err := buildCustomerExceptionNotificationFace(sources.db, os.Getenv)
			return customerExceptionNotificationWiring{notifier: face}, err
		},
		mount: func(channels endpointChannels, wiring customerExceptionNotificationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 客户异常通知口（UC-VE-006 步 6）：把一份已成立的披露决定通知客户，首发与重发同一个口。挂「运营决定」能力面，
				// 与披露授权分开授予。写开关换不了它。
				{Pattern: "/customer-exception-notifications", Handler: visibilityhttp.NewNotifyCustomerExceptionEndpoint(channels.operatorDecisions.visibility, wiring.notifier)},
			}
		},
	})
}
