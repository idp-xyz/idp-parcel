package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/refdata/refdatahttp"
)

// referenceDataHandlers 是基础数据目录三口（ADR-0335 决定六）装好的处理器。
type referenceDataHandlers struct {
	countries  http.Handler
	currencies http.Handler
	units      http.Handler
}

// buildReferenceData 装基础数据目录三口：与操作者绑定、作业设备册同一个认证件——主数据与运营查阅读，不判准入范围，
// 租户只取自信封。目录在 platform 下、不能导入 accessidentity，所以把共享接入身份的答复格译成 refdatahttp 的格
// 这一步放在组合根，同各上下文 adapters/accessidentity 的做法。
func buildReferenceData(minter *accessidentity.OperatorMinter) (referenceDataHandlers, error) {
	if minter == nil {
		return referenceDataHandlers{}, errors.New("reference data needs a minter")
	}
	catalogue, err := identityhttp.NewOperatorCatalogueAuthenticator(minter)
	if err != nil {
		return referenceDataHandlers{}, err
	}
	authenticator := referenceDataAuthenticator{catalogue: catalogue}
	countries, err := refdatahttp.NewCountriesEndpoint(authenticator)
	if err != nil {
		return referenceDataHandlers{}, err
	}
	currencies, err := refdatahttp.NewCurrenciesEndpoint(authenticator)
	if err != nil {
		return referenceDataHandlers{}, err
	}
	units, err := refdatahttp.NewUnitsEndpoint(authenticator)
	if err != nil {
		return referenceDataHandlers{}, err
	}
	return referenceDataHandlers{countries: countries, currencies: currencies, units: units}, nil
}

// referenceDataAuthenticator 实现 refdatahttp.OperationsReadAuthenticator：认证交给共享的操作者查阅认证件，
// 失败译成目录读口认的四格。
type referenceDataAuthenticator struct {
	catalogue identityhttp.OperatorCatalogueAuthenticator
}

var _ refdatahttp.OperationsReadAuthenticator = referenceDataAuthenticator{}

func (authenticator referenceDataAuthenticator) AuthenticateOperationsRead(ctx context.Context, bearerToken string) (string, error) {
	tenant, err := authenticator.catalogue.AuthenticateCatalogueRead(ctx, bearerToken)
	if err != nil {
		return "", answerReferenceDataAuthentication(err)
	}
	return tenant, nil
}

func answerReferenceDataAuthentication(err error) error {
	switch {
	case errors.Is(err, accessidentity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", refdatahttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, accessidentity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", refdatahttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, accessidentity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", refdatahttp.ErrOperatorNotGranted, err)
	case errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable),
		errors.Is(err, accessidentity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", refdatahttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
