package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 申报通道登记的译装（ADR-0342 决定三）。凭据只收运行环境里的变量名，不收密钥本身。
type declarationChannelDocument struct {
	TenantID      string `json:"tenantId"`
	Target        string `json:"target"`
	Endpoint      string `json:"endpoint"`
	CredentialRef string `json:"credentialRef"`
}

func DeclarationChannelFromJSON(raw []byte) (application.RegisterDeclarationChannelCommand, error) {
	none := application.RegisterDeclarationChannelCommand{}
	var document declarationChannelDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("申报通道登记输入不是本入口的形状：%w", err)
	}
	tenant, err := tenantFromDocument(document.TenantID)
	if err != nil {
		return none, err
	}
	channel, err := domain.NewDeclarationChannel(document.Target, document.Endpoint, document.CredentialRef)
	if err != nil {
		return none, err
	}
	return application.RegisterDeclarationChannelCommand{TenantID: tenant, Channel: channel}, nil
}
