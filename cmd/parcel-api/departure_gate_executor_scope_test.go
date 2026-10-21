package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证出发门禁读数回给执行方的那一格（ADR-0237；票 demo-chain-gaps/146）：出发前问询与出发两口经生产装配
// 与作业事实渠道，回给设备的逐对象那一格只列没得出放行的对象，库面那一份照旧记全。同一段里装着两个客户的包裹——「客户 A」「客户 B」
// 只是合成标识里的字样，运输履约不知道包裹属于哪个客户，也不需要知道：判法只看各自那一格。证据只记 `S`。

type executorScopeWire struct {
	Answer  string `json:"answer"`
	From    string `json:"from"`
	To      string `json:"to"`
	Objects []struct {
		Object  string    `json:"object"`
		Answer  string    `json:"answer"`
		Members *[]string `json:"members"`
	} `json:"objects"`
}

func executorScopeOf(t *testing.T, raw string) executorScopeWire {
	t.Helper()
	var reading executorScopeWire
	if err := json.Unmarshal([]byte(raw), &reading); err != nil {
		t.Fatalf("gateReading 解不开：%v（%s）", err, raw)
	}
	return reading
}

// recordedObjects 取库面那一行逐对象那几格的对象，定序。
func (line *previewLine) recordedObjects(fact string) []string {
	line.t.Helper()
	var raw []byte
	if err := line.pool.QueryRow(line.t.Context(),
		`SELECT gate_reading_detail FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = $1`, fact).Scan(&raw); err != nil {
		line.t.Fatalf("读库面：%v", err)
	}
	var detail struct {
		Findings []struct {
			Object    string `json:"object"`
			Judgement string `json:"judgement"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		line.t.Fatalf("解库面：%v", err)
	}
	var objects []string
	for _, finding := range detail.Findings {
		objects = append(objects, finding.Object)
	}
	slices.Sort(objects)
	return objects
}

// Covers: AT-TF-316、AT-TF-318、AT-TF-319 真库（ADR-0237，证据 S）——演示动线上自营 SHA→SZX 一段里装着两个客户的货：客户 A 一件直接
// 随车、被关务限制装载出发，另一件装在一袋里、同样被限制；客户 B 一件直接随车、一件在同一袋里，都不受门禁。出发前问询与随后同参出发
// 都答阻断，回给设备的逐对象那一格只列袋与客户 A 被挡的那两件，袋那一格只指名被挡的成员；客户 B 的两件在答复里哪儿都不出现。两口
// `gateReading` 逐字节相同。库面那一行照旧每件一格、各带关务判断标识，运营与 VE 查得到全部。
func TestTheExecutorIsShownOnlyTheObjectsThatDidNotPass(t *testing.T) {
	line := newPreviewLine(t)
	const (
		segment     = "SYN-SEG-146-SHARED"
		bag         = "SYN-BAG-146"
		heldLoose   = "SYN-PARCEL-146-CUST-A-LOOSE"
		heldBagged  = "SYN-PARCEL-146-CUST-A-BAGGED"
		cleanLoose  = "SYN-PARCEL-146-CUST-B-LOOSE"
		cleanBagged = "SYN-PARCEL-146-CUST-B-BAGGED"
	)
	line.openUnit(bag)
	line.receive("SYN-HU-146-A", heldBagged)
	line.receive("SYN-HU-146-B", cleanBagged)
	line.putIn(bag, "SYN-HU-146-A")
	line.putIn(bag, "SYN-HU-146-B")
	line.hold(heldLoose)
	line.hold(heldBagged)
	for _, object := range []string{heldLoose, cleanLoose, bag} {
		line.handOver(segment, object)
	}

	asked := line.ask(segment)
	if asked.Code != http.StatusOK {
		t.Fatalf("问询 HTTP %d %s", asked.Code, asked.Body.String())
	}
	departed := line.departAs("SYN-MOVE-146-SHARED", segment)
	if departed.Code != http.StatusCreated {
		t.Fatalf("出发 HTTP %d %s", departed.Code, departed.Body.String())
	}
	previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed)
	if previewed != recorded {
		t.Fatalf("问询与同参出发回给设备的不是同一份：\n问询 %s\n出发 %s", previewed, recorded)
	}

	reading := executorScopeOf(t, recorded)
	if reading.Answer != "BLOCKED" || reading.From != "SYN-NODE-SHA-HUB" || reading.To != "SYN-NODE-SZX-GATE" {
		t.Fatalf("整格与起讫要照实：%s", recorded)
	}
	shown := map[string][]string{}
	for _, object := range reading.Objects {
		if object.Answer != "BLOCKED" {
			t.Fatalf("%s 答 %s 却在列：%s", object.Object, object.Answer, recorded)
		}
		members := []string{"-"}
		if object.Members != nil {
			members = *object.Members
		}
		shown[object.Object] = members
	}
	want := map[string][]string{bag: {heldBagged}, heldBagged: {"-"}, heldLoose: {"-"}}
	if len(shown) != len(want) {
		t.Fatalf("回给设备的 = %v，want %v", shown, want)
	}
	for object, members := range want {
		if !slices.Equal(shown[object], members) {
			t.Fatalf("回给设备的 = %v，want %v", shown, want)
		}
	}
	for _, leaked := range []string{cleanLoose, cleanBagged, "SYN-HU-146", "AADJ-"} {
		if strings.Contains(asked.Body.String(), leaked) || strings.Contains(departed.Body.String(), leaked) {
			t.Fatalf("答复里出现了 %s：\n问询 %s\n出发 %s", leaked, asked.Body.String(), departed.Body.String())
		}
	}

	all := []string{bag, heldBagged, heldLoose, cleanBagged, cleanLoose}
	slices.Sort(all)
	if got := line.recordedObjects("SYN-MOVE-146-SHARED"); !slices.Equal(got, all) {
		t.Fatalf("库面逐对象那几格 = %v，want 每件一格 %v", got, all)
	}
	var judged int
	if err := line.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.transport_movement_fact f,
		        jsonb_array_elements(f.gate_reading_detail -> 'findings') finding
		  WHERE f.fact_ref = 'SYN-MOVE-146-SHARED' AND finding ->> 'judgement' LIKE 'AADJ-%'`).Scan(&judged); err != nil {
		t.Fatalf("数库面判断标识：%v", err)
	}
	if judged != 4 {
		t.Fatalf("库面带关务判断标识的格 %d，want 四件包裹各一格（袋那一格不问关务）", judged)
	}
}

// Covers: AT-TF-317 真库（ADR-0237，证据 S）——段里那件包裹不受门禁：问询与出发都答不受门禁、起讫照实，回给设备的答复里没有逐对象
// 那一格（连 `objects` 键都没有），两口逐字节相同；库面那一行照旧记着那一件与它的关务判断标识。
func TestADepartureThatPassesShowsTheExecutorNoObjectsButRecordsThemAll(t *testing.T) {
	line := newPreviewLine(t)
	line.handOver("SYN-SEG-146-CLEAN", "SYN-PARCEL-146-CLEAN")

	asked := line.ask("SYN-SEG-146-CLEAN")
	departed := line.departAs("SYN-MOVE-146-CLEAN", "SYN-SEG-146-CLEAN")
	if asked.Code != http.StatusOK || departed.Code != http.StatusCreated {
		t.Fatalf("问询 %d %s / 出发 %d %s", asked.Code, asked.Body.String(), departed.Code, departed.Body.String())
	}
	previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed)
	if previewed != recorded {
		t.Fatalf("两口回给设备的不是同一份：\n问询 %s\n出发 %s", previewed, recorded)
	}
	if reading := executorScopeOf(t, recorded); reading.Answer != "NOT_GUARDED" || reading.From != "SYN-NODE-SHA-HUB" ||
		reading.To != "SYN-NODE-SZX-GATE" || strings.Contains(recorded, `"objects"`) {
		t.Fatalf("回给设备的 = %s，want 整格不受门禁、没有逐对象那一格", recorded)
	}
	if got := line.recordedObjects("SYN-MOVE-146-CLEAN"); !slices.Equal(got, []string{"SYN-PARCEL-146-CLEAN"}) {
		t.Fatalf("库面逐对象那几格 = %v，want 照旧记着那一件", got)
	}
}

