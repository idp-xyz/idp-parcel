package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var labelTransactionAt = time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)

func labelTransactionSpec(t *testing.T, parcels ...string) domain.EstablishLabelTransactionSpec {
	t.Helper()
	covered := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		covered = append(covered, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}
	return domain.EstablishLabelTransactionSpec{
		Tenant:                 mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                     mustValue(t, domain.NewLabelTransactionID, "label-txn-1"),
		CoveredParcels:         covered,
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelTransactionAt,
	}
}

func establishedLabelTransaction(t *testing.T, parcels ...string) domain.LabelTransaction {
	t.Helper()
	transaction, err := domain.EstablishLabelTransaction(labelTransactionSpec(t, parcels...))
	if err != nil {
		t.Fatalf("establish label transaction: %v", err)
	}
	return transaction
}

// Covers: CONTEXT 生命周期「建立交易 → 已提交渠道：交易固定所覆盖包裹、渠道账号、参与方
// 角色、合同、费率和责任依据」的领域面（ADR-0084 决定二）——这七项是出生属性，缺任何一项
// 交易都立不起来。少一项就收下，等于让一笔说不清是谁在哪个账号下按哪份合同发出的渠道请求
// 进入登记册，而这些正是渠道责任结果日后要归属的对象。
func TestEstablishingALabelTransactionFixesItsCoverageAndBasis(t *testing.T) {
	transaction := establishedLabelTransaction(t, "parcel-1", "parcel-2")

	if transaction.State() != domain.LabelTransactionEstablished {
		t.Fatalf("state = %s; 建立后应停在`已建立`", transaction.State())
	}
	if transaction.ID().String() != "label-txn-1" || transaction.Tenant().String() != "tenant-1" {
		t.Fatalf("key = (%s, %s); 聚合键是租户加面单交易标识", transaction.Tenant(), transaction.ID())
	}
	if got := stringValues(transaction.CoveredParcels()); len(got) != 2 || got[0] != "parcel-1" || got[1] != "parcel-2" {
		t.Fatalf("covered parcels = %v; 一笔交易可以覆盖多个明确包裹", got)
	}
	if transaction.ChannelAccount().String() != "channel-account-1" ||
		transaction.AccountHolder().String() != "party-holder-1" ||
		transaction.ServiceProvider().String() != "party-channel-1" ||
		transaction.SettlementCounterparty().String() != "party-settlement-1" ||
		transaction.Contract().String() != "contract-1" ||
		transaction.Rate().String() != "rate-1" ||
		transaction.ResponsibilityBasis().String() != "basis-snapshot-1" {
		t.Fatalf("固定下来的依据与建立时给的不一致")
	}
	if !transaction.EstablishedAt().Equal(labelTransactionAt) {
		t.Fatalf("established at = %s", transaction.EstablishedAt())
	}

	// 三个参与方角色各自独立缺席：渠道账号持有人、渠道服务方、合同与结算相对方在
	// party-commercial 里是三份不同的授权对象，压成一个「相对方」会让结算责任找错人。
	for name, mutate := range map[string]func(*domain.EstablishLabelTransactionSpec){
		"account holder": func(s *domain.EstablishLabelTransactionSpec) {
			s.AccountHolder = domain.ChannelAccountHolderReference{}
		},
		"service provider": func(s *domain.EstablishLabelTransactionSpec) {
			s.ServiceProvider = domain.ChannelServiceProviderReference{}
		},
		"settlement counterparty": func(s *domain.EstablishLabelTransactionSpec) {
			s.SettlementCounterparty = domain.SettlementCounterpartyReference{}
		},
		"channel account": func(s *domain.EstablishLabelTransactionSpec) { s.ChannelAccount = domain.ChannelAccountReference{} },
		"contract":        func(s *domain.EstablishLabelTransactionSpec) { s.Contract = domain.ChannelContractReference{} },
		"rate":            func(s *domain.EstablishLabelTransactionSpec) { s.Rate = domain.ChannelRateReference{} },
		"responsibility basis": func(s *domain.EstablishLabelTransactionSpec) {
			s.ResponsibilityBasis = domain.ResponsibilityBasisSnapshotReference{}
		},
		"business time": func(s *domain.EstablishLabelTransactionSpec) { s.EstablishedAt = time.Time{} },
	} {
		t.Run("missing "+name, func(t *testing.T) {
			spec := labelTransactionSpec(t, "parcel-1")
			mutate(&spec)
			if _, err := domain.EstablishLabelTransaction(spec); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
				t.Fatalf("err = %v; 缺 %s 的交易被收下了", err, name)
			}
		})
	}
}

