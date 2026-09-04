package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// JudgmentPendingReason 指名本轮为何有一件事没有办完：多数取值说的是接受判断任务没能推进，
// `ControlReleasePending` 说的是决定已经成立、随附的补偿没能确定完成。两类共用一个封闭集合，
// 是因为续办引用由「原因 + 范围」派生，原因分成两套会让同一范围派生出两族互不相认的引用。
//
// 它是封闭集合而非自由文本，这样未决才能按依赖阶段分类统计——用例的可观测性一节明禁用自由
// 文本聚合原因维度。取值与产生它的那条路径同时出现。
//
// 依赖调不通也在这里取值，而不是上抛技术错误。用例把`依赖不可用`列为`尚未决定`的成因，
// 并要求该结果返回未决原因与安全续办引用，而一个 error 两样都给不出。指名是哪个依赖停了，
// 也比一句错误文本更能保住「权威答不出」与「本地坏了」的区别——后者正是上抛想护住的东西。
//
// 这不等于吞掉错误：破坏不变量的失败仍然上抛。分界与 settlement-accounting 一致——依赖
// 答不出是业务结果，取回的东西根本不属于这个请求则是编程错误。
type JudgmentPendingReason uint8

const (
	PendingReasonNone JudgmentPendingReason = iota
	CommercialBasisNotApplicable
	CommercialBasisUndetermined
	CommercialBasisUnavailable
	AdoptedCommercialBasisNotRecorded
	CommercialBasisSuperseded
	// 第三阶段取回那一步的两种拒绝。与第二阶段的 *AsOfBasisNotResolved 各自成格：运营要
	// 知道停的是哪一阶段，而续办引用由原因派生，共用会把两阶段的续办路径并成一条。
	CommercialRevalidationBasisNotResolved
	CommercialRevalidationInputNotAccepted
	ReachabilityAsOfNotDeclared
	ReachabilityAsOfBasisNotResolved
	ReachabilityAsOfNotConfigured
	ReachabilityAsOfUnavailable
	ReachabilityAsOfValueRejected
	ReachabilityAsOfInputNotAccepted
	ReachabilityAuthorityUnavailable
	ReachabilityJudgmentNotFormed
	ReachabilityRequestConflict
	ReachabilityRequestNotAccepted
	// 提交前重校可达性那一步（AT-PS-037）的五格，与商业第三阶段的分格同一套理由：`已换代`
	// 要以新时点重新请求判断，`未找回`同为重判但可观测性是关联丢失，`无法判定`与`答不出`
	// 等权威恢复，`入参未受理`要本方改引用——续办引用由原因派生，压格就会让不同的缺口共用
	// 一条引用。
	ReachabilityJudgmentSuperseded
	ReachabilityRevalidationJudgmentNotFound
	ReachabilityRevalidationUndetermined
	ReachabilityRevalidationUnavailable
	ReachabilityRevalidationInputNotAccepted
	FinancialControlAsOfNotDeclared
	FinancialControlAsOfBasisNotResolved
	FinancialControlAsOfNotConfigured
	FinancialControlAsOfUnavailable
	FinancialControlAsOfValueRejected
	FinancialControlAsOfInputNotAccepted
	FinancialControlUnavailable
	FinancialControlNotFormed
	FinancialControlRequestConflict
	FinancialControlRequestNotAccepted
	JudgmentNotRecorded
	ShipmentRequestUnavailable
	ManualReviewPolicyNotDeclared
	RecordedJudgmentsUnavailable
	DecisionIdentityUnavailable
	AcceptanceJudgmentIncomplete
	DecisionNotRecorded
	ControlReleasePending
	RejectionAuthorityUnavailable
	CustomerSupplementPending
	ManualReviewPending
	WithdrawalAuthorityUnavailable
	SourceDataAmendmentAuthorityUnavailable
	SourceDataRuleUnavailable
	SourceDataVersionIdentityUnavailable
	AmendedRequestNotSaved
	// 受控补充的两格：版本/任务身份签发不出（建单期工厂答不出，重试即可），与换代后的
	// 委托没落库（新版本要重放这次补充）。与修订那两格分开，续办引用按原因派生，共用会
	// 让两条路的续办互相认领。
	SubmissionIdentityUnavailable
	SupplementedRequestNotSaved
	SourceDataVersionNotHandedOff
	// AcceptanceDecisionNotHandedOff 说的是决定已越过提交边界、它的发布意图没能确定交出。
	// 与 ControlReleasePending 同类：不是判断没推进，是已成立决定留下的随附事项。
	AcceptanceDecisionNotHandedOff
	// StaleShipmentRequestRevision 说的是保存被并发写入抢先：这一份聚合读出来之后，有人
	// 先落了一步。它与`决定没落库`分开，因为两者的运维含义相反——一个可能是库坏了，一个是
	// 正常竞争，而续办引用由原因派生，压成一格会让两种缺口共用同一条引用。
	//
	// 四个保存调用点共用它，不按调用点拆：引用由原因**与范围**共同派生，而四处的范围本就
	// 不同（资料修订那一处取的是修订请求自己的身份加资料范围）。拆开不会让引用更可分，只会
	// 让未决统计多几行说同一件事。
	StaleShipmentRequestRevision

	// 三个授权端口各自的`授权规则未配置`。它与同一支上的 *AuthorityUnavailable 分开：后者
	// 是授权服务答不出、等它恢复，前者是这个范围此刻一条现行规则都没有、等租户把
	// `PAR-COM-14` 登记上。压成一格会对着一个没配置的租户参数无休止内部重试，而重试永远
	// 等不到一次登记——`ReachabilityAsOfNotConfigured` 早为同一个参数写过这句话。
	//
	// 三处不共用一格：续办引用由原因**与范围**共同派生，而这三支催的是三份不同的授权规则
	// （运营侧拒绝权、客户撤回权、资料修订权），共用会让运维拿一条引用查回来另一种缺口。
	RejectionAuthorityRulesNotConfigured
	WithdrawalAuthorityRulesNotConfigured
	SourceDataAmendmentAuthorityRulesNotConfigured

	// OperatorRegistrationWaitNotSaved 说的是`等待运营登记`那一停没能落库（ADR-0094 Decision 五）。
	//
	// 它与 *AsOfNotConfigured 分开，而且必须分开：那两格是消费门提交暂停的凭据，暂停没落库就交
	// 它们，等待态会随本轮回滚蒸发而投递已被记为完毕，队列上从此没有这份委托。改交本格，消费门
	// 照旧回滚重投，下一轮重新走到这里。它与 DecisionNotRecorded 同族不同格：那一格说的是决定，
	// 这一格说的是决定之前的等待态；续办引用由原因派生，共用会让两种缺口拿到同一条引用。
	OperatorRegistrationWaitNotSaved

	// judgmentPendingReasonEnd 不是一个原因，是封闭集合的上界，**必须永远排在最后**。
	//
	// 它让「每个取值都有 String()」可以被遍历检查，而那条检查堵的是一条静默链：漏补
	// String() 的取值交回空串 → recordAttempt 里 NewProcessingAttemptReason 拿空串报
	// ErrBlankValue → 处理尝试悄悄不落库，打掉用例要的「任务同时留下判断与处理尝试」；
	// 同一个空串还会进 judgmentContinuation 的摘要，让所有漏登记的原因共用一条续办引用。
	// 全程没有任何东西变红。守它的是 TestEveryPendingReasonHasAStringAndAResumePath。
	judgmentPendingReasonEnd
)

