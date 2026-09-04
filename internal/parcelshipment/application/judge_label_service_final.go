// judge_label_service_final.go 编排面单渠道服务的包裹终局判断（票 label-channel-service-first-release/11）：
// 跨该包裹全部相关面单交易、包裹级继续尝试判断与实际承运商首次有效收寄事实形成包裹级判断，
// 再把判出的格作为一份责任结果交给既有的终局采用路径（form_parcel_final.go）。
//
// 它不自己形成终局：两种服务形态的产物都叫「终局服务结果」，委托完成派生、取消前核验与继续
// 尝试判断只认 FinalOutcomeStore 里那一处当前有效终局——面单渠道服务的终局若另存一处，这三处
// 都看不见它。跨聚合谓词也不塞进 FinalRuleView：那个口答的是「这一格在该产品/合同下算不算
// 终局、什么类型、哪版规则」，而「全部相关交易均已定案且无有效面单结果」是本上下文自己的判断
// （CONTEXT「`parcel-shipment` 必须跨该包裹全部相关交易及实际承运商收寄事实形成包裹级判断」）。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// LabelServiceFinalOutcome 是一次判断的应用处理结果。`已交采用`不是终局已形成——采用路径
// 自己分格（形成/重派生/既有/未决/不采用/冲突），从 Adoption() 读；这里只说判断到了哪一步。
type LabelServiceFinalOutcome uint8

const (
	LabelServiceFinalOutcomeInvalid LabelServiceFinalOutcome = iota
	LabelServiceFinalAdopted
	LabelServiceNotFinalOutcome
	LabelServiceCancellationStandsOutcome
	LabelServiceJudgmentNotAccepted
	LabelServiceJudgmentUndecided
)

func (outcome LabelServiceFinalOutcome) String() string {
	switch outcome {
	case LabelServiceFinalAdopted:
		return "LABEL_SERVICE_FINAL_ADOPTED"
	case LabelServiceNotFinalOutcome:
		return "LABEL_SERVICE_NOT_FINAL"
	case LabelServiceCancellationStandsOutcome:
		return "CANCELLATION_STANDS"
	case LabelServiceJudgmentNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	case LabelServiceJudgmentUndecided:
		return "JUDGMENT_UNDECIDED"
	default:
		return ""
	}
}

// LabelServiceUndecidedReason 指名判断停在哪一步等谁。四个读口各占一格：等的东西不同。
type LabelServiceUndecidedReason uint8

const (
	LabelServiceUndecidedReasonNone LabelServiceUndecidedReason = iota
	LabelTransactionsUnavailable
	ContinuedAttemptRegisterUnavailable
	LabelCancellationViewUnavailable
	LabelValidityRuleUnavailable
)

func (reason LabelServiceUndecidedReason) String() string {
	switch reason {
	case LabelTransactionsUnavailable:
		return "LABEL_TRANSACTIONS_UNAVAILABLE"
	case ContinuedAttemptRegisterUnavailable:
		return "CONTINUED_ATTEMPT_REGISTER_UNAVAILABLE"
	case LabelCancellationViewUnavailable:
		return "CANCELLATION_VIEW_UNAVAILABLE"
	case LabelValidityRuleUnavailable:
		return "LABEL_VALIDITY_RULE_UNAVAILABLE"
	default:
		return ""
	}
}

// JudgeLabelServiceFinalCommand 指名要判哪件包裹。FirstEffectivePickup 零值即缺席：收寄事实
// 到达那一路带它进来（TF 外部承运轨迹事实的引用与版本，只引用不复制），引用本体在编排里经
// 领域构造器立起来（同 FormParcelFinalCommand 带 ResponsibilityOutcomeSpec 的先例）；交易定案或
// 关闭生效那一路不带它，判断走关闭路径。委托来源身份与委托标识是采用路径核成员用的。
type JudgeLabelServiceFinalCommand struct {
	Identity             domain.SourceIdentity
	ShipmentRequestID    domain.ShipmentRequestID
	Parcel               domain.DeclaredParcelID
	FirstEffectivePickup domain.CarrierFirstEffectivePickupSpec
}

type LabelServiceFinalResult struct {
	outcome   LabelServiceFinalOutcome
	reason    LabelServiceUndecidedReason
	verdict   domain.LabelServiceFinalVerdict
	adoption  FormParcelFinalResult
	delegated bool
}