// Covers: CONTEXT「一笔面单交易按一次真实渠道业务请求划分，可以关联一个或多个
// 明确包裹」（ADR-0084 决定二的「至少一件，重复拒绝」）——空覆盖的交易没有可归属的包裹级
// 结果，重复覆盖会让「每件覆盖包裹恰一条结果」的完备校验自相矛盾。固定还要挡住建立之后
// 从外部改写：调用方手里的切片与聚合内部共享底层数组时，覆盖范围其实并没有被固定。
func TestALabelTransactionCoversAtLeastOneParcelWithoutRepetition(t *testing.T) {
	if _, err := domain.EstablishLabelTransaction(labelTransactionSpec(t)); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 不覆盖任何包裹的交易被收下了", err)
	}

	repeated := labelTransactionSpec(t, "parcel-1", "parcel-1")
	if _, err := domain.EstablishLabelTransaction(repeated); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 重复覆盖同一件包裹被收下了", err)
	}

	blank := labelTransactionSpec(t, "parcel-1")
	blank.CoveredParcels = append(blank.CoveredParcels, domain.DeclaredParcelID{})
	if _, err := domain.EstablishLabelTransaction(blank); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 空包裹标识进了覆盖范围", err)
	}

	spec := labelTransactionSpec(t, "parcel-1", "parcel-2")
	transaction, err := domain.EstablishLabelTransaction(spec)
	if err != nil {
		t.Fatalf("establish label transaction: %v", err)
	}
	spec.CoveredParcels[0] = mustValue(t, domain.NewDeclaredParcelID, "parcel-9")
	transaction.CoveredParcels()[1] = mustValue(t, domain.NewDeclaredParcelID, "parcel-9")
	if got := stringValues(transaction.CoveredParcels()); got[0] != "parcel-1" || got[1] != "parcel-2" {
		t.Fatalf("covered parcels = %v; 建立时固定的覆盖范围被外部改写了", got)
	}
}

// Covers: CONTEXT 生命周期「建立交易 → 已提交渠道」与「已提交渠道 → 结果不确定：暂时无法
// 确认渠道是否受理；该结果不得直接按失败处理」——`结果不确定`是状态集合里独立的一格，从
// 这一格出发仍可走向成功/部分成功/失败（下一条测试）。越级（尚未提交就报不确定）与重复
// 提交都拒绝，且用与输入错误分开的哨兵：调用方对前者该去看这笔交易此刻在哪一格，对后者
// 才是改输入重来。
func TestALabelTransactionReachesTheChannelBeforeAnyResultIsObserved(t *testing.T) {
	established := establishedLabelTransaction(t, "parcel-1")

	if _, err := established.MarkResultUncertain(); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Fatalf("err = %v; 尚未提交渠道就报结果不确定被收下了", err)
	}

	submittedAt := labelTransactionAt.Add(time.Minute)
	submitted, err := established.SubmitToChannel(submittedAt)
	if err != nil {
		t.Fatalf("submit to channel: %v", err)
	}
	if submitted.State() != domain.LabelTransactionSubmitted {
		t.Fatalf("state = %s; 建立后提交应到`已提交渠道`", submitted.State())
	}
	if !submitted.SubmittedAt().Equal(submittedAt) {
		t.Fatalf("submitted at = %s", submitted.SubmittedAt())
	}
	if established.State() != domain.LabelTransactionEstablished {
		t.Fatalf("原值被就地改写；转移必须交回新值")
	}

	if _, err := submitted.SubmitToChannel(submittedAt); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Fatalf("err = %v; 同一笔交易被提交了两次", err)
	}
	if _, err := established.SubmitToChannel(labelTransactionAt.Add(-time.Minute)); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 提交时间早于建立时间被收下了", err)
	}

	uncertain, err := submitted.MarkResultUncertain()
	if err != nil {
		t.Fatalf("mark result uncertain: %v", err)
	}
	if uncertain.State() != domain.LabelTransactionResultUncertain {
		t.Fatalf("state = %s; 结果不确定是独立一格，不是失败的别名", uncertain.State())
	}
	if uncertain.State() == domain.LabelTransactionFailed {
		t.Fatalf("结果不确定被当成失败处理")
	}
}