// resumePath 由未决原因导出续办方，取值与 CONTEXT 接受判断任务的三个等待态一一对应。
//
// 它是原因的全函数，而不是调用点上的常量：写成常量，新增一个原因就会静默继承上一个调用点的
// 路径，而路径错了等于催错人——依赖抖动去催客户补件，或者对着一件只有人能推进的复核无休止
// 地内部重试。
func (reason JudgmentPendingReason) resumePath() domain.ResumePath {
	switch reason {
	case CustomerSupplementPending:
		return domain.ResumeByCustomerSupplement
	case ManualReviewPending:
		return domain.ResumeByManualReview
	case RejectionAuthorityRulesNotConfigured,
		WithdrawalAuthorityRulesNotConfigured,
		SourceDataAmendmentAuthorityRulesNotConfigured,
		ReachabilityAsOfNotConfigured,
		FinancialControlAsOfNotConfigured:
		// 这一族等的是运营企业做一次登记，不是等某个权威恢复（ADR-0094）。它们与同一支上的
		// *Unavailable 只差一个词而恢复动作相反：权威答不出会自愈，而一个从未登记过的范围
		// 重试一万次也长不出一条登记。这几个取值自己的注释早写过这句话，此前却落在下面那个
		// default 的内部重试上——判据在、细分在，只是导出到恢复动作时又被压回去了。
		//
		// 逐个列举不写成按名字前缀判：命名相近不构成同一个恢复动作，而这份名单正是那条判据
		// 的落地——新增一个原因时要回答的是它等谁，不是它叫什么。
		return domain.ResumeByOperatorRegistration
	default:
		// 其余取值全是依赖答不出或声明未到，只有本方推得动。这一条不靠任何未确认规则：
		// 客户和复核角色都补不出一个查不回来的授权，或者一次没落库的保存。
		//
		// 第三阶段取回那两格是**有意**落在这里的，不是漏了：`依据未解析`的恢复动作是回第一
		// 阶段重解，而重解由本方发起；`输入未受理`是本方连身份或标识都立不起来，更只有本方
		// 改得动。客户补件与人工复核对这两者都无能为力。
		//
		// `聚合版本已过期`同样是有意落在这里。它的恢复动作是重读再重放，而四个保存调用点
		// 所在的编排**都以 FindBySourceIdentity 开头**，因此一次内部续办重入天然就重读了
		// 一遍；客户补不出一份被别人抢先写掉的版本，复核角色也补不出（ADR-0031）。
		return domain.ResumeByInternalRetry
	}
}

