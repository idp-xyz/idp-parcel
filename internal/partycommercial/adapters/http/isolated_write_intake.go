package commercialhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// IsolatedPartyIdentityIntake 是隔离写路径准入（ADR-0091）在参与方身份族登记口的注入式放行。
//
// 它与本包的 IsolatedOperationsReadIntake 同层同款：租户格在构造时由装配点给定，进请求路径后不读任何授权输入。
// 与读面不同的是它**会构造命令、命令会落行**——ADR-0078 的「零持久化」论证在这里不成立，可分辨物改由租户维本身
// 的 `SYN-` 前缀承担（ADR-0091 决定三）；前缀门禁、启用与否与启动日志都在 cmd/parcel-api，本类型只收一个立得住的租户。
//
// register_party_identity.go 包注释给真渠道 Intake 定的形状——「把认证结果填进租户格、只从载荷取行内容」——隔离
// 形态照办，认证结果换成开关值。因此载荷里**不许带 tenantId**：受控 CLI 的批文带整批的租户是治理动作（运维在库网内
// 手跑），搬到在线口上就是采信自报租户；值与开关碰巧相同也拒，否则同一份载荷在别的环境里就是穿透 ADR-0003 隔离边界
// 的第一步。
//
// 逐口放行（ADR-0091 Consequences）：本类型只实现已经成笔的那几口的 Intake 接口，未成笔的口在装配点仍挂字面量
// UnconfiguredIntake{}，且在类型上就装不进本类型——每放一口在这里多一个方法，装配点多换一行，两处都看得见。
type IsolatedPartyIdentityIntake struct {
	tenant domain.TenantID
}

var _ LegalEntityRegistrationIntake = (*IsolatedPartyIdentityIntake)(nil)

// NewIsolatedPartyIdentityIntake 由装配点以显式合成值构造。立不起来的租户在这里拒：装配错误要在启动时暴露，
// 不该等到第一个请求。
func NewIsolatedPartyIdentityIntake(tenant string) (*IsolatedPartyIdentityIntake, error) {
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return nil, fmt.Errorf("party commercial http: isolated party identity intake: %w", err)
	}
	return &IsolatedPartyIdentityIntake{tenant: tenantID}, nil
}

// partyIdentityBatchDocument 是身份族在线口的载荷外壳：镜像受控 CLI `register-parties` 的输入文档，去掉整批的 tenantId。
//
// TenantID 一格**只为拒而存在**：DisallowUnknownFields 本来也会把它当未知字段拒掉，但那样的拒绝理由是「未知字段」，
// 登记方读不出这是一条规则而不是一次拼写错误；且日后有人把租户格加回文档结构时，未知字段那道门会静默放开。这一格用
// json.RawMessage 而不是 *string：`"tenantId": null` 也是自报（键在场），要一并拒。
type partyIdentityBatchDocument struct {
	TenantID      json.RawMessage       `json:"tenantId"`
	LegalEntities []legalEntityDocument `json:"legalEntities"`
}

// legalEntityDocument 与受控 CLI 的一项逐字同形（票 02 表单载荷即镜像它）：两口对同一项译出同一条命令。
// 字段不做首尾空白裁切，与 CLI 一致——裁切是表单那一层已经做过的编码层动作，服务端再做一遍会让 " X" 与 "X" 在
// 这一口成为同一身份、在受控 CLI 却是两个。
type legalEntityDocument struct {
	LegalEntityID string    `json:"legalEntityId"`
	PartyID       string    `json:"partyId"`
	Revision      int       `json:"revision"`
	Basis         string    `json:"basis"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

// IntakeLegalEntityRegistration 把一次管理台登记译成责任法人身份修订登记命令。
//
// 一次一笔：本端点一次只收一项，批走受控 CLI——零项与多项都不是这一口的形状。形状级失败包 ErrMalformedRequest
// （4xx，重发同样内容不会好，判据同 ADR-0029）；修订是否连续、参与方是否在册且届时已生效，一律送进用例让它答
// `未受理`——那是登记册对这次登记的判断，不是请求的形状问题（票 02 红线「表单不裁任何门」，服务端这一层同样不在
// Intake 里预判）。
func (intake *IsolatedPartyIdentityIntake) IntakeLegalEntityRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterLegalEntityCommand, error) {
	none := application.RegisterLegalEntityCommand{}
	document, err := intake.decodeBatch(request)
	if err != nil {
		return none, err
	}
	if len(document.LegalEntities) != 1 {
		return none, fmt.Errorf("%w: legalEntities must carry exactly one item, got %d", ErrMalformedRequest, len(document.LegalEntities))
	}
	item := document.LegalEntities[0]
	entity, err := domain.NewLegalEntityReference(item.LegalEntityID)
	if err != nil {
		return none, fmt.Errorf("%w: legalEntities[0].legalEntityId: %v", ErrMalformedRequest, err)
	}
	party, err := domain.NewPartyID(item.PartyID)
	if err != nil {
		return none, fmt.Errorf("%w: legalEntities[0].partyId: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: legalEntities[0].basis: %v", ErrMalformedRequest, err)
	}
	return application.RegisterLegalEntityCommand{
		Tenant:        intake.tenant,
		Entity:        entity,
		Party:         party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}, nil
}

// decodeBatch 解外壳并执行自报租户那道拒。DisallowUnknownFields：这一口的形状是封闭的，多出来的键不是可以忽略的
// 噪声——它多半是登记方把 CLI 文档的别的格（或别的口的项）误投到了这里。
func (intake *IsolatedPartyIdentityIntake) decodeBatch(request *http.Request) (partyIdentityBatchDocument, error) {
	var document partyIdentityBatchDocument
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return partyIdentityBatchDocument{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if len(document.TenantID) != 0 {
		return partyIdentityBatchDocument{}, fmt.Errorf(
			"%w: tenantId must not be carried in the payload; the tenant grid is filled by the access channel, not by the request",
			ErrMalformedRequest,
		)
	}
	return document, nil
}
