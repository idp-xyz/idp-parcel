package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

const finalOutcomeFormedEventType = "parcel-shipment.final-outcome.formed"

// Covers: AT-PS-173；ADR-0295 决定三、四、六——认定取消依据不成立并续办之后，重新形成的终局自己交出一封 `parcel-shipment.final-outcome.formed` 信封。
// 作废之前，依据这份取消的不采用行已经交过一封同名信封并已投递；信封 ID 若只由采用键认领，终局与不采用行同键，入队按（来源，事件标识）去重，
// 重新形成的终局就没有信封，结算侧的有效交付终局计价时点与异常侧的终局投影都停在「不采用」。作废代次折进该包裹终局信封的交接身份，两封各有各的 ID。
func TestARedrivenFinalHandsOffItsOwnFormedEnvelopeAfterTheRefusalEnvelopeWasDelivered(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}
	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——取消在册，交付不采用并入账", code)
	}
	formedBefore := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`)
	if formedBefore != 1 {
		t.Fatalf("不采用之后 formed 信封 %d 封, want 1——不采用行也交一封", formedBefore)
	}
	// 让这封不采用信封被投递出去（下游已消费），之后的决定与续办面对的是已投递的旧信封。
	for i := 0; i < 3; i++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("再拍：%v", err)
		}
	}

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	if n := fixture.countFinalizedOfParcel(t); n != 1 {
		t.Fatalf("续办之后当前有效终局 %d 份, want 1", n)
	}
	formedAfter := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`)
	if formedAfter != formedBefore+1 {
		t.Fatalf("续办之后 formed 信封 %d 封, want %d——重新形成的终局要有自己的信封，不能被不采用行那一封的去重吞掉", formedAfter, formedBefore+1)
	}
	if distinct := fixture.countSQL(t, `SELECT count(DISTINCT event_id) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`); distinct != formedAfter {
		t.Fatalf("formed 信封 ID 去重后 %d 个, want %d", distinct, formedAfter)
	}
	if partitions := fixture.countSQL(t, `SELECT count(DISTINCT partition_key) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`); partitions != 1 {
		t.Fatalf("formed 信封分区键 %d 个, want 1——同一包裹的先后两封排在一条队里", partitions)
	}
}

