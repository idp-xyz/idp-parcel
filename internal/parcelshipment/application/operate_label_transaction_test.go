package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证面单交易写侧编排链五步（ADR-0084 的写入方，label-channel/06）。
//
// 出向调用不在这条链上：编排只在提交那一步之后留缝，渠道回来的答案经 MarkResultUncertain 或
// RecordChannelResult 交回来。因此这里没有任何渠道替身——有的话就说明编排替调用方发了请求。

// Covers: CONTEXT 生命周期第一句「建立时固定覆盖与依据」——建立落库且状态为`已建立`，
// 建立时间取编排时钟而不是调用方自报（发起交易是我方的动作）。
func TestEstablishingALabelTransactionFixesItsCoverageAndBasis(t *testing.T) {
	fixture := newLabelTransactionFixture(t)

	result, err := fixture.handler.Establish(context.Background(), fixture.establishCommand(t, "LT-1"))
	if err != nil {
		t.Fatalf("establish: %v", err)
	}

	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}
	transaction, present := result.Transaction()
	if !present {
		t.Fatal("一次成立的建立没有交回交易")
	}
	if transaction.State() != domain.LabelTransactionEstablished {
		t.Fatalf("state = %q, want ESTABLISHED", transaction.State())
	}
	if !transaction.EstablishedAt().Equal(handlerClockAt) {
		t.Fatalf("establishedAt = %v, want 编排时钟 %v", transaction.EstablishedAt(), handlerClockAt)
	}
	if fixture.repository.inserted == nil {
		t.Fatal("建立没有到达仓储")
	}
	if len(fixture.repository.inserted.CoveredParcels()) != 2 {
		t.Fatalf("覆盖范围 = %v，建立时固定的两件包裹没进聚合", fixture.repository.inserted.CoveredParcels())
	}
}

// Covers: ADR-0031 写入代数`已存在`那一格——同标识再建一次是重放，读回既有那一笔，
// **不覆盖**。建立时固定的依据此后改不了，所以「同标识不同内容」不是一次更正。
func TestEstablishingTheSameTransactionTwiceReadsBackTheExistingOne(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	first := fixture.mustEstablish(t, "LT-1")
	fixture.repository.inserted = nil

	result, err := fixture.handler.Establish(context.Background(), fixture.establishCommand(t, "LT-1"))
	if err != nil {
		t.Fatalf("establish again: %v", err)
	}

	if result.Outcome() != application.LabelTransactionAlreadyApplied {
		t.Fatalf("outcome = %q, want ALREADY_APPLIED", result.Outcome())
	}
	if fixture.repository.inserted != nil {
		t.Fatal("重放又插了一笔——同一标识不得产生第二笔交易")
	}
	existing, present := result.Transaction()
	if !present || existing.ID() != first.ID() {
		t.Fatalf("重放交回的不是既有那一笔：%#v", existing)
	}
}

// Covers: CONTEXT「不得直接按失败处理」在建立处的落点——对一笔`结果不确定`的原交易发起
// 重试，就是把它按失败处理了。这一格与`输入未受理`分开：恢复动作是等结果回来，不是改输入。
func TestARetryOffAnUnfinalizedPriorTransactionIsRefusedOnItsOwnGrade(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.mustMarkUncertain(t, "LT-1")

	command := fixture.establishCommand(t, "LT-2")
	command.PriorTransactionID = mustValue(t, domain.NewLabelTransactionID, "LT-1")
	command.PriorLinkKind = domain.LabelTransactionRetry
	// 清掉夹具建 LT-1 时留下的那一笔，下面断言的才是本次建立有没有落库。
	fixture.repository.inserted = nil

	result, err := fixture.handler.Establish(context.Background(), command)
	if err != nil {
		t.Fatalf("establish retry: %v", err)
	}

	if result.Outcome() != application.LabelTransactionPriorNotFinalized {
		t.Fatalf("outcome = %q, want PRIOR_NOT_FINALIZED", result.Outcome())
	}
	if _, present := result.Transaction(); present {
		t.Fatal("被拒的建立交回了一笔交易——它根本没有形成")
	}
	if fixture.repository.inserted != nil {
		t.Fatal("被拒的建立仍然落了库")
	}
}

