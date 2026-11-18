package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// reweighWired 答处理代次链上已有费用的那几件依赖装了没有。它们要么全装要么全不装，构造时已核过。
func (handler *FormEstimatedCustomerChargeHandler) reweighWired() bool {
	return handler.deps.PriorCharges != nil
}

// reweighPreviousCharge 让这一代的评价对着这条代次链上已有的费用办。交回的 settled 为真时这一代已有结论，调用方照答。
//
// 先取包裹确认锁：同一件包裹的各代形成与终局那一头的确认排成一列，之后读到的链是别人提交之后的样子。
// 这一代已被更晚的一代越过，就不再动钱。否则往回找链上最近的一笔费用，再锁它的行、锁到手才判阶段：已确认的在这里结清
// 差额；仍是预估或暂估的，交回它的标识，等这一代的新预估落下之后记替代，评价合计为零的话没有新预估，就记没有后继的替代。
// 锁不能省，也不能挪到判阶段之后——管理台手工
// 确认不经包裹，若只靠包裹锁，它可以夹在「读到仍是预估」与「记替代」之间把这笔费用确认掉，同一件包裹两笔都算数。
// 链上一笔费用都没有，这一代照旧形成首笔预估。
func (handler *FormEstimatedCustomerChargeHandler) reweighPreviousCharge(
	ctx context.Context,
	command FormEstimatedCustomerChargeCommand,
	record ports.SellEvaluationRequestRecord,
) (domain.CustomerChargeID, bool, EstimatedCustomerChargeResult, error) {
	var none domain.CustomerChargeID
	if !handler.reweighWired() {
		return none, false, EstimatedCustomerChargeResult{}, nil
	}
	key := ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: command.Evaluation.Request}
	if err := handler.deps.Serializer.SerializeParcelConfirmation(ctx, command.TenantID, record.Request.Parcel()); err != nil {
		return none, true, undecidedEstimatedCharge(SellEvaluationRequestViewUnavailable, err), nil
	}
	later, overtaken, err := handler.overtakenBy(ctx, key)
	if err != nil {
		return none, true, undecidedEstimatedCharge(SellEvaluationRequestViewUnavailable, err), nil
	}
	if overtaken {
		result, err := handler.concludeAsOvertaken(ctx, key, later)
		return none, true, result, err
	}
	earlier, found, err := handler.latestChargeBefore(ctx, command.TenantID, record)
	if errors.Is(err, ErrSellEvaluationRequestMissing) {
		return none, true, EstimatedCustomerChargeResult{}, err
	}
	if err != nil {
		return none, true, undecidedEstimatedCharge(SellEvaluationRequestViewUnavailable, err), nil
	}
	if !found {
		return none, false, EstimatedCustomerChargeResult{}, nil
	}
	// 判阶段之前先锁这一行：手工确认入口不经包裹锁，只有这把锁能让它与这里排成一列。锁到事务结束，下面记替代时仍握着。
	if err := handler.deps.ChargeLock.LockChargeStage(ctx, command.TenantID, earlier); err != nil {
		return none, true, undecidedEstimatedCharge(SellChargeFormationUndecided, err), nil
	}
	charge, found, err := handler.deps.PriorCharges.FindByID(ctx, command.TenantID, earlier)
	if err != nil {
		return none, true, undecidedEstimatedCharge(SellChargeFormationUndecided, err), nil
	}
	if !found {
		// 请求册上的费用标识对客户费用有外键。读不到是两侧说法矛盾：当成没有费用往下走，会给这件包裹再建一笔全额。
		return none, true, EstimatedCustomerChargeResult{}, fmt.Errorf(
			"settlement accounting: request %s follows charge %s, which is absent from the charge store", key.Request, earlier)
	}
	if charge.Stage() == domain.ChargeConfirmed {
		result, err := handler.settleConfirmedReweigh(ctx, command, record, earlier)
		return none, true, result, err
	}
	return charge.ID(), false, EstimatedCustomerChargeResult{}, nil
}

// overtakenBy 沿后继往后找：这一代之后已有一代记下了已形成或零额，这一代的评价就过期了。停下的与还没结论的不算，
// 接着往后看。评价各自成一条队，旧一代的评价可以晚于新一代的到。
func (handler *FormEstimatedCustomerChargeHandler) overtakenBy(
	ctx context.Context,
	key ports.SellEvaluationRequestKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	for {
		successor, found, err := handler.deps.Successors.FindSuccessor(ctx, key)
		if err != nil {
			return ports.SellEvaluationRequestRecord{}, false, err
		}
		if !found {
			return ports.SellEvaluationRequestRecord{}, false, nil
		}
		if successor.State == domain.SellRequestFormed || successor.State == domain.SellRequestZeroTotal {
			return successor, true, nil
		}
		key = ports.SellEvaluationRequestKey{TenantID: key.TenantID, Request: successor.Request.ID()}
	}
}

