package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var (
	labelServiceResultAt  = labelTransactionAt.Add(2 * time.Hour)
	labelServiceClosureAt = labelTransactionAt.Add(24 * time.Hour)
	labelServicePickupAt  = labelTransactionAt.Add(6 * time.Hour)
)

// resultedLabelTransaction 造一笔已提交并已记下渠道结果的交易：`结果`是交易级三格之一，
// 逐包裹结果按覆盖范围一一给出。
func resultedLabelTransaction(
	t *testing.T,
	id string,
	outcome domain.LabelTransactionState,
	results ...domain.LabelTransactionParcelResultSpec,
) domain.LabelTransaction {
	t.Helper()
	parcels := make([]string, 0, len(results))
	for _, result := range results {
		parcels = append(parcels, result.Parcel.String())
	}
	spec := labelTransactionSpec(t, parcels...)
	spec.ID = mustValue(t, domain.NewLabelTransactionID, id)
	transaction, err := domain.EstablishLabelTransaction(spec)
	if err != nil {
		t.Fatalf("establish %s: %v", id, err)
	}
	transaction, err = transaction.SubmitToChannel(labelTransactionAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("submit %s: %v", id, err)
	}
	transaction, err = transaction.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome:       outcome,
		ParcelResults: results,
		ObservedAt:    labelServiceResultAt,
	})
	if err != nil {
		t.Fatalf("record result on %s: %v", id, err)
	}
	return transaction
}

func voided(t *testing.T, transaction domain.LabelTransaction, parcels ...string) domain.LabelTransaction {
	t.Helper()
	scope := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		scope = append(scope, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}
	transaction, err := transaction.AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelVoidAction,
		Parcels:    scope,
		Reason:     mustValue(t, domain.NewChannelResultReasonReference, "VOIDED_BY_OPERATOR"),
		OccurredAt: labelServiceResultAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	return transaction
}

func closedRegister(t *testing.T) domain.ContinuedAttemptRegister {
	t.Helper()
	register, err := emptyRegister(t).Append(closureSpec(t, "closure-1", labelServiceClosureAt), false)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	return register
}

func firstPickup(t *testing.T) domain.CarrierFirstEffectivePickup {
	t.Helper()
	pickup, err := domain.ReferenceCarrierFirstEffectivePickup(domain.CarrierFirstEffectivePickupSpec{
		Fact:        mustValue(t, domain.NewCarrierFirstEffectivePickupFactReference, "TF-TRACK-FACT-7"),
		Version:     mustValue(t, domain.NewCarrierFirstEffectivePickupFactVersion, "TF-TRACK-FACT-7/v1"),
		EffectiveAt: labelServicePickupAt,
	})
	if err != nil {
		t.Fatalf("reference pickup: %v", err)
	}
	return pickup
}

func labelServiceInput(t *testing.T, register domain.ContinuedAttemptRegister, transactions ...domain.LabelTransaction) domain.LabelServiceFinalInput {
	t.Helper()
	return domain.LabelServiceFinalInput{
		Parcel:       mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Transactions: transactions,
		Register:     register,
	}
}

func judge(t *testing.T, input domain.LabelServiceFinalInput) domain.LabelServiceFinalVerdict {
	t.Helper()
	verdict, err := domain.JudgeLabelServiceFinal(input)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	return verdict
}

