package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	tfnode "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证出发门禁读数展开集运单元（ADR-0225；票 demo-chain-gaps/42）：节点作业的单元、成员事实与收寄身份，
// 关务的限制，运输履约的段与班次都在同一个库里各自的表上，装配是生产那一套。测试输入是隔离合成，只记 `S`。

// wiredConsolidationMembership 是生产装配里那一份集运成员关系读口，给要换掉关务那一口的用例自己装读数编排时用。
func wiredConsolidationMembership(t *testing.T, db *bentopg.DB) tfports.ConsolidationMembershipView {
	t.Helper()
	memberships, err := nopostgres.NewConsolidationMembershipsAt(db)
	if err != nil {
		t.Fatalf("成员关系读口：%v", err)
	}
	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("收寄身份读口：%v", err)
	}
	view, err := tfnode.NewConsolidationMembershipView(memberships, identities)
	if err != nil {
		t.Fatalf("成员关系适配器：%v", err)
	}
	return view
}

// openUnit 在节点作业的单元册上开一件集运单元。
func (line *departureLine) openUnit(unit string) {
	line.t.Helper()
	line.seed(`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ($1, $2, 'SYN-ASSET-' || $2, 'OPEN', '[]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'SYN-OPEN-' || $2, 'performedBy', 'SYN-PACKER', 'evidence', 'SYN-WORK',
		                            'occurredAt', $3::timestamptz))`,
		departureLineTenant, unit, departureLineHandedOverAt.Add(-3*time.Hour))
}

// putIn 落一条移入事实：出发之前两小时，那件实物进了那件单元。
func (line *departureLine) putIn(unit, member string) {
	line.t.Helper()
	at := departureLineHandedOverAt.Add(-2 * time.Hour)
	line.seed(`INSERT INTO node_operations.consolidation_fact
			(tenant_id, source_id, content_digest, action, unit_id, member_id, performed_by, evidence, occurred_at, recorded_at)
		 VALUES ($1, 'SYN-ADD-' || $2 || '-' || $3, 'syn-digest', 'ADD_MEMBER', $2, $3, 'SYN-PACKER', 'SYN-WORK', $4, $4)`,
		departureLineTenant, unit, member, at)
}

// receive 在节点作业的收寄册上落一次识别成功的收寄：作业实物 handling 的正式包裹关联是 parcel。
func (line *departureLine) receive(handling, parcel string) {
	line.t.Helper()
	receivedAt := departureLineHandedOverAt.Add(-4 * time.Hour)
	line.seed(`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
			 service_result_check, recorded_at)
		 VALUES ($1, 'SYN-SCAN-' || $2, 'syn-digest', 'INTAKE_FORMED',
		         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'deliveredBy', 'SYN-COURIER',
		                            'evidence', 'SYN-EVIDENCE', 'version', 'SYN-INTAKE-' || $2,
		                            'association', $3::text, 'receivedAt', $4::timestamptz),
		         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'kind', 'NODE_INTAKE',
		                            'basis', 'SYN-INTAKE-' || $2, 'establishedAt', $4::timestamptz),
		         '[]'::jsonb, false, '[]'::jsonb, 'CHECKED', $4)`,
		departureLineTenant, handling, parcel, receivedAt)
}

// hold 在关务那边给一件包裹落一条约束装载出发的限制。
func (line *departureLine) hold(parcel string) {
	line.t.Helper()
	line.seed(`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-' || $2, 'SYN-DECISION-HOLD', $2, jsonb_build_array('LOADING_DEPARTURE'), $3)`,
		departureLineTenant, parcel, departureLineDepartedAt.Add(-3*time.Hour))
}

func gateCells(reading tfdomain.DepartureGateReading) map[string]tfdomain.GateFinding {
	cells := map[string]tfdomain.GateFinding{}
	for _, finding := range reading.Findings() {
		cells[finding.Object().String()] = finding
	}
	return cells
}

func unitMembers(t *testing.T, finding tfdomain.GateFinding) []string {
	t.Helper()
	members, unit := finding.ConsolidationUnit()
	if !unit {
		t.Fatalf("%s 不是单元那一格", finding.Object())
	}
	names := []string{}
	for _, member := range members {
		names = append(names, member.String())
	}
	return names
}

