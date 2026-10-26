package main

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证复核按业务时间排（ADR-0235）：触发的业务时间早于路由历史里最近一次已判控制触发时，入账为
// 「触发早于当前位置」、不判；收寄采用与交接两路同一条判据，两路接同一份复核编排（与生产同形，只换证据视图与成本取数侧）。已接受
// 委托由 PS 编排形成，交接经 TF 生产登记编排登记并真交出信封，收寄采用记录经 PS 真交接适配器交出信封。夹具全部合成，
// 只记 `S`。

type reassessBothLanesFixture struct {
	syn      *synVerticalFixture
	register *tfapplication.RegisterTransportHandoverHandler
	handover dispatch.Consumer
	intake   dispatch.Consumer
	evidence *carriedGeoEvidence
	costs    *reassessCostsDouble
}

// newReassessBothLanesFixture 在已接受的委托上接好两路复核（reassessBothLanesOver），判断键上落一份上海枢纽 → 新加坡枢纽 →
// 新加坡末端的计划，冻结形态已声明、未越过边界。
func newReassessBothLanesFixture(t *testing.T) *reassessBothLanesFixture {
	t.Helper()
	syn := newSYNVerticalFixture(t)
	acceptWithDeclaredGeo(t, syn)
	saveThroughSingaporePlan(t, syn)
	return reassessBothLanesOver(t, syn)
}

