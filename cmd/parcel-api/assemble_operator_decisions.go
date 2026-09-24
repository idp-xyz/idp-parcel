package main

import (
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// operatorDecisionIntakes 是操作者渠道「运营决定」能力面在两个上下文的 Intake（ADR-0151；票 operator-channel/15）。
type operatorDecisionIntakes struct {
	shipment  *shipmenthttp.OperatorDecisionIntake
	transport *tfhttp.OperatorDecisionIntake
}

// buildOperatorDecisionIntakes 用一只操作者铸造器装两边的防腐认证方与 Intake。
//
// 准入范围取 UnconfiguredAdmissionScope：租户到治理坐标（对象范围 × 能力 × 事实类型）的对照是租户随试点登记的实例
// 半边，还没有任何租户登过，一切运营决定答「不在准入范围」——首个租户登记区间之前任何生产写都不在准入范围，是
// ADR-0149 越权风险点 3 写明的设计。对照登进来的那一笔，把这里换成「对照 + pilot-governance 的 AuthorityCoverage」桥。
// 发行方参数没设时核验方就是未配置那一只，各口照旧答 ACCESS_CHANNEL_NOT_CONFIGURED，与换口之前逐字节相同。
func buildOperatorDecisionIntakes(
	verifier accessidentity.OperatorCredentialVerifier,
	registry accessidentity.OperatorRegistry,
	targets psports.OperatorDecisionTargets,
) (operatorDecisionIntakes, error) {
	if verifier == nil || registry == nil || targets == nil {
		return operatorDecisionIntakes{}, errors.New("operator decision intakes need a verifier, an operator registry and a shipment request lookup")
	}
	minter, err := accessidentity.NewOperatorMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
	if err != nil {
		return operatorDecisionIntakes{}, err
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
