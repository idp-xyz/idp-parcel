package main

import (
	"context"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证票 routing-first-cut/19 拆格那一笔的闭环：接受前财务控制答端口的`未配置`格
// （提供方原因 SA-CONTROL_POLICY_NOT_CONFIGURED），链在控制那一步落`等待运营登记`，提交门入账而不回滚重投，
// 处理尝试带新未决原因与按原因派生的续办引用留在库里（ADR-0095 决定一）；party-commercial 侧受理前财务
// 控制策略落库时经真交接适配器铸出种类为 PRE_ACCEPTANCE_CONTROL_POLICY 的「参数已登记」信封，登记续办门按
// 租户重驱，同一条链越过这一格直到接受。
//
// 与时点那条闭环同一份装配（newOperatorRegistrationLoopFixtureWith），只把控制那一口换成可拨的替身；时点
// 照声明形成。哪些 SA 原因落`未配置`格由 PS 适配器判，那一半的证据在 adapters/settlementaccounting 的用例。

// Covers: 票 routing-first-cut/19 完成判据二（控制策略未配置：发布受理前财务控制策略之后重驱越过这一格）、
// ADR-0094 决定三至五在控制那一步的落法。
func TestAControlPolicyWaitIsResumedByTheControlPolicyRegistrationEnvelope(t *testing.T) {
	control := &synRSwitchableControlPolicy{t: t}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()
	tenant := fixture.identity.TenantID()

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：可达性那条腿形成，控制那一步答`未配置`——等待态先落库，消费门入账。回滚重投推不动一次
	// 登记，只会烧完预算让信封落 ABANDONED，任务上一行不留。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("控制策略未配置必须按处理完毕入账：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——等待态没有随本份投递入账", n)
	}
	if control.asked == 0 {
		t.Fatal("链没有走到控制那一步——停顿不是本用例要的那一格")
	}
	waiting := fixture.mustLoadRequest(t, ctx)
	if waiting.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("等待期状态 = %q, want SUBMITTED", waiting.State())
	}
	if waitingOn, present := waiting.AcceptanceDecisionTask().WaitingOn(); !present || waitingOn != psdomain.ResumeByOperatorRegistration {
		t.Fatalf("等待态 = %v（present=%v）, want OPERATOR_REGISTRATION", waitingOn, present)
	}
	if queue := fixture.listRegistrationQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待运营登记队列 = %+v, want 恰好这一份委托", queue)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND continuation_ref LIKE 'CONT-%'`); n != 1 {
		t.Fatalf("带新原因与续办引用的处理尝试 = %d 行, want 1——停在哪一格没有留在任务上", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_financial_control`); n != 0 {
		t.Fatalf("控制没形成却记了 %d 份控制结果", n)
	}
	fixture.markPublished(t, submittedDelivery)

	// 登记：受理前财务控制策略落库，同事务经真交接适配器铸「参数已登记」信封。替身从此答`已冻结`。
	control.configured = true
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.registration.HandOffOperatorRegistrationCompleted(txCtx, pcports.OperatorRegistrationCompletedIntent{
			Kind:         pcports.PreAcceptanceControlPolicyRegistered,
			Registration: synCommercialVersion(t, tenant.String(), pcdomain.PreAcceptanceFinancialControlPolicyObject, "SYN-FCP-R1", "v1"),
			RegisteredAt: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC),
		})
	})
	if n := fixture.countOutboxOfType(t, string(psinbox.OperatorRegistrationCompletedEventType)); n != 1 {
		t.Fatalf("「参数已登记」信封 = %d 封, want 恰好 1", n)
	}

	// 登记续办门：按租户取队列，把同一条链再驱一拍——控制形成并记下，形成决定成立接受，队列出列。
	registrationDelivery := fixture.claimDeliveryOfType(t, string(psinbox.OperatorRegistrationCompletedEventType))
	if err := fixture.registrationGate.Consume(ctx, registrationDelivery.Envelope); err != nil {
		t.Fatalf("登记续办一拍：%v", err)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if remaining := fixture.listRegistrationQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后队列仍列出 %d 行", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}
}

// synRSwitchableControlPolicy 是 settlement-accounting 权威替身（隔离合成 `S`，不进生产装配）：configured 为假
// 时答端口的`未配置`格并带提供方原因 SA-CONTROL_POLICY_NOT_CONFIGURED——生产里 PS 适配器把同名的 SA 原因
// 译到这一格；拨真后照 synRFinancialAuthority 答`已冻结`。
type synRSwitchableControlPolicy struct {
	t          *testing.T
	configured bool
	asked      int
}

func (double *synRSwitchableControlPolicy) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	double.t.Helper()
	double.asked++
	if !double.configured {
		return psports.PreAcceptanceControlAssessment{
			Outcome: psports.PreAcceptanceControlNotConfigured,
			Reason:  mustPS(double.t, psdomain.NewCheckReason, "SA-CONTROL_POLICY_NOT_CONFIGURED"),
		}, nil
	}
	return synRFinancialAuthority{t: double.t}.ApplyPreAcceptanceFinancialControl(ctx, request)
}

var _ psports.PreAcceptanceFinancialController = (*synRSwitchableControlPolicy)(nil)
