package main

import (
	"errors"
	"net/http"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// 操作者接入管理面各口的装配（ADR-0363 决定二）：与操作者绑定、授予自查同一只铸造器，守口换成操作者接入管理这一格，不判准入
// 范围——登记操作者与授予不是生产写。每个写口各自在框架事务里登记，答复代数与受控命令行一致（同一个 OperatorRegistrar）。

// buildOperatorCatalogue 装本租户操作者册的查阅读。issuer 是本进程信任的发行方，回给管理台预填；渠道未配置时为空，那时整族
// 答未配置、没有人读得到它，所以这里不拒空串。
func buildOperatorCatalogue(minter *accessidentity.OperatorMinter, catalogue accessidentity.OperatorCatalogue, issuer string) (http.Handler, error) {
	if minter == nil || catalogue == nil {
		return nil, errors.New("operator catalogue needs a minter and an operator catalogue")
	}
	authenticator, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewQueryOperatorsEndpoint(authenticator, catalogue, issuer)
}

func buildOperatorRegistration(minter *accessidentity.OperatorMinter, transactor bentoapp.Transactor, registrar accessidentity.OperatorRegistrar) (http.Handler, error) {
	if minter == nil || transactor == nil || registrar == nil {
		return nil, errors.New("operator registration needs a minter, a transactor and an operator registrar")
	}
	authenticator, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewRegisterOperatorEndpoint(authenticator, transactor, registrar)
}

func buildOperatorGrantRegistration(minter *accessidentity.OperatorMinter, transactor bentoapp.Transactor, registrar accessidentity.OperatorRegistrar) (http.Handler, error) {
	if minter == nil || transactor == nil || registrar == nil {
		return nil, errors.New("operator grant registration needs a minter, a transactor and an operator registrar")
	}
	authenticator, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewRegisterOperatorGrantEndpoint(authenticator, transactor, registrar)
}

// buildOperatorGrantRevocation 的 catalogue 给写入前的防锁死判断取那笔授予（ADR-0363 决定三）；「此刻」取进程时钟。
func buildOperatorGrantRevocation(
	minter *accessidentity.OperatorMinter,
	transactor bentoapp.Transactor,
	registrar accessidentity.OperatorRegistrar,
	catalogue accessidentity.OperatorCatalogue,
) (http.Handler, error) {
	if minter == nil || transactor == nil || registrar == nil || catalogue == nil {
		return nil, errors.New("operator grant revocation needs a minter, a transactor, an operator registrar and an operator catalogue")
	}
	authenticator, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewRegisterOperatorGrantRevocationEndpoint(authenticator, transactor, registrar, catalogue, time.Now)
}