func (reason JudgmentPendingReason) String() string {
	switch reason {
	case CommercialBasisNotApplicable:
		return "COMMERCIAL_BASIS_NOT_APPLICABLE"
	case CommercialBasisUndetermined:
		return "COMMERCIAL_BASIS_UNDETERMINED"
	case CommercialBasisUnavailable:
		return "COMMERCIAL_BASIS_UNAVAILABLE"
	case AdoptedCommercialBasisNotRecorded:
		return "ADOPTED_COMMERCIAL_BASIS_NOT_RECORDED"
	case CommercialBasisSuperseded:
		return "COMMERCIAL_BASIS_SUPERSEDED"
	case CommercialRevalidationBasisNotResolved:
		return "COMMERCIAL_REVALIDATION_BASIS_NOT_RESOLVED"
	case CommercialRevalidationInputNotAccepted:
		return "COMMERCIAL_REVALIDATION_INPUT_NOT_ACCEPTED"
	case ReachabilityAsOfNotDeclared:
		return "REACHABILITY_AS_OF_NOT_DECLARED"
	case ReachabilityAsOfBasisNotResolved:
		return "REACHABILITY_AS_OF_BASIS_NOT_RESOLVED"
	case ReachabilityAsOfNotConfigured:
		return "REACHABILITY_AS_OF_NOT_CONFIGURED"
	case ReachabilityAsOfUnavailable:
		return "REACHABILITY_AS_OF_UNAVAILABLE"
	case ReachabilityAsOfValueRejected:
		return "REACHABILITY_AS_OF_VALUE_REJECTED"
	case ReachabilityAsOfInputNotAccepted:
		return "REACHABILITY_AS_OF_INPUT_NOT_ACCEPTED"
	case ReachabilityAuthorityUnavailable:
		return "REACHABILITY_AUTHORITY_UNAVAILABLE"
	case ReachabilityJudgmentNotFormed:
		return "REACHABILITY_JUDGMENT_NOT_FORMED"
	case ReachabilityRequestConflict:
		return "REACHABILITY_REQUEST_CONFLICT"
	case ReachabilityRequestNotAccepted:
		return "REACHABILITY_REQUEST_NOT_ACCEPTED"
	case ReachabilityJudgmentSuperseded:
		return "REACHABILITY_JUDGMENT_SUPERSEDED"
	case ReachabilityRevalidationJudgmentNotFound:
		return "REACHABILITY_REVALIDATION_JUDGMENT_NOT_FOUND"
	case ReachabilityRevalidationUndetermined:
		return "REACHABILITY_REVALIDATION_UNDETERMINED"
	case ReachabilityRevalidationUnavailable:
		return "REACHABILITY_REVALIDATION_UNAVAILABLE"
	case ReachabilityRevalidationInputNotAccepted:
		return "REACHABILITY_REVALIDATION_INPUT_NOT_ACCEPTED"
	case FinancialControlAsOfNotDeclared:
		return "FINANCIAL_CONTROL_AS_OF_NOT_DECLARED"
	case FinancialControlAsOfBasisNotResolved:
		return "FINANCIAL_CONTROL_AS_OF_BASIS_NOT_RESOLVED"
	case FinancialControlAsOfNotConfigured:
		return "FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED"
	case FinancialControlAsOfUnavailable:
		return "FINANCIAL_CONTROL_AS_OF_UNAVAILABLE"
	case FinancialControlAsOfValueRejected:
		return "FINANCIAL_CONTROL_AS_OF_VALUE_REJECTED"
	case FinancialControlAsOfInputNotAccepted:
		return "FINANCIAL_CONTROL_AS_OF_INPUT_NOT_ACCEPTED"
	case FinancialControlUnavailable:
		return "FINANCIAL_CONTROL_UNAVAILABLE"
	case FinancialControlNotFormed:
		return "FINANCIAL_CONTROL_NOT_FORMED"
	case FinancialControlRequestConflict:
		return "FINANCIAL_CONTROL_REQUEST_CONFLICT"
	case FinancialControlRequestNotAccepted:
		return "FINANCIAL_CONTROL_REQUEST_NOT_ACCEPTED"
	case JudgmentNotRecorded:
		return "JUDGMENT_NOT_RECORDED"
	case ShipmentRequestUnavailable:
		return "SHIPMENT_REQUEST_UNAVAILABLE"
	case ManualReviewPolicyNotDeclared:
		return "MANUAL_REVIEW_POLICY_NOT_DECLARED"
	case RecordedJudgmentsUnavailable:
		return "RECORDED_JUDGMENTS_UNAVAILABLE"
	case DecisionIdentityUnavailable:
		return "DECISION_IDENTITY_UNAVAILABLE"
	case AcceptanceJudgmentIncomplete:
		return "ACCEPTANCE_JUDGMENT_INCOMPLETE"
	case DecisionNotRecorded:
		return "DECISION_NOT_RECORDED"
	case ControlReleasePending:
		return "CONTROL_RELEASE_PENDING"
	case RejectionAuthorityUnavailable:
		return "REJECTION_AUTHORITY_UNAVAILABLE"
	case CustomerSupplementPending:
		return "CUSTOMER_SUPPLEMENT_PENDING"
	case ManualReviewPending:
		return "MANUAL_REVIEW_PENDING"
	case WithdrawalAuthorityUnavailable:
		return "WITHDRAWAL_AUTHORITY_UNAVAILABLE"
	case SourceDataAmendmentAuthorityUnavailable:
		return "SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE"
	case SourceDataRuleUnavailable:
		return "SOURCE_DATA_RULE_UNAVAILABLE"
	case SourceDataVersionIdentityUnavailable:
		return "SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE"
	case AmendedRequestNotSaved:
		return "AMENDED_REQUEST_NOT_SAVED"
	case SubmissionIdentityUnavailable:
		return "SUBMISSION_IDENTITY_UNAVAILABLE"
	case SupplementedRequestNotSaved:
		return "SUPPLEMENTED_REQUEST_NOT_SAVED"
	case SourceDataVersionNotHandedOff:
		return "SOURCE_DATA_VERSION_NOT_HANDED_OFF"
	case AcceptanceDecisionNotHandedOff:
		return "ACCEPTANCE_DECISION_NOT_HANDED_OFF"
	case StaleShipmentRequestRevision:
		return "STALE_SHIPMENT_REQUEST_REVISION"
	case RejectionAuthorityRulesNotConfigured:
		return "REJECTION_AUTHORITY_RULES_NOT_CONFIGURED"
	case WithdrawalAuthorityRulesNotConfigured:
		return "WITHDRAWAL_AUTHORITY_RULES_NOT_CONFIGURED"
	case SourceDataAmendmentAuthorityRulesNotConfigured:
		return "SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED"
	case OperatorRegistrationWaitNotSaved:
		return "OPERATOR_REGISTRATION_WAIT_NOT_SAVED"
	default:
		return ""
	}
}

