package main

import (
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	ccaccess "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	nraccess "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	ppaccess "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcaccess "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/accessidentity"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// operatorDecisionIntakes 是操作者渠道「运营决定」能力面在两个上下文的 Intake（ADR-0151；票 operator-channel/15）。
type operatorDecisionIntakes struct {
	shipment  *shipmenthttp.OperatorDecisionIntake
	transport *tfhttp.OperatorDecisionIntake
}

// buildOperatorMinter 建本进程唯一一只操作者铸造器：运营决定与登记写面各口共用它。
//
// 准入范围取 UnconfiguredAdmissionScope：租户到治理坐标（对象范围 × 能力 × 事实类型）的对照是租户随试点登记的实例
// 半边，还没有任何租户登过，一切运营决定答「不在准入范围」——首个租户登记区间之前任何生产写都不在准入范围，是
// ADR-0149 越权风险点 3 写明的设计。对照登进来的那一笔，把这里换成「对照 + pilot-governance 的 AuthorityCoverage」桥。
// 登记写面的请求不带准入要求，这一格对它们不起作用。发行方参数没设时核验方就是未配置那一只，各口照旧答
// ACCESS_CHANNEL_NOT_CONFIGURED，与换口之前逐字节相同。
func buildOperatorMinter(verifier accessidentity.OperatorCredentialVerifier, registry accessidentity.OperatorRegistry) (*accessidentity.OperatorMinter, error) {
	if verifier == nil || registry == nil {
		return nil, errors.New("operator minter needs a verifier and an operator registry")
	}
	return accessidentity.NewOperatorMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
}

// buildOperatorDecisionIntakes 装两边运营决定口的防腐认证方与 Intake。
func buildOperatorDecisionIntakes(minter *accessidentity.OperatorMinter, targets psports.OperatorDecisionTargets) (operatorDecisionIntakes, error) {
	if minter == nil || targets == nil {
		return operatorDecisionIntakes{}, errors.New("operator decision intakes need a minter and a shipment request lookup")
	}
	shipmentAuthenticator, err := psaccess.NewOperatorAuthenticator(minter)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	transportAuthenticator, err := tfaccess.NewOperatorAuthenticator(minter)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	shipment, err := shipmenthttp.NewOperatorDecisionIntake(shipmentAuthenticator, targets)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	transport, err := tfhttp.NewOperatorDecisionIntake(transportAuthenticator)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	return operatorDecisionIntakes{shipment: shipment, transport: transport}, nil
}

// operatorRegistryIntakes 是操作者渠道登记册配置写能力面在各上下文的 Intake（ADR-0100 决定四；票 operator-channel/04）。
// 逐上下文补：每补一个上下文，这里多一格、端点表多换几行。
type operatorRegistryIntakes struct {
	visibility *visibilityhttp.OperatorRegistryIntake
	customs    *customshttp.OperatorRegistryIntake
	network    *networkhttp.OperatorRegistryIntake
	pricing    *pricinghttp.OperatorRegistryIntake
	commercial *commercialhttp.OperatorRegistryIntake
}

func buildOperatorRegistryIntakes(minter *accessidentity.OperatorMinter) (operatorRegistryIntakes, error) {
	if minter == nil {
		return operatorRegistryIntakes{}, errors.New("operator registry intakes need a minter")
	}
	visibilityAuthenticator, err := veaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	visibility, err := visibilityhttp.NewOperatorRegistryIntake(visibilityAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	customsAuthenticator, err := ccaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	customs, err := customshttp.NewOperatorRegistryIntake(customsAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	networkAuthenticator, err := nraccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	network, err := networkhttp.NewOperatorRegistryIntake(networkAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	pricingAuthenticator, err := ppaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	pricing, err := pricinghttp.NewOperatorRegistryIntake(pricingAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	commercialAuthenticator, err := pcaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	commercial, err := commercialhttp.NewOperatorRegistryIntake(commercialAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	return operatorRegistryIntakes{visibility: visibility, customs: customs, network: network, pricing: pricing, commercial: commercial}, nil
}
