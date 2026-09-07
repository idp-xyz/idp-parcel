package domain

// AmendmentStage 是「资料修订阶段」的封闭六格（PS CONTEXT 词条；取值与次序照 `UC-PS-002`
// 「资料范围与阶段边界」那张表的原词，一格不拆、一格不加）。它是允许矩阵的一维：矩阵按
// （资料组 × 阶段 × 意图）登记「能不能这样改」，阶段判不出，那一格就无从查起。
//
// 零值是`判不出`而不是最早的那一格。让零值落在「已接受、尚未收寄」上，一个读面还没接上的
// 邻接上下文会让每一次修订都被当成最早阶段去问矩阵——那正是 `BD-PS-010`「未登记的字段和
// 阶段不默认允许」在阶段这一维上的同一个错：不猜一个阶段去放行。
type AmendmentStage uint8

const (
	AmendmentStageUndetermined AmendmentStage = iota
	// StageAcceptedNotYetReceived 已接受、尚未收寄。
	StageAcceptedNotYetReceived
	// StageReceivedOrMeasured 已收寄或已测量。
	StageReceivedOrMeasured
	// StageLabelledOrBagged 已制签或已装袋。
	StageLabelledOrBagged
	// StageCustomsDataFormingNotSubmitted 关务资料形成中、尚未提交。
	StageCustomsDataFormingNotSubmitted
	// StageCustomsSubmitted 已提交关务。
	StageCustomsSubmitted
	// StageCaseClosedOrServiceCompleted 案件已关闭或服务已完成。
	StageCaseClosedOrServiceCompleted
)

func (stage AmendmentStage) String() string {
	switch stage {
	case StageAcceptedNotYetReceived:
		return "ACCEPTED_NOT_YET_RECEIVED"
	case StageReceivedOrMeasured:
		return "RECEIVED_OR_MEASURED"
	case StageLabelledOrBagged:
		return "LABELLED_OR_BAGGED"
	case StageCustomsDataFormingNotSubmitted:
		return "CUSTOMS_DATA_FORMING_NOT_SUBMITTED"
	case StageCustomsSubmitted:
		return "CUSTOMS_SUBMITTED"
	case StageCaseClosedOrServiceCompleted:
		return "CASE_CLOSED_OR_SERVICE_COMPLETED"
	default:
		return ""
	}
}

// Determined 报告阶段是否已判出。零值不是一种阶段——它是「事实不够、说不出在哪」。
func (stage AmendmentStage) Determined() bool {
	return stage >= StageAcceptedNotYetReceived && stage <= StageCaseClosedOrServiceCompleted
}

// StageFact 是判断阶段所用的一项事实的三态：在、不在、不知道。
//
// 三态而不是布尔，是因为「不知道」与「不在」在这里必须分得开：一个还没接上读面的邻接上下文
// 答不出「有没有」，把它读成「没有」就是把最早阶段偷偷设成了默认值。零值取`不知道`：漏填一项
// 事实的后果是判不出阶段（停在未决），而不是判出一个偏早的阶段。
type StageFact uint8

const (
	StageFactUnknown StageFact = iota
	StageFactAbsent
	StageFactPresent
)

func (fact StageFact) String() string {
	switch fact {
	case StageFactAbsent:
		return "ABSENT"
	case StageFactPresent:
		return "PRESENT"
	default:
		return ""
	}
}

// EitherStageFact 把两项事实按「任一成立即成立」合成一项：`UC-PS-002` 有两格各由两个上下文的
// 事实并成（「已制签或已装袋」是本上下文的面单结果与节点作业的装袋；「案件已关闭或服务已完成」
// 是关务的案件关闭与本上下文的包裹终局）。任一在即在；两者都不在才不在；其余是不知道——一边
// 不在、另一边不知道时，合成的那一格仍然判不出，不因为已知的那半是「不在」就当整格不在。
func EitherStageFact(first, second StageFact) StageFact {
	switch {
	case first == StageFactPresent || second == StageFactPresent:
		return StageFactPresent
	case first == StageFactAbsent && second == StageFactAbsent:
		return StageFactAbsent
	default:
		return StageFactUnknown
	}
}