// reassessBothLanesOver 接交接与收寄采用两路到同一份复核编排上：交接那一路走生产的 Over 与哨兵名单，收寄采用那一路按生产
// 同样的接法（采用仓储与 PS 委托仓储 → 适配器 → 消费门）。委托与路由历史由调用方先备好。
func reassessBothLanesOver(t *testing.T, syn *synVerticalFixture) *reassessBothLanesFixture {
	t.Helper()
	outboxStore, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	limit := 0
	evidence := &carriedGeoEvidence{evidence: nrports.InitialRouteEvidence{
		Strategy:                    mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision:                mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-VIEW-REV-1"),
		FreezeForm:                  nrdomain.RemainingSegmentCountFreeze,
		FreezeRemainingSegmentLimit: &limit,
	}}
	costs := &reassessCostsDouble{}
	reassess, err := reassessRouteHandlerOver(syn.db, systemClock{}, evidence, costs)
	if err != nil {
		t.Fatalf("装配复核编排：%v", err)
	}
	purpose := mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE")
	handoverConsumer, err := reassessOnHandoverConsumerOver(syn.db, inboxStore, reassess, purpose)
	if err != nil {
		t.Fatalf("装配交接那一路：%v", err)
	}
	handoverLane, err := dispatch.WithUndecidedSentinels(handoverConsumer, reassessOnHandoverUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包未决哨兵：%v", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(syn.db)
	if err != nil {
		t.Fatalf("构造采用仓储：%v", err)
	}
	accepted, err := nrparcelshipment.NewAcceptedCarriedGeoSource(syn.requests)
	if err != nil {
		t.Fatalf("构造已接受委托读口：%v", err)
	}
	intakes, err := nrparcelshipment.NewReassessOnNetworkIntakeAdapter(adoptions, accepted,
		nrparcelshipment.NewReassessOnIntakeAdapter(reassess, purpose))
	if err != nil {
		t.Fatalf("装配收寄采用那一路：%v", err)
	}
	intakeConsumer, err := nrinbox.NewNetworkIntakeConsumer(syn.db.Transactor(), inboxStore, intakes)
	if err != nil {
		t.Fatalf("装配收寄采用消费门：%v", err)
	}
	return &reassessBothLanesFixture{
		syn:      syn,
		register: newHandoverRegistration(t, syn.db, outboxStore),
		handover: handoverLane,
		intake:   intakeConsumer,
		evidence: evidence,
		costs:    costs,
	}
}

// handoverInto 登一次`已交接`进 receiving 的交接，业务时间取 at；范围与版本各取 tag，好让同一对象的几次交接各成一封信。
func (fixture *reassessBothLanesFixture) handoverInto(t *testing.T, tag, receiving string, at time.Time) string {
	t.Helper()
	command := handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/132/"+tag, tfdomain.ObjectHandedOver, "", receiving)
	command.Scope = reassessHandoverScope + "-" + tag
	command.JudgedAt = at
	return registerHandover(t, fixture.syn, fixture.register, command)
}

// intakeAt 落一份`采用`格的收寄采用记录，来源类型（节点收寄或场外揽收）、收寄地点与业务时间由用例给，并在同一事务经 PS
// 真交接适配器交出信封；交回信封 ID（前后两次列 outbox 取差，不在测试里重拼）。
func (fixture *reassessBothLanesFixture) intakeAt(
	t *testing.T, kind psdomain.IntakeSourceKind, version, place string, at time.Time,
) string {
	t.Helper()
	syn := fixture.syn
	ctx := t.Context()
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(syn.db)
	if err != nil {
		t.Fatalf("构造采用仓储：%v", err)
	}
	handoff, err := pspostgres.NewOutboxNetworkIntakeHandoff(syn.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄采用交接：%v", err)
	}
	key := psports.IntakeAdoptionKey{
		TenantID: syn.identity.TenantID(),
		Parcel:   mustPS(t, psdomain.NewDeclaredParcelID, reassessHandoverParcel),
		Kind:     kind,
		Version:  mustPS(t, psdomain.NewSourceResultVersion, version),
	}
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       key.Kind,
		Object:     mustPS(t, psdomain.NewSourceObjectReference, "SYN-UNIT-132"),
		Parcel:     key.Parcel,
		Place:      mustPS(t, psdomain.NewIntakePlaceReference, place),
		Control:    mustPS(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-132"),
		Version:    key.Version,
		OccurredAt: at,
	})
	if err != nil {
		t.Fatalf("收寄来源：%v", err)
	}
	intake, err := psdomain.AdoptNetworkIntake(source, syn.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID())
	if err != nil {
		t.Fatalf("采用收寄：%v", err)
	}
	commitment, err := psdomain.FormFormalCommitment(
		mustPS(t, psdomain.NewCommitmentVersionID, "SYN-CMT-132-"+version),
		intake,
		mustPS(t, psdomain.NewExpectedCommitmentReference, "SYN-RES-132@2026-10-18T00:00:00Z"),
	)
	if err != nil {
		t.Fatalf("正式承诺：%v", err)
	}
	record := psports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: syn.identity.CustomerAccountID(),
		ShipmentRequestID: syn.requestID,
		ContentDigest:     "SYN-ADOPTION-DIGEST-132-" + version,
		Adopted:           true,
		Intake:            intake,
		Commitment:        commitment,
		AdoptedAt:         at.Add(time.Minute),
	}
	before := syn.outboxIDsOfType(t, string(nrinbox.AdoptedNetworkIntakeEventType))
	mustWithinTX(t, syn.transactor, ctx, func(txCtx context.Context) error {
		saved, err := adoptions.Save(txCtx, record)
		if err != nil {
			return err
		}
		if saved != psports.IntakeAdoptionSaved {
			t.Errorf("save outcome = %v, want 已写入", saved)
		}
		return handoff.HandOffNetworkIntake(txCtx, psports.NetworkIntakeHandoffIntent{Record: record})
	})
	for _, id := range syn.outboxIDsOfType(t, string(nrinbox.AdoptedNetworkIntakeEventType)) {
		if !slices.Contains(before, id) {
			return id
		}
	}
	t.Fatalf("收寄采用 %s 没有交出信封", version)
	return ""
}

func deliver(t *testing.T, syn *synVerticalFixture, lane dispatch.Consumer, eventID, asID string) {
	t.Helper()
	if err := lane.Consume(t.Context(), outboxEnvelope(t, syn, eventID, asID)); err != nil {
		t.Fatalf("投递 %s = %v, want 入账", asID, err)
	}
}

// conclusionsByCorrelation 把复核行按关联排成一张表，好按触发点名。
func conclusionsByCorrelation(t *testing.T, syn *synVerticalFixture) map[string]string {
	t.Helper()
	found := map[string]string{}
	for _, row := range reassessmentRows(t, syn) {
		found[row.correlation] = row.conclusion
	}
	return found
}