// registerSeededSchedule 照演示种子原样登一份周期安排，并按运行日期显式登那一班——与 newDepartureLine 登上海→深圳那一班同一套
// 登记用例。
func (line *previewLine) registerSeededSchedule(templateFile, runningDate string) {
	line.t.Helper()
	t := line.t
	schedules, err := buildTransportScheduleRegistration(line.db)
	if err != nil {
		t.Fatalf("装配班次登记：%v", err)
	}
	var template struct {
		TenantID string `json:"tenantId"`
		tfhttp.ScheduleTemplateRegistrationPayload
	}
	if err := json.Unmarshal(demoSeed(t, "transport", templateFile), &template); err != nil {
		t.Fatalf("读周期安排种子 %s：%v", templateFile, err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, template.TenantID)
	templateCommand, err := template.ScheduleTemplateRegistrationPayload.Command(tenant)
	if err != nil {
		t.Fatalf("译周期安排：%v", err)
	}
	if result, err := schedules.RegisterTemplate(t.Context(), templateCommand); err != nil || result.Outcome() != tfapp.ScheduleTemplateRegistered {
		t.Fatalf("登周期安排：%s / %v", result.Outcome(), err)
	}
	scheduleCommand, err := tfhttp.TransportScheduleRegistrationPayload{Template: template.Template, RunningDate: runningDate}.Command(tenant)
	if err != nil {
		t.Fatalf("译班次：%v", err)
	}
	if result, err := schedules.RegisterSchedule(t.Context(), scheduleCommand); err != nil || result.Outcome() != tfapp.TransportScheduleRegistered {
		t.Fatalf("登班次：%s / %v", result.Outcome(), err)
	}
}

// askOn 以演示场外设备的出示，按指定班次问一段出发前那一问。
func (line *previewLine) askOn(schedule, segment string) string {
	line.t.Helper()
	response := line.post(line.preview, "/transport-fulfillment/departure-gate-previews", "SYN-OFFSITE-01",
		`{"schedule":"`+schedule+`","segment":"`+segment+`","departsAt":"2026-10-21T22:05:00+08:00"}`)
	if response.Code != http.StatusOK {
		line.t.Fatalf("问询 HTTP %d %s", response.Code, response.Body.String())
	}
	return response.Body.String()
}

// Covers: AT-TF-114、AT-TF-319 真库（ADR-0237 决定五，证据 S）——评审 PoC 的那一格：演示动线上一段境内的上海→深圳自营段装着两个客户
// 的包裹，都不在任何关务案卷里。设备按本班问，答不受门禁、一件不列。设备改配演示种子里一班深圳→新加坡的跨区班次去问同一段：读数
// 停在本侧「班次不服务该段」、答未决，一件不列，整库一张表都不变——连关务都没被问到，于是它也问不出「段里每件跨区无案、都未决」那
// 一份全列的读数。照这么配登出发，出发照样 201 落册，读数同样停在本侧、库面没有逐对象那一格。另一段的交接没声明交出与去向节点、
// 核不上段：按本班问照问关务、答不受门禁，回给设备的标「未绑定」、一件不列；给它也配那一班跨区班次去问，关务逐件答未决，回给设备的
// 仍只有整格与那一位——配假班次在这类段上只换得到一位汇总（AT-TF-317 真库半边）。
func TestPairingASegmentWithAnotherSchedulesDirectionNoLongerWidensTheReading(t *testing.T) {
	line := newPreviewLine(t)
	line.registerSeededSchedule("02-schedule-template-szx-sin.json", "2026-10-21")
	const (
		segment     = "SYN-SEG-146-DOMESTIC"
		crossZone   = "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"
		customerA   = "SYN-PARCEL-146-POC-CUST-A"
		customerB   = "SYN-PARCEL-146-POC-CUST-B"
		undeclared  = "SYN-SEG-146-UNDECLARED"
		undeclaredP = "SYN-PARCEL-146-POC-UNDECLARED"
	)
	line.handOver(segment, customerA)
	line.handOver(segment, customerB)
	line.handOverDeclaring(undeclared, undeclaredP, "", "")

	honest := line.askOn(departureLineSchedule, segment)
	if !strings.Contains(honest, `"answer":"NOT_GUARDED"`) || strings.Contains(honest, "SYN-PARCEL-146-POC") {
		t.Fatalf("按本班问 = %s，want 不受门禁、一件不列", honest)
	}

	before := line.snapshot()
	probe := line.askOn(crossZone, segment)
	var answer struct {
		GateReading executorScopeWire `json:"gateReading"`
	}
	if err := json.Unmarshal([]byte(probe), &answer); err != nil {
		t.Fatalf("decode %s：%v", probe, err)
	}
	if answer.GateReading.Answer != "UNDECIDED" || answer.GateReading.From != "SYN-NODE-SZX-GATE" ||
		!strings.Contains(probe, `"stops":["SCHEDULE_NOT_SERVING_SEGMENT"]`) || len(answer.GateReading.Objects) != 0 ||
		strings.Contains(probe, "SYN-PARCEL-146-POC") {
		t.Fatalf("改配跨区班次问同一段 = %s，want 停在本侧、一件不列", probe)
	}
	if changed := changedSince(before, line.snapshot()); len(changed) != 0 {
		t.Fatalf("改配班次的问询改了 %v，want 连关务都没问到", changed)
	}

	departed := line.post(line.movement, "/transport-fulfillment/movement-facts", "SYN-OFFSITE-01",
		`{"fact":"SYN-MOVE-146-POC","schedule":"`+crossZone+`","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",`+
			`"version":"v1","occurredAt":"2026-10-21T22:05:00+08:00","segment":"`+segment+`"}`)
	if departed.Code != http.StatusCreated || gateReadingBytes(t, departed) != gateReadingBytes(t, line.post(line.preview,
		"/transport-fulfillment/departure-gate-previews", "SYN-OFFSITE-01",
		`{"schedule":"`+crossZone+`","segment":"`+segment+`","departsAt":"2026-10-21T22:05:00+08:00"}`)) {
		t.Fatalf("照这么配登出发 = %d %s，want 201、读数与问询同停在本侧", departed.Code, departed.Body.String())
	}
	if recorded := line.recordedObjects("SYN-MOVE-146-POC"); len(recorded) != 0 {
		t.Fatalf("落册的读数有逐对象那几格 %v，want 停在本侧", recorded)
	}

	unbound := line.askOn(departureLineSchedule, undeclared)
	if !strings.Contains(unbound, `"gateReading":{"answer":"NOT_GUARDED","from":"SYN-NODE-SHA-HUB","to":"SYN-NODE-SZX-GATE","entryUnbound":true}`) {
		t.Fatalf("交接没声明节点的段按本班问 = %s，want 照问关务、标未绑定、一件不列", unbound)
	}
	faked := line.askOn(crossZone, undeclared)
	if !strings.Contains(faked, `"gateReading":{"answer":"UNDECIDED","from":"SYN-NODE-SZX-GATE","to":"SYN-NODE-SIN-HUB","entryUnbound":true}`) ||
		strings.Contains(faked, undeclaredP) {
		t.Fatalf("核不上的段配跨区班次问 = %s，want 只有整格与未绑定那一位、一件不列", faked)
	}
}

// pickUp 把一件凭场外揽收交进 segment：揽收事实只有客户门口、没有节点，这一段的进段节点本就说不出。
func (line *previewLine) pickUp(segment, parcel string) {
	line.t.Helper()
	controlFacts, err := buildControlFactOrchestrations(line.db)
	if err != nil {
		line.t.Fatalf("装配控制事实：%v", err)
	}
	result, err := controlFacts.pickupRegistration.Register(line.t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:   mustValue(line.t, tfdomain.NewTenantID, departureLineTenant),
		Object:     parcel,
		Task:       "SYN-PICKUP-TASK-" + parcel,
		Attempt:    "SYN-ATTEMPT-" + parcel,
		Place:      "SYN-DOOR-" + parcel,
		Control:    "SYN-TRANSPORT-CONTROL-" + parcel,
		ExecutedBy: "SYN-COURIER-146",
		OccurredAt: departureLineDepartedAt.Add(-2 * time.Hour),
		Segment:    segment,
	})
	if err != nil || result.Outcome() != tfapp.PickupRegistered {
		line.t.Fatalf("揽收 %s：%v / %v", parcel, result.Outcome(), err)
	}
}