// decideConflictOf 在一笔事务里对册上这件包裹、指名这份取消的那一份冲突下决定（与生产事务壳同形）。
func (rig *redriveRig) decideConflictOf(t *testing.T, cancellation string, kind psdomain.CancellationOrderConflictDecisionKind) {
	t.Helper()
	conflicts, err := pspostgres.NewCancellationOrderConflicts(rig.fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := conflicts.ListByParcel(t.Context(), rig.fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation))
	if err != nil {
		t.Fatalf("按包裹查冲突：%v", err)
	}
	for _, entry := range entries {
		if entry.Cancellation.String() != cancellation {
			continue
		}
		mustWithinTX(t, rig.fixture.transactor, t.Context(), func(txCtx context.Context) error {
			_, decideErr := rig.decide.Handle(txCtx, psapplication.DecideCancellationOrderConflictCommand{
				Tenant: entry.TenantID, Parcel: entry.Parcel, BoundaryKind: entry.Boundary.Kind, BoundaryRef: entry.Boundary.Reference,
				Cancellation: entry.Cancellation, Kind: kind,
				Decider: mustPS(t, psdomain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#SYN-DECIDER-CODE76"),
				Basis:   mustPS(t, psdomain.NewCancellationOrderConflictDecisionBasis, "SYN-NODE-SCAN-CHECKED"),
			})
			return decideErr
		})
		return
	}
	t.Fatalf("册上没有指名 %s 的冲突：%+v", cancellation, entries)
}

// redriveCancellation 把指名这份取消的续办信封交给续办适配器，在事务里跑。
func (rig *redriveRig) redriveCancellation(t *testing.T, cancellation string) {
	t.Helper()
	var raw []byte
	querier, err := rig.fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(),
		`SELECT payload FROM bento.outbox WHERE event_type = $1 AND payload->>'cancellation' = $2`,
		notUpheldEventType, cancellation).Scan(&raw); err != nil {
		t.Fatalf("读指名 %s 的续办信封：%v", cancellation, err)
	}
	var payload struct {
		TenantID     string `json:"tenantId"`
		Parcel       string `json:"parcel"`
		Cancellation string `json:"cancellation"`
		BoundaryKind string `json:"boundaryKind"`
		BoundaryRef  string `json:"boundaryRef"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("解信封：%v", err)
	}
	mustWithinTX(t, rig.fixture.transactor, t.Context(), func(txCtx context.Context) error {
		return rig.redriver.HandleNotUpheldCancellation(txCtx, psinbox.NotUpheldCancellation{
			TenantID: payload.TenantID, Parcel: payload.Parcel, Cancellation: payload.Cancellation,
			BoundaryKind: payload.BoundaryKind, BoundaryRef: payload.BoundaryRef,
		})
	})
}

// beatUntil 一拍一拍地派发，直到条件成立或到期。同包裹的收寄信封在本夹具里停在下游未决，终局信封排在同一条分区队里，要等它的失败预算耗尽才轮到。
func (fixture *synVerticalFixture) beatUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("派发超时仍未达到：%s", what)
		}
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("派发：%v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// visibilityFinalFacts 取 VE 事实库里这件包裹终局流的各事实：版本与类型到前身。
func visibilityFinalFacts(t *testing.T, fixture *synVerticalFixture) map[string]string {
	t.Helper()
	facts, err := vepostgres.NewAcceptedFacts(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	records, err := facts.FindByParcel(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation))
	if err != nil {
		t.Fatalf("读 VE 事实库：%v", err)
	}
	versions := map[string]string{}
	for _, record := range records {
		if record.Fact.Source() != vedomain.SourceParcelShipment || record.Fact.Fact().String() != "final-outcome/"+nodeIntakeAssociation+"/EFFECTIVE_DELIVERY" {
			continue
		}
		predecessor, _ := record.Fact.Supersedes()
		versions[record.Fact.Version().String()+"/"+record.Fact.Kind().String()] = predecessor.String()
	}
	return versions
}

// visibilityCarriesTheFinal 问 VE 当前投影有没有已采用终局，并返回其中来自 PS 的终局条目（已采用与不采用）的版本。
func visibilityCarriesTheFinal(t *testing.T, fixture *synVerticalFixture) (bool, []string) {
	t.Helper()
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation))
	if err != nil {
		t.Fatalf("读 VE 当前投影：%v", err)
	}
	if !found {
		return false, nil
	}
	var entries []string
	for _, entry := range projection.Entries() {
		if entry.Fact().Source() == vedomain.SourceParcelShipment &&
			(entry.Fact().Kind().String() == vedomain.FinalOutcomeAdoptedFactKind || entry.Fact().Kind().String() == vedomain.FinalOutcomeNotAdoptedFactKind) {
			entries = append(entries, entry.Fact().Version().String()+"/"+entry.Fact().Kind().String())
		}
	}
	return projection.CarriesCustomerServiceFinal(), entries
}

// 把包裹带到「不采用的信封已投递、VE 已有不采用事实」这一步：取消在册，收寄停在冲突上，有效交付因取消被不采用。
func arriveAtADeliveredRefusal(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}
	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——取消在册，交付不采用并入账", code)
	}
	fixture.beatUntil(t, "不采用信封被 VE 消费", func() bool {
		facts := visibilityFinalFacts(t, fixture)
		return len(facts) == 1
	})
}

// Covers: AT-PS-173、AT-VE-174；ADR-0295 决定六——认定取消依据不成立并续办之后，VE 终局投影出现重新形成的已采用终局（结算侧按键回读，见下）。
// 事实键（租户、来源、事实引用、版本）若仍是采用键，重新形成的终局与被删的不采用行同键、内容不同，被判成来源冲突、保留原事实，投影停在「不采用」；
// 信封带作废代次，VE 译装器给新一代另一个事实版本并指名不采用那一版为前身，被取代的不采用事实留档、不参与派生。
func TestARedrivenFinalReachesTheVisibilityProjectionAsAnAdoptedFinal(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	arriveAtADeliveredRefusal(t, fixture)
	if carried, entries := visibilityCarriesTheFinal(t, fixture); carried || len(entries) != 1 {
		t.Fatalf("决定之前 VE 投影：已采用终局 %t，终局条目 %v；want 只有不采用", carried, entries)
	}
	before := fixture.outboxIDsOfType(t, finalOutcomeFormedEventType)

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	after := fixture.outboxIDsOfType(t, finalOutcomeFormedEventType)
	if len(after) != len(before)+1 {
		t.Fatalf("formed 信封 %d 封, want %d", len(after), len(before)+1)
	}
	newEnvelope := ""
	for _, id := range after {
		if id != before[0] {
			newEnvelope = id
		}
	}
	// VE 与结算侧是同一封信封的两个消费方。结算侧在本夹具里停在计价参数未登记的未决上（租户实例半边，本包不种），信封因此重投到失败预算耗尽；
	// VE 那一路在第一次投递就落了账，之后的重投答已有结果。
	fixture.beatUntil(t, "重新形成的终局被 VE 消费", func() bool {
		return fixture.countInbox(t, "visibility-exception/derive-projection-from-final-outcome", newEnvelope) == 1
	})

	carried, entries := visibilityCarriesTheFinal(t, fixture)
	if !carried || len(entries) != 1 || !strings.HasSuffix(entries[0], "#void-1/final-outcome-adopted") {
		t.Fatalf("续办之后 VE 投影：已采用终局 %t，终局条目 %v；want 恰一条已采用终局（代次一的版本），不采用那一条被取代", carried, entries)
	}
	facts := visibilityFinalFacts(t, fixture)
	if len(facts) != 2 {
		t.Fatalf("VE 事实库的终局流 = %v, want 不采用与已采用各一份留档", facts)
	}
	for version, predecessor := range facts {
		if strings.HasSuffix(version, "#void-1/final-outcome-adopted") && !strings.HasPrefix(version, predecessor+"#void-1") {
			t.Fatalf("已采用的事实指名前身 %q, want 它取代的不采用那一版 %v", predecessor, facts)
		}
	}
	if predecessor, kept := facts["SYN-DELIVERY-V1/final-outcome-not-adopted"]; !kept || predecessor != "" {
		t.Fatalf("VE 事实库 = %v, want 原不采用事实原样留档", facts)
	}

	// 同代次再跑几拍：重放不翻倍。
	for i := 0; i < 3; i++ {
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("再拍：%v", err)
		}
	}
	if again := visibilityFinalFacts(t, fixture); len(again) != 2 {
		t.Fatalf("再拍之后 VE 事实库 = %v, want 仍是两份", again)
	}
}

// Covers: AT-PS-173、AT-VE-174；ADR-0295 决定六——同一采用键的记录可以不止被取消挡一次：第一份取消被认定不成立后，续办重新形成的记录又被同一包裹上的第二份取消挡成不采用
// （代次一，不采用事实换到代次一的版本），第二份取消再被认定不成立，续办形成终局（代次二）。每一代信封带自己的作废代次，VE 的替代链一代取代一代：事实库三代都留档，
// 当前投影里这条终局流只剩最后那一代已采用终局；同代次重放不翻倍。
func TestASecondVoidedCancellationClosesAnotherGenerationInTheVisibilityProjection(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	arriveAtADeliveredRefusal(t, fixture)
	// 同一件包裹上的第二份取消：第一份被认定不成立之后它才是有效取消；业务时间晚于收寄，所以续办交回的收寄会对它再判出冲突。
	seedStandingCancellationNamed(t, fixture, nodeIntakeAssociation, time.Now().UTC().Add(time.Hour), "SYN-PCXL-02",
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-CANCEL-REQUEST-02"))

	rig := newRedriveRig(t, fixture)
	rig.decideConflictOf(t, "SYN-PCXL-01", psdomain.CancellationNotUpheld)
	rig.redriveCancellation(t, "SYN-PCXL-01")
	fixture.beatUntil(t, "代次一的不采用事实进 VE", func() bool {
		return len(visibilityFinalFacts(t, fixture)) == 2
	})
	if carried, entries := visibilityCarriesTheFinal(t, fixture); carried || len(entries) != 1 || !strings.HasSuffix(entries[0], "#void-1/final-outcome-not-adopted") {
		t.Fatalf("第二份取消挡住之后 VE 投影：已采用终局 %t，终局条目 %v；want 只有代次一的不采用", carried, entries)
	}

	rig.decideConflictOf(t, "SYN-PCXL-02", psdomain.CancellationNotUpheld)
	rig.redriveCancellation(t, "SYN-PCXL-02")
	fixture.beatUntil(t, "代次二的终局进 VE", func() bool {
		return len(visibilityFinalFacts(t, fixture)) == 3
	})

	facts := visibilityFinalFacts(t, fixture)
	want := map[string]string{
		"SYN-DELIVERY-V1/final-outcome-not-adopted":        "",
		"SYN-DELIVERY-V1#void-1/final-outcome-not-adopted": "SYN-DELIVERY-V1",
		"SYN-DELIVERY-V1#void-2/final-outcome-adopted":     "SYN-DELIVERY-V1#void-1",
	}
	if len(facts) != len(want) {
		t.Fatalf("VE 事实库 = %v, want %v", facts, want)
	}
	for version, predecessor := range want {
		if got, kept := facts[version]; !kept || got != predecessor {
			t.Fatalf("VE 事实库 = %v, want %v", facts, want)
		}
	}
	carried, entries := visibilityCarriesTheFinal(t, fixture)
	if !carried || len(entries) != 1 || entries[0] != "SYN-DELIVERY-V1#void-2/final-outcome-adopted" {
		t.Fatalf("两次作废之后 VE 投影：已采用终局 %t，终局条目 %v；want 恰一条代次二的已采用终局", carried, entries)
	}
	if n := fixture.countFinalizedOfParcel(t); n != 1 {
		t.Fatalf("PS 当前有效终局 %d 份, want 1", n)
	}

	// 同代次再跑几拍：重放不翻倍。
	for i := 0; i < 3; i++ {
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("再拍：%v", err)
		}
	}
	if again := visibilityFinalFacts(t, fixture); len(again) != len(want) {
		t.Fatalf("再拍之后 VE 事实库 = %v, want 仍是三份", again)
	}
}
