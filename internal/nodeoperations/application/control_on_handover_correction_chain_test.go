package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证节点控制按权威运输交接推进时，更正链只按 TF 的更正回指认（ADR-0220）：同一（对象，范围）下另起的首登是另一次交接，
// 不是前一次的更正。替身的交接来源只在 Earlier 里给出回指上溯的前版；更正链既有各格（后继版仍`已交接`零写入、撤回控制入账待重算、
// 一次交接不转出它自己立起的控制）由 transfer_control_on_handover_test.go 与 establish_control_on_handover_test.go 证，那里的更正
// 都显式回指前版。

const sameScope = "SYN-SCOPE-SZX"

// Covers: `AT-NO-126`（ADR-0220）——同一范围下先交进节点、再从同一节点交出，两次都是首登：交出那一次照常转出交进那一次立起的控制，
// 不剔掉它；声明的交出节点就是那一份的节点，不记信息格。演示动线重走（psb/05 停点一）里那件包裹就停在这里。
func TestAHandoverOutOfANodeUnderTheSameScopeTransfersOutTheControlTheHandoverInEstablished(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.controls.active = nil
	fixture.declaredHandover(t, sameScope, "H2/v1", "", shenzhenGW, true)
	inbound := establishedIn(t, fixture.advanceIn(t, sameScope, "H2/v1").In())

	fixture.declaredHandover(t, sameScope, "H3/v1", shenzhenGW, "", true)
	result := fixture.advanceIn(t, sameScope, "H3/v1")

	if result.Out().Outcome() != application.ControlTransferredOutOnHandover {
		t.Fatalf("交出那一次的转出 = %s, want CONTROL_TRANSFERRED_OUT——它不是交进那一次的更正", result.Out().Outcome())
	}
	out, _ := result.Out().Transition()
	released, _ := out.Transition.Control()
	if released.Basis() != inbound.Basis() || released.Active() {
		t.Fatalf("转出的那一份 = %+v, want 交进那一次立起的 %s", released, inbound.Basis())
	}
	if _, mismatched := out.Transition.ReleasingNodeMismatch(); mismatched {
		t.Fatal("声明的交出节点就是那一份的节点，不该记信息格")
	}
	if result.In().Outcome() != application.HandoverDeclaresNoReceivingNode {
		t.Fatalf("交出那一次的转入 = %s", result.In().Outcome())
	}
	if active := fixture.activeControls(t); len(active) != 0 {
		t.Fatalf("在身控制 = %v, want 交出之后一份不剩", active)
	}
}

// Covers: `AT-NO-126`（ADR-0220）——同一范围下另起的首登在两个方向上都自己判：前一次转出过，它不答「前版已转出」，照此刻在身的控制
// 判；前一次转入过，它不答「前版已转入」，照常转出那一份、在自己声明的接收节点立控制。
func TestASeparateFirstRegistrationUnderTheSameScopeIsJudgedOnItsOwn(t *testing.T) {
	t.Run("转出方向", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handedOver(t, "v1")
		if first := fixture.transfer(t, "v1"); first.Outcome() != application.ControlTransferredOutOnHandover {
			t.Fatalf("前一次 = %s", first.Outcome())
		}

		fixture.handedOver(t, "v2")
		result := fixture.transfer(t, "v2")

		recorded, _ := result.Transition()
		if result.Outcome() != application.ControlNotAdvancedOnHandover || recorded.Transition.Reason() != domain.NoActiveControl {
			t.Fatalf("另起的首登 = %s %s, want 照此刻判、入账「无在身控制」而不是零写入", result.Outcome(), recorded.Transition.Reason())
		}
	})
	t.Run("转入方向", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.controls.active = nil
		fixture.declaredHandover(t, sameScope, "H2/v1", "", shenzhenGW, true)
		inbound := establishedIn(t, fixture.advanceIn(t, sameScope, "H2/v1").In())

		fixture.declaredHandover(t, sameScope, "H2b/v1", shenzhenGW, singaporeHB, true)
		result := fixture.advanceIn(t, sameScope, "H2b/v1")

		out, _ := result.Out().Transition()
		if released, _ := out.Transition.Control(); result.Out().Outcome() != application.ControlTransferredOutOnHandover ||
			released.Basis() != inbound.Basis() {
			t.Fatalf("转出 = %s，转出的那一份 %+v", result.Out().Outcome(), released)
		}
		established := establishedIn(t, result.In())
		if established.Node().String() != singaporeHB {
			t.Fatalf("转入立在 %s, want 这一次声明的接收节点", established.Node())
		}
	})
}

// Covers: `AT-NO-127`（ADR-0220）——同一范围下另起的首登是拒收或待确认：两个方向都零写入，不入账「控制来源链待重算」——前一次推进过，
// 可它不是前一次的更正；前一次立起的控制照旧在身。
func TestARefusedFirstRegistrationUnderTheSameScopeIsNotAWithdrawingCorrection(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, sameScope, "H2/v1", shanghaiHub, shenzhenGW, true)
	inbound := establishedIn(t, fixture.advanceIn(t, sameScope, "H2/v1").In())
	saves := fixture.controls.saves

	fixture.declaredHandover(t, sameScope, "H3/v1", shenzhenGW, singaporeHB, false)
	result := fixture.advanceIn(t, sameScope, "H3/v1")

	if result.Out().Outcome() != application.HandoverDoesNotTransferControl ||
		result.In().Outcome() != application.HandoverDoesNotTransferControl || fixture.controls.saves != saves {
		t.Fatalf("转出 %s、转入 %s、又落册 %d 行, want 零写入", result.Out().Outcome(), result.In().Outcome(), fixture.controls.saves-saves)
	}
	if active := fixture.activeControls(t); len(active) != 1 || active[0] != inbound {
		t.Fatalf("在身控制 = %v, want 前一次立起的那一份", active)
	}
}

// Covers: `AT-NO-128` 的编排面（ADR-0220）——交接来源给的前版连不成一条更正链（前版换了范围）：不推进、不落行，作为错误交回，让消费门
// 落发布失败——这是来源的缺陷，重投不会变。
func TestEarlierVersionsThatAreNotACorrectionChainFailLoudly(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.handedOver(t, "v2")
	otherScope, err := domain.NewAuthoritativeHandoverReference("parcel-1", "SYN-SCOPE-OTHER", "v1")
	if err != nil {
		t.Fatal(err)
	}
	answer := fixture.handovers.answers["v2"]
	answer.Earlier = []domain.AuthoritativeHandoverReference{otherScope}
	fixture.handovers.answers["v2"] = answer

	_, err = fixture.handler.Advance(t.Context(), application.TransferControlOnHandoverCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), Handover: handoverVersion(t, "v2"),
	})

	if !errors.Is(err, domain.ErrInvalidHandoverCorrectionChain) || fixture.controls.saves != 0 || fixture.objects.asked != 0 {
		t.Fatalf("err = %v，落册 %d、认对象 %d, want 链立不起来、什么都不做", err, fixture.controls.saves, fixture.objects.asked)
	}
}

func (fixture *transferFixture) activeControls(t *testing.T) []domain.PhysicalControl {
	t.Helper()
	active, err := fixture.controls.FindActiveControls(t.Context(), mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewHandlingUnitID, "unit-1"))
	if err != nil {
		t.Fatal(err)
	}
	return active
}
