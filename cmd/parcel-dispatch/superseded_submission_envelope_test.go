package main

import (
	"context"
	"testing"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证票 product-strategy-boundary/27 落在消费门的那一格：首版的「委托已提交」信封在
// 客户受控补充之后才投到（发布侧滞后，或首投回滚之后的重投），接受链答`版本已换代`，提交门把这份投递按处理完毕
// 入账——不回滚、不烧失败预算，首版一步都不判。夹具借受控补充那条闭环（customer_supplement_resume_loop_test.go）：
// 仓储、判断账、Outbox/Inbox、消费门、受控补充编排与接受编排全是生产实现，只有权威口换成替身。
//
// 链不走由编排层用例证（application 包的 TestASupersededVersionIsNotJudgedAgain）；这里证的是那个结果落在消费门
// 哪一格。`AdvanceAcceptanceChainResult` 的取值只有编排自己造得出，消费门这半只能接真编排证
// （shipment_request_submitted_consumer_test.go 文件头）。

// Covers: 票 product-strategy-boundary/27、ADR-0179 决定二与决定四「委托当前提交版本」、ADR-0045「新版本成为唯一
// 待判断版本」、ADR-0106 Decision 三 — 首版信封迟到：提交门入账，可达性权威不问，判断账、控制账与接受信封都不多
// 一行；新版本照旧由「新提交版本已形成」那一封驱到接受。
func TestALateSubmittedEnvelopeOfASupersededVersionSettlesWithoutJudging(t *testing.T) {
	fixture := newCustomerSupplementLoopFixture(t)
	ctx := t.Context()

	// 提交成立并按生产发布路径铸首版信封；信封还没投到，客户已以首版为基准补充，委托在`已提交`时换了代。
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	supplemented := fixture.supplementOn(t, ctx, request.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）",
			supplemented.Outcome(), supplemented.PendingReason())
	}

	// 两封信同一分区，Outbox 只交出队头：首版那一封先到，新版本那一封要等它定稿才认领得到。所以首版这一封回滚
	// 重投的代价不只是它自己的失败预算——它堵在队头，新版本的驱动随它一起等。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	submitted := submittedDelivery.Envelope
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("已换代版本的信封必须按处理完毕入账——重投换不回版本，只会烧完失败预算：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submitted.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——这份投递没有入账", n)
	}
	fixture.assertSupersededVersionUnjudged(t, ctx)
	fixture.markPublished(t, submittedDelivery)

	// 新版本那一封照旧驱链：可达性只为新版本问一次，接受成立。
	resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("新提交版本续办一拍：%v", err)
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("可达性权威被问了 %d 次, want 1——只该为新版本问", fixture.reachability.assessments)
	}
	if got := fixture.reachability.lastVersion.String(); got != synRSupplementVersionID {
		t.Fatalf("问权威用的版本 = %q, want 新版本 %s", got, synRSupplementVersionID)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}
	// 两本账在新版本这一拍各落一行：上面那几个零数的确实是链会写的账，不是两张空表。
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_reachability_judgment`); n != 1 {
		t.Fatalf("新版本之后可达性判断行数 = %d, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_financial_control`); n != 1 {
		t.Fatalf("新版本之后财务控制行数 = %d, want 1", n)
	}
}

// assertSupersededVersionUnjudged 断言首版信封入账后链上什么也没记：权威没被问，判断账与控制账没有行，接受信封
// 没有入队，委托仍是`已提交`、当前版本仍是补充形成的那一版。控制账为空即没为旧版本占资金——财务控制权威替身
// 不计数，判据落在它答了之后必写的那本账上。
func (fixture *customerSupplementLoopFixture) assertSupersededVersionUnjudged(t *testing.T, ctx context.Context) {
	t.Helper()

	if fixture.reachability.assessments != 0 {
		t.Fatalf("可达性权威被问了 %d 次（最近一次版本 %q）, want 0——为已换代的版本判了可达性",
			fixture.reachability.assessments, fixture.reachability.lastVersion)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_reachability_judgment`); n != 0 {
		t.Fatalf("可达性判断行数 = %d, want 0", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_financial_control`); n != 0 {
		t.Fatalf("财务控制行数 = %d, want 0——为已换代的版本发起了资金占用", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("入队了 %d 封接受信封", n)
	}
	current := fixture.mustLoadRequest(t, ctx)
	if current.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("状态 = %q, want SUBMITTED——换代不是决定", current.State())
	}
	if got := current.CurrentSubmissionVersion().VersionID().String(); got != synRSupplementVersionID {
		t.Fatalf("当前版本 = %q, want %s", got, synRSupplementVersionID)
	}
}