func (result LabelServiceFinalResult) Outcome() LabelServiceFinalOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result LabelServiceFinalResult) UndecidedReason() LabelServiceUndecidedReason {
	return result.reason
}

// Verdict 是领域判断本体（格、原因、证据、生效时间），判断走到了就有，不论采用路径答什么。
func (result LabelServiceFinalResult) Verdict() domain.LabelServiceFinalVerdict {
	return result.verdict
}

// Adoption 只在判出终局并交给采用路径后给出：终局有没有真的形成，看它。
func (result LabelServiceFinalResult) Adoption() (FormParcelFinalResult, bool) {
	return result.adoption, result.delegated
}

// ParcelFinalAdopter 是既有终局采用路径的入口；生产装配接 FormParcelFinalHandler。
type ParcelFinalAdopter interface {
	Handle(ctx context.Context, command FormParcelFinalCommand) (FormParcelFinalResult, error)
}

// JudgeLabelServiceFinalDeps 是判断的依赖。四个读口都只读：判断不写，形成走 Adoption。
// Validity 可为 nil——有效期规则是实例半边，装配没有它时按「未配置」办，不推算失效。
type JudgeLabelServiceFinalDeps struct {
	Transactions  ports.LabelTransactionsByParcelView
	Registers     ports.ContinuedAttemptRegisterView
	Cancellations ports.ParcelCancellationView
	Validity      ports.LabelValidityRuleView
	Adoption      ParcelFinalAdopter
	Clock         ports.Clock
}

type JudgeLabelServiceFinalHandler struct {
	deps JudgeLabelServiceFinalDeps
}

func NewJudgeLabelServiceFinalHandler(deps JudgeLabelServiceFinalDeps) *JudgeLabelServiceFinalHandler {
	return &JudgeLabelServiceFinalHandler{deps: deps}
}

// Handle 判一件包裹：取消视图 → 全部相关交易（整册，不筛）→ 登记册（未开册即空册）→ 对仍有效的
// 成功结果问有效期规则（未配置不失效）→ JudgeLabelServiceFinal → 形成终局的格译成责任结果交
// 采用路径；不形成与取消在先不走采用。
func (handler *JudgeLabelServiceFinalHandler) Handle(
	ctx context.Context,
	command JudgeLabelServiceFinalCommand,
) (LabelServiceFinalResult, error) {
	if command.Parcel.String() == "" || command.Identity.TenantID().String() == "" ||
		command.ShipmentRequestID.String() == "" {
		return LabelServiceFinalResult{outcome: LabelServiceJudgmentNotAccepted}, nil
	}
	tenant := command.Identity.TenantID()
	var pickup domain.CarrierFirstEffectivePickup
	if command.FirstEffectivePickup != (domain.CarrierFirstEffectivePickupSpec{}) {
		// 半份引用（有事实没版本、有版本没有效时间）是提交矛盾，构造期就拦；只引用不复制。
		referenced, err := domain.ReferenceCarrierFirstEffectivePickup(command.FirstEffectivePickup)
		if err != nil {
			return LabelServiceFinalResult{outcome: LabelServiceJudgmentNotAccepted}, nil
		}
		pickup = referenced
	}

	_, cancelled, err := handler.deps.Cancellations.FindCancellation(ctx, tenant, command.Parcel)
	if err != nil {
		return undecidedLabelServiceFinal(LabelCancellationViewUnavailable), nil
	}
	transactions, err := handler.deps.Transactions.ListByCoveredParcel(ctx, tenant, command.Parcel)
	if err != nil {
		return undecidedLabelServiceFinal(LabelTransactionsUnavailable), nil
	}
	register, found, err := handler.deps.Registers.FindByParcel(ctx, tenant, command.Parcel)
	if err != nil {
		return undecidedLabelServiceFinal(ContinuedAttemptRegisterUnavailable), nil
	}
	if !found {
		// 没开过册与空册是同一格（CONTEXT：无生效关闭且无当前有效终局即开放）。
		if register, err = domain.OpenContinuedAttemptRegister(tenant, command.Parcel); err != nil {
			return LabelServiceFinalResult{outcome: LabelServiceJudgmentNotAccepted}, nil
		}
	}
	lapsed, err := handler.lapsedTransactions(ctx, tenant, command.Parcel, transactions)
	if err != nil {
		return undecidedLabelServiceFinal(LabelValidityRuleUnavailable), nil
	}

	verdict, err := domain.JudgeLabelServiceFinal(domain.LabelServiceFinalInput{
		Parcel:               command.Parcel,
		Transactions:         transactions,
		LapsedTransactions:   lapsed,
		Register:             register,
		FirstEffectivePickup: pickup,
		CancellationStands:   cancelled,
	})
	if err != nil {
		// 读口交回了别的包裹的东西：装配缺陷，响亮报错不吸收。
		return LabelServiceFinalResult{}, fmt.Errorf("judge label service final: %w", err)
	}

	result := LabelServiceFinalResult{verdict: verdict}
	switch verdict.Judgment() {
	case domain.LabelServiceCancellationStands:
		result.outcome = LabelServiceCancellationStandsOutcome
		return result, nil
	case domain.LabelServiceNotFinal:
		result.outcome = LabelServiceNotFinalOutcome
		return result, nil
	}

	spec, err := responsibilityOutcomeOf(command.Parcel, verdict)
	if err != nil {
		return LabelServiceFinalResult{}, fmt.Errorf("judge label service final: %w", err)
	}
	adoption, err := handler.deps.Adoption.Handle(ctx, FormParcelFinalCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		Outcome:           spec,
	})
	if err != nil {
		return LabelServiceFinalResult{}, err
	}
	result.outcome = LabelServiceFinalAdopted
	result.adoption = adoption
	result.delegated = true
	return result, nil
}

