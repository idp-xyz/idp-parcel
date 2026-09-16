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

// 已成笔的口。每放一口在这里多一行断言、多一个方法，装配点多换一行。
var (
	_ LegalEntityRegistrationIntake       = (*IsolatedPartyIdentityIntake)(nil)
	_ BusinessPartyRegistrationIntake     = (*IsolatedPartyIdentityIntake)(nil)
	_ CustomerAccountRegistrationIntake   = (*IsolatedPartyIdentityIntake)(nil)
	_ PartyRelationshipRegistrationIntake = (*IsolatedPartyIdentityIntake)(nil)
	_ PartyIdentityDeactivationIntake     = (*IsolatedPartyIdentityIntake)(nil)
)

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
	TenantID         json.RawMessage             `json:"tenantId"`
	BusinessParties  []businessPartyDocument     `json:"businessParties"`
	LegalEntities    []legalEntityDocument       `json:"legalEntities"`
	CustomerAccounts []customerAccountDocument   `json:"customerAccounts"`
	Relationships    []partyRelationshipDocument `json:"relationships"`
}

// itemCount 数外壳里全部口的项。一口只收本口的项：别的口的数组在这份外壳里解得开，解得开不等于可以忽略——
// 一份同时带着两口项的载荷投到其中一口，另一口那项会被无声丢掉，登记方以为两样都登了。
func (document partyIdentityBatchDocument) itemCount() int {
	return len(document.BusinessParties) + len(document.LegalEntities) +
		len(document.CustomerAccounts) + len(document.Relationships)
}

// exactlyOne 是五口共用的形状门：本口恰一项、整份外壳也恰一项（即没有别的口的项）。一次一笔——本端点一次只收
// 一项，批走受控 CLI，零项与多项都不是这一口的形状。
func (document partyIdentityBatchDocument) exactlyOne(line string, own int) error {
	if own != 1 {
		return fmt.Errorf("%w: %s must carry exactly one item, got %d", ErrMalformedRequest, line, own)
	}
	if document.itemCount() != 1 {
		return fmt.Errorf("%w: this endpoint only takes %s; items for other lines must be posted to their own endpoints", ErrMalformedRequest, line)
	}
	return nil
}

