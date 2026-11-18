package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证仓登记用例（UC-WF-001 的仓修订）：判定次序、结果代数与并发。登记册用内存替身，真库上的重放、冲突与并发
// 另在 adapters/postgres 的用例里证。国家取产品内置目录里的真码（CN、US），业务时区取时区库里的真名。

func valueOf[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// fakeWarehouseRegistry 是仓登记册的内存替身：同键同内容答已在册、同键异内容答冲突，与真适配器同判据。保存在锁里
// 串行，等于主键唯一约束；afterLookup 让并发用例把两笔都停在「已查过在册」之后，逼出真实的竞态窗口。
type fakeWarehouseRegistry struct {
	mutex         sync.Mutex
	rows          map[string][]domain.WarehouseRevision
	saves         int
	loadErr       error
	loadLatestErr error
	saveErr       error
	saveOutcome   ports.WarehouseSaveOutcome
	afterLookup   func()
	beforeSave    func()
}

func newFakeWarehouseRegistry() *fakeWarehouseRegistry {
	return &fakeWarehouseRegistry{rows: make(map[string][]domain.WarehouseRevision)}
}

func warehouseKey(tenant domain.TenantID, code domain.WarehouseCode) string {
	return tenant.String() + "/" + code.String()
}

func (fake *fakeWarehouseRegistry) LoadWarehouseRevision(
	_ context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
	revision int,
) (domain.WarehouseRevision, bool, error) {
	if fake.loadErr != nil {
		return domain.WarehouseRevision{}, false, fake.loadErr
	}
	fake.mutex.Lock()
	var found domain.WarehouseRevision
	var has bool
	for _, row := range fake.rows[warehouseKey(tenant, code)] {
		if row.Revision() == revision {
			found, has = row, true
		}
	}
	fake.mutex.Unlock()
	if fake.afterLookup != nil {
		fake.afterLookup()
	}
	return found, has, nil
}

func (fake *fakeWarehouseRegistry) LoadLatestWarehouse(
	_ context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.WarehouseRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var latest domain.WarehouseRevision
	var has bool
	for _, row := range fake.rows[warehouseKey(tenant, code)] {
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has, nil
}

func (fake *fakeWarehouseRegistry) SaveWarehouseRevision(
	_ context.Context,
	revision domain.WarehouseRevision,
) (ports.WarehouseSaveOutcome, error) {
	if fake.beforeSave != nil {
		fake.beforeSave()
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.saves++
	if fake.saveErr != nil {
		return ports.WarehouseSaveOutcomeInvalid, fake.saveErr
	}
	if fake.saveOutcome != ports.WarehouseSaveOutcomeInvalid {
		return fake.saveOutcome, nil
	}
	key := warehouseKey(revision.Tenant(), revision.Code())
	for _, existing := range fake.rows[key] {
		if existing.Revision() != revision.Revision() {
			continue
		}
		if existing.SameContentAs(revision) {
			return ports.WarehouseRevisionAlreadyRegistered, nil
		}
		return ports.WarehouseRevisionContentConflict, nil
	}
	fake.rows[key] = append(fake.rows[key], revision)
	return ports.WarehouseRevisionSaved, nil
}

// seed 直接把一笔修订放进册，演「册上早就有」。
func (fake *fakeWarehouseRegistry) seed(revision domain.WarehouseRevision) {
	key := warehouseKey(revision.Tenant(), revision.Code())
	fake.rows[key] = append(fake.rows[key], revision)
}

func (fake *fakeWarehouseRegistry) revisionsOf(code string) []domain.WarehouseRevision {
	return fake.rows["SYN-TENANT-01/"+code]
}

type fixedClock struct{ at time.Time }

func (clock *fixedClock) Now() time.Time { return clock.at }

var registrationInstant = time.Date(2026, 11, 18, 9, 30, 0, 123456789, time.UTC)

func contentSpec(t *testing.T) domain.WarehouseContentSpec {
	t.Helper()
	return domain.WarehouseContentSpec{
		Name: valueOf(t, domain.NewWarehouseName, "合成一号仓"),
		Kind: domain.WarehouseKindSelfOperated,
		Address: domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "CN"),
			valueOf(t, domain.NewAddressText, "合成路 1 号"),
		),
		BusinessTimeZone: valueOf(t, domain.NewBusinessTimeZone, "Asia/Shanghai"),
	}
}

func contentWith(t *testing.T, edit func(*domain.WarehouseContentSpec)) domain.WarehouseContent {
	t.Helper()
	spec := contentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		t.Fatalf("NewWarehouseContent: %v", err)
	}
	return content
}