// ErrUnexpectedAsOfOutcome 说明第二阶段端口交回了封闭集合以外的答复。它上抛而不形成未决：
// 依赖答不出是业务结果，答出一个不属于这个集合的东西则是端口坏了（见 JudgmentPendingReason
// 的分界）。
var ErrUnexpectedAsOfOutcome = errors.New("parcel shipment: unexpected judgment as-of outcome")

// ErrUnexpectedAssessmentOutcome 说明权威判断端口交回了封闭集合以外的答复。与上一条同一
// 分界：那不是「权威答不出」，是端口本身坏了。
var ErrUnexpectedAssessmentOutcome = errors.New("parcel shipment: unexpected authority assessment outcome")

// ErrUnexpectedRevalidationOutcome 同上，说的是提交前重校验那一支。
var ErrUnexpectedRevalidationOutcome = errors.New("parcel shipment: unexpected commercial revalidation outcome")

// ErrUnexpectedSaveOutcome 同上，说的是委托聚合的写入那一步。
var ErrUnexpectedSaveOutcome = errors.New("parcel shipment: unexpected shipment request save outcome")

// ErrUnexpectedAuthorizationOutcome 同上，说的是三个授权端口。三处共用一个哨兵，因为它们
// 共用同一个封闭集合：日后多一种答复，三个调用点会一起报错，而不是各自静默归入某一格。
var ErrUnexpectedAuthorizationOutcome = errors.New("parcel shipment: unexpected authorization outcome")