// Covers: ADR-0084 决定二——关系是**新**交易的出生属性，原交易一概不动。
func TestAReplacementOffAFinalizedPriorCarriesTheLinkAndLeavesThePriorAlone(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	prior := fixture.mustRecordResult(t, "LT-1", domain.LabelTransactionFailed, false)

	command := fixture.establishCommand(t, "LT-2")
	command.PriorTransactionID = mustValue(t, domain.NewLabelTransactionID, "LT-1")
	command.PriorLinkKind = domain.LabelTransactionReplacement

	result, err := fixture.handler.Establish(context.Background(), command)
	if err != nil {
		t.Fatalf("establish replacement: %v", err)
	}
	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}

	replacement, _ := result.Transaction()
	link, established := replacement.PriorLink()
	if !established || link.PriorTransactionID() != prior.ID() || link.Kind() != domain.LabelTransactionReplacement {
		t.Fatalf("新交易没带上替代关系：%#v", link)
	}
	stored := fixture.repository.stored[prior.ID()]
	if stored.State() != domain.LabelTransactionFailed || len(stored.FollowUpActions()) != 0 {
		t.Fatal("建立替代交易动了原交易——关系只存在于新交易上")
	}
}

// Covers: 提交那一步的时间取编排时钟（发出请求是我方的动作），且状态推进到`已提交渠道`。
func TestSubmittingToChannelAdvancesTheStateWithTheOrchestrationClock(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")

	result, err := fixture.handler.SubmitToChannel(context.Background(), application.SubmitLabelTransactionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}
	submitted, _ := result.Transaction()
	if submitted.State() != domain.LabelTransactionSubmitted {
		t.Fatalf("state = %q, want SUBMITTED_TO_CHANNEL", submitted.State())
	}
	if !submitted.SubmittedAt().Equal(handlerClockAt) {
		t.Fatalf("submittedAt = %v, want 编排时钟 %v", submitted.SubmittedAt(), handlerClockAt)
	}
}

// Covers: **ADR-0090「`答案未确定` 不得重发」在本上下文的落点。**
//
// `结果不确定`落库之后，重发路径读到的不再是`已提交渠道`，SubmitToChannel 的状态门因此自动
// 挡住第二次提交——纪律由状态机交付，不靠调用方自觉。这一条同时钉住那次拒绝落在`状态不允许`
// 而不是`输入未受理`：恢复动作是去查渠道，不是改参数重发。
func TestOnceTheResultIsUncertainTheTransactionCannotBeSubmittedAgain(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.mustMarkUncertain(t, "LT-1")

	result, err := fixture.handler.SubmitToChannel(context.Background(), application.SubmitLabelTransactionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
	})
	if err != nil {
		t.Fatalf("submit again: %v", err)
	}

	if result.Outcome() != application.LabelTransactionStepNotAdmitted {
		t.Fatalf("outcome = %q, want STATE_NOT_ADMITTED——结果不确定时重发会重复购买面单", result.Outcome())
	}
	stuck, present := result.Transaction()
	if !present || stuck.State() != domain.LabelTransactionResultUncertain {
		t.Fatal("被拒时没交回交易停在哪一格，调用方还得再读一次")
	}
}

