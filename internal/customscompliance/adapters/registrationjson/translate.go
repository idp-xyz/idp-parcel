// Package registrationjson 把关务登记输入 JSON 译装成登记用例命令。
//
// 它从 `cmd/parcel-customs-register` 下沉到本上下文，理由只有一条：登记快照的形状要
// 由**一份**代码把门。受控 CLI 的 `-input` 与在线登记端点收的是同源载荷（ADR-0085
// Decision 一：CLI 与端点消费同一登记用例，答案代数一致），而 `cmd` 包不可被
// `internal` 导入——留在那里，在线口就只能再拄一份译装，两份的严格性此后各自漂移，
// 而漂移的那一半不会有任何东西报出来。
//
// 本包不认证、不采信任何自报身份：`tenantId` 在这里只是登记快照的一个字段，它是不是
// 调用方有权写的那个租户由调用侧回答——CLI 靠「能进数据库网络」这道运维边界，在线口
// 靠真渠道 Intake（`PAR-INT-01` 待提供，采信报文自称的租户会穿透 ADR-0003 的隔离
// 边界）。译装本身对两侧同形，那道判断不在本包。
package registrationjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 译装严格且零默认：未知字段拒收（打错字段名不得静默变成「没给」）、有构造门的标识在
// 这里就拒、封闭词表在这里就核——词表外的取值走到库上 CHECK 才被拦时，答案已经滑到
// 未决，而它明明是用法错误。时间与撤销原因那类有领域门的内容原样递给领域，这里绝不
// 代判。
//
// 两处译装必须自己把门，因为它们身后没有别的门：
//   - 两本目录的 registeredAt——领域与库都没有零值门（timestamptz 装得下 0001 年），
//     缺格会静默变成一个错的事实；
//   - 义务项的承接配对与内容格——库的 CHECK 拦得住，但那属「依赖故障」的退出码，
//     缺格该在入库前指名拒绝。

type readinessRegisterDocument struct {
	TenantID string    `json:"tenantId"`
	UnitID   string    `json:"unitId"`
	BasisRef string    `json:"basisRef"`
	JudgedAt time.Time `json:"judgedAt"`
}

