package labelfinal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证关闭 / 重开决定那一路（lc/27）的处理方：信封的（租户 + 包裹）交给与 26 共用的核，不带收寄事实、
// 不读回登记册；关闭之后判断按 CONTEXT 分四格（票 27 判据 2），采用幂等的来源版本 = 生效关闭决定标识
// （判据 3）；五值翻译经同一只核、同一张表（判据 4）。判断编排是真的，四个读口与采用路径用替身。

// continuedAttemptDue 是一封关闭 / 重开决定判断意图的译码结果；决定标识只作追溯，处理方不读它。
func continuedAttemptDue(decision string) psinbox.ContinuedAttemptDecisionJudgmentDue {
	return psinbox.ContinuedAttemptDecisionJudgmentDue{TenantID: "tenant-1", Parcel: "parcel-1", Decision: decision}
}

func newContinuedAttemptAdapter(t *testing.T, f *fixture) *adapter.ContinuedAttemptDecisionJudgmentAdapter {
	t.Helper()
	handler, err := adapter.NewContinuedAttemptDecisionJudgmentAdapter(f.core)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return handler
}

// succeededTransaction 造一笔覆盖 parcel-1、交易级成功、包裹被受理的已定案交易；voided 为真时再追加一条
// 作用到该包裹的渠道作废。
func succeededTransaction(t *testing.T, voided bool) psdomain.LabelTransaction {
	t.Helper()
	parcel := value(t, psdomain.NewDeclaredParcelID, "parcel-1")
	established, err := psdomain.EstablishLabelTransaction(psdomain.EstablishLabelTransactionSpec{
		Tenant:                 value(t, psdomain.NewTenantID, "tenant-1"),
		ID:                     value(t, psdomain.NewLabelTransactionID, "LT-2"),
		CoveredParcels:         []psdomain.DeclaredParcelID{parcel},
		ChannelAccount:         value(t, psdomain.NewChannelAccountReference, "ACCT-1"),
		AccountHolder:          value(t, psdomain.NewChannelAccountHolderReference, "HOLDER-1"),
		ServiceProvider:        value(t, psdomain.NewChannelServiceProviderReference, "PROVIDER-1"),
		SettlementCounterparty: value(t, psdomain.NewSettlementCounterpartyReference, "COUNTERPARTY-1"),
		Contract:               value(t, psdomain.NewChannelContractReference, "CONTRACT-1"),
		Rate:                   value(t, psdomain.NewChannelRateReference, "RATE-1"),
		ResponsibilityBasis:    value(t, psdomain.NewResponsibilityBasisSnapshotReference, "BASIS-1"),
		EstablishedAt:          judgedAt.Add(-3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	submitted, err := established.SubmitToChannel(judgedAt.Add(-2 * time.Hour))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	succeeded, err := submitted.RecordChannelResult(psdomain.RecordChannelResultSpec{
		Outcome: psdomain.LabelTransactionSucceeded,
		ParcelResults: []psdomain.LabelTransactionParcelResultSpec{{
			Parcel:     parcel,
			Accepted:   true,
			Identifier: value(t, psdomain.NewChannelParcelIdentifier, "CHANNEL-PARCEL-1"),
		}},
		ObservedAt: judgedAt.Add(-90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("record result: %v", err)
	}
	if !voided {
		return succeeded
	}
	voidedTransaction, err := succeeded.AppendFollowUpAction(psdomain.FollowUpActionSpec{
		Kind:       psdomain.ChannelVoidAction,
		Parcels:    []psdomain.DeclaredParcelID{parcel},
		Reason:     value(t, psdomain.NewChannelResultReasonReference, "SHIPPER_STOP"),
		OccurredAt: judgedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	return voidedTransaction
}

// reopenedRegister 在 closedRegister 之上追加一条重开：关闭被清位，判断眼里继续尝试仍开放。
func reopenedRegister(t *testing.T) psdomain.ContinuedAttemptRegister {
	t.Helper()
	reopened, err := closedRegister(t).Append(psdomain.ContinuedAttemptDecisionSpec{
		ID:                  value(t, psdomain.NewContinuedAttemptDecisionID, "CAD-2"),
		Kind:                psdomain.ReopeningDecision,
		Decider:             value(t, psdomain.NewDeciderReference, "OPERATOR-1"),
		AuthorityRole:       value(t, psdomain.NewContinuedAttemptAuthorityRoleReference, "ROLE-1"),
		AuthoritySnapshot:   value(t, psdomain.NewContinuedAttemptAuthoritySnapshot, "GRANT-1"),
		Reason:              value(t, psdomain.NewContinuedAttemptReasonReference, "RESUME"),
		EffectiveAt:         judgedAt.Add(-20 * time.Minute),
		RelatedPriorClosure: value(t, psdomain.NewContinuedAttemptDecisionID, "CAD-1"),
	}, false)
	if err != nil {
		t.Fatalf("append reopening: %v", err)
	}
	return reopened
}

// Covers: 处理方只把（租户 + 包裹）交给核——不带收寄事实、不读回登记册（核没有登记册口，判断自己读）；
// 委托来源身份与委托标识取自反查结果。
func TestTheContinuedAttemptAdapterDelegatesToTheCoreWithoutAPickup(t *testing.T) {
	f := newFixture(t)
	handler := newContinuedAttemptAdapter(t, f)

	if err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-1")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if f.targets.tenant != "tenant-1" || f.targets.parcel != "parcel-1" {
		t.Fatalf("反查用的键 = %s/%s", f.targets.tenant, f.targets.parcel)
	}
	if len(f.judge.commands) != 1 {
		t.Fatalf("判断次数 = %d, want 1", len(f.judge.commands))
	}
	command := f.judge.commands[0]
	if command.Identity != identity(t) || command.ShipmentRequestID.String() != "request-1" ||
		command.Parcel.String() != "parcel-1" {
		t.Fatalf("命令折错了：%#v", command)
	}
	if command.FirstEffectivePickup != (psdomain.CarrierFirstEffectivePickupSpec{}) {
		t.Fatal("关闭 / 重开那一路的命令带了收寄事实——它该走关闭路径")
	}
}

// Covers: 判据 2 四格——关闭之后，全部交易明确失败 → LABEL_SERVICE_FAILURE；成功结果全部作废 →
// LABEL_SERVICE_OUTCOME；仍有可用成功结果 → NOT_FINAL；重开之后 → NOT_FINAL（ContinuedAttemptStillOpen）。
// 分格由 JudgeLabelServiceFinal 答，处理方不看信封里的决定种类。形成终局的两格交采用路径，来源版本与执行
// 证据都指生效关闭决定的标识（判据 3 的幂等键）。
func TestAClosureDecisionIsJudgedIntoTheFourContextGrids(t *testing.T) {
	type outcomeCase struct {
		name         string
		transactions []psdomain.LabelTransaction
		register     psdomain.ContinuedAttemptRegister
		formsFinal   bool
		kind         psdomain.ResponsibilityOutcomeKind
		reason       psdomain.LabelServiceNotFinalReason
	}
	cases := []outcomeCase{
		{
			name:         "关闭 + 全部交易明确失败 → 终局失败结果",
			transactions: []psdomain.LabelTransaction{failedTransaction(t)},
			register:     closedRegister(t),
			formsFinal:   true,
			kind:         psdomain.LabelServiceFailure,
		},
		{
			name:         "关闭 + 成功结果全部作废 → 终局服务结果",
			transactions: []psdomain.LabelTransaction{succeededTransaction(t, true)},
			register:     closedRegister(t),
			formsFinal:   true,
			kind:         psdomain.LabelServiceOutcome,
		},
		{
			name:         "关闭 + 仍有可用成功结果 → NOT_FINAL",
			transactions: []psdomain.LabelTransaction{succeededTransaction(t, false)},
			register:     closedRegister(t),
			reason:       psdomain.UsableLabelResultOutstanding,
		},
		{
			name:         "重开 → NOT_FINAL（继续尝试仍开放）",
			transactions: []psdomain.LabelTransaction{failedTransaction(t)},
			register:     reopenedRegister(t),
			reason:       psdomain.ContinuedAttemptStillOpen,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.transactions.transactions = tc.transactions
			f.registers.register, f.registers.found = tc.register, true
			handler := newContinuedAttemptAdapter(t, f)

			err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-1"))

			if !tc.formsFinal {
				if err != nil {
					t.Fatalf("NOT_FINAL 应入账，实得：%v", err)
				}
				if len(f.adopter.commands) != 0 {
					t.Fatal("不形成终局却交了采用路径")
				}
				if got := f.judge.results[0].Verdict().Reason(); got != tc.reason {
					t.Fatalf("NOT_FINAL 原因 = %v, want %v", got, tc.reason)
				}
				return
			}
			// 替身采用路径交回集合外的零值结果，finalconsume 响亮报错——这正证明结果走到了它手上。
			if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
				t.Fatalf("采用结果应经 finalconsume 收口，实得：%v", err)
			}
			if len(f.adopter.commands) != 1 {
				t.Fatalf("采用路径调用次数 = %d, want 1", len(f.adopter.commands))
			}
			outcome := f.adopter.commands[0].Outcome
			if outcome.Kind != tc.kind {
				t.Fatalf("责任结果种类 = %v, want %v", outcome.Kind, tc.kind)
			}
			if outcome.Version.String() != "CAD-1" || outcome.Execution.String() != "CONTINUED-ATTEMPT-CLOSURE/CAD-1" {
				t.Fatalf("来源版本 / 执行证据 = %q / %q，应指生效关闭决定 CAD-1", outcome.Version, outcome.Execution)
			}
		})
	}
}

// Covers: 判据 3——同一份关闭的信封重放，两次折出同一个来源版本（= 关闭决定标识），采用路径据此返原
// （返原本身归 FormParcelFinalHandler 既有用例，这里证交给它的键没变）；关过—重开—再关是新决定标识，
// 来源版本随之换成新关闭的，走重派生。
func TestReplayingOneClosureHandsTheSameSourceVersionAndANewClosureHandsANewOne(t *testing.T) {
	f := newFixture(t)
	f.transactions.transactions = []psdomain.LabelTransaction{failedTransaction(t)}
	f.registers.register, f.registers.found = closedRegister(t), true
	handler := newContinuedAttemptAdapter(t, f)

	for range 2 {
		err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-1"))
		if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
			t.Fatalf("重放也该走到采用路径：%v", err)
		}
	}
	if len(f.adopter.commands) != 2 {
		t.Fatalf("采用路径调用次数 = %d, want 2", len(f.adopter.commands))
	}
	if f.adopter.commands[0].Outcome.Version != f.adopter.commands[1].Outcome.Version ||
		f.adopter.commands[0].Outcome.Version.String() != "CAD-1" {
		t.Fatalf("重放的来源版本 = %q / %q，应同为 CAD-1", f.adopter.commands[0].Outcome.Version, f.adopter.commands[1].Outcome.Version)
	}

	// 关过—重开—再关：册上第三条 CAD-3 成为生效关闭，来源版本换成它。
	reclosed, err := reopenedRegister(t).Append(psdomain.ContinuedAttemptDecisionSpec{
		ID:                          value(t, psdomain.NewContinuedAttemptDecisionID, "CAD-3"),
		Kind:                        psdomain.ControlledClosureDecision,
		Decider:                     value(t, psdomain.NewDeciderReference, "OPERATOR-1"),
		AuthorityRole:               value(t, psdomain.NewContinuedAttemptAuthorityRoleReference, "ROLE-1"),
		AuthoritySnapshot:           value(t, psdomain.NewContinuedAttemptAuthoritySnapshot, "GRANT-1"),
		Reason:                      value(t, psdomain.NewContinuedAttemptReasonReference, "NO_MORE_ATTEMPTS"),
		EffectiveAt:                 judgedAt.Add(-10 * time.Minute),
		CutoffBoundary:              value(t, psdomain.NewAuthoritativeCutoffBoundary, "CAD-3"),
		ClosureResponsibilitySource: value(t, psdomain.NewClosureResponsibilitySourceReference, "OPERATOR-ACTION/SYN-OPS-1"),
	}, false)
	if err != nil {
		t.Fatalf("append second closure: %v", err)
	}
	f.registers.register = reclosed
	if err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-3")); !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("再关也该走到采用路径：%v", err)
	}
	if got := f.adopter.commands[2].Outcome.Version.String(); got != "CAD-3" {
		t.Fatalf("再关的来源版本 = %q, want CAD-3", got)
	}
}

// Covers: 判据 4——五值翻译经同一只核：读口答不出 → 同一个未决哨兵；反查不中 → 同一个不猜委托哨兵。
// 这一路没有自己的翻译表可以走偏。
func TestTheContinuedAttemptAdapterSharesTheCoreTranslation(t *testing.T) {
	f := newFixture(t)
	f.transactions.err = errors.New("label transactions unavailable")
	handler := newContinuedAttemptAdapter(t, f)
	if err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-1")); !errors.Is(err, adapter.ErrJudgmentUndecided) {
		t.Fatalf("err = %v, want ErrJudgmentUndecided", err)
	}

	f = newFixture(t)
	f.targets.found = false
	handler = newContinuedAttemptAdapter(t, f)
	if err := handler.HandleContinuedAttemptDecisionJudgmentDue(context.Background(), continuedAttemptDue("CAD-1")); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
}

func TestTheContinuedAttemptAdapterRefusesANilCore(t *testing.T) {
	if _, err := adapter.NewContinuedAttemptDecisionJudgmentAdapter(nil); err == nil {
		t.Fatal("nil 核被接受了")
	}
}