// recordedDetail 取库面那一行的明细：逐对象各格的对象、格、首条依据，以及「未绑定」那一位。
func (line *previewLine) recordedDetail(fact string) (map[string][2]string, bool) {
	line.t.Helper()
	var raw []byte
	if err := line.pool.QueryRow(line.t.Context(),
		`SELECT gate_reading_detail FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = $1`, fact).Scan(&raw); err != nil {
		line.t.Fatalf("读库面：%v", err)
	}
	var detail struct {
		EntryUnbound bool `json:"entryUnbound"`
		Findings     []struct {
			Object    string `json:"object"`
			Answer    string `json:"answer"`
			Judgement string `json:"judgement"`
			Bases     []struct {
				Cause string `json:"cause"`
			} `json:"bases"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		line.t.Fatalf("解库面：%v", err)
	}
	cells := map[string][2]string{}
	for _, finding := range detail.Findings {
		if !strings.HasPrefix(finding.Judgement, "AADJ-") || len(finding.Bases) == 0 {
			line.t.Fatalf("库面 %s 那一格没有关务判断标识或依据：%s", finding.Object, raw)
		}
		cells[finding.Object] = [2]string{finding.Answer, finding.Bases[0].Cause}
	}
	return cells, detail.EntryUnbound
}

// Covers: AT-TF-317 真库（ADR-0237 决定五，证据 S；二轮评审探针那一格）——凭场外揽收进的一段装着两件包裹，一件被关务限制装载出发：
// 揽收事实没有节点，起讫核不上段，但不停在本侧——按演示班次上海→深圳问关务。问询与同参出发都答阻断，回给设备的只有整格、起讫与
// 「未绑定」那一位，被挡那一件也不列，两口逐字节相同；库面那一行两件各一格、带关务判断标识，被扣那一件依据是那条限制，明细记着那一位
// ——全程追踪据它照旧形成出发门禁阻断信号（派发那一半在 cmd/parcel-dispatch）。另一段的进段交接只声明了交出节点、没报去向（演示尾程
// 交接的样子），同样照问、同样只回整格。
func TestASegmentEnteredByPickupStillAsksCustomsAndRecordsTheBlock(t *testing.T) {
	line := newPreviewLine(t)
	const (
		pickupSegment = "SYN-SEG-146-PICKUP"
		held          = "SYN-PARCEL-146-PICKUP-HELD"
		clean         = "SYN-PARCEL-146-PICKUP-CLEAN"
		noOnward      = "SYN-SEG-146-NO-ONWARD"
		noOnwardHeld  = "SYN-PARCEL-146-NO-ONWARD-HELD"
	)
	line.pickUp(pickupSegment, held)
	line.pickUp(pickupSegment, clean)
	line.hold(held)
	line.handOverDeclaring(noOnward, noOnwardHeld, "SYN-NODE-SHA-HUB", "")
	line.hold(noOnwardHeld)

	for segment, fact := range map[string]string{pickupSegment: "SYN-MOVE-146-PICKUP", noOnward: "SYN-MOVE-146-NO-ONWARD"} {
		asked := line.ask(segment)
		departed := line.departAs(fact, segment)
		if asked.Code != http.StatusOK || departed.Code != http.StatusCreated {
			t.Fatalf("%s 问询 %d / 出发 %d：%s", segment, asked.Code, departed.Code, departed.Body.String())
		}
		const want = `{"answer":"BLOCKED","from":"SYN-NODE-SHA-HUB","to":"SYN-NODE-SZX-GATE","entryUnbound":true}`
		if previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed); previewed != want || recorded != want {
			t.Fatalf("%s 回给设备的：\n问询 %s\n出发 %s\nwant %s", segment, previewed, recorded, want)
		}
	}

	cells, unbound := line.recordedDetail("SYN-MOVE-146-PICKUP")
	if !unbound || len(cells) != 2 || cells[held] != [2]string{"BLOCKED", "RESTRICTION_IN_FORCE"} || cells[clean][0] != "NOT_GUARDED" {
		t.Fatalf("库面 = %v，未绑定 %v，want 两件各一格、被扣那一件阻断、依据那条限制、记着未绑定", cells, unbound)
	}
	if cells, unbound := line.recordedDetail("SYN-MOVE-146-NO-ONWARD"); !unbound || cells[noOnwardHeld][0] != "BLOCKED" {
		t.Fatalf("没报去向那一段的库面 = %v，未绑定 %v", cells, unbound)
	}
}