// lapsedTransactions 对每笔上本包裹被受理的交易问一次有效期规则。规则未配置或读口缺席都不失效
// ——「自然失效必须来自渠道确认或接受时固定的有效期规则」，不来自本编排的墙钟。
func (handler *JudgeLabelServiceFinalHandler) lapsedTransactions(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	transactions []domain.LabelTransaction,
) ([]domain.LabelTransactionID, error) {
	if handler.deps.Validity == nil {
		return nil, nil
	}
	var lapsed []domain.LabelTransactionID
	asOf := handler.deps.Clock.Now()
	for _, transaction := range transactions {
		result, found := transaction.ParcelResult(parcel)
		if !found || !result.Accepted() {
			continue
		}
		hasLapsed, configured, err := handler.deps.Validity.JudgeLabelLapsed(ctx, tenant, transaction, parcel, asOf)
		if err != nil {
			return nil, err
		}
		if configured && hasLapsed {
			lapsed = append(lapsed, transaction.ID())
		}
	}
	return lapsed, nil
}

// responsibilityOutcomeOf 把形成终局的判断译成一份责任结果：决定是本上下文自己的面单服务终局
// 判断（由包裹、格与证据定值——CONTEXT 把这一判断判给 PS），执行证据是 TF 收寄事实（引用@版本）
// 或生效的受控关闭决定，来源版本取证据的版本——同一事实版本/同一关闭决定重放返原，新版本走重派生。
func responsibilityOutcomeOf(
	parcel domain.DeclaredParcelID,
	verdict domain.LabelServiceFinalVerdict,
) (domain.ResponsibilityOutcomeSpec, error) {
	kind, forms := verdict.ResponsibilityOutcomeKind()
	if !forms {
		return domain.ResponsibilityOutcomeSpec{}, errors.New("verdict forms no final")
	}
	var evidence, version string
	if pickup, byPickup := verdict.FirstEffectivePickup(); byPickup {
		evidence = pickup.Fact().String() + "@" + pickup.Version().String()
		version = pickup.Version().String()
	} else if closure, byClosure := verdict.Closure(); byClosure {
		evidence = "CONTINUED-ATTEMPT-CLOSURE/" + closure.ID().String()
		version = closure.ID().String()
	} else {
		return domain.ResponsibilityOutcomeSpec{}, errors.New("verdict carries no evidence")
	}
	decision, err := domain.NewResponsibilityDecisionReference(
		"LABEL-SERVICE-FINAL/" + parcel.String() + "/" + verdict.Judgment().String() + "/" + evidence)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	execution, err := domain.NewExecutionEvidenceReference(evidence)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	outcomeVersion, err := domain.NewResponsibilityOutcomeVersion(version)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	return domain.ResponsibilityOutcomeSpec{
		Kind:       kind,
		Parcel:     parcel,
		Decision:   decision,
		Execution:  execution,
		Version:    outcomeVersion,
		OccurredAt: verdict.EffectiveAt(),
	}, nil
}

func undecidedLabelServiceFinal(reason LabelServiceUndecidedReason) LabelServiceFinalResult {
	return LabelServiceFinalResult{outcome: LabelServiceJudgmentUndecided, reason: reason}
}
