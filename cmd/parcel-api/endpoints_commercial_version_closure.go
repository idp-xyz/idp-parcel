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

type commercialVersionClosureWiring struct {
	versionClosure commercialhttp.VersionClosureRegistrar
}

func init() {
	registerEndpoints(endpointFeature[commercialVersionClosureWiring]{
		lines: map[string]endpointLine{
			"/commercial-version-successor-cuts": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-version-retirements":    {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (commercialVersionClosureWiring, error) {
			registrar, err := buildCommercialVersionClosure(sources.db)
			return commercialVersionClosureWiring{versionClosure: registrar}, err
		},
		mount: func(channels endpointChannels, wiring commercialVersionClosureWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 按后继收尾旧版两口（ADR-0233）与商业发布同挂操作者渠道的登记册 Intake：它们改的是解析在同一对象两版之间选
				// 哪一版，提交方与授予都与发布同一份（ADR-0151 的定族判据）；写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/commercial-version-successor-cuts", Handler: commercialhttp.NewCutCommercialVersionAtSuccessorEndpoint(channels.operatorRegistries.commercial, wiring.versionClosure)},
				{Pattern: "/commercial-version-retirements", Handler: commercialhttp.NewRetireCommercialVersionAtSameStartEndpoint(channels.operatorRegistries.commercial, wiring.versionClosure)},
			}
		},
	})
}

// transactionalCommercialVersionClosure 把一次收尾包进一笔事务：锁旧版行、读整册与写更正或退役必须在同一笔里，锁才
// 守得住收尾的判据（ports.VersionClosureRegistry）。编排返回 error 时整笔回滚。
type transactionalCommercialVersionClosure struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.CloseCommercialVersionHandler
}

var _ commercialhttp.VersionClosureRegistrar = transactionalCommercialVersionClosure{}

func (closure transactionalCommercialVersionClosure) Cut(
	ctx context.Context,
	command commercialapp.CutCommercialVersionAtSuccessorCommand,
) (commercialapp.VersionClosureResult, error) {
	return commercialInTransaction(ctx, closure.transactor,
		func(txCtx context.Context) (commercialapp.VersionClosureResult, error) {
			return closure.inner.Cut(txCtx, command)
		})
}

func (closure transactionalCommercialVersionClosure) RetireAtSameStart(
	ctx context.Context,
	command commercialapp.RetireCommercialVersionAtSameStartCommand,
) (commercialapp.VersionClosureResult, error) {
	return commercialInTransaction(ctx, closure.transactor,
		func(txCtx context.Context) (commercialapp.VersionClosureResult, error) {
			return closure.inner.RetireAtSameStart(txCtx, command)
		})
}

func buildCommercialVersionClosure(db *bentopg.DB) (commercialhttp.VersionClosureRegistrar, error) {
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: commercial publication registry for version closures: %w", err)
	}
	return transactionalCommercialVersionClosure{
		transactor: db.Transactor(),
		inner:      commercialapp.NewCloseCommercialVersionHandler(publications, systemClock{}),
	}, nil
}
