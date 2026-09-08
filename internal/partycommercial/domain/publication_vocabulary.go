package domain

import "math"

// 本文件是商业发布词表读口的领域半边（票 admin-write-faces/20，通道 1 代裁）：按册答正文里各封闭集的码，
// 供运营操作者面的发布表单做下拉，表单因此不内置枚举。
//
// 集合的唯一权威是本包既有的领域枚举：每个集合都从同一个枚举的接受判据（valid / declarable / Declared）与
// String() 逐值列出，不另写一份常量表。另一份表就是同一封闭集的第二种写法，漂了无人报——而 ADR-0126 之后
// 正文由服务端按册规范化，表单若自带一份枚举正是那第二份；本口存在的理由就是让它不必存在。
//
// 只答码不答中文。中文留在 admin-web 各页词表（「集外取值原样示出」的既有约定）：服务端给中文会让呈现层的词
// 进本上下文，所有权错位；给码，表单拿到的正是各册构造门接受的那些词。

// VocabularySet 是一册正文里一个封闭集的全部码。Name 就是载荷里那格的键名（接单规则包的 `stage` / `intent`、
// 结算政策的 `method`……），Codes 按领域枚举的声明顺序排列——一字不多不少地等于该格构造门接受的那些词。
type VocabularySet struct {
	Name  string
	Codes []string
}

// PublicationVocabulary 按册答正文里的全部封闭集。
//
// kind 合法而该册正文没有枚举格（服务产品、客户合同……）答空列表：kind 合法只是没词，不是缺陷，也不是 404。
// kind 不在 CommercialObjectKind 内答 ErrInvalidCommercialVersion，与 CanonicalizePublicationContent 对坏 kind
// 的答复同一格——「没这一册」与「这一册没词」要人做的事不同，不折成一份空列表。
//
// 列哪几册、每册列哪几格，以第 2 波表单票（admin-write-faces/12、13、15、17）票面点名的载荷为准：那几格里凡是
// 领域枚举的都在；开放引用（规则引用、时点语义、资料组、费用范围、责任方、终局类型……）不是封闭集，不在。
// 其余各册今天要么正文没有枚举格，要么表单票不经词表，都答空列表；某册日后要经词表，在这里加一支 case，不换形。
func PublicationVocabulary(kind CommercialObjectKind) ([]VocabularySet, error) {
	if !kind.valid() {
		return nil, ErrInvalidCommercialVersion
	}
	switch kind {
	case AcceptanceRulePackageObject:
		// 票 12「册与载荷」逐节：正文 rules[].category；asOfPolicies[].judgment；acceptanceContent 的
		// applicableGroups[] 与 manualReview；intakeQualification.sources[]；finalRules[].outcome（finalKind 是
		// 开放引用）；finalRuleValidity.anchor；sourceDataAmendment.rules[] 的 stage / intent / allowance。
		// manualReview 与 allowance 只列可登成一格的值：「未声明」是缺格的读法，不是一行能选的取值（ADR-0120
		// Decision 三），构造门也不收它。
		return []VocabularySet{
			{Name: "category", Codes: closedCodes(RuleCategory.valid, RuleCategory.String)},
			{Name: "judgment", Codes: closedCodes(JudgmentType.valid, JudgmentType.String)},
			{Name: "applicableGroups", Codes: closedCodes(AcceptanceCheckGroupType.valid, AcceptanceCheckGroupType.String)},
			{Name: "manualReview", Codes: closedCodes(ManualReviewDirective.Declared, ManualReviewDirective.String)},
			{Name: "sources", Codes: closedCodes(DeclaredIntakeSource.valid, DeclaredIntakeSource.String)},
			{Name: "outcome", Codes: closedCodes(DeclaredResponsibilityOutcome.valid, DeclaredResponsibilityOutcome.String)},
			{Name: "anchor", Codes: closedCodes(ValidityAnchorKind.valid, ValidityAnchorKind.String)},
			{Name: "stage", Codes: closedCodes(DeclaredAmendmentStage.valid, DeclaredAmendmentStage.String)},
			{Name: "intent", Codes: closedCodes(DeclaredAmendmentIntent.valid, DeclaredAmendmentIntent.String)},
			{Name: "allowance", Codes: closedCodes(AmendmentAllowance.declarable, AmendmentAllowance.String)},
		}, nil
	case PreAcceptanceFinancialControlPolicyObject:
		// 票 13：preAcceptanceFinancialControlPolicyBody 的 jointPassCondition 与 controls[] 的 control / onFailure，
		// 键名照批文（与读面 query_commercial_policies.go 的控制项键同名）。控制种类里没有「无控制」那一格
		// （ADR-0115 Decision 一），枚举里没有，这里自然也没有。
		return []VocabularySet{
			{Name: "jointPassCondition", Codes: closedCodes(JointPassCondition.valid, JointPassCondition.String)},
			{Name: "control", Codes: closedCodes(PreAcceptanceControlKind.valid, PreAcceptanceControlKind.String)},
			{Name: "onFailure", Codes: closedCodes(ControlFailureDisposition.valid, ControlFailureDisposition.String)},
		}, nil
	case SettlementPolicyObject:
		// 票 15：settlementPolicyBody.method（预付 / 账期一族）。
		return []VocabularySet{
			{Name: "method", Codes: closedCodes(SettlementMethod.valid, SettlementMethod.String)},
		}, nil
	case AuthorizationRuleObject:
		// 票 17：cancellationAuthority[].party（请求方）。授权授予册的 AuthorizedAction 不经这条发布路，不在。
		return []VocabularySet{
			{Name: "party", Codes: closedCodes(DeclaredCancellationParty.valid, DeclaredCancellationParty.String)},
		}, nil
	default:
		return []VocabularySet{}, nil
	}
}

// closedCodes 从一个 uint8 枚举里按声明顺序列出被 accepts 接受的每个值的原词。
//
// 它扫整个 uint8 值域而不是「首格到末格」：集合的边界就是接受判据的边界，这里不另抄一遍首末常量——抄了
// 就又是一份要人记得同步的表。收方法表达式（RuleCategory.valid 那种）而不是收接口，是因为两个判据的名字
// 各族不同（valid / declarable / Declared），而那正是各族对「零值是不是一格取值」各自的立场，不该为了
// 塞进一个接口而统一。
func closedCodes[Code ~uint8](accepts func(Code) bool, name func(Code) string) []string {
	codes := make([]string, 0)
	for raw := 0; raw <= math.MaxUint8; raw++ {
		code := Code(raw)
		if accepts(code) {
			codes = append(codes, name(code))
		}
	}
	return codes
}
