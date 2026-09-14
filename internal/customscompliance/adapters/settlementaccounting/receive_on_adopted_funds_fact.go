package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var (
	// ErrAdoptedFactNotVisible 表示按信封引用还读不回那一版已采用事实。可见性滞后是续办，重投会
	// 改变结果；不当毒丸拒收。生产装配把它登进 WithUndecidedSentinels：运维据此去查提供方那一侧，
	// 而不是查传输。
	ErrAdoptedFactNotVisible = errors.New(
		"customs compliance settlementaccounting adapter: adopted funds fact is not yet visible")
	// ErrAdoptedFactVersionInconsistent 表示按信封所指版本回查，交回的事实本体却自称另一版——提供方视图答非所问，
	// 是装配 / 视图缺陷，不是等谁：重投不会把视图答对。它与 ErrAdoptedFactNotVisible 分开正是为了别把永久损坏
	// 登成可续办（ADR-0029）；生产装配不得把它进 WithUndecidedSentinels。不核对这一格，别版内容会登在信封那一版
	// 名下（票 sa-cc/13 补评审 Standards 2）。
	ErrAdoptedFactVersionInconsistent = errors.New(
		"customs compliance settlementaccounting adapter: adopted funds fact disagrees with the version the envelope names")
	// ErrFundsFactReceiveUndecided 表示入向登记编排停在自己的未决上（登记册不可用）。同样是续办，
	// 同样进 WithUndecidedSentinels。
	ErrFundsFactReceiveUndecided = errors.New(
		"customs compliance settlementaccounting adapter: receiving the funds fact is undecided")
	// ErrUnexpectedReceiveOutcome 表示编排交回了封闭集合以外的结果。静默入账等于替编排作判断，
	// 因此不留 default 兜底。
	ErrUnexpectedReceiveOutcome = errors.New(
		"customs compliance settlementaccounting adapter: unexpected receive outcome")
)

// FundsFactReceiver 是入向登记编排的那一口（UC-CC-009 步 6 的 CC 半边）。真实装配接
// ccapplication.DutyPaymentReconciliationHandler。
type FundsFactReceiver interface {
	ReceiveFundsFact(
		ctx context.Context,
		command ccapplication.ReceiveExternalFundsFactCommand,
	) (ccapplication.DutyReconciliationResult, error)
}

// ReceiveOnAdoptedFundsFactAdapter 是 ccinbox.ExternalFundsFactConsumer 的真实处理方：按信封引用向
// 提供方回查事实内容，译成入向登记交 ReceiveFundsFact。
//
// 只译不判（票 sa-cc/03 红线）：不关联、不核对——关联依据与三轴由 VerifyPayment 的调用方交；
// 同版本重放答 `已存在`、同版本换内容答 `内容冲突`、新版本答 `已接收` 都是编排按（引用 + 版本）
// 给出的答案，这里照单入账——更正还是冲突不在这里分路（票 sa-cc/13 做法 2 / 3）。
type ReceiveOnAdoptedFundsFactAdapter struct {
	source   ccports.AdoptedFundsFactSource
	receiver FundsFactReceiver
}

func NewReceiveOnAdoptedFundsFactAdapter(
	source ccports.AdoptedFundsFactSource,
	receiver FundsFactReceiver,
) (*ReceiveOnAdoptedFundsFactAdapter, error) {
	if source == nil {
		return nil, fmt.Errorf("customs compliance settlementaccounting adapter: adopted funds fact source is nil")
	}
	if receiver == nil {
		return nil, fmt.Errorf("customs compliance settlementaccounting adapter: funds fact receiver is nil")
	}
	return &ReceiveOnAdoptedFundsFactAdapter{source: source, receiver: receiver}, nil
}

var _ ccinbox.AdoptedExternalFundsFactHandler = (*ReceiveOnAdoptedFundsFactAdapter)(nil)

// HandleAdoptedExternalFundsFact 回查 → 译 → 交编排 → 把编排结果落成消费两格。
//
// 编排答 `未受理`（形状矛盾——例如提供方那一版回指了自己）同样入账、不重投：重投同样内容不会把形状
// 改对，它是本上下文的诚实停点，不是传输故障——与交付消费者对 `REQUEST_NOT_ACCEPTED` 的处置同形
// （finalconsume.Consumption）。付款人缺席自票 sa-cc/12 起不再是这一格：登记照单记「未提供」。
func (adapter *ReceiveOnAdoptedFundsFactAdapter) HandleAdoptedExternalFundsFact(
	ctx context.Context,
	adopted ccinbox.AdoptedExternalFundsFact,
) error {
	tenant, err := ccdomain.NewTenantID(adopted.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	fact, err := ccdomain.NewExternalFundsFactReference(adopted.Fact)
	if err != nil {
		return fmt.Errorf("%w: fact: %v", ErrUntranslatableReference, err)
	}

	version, err := ccdomain.NewFundsFactVersion(adopted.Version)
	if err != nil {
		return fmt.Errorf("%w: version: %v", ErrUntranslatableReference, err)
	}

	content, found, err := adapter.source.LoadAdoptedFundsFact(ctx, tenant, fact, adopted.Version)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAdoptedFactNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: fact %q version %q", ErrAdoptedFactNotVisible, adopted.Fact, adopted.Version)
	}
	if content.Version != version {
		return fmt.Errorf("%w: envelope names %q, source answered %q",
			ErrAdoptedFactVersionInconsistent, adopted.Version, content.Version)
	}

	// 版本取信封所指的那一版、回指取回查到的事实本体（票 sa-cc/13 做法 2）：读口按版本取，两者本该同源，上面那句
	// 相等校验把「本该」钉成「必须」；回指是事实本体的一维，信封载荷里那份可缺席的 corrects 不进译码（消费者头注）。
	result, err := adapter.receiver.ReceiveFundsFact(ctx, ccapplication.ReceiveExternalFundsFactCommand{
		TenantID: tenant,
		Registration: ccports.ExternalFundsFactRegistration{
			Fact:        fact,
			Version:     version,
			Corrects:    content.Corrects,
			Source:      content.Source,
			Payer:       content.Payer,
			Currency:    content.Currency,
			AmountMinor: content.AmountMinor,
			OccurredAt:  content.OccurredAt,
		},
	})
	if err != nil {
		return err
	}
	return receiveConsumption(result)
}

// receiveConsumption 把编排结果落成消费两格：成功、已存在、内容冲突、未受理都是编排给出的答案，
// 入账；未决是等依赖，重投。封闭集之外响亮报错。
func receiveConsumption(result ccapplication.DutyReconciliationResult) error {
	switch result.Outcome() {
	case ccapplication.FundsFactReceived,
		ccapplication.FundsFactExisting,
		ccapplication.FundsFactContentConflict,
		ccapplication.DutyReconciliationNotAccepted:
		return nil
	case ccapplication.DutyReconciliationUndecided:
		return fmt.Errorf("%w: %s", ErrFundsFactReceiveUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedReceiveOutcome, result.Outcome())
	}
}