// Covers: CONTEXT「部分成功、部分失败或不同作废范围不得压缩成一个无法解释的通用状态」——
// 交易级`部分成功`与逐包裹两条不同结果一次写下，两层各自留存不互推。
func TestRecordingAChannelResultKeepsBothLayersWithoutFlatteningThem(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")

	observedAt := handlerClockAt.Add(2 * time.Hour)
	result, err := fixture.handler.RecordChannelResult(context.Background(), application.RecordLabelChannelResultCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		Outcome:       domain.LabelTransactionPartiallySucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			{Parcel: fixture.parcels[0], Accepted: true, Identifier: mustValue(t, domain.NewChannelParcelIdentifier, "CN-1")},
			{Parcel: fixture.parcels[1], Accepted: false, Reason: mustValue(t, domain.NewChannelResultReasonReference, "ADDRESS_INVALID")},
		},
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("record result: %v", err)
	}

	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}
	recorded, _ := result.Transaction()
	if recorded.State() != domain.LabelTransactionPartiallySucceeded {
		t.Fatalf("state = %q, want PARTIALLY_SUCCEEDED", recorded.State())
	}
	if !recorded.ResultObservedAt().Equal(observedAt) {
		t.Fatalf("observedAt = %v, want 渠道给的 %v——外部事实的时间不代铸", recorded.ResultObservedAt(), observedAt)
	}
	first, _ := recorded.ParcelResult(fixture.parcels[0])
	second, _ := recorded.ParcelResult(fixture.parcels[1])
	if !first.Accepted() || first.Identifier().String() != "CN-1" {
		t.Fatalf("受理那件没留下渠道标识：%#v", first)
	}
	if second.Accepted() || second.Reason().String() != "ADDRESS_INVALID" {
		t.Fatalf("未受理那件没留下原因：%#v", second)
	}
}

// Covers: matchResultsToCoverage 的完备性在应用层的答案——漏一件包裹落`输入未受理`而不是
// `状态不允许`，两者恢复动作不同（改输入重来 vs 去看它停在哪一格）。
func TestAResultThatMissesACoveredParcelIsRefusedAsBadInput(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")

	result, err := fixture.handler.RecordChannelResult(context.Background(), application.RecordLabelChannelResultCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		Outcome:       domain.LabelTransactionSucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			{Parcel: fixture.parcels[0], Accepted: true, Identifier: mustValue(t, domain.NewChannelParcelIdentifier, "CN-1")},
		},
		ObservedAt: handlerClockAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("record result: %v", err)
	}

	if result.Outcome() != application.LabelTransactionNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
}

// Covers: CONTEXT「后续动作可以与未受影响包裹的原结果并存」——作废追加到已定案交易上，
// 不改交易级状态、不改任何包裹结果、也不动定案谓词。
func TestAFollowUpActionIsAppendedWithoutRewritingTheResult(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.mustRecordResult(t, "LT-1", domain.LabelTransactionSucceeded, true)

	result, err := fixture.handler.AppendFollowUpAction(context.Background(), application.AppendLabelFollowUpActionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		Kind:          domain.ChannelVoidAction,
		Parcels:       []domain.DeclaredParcelID{fixture.parcels[0]},
		Reason:        mustValue(t, domain.NewChannelResultReasonReference, "CUSTOMER_CANCELLED"),
		OccurredAt:    handlerClockAt.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("append follow-up: %v", err)
	}

	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}
	appended, _ := result.Transaction()
	if appended.State() != domain.LabelTransactionSucceeded || !appended.Finalized() {
		t.Fatal("追加后续动作改了交易级结果或定案谓词")
	}
	actions := appended.FollowUpActions()
	if len(actions) != 1 || actions[0].Kind() != domain.ChannelVoidAction {
		t.Fatalf("后续动作没留下：%#v", actions)
	}
	voided, _ := appended.ParcelResult(fixture.parcels[0])
	if !voided.Accepted() {
		t.Fatal("渠道作废改写了「当初有没有被受理」——那是两件事")
	}
}

// Covers: 后续动作只对已定案交易开放——结果还没回来时没有可作用的对象。
func TestAFollowUpActionOnAnUnfinalizedTransactionIsRefused(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")

	result, err := fixture.handler.AppendFollowUpAction(context.Background(), application.AppendLabelFollowUpActionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		Kind:          domain.ChannelRefundAction,
		Reason:        mustValue(t, domain.NewChannelResultReasonReference, "OVERCHARGE"),
		OccurredAt:    handlerClockAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("append follow-up: %v", err)
	}

	if result.Outcome() != application.LabelTransactionStepNotAdmitted {
		t.Fatalf("outcome = %q, want STATE_NOT_ADMITTED", result.Outcome())
	}
}