func registerCommand(t *testing.T, revision int, edit func(*domain.WarehouseContentSpec)) application.RegisterWarehouseCommand {
	t.Helper()
	return application.RegisterWarehouseCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:       valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Revision:   revision,
		Content:    contentWith(t, edit),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func newHandler(registry *fakeWarehouseRegistry) *application.RegisterWarehouseHandler {
	return application.NewRegisterWarehouseHandler(registry, &fixedClock{at: registrationInstant})
}

func mustRegister(
	t *testing.T,
	handler *application.RegisterWarehouseHandler,
	command application.RegisterWarehouseCommand,
) application.RegisterWarehouseResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register 修订 %d：业务格不该走 error：%v", command.Revision, err)
	}
	return result
}

func expectOutcome(
	t *testing.T,
	label string,
	result application.RegisterWarehouseResult,
	want application.RegisterWarehouseOutcome,
) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s（拒绝理由 %s，指向 %s），want %s",
			label, result.Outcome(), result.Refusal(), result.Field(), want)
	}
}

func expectRefusal(
	t *testing.T,
	label string,
	result application.RegisterWarehouseResult,
	want application.RegisterWarehouseRefusal,
	wantField domain.WarehouseField,
) {
	t.Helper()
	expectOutcome(t, label, result, application.WarehouseNotAccepted)
	if result.Refusal() != want || result.Field() != wantField {
		t.Fatalf("%s：拒绝理由 = %s 指向 %q，want %s 指向 %q",
			label, result.Refusal(), result.Field(), want, wantField)
	}
}

// Covers: AT-WF-001（仓的部分）——首次登记答 REGISTERED 且修订 1 落册；原样重投答 ALREADY_REGISTERED、册里仍只有一份；
// 同一修订号换内容答 CONTENT_CONFLICT、在册不变；首笔修订号给 2 答 NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 并回显 0；
// 已有修订 1 时给 3 同样答错位并回显 1。
func TestRegisterWarehouseFirstRevisionReplayConflictAndOutOfSequence(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)

	first := mustRegister(t, handler, registerCommand(t, 1, nil))
	expectOutcome(t, "首次登记", first, application.WarehouseRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("回显的修订 = %+v（%v），want 修订 1、登记责任方 operator-1、时钟给的登记时刻", landed, has)
	}
	if len(registry.revisionsOf("SYN-WH-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(registry.revisionsOf("SYN-WH-01")))
	}

	replay := mustRegister(t, handler, registerCommand(t, 1, nil))
	expectOutcome(t, "原样重投", replay, application.WarehouseAlreadyRegistered)
	if registry.saves != 1 || len(registry.revisionsOf("SYN-WH-01")) != 1 {
		t.Fatalf("重放之后保存 %d 次、落册 %d 笔，want 1 次、1 笔", registry.saves, len(registry.revisionsOf("SYN-WH-01")))
	}

	conflict := mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（改名）")
	}))
	expectOutcome(t, "同一修订号换内容", conflict, application.WarehouseContentConflict)
	if got := registry.revisionsOf("SYN-WH-01")[0].Content().Name().String(); got != "合成一号仓" {
		t.Fatalf("冲突之后在册的名称 = %q，在册的不该变", got)
	}

	skipped := registerCommand(t, 2, nil)
	skipped.Code = valueOf(t, domain.NewWarehouseCode, "SYN-WH-02")
	result := mustRegister(t, handler, skipped)
	expectRefusal(t, "首笔给 2", result, application.WarehouseRefusalRevisionOutOfSequence, domain.WarehouseFieldInvalid)
	if result.LatestRevision() != 0 || len(registry.revisionsOf("SYN-WH-02")) != 0 {
		t.Fatalf("首笔错位回显 %d、落册 %d 笔，want 回显 0、不落册", result.LatestRevision(), len(registry.revisionsOf("SYN-WH-02")))
	}

	result = mustRegister(t, handler, registerCommand(t, 3, nil))
	expectRefusal(t, "已有修订 1 时给 3", result, application.WarehouseRefusalRevisionOutOfSequence, domain.WarehouseFieldInvalid)
	if result.LatestRevision() != 1 || len(registry.revisionsOf("SYN-WH-01")) != 1 {
		t.Fatalf("跳号回显 %d、落册 %d 笔，want 回显 1、仍是 1 笔", result.LatestRevision(), len(registry.revisionsOf("SYN-WH-01")))
	}
}