func submittedLabelTransaction(t *testing.T, parcels ...string) domain.LabelTransaction {
	t.Helper()
	transaction, err := establishedLabelTransaction(t, parcels...).SubmitToChannel(labelTransactionAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("submit to channel: %v", err)
	}
	return transaction
}

func acceptedParcelResult(t *testing.T, parcel, identifier string) domain.LabelTransactionParcelResultSpec {
	t.Helper()
	return domain.LabelTransactionParcelResultSpec{
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, parcel),
		Accepted:   true,
		Identifier: mustValue(t, domain.NewChannelParcelIdentifier, identifier),
	}
}

func refusedParcelResult(t *testing.T, parcel, reason string) domain.LabelTransactionParcelResultSpec {
	t.Helper()
	return domain.LabelTransactionParcelResultSpec{
		Parcel: mustValue(t, domain.NewDeclaredParcelID, parcel),
		Reason: mustValue(t, domain.NewChannelResultReasonReference, reason),
	}
}

// Covers: CONTEXT 生命周期「已提交渠道或结果不确定 → 成功、部分成功或失败：同时保存整笔
// 交易结果和各包裹结果」与「多包裹面单交易可以具有共同交易结果，也必须分别保存每个
// 包裹的业务结果；部分成功、部分失败或不同作废范围不得压缩成一个无法解释的通用状态」
// （ADR-0084 决定三）——两层在同一次写入里落地，校验只到结构为止：覆盖完备、无越界、无重复，
// 每条自身说得通（受理才有包裹级标识，未受理必须给原因）。
func TestRecordingAChannelResultWritesBothLevelsAtOnce(t *testing.T) {
	submitted := submittedLabelTransaction(t, "parcel-1", "parcel-2")
	observedAt := labelTransactionAt.Add(2 * time.Minute)
	spec := domain.RecordChannelResultSpec{
		Outcome: domain.LabelTransactionPartiallySucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
			refusedParcelResult(t, "parcel-2", "ADDRESS_UNSUPPORTED"),
		},
		ObservedAt: observedAt,
	}

	recorded, err := submitted.RecordChannelResult(spec)
	if err != nil {
		t.Fatalf("record channel result: %v", err)
	}
	if recorded.State() != domain.LabelTransactionPartiallySucceeded {
		t.Fatalf("state = %s; 交易级结果没落下来", recorded.State())
	}
	if !recorded.ResultObservedAt().Equal(observedAt) {
		t.Fatalf("observed at = %s", recorded.ResultObservedAt())
	}
	accepted, found := recorded.ParcelResult(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !found || !accepted.Accepted() || accepted.Identifier().String() != "channel-parcel-1" {
		t.Fatalf("parcel-1 result = %+v, found = %v; 受理包裹应带回取得的包裹级标识", accepted, found)
	}
	refused, found := recorded.ParcelResult(mustValue(t, domain.NewDeclaredParcelID, "parcel-2"))
	if !found || refused.Accepted() || refused.Reason().String() != "ADDRESS_UNSUPPORTED" {
		t.Fatalf("parcel-2 result = %+v, found = %v; 未受理包裹应带回原因引用", refused, found)
	}

	for name, mutate := range map[string]func(*domain.RecordChannelResultSpec){
		"incomplete coverage": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults = s.ParcelResults[:1]
		},
		"parcel outside the coverage": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults[1] = acceptedParcelResult(t, "parcel-9", "channel-parcel-9")
		},
		"repeated parcel": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults[1] = acceptedParcelResult(t, "parcel-1", "channel-parcel-2")
		},
		"accepted without a channel identifier": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults[0].Identifier = domain.ChannelParcelIdentifier{}
		},
		"refused without a reason": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults[1].Reason = domain.ChannelResultReasonReference{}
		},
		"refused yet carrying a channel identifier": func(s *domain.RecordChannelResultSpec) {
			s.ParcelResults[1].Identifier = mustValue(t, domain.NewChannelParcelIdentifier, "channel-parcel-2")
		},
		"outcome outside the closed result set": func(s *domain.RecordChannelResultSpec) {
			s.Outcome = domain.LabelTransactionResultUncertain
		},
		"result observed before the request left": func(s *domain.RecordChannelResultSpec) {
			s.ObservedAt = labelTransactionAt
		},
	} {
		t.Run(name, func(t *testing.T) {
			broken := domain.RecordChannelResultSpec{
				Outcome: domain.LabelTransactionPartiallySucceeded,
				ParcelResults: []domain.LabelTransactionParcelResultSpec{
					acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
					refusedParcelResult(t, "parcel-2", "ADDRESS_UNSUPPORTED"),
				},
				ObservedAt: observedAt,
			}
			mutate(&broken)
			if _, err := submitted.RecordChannelResult(broken); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
				t.Fatalf("err = %v; %s 被收下了", err, name)
			}
		})
	}
}