// saveStallReason 把一次没能落库的写入结果译成本层的未决原因。
//
// 逐取值分派，不留兜底（ADR-0031）：端口日后新增一个写入结果时这里会报错而不是静默继承
// 某一格，而那一格决定的是运维该去查库还是该等下一轮重放。
//
// `已保存`落在 default 是有意的：它根本不该走到这里——调用方拿着它继续办事，把它译成一个
// 未决原因会让一次成功的保存看起来像停滞。
func saveStallReason(outcome ports.ShipmentRequestSaveOutcome) (JudgmentPendingReason, error) {
	switch outcome {
	case ports.ShipmentRequestRevisionConflict:
		return StaleShipmentRequestRevision, nil
	default:
		return PendingReasonNone, ErrUnexpectedSaveOutcome
	}
}

// commercialBasisPendingReason 把一次非唯一的商业解析分到它自己的未决原因上。
//
// `确定不适用`是权威说了这个范围没有适用依据，`解析未决`是权威根本没得出答案。两者压成一格
// 会让一次读取失败看起来像这个客户没有合同，而后者是能拿去拒单的结论。
//
// 更细的分别（`无适用依据`、`适用冲突`、`输入未受理`、`已失效`）由 party-commercial 的原因
// 引用带过来，本上下文不重新声明一套口径；引用参与续办派生，因此四者的续办引用仍然互不相同，
// 一次越权探测不会与一次权威读不到共用一条续办路径。
//
// 非法取值落在`解析未决`一侧。它不该出现——端口交回的适用性译不出时 CommercialBasisChecksFor
// 会先失败——而落错这个方向只会多停一轮，不会放行任何东西。
func commercialBasisPendingReason(applicability domain.CommercialApplicability) JudgmentPendingReason {
	if applicability == domain.CommerciallyNotApplicable {
		return CommercialBasisNotApplicable
	}
	return CommercialBasisUndetermined
}