// 修订号小于 1 不是「最新加一」：与跳号同答错位并回显在册最新修订号，不另造一格。
func TestRegisterWarehouseTreatsANonPositiveRevisionNumberAsOutOfSequence(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)

	for _, number := range []int{0, -1} {
		result := mustRegister(t, handler, registerCommand(t, number, nil))
		expectRefusal(t, fmt.Sprintf("空册给 %d", number), result, application.WarehouseRefusalRevisionOutOfSequence, domain.WarehouseFieldInvalid)
		if result.LatestRevision() != 0 {
			t.Fatalf("空册给 %d 回显 %d，want 0", number, result.LatestRevision())
		}
	}
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)
	result := mustRegister(t, handler, registerCommand(t, 0, nil))
	expectRefusal(t, "已有修订 1 时给 0", result, application.WarehouseRefusalRevisionOutOfSequence, domain.WarehouseFieldInvalid)
	if result.LatestRevision() != 1 || registry.saves != 1 {
		t.Fatalf("回显 %d、保存 %d 次，want 回显 1、保存 1 次", result.LatestRevision(), registry.saves)
	}
}

// Covers: AT-WF-001（仓的部分）最后一行——并发提交同一修订号、内容相同的两笔，一笔答 REGISTERED、一笔答
// ALREADY_REGISTERED；册里只有一份，答重放的那一笔回显的是已落册的那一份（登记责任方是先落册者的）。内容不同时一笔落册、
// 另一笔答 CONTENT_CONFLICT。两笔都停在「已查过在册」之后再各自保存，逼的是真实的竞态窗口。
func TestRegisterWarehouseConcurrentSubmissionsOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		wantLoser  application.RegisterWarehouseOutcome
	}{
		"内容相同": {secondName: "合成一号仓", wantLoser: application.WarehouseAlreadyRegistered},
		"内容不同": {secondName: "合成一号仓（另一份）", wantLoser: application.WarehouseContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			registry := newFakeWarehouseRegistry()
			rendezvous := newRendezvous(2)
			registry.afterLookup = rendezvous.arrive
			handler := newHandler(registry)

			commands := []application.RegisterWarehouseCommand{
				registerCommand(t, 1, nil),
				registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
					spec.Name = valueOf(t, domain.NewWarehouseName, test.secondName)
				}),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")

			results := make([]application.RegisterWarehouseResult, len(commands))
			errs := make([]error, len(commands))
			var wait sync.WaitGroup
			for index := range commands {
				wait.Add(1)
				go func() {
					defer wait.Done()
					results[index], errs[index] = handler.Register(t.Context(), commands[index])
				}()
			}
			wait.Wait()

			for index, err := range errs {
				if err != nil {
					t.Fatalf("第 %d 笔：%v", index+1, err)
				}
			}
			if !rendezvous.met {
				t.Fatalf("两笔没有同时停在查册之后，没有覆盖竞态窗口")
			}
			landed, others := 0, []application.RegisterWarehouseResult{}
			for _, result := range results {
				if result.Outcome() == application.WarehouseRegistered {
					landed++
				} else {
					others = append(others, result)
				}
			}
			if landed != 1 || len(others) != 1 || others[0].Outcome() != test.wantLoser {
				t.Fatalf("两笔的结果 = %s、%s，want 一笔 REGISTERED、一笔 %s", results[0].Outcome(), results[1].Outcome(), test.wantLoser)
			}
			rows := registry.revisionsOf("SYN-WH-01")
			if len(rows) != 1 {
				t.Fatalf("落册 %d 笔，want 至多 1 笔", len(rows))
			}
			if test.wantLoser == application.WarehouseAlreadyRegistered {
				echoed, has := others[0].Revision()
				if !has || echoed.Registrant() != rows[0].Registrant() {
					t.Fatalf("答重放的一笔回显的登记责任方 = %q（%v），want 已落册那一份的 %q", echoed.Registrant(), has, rows[0].Registrant())
				}
			}
		})
	}
}

// rendezvous 让 n 个调用方都到了才一起放行；等不齐（超时）就放行并记下 met 为假，免得用例挂死。
type rendezvous struct {
	mutex   sync.Mutex
	waiting int
	want    int
	release chan struct{}
	met     bool
}

func newRendezvous(want int) *rendezvous {
	return &rendezvous{want: want, release: make(chan struct{})}
}

func (point *rendezvous) arrive() {
	point.mutex.Lock()
	point.waiting++
	if point.waiting == point.want {
		point.met = true
		close(point.release)
	}
	point.mutex.Unlock()
	select {
	case <-point.release:
	case <-time.After(5 * time.Second):
	}
}