// Covers: CONTEXT「交易级失败不能推导包裹失败」与生命周期「不能由一个层次覆盖另一个
// 层次」（ADR-0084 决定三「不做跨层推导」）——一笔交易级失败里仍可以有已被受理的包裹（渠道
// 整单判失败但个别包裹已下号），两层各自记各自的事实。这条测试守的是「别顺手加一致性校验」：
// 加上「失败 ⇒ 全部未受理」这类看似合理的规则，就是用一个层次覆盖另一个层次。
func TestNeitherResultLevelIsDerivedFromTheOther(t *testing.T) {
	submitted := submittedLabelTransaction(t, "parcel-1", "parcel-2")
	observedAt := labelTransactionAt.Add(2 * time.Minute)

	failedYetOneAccepted, err := submitted.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome: domain.LabelTransactionFailed,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
			refusedParcelResult(t, "parcel-2", "CHANNEL_REJECTED"),
		},
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("record channel result: %v; 交易级失败被拿去推导包裹级结果了", err)
	}
	accepted, found := failedYetOneAccepted.ParcelResult(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !found || !accepted.Accepted() {
		t.Fatalf("parcel-1 accepted = %v, found = %v; 包裹级受理被交易级失败抹掉了", accepted.Accepted(), found)
	}

	succeededYetOneRefused, err := submitted.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome: domain.LabelTransactionSucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
			refusedParcelResult(t, "parcel-2", "CHANNEL_REJECTED"),
		},
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("record channel result: %v; 包裹级结果被拿去推翻交易级结果了", err)
	}
	if succeededYetOneRefused.State() != domain.LabelTransactionSucceeded {
		t.Fatalf("state = %s; 交易级结果被包裹级结果改写了", succeededYetOneRefused.State())
	}
}

// Covers: CONTEXT 生命周期「已提交渠道或结果不确定 → 成功、部分成功或失败」的两侧闸门——
// 出发格只有这两个（没提交就没有渠道结果可言），且结果只记一次；重记要走的是后续动作或
// 新交易，不是原地改写已经形成的渠道责任结果。
func TestAChannelResultIsRecordedOnceFromAnAdmittedState(t *testing.T) {
	observedAt := labelTransactionAt.Add(2 * time.Minute)
	result := func(t *testing.T) domain.RecordChannelResultSpec {
		t.Helper()
		return domain.RecordChannelResultSpec{
			Outcome:       domain.LabelTransactionSucceeded,
			ParcelResults: []domain.LabelTransactionParcelResultSpec{acceptedParcelResult(t, "parcel-1", "channel-parcel-1")},
			ObservedAt:    observedAt,
		}
	}

	established := establishedLabelTransaction(t, "parcel-1")
	if _, err := established.RecordChannelResult(result(t)); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Fatalf("err = %v; 尚未提交渠道的交易收下了渠道结果", err)
	}

	uncertain, err := submittedLabelTransaction(t, "parcel-1").MarkResultUncertain()
	if err != nil {
		t.Fatalf("mark result uncertain: %v", err)
	}
	recorded, err := uncertain.RecordChannelResult(result(t))
	if err != nil {
		t.Fatalf("record channel result: %v; 结果不确定也是合法出发格", err)
	}
	if _, err := recorded.RecordChannelResult(result(t)); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Fatalf("err = %v; 已形成的渠道结果被第二次记录改写了", err)
	}
}

func recordedLabelTransaction(
	t *testing.T,
	outcome domain.LabelTransactionState,
	parcelResults ...domain.LabelTransactionParcelResultSpec,
) domain.LabelTransaction {
	t.Helper()
	parcels := make([]string, 0, len(parcelResults))
	for _, result := range parcelResults {
		parcels = append(parcels, result.Parcel.String())
	}
	recorded, err := submittedLabelTransaction(t, parcels...).RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome:       outcome,
		ParcelResults: parcelResults,
		ObservedAt:    labelTransactionAt.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("record channel result: %v", err)
	}
	return recorded
}

