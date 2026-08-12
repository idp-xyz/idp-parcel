// Package partycommercial 是 parcel-shipment 对 party-commercial 的消费侧适配器——
// ADR-0025 点名的第一个跨上下文适配器。只有本包可以同时导入两个上下文；端口说消费方
// 语言，适配器只翻译不判断，翻译必须是全函数。
package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容。它是编程错误而不是
// 业务答案：静默映射到任何一格都等于替消费方作判断——ADR-0025 不留 default 兜底，防的
// 正是提供方新增取值被悄悄吸收。
var ErrUntranslatableAnswer = errors.New("parcel shipment partycommercial adapter: untranslatable answer")

// AsOfValueSource 按声明的时点语义为一项判断形成值。
//
// 语义如何折成一个时刻属实例半边（`PAR-COM-14` 的租户政策），ports.JudgmentAsOfQuery 的
// 类型注释把「消费方」定为适配器，值只在这里形成。第二个返回值报告「这项语义我形不成」，
// error 报告「我此刻答不出」——前者等登记、后者等恢复，压成一个调用方就不知道该催人还是
// 该重试。
type AsOfValueSource interface {
	FormAsOfValue(ctx context.Context, query psports.JudgmentAsOfQuery) (time.Time, bool, error)
}

// FormJudgmentAsOf 执行 `UC-PC-002` 步骤 6 的消费方半边：按声明的语义形成值，交提供方
// 校验回显，再把答复译回本上下文的落点。
//
// 形不出值就停下，不去问提供方：没有值的查询送过去只会换回`值不合法`，把「等登记」错报
// 成「改这次请求」，两者的恢复动作不同。
func (adapter *CommercialBasisAdapter) FormJudgmentAsOf(
	ctx context.Context,
	query psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	judgment, err := providerJudgmentTypeFor(query.Declared.Kind())
	if err != nil {
		return psports.JudgmentAsOfFormation{}, err
	}

	if adapter.values == nil {
		return psports.JudgmentAsOfFormation{Outcome: psports.JudgmentAsOfNotConfigured}, nil
	}
	at, formed, err := adapter.values.FormAsOfValue(ctx, query)
	if err != nil {
		return psports.JudgmentAsOfFormation{}, fmt.Errorf("form as-of value: %w", err)
	}
	if !formed {
		return psports.JudgmentAsOfFormation{Outcome: psports.JudgmentAsOfNotConfigured}, nil
	}

	answer, err := adapter.judgments.Handle(ctx, pcapplication.FormJudgmentAsOfCommand{
		Caller:     callerFor(query.Identity),
		Resolution: resolutionFor(query.Resolution),
		Judgments:  []pcapplication.JudgmentAsOfRequest{{Judgment: judgment, At: at}},
	})
	if err != nil {
		return psports.JudgmentAsOfFormation{}, fmt.Errorf("form judgment as-of: %w", err)
	}

	outcome, err := judgmentOutcomeFor(answer.Outcome())
	if err != nil {
		return psports.JudgmentAsOfFormation{}, err
	}
	if outcome != psports.JudgmentAsOfFormed {
		return psports.JudgmentAsOfFormation{Outcome: outcome}, nil
	}

	echoed, present := answer.AsOfFor(judgment)
	if !present {
		// 提供方说`已形成`却没带这项判断的时点，是阶段契约被打破，不是一种未决。
		return psports.JudgmentAsOfFormation{}, fmt.Errorf("%w: formed answer carries no as-of for %q",
			ErrUntranslatableAnswer, judgment)
	}
	translated, err := judgmentAsOfFor(echoed)
	if err != nil {
		return psports.JudgmentAsOfFormation{}, err
	}
	return psports.JudgmentAsOfFormation{Outcome: psports.JudgmentAsOfFormed, AsOf: translated}, nil
}

// callerFor 把来源身份译成提供方的调用方身份。立不起来的部分译成零值而不是报错：
// 「最小身份成不成立」是提供方结果代数里自己的一格（`输入未受理`），适配器替它先答
// 就是判断而不是翻译。
func callerFor(identity psdomain.SourceIdentity) pcapplication.CallerScope {
	scope := pcapplication.CallerScope{}
	if tenant, err := pcdomain.NewTenantID(identity.TenantID().String()); err == nil {
		scope.TenantID = tenant
	}
	if account, err := pcdomain.NewCustomerAccountID(identity.CustomerAccountID().String()); err == nil {
		scope.CustomerAccountID = account
	}
	return scope
}