// 竞态的两个窗口各演一遍，不靠调度碰运气：别的事务在「本事务查过册（没有）」之后，或在「本事务读过最新修订」之后、保存之前，
// 把同一修订号落了册。前一个窗口里，本事务随后读到的最新修订号已不小于自己的——不能当成跳号答错位，要回去再按内容答重放
// 或冲突；后一个窗口里靠登记册的主键，保存按内容答已在册或冲突。两个窗口的答案必须一样。
func TestRegisterWarehouseJudgesAConcurrentLandingByContentInBothRaceWindows(t *testing.T) {
	for windowName, install := range map[string]func(*fakeWarehouseRegistry, func()){
		"查册之后": func(registry *fakeWarehouseRegistry, land func()) { registry.afterLookup = land },
		"保存之前": func(registry *fakeWarehouseRegistry, land func()) { registry.beforeSave = land },
	} {
		for contentName, test := range map[string]struct {
			competitorName string
			want           application.RegisterWarehouseOutcome
		}{
			"内容相同": {"合成一号仓", application.WarehouseAlreadyRegistered},
			"内容不同": {"合成一号仓（别人先落的）", application.WarehouseContentConflict},
		} {
			t.Run(windowName+"/"+contentName, func(t *testing.T) {
				registry := newFakeWarehouseRegistry()
				competitorCommand := registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
					spec.Name = valueOf(t, domain.NewWarehouseName, test.competitorName)
				})
				competitorCommand.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
				competitor, err := domain.NewWarehouseRevision(
					competitorCommand.Tenant, competitorCommand.Code, 1, competitorCommand.Content,
					competitorCommand.Registrant, registrationInstant.Add(-time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				landed := false
				install(registry, func() {
					if !landed {
						landed = true
						registry.seed(competitor)
					}
				})

				result := mustRegister(t, newHandler(registry), registerCommand(t, 1, nil))
				expectOutcome(t, "别人先落册之后的这一笔", result, test.want)
				if !landed || len(registry.revisionsOf("SYN-WH-01")) != 1 {
					t.Fatalf("竞争者落册 %v、册里 %d 笔，want 已落、只有 1 笔", landed, len(registry.revisionsOf("SYN-WH-01")))
				}
				if test.want == application.WarehouseAlreadyRegistered {
					echoed, has := result.Revision()
					if !has || echoed.Registrant().String() != "operator-2" {
						t.Fatalf("回显的登记责任方 = %q（%v），want 先落册那一笔的 operator-2", echoed.Registrant(), has)
					}
				}
			})
		}
	}
}

// Covers: AT-WF-002——新修订把自营仓改成代理仓答 NOT_ACCEPTED/IMMUTABLE_FIELD 并指出种类那一格，册不变；反过来同样；
// 只改名称与地址答 REGISTERED，修订号加一，旧修订照留。
func TestRegisterWarehouseKeepsTheKindAndLetsNameAndAddressChange(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)

	toAgent := mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	expectRefusal(t, "自营仓改代理仓", toAgent, application.WarehouseRefusalImmutableField, domain.WarehouseFieldKind)
	if len(registry.revisionsOf("SYN-WH-01")) != 1 || registry.saves != 1 {
		t.Fatalf("被拒之后落册 %d 笔、保存 %d 次，want 册不变", len(registry.revisionsOf("SYN-WH-01")), registry.saves)
	}

	renamed := mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（迁址）")
		spec.Address = domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "US"),
			valueOf(t, domain.NewAddressText, "合成大道 2 号"),
		)
	}))
	expectOutcome(t, "只改名称与地址", renamed, application.WarehouseRegistered)
	rows := registry.revisionsOf("SYN-WH-01")
	if len(rows) != 2 || rows[0].Content().Name().String() != "合成一号仓" || rows[0].Content().Address().Country().String() != "CN" ||
		rows[1].Revision() != 2 || rows[1].Content().Address().Country().String() != "US" {
		t.Fatalf("改名改址之后的册 = %d 笔，旧修订该照留、新修订占修订号 2", len(rows))
	}

	agent := registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) { spec.Kind = domain.WarehouseKindAgent })
	agent.Code = valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT")
	expectOutcome(t, "代理仓修订 1", mustRegister(t, handler, agent), application.WarehouseRegistered)
	agent.Revision = 2
	agent.Content = contentWith(t, func(spec *domain.WarehouseContentSpec) { spec.Kind = domain.WarehouseKindSelfOperated })
	expectRefusal(t, "代理仓改自营仓", mustRegister(t, handler, agent), application.WarehouseRefusalImmutableField, domain.WarehouseFieldKind)
}