// Covers: ADR-0031——版本冲突是业务答案不是错误，且**不交回手上那份陈旧的聚合**：交回去
// 调用方会把它当最新的接着改，而正确动作是重读再重放。
func TestARevisionConflictIsAnAnswerAndCarriesNoStaleAggregate(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.repository.saveOutcome = ports.LabelTransactionRevisionConflict

	result, err := fixture.handler.SubmitToChannel(context.Background(), application.SubmitLabelTransactionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if result.Outcome() != application.LabelTransactionWriteConflict {
		t.Fatalf("outcome = %q, want REVISION_CONFLICT", result.Outcome())
	}
	if _, present := result.Transaction(); present {
		t.Fatal("版本冲突交回了一份陈旧聚合")
	}
}

// Covers: 指名一笔查不到的交易落`输入未受理`——改输入重来。不细分「不存在」与「不属于你」，
// 仓储以租户为键，否定结果本来就不携带这个差别。
func TestAnUnknownTransactionIsRefusedAsBadInput(t *testing.T) {
	fixture := newLabelTransactionFixture(t)

	result, err := fixture.handler.SubmitToChannel(context.Background(), application.SubmitLabelTransactionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-NOPE"),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if result.Outcome() != application.LabelTransactionNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
}

// Covers: ADR-0134 决定一 / 四——记录渠道结果那一拍 `Save` 成功后，按覆盖包裹**逐件**交一份终局判断
// 意图（一封一包裹），意图带交易标识、包裹、`Save` 成功那一代的版本与哪一拍；前三步一封不交。
func TestRecordingAChannelResultHandsOffOneJudgmentIntentPerCoveredParcel(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	if len(fixture.judgments.intents) != 0 {
		t.Fatalf("建立与提交交出了 %d 份判断意图——前三步不触发判断", len(fixture.judgments.intents))
	}

	recorded := fixture.mustRecordResult(t, "LT-1", domain.LabelTransactionSucceeded, true)

	intents := fixture.judgments.intents
	if len(intents) != len(fixture.parcels) {
		t.Fatalf("意图 %d 份，want 覆盖包裹每件一份（%d）", len(intents), len(fixture.parcels))
	}
	for index, parcel := range fixture.parcels {
		intent := intents[index]
		if intent.Parcel != parcel || intent.TransactionID != recorded.ID() || intent.Tenant != fixture.tenant {
			t.Fatalf("第 %d 份意图指错了对象：%#v", index, intent)
		}
		if intent.Beat != ports.LabelTransactionResultRecorded {
			t.Fatalf("第 %d 份意图的拍 = %q, want RESULT_RECORDED", index, intent.Beat)
		}
		// 版本取 `Save` 成功那一代：仓储按预期版本加一写回，聚合本体上仍是读出时那一代。
		if intent.Revision != recorded.Revision()+1 {
			t.Fatalf("第 %d 份意图的版本 = %d, want %d", index, intent.Revision, recorded.Revision()+1)
		}
		if !intent.OccurredAt.Equal(handlerClockAt.Add(time.Hour)) {
			t.Fatalf("第 %d 份意图的业务时间 = %v, want 渠道形成结果的时间", index, intent.OccurredAt)
		}
	}
}

// Covers: ADR-0134 决定四——追加后续动作（作废）那一拍同样触发：它改变关闭路径「已有成功结果均已成功
// 作废」那一格的输入。范围只到指名包裹时仍按覆盖包裹逐件交——判断读全册，哪件被作废由判断自己看。
func TestAppendingAFollowUpActionHandsOffJudgmentIntentsOnItsOwnBeat(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.mustRecordResult(t, "LT-1", domain.LabelTransactionSucceeded, true)
	fixture.judgments.intents = nil

	result, err := fixture.handler.AppendFollowUpAction(context.Background(), application.AppendLabelFollowUpActionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		Kind:          domain.ChannelVoidAction,
		Parcels:       fixture.parcels[:1],
		Reason:        mustValue(t, domain.NewChannelResultReasonReference, "VOIDED"),
		OccurredAt:    handlerClockAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("append follow-up: %v", err)
	}
	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("outcome = %q, want APPLIED", result.Outcome())
	}

	intents := fixture.judgments.intents
	if len(intents) != len(fixture.parcels) {
		t.Fatalf("作废那一拍交出 %d 份意图，want 覆盖包裹每件一份（%d）", len(intents), len(fixture.parcels))
	}
	for _, intent := range intents {
		if intent.Beat != ports.LabelTransactionFollowUpAppended {
			t.Fatalf("拍 = %q, want FOLLOW_UP_APPENDED", intent.Beat)
		}
		if !intent.OccurredAt.Equal(handlerClockAt.Add(2 * time.Hour)) {
			t.Fatalf("业务时间 = %v, want 作废发生的时间", intent.OccurredAt)
		}
	}
}

// Covers: 意图只在 `Save` 成功之后交——版本冲突（别人先写了）与状态门拒绝都不入队，否则一封信会指着
// 一拍根本没落库的结果。
func TestNoJudgmentIntentIsHandedOffWhenTheBeatDoesNotLand(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")

	fixture.repository.saveOutcome = ports.LabelTransactionRevisionConflict
	result, err := fixture.handler.RecordChannelResult(context.Background(), fixture.recordResultCommand(t, "LT-1"))
	if err != nil {
		t.Fatalf("record result: %v", err)
	}
	if result.Outcome() != application.LabelTransactionWriteConflict {
		t.Fatalf("outcome = %q, want REVISION_CONFLICT", result.Outcome())
	}
	if len(fixture.judgments.intents) != 0 {
		t.Fatalf("版本冲突仍交出了 %d 份意图", len(fixture.judgments.intents))
	}

	fixture.repository.saveOutcome = ports.LabelTransactionSaved
	fixture.mustRecordResult(t, "LT-1", domain.LabelTransactionSucceeded, true)
	fixture.judgments.intents = nil
	result, err = fixture.handler.RecordChannelResult(context.Background(), fixture.recordResultCommand(t, "LT-1"))
	if err != nil {
		t.Fatalf("record result twice: %v", err)
	}
	if result.Outcome() != application.LabelTransactionStepNotAdmitted {
		t.Fatalf("outcome = %q, want STATE_NOT_ADMITTED", result.Outcome())
	}
	if len(fixture.judgments.intents) != 0 {
		t.Fatalf("状态门拒绝仍交出了 %d 份意图", len(fixture.judgments.intents))
	}
}

// Covers: ADR-0134 决定三——入队失败即本步 error 上抛（事务回滚，调用方重放），不交回 `APPLIED`：
// 不留「结果已落、判断意图丢了」的中间态，那正是取乙要消掉的东西。
func TestAFailedJudgmentHandoffFailsTheWholeBeat(t *testing.T) {
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.judgments.failWith = errors.New("outbox unavailable")

	result, err := fixture.handler.RecordChannelResult(context.Background(), fixture.recordResultCommand(t, "LT-1"))
	if !errors.Is(err, fixture.judgments.failWith) {
		t.Fatalf("err = %v, want 入队失败原样上抛", err)
	}
	if result.Outcome() != application.LabelTransactionOutcomeInvalid {
		t.Fatalf("入队失败仍交回了业务答案 %q", result.Outcome())
	}
}

// ---- 夹具 ----

type labelTransactionFixture struct {
	handler    *application.LabelTransactionHandler
	repository *labelTransactionRepositoryDouble
	judgments  *judgmentHandoffDouble
	tenant     domain.TenantID
	parcels    []domain.DeclaredParcelID
}

func newLabelTransactionFixture(t *testing.T) *labelTransactionFixture {
	t.Helper()
	repository := newLabelTransactionRepositoryDouble()
	judgments := &judgmentHandoffDouble{}
	return &labelTransactionFixture{
		handler: application.NewLabelTransactionHandler(application.LabelTransactionDeps{
			Transactions: repository,
			Judgments:    judgments,
			Clock:        fixedClock{at: handlerClockAt},
		}),
		repository: repository,
		judgments:  judgments,
		tenant:     mustValue(t, domain.NewTenantID, "tenant-1"),
		parcels: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "PARCEL-1"),
			mustValue(t, domain.NewDeclaredParcelID, "PARCEL-2"),
		},
	}
}