// asOfPendingReasons 是一类判断在第二阶段四种未成形上各自的未决原因。
//
// 两类判断各配一套而不共用：运营要知道停的是哪一项判断的时点，而续办引用由原因派生——共用
// 一套会让可达性与财务控制停在同一处时拿到同一个引用，两条各自的续办路径就此并成一条。
type asOfPendingReasons struct {
	basisNotResolved JudgmentPendingReason
	notConfigured    JudgmentPendingReason
	unavailable      JudgmentPendingReason
	valueRejected    JudgmentPendingReason
	inputNotAccepted JudgmentPendingReason
}

// forOutcome 逐取值分派，不留兜底。提供方日后新增一个未成形取值时这里会返回错误而不是静默
// 继承某一格——那一格决定的是催谁，催错人比停下来更难发现。
func (reasons asOfPendingReasons) forOutcome(outcome ports.JudgmentAsOfOutcome) (JudgmentPendingReason, error) {
	switch outcome {
	case ports.JudgmentAsOfBasisNotResolved:
		return reasons.basisNotResolved, nil
	case ports.JudgmentAsOfNotConfigured:
		return reasons.notConfigured, nil
	case ports.JudgmentAsOfPending:
		return reasons.unavailable, nil
	case ports.JudgmentAsOfValueRejected:
		return reasons.valueRejected, nil
	case ports.JudgmentAsOfInputNotAccepted:
		// 提供方在任何查询发生之前就短路拒绝了：身份或解析标识立不起来。这是本上下文自己
		// 的缺陷，不是权威答不出，因此它与`未决`分开——重试改不了一份立不起来的入参。
		//
		// 它不再包括「指名了一份不属于自己的解析」：越权那一支按 ADR-0029 改落`依据未解析`，
		// 与「查无此解析」同格。本落点仍有来源（短路支），因此保留。
		return reasons.inputNotAccepted, nil
	default:
		return PendingReasonNone, ErrUnexpectedAsOfOutcome
	}
}

// commercialBasisScope 是两个判断编排共有的那部分范围。可达性那一支还带声明包裹，但前半段
// 用不到它——商业依据与逐项时点都按提交版本取，不按成员取。
type commercialBasisScope struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

// basisStall 说明前半段停在哪里。零值表示没停；scope 是要额外参与续办派生的东西。
type basisStall struct {
	reason JudgmentPendingReason
	scope  []string
}

func (stall basisStall) stopped() bool {
	return stall.reason != PendingReasonNone
}

// adoptedBasis 是前半段的产物：本轮采用的商业依据，以及该类判断经权威回显的时点。
type adoptedBasis struct {
	snapshot domain.CommercialBasisSnapshot
	asOf     domain.JudgmentAsOf
}

