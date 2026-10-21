// resume_evaluation_by_operator.go 是 ADR-0229 决定六的运营续办口：运营人员确认一件包裹或一版发生项缺的东西已经补上
// （或那一格本就没有参数登记解得开，例如包裹没有商业解析、计价输入不可得），点名它、给一个续办引用，续办在它上面等着的
// 每一件。它是兜底，不替代「参数已登记」：登记口同笔交信封的那几类，补登时本来就会续办。
package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// operatorResumeLimit 是一次运营续办至多办几件。点名一件包裹或一版发生项，等着的至多是每个计费时点一份发起停办或
// 一份请求链头，远不到这个数；它只防点名写错时一次扫穿整个租户。
const operatorResumeLimit = 50

// OperatorResumeEvaluationCommand 点名一件包裹（SELL）或一版发生项（BUY），并带运营人员给的续办引用。同一个引用重跑
// 是重放：凭它续办过的不再推。
type OperatorResumeEvaluationCommand struct {
	TenantID          domain.TenantID
	Direction         domain.EvaluationDirection
	Parcel            domain.ParcelReference
	Occurrence        domain.ChargeOccurrenceID
	OccurrenceVersion domain.OccurrenceVersion
	Reference         string
}

// OperatorResumedEvaluation 是续办的一件与它的答案。
type OperatorResumedEvaluation struct {
	Item   ports.StalledEvaluation
	Result ResumeStalledEvaluationResult
}

// OperatorResumeEvaluationOutcome 是一次运营续办的总答案。
type OperatorResumeEvaluationOutcome uint8

const (
	OperatorResumeEvaluationOutcomeInvalid OperatorResumeEvaluationOutcome = iota
	// OperatorResumeEvaluationDone：点名的那一件上等着的都办过了一次（逐件答案在 Items 里）；一件都没有也落这里——
	// 没有停着的，或凭这个引用都续办过了。
	OperatorResumeEvaluationDone
	// OperatorResumeEvaluationNotAccepted：命令立不住（方向与点名对不上、没有续办引用）。
	OperatorResumeEvaluationNotAccepted
	// OperatorResumeEvaluationUndecided：某一件停在依赖答不出上，调用方整笔回滚、同一个引用重跑。
	OperatorResumeEvaluationUndecided
)

func (outcome OperatorResumeEvaluationOutcome) String() string {
	switch outcome {
	case OperatorResumeEvaluationDone:
		return "OPERATOR_RESUME_DONE"
	case OperatorResumeEvaluationNotAccepted:
		return "OPERATOR_RESUME_NOT_ACCEPTED"
	case OperatorResumeEvaluationUndecided:
		return "OPERATOR_RESUME_UNDECIDED"
	default:
		return ""
	}
}

// OperatorResumeEvaluationResult 是一次运营续办的结果。
type OperatorResumeEvaluationResult struct {
	Outcome OperatorResumeEvaluationOutcome
	Items   []OperatorResumedEvaluation
}

type OperatorResumeEvaluationHandler struct {
	queue   ports.StalledEvaluationQueue
	resumer *ResumeStalledEvaluationHandler
}

func NewOperatorResumeEvaluationHandler(
	queue ports.StalledEvaluationQueue,
	resumer *ResumeStalledEvaluationHandler,
) (*OperatorResumeEvaluationHandler, error) {
	if queue == nil || resumer == nil {
		return nil, fmt.Errorf("%w: operator resume evaluation", ErrNilDependency)
	}
	return &OperatorResumeEvaluationHandler{queue: queue, resumer: resumer}, nil
}

// Handle 取点名那一件上等着的全部、逐件续办。全跑在调用方的一笔事务里：一件未决就整笔交回未决，调用方回滚、同一个引用
// 重跑——已续办的那几件随回滚一起撤，重跑时再办，不会只留一半。停因不设限：运营人员看过停因才来点名。
func (handler *OperatorResumeEvaluationHandler) Handle(
	ctx context.Context,
	command OperatorResumeEvaluationCommand,
) (OperatorResumeEvaluationResult, error) {
	notAccepted := OperatorResumeEvaluationResult{Outcome: OperatorResumeEvaluationNotAccepted}
	if command.TenantID.String() == "" {
		return notAccepted, nil
	}
	basis, err := domain.NewOperatorResumeBasis(command.Reference)
	if err != nil {
		return notAccepted, nil
	}
	query := ports.StalledEvaluationQuery{Causes: domain.AllEvaluationStopCauses(), ExcludeBasis: basis}
	switch command.Direction {
	case domain.SellEvaluationDirection:
		if command.Parcel.String() == "" || command.Occurrence.String() != "" {
			return notAccepted, nil
		}
		query.Parcel = command.Parcel
	case domain.BuyEvaluationDirection:
		if command.Occurrence.String() == "" || command.OccurrenceVersion.String() == "" || command.Parcel.String() != "" {
			return notAccepted, nil
		}
		query.Occurrence = command.Occurrence
		query.OccurrenceVersion = command.OccurrenceVersion
	default:
		return notAccepted, nil
	}

	items, err := handler.queue.ListStalled(ctx, command.TenantID, query, operatorResumeLimit)
	if err != nil {
		return OperatorResumeEvaluationResult{Outcome: OperatorResumeEvaluationUndecided}, nil
	}
	result := OperatorResumeEvaluationResult{Outcome: OperatorResumeEvaluationDone}
	for _, item := range items {
		resumed, err := handler.resumer.Handle(ctx, ResumeStalledEvaluationCommand{
			TenantID: command.TenantID,
			Item:     item,
			Basis:    basis,
			Causes:   query.Causes,
		})
		if err != nil {
			return OperatorResumeEvaluationResult{}, err
		}
		result.Items = append(result.Items, OperatorResumedEvaluation{Item: item, Result: resumed})
		if resumed.Outcome() == StalledEvaluationUndecided {
			result.Outcome = OperatorResumeEvaluationUndecided
			return result, nil
		}
	}
	return result, nil
}
