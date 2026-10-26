package main

import (
	"context"
	"fmt"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type commercialRelationshipEndingWiring struct {
	relationshipEnding commercialhttp.PartyRelationshipEndingRegistrar
}

func init() {
	registerEndpoints(endpointFeature[commercialRelationshipEndingWiring]{
		lines: map[string]endpointLine{
			"/commercial-party-relationship-revocations":   {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-party-relationship-expirations":   {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-party-relationship-supersessions": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (commercialRelationshipEndingWiring, error) {
			registrar, err := buildCommercialRelationshipEnding(sources.db)
			return commercialRelationshipEndingWiring{relationshipEnding: registrar}, err
		},
		mount: func(channels endpointChannels, wiring commercialRelationshipEndingWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 参与方关系三个结束口（票 ps-external-mark-relations/17）与身份族四类登记加停用同挂操作者渠道的登记册 Intake：
				// 改的是同一本参与方册，提交方与授予同一份（ADR-0151 的定族判据）；写开关换不了它们（ADR-0150 决定三）。名字不叫
				// `-deactivations`：关系的终止走撤销/到期/替代，不叫停用。
				{Pattern: "/commercial-party-relationship-revocations", Handler: commercialhttp.NewRevokePartyRelationshipEndpoint(channels.operatorRegistries.commercial, wiring.relationshipEnding)},
				{Pattern: "/commercial-party-relationship-expirations", Handler: commercialhttp.NewExpirePartyRelationshipEndpoint(channels.operatorRegistries.commercial, wiring.relationshipEnding)},
				{Pattern: "/commercial-party-relationship-supersessions", Handler: commercialhttp.NewSupersedePartyRelationshipEndpoint(channels.operatorRegistries.commercial, wiring.relationshipEnding)},
			}
		},
	})
}

// transactionalPartyRelationshipEnding 把一次结束包进一笔事务：取最新修订与落下一笔看同一份快照；替代的接替关系与原关系的
// 终止修订同笔落库，用例交回 error 时整笔回滚，不留「接替关系立了、原关系还生效」的半份。
type transactionalPartyRelationshipEnding struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.PartyRelationshipEndingHandler
}

var _ commercialhttp.PartyRelationshipEndingRegistrar = transactionalPartyRelationshipEnding{}

func (ending transactionalPartyRelationshipEnding) Revoke(
	ctx context.Context,
	command commercialapp.RevokePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, ending.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return ending.inner.Revoke(txCtx, command)
		})
}

func (ending transactionalPartyRelationshipEnding) Expire(
	ctx context.Context,
	command commercialapp.ExpirePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, ending.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return ending.inner.Expire(txCtx, command)
		})
}

func (ending transactionalPartyRelationshipEnding) Supersede(
	ctx context.Context,
	command commercialapp.SupersedePartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, ending.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return ending.inner.Supersede(txCtx, command)
		})
}

func buildCommercialRelationshipEnding(db *bentopg.DB) (commercialhttp.PartyRelationshipEndingRegistrar, error) {
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: party identity registry for relationship endings: %w", err)
	}
	return transactionalPartyRelationshipEnding{
		transactor: db.Transactor(),
		inner:      commercialapp.NewPartyRelationshipEndingHandler(identities, systemClock{}),
	}, nil
}
