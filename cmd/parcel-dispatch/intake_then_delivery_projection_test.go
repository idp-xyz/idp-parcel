package main

import (
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-TF-B：先节点收寄再有效交付，同一包裹追加第二条已接受事实。
// 第二拍投影两条未归类并换版；PS 终局仍停在规则未配置。不种 PAR-VIS-01。

const (
	deriveDeliveryConsumerName = "visibility-exception/derive-projection-from-effective-delivery"
	deliveryFactRef            = "effective-delivery/" + effectiveDeliveryObject + "/" + effectiveDeliveryAttempt
)

// Covers: 同一 SYN-PARCEL-01 上先形成节点收寄投影，再追加有效交付事实。第二拍必须
// 换版且两条都未归类——这是「第二条已接受事实追加投影」，不是覆盖第一版。
func TestNodeIntakeThenEffectiveDeliveryAppendsASecondUnclassifiedFact(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
	intakeEventID := recordFormedNodeIntake(t, fixture)
	assertAdoptionPreconditions(t, fixture)
	assertIntakeEligibilityUnproven(t, fixture, psdomain.NodeIntakeSource)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("第一拍定稿了 %d 条", published)
	}
	if got := recordedFailureCode(t, fixture.db, intakeEventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("收寄失败码 = %q, want dispatch.consumer_undecided", got)
	}
	assertUnclassifiedNodeIntakeProjection(t, fixture)
	versionAfterIntake := currentProjectionVersion(t, fixture)

	deliveryEventID := recordRegisteredEffectiveDelivery(t, fixture)
	assertEffectiveDeliveryFinalPreconditions(t, fixture)
	assertFinalRuleUnconfigured(t, fixture)

	// 派生交接与有效交付共用（租户+包裹）分区。收寄那版派生信封由客户视图链接住
	//（WIRE-CUSTOMER-VIEW）并定稿放行分区头，交付信封随后被认领、停在终局规则未配置。
	// 连拍直到交付信封有失败码。
	published = 0
	for i := 0; i < 8; i++ {
		n, err := fixture.beat.DispatchOnce(ctx)
		if err != nil {
			t.Fatalf("交付后第 %d 拍：%v", i+1, err)
		}
		published += n
		if recordedFailureCode(t, fixture.db, deliveryEventID) != "" {
			break
		}
	}
	if published != 1 {
		t.Fatalf("交付后定稿 %d 条, want 1（仅收寄版派生信封经视图链定稿）——终局未配置不得把交付信封定稿", published)
	}
	if got := recordedFailureCode(t, fixture.db, deliveryEventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("交付失败码 = %q, want dispatch.consumer_undecided（FINAL_RULE_UNCONFIGURED）", got)
	}

	assertTwoUnclassifiedFacts(t, fixture)
	if currentProjectionVersion(t, fixture) == versionAfterIntake {
		t.Fatal("追加第二条事实却没换版")
	}
	if n := fixture.countInbox(t, deriveDeliveryConsumerName, deliveryEventID); n != 1 {
		t.Fatalf("VE 交付 inbox = %d, want 1", n)
	}
	assertNoFinalOutcomeTrace(t, fixture, deliveryEventID)
}

func assertTwoUnclassifiedFacts(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation)
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	if len(projection.Entries()) != 2 {
		t.Fatalf("entries = %d, want 2——第二条已接受事实必须追加，不得覆盖", len(projection.Entries()))
	}

	facts := make([]string, 0, 2)
	for i, entry := range projection.Entries() {
		if _, classified := entry.Milestone(); classified {
			t.Fatalf("entries[%d] 归了类——禁止发明里程碑", i)
		}
		if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
			t.Fatalf("entries[%d] mapping = %q", i, entry.MappingVersion())
		}
		facts = append(facts, entry.Fact().Fact().String())
	}
	if facts[0] != nodeIntakeSourceID {
		t.Fatalf("第一条 fact = %s, want %s", facts[0], nodeIntakeSourceID)
	}
	if facts[1] != deliveryFactRef {
		t.Fatalf("第二条 fact = %s, want %s", facts[1], deliveryFactRef)
	}
}
