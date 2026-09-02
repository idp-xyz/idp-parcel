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
	// 承接配对双向核（CONTEXT 硬句 219 承接项必须指名接收责任方；库 CHECK 同句）。
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

// gateKeyFrom 译装门禁两命令共用的判断身份三维加租户。门禁判断绑定动作与边界
// （CONTEXT 硬句 216），键上四件缺一不可。
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

// 下面四个解析器把封闭词表译回领域常量。词表与领域 String()/迁移 CHECK 同字——
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