// Covers: CONTEXT「面单交易定案」——交易级及其相关包裹级结果已经明确、不再处于结果待确认
// 状态（ADR-0084 决定四：定案是派生谓词，不是存储状态）。逐包裹完备性已由记录时的结构校验
// 保证，所以三个结果格就是定案的全部条件；`结果不确定`恰恰是待确认，不定案。
//
// 存一列「定案状态」会造第二个来源：改结果不改列的一次写入让两处各说各话。这条测试同时守
// 「成功且仍有效的交易可以已经定案」——成功不是未定案的同义词。
func TestFinalisationIsDerivedFromTheTransactionLevelResult(t *testing.T) {
	accepted := acceptedParcelResult(t, "parcel-1", "channel-parcel-1")

	if established := establishedLabelTransaction(t, "parcel-1"); established.Finalized() {
		t.Fatalf("刚建立的交易被判为已定案")
	}
	if submitted := submittedLabelTransaction(t, "parcel-1"); submitted.Finalized() {
		t.Fatalf("刚提交渠道、结果未回的交易被判为已定案")
	}
	uncertain, err := submittedLabelTransaction(t, "parcel-1").MarkResultUncertain()
	if err != nil {
		t.Fatalf("mark result uncertain: %v", err)
	}
	if uncertain.Finalized() {
		t.Fatalf("结果不确定被判为已定案；那正是结果待确认")
	}

	for _, outcome := range []domain.LabelTransactionState{
		domain.LabelTransactionSucceeded,
		domain.LabelTransactionPartiallySucceeded,
		domain.LabelTransactionFailed,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			if !recordedLabelTransaction(t, outcome, accepted).Finalized() {
				t.Fatalf("交易级结果为 %s 却未定案", outcome)
			}
		})
	}

	// 读面不重建聚合（读的是「登记过什么」），却同样要答定案。它必须用同一条规则算，
	// 而不是在适配器里另写一个「state 是不是那三格」——两处各写一份，某天集合增减时
	// 只会改到其中一处。
	for _, state := range []domain.LabelTransactionState{
		domain.LabelTransactionEstablished,
		domain.LabelTransactionSubmitted,
		domain.LabelTransactionResultUncertain,
		domain.LabelTransactionSucceeded,
		domain.LabelTransactionPartiallySucceeded,
		domain.LabelTransactionFailed,
	} {
		transaction := recordedLabelTransaction(t, domain.LabelTransactionSucceeded, accepted)
		if state.IsChannelResult() != (state == domain.LabelTransactionSucceeded ||
			state == domain.LabelTransactionPartiallySucceeded ||
			state == domain.LabelTransactionFailed) {
			t.Fatalf("%s 的定案派生与结果格集合不一致", state)
		}
		if transaction.State().IsChannelResult() != transaction.Finalized() {
			t.Fatalf("聚合与读面对同一笔交易的定案派生不一致")
		}
	}
}