// Covers: AT-TF-276 真库（ADR-0225 决定一、三，证据 S）——自营 SHA→SZX 境内出发，段里在控的是一袋，袋里装着一件被关务限制装载出发
// 的包裹与一件干净包裹：读数不再是不受门禁，而是阻断；袋那一格取成员里最重的阻断、列出两件成员，两件各自一格进逐对象明细，
// 被扣那一件的依据是那条限制；库面那一行存着成员。出发之前同参问询读到的是同一份。
// AT-TF-280 真库半边：问询走同一条展开路径。
func TestABagHoldingADetainedParcelNoLongerDepartsDomesticallyClean(t *testing.T) {
	line := newDepartureLine(t)
	line.openUnit("SYN-BAG-42")
	line.receive("SYN-HU-HELD", "SYN-PARCEL-HELD-42")
	line.receive("SYN-HU-CLEAN", "SYN-PARCEL-CLEAN-42")
	line.putIn("SYN-BAG-42", "SYN-HU-HELD")
	line.putIn("SYN-BAG-42", "SYN-HU-CLEAN")
	line.hold("SYN-PARCEL-HELD-42")
	line.handOver("SYN-SEG-BAG-42", "SYN-BAG-42")

	preview, err := buildDepartureGatePreview(line.db)
	if err != nil {
		t.Fatalf("装问询：%v", err)
	}
	command := line.departureCommand("SYN-MOVE-BAG-42", "SYN-SEG-BAG-42")
	previewed, err := preview.Preview(t.Context(), tfapp.PreviewDepartureGateCommand{
		TenantID: command.TenantID, Schedule: command.Schedule, Segment: command.Segment, DepartsAt: command.OccurredAt,
	})
	if err != nil || previewed.Outcome() != tfapp.DepartureGatePreviewed {
		t.Fatalf("问询：%s / %v", previewed.Outcome(), err)
	}

	reading := line.depart("SYN-MOVE-BAG-42", "SYN-SEG-BAG-42")
	if reading.Answer() != tfdomain.GateBlocked {
		t.Fatalf("袋里有被扣包裹的境内出发 = %s，要阻断而不是不受门禁", reading.Answer())
	}
	cells := gateCells(reading)
	bag := cells["SYN-BAG-42"]
	if bag.Answer() != tfdomain.GateBlocked ||
		!slices.Equal(unitMembers(t, bag), []string{"SYN-PARCEL-CLEAN-42", "SYN-PARCEL-HELD-42"}) {
		t.Fatalf("袋那一格 = %s %v", bag.Answer(), unitMembers(t, bag))
	}
	if held := cells["SYN-PARCEL-HELD-42"]; held.Answer() != tfdomain.GateBlocked ||
		held.Bases()[0].Cause != "RESTRICTION_IN_FORCE" || held.Bases()[0].Sources[0] != "SYN-RESTRICTION-SYN-PARCEL-HELD-42" {
		t.Fatalf("被扣那一件 = %s %+v", held.Answer(), held.Bases())
	}
	if clean := cells["SYN-PARCEL-CLEAN-42"]; clean.Answer() != tfdomain.GateNotGuarded {
		t.Fatalf("干净那一件 = %s", clean.Answer())
	}
	if previewReading, _ := previewed.Reading(); !reflect.DeepEqual(previewReading, reading) {
		t.Fatalf("问询读数 %+v 与出发读数 %+v 不是同一份", previewReading, reading)
	}

	var raw []byte
	if err := line.pool.QueryRow(t.Context(),
		`SELECT gate_reading_detail FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = 'SYN-MOVE-BAG-42'`,
	).Scan(&raw); err != nil {
		t.Fatalf("读库面：%v", err)
	}
	var detail struct {
		Findings []struct {
			Object  string    `json:"object"`
			Members *[]string `json:"members"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("解库面：%v", err)
	}
	for _, finding := range detail.Findings {
		if (finding.Object == "SYN-BAG-42") != (finding.Members != nil) {
			t.Fatalf("库面 %s 的成员键 = %v，只有袋那一格带", finding.Object, finding.Members)
		}
	}
}

// Covers: AT-TF-277 真库（ADR-0225 决定四，证据 S）——成员读不动时袋那一格如实记读不动、依据指名节点作业那一口，不读成不受门禁：
// 收寄身份那一本读不动，袋那一格依据指名那件作业实物；成员关系那一本读不动，说不出这一件是不是袋，那一件记读不动、不按包裹问
// 关务。出发照样落册。
func TestABagWhoseMembersCannotBeReadIsRecordedUnreadable(t *testing.T) {
	t.Run("收寄身份读不动", func(t *testing.T) {
		line := newDepartureLine(t)
		line.openUnit("SYN-BAG-43")
		line.receive("SYN-HU-43", "SYN-PARCEL-43")
		line.putIn("SYN-BAG-43", "SYN-HU-43")
		line.handOver("SYN-SEG-BAG-43", "SYN-BAG-43")
		line.seed(`ALTER TABLE node_operations.reception RENAME TO reception_unreachable`)

		reading := line.depart("SYN-MOVE-BAG-43", "SYN-SEG-BAG-43")
		bag := gateCells(reading)["SYN-BAG-43"]
		if reading.Answer() != tfdomain.GateUnreadable || bag.Answer() != tfdomain.GateUnreadable ||
			bag.Bases()[0].Cause != tfdomain.ConsolidationMemberIdentityUnreadable.String() || bag.Bases()[0].Sources[0] != "SYN-HU-43" {
			t.Fatalf("袋那一格 = %s %+v，整格 %s", bag.Answer(), bag.Bases(), reading.Answer())
		}
	})
	t.Run("成员关系读不动", func(t *testing.T) {
		line := newDepartureLine(t)
		line.openUnit("SYN-BAG-44")
		line.handOver("SYN-SEG-BAG-44", "SYN-BAG-44")
		line.seed(`ALTER TABLE node_operations.consolidation_fact RENAME TO consolidation_fact_unreachable`)
		judgements := func() int {
			var count int
			if err := line.pool.QueryRow(t.Context(),
				`SELECT count(*) FROM customs_compliance.action_admission_judgement WHERE tenant_id = $1`,
				departureLineTenant).Scan(&count); err != nil {
				t.Fatalf("数关务判断册：%v", err)
			}
			return count
		}
		before := judgements()

		reading := line.depart("SYN-MOVE-BAG-44", "SYN-SEG-BAG-44")
		bag := gateCells(reading)["SYN-BAG-44"]
		if _, unit := bag.ConsolidationUnit(); unit || bag.Answer() != tfdomain.GateUnreadable ||
			bag.Bases()[0].Cause != "CONSOLIDATION_MEMBERSHIP_UNAVAILABLE" {
			t.Fatalf("说不出种类的那一件 = %s %+v unit=%v", bag.Answer(), bag.Bases(), unit)
		}
		if after := judgements(); after != before {
			t.Fatalf("关务判断册 %d → %d 行，说不出种类的那一件不该按包裹问关务", before, after)
		}
	})
}

// Covers: AT-TF-278 真库（ADR-0225 决定四，证据 S）——袋套袋：笼里装着袋与一件散件，袋里装着被扣包裹，三层逐层展开，笼那一格沿袋取到
// 阻断；空单元（出发那一刻什么也没装）答未决、依据「单元无成员」；没有收寄记录的成员让那一袋答未决、依据指名那件作业实物。
func TestNestedAndEmptyUnitsAreReadThroughTheirMembers(t *testing.T) {
	line := newDepartureLine(t)
	line.openUnit("SYN-CAGE-45")
	line.openUnit("SYN-BAG-45")
	line.receive("SYN-HU-45-HELD", "SYN-PARCEL-45-HELD")
	line.receive("SYN-HU-45-LOOSE", "SYN-PARCEL-45-LOOSE")
	line.putIn("SYN-BAG-45", "SYN-HU-45-HELD")
	line.putIn("SYN-CAGE-45", "SYN-BAG-45")
	line.putIn("SYN-CAGE-45", "SYN-HU-45-LOOSE")
	line.hold("SYN-PARCEL-45-HELD")
	line.handOver("SYN-SEG-CAGE-45", "SYN-CAGE-45")

	reading := line.depart("SYN-MOVE-CAGE-45", "SYN-SEG-CAGE-45")
	cells := gateCells(reading)
	if reading.Answer() != tfdomain.GateBlocked || cells["SYN-CAGE-45"].Answer() != tfdomain.GateBlocked ||
		cells["SYN-BAG-45"].Answer() != tfdomain.GateBlocked {
		t.Fatalf("整格 %s、笼 %s、袋 %s，要沿袋取到阻断", reading.Answer(), cells["SYN-CAGE-45"].Answer(), cells["SYN-BAG-45"].Answer())
	}
	if members := unitMembers(t, cells["SYN-CAGE-45"]); !slices.Equal(members, []string{"SYN-BAG-45", "SYN-PARCEL-45-LOOSE"}) {
		t.Fatalf("笼的成员 = %v", members)
	}
	if len(cells) != 4 {
		t.Fatalf("逐对象 %d 格，要笼、袋与两件包裹", len(cells))
	}

	line.openUnit("SYN-BAG-46-EMPTY")
	line.handOver("SYN-SEG-BAG-46", "SYN-BAG-46-EMPTY")
	empty := gateCells(line.depart("SYN-MOVE-BAG-46", "SYN-SEG-BAG-46"))["SYN-BAG-46-EMPTY"]
	if empty.Answer() != tfdomain.GateUndecided || empty.Bases()[0].Cause != tfdomain.ConsolidationUnitWithoutMembers.String() {
		t.Fatalf("空单元 = %s %+v，要未决而不是不受门禁", empty.Answer(), empty.Bases())
	}

	line.openUnit("SYN-BAG-47")
	line.putIn("SYN-BAG-47", "SYN-HU-47-UNRECEIVED")
	line.handOver("SYN-SEG-BAG-47", "SYN-BAG-47")
	unresolved := gateCells(line.depart("SYN-MOVE-BAG-47", "SYN-SEG-BAG-47"))["SYN-BAG-47"]
	if unresolved.Answer() != tfdomain.GateUndecided ||
		unresolved.Bases()[0].Cause != tfdomain.ConsolidationMemberIdentityUnresolved.String() ||
		unresolved.Bases()[0].Sources[0] != "SYN-HU-47-UNRECEIVED" {
		t.Fatalf("成员身份不成立的袋 = %s %+v", unresolved.Answer(), unresolved.Bases())
	}
}
