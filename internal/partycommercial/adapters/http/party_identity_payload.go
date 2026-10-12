package commercialhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 参与方身份族在线口（四类登记、停用与法人资料登记）的载荷外壳与逐项翻译。Intake 方法在 OperatorRegistryIntake 上：先认证、
// 再解外壳、再交这里把一项译成一条命令。register_party_identity.go 包注释给真渠道 Intake 定的形状是「把认证结果填进租户格、
// 只从载荷取行内容」，所以这里每一项的翻译都收调用方给的租户，载荷里不许带租户。

// partyIdentityBatchDocument 是身份族在线口的载荷外壳：镜像受控 CLI `register-parties` 的输入文档，去掉整批的 tenantId。
//
// TenantID 一格**只为拒而存在**：受控 CLI 的批文带整批的租户是治理动作（运维在库网内手跑），搬到在线口上就是采信自报
// 租户；值与认证结果碰巧相同也拒，否则同一份载荷在别的环境里就是穿透 ADR-0003 隔离边界的第一步。DisallowUnknownFields
// 本来也会把它当未知字段拒掉，但那样的拒绝理由是「未知字段」，登记方读不出这是一条规则而不是一次拼写错误；且日后有人把
// 租户格加回文档结构时，未知字段那道门会静默放开。这一格用 json.RawMessage 而不是 *string：`"tenantId": null` 也是自报
// （键在场），要一并拒。
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

func (document partyIdentityBatchDocument) exactlyOne(line string, own int) error {
	return exactlyOneItem(line, own, document.itemCount())
}