// Covers: CONTEXT 生命周期「渠道作废、渠道退款和替代是针对明确交易范围或包裹范围形成的后续
// 业务动作及结果，可以与未受影响包裹的原结果并存；它们不构成一条覆盖整笔交易的统一线性
// 状态」（ADR-0084 决定五）——聚合上是追加式清单：原交易级结果、原包裹结果与定案谓词都不动，
// 未受影响的包裹继续按原结果读。范围要么整笔要么指名包裹，指名的必须在覆盖范围内。
func TestFollowUpActionsAccumulateWithoutRewritingTheOriginalResults(t *testing.T) {
	recorded := recordedLabelTransaction(
		t,
		domain.LabelTransactionPartiallySucceeded,
		acceptedParcelResult(t, "parcel-1", "channel-parcel-1"),
		acceptedParcelResult(t, "parcel-2", "channel-parcel-2"),
	)
	voidedAt := labelTransactionAt.Add(3 * time.Minute)
	refundedAt := labelTransactionAt.Add(4 * time.Minute)

	voided, err := recorded.AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelVoidAction,
		Parcels:    []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-2")},
		Reason:     mustValue(t, domain.NewChannelResultReasonReference, "CUSTOMER_WITHDREW"),
		OccurredAt: voidedAt,
	})
	if err != nil {
		t.Fatalf("append follow-up action: %v", err)
	}
	both, err := voided.AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelRefundAction,
		Reason:     mustValue(t, domain.NewChannelResultReasonReference, "CHANNEL_GOODWILL"),
		OccurredAt: refundedAt,
	})
	if err != nil {
		t.Fatalf("append whole-transaction follow-up action: %v", err)
	}

	actions := both.FollowUpActions()
	if len(actions) != 2 {
		t.Fatalf("follow-up actions = %d; 追加式清单没留住两条", len(actions))
	}
	if actions[0].Kind() != domain.ChannelVoidAction || actions[0].CoversWholeTransaction() {
		t.Fatalf("第一条应是指名包裹范围的渠道作废，得到 %s", actions[0].Kind())
	}
	if got := stringValues(actions[0].Parcels()); len(got) != 1 || got[0] != "parcel-2" {
		t.Fatalf("第一条范围 = %v; 指名包裹范围没留住", got)
	}
	if !actions[1].CoversWholeTransaction() || len(actions[1].Parcels()) != 0 {
		t.Fatalf("第二条应是整笔范围")
	}
	if !actions[1].OccurredAt().Equal(refundedAt) {
		t.Fatalf("occurred at = %s", actions[1].OccurredAt())
	}

	// 原结果原样保留：被作废的 parcel-2 与未受影响的 parcel-1 都还读回渠道当时给的受理结果，
	// 交易级结果与定案谓词也不因后续动作改变（渠道退款明文不属定案条件）。
	if both.State() != domain.LabelTransactionPartiallySucceeded || !both.Finalized() {
		t.Fatalf("state = %s, finalized = %v; 后续动作改写了交易级结果", both.State(), both.Finalized())
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		result, found := both.ParcelResult(mustValue(t, domain.NewDeclaredParcelID, parcel))
		if !found || !result.Accepted() {
			t.Fatalf("%s 的原结果被后续动作改写了", parcel)
		}
	}
	if len(recorded.FollowUpActions()) != 0 || len(voided.FollowUpActions()) != 1 {
		t.Fatalf("追加动作就地改写了此前的值")
	}
	both.FollowUpActions()[0] = domain.LabelTransactionFollowUpAction{}
	if both.FollowUpActions()[0].Kind() != domain.ChannelVoidAction {
		t.Fatalf("交回的清单与内部共享底层数组，追加式记录可被外部抹掉")
	}
}

// Covers: 后续动作的两侧闸门——它是「后续」：CONTEXT 把渠道作废/退款/替代定义为对已经形成的
// 结果范围采取的动作，未定案的交易上没有可作用的结果；范围内的包裹必须真在这笔交易的覆盖
// 范围里，否则一件本交易从未覆盖的包裹会凭空得到作废痕迹。
func TestAFollowUpActionNeedsAResultAndAScopeInsideTheCoverage(t *testing.T) {
	valid := func(t *testing.T) domain.FollowUpActionSpec {
		t.Helper()
		return domain.FollowUpActionSpec{
			Kind:       domain.ChannelReplacementAction,
			Parcels:    []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1")},
			Reason:     mustValue(t, domain.NewChannelResultReasonReference, "REISSUED"),
			OccurredAt: labelTransactionAt.Add(3 * time.Minute),
		}
	}

	submitted := submittedLabelTransaction(t, "parcel-1")
	if _, err := submitted.AppendFollowUpAction(valid(t)); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Fatalf("err = %v; 结果尚未形成就追加了后续动作", err)
	}

	recorded := recordedLabelTransaction(t, domain.LabelTransactionSucceeded, acceptedParcelResult(t, "parcel-1", "channel-parcel-1"))
	for name, mutate := range map[string]func(*domain.FollowUpActionSpec){
		"parcel outside the coverage": func(s *domain.FollowUpActionSpec) {
			s.Parcels = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-9")}
		},
		"repeated parcel": func(s *domain.FollowUpActionSpec) {
			parcel := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
			s.Parcels = []domain.DeclaredParcelID{parcel, parcel}
		},
		"unknown kind":   func(s *domain.FollowUpActionSpec) { s.Kind = domain.FollowUpActionKindInvalid },
		"missing reason": func(s *domain.FollowUpActionSpec) { s.Reason = domain.ChannelResultReasonReference{} },
		"missing business time": func(s *domain.FollowUpActionSpec) {
			s.OccurredAt = time.Time{}
		},
		"acting before the result came back": func(s *domain.FollowUpActionSpec) {
			s.OccurredAt = labelTransactionAt
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := valid(t)
			mutate(&spec)
			if _, err := recorded.AppendFollowUpAction(spec); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
				t.Fatalf("err = %v; %s 被收下了", err, name)
			}
		})
	}
}