// ReadinessRegisterFromJSON 译装一次申报就绪判断登记。
func ReadinessRegisterFromJSON(raw []byte) (application.RegisterReadinessCommand, error) {
	none := application.RegisterReadinessCommand{}
	var document readinessRegisterDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("就绪登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	unit, err := domain.NewDeclarationUnitID(document.UnitID)
	if err != nil {
		return none, err
	}
	basis, err := domain.NewReadinessBasisReference(document.BasisRef)
	if err != nil {
		return none, err
	}
	return application.RegisterReadinessCommand{
		TenantID: tenant,
		Unit:     unit,
		Basis:    basis,
		JudgedAt: document.JudgedAt,
	}, nil
}

type revocationDocument struct {
	TenantID string    `json:"tenantId"`
	UnitID   string    `json:"unitId"`
	Cause    string    `json:"cause"`
	At       time.Time `json:"at"`
}

// revocationFromJSON 是两条撤销命令共用的译装：形状相同，撤销的合法性（原因非空、
// 不早于形成时间、不重复撤销）全在领域门，这里只构造标识。
func revocationFromJSON(raw []byte, kind string) (domain.TenantID, domain.DeclarationUnitID, revocationDocument, error) {
	none := revocationDocument{}
	var document revocationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.DeclarationUnitID{}, none,
			fmt.Errorf("%s输入不是本入口的形状：%w", kind, err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.DeclarationUnitID{}, none, err
	}
	unit, err := domain.NewDeclarationUnitID(document.UnitID)
	if err != nil {
		return domain.TenantID{}, domain.DeclarationUnitID{}, none, err
	}
	return tenant, unit, document, nil
}

// ReadinessRevokeFromJSON 译装一次`不再就绪`。
func ReadinessRevokeFromJSON(raw []byte) (application.RevokeReadinessCommand, error) {
	tenant, unit, document, err := revocationFromJSON(raw, "就绪撤销")
	if err != nil {
		return application.RevokeReadinessCommand{}, err
	}
	return application.RevokeReadinessCommand{
		TenantID: tenant,
		Unit:     unit,
		Cause:    document.Cause,
		At:       document.At,
	}, nil
}

type authorityGrantDocument struct {
	TenantID     string    `json:"tenantId"`
	UnitID       string    `json:"unitId"`
	AuthorityRef string    `json:"authorityRef"`
	GrantedAt    time.Time `json:"grantedAt"`
}

// AuthorityGrantFromJSON 译装一次提交授权登记。
func AuthorityGrantFromJSON(raw []byte) (application.GrantSubmissionAuthorityCommand, error) {
	none := application.GrantSubmissionAuthorityCommand{}
	var document authorityGrantDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("授权登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	unit, err := domain.NewDeclarationUnitID(document.UnitID)
	if err != nil {
		return none, err
	}
	authority, err := domain.NewSubmissionAuthorityReference(document.AuthorityRef)
	if err != nil {
		return none, err
	}
	return application.GrantSubmissionAuthorityCommand{
		TenantID:  tenant,
		Unit:      unit,
		Authority: authority,
		GrantedAt: document.GrantedAt,
	}, nil
}

// AuthorityRevokeFromJSON 译装一次授权失效。
func AuthorityRevokeFromJSON(raw []byte) (application.RevokeSubmissionAuthorityCommand, error) {
	tenant, unit, document, err := revocationFromJSON(raw, "授权撤销")
	if err != nil {
		return application.RevokeSubmissionAuthorityCommand{}, err
	}
	return application.RevokeSubmissionAuthorityCommand{
		TenantID: tenant,
		Unit:     unit,
		Cause:    document.Cause,
		At:       document.At,
	}, nil
}

type interpretationRuleDocument struct {
	TenantID        string    `json:"tenantId"`
	ResultLayer     string    `json:"resultLayer"`
	JurisdictionRef string    `json:"jurisdictionRef"`
	RuleRef         string    `json:"ruleRef"`
	AppliesFrom     time.Time `json:"appliesFrom"`
}

// InterpretationRuleFromJSON 译装一次解释规则版本登记。
func InterpretationRuleFromJSON(raw []byte) (application.RegisterInterpretationRuleCommand, error) {
	none := application.RegisterInterpretationRuleCommand{}
	var document interpretationRuleDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("解释规则登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	layer, err := resultLayerFrom(document.ResultLayer)
	if err != nil {
		return none, err
	}
	jurisdiction, err := domain.NewRegulatoryJurisdictionReference(document.JurisdictionRef)
	if err != nil {
		return none, err
	}
	rule, err := domain.NewInterpretationRuleReference(document.RuleRef)
	if err != nil {
		return none, err
	}
	// 法定生效起点在键上且无默认可言（timestamptz 装得下 0001 年，缺格会静默变成
	// 一个错的版本边界）；终点不是输入——后继版本登记时自动给前版落终点（换版）。
	if document.AppliesFrom.IsZero() {
		return none, fmt.Errorf("解释规则登记缺 appliesFrom——法定生效起点没有默认值")
	}
	return application.RegisterInterpretationRuleCommand{
		TenantID:     tenant,
		Layer:        layer,
		Jurisdiction: jurisdiction,
		Rule:         rule,
		AppliesFrom:  document.AppliesFrom,
	}, nil
}

type obligationCatalogDocument struct {
	TenantID     string    `json:"tenantId"`
	CaseRef      string    `json:"caseRef"`
	RegisteredAt time.Time `json:"registeredAt"`
}

// ObligationCatalogFromJSON 译装一次关闭义务目录登记。
func ObligationCatalogFromJSON(raw []byte) (application.RegisterObligationCatalogCommand, error) {
	none := application.RegisterObligationCatalogCommand{}
	var document obligationCatalogDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("义务目录登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	caseRef, err := domain.NewCustomsCaseID(document.CaseRef)
	if err != nil {
		return none, err
	}
	if document.RegisteredAt.IsZero() {
		return none, fmt.Errorf("义务目录登记缺 registeredAt——登记时刻没有默认值")
	}
	return application.RegisterObligationCatalogCommand{
		TenantID:     tenant,
		CaseRef:      caseRef,
		RegisteredAt: document.RegisteredAt,
	}, nil
}

type obligationItemDocument struct {
	TenantID     string     `json:"tenantId"`
	CaseRef      string     `json:"caseRef"`
	Obligation   string     `json:"obligation"`
	Scope        string     `json:"scope"`
	State        string     `json:"state"`
	Basis        string     `json:"basis"`
	HandedTo     string     `json:"handedTo"`
	AppliesFrom  time.Time  `json:"appliesFrom"`
	AppliesUntil *time.Time `json:"appliesUntil"`
}

// ObligationItemFromJSON 译装一项关闭义务及其适用区间。
func ObligationItemFromJSON(raw []byte) (application.RegisterObligationItemCommand, error) {
	none := application.RegisterObligationItemCommand{}
	var document obligationItemDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("义务项登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	caseRef, err := domain.NewCustomsCaseID(document.CaseRef)
	if err != nil {
		return none, err
	}
	state, err := obligationItemStateFrom(document.State)
	if err != nil {
		return none, err
	}
	for field, value := range map[string]string{
		"obligation": document.Obligation,
		"scope":      document.Scope,
		"basis":      document.Basis,
	} {
		if strings.TrimSpace(value) == "" {
			return none, fmt.Errorf("义务项登记缺 %s——内容格逐格必填，无默认值", field)
		}
	}
	// 承接配对双向核（CONTEXT「来源责任方、接收责任方、接受决定及权限」承接项必须指名接收责任方；库 CHECK 同句）。
	// 反向也拒：非承接项带承接对象会被写口静默折成 NULL，操作员会以为登进去的比
	// 实际多——静默丢弃正是译装要挡的事。
	handedTo := strings.TrimSpace(document.HandedTo)
	if state == domain.ObligationHandedOver && handedTo == "" {
		return none, fmt.Errorf("义务项登记缺 handedTo——承接项必须指名接收责任方")
	}
	if state != domain.ObligationHandedOver && handedTo != "" {
		return none, fmt.Errorf("义务项登记的 handedTo 只属承接项——state=%s 不携带承接对象", state)
	}
	return application.RegisterObligationItemCommand{
		TenantID: tenant,
		CaseRef:  caseRef,
		Registration: ports.ObligationRegistration{
			Item: domain.ClosureObligationItem{
				Obligation: document.Obligation,
				Scope:      document.Scope,
				State:      state,
				Basis:      document.Basis,
				HandedTo:   document.HandedTo,
			},
			AppliesFrom:  document.AppliesFrom,
			AppliesUntil: timeOf(document.AppliesUntil),
		},
	}, nil
}

type gateCatalogDocument struct {
	TenantID     string    `json:"tenantId"`
	ScopeRef     string    `json:"scopeRef"`
	Action       string    `json:"action"`
	BoundaryRef  string    `json:"boundaryRef"`
	RegisteredAt time.Time `json:"registeredAt"`
}

// GateCatalogFromJSON 译装一次门禁前置条件目录登记。
func GateCatalogFromJSON(raw []byte) (application.RegisterGateCatalogCommand, error) {
	none := application.RegisterGateCatalogCommand{}
	var document gateCatalogDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("门禁目录登记输入不是本入口的形状：%w", err)
	}
	tenant, scope, action, boundary, err := gateKeyFrom(
		document.TenantID, document.ScopeRef, document.Action, document.BoundaryRef)
	if err != nil {
		return none, err
	}
	if document.RegisteredAt.IsZero() {
		return none, fmt.Errorf("门禁目录登记缺 registeredAt——登记时刻没有默认值")
	}
	return application.RegisterGateCatalogCommand{
		TenantID:     tenant,
		Scope:        scope,
		Action:       action,
		Boundary:     boundary,
		RegisteredAt: document.RegisteredAt,
	}, nil
}

type gateFindingDocument struct {
	TenantID        string `json:"tenantId"`
	ScopeRef        string `json:"scopeRef"`
	Action          string `json:"action"`
	BoundaryRef     string `json:"boundaryRef"`
	PreconditionRef string `json:"preconditionRef"`
	State           string `json:"state"`
}

// GateFindingFromJSON 译装一项前置条件判断。
func GateFindingFromJSON(raw []byte) (application.RegisterGateFindingCommand, error) {
	none := application.RegisterGateFindingCommand{}
	var document gateFindingDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("门禁判断登记输入不是本入口的形状：%w", err)
	}
	tenant, scope, action, boundary, err := gateKeyFrom(
		document.TenantID, document.ScopeRef, document.Action, document.BoundaryRef)
	if err != nil {
		return none, err
	}
	precondition, err := domain.NewPreconditionReference(document.PreconditionRef)
	if err != nil {
		return none, err
	}
	state, err := preconditionStateFrom(document.State)
	if err != nil {
		return none, err
	}
	return application.RegisterGateFindingCommand{
		TenantID: tenant,
		Scope:    scope,
		Action:   action,
		Boundary: boundary,
		Finding: domain.PreconditionFinding{
			Precondition: precondition,
			State:        state,
		},
	}, nil
}

type caseRequirementDocument struct {
	TenantID        string `json:"tenantId"`
	JurisdictionRef string `json:"jurisdictionRef"`
	Direction       string `json:"direction"`
	ProcedureRef    string `json:"procedureRef"`
	Required        *bool  `json:"required"`
	Basis           string `json:"basis"`
}

// CaseRequirementFromJSON 译装一次建案要求规则登记。
func CaseRequirementFromJSON(raw []byte) (application.RegisterCaseRequirementRuleCommand, error) {
	none := application.RegisterCaseRequirementRuleCommand{}
	var document caseRequirementDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("建案要求规则登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	jurisdiction, err := domain.NewRegulatoryJurisdictionReference(document.JurisdictionRef)
	if err != nil {
		return none, err
	}
	direction, err := manifestDirectionFrom(document.Direction)
	if err != nil {
		return none, err
	}
	procedure, err := domain.NewCustomsProcedureReference(document.ProcedureRef)
	if err != nil {
		return none, err
	}
	// 判断格必须显式给：required 缺席时 Go 的零值是 false，静默落成「不要求建案」
	// 正是「缺格变成错事实」——指针分辨「没给」与「给了 false」，拒前者。
	if document.Required == nil {
		return none, fmt.Errorf("建案要求规则登记缺 required——「要求」与「不要求」都要显式说")
	}
	return application.RegisterCaseRequirementRuleCommand{
		TenantID:     tenant,
		Jurisdiction: jurisdiction,
		Direction:    direction,
		Procedure:    procedure,
		Required:     *document.Required,
		Basis:        document.Basis,
	}, nil
}

type candidatePortDocument struct {
	TenantID    string    `json:"tenantId"`
	PortRef     string    `json:"portRef"`
	AppliesFrom time.Time `json:"appliesFrom"`
}

// CandidatePortFromJSON 译装一次口岸合规候选版本登记。
func CandidatePortFromJSON(raw []byte) (application.RegisterCandidatePortCommand, error) {
	none := application.RegisterCandidatePortCommand{}
	var document candidatePortDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("口岸目录登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	port, err := domain.NewCustomsPortReference(document.PortRef)
	if err != nil {
		return none, err
	}
	// 生效起点在键上且无默认可言（同解释规则那格）；终点不是输入——后继版本登记时
	// 自动给前版落终点（换版）。
	if document.AppliesFrom.IsZero() {
		return none, fmt.Errorf("口岸目录登记缺 appliesFrom——生效起点没有默认值")
	}
	return application.RegisterCandidatePortCommand{
		TenantID:    tenant,
		Port:        port,
		AppliesFrom: document.AppliesFrom,
	}, nil
}

type declarationPathDocument struct {
	TenantID        string    `json:"tenantId"`
	PathRef         string    `json:"pathRef"`
	PortRef         string    `json:"portRef"`
	Direction       string    `json:"direction"`
	DeclarationMode string    `json:"declarationMode"`
	AppliesFrom     time.Time `json:"appliesFrom"`
}

// DeclarationPathFromJSON 译装一次申报路径版本登记。
func DeclarationPathFromJSON(raw []byte) (application.RegisterDeclarationPathCommand, error) {
	none := application.RegisterDeclarationPathCommand{}
	var document declarationPathDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("申报路径登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	path, err := domain.NewDeclarationPathReference(document.PathRef)
	if err != nil {
		return none, err
	}
	port, err := domain.NewCustomsPortReference(document.PortRef)
	if err != nil {
		return none, err
	}
	direction, err := manifestDirectionFrom(document.Direction)
	if err != nil {
		return none, err
	}
	// 申报模式是引用不是封闭词表（真实模式集属监管规则实例半边），构造门只拒空白。
	mode, err := domain.NewDeclarationModeReference(document.DeclarationMode)
	if err != nil {
		return none, err
	}
	route, err := domain.NewDeclarationPathRoute(port, direction, mode)
	if err != nil {
		return none, err
	}
	if document.AppliesFrom.IsZero() {
		return none, fmt.Errorf("申报路径登记缺 appliesFrom——生效起点没有默认值")
	}
	return application.RegisterDeclarationPathCommand{
		TenantID:    tenant,
		Path:        path,
		Route:       route,
		AppliesFrom: document.AppliesFrom,
	}, nil
}

type regulatoryCredentialDocument struct {
	TenantID     string    `json:"tenantId"`
	CredentialID string    `json:"credentialId"`
	IssuerRef    string    `json:"issuerRef"`
	HolderRef    string    `json:"holderRef"`
	ProcedureRef string    `json:"procedureRef"`
	ValidFrom    time.Time `json:"validFrom"`
	ValidTo      time.Time `json:"validTo"`
	Uses         int       `json:"uses"`
}

// RegulatoryCredentialFromJSON 译装一版监管凭证登记（票 sa-cc/07）。有效期两端与额度的
// 门在领域（期限有序、额度非负），原样递过去不代判；uses 缺席与 0 同义——领域把零约定为
// 「来源未提供次数额度」并经 Uses 的第二个返回值显式交出，所以这一格不用指针分「没给」
// 与「给了 0」：两者在领域上就是同一格。
func RegulatoryCredentialFromJSON(raw []byte) (application.RegisterCredentialCommand, error) {
	none := application.RegisterCredentialCommand{}
	var document regulatoryCredentialDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("凭证登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewCredentialID(document.CredentialID)
	if err != nil {
		return none, err
	}
	issuer, err := domain.NewRegulatoryAuthorityReference(document.IssuerRef)
	if err != nil {
		return none, err
	}
	holder, err := domain.NewCredentialHolderReference(document.HolderRef)
	if err != nil {
		return none, err
	}
	procedure, err := domain.NewCustomsProcedureReference(document.ProcedureRef)
	if err != nil {
		return none, err
	}
	return application.RegisterCredentialCommand{
		TenantID:  tenant,
		ID:        id,
		Issuer:    issuer,
		Holder:    holder,
		Procedure: procedure,
		ValidFrom: document.ValidFrom,
		ValidTo:   document.ValidTo,
		Uses:      document.Uses,
	}, nil
}

type dutyCollaborationDocument struct {
	TenantID       string `json:"tenantId"`
	Kind           string `json:"kind"`
	DutyRef        string `json:"dutyRef"`
	NoPayBasis     string `json:"noPayBasis"`
	ScopeRef       string `json:"scopeRef"`
	ObligorRef     string `json:"obligorRef"`
	RequirementRef string `json:"requirementRef"`
	TargetRef      string `json:"targetRef"`
}

// DutyCollaborationFromJSON 译装一次税费付款协作事项形成（票 sa-cc/07）。义务依据两格
// 的形状（核定格带税费引用不带无需付款依据、无需付款格反之）由领域 FormDutyCollaboration
// 把门，这里不复述；形成时间不是输入，取编排的时钟。
//
// kind 缺席刻意放行而不当用法错误拒：两格都没有是「既无核定税费也无明确无需付款依据」
// ——UC-CC-009 步 4 的第四个结果`未决`，编排答 DUTY_OBLIGATION_BASIS_ABSENT 让登记方等税费
// 结果；在这里拒掉它，那一格就从 CLI 上消失了，等于入口替编排改判。打错的词另论：词表外
// 的取值就是用法错误，在这里指名拒。
func DutyCollaborationFromJSON(raw []byte) (application.FormDutyCollaborationCommand, error) {
	none := application.FormDutyCollaborationCommand{}
	var document dutyCollaborationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("协作事项登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	kind, err := dutyObligationKindFrom(document.Kind)
	if err != nil {
		return none, err
	}
	// 无需付款格的税费引用在键上就是空（0016 自注）：那一格要的是零值引用，不是一个
	// 「空串」引用——后者构造期就拒，而空缺本身是这一格的正当形状。
	var duty domain.AssessedDutyReference
	if document.DutyRef != "" {
		if duty, err = domain.NewAssessedDutyReference(document.DutyRef); err != nil {
			return none, err
		}
	}
	scope, err := domain.NewDecisionScopeReference(document.ScopeRef)
	if err != nil {
		return none, err
	}
	obligor, err := domain.NewLegalObligorReference(document.ObligorRef)
	if err != nil {
		return none, err
	}
	requirement, err := domain.NewPaymentRequirementSource(document.RequirementRef)
	if err != nil {
		return none, err
	}
	target, err := domain.NewResponsibilityTargetReference(document.TargetRef)
	if err != nil {
		return none, err
	}
	return application.FormDutyCollaborationCommand{
		TenantID:    tenant,
		Kind:        kind,
		Duty:        duty,
		NoPayBasis:  document.NoPayBasis,
		Scope:       scope,
		Obligor:     obligor,
		Requirement: requirement,
		Target:      target,
	}, nil
}

type dutyPaymentVerificationDocument struct {
	TenantID     string `json:"tenantId"`
	DutyRef      string `json:"dutyRef"`
	FundsRef     string `json:"fundsRef"`
	ScopeRef     string `json:"scopeRef"`
	ProcedureRef string `json:"procedureRef"`
	Coverage     string `json:"coverage"`
	Delta        string `json:"delta"`
	Validity     string `json:"validity"`
	Basis        string `json:"basis"`
}

// DutyPaymentVerificationFromJSON 译装一次税费付款核对（票 sa-cc/07）。三轴与关联依据由
// 登记方交进来——真实程序的关联规则属实例半边，入口不从金额相等推任何一轴；basis 空白
// 原样递给编排，那是它的`待关联`格（无权威依据不关联），不是译装该拒的缺格。procedureRef 必填
// （票 sa-cc/12）：付款人那一维的规则按它读，与三轴同一种形——由登记方说这次核对在哪个监管程序下判，
// 入口不从范围推。
func DutyPaymentVerificationFromJSON(raw []byte) (application.VerifyDutyPaymentCommand, error) {
	none := application.VerifyDutyPaymentCommand{}
	var document dutyPaymentVerificationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("付款核对登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	duty, err := domain.NewAssessedDutyReference(document.DutyRef)
	if err != nil {
		return none, err
	}
	funds, err := domain.NewExternalFundsFactReference(document.FundsRef)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewDecisionScopeReference(document.ScopeRef)
	if err != nil {
		return none, err
	}
	procedure, err := domain.NewCustomsProcedureReference(document.ProcedureRef)
	if err != nil {
		return none, err
	}
	coverage, err := dutyCoverageFrom(document.Coverage)
	if err != nil {
		return none, err
	}
	delta, err := dutyDeltaFrom(document.Delta)
	if err != nil {
		return none, err
	}
	validity, err := dutyFactValidityFrom(document.Validity)
	if err != nil {
		return none, err
	}
	return application.VerifyDutyPaymentCommand{
		TenantID:  tenant,
		Duty:      duty,
		Funds:     funds,
		Scope:     scope,
		Procedure: procedure,
		Coverage:  coverage,
		Delta:     delta,
		Validity:  validity,
		Basis:     document.Basis,
	}, nil
}

// gateKeyFrom 译装门禁两命令共用的判断身份三维加租户。门禁判断绑定动作与边界
// （CONTEXT「不能复用于其他动作或监管边界」），键上四件缺一不可。
func gateKeyFrom(tenantID, scopeRef, actionText, boundaryRef string) (
	domain.TenantID,
	domain.DecisionScopeReference,
	domain.GuardedAction,
	domain.CustomsProcedureReference,
	error,
) {
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return domain.TenantID{}, domain.DecisionScopeReference{}, domain.GuardedActionInvalid,
			domain.CustomsProcedureReference{}, err
	}
	scope, err := domain.NewDecisionScopeReference(scopeRef)
	if err != nil {
		return domain.TenantID{}, domain.DecisionScopeReference{}, domain.GuardedActionInvalid,
			domain.CustomsProcedureReference{}, err
	}
	action, err := guardedActionFrom(actionText)
	if err != nil {
		return domain.TenantID{}, domain.DecisionScopeReference{}, domain.GuardedActionInvalid,
			domain.CustomsProcedureReference{}, err
	}
	boundary, err := domain.NewCustomsProcedureReference(boundaryRef)
	if err != nil {
		return domain.TenantID{}, domain.DecisionScopeReference{}, domain.GuardedActionInvalid,
			domain.CustomsProcedureReference{}, err
	}
	return tenant, scope, action, boundary, nil
}

// 下面的解析器把封闭词表译回领域常量。词表与领域 String()/迁移 CHECK 同字——
// 本包与写口适配器各持一份私有映射是既有格局（适配器侧同为私有），真库垂直用例
// 把两份钉在同一词表上。

func resultLayerFrom(raw string) (domain.ResultLayer, error) {
	switch raw {
	case "REGULATORY_RECEIPT":
		return domain.RegulatoryReceiptLayer, nil
	case "BUSINESS_ACCEPTANCE":
		return domain.BusinessAcceptanceLayer, nil
	case "PROCESS_DECISION":
		return domain.ProcessDecisionLayer, nil
	case "ASSESSED_DUTY":
		return domain.AssessedDutyLayer, nil
	case "RELEASE_RESULT":
		return domain.ReleaseResultLayer, nil
	case "DISPOSITION_DECISION":
		return domain.DispositionDecisionLayer, nil
	default:
		return domain.ResultLayerInvalid, fmt.Errorf(
			"resultLayer=%q 不在封闭六层（REGULATORY_RECEIPT / BUSINESS_ACCEPTANCE / PROCESS_DECISION / ASSESSED_DUTY / RELEASE_RESULT / DISPOSITION_DECISION）",
			raw)
	}
}

func guardedActionFrom(raw string) (domain.GuardedAction, error) {
	switch raw {
	case "OUTBOUND_RELEASE":
		return domain.OutboundRelease, nil
	case "LOADING_DEPARTURE":
		return domain.LoadingDeparture, nil
	case "CROSS_CUSTOMS_MOVEMENT":
		return domain.CrossCustomsMovement, nil
	case "FINAL_DELIVERY":
		return domain.FinalDelivery, nil
	default:
		return domain.GuardedActionInvalid, fmt.Errorf(
			"action=%q 不在封闭四值（OUTBOUND_RELEASE / LOADING_DEPARTURE / CROSS_CUSTOMS_MOVEMENT / FINAL_DELIVERY）",
			raw)
	}
}

func obligationItemStateFrom(raw string) (domain.ObligationItemState, error) {
	switch raw {
	case "CONCLUDED":
		return domain.ObligationConcluded, nil
	case "HANDED_OVER":
		return domain.ObligationHandedOver, nil
	case "UNRESOLVED":
		return domain.ObligationUnresolved, nil
	default:
		return domain.ObligationItemStateInvalid, fmt.Errorf(
			"state=%q 不在封闭三值（CONCLUDED / HANDED_OVER / UNRESOLVED）", raw)
	}
}

func manifestDirectionFrom(raw string) (domain.ManifestDirection, error) {
	switch raw {
	case "IMPORT":
		return domain.ImportManifest, nil
	case "EXPORT":
		return domain.ExportManifest, nil
	default:
		return domain.ManifestDirectionInvalid, fmt.Errorf(
			"direction=%q 不在封闭二向（IMPORT / EXPORT）", raw)
	}
}

func preconditionStateFrom(raw string) (domain.PreconditionState, error) {
	switch raw {
	case "MET":
		return domain.PreconditionMet, nil
	case "UNMET":
		return domain.PreconditionUnmet, nil
	case "CONFLICTING":
		return domain.PreconditionConflicting, nil
	default:
		return domain.PreconditionStateInvalid, fmt.Errorf(
			"state=%q 不在封闭三值（MET / UNMET / CONFLICTING）", raw)
	}
}

// dutyObligationKindFrom 与其余解析器有一处不同：空串放行成领域的无效格而不拒——见
// DutyCollaborationFromJSON 头注，那是编排要答的业务未决，不是打错的词。
func dutyObligationKindFrom(raw string) (domain.DutyObligationKind, error) {
	switch raw {
	case "":
		return domain.DutyObligationKindInvalid, nil
	case "ASSESSED_DUTY":
		return domain.ObligationFromAssessedDuty, nil
	case "EXPLICITLY_NOT_REQUIRED":
		return domain.ObligationExplicitlyNotRequired, nil
	default:
		return domain.DutyObligationKindInvalid, fmt.Errorf(
			"kind=%q 不在封闭二值（ASSESSED_DUTY / EXPLICITLY_NOT_REQUIRED；缺席即义务依据未到）", raw)
	}
}

func dutyCoverageFrom(raw string) (domain.DutyCoverage, error) {
	switch raw {
	case "NONE":
		return domain.CoverageNone, nil
	case "PARTIAL":
		return domain.CoveragePartial, nil
	case "COVERED":
		return domain.CoverageFull, nil
	default:
		return domain.DutyCoverageInvalid, fmt.Errorf(
			"coverage=%q 不在封闭三值（NONE / PARTIAL / COVERED）", raw)
	}
}

func dutyDeltaFrom(raw string) (domain.DutyDelta, error) {
	switch raw {
	case "NO_DELTA":
		return domain.DeltaNone, nil
	case "SHORT":
		return domain.DeltaShort, nil
	case "EXCESS":
		return domain.DeltaExcess, nil
	case "PENDING":
		return domain.DeltaPending, nil
	default:
		return domain.DutyDeltaInvalid, fmt.Errorf(
			"delta=%q 不在封闭四值（NO_DELTA / SHORT / EXCESS / PENDING）", raw)
	}
}

func dutyFactValidityFrom(raw string) (domain.DutyFactValidity, error) {
	switch raw {
	case "VALID":
		return domain.FundsFactValid, nil
	case "INVALIDATED":
		return domain.FundsFactInvalidated, nil
	case "CONFLICTING":
		return domain.FundsFactConflicting, nil
	case "PENDING":
		return domain.FundsFactPending, nil
	default:
		return domain.DutyFactValidityInvalid, fmt.Errorf(
			"validity=%q 不在封闭四值（VALID / INVALIDATED / CONFLICTING / PENDING）", raw)
	}
}

// decodeStrict 拒未知字段：打错的键静默丢弃，会让操作员以为登进去的比实际多。
func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// timeOf 把可选时刻译回值语义；零时刻在 ObligationRegistration 里就是「尚无终点」。
func timeOf(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