// Covers: CONTEXT 规则「除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局」与
// 生命周期「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消
// 终局结果：该服务不统一等待实际交付」——收寄在场时不看交易停在哪一格、不看有没有关闭；
// 有效取消在先时不形成非取消终局（「面单渠道服务发生实际承运商收寄……后，包裹不得回退为
// 已取消」的对偶）。
func TestFirstEffectiveCarrierPickupFormsTheLabelServiceFinal(t *testing.T) {
	uncertain, err := submittedLabelTransaction(t, "parcel-1").MarkResultUncertain()
	if err != nil {
		t.Fatalf("mark uncertain: %v", err)
	}
	input := labelServiceInput(t, emptyRegister(t), uncertain)
	input.FirstEffectivePickup = firstPickup(t)

	verdict := judge(t, input)
	if verdict.Judgment() != domain.LabelServiceFinalByFirstPickup {
		t.Fatalf("judgment = %s, want FINAL_BY_FIRST_PICKUP（结果不确定的交易不拦收寄终局）", verdict.Judgment())
	}
	pickup, present := verdict.FirstEffectivePickup()
	if !present || pickup.Fact().String() != "TF-TRACK-FACT-7" || pickup.Version().String() != "TF-TRACK-FACT-7/v1" {
		t.Fatalf("pickup = %+v/%v；终局只引用 TF 事实，不复制", pickup, present)
	}
	if !verdict.EffectiveAt().Equal(labelServicePickupAt) {
		t.Fatalf("effective at = %s，want 收寄的有效时间", verdict.EffectiveAt())
	}
	kind, forms := verdict.ResponsibilityOutcomeKind()
	if !forms || kind != domain.LabelServiceOutcome {
		t.Fatalf("kind = %s/%v，want LABEL_SERVICE_OUTCOME", kind, forms)
	}

	t.Run("a standing cancellation is not overridden by a later pickup", func(t *testing.T) {
		cancelled := input
		cancelled.Cancellation = standingCancellationAt(t, labelServicePickupAt.Add(-time.Minute))
		verdict := judge(t, cancelled)
		if verdict.Judgment() != domain.LabelServiceCancellationStands {
			t.Fatalf("judgment = %s，want CANCELLATION_STANDS", verdict.Judgment())
		}
		if _, forms := verdict.ResponsibilityOutcomeKind(); forms {
			t.Fatal("取消在先仍造出了非取消终局来源")
		}
	})
}

func standingCancellationAt(t *testing.T, requestedAt time.Time) domain.ParcelCancellation {
	t.Helper()
	spec := cancellationSpec(t)
	spec.RequestedAt = requestedAt
	cancellation, err := domain.DecideParcelCancellation(spec, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide cancellation: %v", err)
	}
	return cancellation
}

// Covers: `AT-PS-095` 与 `AT-PS-108`（ADR-0254 决定二第 2、3 格的面单渠道一侧）——有效取消已在，
// 此后到的首次有效收寄按两者的业务时间比：晚于取消，取消在先；不晚于取消（含相等），事实冲突，
// 不形成非取消终局、也不按取消在先作答，判断指名冲突并交回哪份取消对哪一代收寄。不带收寄的两路
// 没有边界事实可比，照旧取消在先。
func TestAStandingCancellationIsComparedWithTheFirstPickupByBusinessTime(t *testing.T) {
	input := labelServiceInput(t, emptyRegister(t))
	input.FirstEffectivePickup = firstPickup(t)

	cases := []struct {
		name        string
		cancelledAt time.Time
		want        domain.LabelServiceFinalJudgment
		reason      domain.LabelServiceNotFinalReason
	}{
		{"the pickup is later than the cancellation", labelServicePickupAt.Add(-time.Second),
			domain.LabelServiceCancellationStands, domain.LabelServiceNotFinalReasonNone},
		{"the pickup is earlier than the cancellation", labelServicePickupAt.Add(time.Hour),
			domain.LabelServiceNotFinal, domain.LabelServiceCancellationOrderConflict},
		{"the pickup is at the same instant as the cancellation", labelServicePickupAt,
			domain.LabelServiceNotFinal, domain.LabelServiceCancellationOrderConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			judged := input
			judged.Cancellation = standingCancellationAt(t, tc.cancelledAt)
			verdict := judge(t, judged)
			if verdict.Judgment() != tc.want || verdict.Reason() != tc.reason {
				t.Fatalf("verdict = %s/%s，want %s/%s", verdict.Judgment(), verdict.Reason(), tc.want, tc.reason)
			}
			if _, forms := verdict.ResponsibilityOutcomeKind(); forms {
				t.Fatal("有效取消在场时造出了非取消终局来源")
			}
			conflict, conflicted := verdict.Conflict()
			if conflicted != (tc.reason == domain.LabelServiceCancellationOrderConflict) {
				t.Fatalf("conflicted = %v，与原因 %s 不符", conflicted, verdict.Reason())
			}
			if !conflicted {
				return
			}
			if conflict.Cancellation().String() != "cancel-1" || !conflict.CancelledAt().Equal(tc.cancelledAt) {
				t.Fatalf("conflict cancellation = %s@%s", conflict.Cancellation(), conflict.CancelledAt())
			}
			boundary := conflict.Boundary()
			if boundary.Kind != domain.CarrierFirstEffectivePickupBoundary ||
				boundary.Reference.String() != "TF-TRACK-FACT-7@TF-TRACK-FACT-7/v1" ||
				!boundary.OccurredAt.Equal(labelServicePickupAt) {
				t.Fatalf("conflict boundary = %+v；要指名哪一代收寄与它的业务发生时间", boundary)
			}
		})
	}

	t.Run("the closure paths carry no pickup and keep the cancellation", func(t *testing.T) {
		closurePath := labelServiceInput(t, emptyRegister(t))
		closurePath.Cancellation = standingCancellationAt(t, labelServicePickupAt.Add(time.Hour))
		verdict := judge(t, closurePath)
		if verdict.Judgment() != domain.LabelServiceCancellationStands {
			t.Fatalf("judgment = %s，want CANCELLATION_STANDS", verdict.Judgment())
		}
	})

	t.Run("another parcel's cancellation is a wiring defect", func(t *testing.T) {
		foreign := input
		spec := cancellationSpec(t)
		spec.Parcel = mustValue(t, domain.NewDeclaredParcelID, "parcel-2")
		cancellation, err := domain.DecideParcelCancellation(spec, domain.CurrentIntakeFact{})
		if err != nil {
			t.Fatalf("decide cancellation: %v", err)
		}
		foreign.Cancellation = cancellation
		if _, err := domain.JudgeLabelServiceFinal(foreign); !errors.Is(err, domain.ErrInvalidLabelServiceFinalInput) {
			t.Fatalf("err = %v，want ErrInvalidLabelServiceFinalInput", err)
		}
	})
}

