package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidComplianceJudgment = errors.New("customs compliance: invalid compliance judgment")
	ErrInvalidCredential         = errors.New("customs compliance: invalid regulatory credential")
	ErrCredentialNotApplicable   = errors.New("customs compliance: the credential is not applicable")
)

// ComplianceTopicReference 指名合规事项（禁限运、商品归类、原产地、申报价值、监管
// 条件、凭证适用性等）。事项目录属规则实例，这里是开放引用。
type ComplianceTopicReference struct{ requiredValue }

func NewComplianceTopicReference(value string) (ComplianceTopicReference, error) {
	required, err := newRequiredValue("compliance topic reference", value)
	return ComplianceTopicReference{required}, err
}

// ComplianceRuleVersionReference 指名适用规则版本。规则版本必须记录适用辖区、法定
// 生效区间与适用时点（191）——那些在规则本体上，这里引用。
type ComplianceRuleVersionReference struct{ requiredValue }

func NewComplianceRuleVersionReference(value string) (ComplianceRuleVersionReference, error) {
	required, err := newRequiredValue("compliance rule version reference", value)
	return ComplianceRuleVersionReference{required}, err
}

// JudgmentMode 是判断方式的封闭二值：规则自动或授权角色人工（CONTEXT 硬句 159：
// 自动与人工都必须保存规则版本、事实依据、决定方式和责任角色）。
type JudgmentMode uint8

const (
	JudgmentModeInvalid JudgmentMode = iota
	AutomaticJudgment
	ManualJudgment
)

func (mode JudgmentMode) valid() bool {
	return mode == AutomaticJudgment || mode == ManualJudgment
}

func (mode JudgmentMode) String() string {
	switch mode {
	case AutomaticJudgment:
		return "AUTOMATIC"
	case ManualJudgment:
		return "MANUAL"
	default:
		return ""
	}
}

// ComplianceJudgmentSpec 是形成一次合规判断所需的全部输入。
type ComplianceJudgmentSpec struct {
	Topic      ComplianceTopicReference
	Scope      DecisionScopeReference
	Rule       ComplianceRuleVersionReference
	Facts      string
	Mode       JudgmentMode
	Role       RoleSnapshotReference
	Conclusion string
	JudgedAt   time.Time
}

// ComplianceJudgment 是版本化合规结论。四件（规则版本/事实依据/决定方式/责任角色）
// 必备——自动与人工一视同仁（159）；监管机构的外部决定仍是最终监管事实，本判断没有
// 覆盖外部事实的入口。人工判断不能删除自动判断，后续自动计算也不能覆盖人工判断
// 历史——Supersede 只追加新版本指回前版。
type ComplianceJudgment struct {
	topic      ComplianceTopicReference
	scope      DecisionScopeReference
	rule       ComplianceRuleVersionReference
	facts      string
	mode       JudgmentMode
	role       RoleSnapshotReference
	conclusion string
	judgedAt   time.Time
	priorMode  JudgmentMode
	priorRule  ComplianceRuleVersionReference
}

func FormComplianceJudgment(spec ComplianceJudgmentSpec) (ComplianceJudgment, error) {
	if !spec.Topic.valid() ||
		!spec.Scope.valid() ||
		!spec.Rule.valid() ||
		spec.Facts == "" ||
		!spec.Mode.valid() ||
		!spec.Role.valid() ||
		spec.Conclusion == "" ||
		spec.JudgedAt.IsZero() {
		return ComplianceJudgment{}, ErrInvalidComplianceJudgment
	}
	return ComplianceJudgment{
		topic:      spec.Topic,
		scope:      spec.Scope,
		rule:       spec.Rule,
		facts:      spec.Facts,
		mode:       spec.Mode,
		role:       spec.Role,
		conclusion: spec.Conclusion,
		judgedAt:   spec.JudgedAt.UTC(),
	}, nil
}

func (judgment ComplianceJudgment) Topic() ComplianceTopicReference {
	return judgment.topic
}

func (judgment ComplianceJudgment) Mode() JudgmentMode {
	return judgment.mode
}

func (judgment ComplianceJudgment) Rule() ComplianceRuleVersionReference {
	return judgment.rule
}

func (judgment ComplianceJudgment) Role() RoleSnapshotReference {
	return judgment.role
}

func (judgment ComplianceJudgment) Conclusion() string {
	return judgment.conclusion
}