// exactlyOneItem 是本包在线登记口共用的形状门：本口恰一项、整份外壳也恰一项（即没有别的口的项）。一次一笔——在线口
// 一次只收一项，批走受控 CLI，零项与多项都不是这一口的形状。total 是外壳里全部口的项数。
func exactlyOneItem(line string, own, total int) error {
	if own != 1 {
		return fmt.Errorf("%w: %s must carry exactly one item, got %d", ErrMalformedRequest, line, own)
	}
	if total != 1 {
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

// legalEntityDocument 的身份层三格（ADR-0145 决定一、二）键可缺：缺即没给，是缺是漏由用例判（新登记缺国家或缺号答
// `未受理`），Intake 只管形状——给了就得是合格的国家形状与非空的类型、号。
type legalEntityDocument struct {
	LegalEntityID               string                           `json:"legalEntityId"`
	PartyID                     string                           `json:"partyId"`
	Revision                    int                              `json:"revision"`
	Basis                       string                           `json:"basis"`
	EffectiveFrom               time.Time                        `json:"effectiveFrom"`
	RegistrationCountry         *string                          `json:"registrationCountry,omitempty"`
	LifetimeRegistrationNumbers []lifetimeRegistrationNumberBody `json:"lifetimeRegistrationNumbers,omitempty"`
	IdentityCorrectionBasis     *string                          `json:"identityCorrectionBasis,omitempty"`
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
// 同名镜像各自独立——各边界各自译同一个封闭集在自己边界上的外部名，领域包不导出解析函数是刻意的：名字属边界，不属模型。
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

// command 把一次管理台登记的关系项译成参与方关系修订登记命令。正文沿 domain.PartyRelationshipSpec，不在这里抄第二份
// 字段清单；带批准事实的登记在用例侧走真转换（候选 → 批准），翻译不构造任何领域对象之外的判断。
func (item partyRelationshipDocument) command(tenant domain.TenantID) (application.RegisterPartyRelationshipCommand, error) {
	none := application.RegisterPartyRelationshipCommand{}
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
		Tenant:   tenant,
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

// command 把一次管理台登记的账户项译成货主客户账户修订登记命令。跨租户绑定由领域构造门在用例侧拒（ADR-0041），
// 这里只译不判；判据同法人项。
func (item customerAccountDocument) command(tenant domain.TenantID) (application.RegisterCustomerAccountCommand, error) {
	none := application.RegisterCustomerAccountCommand{}
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
		Tenant:        tenant,
		Account:       account,
		CustomerParty: party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}, nil
}

// command 把一次管理台登记的参与方项译成业务参与方身份修订登记命令。参与方本体（租户 + 标识 + 名称）在这里由领域
// 构造函数立起来，租户格取调用方给的认证结果；判据同法人项。
func (item businessPartyDocument) command(tenant domain.TenantID) (application.RegisterBusinessPartyCommand, error) {
	none := application.RegisterBusinessPartyCommand{}
	partyID, err := domain.NewPartyID(item.PartyID)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0].partyId: %v", ErrMalformedRequest, err)
	}
	name, err := domain.NewPartyName(item.Name)
	if err != nil {
		return none, fmt.Errorf("%w: businessParties[0].name: %v", ErrMalformedRequest, err)
	}
	party, err := domain.NewBusinessParty(tenant, partyID, name)
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

// command 把一次管理台登记的法人项译成责任法人身份修订登记命令。
//
// 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好，判据同 ADR-0029）；修订是否连续、参与方是否在册且
// 届时已生效，一律送进用例让它答`未受理`——那是登记册对这次登记的判断，不是请求的形状问题（票 02 红线「表单不裁
// 任何门」，服务端这一层同样不在 Intake 里预判）。
func (item legalEntityDocument) command(tenant domain.TenantID) (application.RegisterLegalEntityCommand, error) {
	none := application.RegisterLegalEntityCommand{}
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
	command := application.RegisterLegalEntityCommand{
		Tenant:        tenant,
		Entity:        entity,
		Party:         party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}
	if err := applyLegalEntityIdentityFields(&command, item); err != nil {
		return none, err
	}
	return command, nil
}

// applyLegalEntityIdentityFields 把载荷上的身份层三格译进命令；形状不对包 ErrMalformedRequest，字段名随报。
func applyLegalEntityIdentityFields(command *application.RegisterLegalEntityCommand, item legalEntityDocument) error {
	if item.RegistrationCountry != nil {
		country, err := domain.NewRegistrationCountryCode(*item.RegistrationCountry)
		if err != nil {
			return fmt.Errorf("%w: legalEntities[0].registrationCountry: %v", ErrMalformedRequest, err)
		}
		command.RegistrationCountry = &country
	}
	for index, number := range item.LifetimeRegistrationNumbers {
		typeCode, err := domain.NewRegistrationNumberTypeCode(number.TypeCode)
		if err != nil {
			return fmt.Errorf("%w: legalEntities[0].lifetimeRegistrationNumbers[%d].typeCode: %v", ErrMalformedRequest, index, err)
		}
		value, err := domain.NewRegistrationNumber(number.Number)
		if err != nil {
			return fmt.Errorf("%w: legalEntities[0].lifetimeRegistrationNumbers[%d].number: %v", ErrMalformedRequest, index, err)
		}
		lifetime, err := domain.NewLifetimeRegistrationNumber(typeCode, value)
		if err != nil {
			return fmt.Errorf("%w: legalEntities[0].lifetimeRegistrationNumbers[%d]: %v", ErrMalformedRequest, index, err)
		}
		command.LifetimeNumbers = append(command.LifetimeNumbers, lifetime)
	}
	if item.IdentityCorrectionBasis != nil {
		correction, err := domain.NewIdentityBasisReference(*item.IdentityCorrectionBasis)
		if err != nil {
			return fmt.Errorf("%w: legalEntities[0].identityCorrectionBasis: %v", ErrMalformedRequest, err)
		}
		command.IdentityCorrectionBasis = &correction
	}
	return nil
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

// refuseSelfReportedTenant 是本包在线口各份外壳共用的那道拒：键在场即拒，不看值（理由在 partyIdentityBatchDocument.TenantID）。
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

// identityKindFromName 是 application.PartyIdentityKind 封闭集的名称镜像；关系不在内——关系的终止走撤销/到期/替代，
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

// command 把一次管理台停用译成身份停用命令。Revision 是操作者声明自己看到的册面（= 最新修订 + 1），错位说明册面已被
// 并发推进或意图已陈旧，由用例拒而不是替操作者猜；这里只译。身份标识照原样过线（命令的 ID 是字符串，由用例按 Kind
// 分派到对应册的值对象）。
func (item deactivationDocument) command(tenant domain.TenantID) (application.DeactivatePartyIdentityCommand, error) {
	none := application.DeactivatePartyIdentityCommand{}
	kind, err := identityKindFromName(item.Kind)
	if err != nil {
		return none, fmt.Errorf("%w: deactivations[0].kind: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: deactivations[0].basis: %v", ErrMalformedRequest, err)
	}
	return application.DeactivatePartyIdentityCommand{
		Tenant:   tenant,
		Kind:     kind,
		ID:       item.ID,
		Revision: item.Revision,
		Basis:    basis,
		At:       item.At,
	}, nil
}

// legalEntityProfileBatchDocument 是资料登记口的载荷外壳：镜像受控 CLI `register-legal-entity-profiles` 的输入文档，去掉整批的
// tenantId（这一格只为拒而存在，理由同 partyIdentityBatchDocument.TenantID）。不并进身份族那份外壳：资料不是身份，CLI 里
// 也是两个子命令各收各的文档。
type legalEntityProfileBatchDocument struct {
	TenantID json.RawMessage              `json:"tenantId"`
	Profiles []legalEntityProfileDocument `json:"profiles"`
}

// legalEntityProfileDocument 与受控 CLI 的一项逐字同形，子格沿修订历史读口的答复体——一份资料在读写两个方向上同一个样子。
// 注册地址键可缺：缺即没给，由用例答`未受理`（判据同身份层三格）；给了就得立得住。invoiceTitle 缺席即这笔修订不带开票资料，
// 给了空串照样拒，不当缺席。
type legalEntityProfileDocument struct {
	LegalEntityID          string                      `json:"legalEntityId"`
	Revision               int                         `json:"revision"`
	Basis                  string                      `json:"basis"`
	EffectiveFrom          time.Time                   `json:"effectiveFrom"`
	RegisteredAddress      *registeredAddressBody      `json:"registeredAddress,omitempty"`
	TaxRegistrationNumbers []taxRegistrationNumberBody `json:"taxRegistrationNumbers,omitempty"`
	InvoiceTitle           *string                     `json:"invoiceTitle,omitempty"`
	Contacts               []legalEntityContactBody    `json:"contacts,omitempty"`
}

// command 把一次管理台登记的资料项译成法人资料修订登记命令（票 legal-entity-profile/05）。形状级失败包 ErrMalformedRequest；
// 法人在不在册、地址国家对不对得上身份、税号合不合目录，一律送进用例答`未受理`（判据同法人项）。
func (item legalEntityProfileDocument) command(tenant domain.TenantID) (application.RegisterLegalEntityProfileCommand, error) {
	none := application.RegisterLegalEntityProfileCommand{}
	entity, err := domain.NewLegalEntityReference(item.LegalEntityID)
	if err != nil {
		return none, fmt.Errorf("%w: profiles[0].legalEntityId: %v", ErrMalformedRequest, err)
	}
	basis, err := domain.NewLegalEntityProfileBasisReference(item.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: profiles[0].basis: %v", ErrMalformedRequest, err)
	}
	command := application.RegisterLegalEntityProfileCommand{
		Tenant:        tenant,
		Entity:        entity,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}
	if item.RegisteredAddress != nil {
		country, err := domain.NewRegistrationCountryCode(item.RegisteredAddress.Country)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].registeredAddress.country: %v", ErrMalformedRequest, err)
		}
		address, err := domain.NewRegisteredAddress(country, item.RegisteredAddress.Lines)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].registeredAddress: %v", ErrMalformedRequest, err)
		}
		command.Address = address
	}
	for index, number := range item.TaxRegistrationNumbers {
		typeCode, err := domain.NewRegistrationNumberTypeCode(number.TypeCode)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].taxRegistrationNumbers[%d].typeCode: %v", ErrMalformedRequest, index, err)
		}
		value, err := domain.NewRegistrationNumber(number.Number)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].taxRegistrationNumbers[%d].number: %v", ErrMalformedRequest, index, err)
		}
		tax, err := domain.NewTaxRegistrationNumber(typeCode, value)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].taxRegistrationNumbers[%d]: %v", ErrMalformedRequest, index, err)
		}
		command.TaxNumbers = append(command.TaxNumbers, tax)
	}
	if item.InvoiceTitle != nil {
		title, err := domain.NewInvoiceTitle(*item.InvoiceTitle)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].invoiceTitle: %v", ErrMalformedRequest, err)
		}
		details, err := domain.NewInvoicingDetails(title)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].invoiceTitle: %v", ErrMalformedRequest, err)
		}
		command.Invoicing = &details
	}
	for index, contact := range item.Contacts {
		built, err := domain.NewLegalEntityContact(contact.Name, contact.Email, contact.Phone)
		if err != nil {
			return none, fmt.Errorf("%w: profiles[0].contacts[%d]: %v", ErrMalformedRequest, index, err)
		}
		command.Contacts = append(command.Contacts, built)
	}
	return command, nil
}