func (fixture *labelTransactionFixture) establishCommand(t *testing.T, id string) application.EstablishLabelTransactionCommand {
	t.Helper()
	return application.EstablishLabelTransactionCommand{
		Tenant:                 fixture.tenant,
		TransactionID:          mustValue(t, domain.NewLabelTransactionID, id),
		CoveredParcels:         fixture.parcels,
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "ACCT-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "HOLDER-1"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "PROVIDER-1"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "COUNTERPARTY-1"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "CONTRACT-1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "RATE-1"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "BASIS-1"),
	}
}

func (fixture *labelTransactionFixture) mustEstablish(t *testing.T, id string) domain.LabelTransaction {
	t.Helper()
	result, err := fixture.handler.Establish(context.Background(), fixture.establishCommand(t, id))
	if err != nil {
		t.Fatalf("夹具建立 %s：%v", id, err)
	}
	return fixture.mustApplied(t, result, "establish "+id)
}

func (fixture *labelTransactionFixture) mustSubmit(t *testing.T, id string) domain.LabelTransaction {
	t.Helper()
	result, err := fixture.handler.SubmitToChannel(context.Background(), application.SubmitLabelTransactionCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, id),
	})
	if err != nil {
		t.Fatalf("夹具提交 %s：%v", id, err)
	}
	return fixture.mustApplied(t, result, "submit "+id)
}