// formAdoptedBasis 走完两个判断编排共有的前半段：解析商业依据、记下所采用的那一次、取得本类
// 判断的时点声明，再由第二阶段形成经回显的时点。
//
// 两个编排共用它而不是各留一份：这一段每一步的顺序都有理由——先记标识再形成时点，否则提交
// 决定前无从按原依据重解；值不在编排里形成，否则就是拿本地时钟顶替声明的语义。复制一份等于
// 把这些理由也复制一份，而下一次只会有一份被改。
//
// 它不碰权威判断本身：哪个取值算失败是各自的接受语言，留在各自的编排里。
func formAdoptedBasis(
	ctx context.Context,
	commercial ports.CommercialBasisResolver,
	recorder ports.AcceptanceJudgmentRecorder,
	scope commercialBasisScope,
	kind domain.JudgmentKind,
	asOfNotDeclared JudgmentPendingReason,
	reasons asOfPendingReasons,
) (adoptedBasis, basisStall, error) {
	resolution, err := commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          scope.Identity,
		ShipmentRequestID: scope.ShipmentRequestID,
		SubmissionVersion: scope.SubmissionVersion,
	})
	if err != nil {
		return adoptedBasis{}, basisStall{reason: CommercialBasisUnavailable}, nil
	}
	if resolution.Applicability != domain.CommerciallyApplicable {
		// 原因引用参与续办派生：`输入未受理`与`权威读不到`都落在`解析未决`，但两者要办的事
		// 不同，共用一个引用会让调用方按引用查回来的是另一种缺口。
		return adoptedBasis{}, basisStall{
			reason: commercialBasisPendingReason(resolution.Applicability),
			scope:  []string{resolution.Reason.String()},
		}, nil
	}
	snapshot := resolution.Snapshot

	// 采用哪次解析要先记下，再据它形成时点。顺序不能反：一次已经用来形成时点的解析若没留住
	// 标识，提交决定前就无从按它重解，而判断正是在它的时点策略下形成的。
	if err := recorder.RecordAdoptedCommercialResolution(
		ctx,
		scope.Identity.TenantID(),
		scope.ShipmentRequestID,
		snapshot.ResolutionID(),
	); err != nil {
		return adoptedBasis{}, basisStall{reason: AdoptedCommercialBasisNotRecorded}, nil
	}

	declared, present := snapshot.DeclaredAsOfFor(kind)
	if !present {
		return adoptedBasis{}, basisStall{reason: asOfNotDeclared}, nil
	}

	// 第二阶段：值按声明的语义在适配器里形成，再由 party-commercial 校验回显。编排不参与
	// 形成——它手上只有本地时钟，而用例明禁用一个全局时间代替逐项时点。
	formation, err := commercial.FormJudgmentAsOf(ctx, ports.JudgmentAsOfQuery{
		Identity:          scope.Identity,
		ShipmentRequestID: scope.ShipmentRequestID,
		SubmissionVersion: scope.SubmissionVersion,
		Resolution:        snapshot.ResolutionID(),
		Declared:          declared,
	})
	if err != nil {
		return adoptedBasis{}, basisStall{reason: reasons.unavailable}, nil
	}
	if formation.Outcome != ports.JudgmentAsOfFormed {
		reason, err := reasons.forOutcome(formation.Outcome)
		if err != nil {
			return adoptedBasis{}, basisStall{}, err
		}
		return adoptedBasis{}, basisStall{reason: reason}, nil
	}

	return adoptedBasis{snapshot: snapshot, asOf: formation.AsOf}, basisStall{}, nil
}

