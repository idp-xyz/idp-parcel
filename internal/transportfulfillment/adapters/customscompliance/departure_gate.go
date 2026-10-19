// Package customscompliance 是 transport-fulfillment 消费 customs-compliance 的适配器（ADR-0025 消费方侧）：出发登记
// 时向关务的动作准入判断问「这一件此刻能不能从 A 出发到 B」（ADR-0205、ADR-0206），交接登记时问「这一件此刻能不能在
// A 交出、往 B 去」（ADR-0213 决定四）。只翻译不判断——三源怎么折、适用哪些受门禁动作、跨不跨关务区域，都由关务答；
// 本包只把答复按格译回、把依据原文带回。
package customscompliance

import (
	"context"
	"fmt"

	ccapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：一侧交出了本适配器词汇表之外的东西，是编程错误而
// 不是业务答案。它以 error 交出、不冒充关务的读不动，并包住两个端口各自的译不回来（出发与交出）：事实照记、那一件记
// 读不动，但依据种类与调用失败分开——这一类重问一万次也是同一个结果，要查的是两边的口径。两个端口共用一份对照（answerFor、
// basesFor），译不回来也就是同一件事，两边各认各的哨兵即可。
var ErrUntranslatableAnswer = fmt.Errorf("transport fulfillment customscompliance adapter: untranslatable answer: %w; %w",
	tfports.ErrDepartureGateAnswerUntranslatable, tfports.ErrHandoverGateAnswerUntranslatable)

// ActionAdmissionJudge 是关务动作准入判断编排的那一口。生产上接组合根给它包的那层自带事务的壳：判断在关务自己的
// 事务里记下，在调用方事务里调用即拒（ADR-0206 决定六）。这里只声明问的那一口，测试替身不必背上整个编排。
type ActionAdmissionJudge interface {
	Handle(ctx context.Context, command ccapp.JudgeActionAdmissionCommand) (ccapp.JudgeActionAdmissionResult, error)
}

// DepartureGateView 实现 tfports.DepartureGateView。
type DepartureGateView struct {
	judge ActionAdmissionJudge
}

// NewDepartureGateView 判断口必须在：缺了它每一次出发都只能记读不动，一个装配漏接会长成一串看似依赖故障的读数。
func NewDepartureGateView(judge ActionAdmissionJudge) (*DepartureGateView, error) {
	if judge == nil {
		return nil, fmt.Errorf("transport fulfillment customscompliance adapter: action admission judge is nil")
	}
	return &DepartureGateView{judge: judge}, nil
}

var _ tfports.DepartureGateView = (*DepartureGateView)(nil)

// ReadDepartureGate 以出发意图问一件对象。对象引用按同一串译成关务的包裹引用：载运对象是包裹时两个上下文说的是
// 同一串；是袋或集运单元时关务解析不到它的案卷，按自己的规则作答（ADR-0206 越权风险点「对象只认包裹」），本包不替谁展开
// 成员。
func (view *DepartureGateView) ReadDepartureGate(
	ctx context.Context,
	query tfports.DepartureGateQuery,
) (tfdomain.DepartureGateFinding, error) {
	none := tfdomain.DepartureGateFinding{}
	command, err := admissionCommandFor(query)
	if err != nil {
		return none, err
	}
	result, err := view.judge.Handle(ctx, command)
	if err != nil {
		return none, fmt.Errorf("judge action admission: customs compliance: %w", err)
	}
	return findingFromResult(query.Object, result)
}

// findingFromResult 把关务的一次答复译成那件对象的一格。出发与交出同一份对照：意图不同，答复的形状与五格的对照不分意图。
func findingFromResult(object tfdomain.CarriedObjectReference, result ccapp.JudgeActionAdmissionResult) (tfdomain.GateFinding, error) {
	none := tfdomain.GateFinding{}
	switch result.Outcome() {
	case ccapp.ActionAdmissionJudged, ccapp.ActionAdmissionExisting:
		judgement, judgementID, _ := result.Judgement()
		answer, err := answerFor(judgement.Answer())
		if err != nil {
			return none, err
		}
		return findingOf(tfdomain.NewGateFinding(object, answer, judgementID, basesFor(judgement)))
	case ccapp.ActionAdmissionUnreadable:
		reason := result.UnreadableReason().String()
		if reason == "" {
			return none, fmt.Errorf("%w: customs compliance answered unreadable without naming the port", ErrUntranslatableAnswer)
		}
		return findingOf(tfdomain.NewGateFinding(object, tfdomain.GateUnreadable, "", []tfdomain.GateBasis{{Cause: reason}}))
	default:
		// 未受理：本上下文交过去的都是各自构造门放行过的值，关务仍不受理，说明两边对同一串的口径漂了。
		return none, fmt.Errorf("%w: customs compliance answered %q to the intent", ErrUntranslatableAnswer, result.Outcome())
	}
}

// findingOf 把逐对象那一格立不起来的情形归进译不回来：关务形成了答复，却交不出本上下文认得的形状（例如没有同格依据、
// 判断标识缺席或含 NUL），是两边对答复形状的口径漂了，与关务进程通不通无关。
func findingOf(finding tfdomain.GateFinding, err error) (tfdomain.GateFinding, error) {
	if err != nil {
		return tfdomain.GateFinding{}, fmt.Errorf("%w: %v", ErrUntranslatableAnswer, err)
	}
	return finding, nil
}

func admissionCommandFor(query tfports.DepartureGateQuery) (ccapp.JudgeActionAdmissionCommand, error) {
	none := ccapp.JudgeActionAdmissionCommand{}
	tenant, err := ccdomain.NewTenantID(query.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := ccdomain.NewDeclaredParcelReference(query.Object.String())
	if err != nil {
		return none, fmt.Errorf("%w: object: %v", ErrUntranslatableAnswer, err)
	}
	from, err := ccdomain.NewNetworkNodeReference(query.From.String())
	if err != nil {
		return none, fmt.Errorf("%w: from node: %v", ErrUntranslatableAnswer, err)
	}
	to, err := ccdomain.NewNetworkNodeReference(query.To.String())
	if err != nil {
		return none, fmt.Errorf("%w: to node: %v", ErrUntranslatableAnswer, err)
	}
	intent, err := ccdomain.NewDepartureIntent(from, to)
	if err != nil {
		return none, fmt.Errorf("%w: departure intent: %v", ErrUntranslatableAnswer, err)
	}
	return ccapp.JudgeActionAdmissionCommand{TenantID: tenant, Object: object, Intent: intent, At: query.At}, nil
}

// answerFor 是关务答复（ccdomain.AdmissionAnswer）到门禁读数各格的对照，出发与交出共用。关务的`未配置`落未决，理由见
// ADR-0205 决定二；依据原文里的种类仍说得出缺的是配置。不认得的那一格是译不回来，不猜。
func answerFor(answer ccdomain.AdmissionAnswer) (tfdomain.GateAnswer, error) {
	switch answer {
	case ccdomain.AdmissionNotGuarded:
		return tfdomain.GateNotGuarded, nil
	case ccdomain.AdmissionAdmitted:
		return tfdomain.GateAdmitted, nil
	case ccdomain.AdmissionUndecided, ccdomain.AdmissionNotConfigured:
		return tfdomain.GateUndecided, nil
	case ccdomain.AdmissionBlocked:
		return tfdomain.GateBlocked, nil
	default:
		return tfdomain.GateAnswerInvalid, fmt.Errorf("%w: admission answer %q", ErrUntranslatableAnswer, answer)
	}
}

// basesFor 只带回与整次答复同一格的那些动作的依据：关务的答复由它们推出（ADR-0206 决定五「答复只列同格依据」），
// 别格的依据带回来，读的人会把一条被压下去的「不受门禁」当成这次动作的理由。
func basesFor(judgement ccdomain.ActionAdmissionJudgement) []tfdomain.GateBasis {
	var bases []tfdomain.GateBasis
	for _, finding := range judgement.Findings() {
		if finding.Answer != judgement.Answer() {
			continue
		}
		for _, basis := range finding.Bases {
			bases = append(bases, tfdomain.GateBasis{
				Action:  finding.Action.String(),
				Cause:   basis.Cause.String(),
				Sources: basis.Sources,
			})
		}
	}
	return bases
}
