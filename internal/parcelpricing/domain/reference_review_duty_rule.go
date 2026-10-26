package domain

import "errors"

// 参考复核职责规则（ADR-0262 决定一）：租户对计价参考序列与目录人工复核的治理规则，一租户一条，序列与目录共用——两者复核
// 的都是一次外部事实的转录（ADR-0109 决定二）。它与价卡发布审批职责规则分立：租户对一周一期、常由一个人照公布页转录的
// 燃油率，与对一年几版、要经商务批准的价卡，要求可以不同。形照 PriceCardApprovalDutyRule，复核者主体与授予用同一套
// OperatorSubject 与 OperatorGrant。

var (
	// ErrInvalidReferenceReviewDutyRule 表示规则立不住：没有租户，或要求的授予是空名。
	ErrInvalidReferenceReviewDutyRule = errors.New("parcel pricing: invalid reference review duty rule")
	// ErrReferenceReviewDutyRuleTenantMismatch 拦的是拿他租户的规则裁本租户的复核——租户是身份不是过滤器，这只可能是装配或
	// 读口的错，不是业务答案。
	ErrReferenceReviewDutyRuleTenantMismatch = errors.New("parcel pricing: the reference review duty rule belongs to another tenant")
	// ErrReferenceReviewerLacksRequiredGrant 是规则「复核者须持某一格授予」那一格拒下的形状：复核者的授予集里没有它。恢复动作
	// 是换一个持有它的人经操作者面来，或租户改规则——都不是改载荷。与「复核者就是登记者」分格。
	ErrReferenceReviewerLacksRequiredGrant = errors.New("parcel pricing: the reviewer does not hold the grant the reference review duty rule requires")
)

// ReferenceReviewDutyRule 声明两格：复核者须否与该版本的登记者为不同主体、复核者须持哪一格授予。两格都不要求也是一条合法的
// 租户声明——单人可复核由租户说出，不由系统默认；规则未登记时人工复核答`未配置`、不放行，产品不写死双人也不写死单人。
type ReferenceReviewDutyRule struct {
	tenant           TenantID
	distinctSubjects bool
	requiredGrant    OperatorGrant
}

// NewReferenceReviewDutyRule 立一条规则。requiredGrant 零值即不要求授予。
func NewReferenceReviewDutyRule(tenant TenantID, distinctSubjects bool, requiredGrant OperatorGrant) (ReferenceReviewDutyRule, error) {
	if !tenant.valid() || (requiredGrant != OperatorGrant{} && !requiredGrant.valid()) {
		return ReferenceReviewDutyRule{}, ErrInvalidReferenceReviewDutyRule
	}
	return ReferenceReviewDutyRule{tenant: tenant, distinctSubjects: distinctSubjects, requiredGrant: requiredGrant}, nil
}

func (rule ReferenceReviewDutyRule) Tenant() TenantID { return rule.tenant }

func (rule ReferenceReviewDutyRule) RequiresDistinctSubjects() bool { return rule.distinctSubjects }

// RequiredGrant 交回规则要求复核者持有的授予；第二个返回值为假即不要求。
func (rule ReferenceReviewDutyRule) RequiredGrant() (OperatorGrant, bool) {
	return rule.requiredGrant, rule.requiredGrant.valid()
}

func (rule ReferenceReviewDutyRule) valid() bool { return rule.tenant.valid() }

// admit 按规则裁一位人工复核者。sameSubject 是调用方那一种复核的「复核者就是登记者」哨兵，序列与目录各有各的，恢复动作
// 相同而出处要分得开。
//
// 授予先于主体比较：受控 CLI 的复核者只是批文里的一个串、带不出授予集，规则要求某一格授予时这一路不论复核者是谁都一律答
// 复核者不合格（ADR-0262 决定一）。先比主体的话，登记者本人经 CLI 复核会答须换人——换了人照样过不去，那一格指错了恢复动作。
// 两格都不满足时，要找的人本就得既是另一个人、又持有那一格，先答授予不丢任何续办信息。
func (rule ReferenceReviewDutyRule) admit(tenant TenantID, registrant string, reviewer OperatorSubject, sameSubject error) error {
	if !rule.valid() {
		return ErrInvalidReferenceReviewDutyRule
	}
	if rule.tenant != tenant {
		return ErrReferenceReviewDutyRuleTenantMismatch
	}
	if grant, required := rule.RequiredGrant(); required && !reviewer.Holds(grant) {
		return ErrReferenceReviewerLacksRequiredGrant
	}
	if rule.distinctSubjects && reviewer.reference == registrant {
		return sameSubject
	}
	return nil
}