// awaitOperatorRegistration 在**决定形成之前**把委托停在`等待运营登记`并落库，再交回本轮该报的
// 原因（ADR-0094 Decision 五）。
//
// 只对续办路径为`等待运营登记`的停顿动手，判据取自 ResumePath 而不是原因的名字：谁该落等待态
// 是恢复动作那一层的知识，在这里按名字再列一遍就是第二处定义。其余未决要么由信封重投自然再驱
// （内部重试）、要么由 Decide 看过校验后写下并各自落库（等待人工复核走 pauseForManualReview），
// 这里不重做那份判断，也不多读一次聚合。
//
// 等待态没落库时**不**交回原停顿原因：那一格是消费门（Decision 四落地后）提交暂停的凭据，暂停
// 没落库就交它，等待态会随本轮回滚蒸发而投递已被记为完毕，队列从此列不出这份委托。三种没落库
// 的样子各交回自己那一格（委托查不到、保存没落库、版本被抢先），全部归内部重试，照旧回滚重投，
// 下一轮重新走到这里——与 pauseForManualReview 对`等待人工复核`的处置同一判断。
//
// 转移被聚合拒绝（已越过决定边界、任务已停）时原因照交不改：那份委托已经不在`已提交`的判断路上，
// 没有等待态要落，也没有队列条目要保；本轮真实停在哪一步仍由原因说出来。随后 recordAttempt 对
// 已完成的任务同样会被拒，两处是同一道门。
func awaitOperatorRegistration(
	ctx context.Context,
	requests ports.ShipmentRequestRepository,
	identity domain.SourceIdentity,
	stall JudgmentPendingReason,
) (JudgmentPendingReason, error) {
	if stall.resumePath() != domain.ResumeByOperatorRegistration {
		return stall, nil
	}
	request, found, err := requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		return ShipmentRequestUnavailable, nil
	}
	waiting, err := request.AwaitOperatorRegistration()
	if err != nil {
		return stall, nil
	}
	saved, err := requests.Save(ctx, identity, waiting)
	if err != nil {
		return OperatorRegistrationWaitNotSaved, nil
	}
	if saved != ports.ShipmentRequestSaved {
		return saveStallReason(saved)
	}
	return stall, nil
}

// recordAttempt 把没能推进的这一轮追加到接受判断任务上。用例要求任务同时留下判断与处理
// 尝试，只留成功的判断会让一份卡了十轮的委托看起来和刚建单的一样。
//
// 记录失败不改写本轮的未决原因：原因说的是判断为何没推进，用「记录失败」顶替它会把真实
// 缺口藏起来，而调用方正是按原因决定该补缺口还是该重试依赖。这条记录本身按同一续办引用
// 补写。
//
// 续办路径由原因导出，不在这里给定值：谁能补上这个缺口是原因自带的属性，写死在记录处会让
// 同一个原因在不同调用点走不同路径。
func recordAttempt(
	ctx context.Context,
	recorder ports.AcceptanceJudgmentRecorder,
	clock ports.Clock,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	reason JudgmentPendingReason,
	continuation domain.OwnershipContinuationReference,
) {
	// 只有空串会走到这里，而空串只可能来自一个漏补 String() 的取值——那是编程错误，不是
	// 依赖答不出，因此它不该变成一条记着空原因的处理尝试。它由 judgmentPendingReasonEnd 那
	// 条遍历用例在写下当天就拦住，所以这一支在跑起来时是够不到的。
	attemptReason, err := domain.NewProcessingAttemptReason(reason.String())
	if err != nil {
		return
	}
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       attemptReason,
		ResumePath:   reason.resumePath(),
		Continuation: continuation,
		AttemptedAt:  clock.Now(),
	})
	if err != nil {
		return
	}
	_ = recorder.RecordProcessingAttempt(ctx, tenant, requestID, attempt)
}

// judgmentContinuation 由未决原因与判断范围共同派生，因此同一范围因同一原因停滞时拿到的
// 引用始终相同——这正是调用方能查询原次尝试而不必靠猜的原因。原因参与派生也意味着停在
// 不同阶段的两次未决给出不同引用，用例要求二者分别统计、各走各的续办路径。
//
// 「不同原因给出不同引用」立在 String() 对封闭集合单射上：漏补的空串或抄重的名字都会让两个
// 原因拼出同一份摘要，安静地建出一条撞车的引用。尾部那个错误分支对此拦不住——"CONT-"+hex
// 永远非空，引用永远建得出来，只是建错了。单射由
// TestEveryPendingReasonHasAStringAndAResumePath 守住，不靠这条注释。
func judgmentContinuation(reason JudgmentPendingReason, scope ...string) domain.OwnershipContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join(append([]string{reason.String()}, scope...), "\x00")))
	continuation, err := domain.NewOwnershipContinuationReference("CONT-" + hex.EncodeToString(digest[:8]))
	if err != nil {
		return domain.OwnershipContinuationReference{}
	}
	return continuation
}