// PriorJudgment 只在换版后的判断上给出前版方式与规则。
func (judgment ComplianceJudgment) PriorJudgment() (JudgmentMode, ComplianceRuleVersionReference, bool) {
	return judgment.priorMode, judgment.priorRule, judgment.priorMode.valid()
}

// Supersede 形成新的合规判断版本：同事项同范围、新结论指回前版。人工替自动、自动替
// 人工都走这里——谁都删不掉谁（159），前版方式与规则随新版可查。
func (judgment ComplianceJudgment) Supersede(spec ComplianceJudgmentSpec) (ComplianceJudgment, error) {
	if spec.Topic != judgment.topic || spec.Scope != judgment.scope {
		return ComplianceJudgment{}, ErrInvalidComplianceJudgment
	}
	if spec.JudgedAt.IsZero() || spec.JudgedAt.Before(judgment.judgedAt) {
		return ComplianceJudgment{}, ErrInvalidComplianceJudgment
	}
	superseded, err := FormComplianceJudgment(spec)
	if err != nil {
		return ComplianceJudgment{}, err
	}
	superseded.priorMode = judgment.mode
	superseded.priorRule = judgment.rule
	return superseded, nil
}

// CredentialID 是监管凭证的独立身份。附件文件只是证据，不能代替凭证身份和适用性
// 判断（CONTEXT「监管凭证」语言）。
type CredentialID struct{ requiredValue }

func NewCredentialID(value string) (CredentialID, error) {
	required, err := newRequiredValue("credential ID", value)
	return CredentialID{required}, err
}

// CredentialHolderReference 指名依法使用凭证的持有人。
type CredentialHolderReference struct{ requiredValue }

func NewCredentialHolderReference(value string) (CredentialHolderReference, error) {
	required, err := newRequiredValue("credential holder reference", value)
	return CredentialHolderReference{required}, err
}

// RegulatoryCredential 是监管凭证的不可变版本：签发机构、持有人、适用辖区/商品/
// 程序、有效期与适用额度一次进入。
type RegulatoryCredential struct {
	id        CredentialID
	issuer    RegulatoryAuthorityReference
	holder    CredentialHolderReference
	procedure CustomsProcedureReference
	validFrom time.Time
	validTo   time.Time
	uses      int
}

// RegisterCredential 登记一版凭证。uses 为负是矛盾输入；零表示来源未提供次数额度
// ——未提供必须明确记录，不猜测补齐（与监管决定的数量维度同一条纪律）。
func RegisterCredential(
	id CredentialID,
	issuer RegulatoryAuthorityReference,
	holder CredentialHolderReference,
	procedure CustomsProcedureReference,
	validFrom time.Time,
	validTo time.Time,
	uses int,
) (RegulatoryCredential, error) {
	if !id.valid() || !issuer.valid() || !holder.valid() || !procedure.valid() ||
		validFrom.IsZero() || validTo.IsZero() || !validTo.After(validFrom) || uses < 0 {
		return RegulatoryCredential{}, ErrInvalidCredential
	}
	return RegulatoryCredential{
		id:        id,
		issuer:    issuer,
		holder:    holder,
		procedure: procedure,
		validFrom: validFrom.UTC(),
		validTo:   validTo.UTC(),
		uses:      uses,
	}, nil
}

func (credential RegulatoryCredential) ID() CredentialID {
	return credential.id
}

func (credential RegulatoryCredential) Issuer() RegulatoryAuthorityReference {
	return credential.issuer
}

func (credential RegulatoryCredential) Holder() CredentialHolderReference {
	return credential.holder
}

// JudgeApplicability 判断凭证对给定程序、持有人与时点是否适用：程序相符、持有人
// 相符、时点在有效期内三者齐备才适用——同名附件在别的程序上用不了，过期凭证谁拿着
// 都不适用。答案带依据（哪一维不符）。
func (credential RegulatoryCredential) JudgeApplicability(
	procedure CustomsProcedureReference,
	holder CredentialHolderReference,
	at time.Time,
) error {
	if !procedure.valid() || !holder.valid() || at.IsZero() {
		return ErrInvalidCredential
	}
	if credential.procedure != procedure ||
		credential.holder != holder ||
		at.Before(credential.validFrom) || at.After(credential.validTo) {
		return ErrCredentialNotApplicable
	}
	return nil
}
