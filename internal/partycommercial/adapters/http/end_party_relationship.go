package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 参与方关系的三个结束口（票 ps-external-mark-relations/17）：撤销 `/commercial-party-relationship-revocations`、到期
// `/commercial-party-relationship-expirations`、替代 `/commercial-party-relationship-supersessions`。三口与身份族同属登记册
// 配置写（提交方是管理参与方册的操作者），挂操作者渠道的登记册 Intake；载荷与受控 CLI end-party-relationships 的一项逐字
// 同形（registrationjson.PartyRelationshipEndingBatchDocument），在线口各收自己那一节恰一项。
//
// 三种结束分三口而不合一口带动作字段：三种在册上落成不同的终止状态，依据的含义也不同（撤销凭的是决定、到期凭的是期满、
// 替代另起一段接替关系），合一口之后载荷里会同时躺着动作与三种结束各自的格，登记方读不出自己落下的是哪一种。也不并进
// 停用口：关系的终止走撤销/到期/替代，不叫停用（DeactivatePartyIdentityCommand 注释）。

// PartyRelationshipRevocationIntake 把一次已认证的接入请求翻译成撤销命令。租户格只来自认证结果，同本包其余 Intake。
type PartyRelationshipRevocationIntake interface {
	IntakePartyRelationshipRevocation(ctx context.Context, request *http.Request) (application.RevokePartyRelationshipCommand, error)
}

// PartyRelationshipExpirationIntake 同上，翻译一次到期登记。
type PartyRelationshipExpirationIntake interface {
	IntakePartyRelationshipExpiration(ctx context.Context, request *http.Request) (application.ExpirePartyRelationshipCommand, error)
}

// PartyRelationshipSupersessionIntake 同上，翻译一次替代。
type PartyRelationshipSupersessionIntake interface {
	IntakePartyRelationshipSupersession(ctx context.Context, request *http.Request) (application.SupersedePartyRelationshipCommand, error)
}

// PartyRelationshipEndingRegistrar 是三口转交的编排：Intake 逐口、编排一个用例对象一套答案代数（同 PartyIdentityRegistrar）。
// 事务边界在编排侧给出；替代的两笔必须在同一事务里，用例交回 error 即整笔回滚。
type PartyRelationshipEndingRegistrar interface {
	Revoke(ctx context.Context, command application.RevokePartyRelationshipCommand) (application.PartyRegistryResult, error)
	Expire(ctx context.Context, command application.ExpirePartyRelationshipCommand) (application.PartyRegistryResult, error)
	Supersede(ctx context.Context, command application.SupersedePartyRelationshipCommand) (application.PartyRegistryResult, error)
}

var _ PartyRelationshipEndingRegistrar = (*application.PartyRelationshipEndingHandler)(nil)

var (
	_ PartyRelationshipRevocationIntake   = (*OperatorRegistryIntake)(nil)
	_ PartyRelationshipExpirationIntake   = (*OperatorRegistryIntake)(nil)
	_ PartyRelationshipSupersessionIntake = (*OperatorRegistryIntake)(nil)
)

// NewRevokePartyRelationshipEndpoint 交回撤销的 HTTP 入口。答复照身份族：落下终止修订答 201 `ENDED`，其余治理答案 200。
func NewRevokePartyRelationshipEndpoint(intake PartyRelationshipRevocationIntake, registrar PartyRelationshipEndingRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakePartyRelationshipRevocation, registrar.Revoke, writePartyRegistryAnswer)
}

// NewExpirePartyRelationshipEndpoint 交回到期登记的 HTTP 入口。
func NewExpirePartyRelationshipEndpoint(intake PartyRelationshipExpirationIntake, registrar PartyRelationshipEndingRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakePartyRelationshipExpiration, registrar.Expire, writePartyRegistryAnswer)
}

// NewSupersedePartyRelationshipEndpoint 交回替代的 HTTP 入口。
func NewSupersedePartyRelationshipEndpoint(intake PartyRelationshipSupersessionIntake, registrar PartyRelationshipEndingRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakePartyRelationshipSupersession, registrar.Supersede, writePartyRegistryAnswer)
}

func (intake *OperatorRegistryIntake) IntakePartyRelationshipRevocation(ctx context.Context, request *http.Request) (application.RevokePartyRelationshipCommand, error) {
	var none application.RevokePartyRelationshipCommand
	tenant, document, err := authenticateAndDecode[registrationjson.PartyRelationshipEndingBatchDocument](ctx, intake, request, endingBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("revocations", len(document.Revocations), document.ItemCount()); err != nil {
		return none, err
	}
	return malformedUnless(registrationjson.PartyRelationshipRevocationCommand(tenant, document.Revocations[0]))
}

func (intake *OperatorRegistryIntake) IntakePartyRelationshipExpiration(ctx context.Context, request *http.Request) (application.ExpirePartyRelationshipCommand, error) {
	var none application.ExpirePartyRelationshipCommand
	tenant, document, err := authenticateAndDecode[registrationjson.PartyRelationshipEndingBatchDocument](ctx, intake, request, endingBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("expirations", len(document.Expirations), document.ItemCount()); err != nil {
		return none, err
	}
	return malformedUnless(registrationjson.PartyRelationshipExpirationCommand(tenant, document.Expirations[0]))
}

func (intake *OperatorRegistryIntake) IntakePartyRelationshipSupersession(ctx context.Context, request *http.Request) (application.SupersedePartyRelationshipCommand, error) {
	var none application.SupersedePartyRelationshipCommand
	tenant, document, err := authenticateAndDecode[registrationjson.PartyRelationshipEndingBatchDocument](ctx, intake, request, endingBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("supersessions", len(document.Supersessions), document.ItemCount()); err != nil {
		return none, err
	}
	return malformedUnless(registrationjson.PartyRelationshipSupersessionCommand(tenant, document.Supersessions[0]))
}

func endingBatchTenant(document registrationjson.PartyRelationshipEndingBatchDocument) []byte {
	return document.TenantID
}