// AmendmentStageEvidence 是判断一个包裹所处资料修订阶段的六项事实，一项对应一格。事实由谁
// 出在各字段注释里写明；本类型不管它们从哪个端口读来，只管按次序判。
type AmendmentStageEvidence struct {
	// Accepted 委托已接受（接受基线已固定）。本上下文自有事实。
	Accepted StageFact
	// Received 该包裹已形成有效网络收寄采用（责任起点在）。本上下文自有事实；「已测量」
	// 那半是节点作业的实测，它不单独改变阶段——测量发生在收寄之后，收寄在即已进这一格。
	Received StageFact
	// LabelledOrBagged 该包裹已有面单交易结果（本上下文），或已装入集运单元（节点作业）。
	LabelledOrBagged StageFact
	// CustomsDataForming 关务已为该包裹形成申报资料且尚未提交。关务事实。
	CustomsDataForming StageFact
	// CustomsSubmitted 关务已为该包裹提交申报。关务事实。
	CustomsSubmitted StageFact
	// CaseClosedOrServiceCompleted 关务案件已关闭（关务），或该包裹已形成终局服务结果
	// （本上下文）。
	CaseClosedOrServiceCompleted StageFact
}

// JudgeAmendmentStage 按六格次序判出阶段：从最后一格往前看，第一格`在`的事实就是所处阶段，
// 前提是它后面的每一格都已知`不在`。
//
// 靠后的格压过靠前的格，是这张表的读法而不是本函数的发明：CONTEXT 把「已收寄、制签、装袋、
// 运输、申报、提交、案件关闭或服务完成后收到的客户更正」按这个次序排成一句，`UC-PS-002` 的表
// 也按它分行——一个包裹既已收寄又已提交关务时，允许什么由「已提交关务」那一行说，收寄那一行
// 管不到它。
//
// 途中遇到任何一格`不知道`就判不出：那一格若其实`在`，阶段就该是它而不是更早的那格，而我们
// 无从排除。这一句就是「判不出阶段不猜、不默认最早阶段」在代码里的全部落点。第一格自己也要
// `在`：委托没接受，本用例根本不成立，交回判不出由调用方按`委托未接受`上抛。
func JudgeAmendmentStage(evidence AmendmentStageEvidence) AmendmentStage {
	ordered := [...]struct {
		fact  StageFact
		stage AmendmentStage
	}{
		{evidence.CaseClosedOrServiceCompleted, StageCaseClosedOrServiceCompleted},
		{evidence.CustomsSubmitted, StageCustomsSubmitted},
		{evidence.CustomsDataForming, StageCustomsDataFormingNotSubmitted},
		{evidence.LabelledOrBagged, StageLabelledOrBagged},
		{evidence.Received, StageReceivedOrMeasured},
		{evidence.Accepted, StageAcceptedNotYetReceived},
	}
	for _, step := range ordered {
		switch step.fact {
		case StageFactPresent:
			return step.stage
		case StageFactAbsent:
			continue
		default:
			return AmendmentStageUndetermined
		}
	}
	return AmendmentStageUndetermined
}

// FurthestAmendmentStage 把同一份委托下多个包裹各自的阶段合成委托级阶段：取最靠后的那一格。
//
// 作用于整份委托的资料范围（寄件人一类）没有自己的包裹，它所处的阶段只能从成员推：同一句
// 「靠后的格压过靠前的格」在成员之间同样成立——一个成员已提交关务，改整份委托的寄件人就
// 动到了那份已提交的申报，允许什么得由最靠后的那一行说。任一成员判不出，委托级就判不出；
// 没有成员同样判不出（接受基线至少含一个包裹，空集合说明调用方拿错了对象）。
func FurthestAmendmentStage(stages ...AmendmentStage) AmendmentStage {
	if len(stages) == 0 {
		return AmendmentStageUndetermined
	}
	furthest := AmendmentStageUndetermined
	for _, stage := range stages {
		if !stage.Determined() {
			return AmendmentStageUndetermined
		}
		if stage > furthest {
			furthest = stage
		}
	}
	return furthest
}