// Covers: CONTEXT 生命周期「当前受控关闭已经生效且未被重开，全部相关面单交易均已定案为明确
// 失败，并且不存在有效或结果待确认的面单结果 → 终局失败结果」，以及规则「交易级失败不能推导
// 包裹失败……受控关闭只将包裹级继续尝试判断派生为不允许新增尝试……不直接形成包裹终局」——
// 缺关闭、缺定案、缺「全部失败」三者任一不成立都不是终局。
func TestTheClosurePathFormsAFailureOnlyWhenEverythingIsFinalizedFailed(t *testing.T) {
	failed := resultedLabelTransaction(t, "label-txn-1", domain.LabelTransactionFailed,
		refusedParcelResult(t, "parcel-1", "ADDRESS_REJECTED"))

	t.Run("a failed transaction without a closure is not a parcel failure", func(t *testing.T) {
		verdict := judge(t, labelServiceInput(t, emptyRegister(t), failed))
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.ContinuedAttemptStillOpen {
			t.Fatalf("judgment = %s/%s；交易级失败不能推导包裹失败", verdict.Judgment(), verdict.Reason())
		}
	})

	t.Run("an unfinalized transaction blocks the closure path", func(t *testing.T) {
		uncertain, err := submittedLabelTransaction(t, "parcel-1").MarkResultUncertain()
		if err != nil {
			t.Fatalf("mark uncertain: %v", err)
		}
		verdict := judge(t, labelServiceInput(t, closedRegister(t), failed, uncertain))
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.LabelTransactionNotFinalized {
			t.Fatalf("judgment = %s/%s；结果不确定的交易不得按失败处理", verdict.Judgment(), verdict.Reason())
		}
	})

	t.Run("closure plus all-failed transactions forms the failure final", func(t *testing.T) {
		second := resultedLabelTransaction(t, "label-txn-2", domain.LabelTransactionFailed,
			refusedParcelResult(t, "parcel-1", "CHANNEL_DECLINED"))
		verdict := judge(t, labelServiceInput(t, closedRegister(t), failed, second))
		if verdict.Judgment() != domain.LabelServiceFinalFailureByClosure {
			t.Fatalf("judgment = %s，want FINAL_FAILURE_BY_CLOSURE", verdict.Judgment())
		}
		closure, present := verdict.Closure()
		if !present || closure.ID().String() != "closure-1" {
			t.Fatalf("closure = %+v/%v；证据是生效的那份关闭决定", closure, present)
		}
		// 生效时间取关闭生效与最后一份结果之中较晚者：终局在最后一个条件成立那一刻才成立。
		if !verdict.EffectiveAt().Equal(labelServiceClosureAt) {
			t.Fatalf("effective at = %s，want %s", verdict.EffectiveAt(), labelServiceClosureAt)
		}
		kind, forms := verdict.ResponsibilityOutcomeKind()
		if !forms || kind != domain.LabelServiceFailure {
			t.Fatalf("kind = %s/%v，want LABEL_SERVICE_FAILURE", kind, forms)
		}
	})

	t.Run("a multi-parcel transaction is judged on this parcel's own result", func(t *testing.T) {
		partial := resultedLabelTransaction(t, "label-txn-3", domain.LabelTransactionPartiallySucceeded,
			refusedParcelResult(t, "parcel-1", "ADDRESS_REJECTED"),
			acceptedParcelResult(t, "parcel-2", "CHN-2"))
		verdict := judge(t, labelServiceInput(t, closedRegister(t), partial))
		if verdict.Judgment() != domain.LabelServiceFinalFailureByClosure {
			t.Fatalf("judgment = %s；另一件包裹的受理不算到本件头上（面单交易包裹结果不能由其他包裹结果推断）", verdict.Judgment())
		}
	})

	t.Run("a closure with no transaction at all is a failure final", func(t *testing.T) {
		verdict := judge(t, labelServiceInput(t, closedRegister(t)))
		if verdict.Judgment() != domain.LabelServiceFinalFailureByClosure {
			t.Fatalf("judgment = %s；关闭之下从未取得任何面单结果，服务没有发生", verdict.Judgment())
		}
	})
}