// resolutionFor 把本方记下的解析标识重建为提供方的回指入口（ADR-0027：标识是作用中的
// 入口）。空或立不起来同样译成零值，交提供方短路作答。
func resolutionFor(resolution psdomain.CommercialResolutionID) pcdomain.ResolutionID {
	rebuilt, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return pcdomain.ResolutionID{}
	}
	return rebuilt
}

// providerJudgmentTypeFor 把本上下文的判断类别译成提供方的。两边各自封闭，逐格显式对应，
// default 报错不吸收。
func providerJudgmentTypeFor(kind psdomain.JudgmentKind) (pcdomain.JudgmentType, error) {
	switch kind {
	case psdomain.ReachabilityJudgmentKind:
		return pcdomain.NetworkReachabilityJudgment, nil
	case psdomain.FinancialControlJudgmentKind:
		return pcdomain.PreAcceptanceFinancialControlJudgment, nil
	default:
		return pcdomain.JudgmentTypeInvalid, fmt.Errorf("%w: judgment kind %d", ErrUntranslatableAnswer, kind)
	}
}

// consumerJudgmentKindFor 是上一条的回程，用在译提供方答复时。
func consumerJudgmentKindFor(judgment pcdomain.JudgmentType) (psdomain.JudgmentKind, error) {
	switch judgment {
	case pcdomain.NetworkReachabilityJudgment:
		return psdomain.ReachabilityJudgmentKind, nil
	case pcdomain.PreAcceptanceFinancialControlJudgment:
		return psdomain.FinancialControlJudgmentKind, nil
	default:
		return psdomain.JudgmentKindInvalid, fmt.Errorf("%w: judgment type %q", ErrUntranslatableAnswer, judgment)
	}
}

// judgmentOutcomeFor 是提供方第二阶段封闭集合到本上下文落点的全函数。提供方新增一格时
// 这里必须跟着决定落点——default 报错，静默继承等于让提供方替消费方作业务判断。
func judgmentOutcomeFor(outcome pcapplication.JudgmentAsOfOutcome) (psports.JudgmentAsOfOutcome, error) {
	switch outcome {
	case pcapplication.JudgmentAsOfFormed:
		return psports.JudgmentAsOfFormed, nil
	case pcapplication.JudgmentAsOfBasisNotResolved:
		return psports.JudgmentAsOfBasisNotResolved, nil
	case pcapplication.JudgmentAsOfNotConfigured:
		return psports.JudgmentAsOfNotConfigured, nil
	case pcapplication.JudgmentAsOfPending:
		return psports.JudgmentAsOfPending, nil
	case pcapplication.JudgmentAsOfValueInvalid:
		return psports.JudgmentAsOfValueRejected, nil
	case pcapplication.JudgmentAsOfInputNotAccepted:
		return psports.JudgmentAsOfInputNotAccepted, nil
	default:
		return psports.JudgmentAsOfOutcomeInvalid, fmt.Errorf("%w: judgment as-of outcome %d", ErrUntranslatableAnswer, outcome)
	}
}

// judgmentAsOfFor 从提供方校验回显的答复构造消费方时点。回显必须取自答复而不是转手第一
// 阶段的声明——两次调用之间规则包可能换过政策，转手会把「规则包说锚在这」冒充成「权威
// 确认过这个时刻」（EchoedAsOfPolicy 的类型注释由类型挡住了后一半，这里挡前一半）。
func judgmentAsOfFor(echoed pcdomain.JudgmentAsOf) (psdomain.JudgmentAsOf, error) {
	kind, err := consumerJudgmentKindFor(echoed.Judgment())
	if err != nil {
		return psdomain.JudgmentAsOf{}, err
	}
	semantics, err := psdomain.NewAsOfSemanticsReference(echoed.Policy().Semantics().String())
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed semantics: %v", ErrUntranslatableAnswer, err)
	}
	policyVersion, err := psdomain.NewAsOfPolicyVersion(echoed.Policy().PolicyVersion().String())
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed policy version: %v", ErrUntranslatableAnswer, err)
	}
	policy, err := psdomain.NewEchoedAsOfPolicy(kind, semantics, policyVersion)
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: echoed policy: %v", ErrUntranslatableAnswer, err)
	}
	translated, err := psdomain.NewJudgmentAsOf(echoed.At(), policy)
	if err != nil {
		return psdomain.JudgmentAsOf{}, fmt.Errorf("%w: formed as-of: %v", ErrUntranslatableAnswer, err)
	}
	return translated, nil
}