// Covers: AT-WF-004（国家）——国家不在内置目录答 NOT_ACCEPTED/CONTENT_INVALID 并指出那一格，不落册；目录只认两位大写的写法，
// 小写与带空白的同样不在目录里。
func TestRegisterWarehouseRefusesACountryOutsideTheBuiltInCatalogue(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	for _, country := range []string{"ZZ", "cn", "CN ", "CHN"} {
		if _, known := refdata.Country(country); known {
			t.Fatalf("前提不成立：%q 在内置国家目录里", country)
		}
		result := mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(valueOf(t, domain.NewCountryCode, country), spec.Address.Text())
		}))
		expectRefusal(t, "国家 "+country, result, application.WarehouseRefusalContentInvalid, domain.WarehouseFieldAddressCountry)
	}
	if registry.saves != 0 || len(registry.revisionsOf("SYN-WH-01")) != 0 {
		t.Fatalf("被拒之后保存 %d 次、落册 %d 笔，want 不落册", registry.saves, len(registry.revisionsOf("SYN-WH-01")))
	}
	expectOutcome(t, "目录里的国家", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)
}

// Covers: AT-WF-005——带同址物流节点登记，节点标识逐字保存；新修订去掉这个指名也答 REGISTERED。本用例不核对节点存不存在
// （BD-WF-001 首版不核），册里也只记标识。
func TestRegisterWarehouseKeepsTheColocatedNodeVerbatimAndLetsItGo(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	const node = "  SYN-Node-01\tx  "

	named := mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
		spec.ColocatedNode = valueOf(t, domain.NewColocatedNode, node)
	}))
	expectOutcome(t, "带同址物流节点", named, application.WarehouseRegistered)
	if got := registry.revisionsOf("SYN-WH-01")[0].Content().ColocatedNode(); !got.Named() || got.String() != node {
		t.Fatalf("在册的同址物流节点 = %q，want 逐字 %q", got, node)
	}

	dropped := mustRegister(t, handler, registerCommand(t, 2, nil))
	expectOutcome(t, "去掉同址物流节点", dropped, application.WarehouseRegistered)
	rows := registry.revisionsOf("SYN-WH-01")
	if len(rows) != 2 || rows[1].Content().ColocatedNode().Named() || !rows[0].Content().ColocatedNode().Named() {
		t.Fatalf("去掉指名之后的册不对：旧修订该仍带节点、新修订该不带")
	}
}

// Covers: AT-WF-006——不带业务时区答 REGISTERED，在册的业务时区如实为没登记；认不出的时区名答 NOT_ACCEPTED/CONTENT_INVALID
// 并指出那一格；之后用新修订补上答 REGISTERED。「Local」会被时区库读成部署机器的时区，带空白的名字也不是时区库里的名，都不收。
func TestRegisterWarehouseTakesNoDefaultBusinessTimeZone(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	without := func(spec *domain.WarehouseContentSpec) { spec.BusinessTimeZone = domain.BusinessTimeZone{} }

	expectOutcome(t, "不带业务时区", mustRegister(t, handler, registerCommand(t, 1, without)), application.WarehouseRegistered)
	if registry.revisionsOf("SYN-WH-01")[0].Content().BusinessTimeZone().Registered() {
		t.Fatalf("没给业务时区，在册的却带了 %q", registry.revisionsOf("SYN-WH-01")[0].Content().BusinessTimeZone())
	}

	for _, name := range []string{"Mars/Olympus_Mons", "Local", " Asia/Shanghai", "Asia/Shanghai ", "Asia/../Shanghai"} {
		result := mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
			spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, name)
		}))
		expectRefusal(t, "业务时区 "+name, result, application.WarehouseRefusalContentInvalid, domain.WarehouseFieldBusinessTimeZone)
	}
	if len(registry.revisionsOf("SYN-WH-01")) != 1 {
		t.Fatalf("认不出的时区被拒之后落册 %d 笔，want 1", len(registry.revisionsOf("SYN-WH-01")))
	}

	expectOutcome(t, "补上业务时区", mustRegister(t, handler, registerCommand(t, 2, nil)), application.WarehouseRegistered)
	rows := registry.revisionsOf("SYN-WH-01")
	if len(rows) != 2 || rows[0].Content().BusinessTimeZone().Registered() || rows[1].Content().BusinessTimeZone().String() != "Asia/Shanghai" {
		t.Fatalf("补登只作用于此后：旧修订该仍没登记、新修订该是 Asia/Shanghai")
	}
}

