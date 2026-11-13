package registrationjson

import (
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type notificationChannelDocument struct {
	TenantID   string `json:"tenantId"`
	Channel    string `json:"channel"`
	Endpoint   string `json:"endpoint"`
	Credential string `json:"credential"`
}

// NotificationChannelFromJSON 译离线口的通知渠道登记：租户、渠道引用（与通知策略里写的同一个词）、推送端点，以及凭据
// 在运行环境里的变量名（可空）。凭据本身不进输入文件。
func NotificationChannelFromJSON(raw []byte) (domain.TenantID, domain.NotificationChannelEndpoint, error) {
	var document notificationChannelDocument
	if err := decodeStrict(raw, &document, "通知渠道"); err != nil {
		return domain.TenantID{}, domain.NotificationChannelEndpoint{}, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.NotificationChannelEndpoint{}, err
	}
	reference, err := domain.NewNotificationChannelReference(document.Channel)
	if err != nil {
		return domain.TenantID{}, domain.NotificationChannelEndpoint{}, err
	}
	channel, err := domain.NewNotificationChannelEndpoint(reference, document.Endpoint, document.Credential)
	if err != nil {
		return domain.TenantID{}, domain.NotificationChannelEndpoint{}, err
	}
	return tenant, channel, nil
}
