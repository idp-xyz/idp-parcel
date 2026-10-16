package main

import (
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	ccaccess "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	nraccess "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
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
// 准入范围由 buildAdmissionScope 给，与集成客户端铸造器同一只（ADR-0180）：本产品的权威串没设时是未配置那一只，一切运营
// 决定答「不在准入范围」；设了是「对照 + pilot-governance 的 AuthorityCoverage」桥，对照没登的租户照旧答不在准入范围——
// 首个租户登记区间之前任何生产写都不在准入范围，是 ADR-0149 越权风险点 3 写明的设计。登记写面的请求不带准入要求，这一格
// 对它们不起作用。发行方参数没设时核验方就是未配置那一只，各口照旧答 ACCESS_CHANNEL_NOT_CONFIGURED，与换口之前逐字节相同。
func buildOperatorMinter(
	verifier accessidentity.OperatorCredentialVerifier,
	registry accessidentity.OperatorRegistry,
	admission accessidentity.AdmissionScope,
) (*accessidentity.OperatorMinter, error) {
	if verifier == nil || registry == nil || admission == nil {
		return nil, errors.New("operator minter needs a verifier, an operator registry and an admission scope")
	}
	return accessidentity.NewOperatorMinter(verifier, registry, admission, time.Now)
}

// frontlineMovementSource 是一线作业端登移动事实时的来源格：自营，由作业事实登记这一面说，不从载荷收。
const frontlineMovementSource = "SELF_OPERATED"

func buildWorkFactIntakes(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (nodeopshttp.ReceptionIntake, *tfhttp.WorkFactCommandIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, nil, err
	}
	transportAuthenticator, err := tfaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		return nil, nil, err
	}
	transport, err := tfhttp.NewWorkFactCommandIntake(transportAuthenticator, frontlineMovementSource)
	if err != nil {
		return nil, nil, err
	}
	receptionAuthenticator, err := noaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		return nil, nil, err
	}
	reception, err := nodeopshttp.NewWorkFactReceptionIntake(receptionAuthenticator)
	if err != nil {
		return nil, nil, err
	}
	return reception, transport, nil
}

// buildWorkDeviceCatalogue 装作业设备册查阅：主数据与运营查阅读，不判准入范围。设备册就是登记用的那一本。
func buildWorkDeviceCatalogue(minter *accessidentity.OperatorMinter, devices accessidentity.DeviceCatalogue) (http.Handler, error) {
	if minter == nil || devices == nil {
		return nil, errors.New("work device catalogue needs a minter and a device catalogue")
	}
	authenticator, err := identityhttp.NewOperatorCatalogueAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewQueryWorkDevicesEndpoint(authenticator, devices)
}

// buildOperatorDecisionIntakes 装两边运营决定口的防腐认证方与 Intake。委托侧按委托标识寻址的三口用 targets，受控关闭与
// 重开两口按包裹寻址、用 parcels（ADR-0183 决定一）。
func buildOperatorDecisionIntakes(
	minter *accessidentity.OperatorMinter,
	targets psports.OperatorDecisionTargets,
	parcels psports.CurrentAcceptedParcelTargetView,
) (operatorDecisionIntakes, error) {
	if minter == nil || targets == nil || parcels == nil {
		return operatorDecisionIntakes{}, errors.New("operator decision intakes need a minter, a shipment request lookup and a parcel lookup")
	}
	shipmentAuthenticator, err := psaccess.NewOperatorAuthenticator(minter)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	transportAuthenticator, err := tfaccess.NewOperatorAuthenticator(minter)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	shipment, err := shipmenthttp.NewOperatorDecisionIntake(shipmentAuthenticator, targets, parcels)
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
	transport  *tfhttp.OperatorRegistryIntake
	// pricingOperationsRead 不是登记写面：试算与价卡目录查阅挂主数据与运营查阅读（ADR-0152 决定六、ADR-0077；
	// 票 operator-channel/06、05）。网络、关务、商业的目录查阅同这一面。与登记写面各口共用同一只铸造器。
	pricingOperationsRead    *pricinghttp.OperatorOperationsReadIntake
	networkOperationsRead    *networkhttp.OperatorOperationsReadIntake
	customsOperationsRead    *customshttp.OperatorOperationsReadIntake
	commercialOperationsRead *commercialhttp.OperatorOperationsReadIntake
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
	pricingReadAuthenticator, err := ppaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	pricingOperationsRead, err := pricinghttp.NewOperatorOperationsReadIntake(pricingReadAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	networkReadAuthenticator, err := nraccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	networkOperationsRead, err := networkhttp.NewOperatorOperationsReadIntake(networkReadAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	customsReadAuthenticator, err := ccaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	customsOperationsRead, err := customshttp.NewOperatorOperationsReadIntake(customsReadAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	commercialReadAuthenticator, err := pcaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	commercialOperationsRead, err := commercialhttp.NewOperatorOperationsReadIntake(commercialReadAuthenticator)
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
	transportAuthenticator, err := tfaccess.NewOperatorRegistryAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	transport, err := tfhttp.NewOperatorRegistryIntake(transportAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	return operatorRegistryIntakes{
		visibility: visibility, customs: customs, network: network, pricing: pricing, commercial: commercial, transport: transport,
		pricingOperationsRead:    pricingOperationsRead,
		networkOperationsRead:    networkOperationsRead,
		customsOperationsRead:    customsOperationsRead,
		commercialOperationsRead: commercialOperationsRead,
	}, nil
}