// Covers: AT-WF-009（仓的部分）——登记仓不以已有仓为前提：一个仓都没登记的租户，登记第一个仓、第二个仓都照常受理，各自从修订 1 起。
func TestRegisterWarehouseDoesNotPresupposeAnExistingWarehouse(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)

	for _, code := range []string{"SYN-WH-01", "SYN-WH-02"} {
		command := registerCommand(t, 1, nil)
		command.Code = valueOf(t, domain.NewWarehouseCode, code)
		expectOutcome(t, "登记 "+code, mustRegister(t, handler, command), application.WarehouseRegistered)
	}
	if len(registry.rows) != 2 {
		t.Fatalf("册里有 %d 个仓，want 2", len(registry.rows))
	}
}

// Covers: 停用与再启用各是一笔新修订，都答 REGISTERED；停用不拦此后的修订，旧修订照留。
func TestRegisterWarehouseDeactivationAndReactivationAreNewRevisions(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)
	expectOutcome(t, "停用", mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	})), application.WarehouseRegistered)
	expectOutcome(t, "停用期间改名", mustRegister(t, handler, registerCommand(t, 3, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（停用中）")
	})), application.WarehouseRegistered)
	expectOutcome(t, "再启用", mustRegister(t, handler, registerCommand(t, 4, nil)), application.WarehouseRegistered)

	rows := registry.revisionsOf("SYN-WH-01")
	if len(rows) != 4 || rows[0].Content().Deactivated() || !rows[1].Content().Deactivated() ||
		!rows[2].Content().Deactivated() || rows[3].Content().Deactivated() {
		t.Fatalf("停用与再启用之后的册不对：四笔的停用标记该依次是 否、是、是、否")
	}
}

// Covers: 重放只比内容，不比登记责任方与登记时刻——换一个登记责任方、晚一些再投仍答 ALREADY_REGISTERED，在册那一笔的登记责任方
// 与登记时刻不变，也不再保存。
func TestRegisterWarehouseReplayByAnotherRegistrantKeepsTheOriginalRegistrant(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	clock := &fixedClock{at: registrationInstant}
	handler := application.NewRegisterWarehouseHandler(registry, clock)
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)

	clock.at = registrationInstant.Add(72 * time.Hour)
	another := registerCommand(t, 1, nil)
	another.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	replay := mustRegister(t, handler, another)
	expectOutcome(t, "换登记责任方重放", replay, application.WarehouseAlreadyRegistered)

	rows := registry.revisionsOf("SYN-WH-01")
	if len(rows) != 1 || rows[0].Registrant().String() != "operator-1" || !rows[0].RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("重放之后在册的登记责任方 / 登记时刻变了：%q %v", rows[0].Registrant(), rows[0].RegisteredAt())
	}
	if registry.saves != 1 {
		t.Fatalf("重放保存了 %d 次，want 只有首笔那 1 次", registry.saves)
	}
	echoed, has := replay.Revision()
	if !has || echoed.Registrant().String() != "operator-1" {
		t.Fatalf("重放回显的登记责任方 = %q（%v），want 在册那一笔的 operator-1", echoed.Registrant(), has)
	}
}

// Covers: 重放与冲突要找的是任何一笔在册的修订，不只是最新那一笔——在册有 1、2，原样重投 1 答 ALREADY_REGISTERED，
// 对 1 换内容答 CONTENT_CONFLICT；两者都不写册。
func TestRegisterWarehouseJudgesReplayAgainstAnyRevisionOnTheRegister(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)
	expectOutcome(t, "修订 2", mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（二）")
	})), application.WarehouseRegistered)

	expectOutcome(t, "重投旧修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseAlreadyRegistered)
	expectOutcome(t, "对旧修订 1 换内容", mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "另一个名字")
	})), application.WarehouseContentConflict)
	if registry.saves != 2 || len(registry.revisionsOf("SYN-WH-01")) != 2 {
		t.Fatalf("保存 %d 次、落册 %d 笔，want 2 次、2 笔", registry.saves, len(registry.revisionsOf("SYN-WH-01")))
	}
}