func (fixture *labelTransactionFixture) mustMarkUncertain(t *testing.T, id string) domain.LabelTransaction {
	t.Helper()
	result, err := fixture.handler.MarkResultUncertain(context.Background(), application.MarkLabelResultUncertainCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, id),
	})
	if err != nil {
		t.Fatalf("夹具标记不确定 %s：%v", id, err)
	}
	return fixture.mustApplied(t, result, "mark uncertain "+id)
}

// recordResultCommand 是一次「两件包裹全受理、交易级成功」的结果记录命令。
func (fixture *labelTransactionFixture) recordResultCommand(t *testing.T, id string) application.RecordLabelChannelResultCommand {
	t.Helper()
	return fixture.recordResultCommandOf(t, id, domain.LabelTransactionSucceeded, true)
}

func (fixture *labelTransactionFixture) recordResultCommandOf(
	t *testing.T,
	id string,
	outcome domain.LabelTransactionState,
	accepted bool,
) application.RecordLabelChannelResultCommand {
	t.Helper()
	specs := make([]domain.LabelTransactionParcelResultSpec, 0, len(fixture.parcels))
	for index, parcel := range fixture.parcels {
		spec := domain.LabelTransactionParcelResultSpec{Parcel: parcel, Accepted: accepted}
		if accepted {
			spec.Identifier = mustValue(t, domain.NewChannelParcelIdentifier, "CN-"+string(rune('1'+index)))
		} else {
			spec.Reason = mustValue(t, domain.NewChannelResultReasonReference, "CHANNEL_REJECTED")
		}
		specs = append(specs, spec)
	}
	return application.RecordLabelChannelResultCommand{
		Tenant:        fixture.tenant,
		TransactionID: mustValue(t, domain.NewLabelTransactionID, id),
		Outcome:       outcome,
		ParcelResults: specs,
		ObservedAt:    handlerClockAt.Add(time.Hour),
	}
}

