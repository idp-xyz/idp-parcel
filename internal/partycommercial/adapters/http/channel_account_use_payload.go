package commercialhttp

import (
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// channelAccountUseDocument 是使用授权登记口的在线载荷（ADR-0093）。本族没有受控 CLI，载荷只此一份；形状照本包在线口
// 的惯例，tenantId 一格只为拒而存在（理由在 partyIdentityBatchDocument.TenantID）。
//
// 技术存续与业务存续两格是发布当时的答复（application.RegisterChannelAccountUseCommand 注释）：缺席译成未答，由领域的
// 发布门判——「没人问过通道」与「问过且不通」要分得开，所以这里不替缺席补任何一格；集外取值拒收不吸收。
type channelAccountUseDocument struct {
	TenantID          json.RawMessage `json:"tenantId"`
	AuthorizationID   string          `json:"authorizationId"`
	Revision          int             `json:"revision"`
	Account           string          `json:"account"`
	Grantor           string          `json:"grantor"`
	Grantee           string          `json:"grantee"`
	Channel           string          `json:"channel"`
	Scope             string          `json:"scope"`
	EffectiveStartsAt time.Time       `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time      `json:"effectiveEndsAt"`
	TechnicalStanding string          `json:"technicalStanding"`
	BusinessStanding  string          `json:"businessStanding"`
	PublishedAt       time.Time       `json:"publishedAt"`
}

func (document channelAccountUseDocument) command(tenant domain.TenantID) (application.RegisterChannelAccountUseCommand, error) {
	var none application.RegisterChannelAccountUseCommand
	id, err := domain.NewChannelAccountUseAuthorizationID(document.AuthorizationID)
	if err != nil {
		return none, err
	}
	account, err := domain.NewChannelAccountID(document.Account)
	if err != nil {
		return none, err
	}
	grantor, err := domain.NewPartyID(document.Grantor)
	if err != nil {
		return none, err
	}
	grantee, err := domain.NewPartyID(document.Grantee)
	if err != nil {
		return none, err
	}
	channel, err := domain.NewChannelProductReference(document.Channel)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, err
	}
	endsAt := time.Time{}
	if document.EffectiveEndsAt != nil {
		endsAt = *document.EffectiveEndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.EffectiveStartsAt, endsAt)
	if err != nil {
		return none, err
	}
	technical, err := technicalStandingFromName(document.TechnicalStanding)
	if err != nil {
		return none, err
	}
	business, err := businessStandingFromName(document.BusinessStanding)
	if err != nil {
		return none, err
	}
	return application.RegisterChannelAccountUseCommand{
		Tenant:      tenant,
		ID:          id,
		Revision:    document.Revision,
		Account:     account,
		Grantor:     grantor,
		Grantee:     grantee,
		Channel:     channel,
		Scope:       scope,
		Effective:   effective,
		Technical:   technical,
		Business:    business,
		PublishedAt: document.PublishedAt,
	}, nil
}

// channelAccountUseRevocationDocument 是撤销口的在线载荷：只带要撤的那一笔、依据与时点，不带授权正文——正文从册上最新
// 修订取回（ADR-0093 决定六），载荷里出现正文的格按未知键拒。
type channelAccountUseRevocationDocument struct {
	TenantID        json.RawMessage `json:"tenantId"`
	AuthorizationID string          `json:"authorizationId"`
	Basis           string          `json:"basis"`
	RevokedAt       time.Time       `json:"revokedAt"`
}

func (document channelAccountUseRevocationDocument) command(tenant domain.TenantID) (application.RevokeChannelAccountUseCommand, error) {
	var none application.RevokeChannelAccountUseCommand
	id, err := domain.NewChannelAccountUseAuthorizationID(document.AuthorizationID)
	if err != nil {
		return none, err
	}
	basis, err := domain.NewChannelAccountRevocationBasisReference(document.Basis)
	if err != nil {
		return none, err
	}
	return application.RevokeChannelAccountUseCommand{Tenant: tenant, ID: id, Basis: basis, RevokedAt: document.RevokedAt}, nil
}

// technicalStandingFromName 是 domain.ChannelAccountTechnicalStanding 封闭集的名称镜像；空串即未答。
func technicalStandingFromName(raw string) (domain.ChannelAccountTechnicalStanding, error) {
	switch raw {
	case "":
		return domain.ChannelAccountTechnicalStandingInvalid, nil
	case domain.ChannelAccountTechnicallyAvailable.String():
		return domain.ChannelAccountTechnicallyAvailable, nil
	case domain.ChannelAccountTechnicallyUnavailable.String():
		return domain.ChannelAccountTechnicallyUnavailable, nil
	}
	return domain.ChannelAccountTechnicalStandingInvalid, fmt.Errorf("unknown technical standing %q", raw)
}

// businessStandingFromName 是 domain.ChannelAccountBusinessStanding 封闭集的名称镜像；空串即未答。
func businessStandingFromName(raw string) (domain.ChannelAccountBusinessStanding, error) {
	switch raw {
	case "":
		return domain.ChannelAccountBusinessStandingInvalid, nil
	case domain.ChannelAccountBusinessAuthorized.String():
		return domain.ChannelAccountBusinessAuthorized, nil
	case domain.ChannelAccountBusinessUnauthorized.String():
		return domain.ChannelAccountBusinessUnauthorized, nil
	}
	return domain.ChannelAccountBusinessStandingInvalid, fmt.Errorf("unknown business standing %q", raw)
}