// Covers: CONTEXT 生命周期「当前受控关闭已经生效且未被重开，全部相关面单交易均已定案，不存在
// 仍可使用或结果待确认的面单结果，并且已有成功结果均已成功作废或依据接受时固定的规则不可逆
// 失效 → 终局服务结果」，以及「成功且仍有效的面单交易……会继续阻止受控关闭路径形成终局」
// 与「受控关闭 → 开放」的重开半句。
func TestTheClosurePathFormsTheOutcomeOnlyAfterSuccessesAreVoidedOrLapsed(t *testing.T) {
	accepted := resultedLabelTransaction(t, "label-txn-1", domain.LabelTransactionSucceeded,
		acceptedParcelResult(t, "parcel-1", "CHN-1"))

	t.Run("a usable label result keeps the parcel out of any final", func(t *testing.T) {
		verdict := judge(t, labelServiceInput(t, closedRegister(t), accepted))
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.UsableLabelResultOutstanding {
			t.Fatalf("judgment = %s/%s；成功且仍有效的面单继续阻止关闭路径", verdict.Judgment(), verdict.Reason())
		}
	})

	t.Run("a voided success lets the closure path form the outcome", func(t *testing.T) {
		verdict := judge(t, labelServiceInput(t, closedRegister(t), voided(t, accepted)))
		if verdict.Judgment() != domain.LabelServiceFinalOutcomeByClosure {
			t.Fatalf("judgment = %s，want FINAL_OUTCOME_BY_CLOSURE", verdict.Judgment())
		}
		kind, forms := verdict.ResponsibilityOutcomeKind()
		if !forms || kind != domain.LabelServiceOutcome {
			t.Fatalf("kind = %s/%v，want LABEL_SERVICE_OUTCOME（成功作废不是失败）", kind, forms)
		}
	})

	t.Run("a void scoped to another parcel does not void this one", func(t *testing.T) {
		two := resultedLabelTransaction(t, "label-txn-2", domain.LabelTransactionSucceeded,
			acceptedParcelResult(t, "parcel-1", "CHN-1"), acceptedParcelResult(t, "parcel-2", "CHN-2"))
		verdict := judge(t, labelServiceInput(t, closedRegister(t), voided(t, two, "parcel-2")))
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.UsableLabelResultOutstanding {
			t.Fatalf("judgment = %s/%s；作废范围是明确交易范围或包裹范围，不外溢", verdict.Judgment(), verdict.Reason())
		}
	})

	t.Run("an irreversibly lapsed success counts as no longer usable", func(t *testing.T) {
		input := labelServiceInput(t, closedRegister(t), accepted)
		input.LapsedTransactions = []domain.LabelTransactionID{accepted.ID()}
		verdict := judge(t, input)
		if verdict.Judgment() != domain.LabelServiceFinalOutcomeByClosure {
			t.Fatalf("judgment = %s；按接受时固定规则不可逆失效的成功不再阻止终局", verdict.Judgment())
		}
	})

	t.Run("a reopened closure no longer closes the parcel", func(t *testing.T) {
		reopened, err := closedRegister(t).Append(
			reopeningSpec(t, "reopen-1", "closure-1", labelServiceClosureAt.Add(time.Hour)), false)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		verdict := judge(t, labelServiceInput(t, reopened, voided(t, accepted)))
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.ContinuedAttemptStillOpen {
			t.Fatalf("judgment = %s/%s；已重开的关闭不再生效", verdict.Judgment(), verdict.Reason())
		}
	})
}