// 各口的一项与受控 CLI 的一项逐字同形（票 02 表单载荷即镜像它）：两口对同一项译出同一条命令。
// 字段不做首尾空白裁切，与 CLI 一致——裁切是表单那一层已经做过的编码层动作，服务端再做一遍会让 " X" 与 "X" 在
// 这一口成为同一身份、在受控 CLI 却是两个。
type businessPartyDocument struct {
	PartyID       string    `json:"partyId"`
	Name          string    `json:"name"`
	Revision      int       `json:"revision"`
	Basis         string    `json:"basis"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

type legalEntityDocument struct {
	LegalEntityID string    `json:"legalEntityId"`
	PartyID       string    `json:"partyId"`
	Revision      int       `json:"revision"`
	Basis         string    `json:"basis"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

type customerAccountDocument struct {
	AccountID       string    `json:"accountId"`
	CustomerPartyID string    `json:"customerPartyId"`
	Revision        int       `json:"revision"`
	Basis           string    `json:"basis"`
	EffectiveFrom   time.Time `json:"effectiveFrom"`
}

// partyRelationshipDocument 的两个可缺格用指针表达缺席：区间终点缺席即开区间，批准事实缺席即登为候选关系、批准另行
// 形成新修订（application.RelationshipApproval 注释）。缺席不能用零值顶——零时刻在 NewEffectiveInterval 里恰是「无终点」，
// 而一个显式给出的坏时刻也会解成零值，两者就分不开了；所以键在场解不出时刻是形状错，由 json 解码直接拒。
type partyRelationshipDocument struct {
	RelationshipID    string                        `json:"relationshipId"`
	Revision          int                           `json:"revision"`
	Holder            string                        `json:"holder"`
	Counterparty      string                        `json:"counterparty"`
	Role              string                        `json:"role"`
	Scope             string                        `json:"scope"`
	Basis             string                        `json:"basis"`
	EffectiveStartsAt time.Time                     `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time                    `json:"effectiveEndsAt"`
	Approval          *relationshipApprovalDocument `json:"approval"`
}

type relationshipApprovalDocument struct {
	Reference  string    `json:"reference"`
	ApprovedAt time.Time `json:"approvedAt"`
}

// partyRoleFromName 是 domain.PartyRole 封闭集的名称镜像；集合外取值拒收不吸收。与受控 CLI 和 postgres 适配器里的
// 同名镜像各自独立——三处译的是同一个封闭集在各自边界上的外部名，领域包不导出解析函数是刻意的：名字属边界，不属模型。
func partyRoleFromName(raw string) (domain.PartyRole, error) {
	for _, role := range []domain.PartyRole{
		domain.CustomerRole, domain.SupplierRole, domain.CarrierAgentRole,
		domain.ResellerRole, domain.AccountHolderRole,
	} {
		if role.String() == raw {
			return role, nil
		}
	}
	return domain.PartyRoleInvalid, fmt.Errorf("unknown party role %q", raw)
}

// IntakePartyRelationshipRegistration 把一次管理台登记译成参与方关系修订登记命令。正文沿 domain.PartyRelationshipSpec，
// 不在这里抄第二份字段清单；带批准事实的登记在用例侧走真转换（候选 → 批准），本 Intake 不构造任何领域对象之外的判断。
func (intake *IsolatedPartyIdentityIntake) IntakePartyRelationshipRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterPartyRelationshipCommand, error) {
	none := application.RegisterPartyRelationshipCommand{}
	document, err := intake.decodeBatch(request)
	if err != nil {
		return none, err
	}
	if err := document.exactlyOne("relationships", len(document.Relationships)); err != nil {
		return none, err
	}
	item := document.Relationships[0]
	id, err := domain.NewRelationshipID(item.RelationshipID)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].relationshipId: %v", ErrMalformedRequest, err)
	}
	holder, err := domain.NewPartyID(item.Holder)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].holder: %v", ErrMalformedRequest, err)
	}
	counterparty, err := domain.NewPartyID(item.Counterparty)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].counterparty: %v", ErrMalformedRequest, err)
	}
	role, err := partyRoleFromName(item.Role)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].role: %v", ErrMalformedRequest, err)
	}
	scope, err := domain.NewCommercialScopeReference(item.Scope)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].scope: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewRelationshipBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].basis: %v", ErrMalformedRequest, err)
	}
	endsAt := time.Time{}
	if item.EffectiveEndsAt != nil {
		endsAt = *item.EffectiveEndsAt
	}
	interval, err := domain.NewEffectiveInterval(item.EffectiveStartsAt, endsAt)
	if err != nil {
		return none, fmt.Errorf("%w: relationships[0].effectiveStartsAt/effectiveEndsAt: %v", ErrMalformedRequest, err)
	}
	command := application.RegisterPartyRelationshipCommand{
		Tenant:   intake.tenant,
		ID:       id,
		Revision: item.Revision,
		Spec: domain.PartyRelationshipSpec{
			Holder:       holder,
			Counterparty: counterparty,
			Role:         role,
			Scope:        scope,
			Basis:        basis,
			Effective:    interval,
		},
	}
	if item.Approval != nil {
		reference, err := domain.NewApprovalReference(item.Approval.Reference)
		if err != nil {
			return none, fmt.Errorf("%w: relationships[0].approval.reference: %v", ErrMalformedRequest, err)
		}
		command.Approval = &application.RelationshipApproval{
			Reference:  reference,
			ApprovedAt: item.Approval.ApprovedAt,
		}
	}
	return command, nil
}

// IntakeCustomerAccountRegistration 把一次管理台登记译成货主客户账户修订登记命令。跨租户绑定由领域构造门在用例侧
// 拒（ADR-0041），这里只译不判；判据同法人口。
func (intake *IsolatedPartyIdentityIntake) IntakeCustomerAccountRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterCustomerAccountCommand, error) {
	none := application.RegisterCustomerAccountCommand{}
	document, err := intake.decodeBatch(request)
	if err != nil {
		return none, err
	}
	if err := document.exactlyOne("customerAccounts", len(document.CustomerAccounts)); err != nil {
		return none, err
	}
	item := document.CustomerAccounts[0]
	account, err := domain.NewCustomerAccountID(item.AccountID)
	if err != nil {
		return none, fmt.Errorf("%w: customerAccounts[0].accountId: %v", ErrMalformedRequest, err)
	}
	party, err := domain.NewPartyID(item.CustomerPartyID)
	if err != nil {
		return none, fmt.Errorf("%w: customerAccounts[0].customerPartyId: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: customerAccounts[0].basis: %v", ErrMalformedRequest, err)
	}
	return application.RegisterCustomerAccountCommand{
		Tenant:        intake.tenant,
		Account:       account,
		CustomerParty: party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}, nil
}

// IntakeBusinessPartyRegistration 把一次管理台登记译成业务参与方身份修订登记命令。参与方本体（租户 + 标识 + 名称）
// 在这里由领域构造函数立起来，租户格取注入值；判据同法人口。
func (intake *IsolatedPartyIdentityIntake) IntakeBusinessPartyRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterBusinessPartyCommand, error) {
	none := application.RegisterBusinessPartyCommand{}
	document, err := intake.decodeBatch(request)
	if err != nil {
		return none, err
	}
	if err := document.exactlyOne("businessParties", len(document.BusinessParties)); err != nil {
		return none, err
	}
	item := document.BusinessParties[0]
	partyID, err := domain.NewPartyID(item.PartyID)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0].partyId: %v", ErrMalformedRequest, err)
	}
	name, err := domain.NewPartyName(item.Name)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0].name: %v", ErrMalformedRequest, err)
	}
	party, err := domain.NewBusinessParty(intake.tenant, partyID, name)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0]: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0].basis: %v", ErrMalformedRequest, err)
	}
	return application.RegisterBusinessPartyCommand{
		Party:         party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}, nil
}

// IntakeLegalEntityRegistration 把一次管理台登记译成责任法人身份修订登记命令。
//
// 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好，判据同 ADR-0029）；修订是否连续、参与方是否在册且
// 届时已生效，一律送进用例让它答`未受理`——那是登记册对这次登记的判断，不是请求的形状问题（票 02 红线「表单不裁
// 任何门」，服务端这一层同样不在 Intake 里预判）。
func (intake *IsolatedPartyIdentityIntake) IntakeLegalEntityRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterLegalEntityCommand, error) {
	none := application.RegisterLegalEntityCommand{}
	document, err := intake.decodeBatch(request)
	if err != nil {
		return none, err
	}
	if err := document.exactlyOne("legalEntities", len(document.LegalEntities)); err != nil {
		return none, err
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

// decodeBatch 解登记外壳并执行自报租户那道拒。
func (intake *IsolatedPartyIdentityIntake) decodeBatch(request *http.Request) (partyIdentityBatchDocument, error) {
	document, err := decodeClosedDocument[partyIdentityBatchDocument](request)
	if err != nil {
		return partyIdentityBatchDocument{}, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return partyIdentityBatchDocument{}, err
	}
	return document, nil
}

// decodeClosedDocument 以封闭形状解载荷，判据与同包发布口共用 decodeStrict：未知键拒——这一口的形状是封闭的，
// 多出来的键不是可以忽略的噪声，多半是登记方把 CLI 文档的别的格（或别的口的项）误投到了这里；尾随的第二个 JSON
// 值也拒——一份载荷只许一个文档，放过它就是无声丢掉一段输入，与 exactlyOne 反对的是同一件事。两道助手若各写一份，
// 同一个包会对同一形状给出两种答复（票 admin-web-group-legal-entities/07 收的就是这道分叉）。
func decodeClosedDocument[Document any](request *http.Request) (Document, error) {
	var document Document
	if err := decodeStrict(request.Body, &document); err != nil {
		var none Document
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return document, nil
}

// refuseSelfReportedTenant 是两份外壳共用的那道拒：键在场即拒，不看值（理由在 partyIdentityBatchDocument.TenantID）。
func refuseSelfReportedTenant(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%w: tenantId must not be carried in the payload; the tenant grid is filled by the access channel, not by the request",
		ErrMalformedRequest,
	)
}

// deactivationBatchDocument 是停用口的外壳，与登记外壳分开：镜像受控 CLI `deactivate-party-identity` 自己那份文档
// （去掉整批的 tenantId）。合成一份外壳会让登记项与停用项躺在同一个文档里，而两者在册上是不同方向的修订。
type deactivationBatchDocument struct {
	TenantID      json.RawMessage        `json:"tenantId"`
	Deactivations []deactivationDocument `json:"deactivations"`
}

type deactivationDocument struct {
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	Revision int       `json:"revision"`
	Basis    string    `json:"basis"`
	At       time.Time `json:"at"`
}

// identityKindFromName 是 application.PartyIdentityKind 封闭三值的名称镜像；关系不在内——关系的终止走撤销/到期/替代，
// 不叫停用（DeactivatePartyIdentityCommand 注释）。判据同 partyRoleFromName。
func identityKindFromName(raw string) (application.PartyIdentityKind, error) {
	for _, kind := range []application.PartyIdentityKind{
		application.BusinessPartyIdentity,
		application.LegalEntityIdentity,
		application.CustomerAccountIdentity,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return application.PartyIdentityKindInvalid, fmt.Errorf("unknown party identity kind %q", raw)
}

// IntakePartyIdentityDeactivation 把一次管理台停用译成身份停用命令。Revision 是操作者声明自己看到的册面（= 最新修订 + 1），
// 错位说明册面已被并发推进或意图已陈旧，由用例拒而不是替操作者猜；这里只译。身份标识照原样过线（命令的 ID 是字符串，
// 由用例按 Kind 分派到对应册的值对象）。
func (intake *IsolatedPartyIdentityIntake) IntakePartyIdentityDeactivation(
	_ context.Context,
	request *http.Request,
) (application.DeactivatePartyIdentityCommand, error) {
	none := application.DeactivatePartyIdentityCommand{}
	document, err := decodeClosedDocument[deactivationBatchDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return none, err
	}
	if len(document.Deactivations) != 1 {
		return none, fmt.Errorf("%w: deactivations must carry exactly one item, got %d", ErrMalformedRequest, len(document.Deactivations))
	}
	item := document.Deactivations[0]
	kind, err := identityKindFromName(item.Kind)
	if err != nil {
		return none, fmt.Errorf("%w: deactivations[0].kind: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: deactivations[0].basis: %v", ErrMalformedRequest, err)
	}
	return application.DeactivatePartyIdentityCommand{
		Tenant:   intake.tenant,
		Kind:     kind,
		ID:       item.ID,
		Revision: item.Revision,
		Basis:    basis,
		At:       item.At,
	}, nil
}
