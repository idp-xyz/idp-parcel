package main

import (
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	craccess "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/accessidentity"
	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
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
	pgaccess "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/accessidentity"
	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	settlementaccess "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// operatorDecisionIntakes 是操作者渠道「运营决定」能力面在各上下文的 Intake（ADR-0151；票 operator-channel/15）。nodeOperations
// 是复核页人工重核待识别实物（ADR-0199 决定三；票 no-reception-source/03）。
type operatorDecisionIntakes struct {
	shipment       *shipmenthttp.OperatorDecisionIntake
	transport      *tfhttp.OperatorDecisionIntake
	nodeOperations *nodeopshttp.OperatorDecisionIntake
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

// buildOperatorBinding 装操作者绑定查阅（票 group-legal-entities-uiux/11）：与作业设备册同一个认证件——主数据与运营查阅读，
// 不判准入范围，答的只是信封上的绑定租户。
func buildOperatorBinding(minter *accessidentity.OperatorMinter) (http.Handler, error) {
	if minter == nil {
		return nil, errors.New("operator binding needs a minter")
	}
	authenticator, err := identityhttp.NewOperatorCatalogueAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return identityhttp.NewQueryOperatorBindingEndpoint(authenticator)
}

// buildOperatorDecisionIntakes 装 operatorDecisionIntakes 各格的防腐认证方与 Intake。shipmenthttp.OperatorDecisionIntake 按
// 委托标识寻址时用 targets，受控关闭与重开按包裹寻址、用 parcels（ADR-0183 决定一）。
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
	nodeOperationsAuthenticator, err := noaccess.NewOperatorDecisionAuthenticator(minter)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	nodeOperations, err := nodeopshttp.NewOperatorDecisionIntake(nodeOperationsAuthenticator)
	if err != nil {
		return operatorDecisionIntakes{}, err
	}
	return operatorDecisionIntakes{shipment: shipment, transport: transport, nodeOperations: nodeOperations}, nil
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
	// 运营查阅面余下各族同一面（票 operator-channel/35）。面单交易与渠道择优决定两口共用委托上下文这一只；委托查阅及其子集视图
	// 不在其中——它们的作用域按客户账户切，操作者信封给不出（票 operator-channel/53）。
	shipmentOperationsRead   *shipmenthttp.OperatorOperationsReadIntake
	nodeOperationsRead       *nodeopshttp.OperatorOperationsReadIntake
	transportOperationsRead  *tfhttp.OperatorOperationsReadIntake
	visibilityOperationsRead *visibilityhttp.OperatorOperationsReadIntake
	collectionOperationsRead *collectionhttp.OperatorOperationsReadIntake
	settlementOperationsRead *settlementhttp.OperatorOperationsReadIntake
	governanceOperationsRead *governancehttp.OperatorOperationsReadIntake
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
	reads, err := buildOperatorOperationsReads(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	reads.visibility, reads.customs, reads.network, reads.pricing, reads.commercial, reads.transport =
		visibility, customs, network, pricing, commercial, transport
	reads.pricingOperationsRead = pricingOperationsRead
	reads.networkOperationsRead = networkOperationsRead
	reads.customsOperationsRead = customsOperationsRead
	reads.commercialOperationsRead = commercialOperationsRead
	return reads, nil
}

// buildOperatorOperationsReads 装运营查阅面余下各族的操作者 Intake（票 operator-channel/35）：每族一只认证方加一只
// Intake，与目录查阅各族同一面、同一只铸造器。
func buildOperatorOperationsReads(minter *accessidentity.OperatorMinter) (operatorRegistryIntakes, error) {
	shipmentAuthenticator, err := psaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	shipment, err := shipmenthttp.NewOperatorOperationsReadIntake(shipmentAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	nodeAuthenticator, err := noaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	node, err := nodeopshttp.NewOperatorOperationsReadIntake(nodeAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	transportAuthenticator, err := tfaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	transport, err := tfhttp.NewOperatorOperationsReadIntake(transportAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	visibilityAuthenticator, err := veaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	visibility, err := visibilityhttp.NewOperatorOperationsReadIntake(visibilityAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	collectionAuthenticator, err := craccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	collection, err := collectionhttp.NewOperatorOperationsReadIntake(collectionAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	settlementAuthenticator, err := settlementaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	settlement, err := settlementhttp.NewOperatorOperationsReadIntake(settlementAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	governanceAuthenticator, err := pgaccess.NewOperatorOperationsReadAuthenticator(minter)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	governance, err := governancehttp.NewOperatorOperationsReadIntake(governanceAuthenticator)
	if err != nil {
		return operatorRegistryIntakes{}, err
	}
	return operatorRegistryIntakes{
		shipmentOperationsRead:   shipment,
		nodeOperationsRead:       node,
		transportOperationsRead:  transport,
		visibilityOperationsRead: visibility,
		collectionOperationsRead: collection,
		settlementOperationsRead: settlement,
		governanceOperationsRead: governance,
	}, nil
}
