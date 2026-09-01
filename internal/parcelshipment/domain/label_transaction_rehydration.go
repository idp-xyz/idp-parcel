package domain

import (
	"errors"
	"time"
)

// ErrInvalidRehydratedLabelTransaction 是面单交易从持久化行重建失败的信号。它与
// ErrInvalidLabelTransaction 分开（ADR-0028/0030 的纪律）：后者说「此刻要发生的这件事不合
// 规则」，前者说「这份已经发生过的东西不可能是本上下文判出来的」——要查的是库里那一行或
// 写它的适配器，不是让调用方改请求重来。
var ErrInvalidRehydratedLabelTransaction = errors.New("parcel shipment: invalid rehydrated label transaction")

// RehydrateLabelTransactionSpec 携带从一行重建面单交易所需的全部字段。
//
// 关系出处在这里摊成两个字段而不是收成 PriorLabelTransactionLink：那个类型的构造器要原交易
// **本体**来核定案，而库里只留下一个交易号——重建门以结果自证一致，不逆推形成过程（同
// RehydrateParcelFinalOutcome 的取法）。原交易当时是否已定案由它自己那一行负责，这里不越界
// 去替另一个聚合作证。
type RehydrateLabelTransactionSpec struct {
	// Revision 是这一行在库里的版本。重建的聚合必然已持久化，所以它必须从 1 起。
	Revision               int64
	Tenant                 TenantID
	ID                     LabelTransactionID
	CoveredParcels         []DeclaredParcelID
	ChannelAccount         ChannelAccountReference
	AccountHolder          ChannelAccountHolderReference
	ServiceProvider        ChannelServiceProviderReference
	SettlementCounterparty SettlementCounterpartyReference
	Contract               ChannelContractReference
	Rate                   ChannelRateReference
	ResponsibilityBasis    ResponsibilityBasisSnapshotReference
	EstablishedAt          time.Time
	PriorTransactionID     LabelTransactionID
	PriorLinkKind          LabelTransactionLinkKind
	SubmittedAt            time.Time
	State                  LabelTransactionState
	ParcelResults          []LabelTransactionParcelResultSpec
	ResultObservedAt       time.Time
	FollowUpActions        []FollowUpActionSpec
}

// RehydrateLabelTransaction 逐字段过领域校验把一行读回聚合。
//
// 它验的是这一行自身能不能成立：出生属性齐全、状态与它该有的痕迹对得上、双层结果仍满足写入
// 时那套结构校验、后续动作的范围仍在覆盖范围内。定案不从行里读——那是派生谓词（决定四），
// 重建之后照样由交易级结果算出来。
func RehydrateLabelTransaction(spec RehydrateLabelTransactionSpec) (LabelTransaction, error) {
	if spec.Revision < 1 ||
		!spec.Tenant.valid() ||
		!spec.ID.valid() ||
		!spec.ChannelAccount.valid() ||
		!spec.AccountHolder.valid() ||
		!spec.ServiceProvider.valid() ||
		!spec.SettlementCounterparty.valid() ||
		!spec.Contract.valid() ||
		!spec.Rate.valid() ||
		!spec.ResponsibilityBasis.valid() ||
		spec.EstablishedAt.IsZero() ||
		spec.State.String() == "" {
		return LabelTransaction{}, ErrInvalidRehydratedLabelTransaction
	}
	// 关系的两半必须同在或同缺：只有方向没有原交易的一行，指不出被重试的是谁；只有原交易
	// 没有方向的一行，分不清这是重试还是换单。
	if spec.PriorTransactionID.valid() != spec.PriorLinkKind.valid() ||
		spec.PriorTransactionID == spec.ID {
		return LabelTransaction{}, ErrInvalidRehydratedLabelTransaction
	}
	covered, err := fixCoveredParcels(spec.CoveredParcels)
	if err != nil {
		return LabelTransaction{}, ErrInvalidRehydratedLabelTransaction
	}

	transaction := LabelTransaction{
		revision:               spec.Revision,
		tenant:                 spec.Tenant,
		id:                     spec.ID,
		coveredParcels:         covered,
		channelAccount:         spec.ChannelAccount,
		accountHolder:          spec.AccountHolder,
		serviceProvider:        spec.ServiceProvider,
		settlementCounterparty: spec.SettlementCounterparty,
		contract:               spec.Contract,
		rate:                   spec.Rate,
		responsibilityBasis:    spec.ResponsibilityBasis,
		establishedAt:          spec.EstablishedAt.UTC(),
		priorLink:              PriorLabelTransactionLink{prior: spec.PriorTransactionID, kind: spec.PriorLinkKind},
		state:                  spec.State,
	}
	if err := transaction.rehydrateChannelTrace(spec); err != nil {
		return LabelTransaction{}, err
	}
	return transaction, nil
}

// rehydrateChannelTrace 按状态核对这一行该有和不该有的痕迹，并把结果与后续动作装回去。
// 一行`已建立`却带着渠道结果，或者一行`成功`却没有任何包裹结果，都不可能是本上下文写出来的。
func (transaction *LabelTransaction) rehydrateChannelTrace(spec RehydrateLabelTransactionSpec) error {
	hasResultTrace := len(spec.ParcelResults) != 0 ||
		!spec.ResultObservedAt.IsZero() ||
		len(spec.FollowUpActions) != 0

	if transaction.state == LabelTransactionEstablished {
		if !spec.SubmittedAt.IsZero() || hasResultTrace {
			return ErrInvalidRehydratedLabelTransaction
		}
		return nil
	}

	if spec.SubmittedAt.IsZero() || spec.SubmittedAt.Before(transaction.establishedAt) {
		return ErrInvalidRehydratedLabelTransaction
	}
	transaction.submittedAt = spec.SubmittedAt.UTC()

	if !transaction.state.isChannelResult() {
		if hasResultTrace {
			return ErrInvalidRehydratedLabelTransaction
		}
		return nil
	}

	if spec.ResultObservedAt.IsZero() || spec.ResultObservedAt.Before(transaction.submittedAt) {
		return ErrInvalidRehydratedLabelTransaction
	}
	results, err := transaction.matchResultsToCoverage(spec.ParcelResults)
	if err != nil {
		return ErrInvalidRehydratedLabelTransaction
	}
	transaction.parcelResults = results
	transaction.resultObservedAt = spec.ResultObservedAt.UTC()

	actions := make([]LabelTransactionFollowUpAction, 0, len(spec.FollowUpActions))
	for _, action := range spec.FollowUpActions {
		if !action.Kind.valid() ||
			!action.Reason.valid() ||
			action.OccurredAt.IsZero() ||
			action.OccurredAt.Before(transaction.resultObservedAt) {
			return ErrInvalidRehydratedLabelTransaction
		}
		scope, err := transaction.scopeWithinCoverage(action.Parcels)
		if err != nil {
			return ErrInvalidRehydratedLabelTransaction
		}
		actions = append(actions, LabelTransactionFollowUpAction{
			kind:       action.Kind,
			parcels:    scope,
			reason:     action.Reason,
			occurredAt: action.OccurredAt.UTC(),
		})
	}
	transaction.followUpActions = actions
	return nil
}