// 输入自证：不覆盖本包裹的交易、册与包裹不一致，都是编排把别人的东西送进来了——拒，不猜。
func TestLabelServiceFinalRefusesInputsAboutAnotherParcel(t *testing.T) {
	foreign := resultedLabelTransaction(t, "label-txn-9", domain.LabelTransactionFailed,
		refusedParcelResult(t, "parcel-9", "ADDRESS_REJECTED"))
	if _, err := domain.JudgeLabelServiceFinal(labelServiceInput(t, closedRegister(t), foreign)); !errors.Is(err, domain.ErrInvalidLabelServiceFinalInput) {
		t.Fatalf("error = %v，want ErrInvalidLabelServiceFinalInput", err)
	}

	otherRegister, err := domain.OpenContinuedAttemptRegister(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewDeclaredParcelID, "parcel-9"))
	if err != nil {
		t.Fatalf("open register: %v", err)
	}
	if _, err := domain.JudgeLabelServiceFinal(labelServiceInput(t, otherRegister)); !errors.Is(err, domain.ErrInvalidLabelServiceFinalInput) {
		t.Fatalf("error = %v，want ErrInvalidLabelServiceFinalInput", err)
	}
}

// Covers: 同一笔交易上两次作废都作用到本包裹时，关闭路径的终局按较晚那次作废生效——终局要的是「全部条件都成立」
// 的时刻，较早那次作废之后条件仍在等第二次之前的状态，取最早会把终局提前到证据还不完整的时刻。
func TestTheClosureOutcomeTakesEffectAtTheLatestVoid(t *testing.T) {
	accepted := resultedLabelTransaction(t, "label-txn-1", domain.LabelTransactionSucceeded,
		acceptedParcelResult(t, "parcel-1", "CHN-1"))
	again, err := voided(t, accepted).AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelVoidAction,
		Reason:     mustValue(t, domain.NewChannelResultReasonReference, "VOIDED_AGAIN"),
		OccurredAt: labelServiceClosureAt.Add(6 * time.Hour),
	})
	if err != nil {
		t.Fatalf("second void: %v", err)
	}
	verdict := judge(t, labelServiceInput(t, closedRegister(t), again))
	if verdict.Judgment() != domain.LabelServiceFinalOutcomeByClosure {
		t.Fatalf("judgment = %s，want FINAL_OUTCOME_BY_CLOSURE", verdict.Judgment())
	}
	if !verdict.EffectiveAt().Equal(labelServiceClosureAt.Add(6 * time.Hour)) {
		t.Fatalf("effective at = %s，want 较晚那次作废 %s", verdict.EffectiveAt(), labelServiceClosureAt.Add(6*time.Hour))
	}
}
