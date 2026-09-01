package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// rehydratedLabelTransactionSpec 是库里一行「部分成功、带一条指名包裹作废、且自身是一次
// 重试」的面单交易该有的样子。它逐字段手写而不从聚合导出：重建门要验的正是行数据自身，
// 拿构造结果反推回来的 spec 只会验出「构造器与自己一致」。
func rehydratedLabelTransactionSpec(t *testing.T) domain.RehydrateLabelTransactionSpec {
	t.Helper()
	return domain.RehydrateLabelTransactionSpec{
		Revision:               3,
		Tenant:                 mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                     mustValue(t, domain.NewLabelTransactionID, "label-txn-2"),
		CoveredParcels:         []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), mustValue(t, domain.NewDeclaredParcelID, "parcel-2")},
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelTransactionAt,
		PriorTransactionID:     mustValue(t, domain.NewLabelTransactionID, "label-txn-1"),
		PriorLinkKind:          domain.LabelTransactionRetry,
		SubmittedAt:            labelTransactionAt.Add(time.Minute),
		State:                  domain.LabelTransactionPartiallySucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
			refusedParcelResult(t, "parcel-2", "ADDRESS_UNSUPPORTED"),
		},
		ResultObservedAt: labelTransactionAt.Add(2 * time.Minute),
		FollowUpActions: []domain.FollowUpActionSpec{{
			Kind:       domain.ChannelVoidAction,
			Parcels:    []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1")},
			Reason:     mustValue(t, domain.NewChannelResultReasonReference, "CUSTOMER_WITHDREW"),
			OccurredAt: labelTransactionAt.Add(3 * time.Minute),
		}},
	}
}

// Covers: ADR-0084 决定八「重建逐字段过领域构造函数」与 ADR-0028 的重建门纪律——已持久化的
// 一行连同版本、双层结果、追加式后续动作与关系出处一起回到聚合，定案仍按交易级结果派生而
// 不从行里读一列。
func TestALabelTransactionRehydratesWithBothResultLevelsAndItsFollowUps(t *testing.T) {
	transaction, err := domain.RehydrateLabelTransaction(rehydratedLabelTransactionSpec(t))
	if err != nil {
		t.Fatalf("rehydrate label transaction: %v", err)
	}

	if transaction.Revision() != 3 {
		t.Fatalf("revision = %d; 仓储要按它认出并发覆盖", transaction.Revision())
	}
	if transaction.State() != domain.LabelTransactionPartiallySucceeded || !transaction.Finalized() {
		t.Fatalf("state = %s, finalized = %v", transaction.State(), transaction.Finalized())
	}
	if got := stringValues(transaction.CoveredParcels()); len(got) != 2 || got[1] != "parcel-2" {
		t.Fatalf("covered parcels = %v", got)
	}
	refused, found := transaction.ParcelResult(mustValue(t, domain.NewDeclaredParcelID, "parcel-2"))
	if !found || refused.Accepted() || refused.Reason().String() != "ADDRESS_UNSUPPORTED" {
		t.Fatalf("parcel-2 result = %+v, found = %v", refused, found)
	}
	actions := transaction.FollowUpActions()
	if len(actions) != 1 || actions[0].Kind() != domain.ChannelVoidAction {
		t.Fatalf("follow-up actions = %+v", actions)
	}
	link, established := transaction.PriorLink()
	if !established || link.Kind() != domain.LabelTransactionRetry || link.PriorTransactionID().String() != "label-txn-1" {
		t.Fatalf("prior link = %+v, established = %v", link, established)
	}
	if !transaction.ResultObservedAt().Equal(labelTransactionAt.Add(2 * time.Minute)) {
		t.Fatalf("result observed at = %s", transaction.ResultObservedAt())
	}
}

// Covers: ADR-0028「坏行在门上暴露」——重建不逆推形成过程，只验结果自证一致：状态与它该有的
// 痕迹必须对得上（未提交的行不该带结果，结果格的行不该缺逐包裹结果），双层结果与后续动作仍
// 受与写入时同一套结构校验。行数据自己说不通时给出独立哨兵：要查的是库里那一行或写它的
// 适配器，不是调用方的请求。
func TestARehydratedLabelTransactionRefusesRowsThatCannotBeWhatTheyClaim(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RehydrateLabelTransactionSpec){
		"unpersisted revision": func(s *domain.RehydrateLabelTransactionSpec) { s.Revision = 0 },
		"missing responsibility basis": func(s *domain.RehydrateLabelTransactionSpec) {
			s.ResponsibilityBasis = domain.ResponsibilityBasisSnapshotReference{}
		},
		"unknown state": func(s *domain.RehydrateLabelTransactionSpec) {
			s.State = domain.LabelTransactionStateInvalid
		},
		"established row carrying a channel result": func(s *domain.RehydrateLabelTransactionSpec) {
			s.State = domain.LabelTransactionEstablished
			s.SubmittedAt = time.Time{}
		},
		"submitted row carrying parcel results": func(s *domain.RehydrateLabelTransactionSpec) {
			s.State = domain.LabelTransactionSubmitted
		},
		"result row without parcel results": func(s *domain.RehydrateLabelTransactionSpec) {
			s.ParcelResults = nil
		},
		"result row without a result time": func(s *domain.RehydrateLabelTransactionSpec) {
			s.ResultObservedAt = time.Time{}
		},
		"parcel result outside the coverage": func(s *domain.RehydrateLabelTransactionSpec) {
			s.ParcelResults[1] = acceptedParcelResult(t, "parcel-9", "channel-parcel-9")
		},
		"follow-up outside the coverage": func(s *domain.RehydrateLabelTransactionSpec) {
			s.FollowUpActions[0].Parcels = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-9")}
		},
		"link kind without a prior transaction": func(s *domain.RehydrateLabelTransactionSpec) {
			s.PriorTransactionID = domain.LabelTransactionID{}
		},
		"self-referencing prior link": func(s *domain.RehydrateLabelTransactionSpec) {
			s.PriorTransactionID = s.ID
		},
		"result observed before the request left": func(s *domain.RehydrateLabelTransactionSpec) {
			s.ResultObservedAt = labelTransactionAt
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := rehydratedLabelTransactionSpec(t)
			mutate(&spec)
			if _, err := domain.RehydrateLabelTransaction(spec); !errors.Is(err, domain.ErrInvalidRehydratedLabelTransaction) {
				t.Fatalf("err = %v; %s 被收下了", err, name)
			}
		})
	}
}

// Covers: ADR-0084 决定八——尚未收到渠道结果的行同样要回得来，且回来之后不带任何结果痕迹：
// 「未定案」在库里是真实且常见的一格（渠道墙未降前更是全部），把它当坏行挡掉，登记册就只剩
// 已定案的交易。
func TestARehydratedLabelTransactionKeepsUnfinalisedRowsUnfinalised(t *testing.T) {
	spec := rehydratedLabelTransactionSpec(t)
	spec.State = domain.LabelTransactionResultUncertain
	spec.ParcelResults = nil
	spec.ResultObservedAt = time.Time{}
	spec.FollowUpActions = nil

	transaction, err := domain.RehydrateLabelTransaction(spec)
	if err != nil {
		t.Fatalf("rehydrate label transaction: %v", err)
	}
	if transaction.Finalized() {
		t.Fatalf("结果不确定的行重建后被判为已定案")
	}
	if len(transaction.ParcelResults()) != 0 || !transaction.ResultObservedAt().IsZero() {
		t.Fatalf("未定案的行重建出了结果痕迹")
	}
}