// concludeAsOvertaken 给被越过的一代记上越过它的那一代的结论：已形成的指向同一笔费用，零额的同记零额。不建费用，
// 不记调整，也不记替代——钱已经照更晚的评价办过了。
func (handler *FormEstimatedCustomerChargeHandler) concludeAsOvertaken(
	ctx context.Context,
	key ports.SellEvaluationRequestKey,
	later ports.SellEvaluationRequestRecord,
) (EstimatedCustomerChargeResult, error) {
	if later.State == domain.SellRequestZeroTotal {
		return handler.conclude(ctx, key, domain.NewZeroTotalSellEvaluationRequestConclusion())
	}
	conclusion, err := domain.NewFormedSellEvaluationRequestConclusion(later.FormedCharge)
	if err != nil {
		return EstimatedCustomerChargeResult{}, err
	}
	return handler.conclude(ctx, key, conclusion)
}

// latestChargeBefore 沿前一代往回找最近一笔费用。中间停下的各代没有费用，跳过：只看紧挨着的前一代，隔着一代停下的
// 请求就认不出更早那笔费用，这一代会给同一件包裹再建一笔全额。前一代不在册上是两侧说法矛盾，交回
// ErrSellEvaluationRequestMissing。
func (handler *FormEstimatedCustomerChargeHandler) latestChargeBefore(
	ctx context.Context,
	tenant domain.TenantID,
	record ports.SellEvaluationRequestRecord,
) (domain.CustomerChargeID, bool, error) {
	var none domain.CustomerChargeID
	for {
		previous, has := record.Request.Previous()
		if !has {
			return none, false, nil
		}
		earlier, found, err := handler.deps.Requests.FindByID(ctx, ports.SellEvaluationRequestKey{TenantID: tenant, Request: previous})
		if err != nil {
			return none, false, err
		}
		if !found {
			return none, false, fmt.Errorf("%w: previous request %s", ErrSellEvaluationRequestMissing, previous)
		}
		if earlier.FormedCharge.String() != "" {
			return earlier.FormedCharge, true, nil
		}
		record = earlier
	}
}

func (handler *FormEstimatedCustomerChargeHandler) settleConfirmedReweigh(
	ctx context.Context,
	command FormEstimatedCustomerChargeCommand,
	record ports.SellEvaluationRequestRecord,
	charge domain.CustomerChargeID,
) (EstimatedCustomerChargeResult, error) {
	key := ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: command.Evaluation.Request}
	adopted, err := handler.deps.ReweighAdoptions.ReweighAdjustmentAdopted(ctx, command.TenantID, record.Request.Account())
	if err != nil {
		return undecidedEstimatedCharge(SellChargeFormationUndecided, err), nil
	}
	if !adopted {
		return handler.stop(ctx, key, domain.StopReweighAdjustmentNotAdopted, command.Evaluation.SourceTrail)
	}
	outcome, err := handler.deps.Reweigh.SettleReweigh(ctx, ReweighDifferenceCommand{
		TenantID:           command.TenantID,
		ChargeID:           charge.String(),
		Evaluation:         command.Evaluation.Evaluation.String(),
		OriginalCurrency:   command.Evaluation.OriginalCurrency.String(),
		OriginalMinor:      command.Evaluation.OriginalMinor,
		SettlementCurrency: command.Evaluation.SettlementCurrency.String(),
		SettlementMinor:    command.Evaluation.SettlementMinor,
		Conversion:         command.Evaluation.Conversion.String(),
	})
	if err != nil {
		return EstimatedCustomerChargeResult{}, err
	}
	switch outcome {
	case ReweighDifferenceNone, ReweighDifferenceRecorded, ReweighDifferenceReplayed:
		conclusion, err := domain.NewFormedSellEvaluationRequestConclusion(charge)
		if err != nil {
			return EstimatedCustomerChargeResult{}, err
		}
		return handler.conclude(ctx, key, conclusion)
	case ReweighDifferenceExceedsCharge:
		return handler.stop(ctx, key, domain.StopReweighAdjustmentExceedsCharge, command.Evaluation.SourceTrail)
	case ReweighDifferenceUndecided:
		return undecidedEstimatedCharge(SellChargeFormationUndecided, fmt.Errorf("reweigh adjustment register")), nil
	default:
		return EstimatedCustomerChargeResult{}, fmt.Errorf("reweigh difference: %s", outcome)
	}
}
