package main

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// buildOperatorGrants 装授予自查（ADR-0315）：与操作者绑定同一个铸造器、同一面守口——主数据与运营查阅读，不判准入范围，
// 答的只是信封上铸造那一刻生效的授予。
func buildOperatorGrants(minter *accessidentity.OperatorMinter) (http.Handler, error) {
	if minter == nil {
		return nil, errors.New("operator grants needs a minter")
	}
	authenticator, err := identityhttp.NewOperatorGrantsAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewQueryOperatorGrantsEndpoint(authenticator)
}