// latestJudgedCorrelation 读路由历史里业务时间最晚的那一次复核是哪个关联：当前位置就是它。
func latestJudgedCorrelation(t *testing.T, syn *synVerticalFixture) string {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var correlation string
	if err := querier.QueryRow(t.Context(),
		`SELECT correlation_id FROM network_routing.route_reassessment
		  WHERE tenant_id = $1 ORDER BY trigger_occurred_at DESC LIMIT 1`, "SYN-TENANT-01",
	).Scan(&correlation); err != nil {
		t.Fatalf("读最晚那一次复核：%v", err)
	}
	return correlation
}

// handoverCorrelation 照交接触发关联的拼法（四维逐维「字节长度:原串」）算 handoverInto 那一次的期望关联。拼法本身由
// AT-NR-064 那条用例按原串钉住，这里只为按触发点名。
func handoverCorrelation(tag string) string {
	correlation := "handover"
	for _, part := range []string{
		"SYN-TENANT-01", reassessHandoverParcel, reassessHandoverScope + "-" + tag, "SYN-HANDOVER/132/" + tag,
	} {
		correlation += "/" + strconv.Itoa(len(part)) + ":" + part
	}
	return correlation
}

// Covers: AT-NR-079、AT-NR-081（交接一路）——包裹已在新加坡枢纽判过（仍适用），一次更早的交接（进吉隆坡枢纽，不在计划上）
// 后到：入账为「触发早于当前位置」，不判——原计划不因那个已经过去的位置失效；未判登记点名比较所用的当前位置时刻。同一版
// 交接另一封信封再投同答。随后一次更晚的交接进吉隆坡枢纽照判，原计划这时才按实际节点与计划不符失效。
func TestAnEarlierHandoverArrivingAfterALaterOneIsNotJudgedOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	later := fixture.handoverInto(t, "later", reassessHandoverReceiving, reassessHandoverAt)
	earlier := fixture.handoverInto(t, "earlier", "SYN-NODE-KUL-HUB", reassessHandoverAt.Add(-3*time.Hour))

	deliver(t, fixture.syn, fixture.handover, later, later)
	deliver(t, fixture.syn, fixture.handover, earlier, earlier)
	deliver(t, fixture.syn, fixture.handover, earlier, earlier+"/again")

	if got := conclusionsByCorrelation(t, fixture.syn); len(got) != 1 || got[handoverCorrelation("later")] != "STILL_APPLICABLE" {
		t.Fatalf("复核 = %v, want 只有较晚那次、仍适用——较早的位置不得据以失效", got)
	}
	rows := notJudgedRows(t, fixture.syn)
	if len(rows) != 1 || rows[0].correlation != handoverCorrelation("earlier") ||
		rows[0].reason != "TRIGGER_PRECEDES_CURRENT_POSITION" ||
		!rows[0].triggerAt.Equal(reassessHandoverAt.Add(-3*time.Hour)) ||
		rows[0].positionAt == nil || !rows[0].positionAt.Equal(reassessHandoverAt) {
		t.Fatalf("未判 = %+v, want 较早那次一行、早于当前位置 %s", rows, reassessHandoverAt)
	}
	for _, id := range []string{earlier, earlier + "/again"} {
		if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, id); status != "PROCESSED" {
			t.Fatalf("%s 的 inbox status = %q, want PROCESSED", id, status)
		}
	}

	newer := fixture.handoverInto(t, "newer", "SYN-NODE-KUL-HUB", reassessHandoverAt.Add(3*time.Hour))
	deliver(t, fixture.syn, fixture.handover, newer, newer)
	if got := conclusionsByCorrelation(t, fixture.syn)[handoverCorrelation("newer")]; got != "PLAN_LAPSED" {
		t.Fatalf("较新那次 = %q, want PLAN_LAPSED——较新的触发照判", got)
	}
}