// 判定次序：先按（身份，修订号）找在册的那一笔，重放与冲突不再过后面的核。册上早就有的修订，哪怕它的国家后来不在目录里
// （目录改版）、种类与最新修订对不上，原样重投仍是重放，换内容仍是冲突，不能变成「内容不合」。
func TestRegisterWarehouseJudgesReplayAndConflictBeforeSequenceKindAndCatalogues(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	retiredCountry := func(spec *domain.WarehouseContentSpec) {
		spec.Address = domain.NewWarehouseAddress(valueOf(t, domain.NewCountryCode, "ZZ"), spec.Address.Text())
		spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, "Mars/Olympus_Mons")
	}
	seeded := registerCommand(t, 1, retiredCountry)
	revision, err := domain.NewWarehouseRevision(seeded.Tenant, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	registry.seed(revision)

	expectOutcome(t, "重放目录外国家与认不出时区的在册修订", mustRegister(t, handler, registerCommand(t, 1, retiredCountry)), application.WarehouseAlreadyRegistered)
	expectOutcome(t, "对它换内容", mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
		retiredCountry(spec)
		spec.Kind = domain.WarehouseKindAgent
	})), application.WarehouseContentConflict)
	if registry.saves != 0 {
		t.Fatalf("重放与冲突保存了 %d 次，want 0", registry.saves)
	}
}

// 未落册的修订依次核：修订号连续 → 种类没有改 → 国家在内置目录 → 业务时区认得出。前面一格不成立就答前面那一格，
// 不继续往后核。
func TestRegisterWarehouseChecksSequenceThenKindThenCountryThenTimeZone(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	expectOutcome(t, "修订 1", mustRegister(t, handler, registerCommand(t, 1, nil)), application.WarehouseRegistered)

	allWrong := func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
		spec.Address = domain.NewWarehouseAddress(valueOf(t, domain.NewCountryCode, "ZZ"), spec.Address.Text())
		spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, "Mars/Olympus_Mons")
	}
	expectRefusal(t, "跳号且各格皆错", mustRegister(t, handler, registerCommand(t, 3, allWrong)),
		application.WarehouseRefusalRevisionOutOfSequence, domain.WarehouseFieldInvalid)
	expectRefusal(t, "改种类且国家、时区皆错", mustRegister(t, handler, registerCommand(t, 2, allWrong)),
		application.WarehouseRefusalImmutableField, domain.WarehouseFieldKind)
	expectRefusal(t, "国家与时区皆错", mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		allWrong(spec)
		spec.Kind = domain.WarehouseKindSelfOperated
	})), application.WarehouseRefusalContentInvalid, domain.WarehouseFieldAddressCountry)
	expectRefusal(t, "只有时区错", mustRegister(t, handler, registerCommand(t, 2, func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, "Mars/Olympus_Mons")
	})), application.WarehouseRefusalContentInvalid, domain.WarehouseFieldBusinessTimeZone)
	if registry.saves != 1 {
		t.Fatalf("各种拒绝之后保存 %d 次，want 只有首笔那 1 次", registry.saves)
	}
}

// 输入被拒是已形成的答案，不是技术失败：以上各格都经 mustRegister 断言 error 为空。这里证反面——册读写不动才返回 error，
// 并且不折成任何一格业务结果（折成「从未登记」会让登记方从修订 1 重登一个本来就在册的仓）。
func TestRegisterWarehouseReturnsAnErrorOnlyWhenTheRegisterCannotBeReadOrWritten(t *testing.T) {
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*fakeWarehouseRegistry){
		"按修订号读不动":  func(registry *fakeWarehouseRegistry) { registry.loadErr = boom },
		"读最新修订读不动": func(registry *fakeWarehouseRegistry) { registry.loadLatestErr = boom },
		"保存不动":     func(registry *fakeWarehouseRegistry) { registry.saveErr = boom },
		"保存答出集外的落点": func(registry *fakeWarehouseRegistry) {
			registry.saveOutcome = ports.WarehouseSaveOutcome(99)
		},
	} {
		registry := newFakeWarehouseRegistry()
		arrange(registry)
		result, err := newHandler(registry).Register(t.Context(), registerCommand(t, 1, nil))
		if err == nil {
			t.Errorf("%s：err 为空，结果 = %s", name, result.Outcome())
		}
		if result.Outcome() != application.RegisterWarehouseOutcomeInvalid {
			t.Errorf("%s：结果 = %s，error 之外不该另带一格业务结果", name, result.Outcome())
		}
	}
}