// Covers: CONTEXT 生命周期「原交易 → 被替代：失败后的新申请、换单或明确重新下单形成新交易，
// 并保留重试或替代关系」与「一个包裹可以关联多笔具有重试、替代、作废或换单关系的
// 交易」（ADR-0084 决定二照 PriorRequestLink 先例）——关系是**新**交易的出生属性，原交易的
// 状态、结果与后续动作一概不动：那句话改的是新交易的出处，不是原交易的状态。
func TestARetryOrReplacementIsBornOnTheNewTransaction(t *testing.T) {
	prior := recordedLabelTransaction(t, domain.LabelTransactionFailed, refusedParcelResult(t, "parcel-1", "CHANNEL_REJECTED"))

	link, err := domain.EstablishPriorLabelTransactionLink(prior, domain.LabelTransactionRetry)
	if err != nil {
		t.Fatalf("establish prior label transaction link: %v", err)
	}
	spec := labelTransactionSpec(t, "parcel-1")
	spec.ID = mustValue(t, domain.NewLabelTransactionID, "label-txn-2")
	spec.PriorLink = link
	retry, err := domain.EstablishLabelTransaction(spec)
	if err != nil {
		t.Fatalf("establish retry transaction: %v", err)
	}

	borne, established := retry.PriorLink()
	if !established || borne.Kind() != domain.LabelTransactionRetry {
		t.Fatalf("prior link = %+v, established = %v; 重试关系没随新交易出生", borne, established)
	}
	if borne.PriorTransactionID().String() != "label-txn-1" {
		t.Fatalf("prior = %s; 关系没指回被重试的那笔交易", borne.PriorTransactionID())
	}
	if prior.State() != domain.LabelTransactionFailed || len(prior.FollowUpActions()) != 0 {
		t.Fatalf("原交易被改动了：state = %s", prior.State())
	}
	if _, borneOnFirst := establishedLabelTransaction(t, "parcel-1").PriorLink(); borneOnFirst {
		t.Fatalf("首笔交易报告了一个并不存在的出处")
	}
}

// Covers: CONTEXT 生命周期「已提交渠道 → 结果不确定：暂时无法确认渠道是否受理；该结果不得
// 直接按失败处理」——对一笔尚未定案的交易发起重试或替代，正是把它按失败处理了。哨兵与输入
// 错误分开：调用方该做的是等渠道结果回来，不是改参数重发。
func TestNoRetryOrReplacementLinksToAnUnfinalisedTransaction(t *testing.T) {
	for name, prior := range map[string]domain.LabelTransaction{
		"submitted to the channel": submittedLabelTransaction(t, "parcel-1"),
		"established only":         establishedLabelTransaction(t, "parcel-1"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.EstablishPriorLabelTransactionLink(prior, domain.LabelTransactionReplacement)
			if !errors.Is(err, domain.ErrPriorLabelTransactionNotFinalized) {
				t.Fatalf("err = %v, want ErrPriorLabelTransactionNotFinalized", err)
			}
		})
	}

	uncertain, err := submittedLabelTransaction(t, "parcel-1").MarkResultUncertain()
	if err != nil {
		t.Fatalf("mark result uncertain: %v", err)
	}
	if _, err := domain.EstablishPriorLabelTransactionLink(uncertain, domain.LabelTransactionRetry); !errors.Is(err, domain.ErrPriorLabelTransactionNotFinalized) {
		t.Fatalf("err = %v; 结果不确定的交易被当成失败重试了", err)
	}

	finalized := recordedLabelTransaction(t, domain.LabelTransactionFailed, refusedParcelResult(t, "parcel-1", "CHANNEL_REJECTED"))
	if _, err := domain.EstablishPriorLabelTransactionLink(finalized, domain.LabelTransactionLinkKindInvalid); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 没有方向的关系被收下了", err)
	}

	selfLink, err := domain.EstablishPriorLabelTransactionLink(finalized, domain.LabelTransactionReplacement)
	if err != nil {
		t.Fatalf("establish prior label transaction link: %v", err)
	}
	selfReferencing := labelTransactionSpec(t, "parcel-1")
	selfReferencing.PriorLink = selfLink
	if _, err := domain.EstablishLabelTransaction(selfReferencing); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Fatalf("err = %v; 一笔指着自己的替代关系被收下了", err)
	}
}