// mustRecordResult 记一次两层结果。accepted 决定两件包裹是全受理还是全不受理，交易级取值
// 由调用方给——夹具不替用例推交易级结果，那正是 CONTEXT 禁止的层间互推。
func (fixture *labelTransactionFixture) mustRecordResult(
	t *testing.T,
	id string,
	outcome domain.LabelTransactionState,
	accepted bool,
) domain.LabelTransaction {
	t.Helper()
	result, err := fixture.handler.RecordChannelResult(context.Background(), fixture.recordResultCommandOf(t, id, outcome, accepted))
	if err != nil {
		t.Fatalf("夹具记结果 %s：%v", id, err)
	}
	return fixture.mustApplied(t, result, "record result "+id)
}

// judgmentHandoffDouble 记下编排交出的每一份判断意图；failWith 非空时模拟入队失败。
type judgmentHandoffDouble struct {
	intents  []ports.LabelTransactionJudgmentIntent
	failWith error
}

func (double *judgmentHandoffDouble) HandOffLabelTransactionJudgment(
	_ context.Context,
	intent ports.LabelTransactionJudgmentIntent,
) error {
	if double.failWith != nil {
		return double.failWith
	}
	double.intents = append(double.intents, intent)
	return nil
}

var _ ports.LabelTransactionJudgmentHandoff = (*judgmentHandoffDouble)(nil)

func (fixture *labelTransactionFixture) mustApplied(
	t *testing.T,
	result application.LabelTransactionResult,
	step string,
) domain.LabelTransaction {
	t.Helper()
	if result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("夹具 %s 的 outcome = %q, want APPLIED", step, result.Outcome())
	}
	transaction, present := result.Transaction()
	if !present {
		t.Fatalf("夹具 %s 没交回交易", step)
	}
	return transaction
}

// labelTransactionRepositoryDouble 是按（租户 + 标识）存的内存替身。它照 ADR-0031 的代数
// 作答，不译成 error——用例要能分辨`已存在`与`版本冲突`两格，替身把它们压成错误就测不出来。
type labelTransactionRepositoryDouble struct {
	stored      map[domain.LabelTransactionID]domain.LabelTransaction
	inserted    *domain.LabelTransaction
	saveOutcome ports.LabelTransactionSaveOutcome
}

func newLabelTransactionRepositoryDouble() *labelTransactionRepositoryDouble {
	return &labelTransactionRepositoryDouble{
		stored:      make(map[domain.LabelTransactionID]domain.LabelTransaction),
		saveOutcome: ports.LabelTransactionSaved,
	}
}

func (double *labelTransactionRepositoryDouble) FindByID(
	_ context.Context,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
) (domain.LabelTransaction, bool, error) {
	transaction, found := double.stored[transactionID]
	if !found || transaction.Tenant() != tenant {
		return domain.LabelTransaction{}, false, nil
	}
	return transaction, true, nil
}

func (double *labelTransactionRepositoryDouble) Insert(
	_ context.Context,
	transaction domain.LabelTransaction,
) (ports.LabelTransactionInsertOutcome, error) {
	if _, exists := double.stored[transaction.ID()]; exists {
		return ports.LabelTransactionAlreadyExists, nil
	}
	double.stored[transaction.ID()] = transaction
	stored := transaction
	double.inserted = &stored
	return ports.LabelTransactionInserted, nil
}

func (double *labelTransactionRepositoryDouble) Save(
	_ context.Context,
	transaction domain.LabelTransaction,
) (ports.LabelTransactionSaveOutcome, error) {
	if double.saveOutcome != ports.LabelTransactionSaved {
		return double.saveOutcome, nil
	}
	double.stored[transaction.ID()] = transaction
	return ports.LabelTransactionSaved, nil
}

var _ ports.LabelTransactionRepository = (*labelTransactionRepositoryDouble)(nil)