// Covers: AT-NR-080——收寄采用一路同一条判据：包裹已经凭交接在新加坡枢纽判过，一份更早的节点收寄采用（上海枢纽）后到，
// 入账为「触发早于当前位置」，不判；收寄采用信封照常入账。
func TestAnEarlierIntakeArrivingAfterAJudgedHandoverIsNotJudgedOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	handover := fixture.handoverInto(t, "hub", reassessHandoverReceiving, reassessHandoverAt)
	intakeAt := reassessHandoverAt.Add(-48 * time.Hour)
	intake := fixture.intakeAt(t, psdomain.NodeIntakeSource, "SYN-INTAKE/132/v1", "SYN-NODE-SHA-HUB", intakeAt)

	deliver(t, fixture.syn, fixture.handover, handover, handover)
	deliver(t, fixture.syn, fixture.intake, intake, intake)

	if got := conclusionsByCorrelation(t, fixture.syn); len(got) != 1 || got[handoverCorrelation("hub")] != "STILL_APPLICABLE" {
		t.Fatalf("复核 = %v, want 只有交接那一次", got)
	}
	rows := notJudgedRows(t, fixture.syn)
	if len(rows) != 1 || rows[0].reason != "TRIGGER_PRECEDES_CURRENT_POSITION" || !rows[0].triggerAt.Equal(intakeAt) ||
		rows[0].positionAt == nil || !rows[0].positionAt.Equal(reassessHandoverAt) {
		t.Fatalf("未判 = %+v, want 收寄那一次一行、早于当前位置 %s", rows, reassessHandoverAt)
	}
	if want := "intake/SYN-TENANT-01/SYN-PARCEL-01/NODE_INTAKE/SYN-INTAKE/132/v1"; rows[0].correlation != want {
		t.Fatalf("未判关联 = %q, want %q", rows[0].correlation, want)
	}
	if status := inboxStatusOf(t, fixture.syn, "network-routing/reassess-on-network-intake", intake); status != "PROCESSED" {
		t.Fatalf("收寄采用信封的 inbox status = %q, want PROCESSED", status)
	}
}

// Covers: AT-NR-082——乱序到达有确定答案：一份节点收寄采用（较早）与一次交接（较晚），按业务时间到达时两次都判；较早的后到
// 时它入账不判。两种到达顺序下，路由历史里业务时间最晚的那一次复核都是交接，当前位置不倒退；收寄那一次恰在复核与未判
// 两处之一。后到的那份收寄再投一次，答案不变。
func TestOutOfOrderReassessmentTriggersHaveADeterminedAnswerOnTheDatabase(t *testing.T) {
	for name, intakeFirst := range map[string]bool{"按业务时间到达": true, "较早的收寄后到": false} {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessBothLanesFixture(t)
			intake := fixture.intakeAt(t, psdomain.NodeIntakeSource, "SYN-INTAKE/132/v1", "SYN-NODE-SHA-HUB", reassessHandoverAt.Add(-48*time.Hour))
			handover := fixture.handoverInto(t, "hub", reassessHandoverReceiving, reassessHandoverAt)
			if intakeFirst {
				deliver(t, fixture.syn, fixture.intake, intake, intake)
				deliver(t, fixture.syn, fixture.handover, handover, handover)
			} else {
				deliver(t, fixture.syn, fixture.handover, handover, handover)
				deliver(t, fixture.syn, fixture.intake, intake, intake)
				deliver(t, fixture.syn, fixture.intake, intake, intake+"/again")
			}

			if got := latestJudgedCorrelation(t, fixture.syn); got != handoverCorrelation("hub") {
				t.Fatalf("业务时间最晚的复核 = %q, want 交接那一次", got)
			}
			intakeCorrelation := "intake/SYN-TENANT-01/SYN-PARCEL-01/NODE_INTAKE/SYN-INTAKE/132/v1"
			_, judged := conclusionsByCorrelation(t, fixture.syn)[intakeCorrelation]
			behind := 0
			for _, row := range notJudgedRows(t, fixture.syn) {
				if row.correlation == intakeCorrelation && row.reason == "TRIGGER_PRECEDES_CURRENT_POSITION" {
					behind++
				}
			}
			if judged != intakeFirst || (behind == 1) == intakeFirst || behind > 1 {
				t.Fatalf("收寄那一次：判了 %v、早于当前位置 %d 行；want 按业务时间到达时判、后到时入账一行", judged, behind)
			}
		})
	}
}
