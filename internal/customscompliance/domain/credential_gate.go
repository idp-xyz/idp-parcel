package domain

import (
	"errors"
	"time"
)

var ErrInvalidCredentialGate = errors.New("customs compliance: invalid credential gate judgment")

// 凭证门禁判断——UC-CC-003 就绪门禁第 4 道「监管凭证」在一次评估请求上判出的那一格，逐门禁
// 判断的第一册（ADR-0137 决定一）。CONTEXT 的话：「逐门禁判断（凭证门禁为第一册）是本上下文
// 登记的不可覆盖事实，就绪判断按不可变引用绑定它们、不内嵌其内部结构」。它只记判断，不占用、
// 不释放、不核销——那三件是凭证使用的生命周期（UC-CC-005 步 7/9、UC-CC-006 步 7），时点由
// 真实程序定（PAR-CUS-04）。
//
// 一条判断把八件一次固定：申报单元、凭证身份、拟使用的程序与持有人、截至时点、结论、依据引用、
// 责任角色，加判断时刻。「凭证身份与版本」在本仓折成一个身份——监管凭证是「一身份一版」（凭证
// 登记册的键），换内容是另一张凭证；这里因此没有第二个版本列，而不是漏了。

// CredentialGateConclusion 是凭证门禁判断的结论封闭四格，与 JudgeCredentialApplicability 判出的
// 四格同词。「凭证未登记」与「不适用」刻意分立：前者是实例半边还没到、续办是登记；后者是判断
// 结论、续办是由凭证责任流程形成有效依据后再发一次评估请求（UC-CC-003 门禁表第 4 行）。
// 「未决」记的是这次评估请求上这道门没判出来（读凭证册的口故障），不是「不适用」。
type CredentialGateConclusion uint8

const (
	CredentialGateConclusionInvalid CredentialGateConclusion = iota
	CredentialGateApplicable
	CredentialGateNotApplicable
	CredentialGateCredentialNotRegistered
	CredentialGateUndecided
)

func (conclusion CredentialGateConclusion) valid() bool {
	switch conclusion {
	case CredentialGateApplicable, CredentialGateNotApplicable,
		CredentialGateCredentialNotRegistered, CredentialGateUndecided:
		return true
	default:
		return false
	}
}

func (conclusion CredentialGateConclusion) String() string {
	switch conclusion {
	case CredentialGateApplicable:
		return "APPLICABLE"
	case CredentialGateNotApplicable:
		return "NOT_APPLICABLE"
	case CredentialGateCredentialNotRegistered:
		return "CREDENTIAL_NOT_REGISTERED"
	case CredentialGateUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// CredentialGateBasisReference 指名这次判断依据的「凭证当前可用依据的来源」——评估请求声称凭
// 什么认为凭证此刻可用（证据、附件或登记快照的引用）。与 ReadinessBasisReference 是两层：那个
// 是就绪判断指向门禁记录的引用，这个是门禁记录指向凭证证据的引用。
type CredentialGateBasisReference struct{ requiredValue }

func NewCredentialGateBasisReference(value string) (CredentialGateBasisReference, error) {
	required, err := newRequiredValue("credential gate basis reference", value)
	return CredentialGateBasisReference{required}, err
}

// ResponsibleRoleReference 指名对这次门禁判断负责的角色（UC-CC-003 范围节「保存每项门禁的……
// 责任角色」）——发起评估请求的那个角色，不是操作员登录身份。
type ResponsibleRoleReference struct{ requiredValue }

func NewResponsibleRoleReference(value string) (ResponsibleRoleReference, error) {
	required, err := newRequiredValue("responsible role reference", value)
	return ResponsibleRoleReference{required}, err
}

// CredentialGateSpec 是落成一条凭证门禁判断所需的全部输入。AsOf 是判断覆盖到的截至时点（拟使用
// 的业务时间，凭证有效期按它算）；JudgedAt 是判断发生的时刻——两者不同轴，不互相推导。
type CredentialGateSpec struct {
	Unit       DeclarationUnitID
	Credential CredentialID
	Procedure  CustomsProcedureReference
	Holder     CredentialHolderReference
	AsOf       time.Time
	Conclusion CredentialGateConclusion
	Basis      CredentialGateBasisReference
	Role       ResponsibleRoleReference
	JudgedAt   time.Time
}

// CredentialGateJudgment 是一条不可变的凭证门禁判断版本。类型上没有任何推进状态的方法：来源
// 变化让它失效是就绪判断那一层的事，重新判断是再发一次评估请求另成一版（ADR-0137 决定二）。
type CredentialGateJudgment struct {
	unit       DeclarationUnitID
	credential CredentialID
	procedure  CustomsProcedureReference
	holder     CredentialHolderReference
	asOf       time.Time
	conclusion CredentialGateConclusion
	basis      CredentialGateBasisReference
	role       ResponsibleRoleReference
	judgedAt   time.Time
}

// RecordCredentialGate 落成一条判断。八件任一缺席即拒，结论集外即拒——门禁记录是就绪判断要按
// 引用绑定的依据，缺一维的记录绑上去等于给就绪判断留了一个说不清的格。
func RecordCredentialGate(spec CredentialGateSpec) (CredentialGateJudgment, error) {
	if !spec.Unit.valid() || !spec.Credential.valid() || !spec.Procedure.valid() || !spec.Holder.valid() ||
		spec.AsOf.IsZero() || !spec.Conclusion.valid() || !spec.Basis.valid() || !spec.Role.valid() ||
		spec.JudgedAt.IsZero() {
		return CredentialGateJudgment{}, ErrInvalidCredentialGate
	}
	return CredentialGateJudgment{
		unit:       spec.Unit,
		credential: spec.Credential,
		procedure:  spec.Procedure,
		holder:     spec.Holder,
		asOf:       spec.AsOf.UTC(),
		conclusion: spec.Conclusion,
		basis:      spec.Basis,
		role:       spec.Role,
		judgedAt:   spec.JudgedAt.UTC(),
	}, nil
}

func (judgment CredentialGateJudgment) Unit() DeclarationUnitID {
	return judgment.unit
}

func (judgment CredentialGateJudgment) Credential() CredentialID {
	return judgment.credential
}

func (judgment CredentialGateJudgment) Procedure() CustomsProcedureReference {
	return judgment.procedure
}

func (judgment CredentialGateJudgment) Holder() CredentialHolderReference {
	return judgment.holder
}

// AsOf 是判断覆盖到的截至时点（AT-CC-056「保存适用性和截至时点」）。
func (judgment CredentialGateJudgment) AsOf() time.Time {
	return judgment.asOf
}

func (judgment CredentialGateJudgment) Conclusion() CredentialGateConclusion {
	return judgment.conclusion
}

func (judgment CredentialGateJudgment) Basis() CredentialGateBasisReference {
	return judgment.basis
}

func (judgment CredentialGateJudgment) Role() ResponsibleRoleReference {
	return judgment.role
}

func (judgment CredentialGateJudgment) JudgedAt() time.Time {
	return judgment.judgedAt
}