// 登记口拒绝的是内容；身份与登记责任方没经构造器（零值）是接入侧装配错了，不是登记方的输入，按技术失败上抛。
// 内容里缺格（零值内容）仍是内容不合并指出缺哪一格。
func TestRegisterWarehouseTreatsUnconstructedIdentityAsAnErrorAndUnconstructedContentAsInvalid(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)

	noTenant := registerCommand(t, 1, nil)
	noTenant.Tenant = domain.TenantID{}
	noRegistrant := registerCommand(t, 1, nil)
	noRegistrant.Registrant = domain.Registrant{}
	for name, command := range map[string]application.RegisterWarehouseCommand{"缺租户": noTenant, "缺登记责任方": noRegistrant} {
		if _, err := handler.Register(t.Context(), command); err == nil {
			t.Errorf("%s：应按技术失败上抛", name)
		}
	}

	noContent := registerCommand(t, 1, nil)
	noContent.Content = domain.WarehouseContent{}
	expectRefusal(t, "零值内容", mustRegister(t, handler, noContent),
		application.WarehouseRefusalContentInvalid, domain.WarehouseFieldName)
	if registry.saves != 0 {
		t.Fatalf("保存了 %d 次，want 0", registry.saves)
	}
}

// 登记时刻取自注入的时钟，一次登记只读一次；回显的修订带它。
func TestRegisterWarehouseStampsTheRegistrationInstantFromTheClock(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	result := mustRegister(t, newHandler(registry), registerCommand(t, 1, nil))
	landed, has := result.Revision()
	if !has || !landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("登记时刻 = %v（%v），want 时钟给的 %v", landed.RegisteredAt(), has, registrationInstant.Truncate(time.Microsecond))
	}
	if stored := registry.revisionsOf("SYN-WH-01")[0]; !stored.RegisteredAt().Equal(landed.RegisteredAt()) {
		t.Fatalf("落册的登记时刻 %v 与回显的 %v 不一致", stored.RegisteredAt(), landed.RegisteredAt())
	}
}

// 结果的各个字面量是对外的词（外半据它译装），改了就是改了对外的契约。
func TestRegisterWarehouseResultLiterals(t *testing.T) {
	for outcome, literal := range map[application.RegisterWarehouseOutcome]string{
		application.WarehouseRegistered:        "REGISTERED",
		application.WarehouseAlreadyRegistered: "ALREADY_REGISTERED",
		application.WarehouseContentConflict:   "CONTENT_CONFLICT",
		application.WarehouseNotAccepted:       "NOT_ACCEPTED",
	} {
		if outcome.String() != literal {
			t.Errorf("结果字面量 = %q，want %q", outcome.String(), literal)
		}
	}
	for refusal, literal := range map[application.RegisterWarehouseRefusal]string{
		application.WarehouseRefusalRevisionOutOfSequence: "REVISION_OUT_OF_SEQUENCE",
		application.WarehouseRefusalImmutableField:        "IMMUTABLE_FIELD",
		application.WarehouseRefusalContentInvalid:        "CONTENT_INVALID",
	} {
		if refusal.String() != literal {
			t.Errorf("拒绝理由字面量 = %q，want %q", refusal.String(), literal)
		}
	}
	if application.RegisterWarehouseOutcomeInvalid.String() != "" || application.WarehouseRefusalNone.String() != "" {
		t.Errorf("零值的字面量该是空串")
	}
}

// 拒绝理由只在 NOT_ACCEPTED 时有值，回显的修订只在 REGISTERED 与 ALREADY_REGISTERED 时有：别的结果上读它们得零值。
func TestRegisterWarehouseResultCarriesDetailOnlyWhereItBelongs(t *testing.T) {
	registry := newFakeWarehouseRegistry()
	handler := newHandler(registry)
	registered := mustRegister(t, handler, registerCommand(t, 1, nil))
	if registered.Refusal() != application.WarehouseRefusalNone || registered.Field() != domain.WarehouseFieldInvalid || registered.LatestRevision() != 0 {
		t.Errorf("REGISTERED 带了拒绝细节：%s %q %d", registered.Refusal(), registered.Field(), registered.LatestRevision())
	}
	conflict := mustRegister(t, handler, registerCommand(t, 1, func(spec *domain.WarehouseContentSpec) {
		spec.Name = valueOf(t, domain.NewWarehouseName, "冲突的名字")
	}))
	if _, has := conflict.Revision(); has || conflict.Refusal() != application.WarehouseRefusalNone {
		t.Errorf("CONTENT_CONFLICT 不该带回显的修订或拒绝理由")
	}
	refused := mustRegister(t, handler, registerCommand(t, 5, nil))
	if _, has := refused.Revision(); has || !strings.Contains(refused.Refusal().String(), "SEQUENCE") {
		t.Errorf("NOT_ACCEPTED 不该带回显的修订")
	}
}
