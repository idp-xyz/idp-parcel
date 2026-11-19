package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证库区登记用例（UC-WF-001 的库区修订）：判定次序、结果代数、指名的仓须已登记与并发。库区册与仓册用内存替身，真库上的
// 重放、冲突与并发另在 adapters/postgres 的用例里证。租户、仓与库区都是隔离合成值，只记 `S`。

// fakeZoneRegistry 是库区登记册的内存替身：同键同内容答已在册、同键异内容答冲突，与真适配器同判据。保存在锁里串行，等于主键
// 唯一约束；afterLookup 与 beforeSave 让并发用例逼出真实的竞态窗口。
type fakeZoneRegistry struct {
	mutex         sync.Mutex
	rows          map[string][]domain.ZoneRevision
	saves         int
	loadErr       error
	loadLatestErr error
	saveErr       error
	saveOutcome   ports.RegistrySaveOutcome
	afterLookup   func()
	beforeSave    func()
}

func newFakeZoneRegistry() *fakeZoneRegistry {
	return &fakeZoneRegistry{rows: make(map[string][]domain.ZoneRevision)}
}

func zoneKey(tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode) string {
	return tenant.String() + "/" + warehouse.String() + "/" + code.String()
}

func (fake *fakeZoneRegistry) LoadZoneRevision(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
	revision int,
) (domain.ZoneRevision, bool, error) {
	if fake.loadErr != nil {
		return domain.ZoneRevision{}, false, fake.loadErr
	}
	fake.mutex.Lock()
	var found domain.ZoneRevision
	var has bool
	for _, row := range fake.rows[zoneKey(tenant, warehouse, code)] {
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

func (fake *fakeZoneRegistry) LoadLatestZone(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.ZoneRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var latest domain.ZoneRevision
	var has bool
	for _, row := range fake.rows[zoneKey(tenant, warehouse, code)] {
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has, nil
}

func (fake *fakeZoneRegistry) SaveZoneRevision(
	_ context.Context,
	revision domain.ZoneRevision,
) (ports.RegistrySaveOutcome, error) {
	if fake.beforeSave != nil {
		fake.beforeSave()
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.saves++
	if fake.saveErr != nil {
		return ports.RegistrySaveOutcomeInvalid, fake.saveErr
	}
	if fake.saveOutcome != ports.RegistrySaveOutcomeInvalid {
		return fake.saveOutcome, nil
	}
	key := zoneKey(revision.Tenant(), revision.Warehouse(), revision.Code())
	for _, existing := range fake.rows[key] {
		if existing.Revision() != revision.Revision() {
			continue
		}
		if existing.SameContentAs(revision) {
			return ports.RegistryRevisionAlreadyRegistered, nil
		}
		return ports.RegistryRevisionContentConflict, nil
	}
	fake.rows[key] = append(fake.rows[key], revision)
	return ports.RegistryRevisionSaved, nil
}

// seed 直接把一笔修订放进册，演「册上早就有」。
func (fake *fakeZoneRegistry) seed(revision domain.ZoneRevision) {
	key := zoneKey(revision.Tenant(), revision.Warehouse(), revision.Code())
	fake.rows[key] = append(fake.rows[key], revision)
}

func (fake *fakeZoneRegistry) revisionsOf(zone string) []domain.ZoneRevision {
	return fake.rows["SYN-TENANT-01/SYN-WH-01/"+zone]
}

// fakeWarehouseLookup 是「仓已登记」的内存替身：按（租户，仓编码）记在册的仓，在册的那一笔修订可以是停用的。并发用例里两个
// goroutine 同时读它，所以 asked 用原子计数。
type fakeWarehouseLookup struct {
	registered  map[string]bool
	deactivated map[string]bool
	err         error
	asked       atomic.Int32
}

func newFakeWarehouseLookup(tenant string, warehouses ...string) *fakeWarehouseLookup {
	lookup := &fakeWarehouseLookup{registered: map[string]bool{}, deactivated: map[string]bool{}}
	for _, code := range warehouses {
		lookup.registered[tenant+"/"+code] = true
	}
	return lookup
}

func mustBuild[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// syntheticWarehouseRevision 造仓册里的一笔合成修订：库区登记只问仓在不在册，不看它的内容，这里给齐各格即可。
func syntheticWarehouseRevision(tenant, code string, deactivated bool) domain.WarehouseRevision {
	content := mustBuild(domain.NewWarehouseContent(domain.WarehouseContentSpec{
		Name: mustBuild(domain.NewWarehouseName("合成仓")),
		Kind: domain.WarehouseKindSelfOperated,
		Address: domain.NewWarehouseAddress(
			mustBuild(domain.NewCountryCode("CN")), mustBuild(domain.NewAddressText("合成路 1 号"))),
		Deactivated: deactivated,
	}))
	return mustBuild(domain.NewWarehouseRevision(
		mustBuild(domain.NewTenantID(tenant)), mustBuild(domain.NewWarehouseCode(code)), 1, content,
		mustBuild(domain.NewRegistrant("operator-w")), registrationInstant))
}

func (fake *fakeWarehouseLookup) LoadLatestWarehouse(
	_ context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	fake.asked.Add(1)
	if fake.err != nil {
		return domain.WarehouseRevision{}, false, fake.err
	}
	key := tenant.String() + "/" + code.String()
	if !fake.registered[key] {
		return domain.WarehouseRevision{}, false, nil
	}
	return syntheticWarehouseRevision(tenant.String(), code.String(), fake.deactivated[key]), true, nil
}

func zoneContentSpec(t *testing.T) domain.ZoneContentSpec {
	t.Helper()
	return domain.ZoneContentSpec{
		Name:  valueOf(t, domain.NewZoneName, "合成存储区"),
		Usage: domain.ZoneUsageStorage,
	}
}

func zoneContentWith(t *testing.T, edit func(*domain.ZoneContentSpec)) domain.ZoneContent {
	t.Helper()
	spec := zoneContentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewZoneContent(spec)
	if err != nil {
		t.Fatalf("NewZoneContent: %v", err)
	}
	return content
}

func registerZoneCommand(t *testing.T, revision int, edit func(*domain.ZoneContentSpec)) application.RegisterZoneCommand {
	t.Helper()
	return application.RegisterZoneCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Warehouse:  valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Code:       valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"),
		Revision:   revision,
		Content:    zoneContentWith(t, edit),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

// zoneStores 是一组登记用例的替身：仓 SYN-WH-01 已登记。
func zoneStores() (*fakeZoneRegistry, *fakeWarehouseLookup) {
	return newFakeZoneRegistry(), newFakeWarehouseLookup("SYN-TENANT-01", "SYN-WH-01")
}

func newZoneHandler(zones *fakeZoneRegistry, warehouses *fakeWarehouseLookup) *application.RegisterZoneHandler {
	return application.NewRegisterZoneHandler(zones, warehouses, &fixedClock{at: registrationInstant})
}

func mustRegisterZone(
	t *testing.T,
	handler *application.RegisterZoneHandler,
	command application.RegisterZoneCommand,
) application.RegisterZoneResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register 修订 %d：业务格不该走 error：%v", command.Revision, err)
	}
	return result
}

func expectZoneOutcome(t *testing.T, label string, result application.RegisterZoneResult, want application.RegistrationOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s（拒绝理由 %s，指向 %s），want %s", label, result.Outcome(), result.Refusal(), result.Field(), want)
	}
}

func expectZoneRefusal(
	t *testing.T,
	label string,
	result application.RegisterZoneResult,
	want application.RegistrationRefusal,
	wantField domain.ZoneField,
) {
	t.Helper()
	expectZoneOutcome(t, label, result, application.RegistrationNotAccepted)
	if result.Refusal() != want || result.Field() != wantField {
		t.Fatalf("%s：拒绝理由 = %s 指向 %q，want %s 指向 %q", label, result.Refusal(), result.Field(), want, wantField)
	}
}

// Covers: AT-WF-001（库区的部分）——首次登记答 REGISTERED 且修订 1 落册；原样重投答 ALREADY_REGISTERED、册里仍只有一份；同一修订号
// 换内容答 CONTENT_CONFLICT、在册不变；首笔修订号给 2 答 NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 并回显 0；已有修订 1 时给 3 同样答
// 错位并回显 1。
func TestRegisterZoneFirstRevisionReplayConflictAndOutOfSequence(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)

	first := mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil))
	expectZoneOutcome(t, "首次登记", first, application.RegistrationRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("回显的修订 = %+v（%v），want 修订 1、登记责任方 operator-1、时钟给的登记时刻", landed, has)
	}
	if len(zones.revisionsOf("SYN-ZONE-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(zones.revisionsOf("SYN-ZONE-01")))
	}

	replay := mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil))
	expectZoneOutcome(t, "原样重投", replay, application.RegistrationAlreadyRegistered)
	if zones.saves != 1 || len(zones.revisionsOf("SYN-ZONE-01")) != 1 {
		t.Fatalf("重放之后保存 %d 次、落册 %d 笔，want 1 次、1 笔", zones.saves, len(zones.revisionsOf("SYN-ZONE-01")))
	}

	conflict := mustRegisterZone(t, handler, registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（改名）")
	}))
	expectZoneOutcome(t, "同一修订号换内容", conflict, application.RegistrationContentConflict)
	if got := zones.revisionsOf("SYN-ZONE-01")[0].Content().Name().String(); got != "合成存储区" {
		t.Fatalf("冲突之后在册的名称 = %q，在册的不该变", got)
	}

	skipped := registerZoneCommand(t, 2, nil)
	skipped.Code = valueOf(t, domain.NewZoneCode, "SYN-ZONE-02")
	result := mustRegisterZone(t, handler, skipped)
	expectZoneRefusal(t, "首笔给 2", result, application.RegistrationRefusalRevisionOutOfSequence, domain.ZoneFieldInvalid)
	if result.LatestRevision() != 0 || len(zones.revisionsOf("SYN-ZONE-02")) != 0 {
		t.Fatalf("首笔错位回显 %d、落册 %d 笔，want 回显 0、不落册", result.LatestRevision(), len(zones.revisionsOf("SYN-ZONE-02")))
	}

	result = mustRegisterZone(t, handler, registerZoneCommand(t, 3, nil))
	expectZoneRefusal(t, "已有修订 1 时给 3", result, application.RegistrationRefusalRevisionOutOfSequence, domain.ZoneFieldInvalid)
	if result.LatestRevision() != 1 || len(zones.revisionsOf("SYN-ZONE-01")) != 1 {
		t.Fatalf("跳号回显 %d、落册 %d 笔，want 回显 1、仍是 1 笔", result.LatestRevision(), len(zones.revisionsOf("SYN-ZONE-01")))
	}
}

// 修订号小于 1 不是「最新加一」：与跳号同答错位并回显在册最新修订号，不另造一格。
func TestRegisterZoneTreatsANonPositiveRevisionNumberAsOutOfSequence(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)

	for _, number := range []int{0, -1} {
		result := mustRegisterZone(t, handler, registerZoneCommand(t, number, nil))
		expectZoneRefusal(t, fmt.Sprintf("空册给 %d", number), result, application.RegistrationRefusalRevisionOutOfSequence, domain.ZoneFieldInvalid)
		if result.LatestRevision() != 0 {
			t.Fatalf("空册给 %d 回显 %d，want 0", number, result.LatestRevision())
		}
	}
	expectZoneOutcome(t, "修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)
	result := mustRegisterZone(t, handler, registerZoneCommand(t, 0, nil))
	expectZoneRefusal(t, "已有修订 1 时给 0", result, application.RegistrationRefusalRevisionOutOfSequence, domain.ZoneFieldInvalid)
	if result.LatestRevision() != 1 || zones.saves != 1 {
		t.Fatalf("回显 %d、保存 %d 次，want 回显 1、保存 1 次", result.LatestRevision(), zones.saves)
	}
}

// Covers: AT-WF-001（库区的部分）最后一行——并发提交同一修订号、内容相同的两笔，一笔答 REGISTERED、一笔答 ALREADY_REGISTERED；
// 册里只有一份，答重放的那一笔回显的是已落册的那一份（登记责任方是先落册者的）。内容不同时一笔落册、另一笔答 CONTENT_CONFLICT。
// 两笔都停在「已查过在册」之后再各自保存，逼的是真实的竞态窗口。
func TestRegisterZoneConcurrentSubmissionsOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		wantLoser  application.RegistrationOutcome
	}{
		"内容相同": {secondName: "合成存储区", wantLoser: application.RegistrationAlreadyRegistered},
		"内容不同": {secondName: "合成存储区（另一份）", wantLoser: application.RegistrationContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			zones, warehouses := zoneStores()
			meeting := newRendezvous(2)
			zones.afterLookup = meeting.arrive
			handler := newZoneHandler(zones, warehouses)

			commands := []application.RegisterZoneCommand{
				registerZoneCommand(t, 1, nil),
				registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
					spec.Name = valueOf(t, domain.NewZoneName, test.secondName)
				}),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")

			results := make([]application.RegisterZoneResult, len(commands))
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
			if !meeting.met {
				t.Fatalf("两笔没有同时停在查册之后，没有覆盖竞态窗口")
			}
			landed, others := 0, []application.RegisterZoneResult{}
			for _, result := range results {
				if result.Outcome() == application.RegistrationRegistered {
					landed++
				} else {
					others = append(others, result)
				}
			}
			if landed != 1 || len(others) != 1 || others[0].Outcome() != test.wantLoser {
				t.Fatalf("两笔的结果 = %s、%s，want 一笔 REGISTERED、一笔 %s", results[0].Outcome(), results[1].Outcome(), test.wantLoser)
			}
			rows := zones.revisionsOf("SYN-ZONE-01")
			if len(rows) != 1 {
				t.Fatalf("落册 %d 笔，want 至多 1 笔", len(rows))
			}
			if test.wantLoser == application.RegistrationAlreadyRegistered {
				echoed, has := others[0].Revision()
				if !has || echoed.Registrant() != rows[0].Registrant() {
					t.Fatalf("答重放的一笔回显的登记责任方 = %q（%v），want 已落册那一份的 %q", echoed.Registrant(), has, rows[0].Registrant())
				}
			}
		})
	}
}

// 竞态的两个窗口各演一遍，不靠调度碰运气：别的事务在「本事务查过册（没有）」之后，或在「本事务读过最新修订」之后、保存之前，
// 把同一修订号落了册。前一个窗口里，本事务随后读到的最新修订号已不小于自己的——不能当成跳号答错位，要回去再按内容答重放或冲突；
// 后一个窗口里靠登记册的主键，保存按内容答已在册或冲突。两个窗口的答案必须一样（仓那一册里处理过的同一个竞态，库区同样要守）。
func TestRegisterZoneJudgesAConcurrentLandingByContentInBothRaceWindows(t *testing.T) {
	for windowName, install := range map[string]func(*fakeZoneRegistry, func()){
		"查册之后": func(zones *fakeZoneRegistry, land func()) { zones.afterLookup = land },
		"保存之前": func(zones *fakeZoneRegistry, land func()) { zones.beforeSave = land },
	} {
		for contentName, test := range map[string]struct {
			competitorName string
			want           application.RegistrationOutcome
		}{
			"内容相同": {"合成存储区", application.RegistrationAlreadyRegistered},
			"内容不同": {"合成存储区（别人先落的）", application.RegistrationContentConflict},
		} {
			t.Run(windowName+"/"+contentName, func(t *testing.T) {
				zones, warehouses := zoneStores()
				competitorCommand := registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
					spec.Name = valueOf(t, domain.NewZoneName, test.competitorName)
				})
				competitorCommand.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
				competitor, err := domain.NewZoneRevision(
					competitorCommand.Tenant, competitorCommand.Warehouse, competitorCommand.Code, 1, competitorCommand.Content,
					competitorCommand.Registrant, registrationInstant.Add(-time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				landed := false
				install(zones, func() {
					if !landed {
						landed = true
						zones.seed(competitor)
					}
				})

				result := mustRegisterZone(t, newZoneHandler(zones, warehouses), registerZoneCommand(t, 1, nil))
				expectZoneOutcome(t, "别人先落册之后的这一笔", result, test.want)
				if !landed || len(zones.revisionsOf("SYN-ZONE-01")) != 1 {
					t.Fatalf("竞争者落册 %v、册里 %d 笔，want 已落、只有 1 笔", landed, len(zones.revisionsOf("SYN-ZONE-01")))
				}
				if test.want == application.RegistrationAlreadyRegistered {
					echoed, has := result.Revision()
					if !has || echoed.Registrant().String() != "operator-2" {
						t.Fatalf("回显的登记责任方 = %q（%v），want 先落册那一笔的 operator-2", echoed.Registrant(), has)
					}
				}
			})
		}
	}
}

// Covers: AT-WF-002（库区的部分）——新修订改库区用途答 NOT_ACCEPTED/IMMUTABLE_FIELD 并指出 usage，册不变；只改名称答 REGISTERED，
// 修订号加一，旧修订照留；改完名称之后再改用途仍被拒（拿最新那一笔比，与最新一笔的用途一致才受理）。
func TestRegisterZoneKeepsTheUsageAndLetsTheNameChange(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)
	expectZoneOutcome(t, "修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)

	for _, usage := range []domain.ZoneUsage{
		domain.ZoneUsageReceivingStaging, domain.ZoneUsagePicking, domain.ZoneUsageDefective,
		domain.ZoneUsageReturnStaging, domain.ZoneUsageOutboundStaging,
	} {
		result := mustRegisterZone(t, handler, registerZoneCommand(t, 2, func(spec *domain.ZoneContentSpec) { spec.Usage = usage }))
		expectZoneRefusal(t, "存储区改成 "+usage.String(), result, application.RegistrationRefusalImmutableField, domain.ZoneFieldUsage)
		if _, has := result.Revision(); has {
			t.Fatalf("被拒的结果不该带回显的修订")
		}
	}
	if len(zones.revisionsOf("SYN-ZONE-01")) != 1 || zones.saves != 1 {
		t.Fatalf("被拒之后落册 %d 笔、保存 %d 次，want 册不变", len(zones.revisionsOf("SYN-ZONE-01")), zones.saves)
	}

	renamed := mustRegisterZone(t, handler, registerZoneCommand(t, 2, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（新名）")
	}))
	expectZoneOutcome(t, "只改名称", renamed, application.RegistrationRegistered)
	rows := zones.revisionsOf("SYN-ZONE-01")
	if len(rows) != 2 || rows[0].Content().Name().String() != "合成存储区" || rows[1].Revision() != 2 ||
		rows[1].Content().Name().String() != "合成存储区（新名）" || rows[1].Content().Usage() != domain.ZoneUsageStorage {
		t.Fatalf("改名之后的册 = %d 笔，旧修订该照留、新修订占修订号 2", len(rows))
	}

	result := mustRegisterZone(t, handler, registerZoneCommand(t, 3, func(spec *domain.ZoneContentSpec) { spec.Usage = domain.ZoneUsagePicking }))
	expectZoneRefusal(t, "改名之后再改用途", result, application.RegistrationRefusalImmutableField, domain.ZoneFieldUsage)
}

// Covers: AT-WF-003（库区的部分）——指名没登记的仓答 NOT_ACCEPTED/REFERENCE_NOT_FOUND 并指出 warehouseCode，不落册、不代建仓；
// 指名别的租户的仓编码同答：仓册按租户查，本租户查不到就是查不到，答复里没有别的租户有没有这个仓的任何痕迹。
func TestRegisterZoneRefusesAWarehouseThatIsNotRegisteredForThisTenant(t *testing.T) {
	zones := newFakeZoneRegistry()
	// SYN-WH-OTHER 只在别的租户名下登记过。
	warehouses := newFakeWarehouseLookup("SYN-TENANT-02", "SYN-WH-OTHER")
	handler := newZoneHandler(zones, warehouses)

	var answers []application.RegisterZoneResult
	for _, code := range []string{"SYN-WH-NEVER", "SYN-WH-OTHER"} {
		command := registerZoneCommand(t, 1, nil)
		command.Warehouse = valueOf(t, domain.NewWarehouseCode, code)
		result := mustRegisterZone(t, handler, command)
		expectZoneRefusal(t, "指名 "+code, result, application.RegistrationRefusalReferenceNotFound, domain.ZoneFieldWarehouseCode)
		if _, has := result.Revision(); has || result.LatestRevision() != 0 {
			t.Fatalf("指名 %s 的答复带了多余的细节", code)
		}
		answers = append(answers, result)
	}
	if answers[0] != answers[1] {
		t.Fatalf("别的租户的仓与没登记过的仓答复不同：%+v 与 %+v，泄露了是否存在", answers[0], answers[1])
	}
	if zones.saves != 0 || len(zones.rows) != 0 {
		t.Fatalf("被拒之后保存 %d 次、册里 %d 个库区，want 不落册", zones.saves, len(zones.rows))
	}
}

// Covers: AT-WF-009（库区的部分）——一个仓都没登记的租户：登记库区答 NOT_ACCEPTED/REFERENCE_NOT_FOUND 并指出仓那一格，不落册；
// 之后登记了仓，同一修订原样重交照常受理（那一笔当初没有落册，重交不是重放）。
func TestRegisterZoneBeforeAnyWarehouseIsRegisteredIsRefusedAndTheSameRevisionLandsAfterwards(t *testing.T) {
	zones := newFakeZoneRegistry()
	warehouses := newFakeWarehouseLookup("SYN-TENANT-01")
	handler := newZoneHandler(zones, warehouses)

	result := mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil))
	expectZoneRefusal(t, "一个仓都没登记", result, application.RegistrationRefusalReferenceNotFound, domain.ZoneFieldWarehouseCode)
	if zones.saves != 0 {
		t.Fatalf("被拒之后保存了 %d 次，want 0", zones.saves)
	}

	warehouses.registered["SYN-TENANT-01/SYN-WH-01"] = true
	expectZoneOutcome(t, "登记仓之后原样重交", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)
}

// Covers: AT-WF-003 与 CONTEXT「主数据登记」——停用的仓也算已登记：仓的停用只挡此后新受理的请求与新下发的选择，不挡在它名下登记
// 库区。仓册里读到的最新那一笔修订是停用的，库区照常受理，修订 1 与后续修订都一样。
func TestRegisterZoneUnderADeactivatedWarehouseIsAccepted(t *testing.T) {
	zones, warehouses := zoneStores()
	warehouses.deactivated["SYN-TENANT-01/SYN-WH-01"] = true
	handler := newZoneHandler(zones, warehouses)

	expectZoneOutcome(t, "停用的仓下登记库区", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)
	expectZoneOutcome(t, "停用的仓下改库区名称", mustRegisterZone(t, handler, registerZoneCommand(t, 2, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（仓停用中）")
	})), application.RegistrationRegistered)
	if warehouses.asked.Load() == 0 {
		t.Fatalf("没有向仓册问过仓在不在册")
	}
}

// 库区编码在仓内才唯一：同一个库区编码在两个仓下是两个互相独立的库区，各自从修订 1 起；同一个仓、同一个编码在另一个租户名下
// 也是另一个库区。
func TestRegisterZoneKeepsZonesOfDifferentWarehousesAndTenantsApart(t *testing.T) {
	zones := newFakeZoneRegistry()
	warehouses := newFakeWarehouseLookup("SYN-TENANT-01", "SYN-WH-01", "SYN-WH-02")
	warehouses.registered["SYN-TENANT-02/SYN-WH-01"] = true
	handler := newZoneHandler(zones, warehouses)

	expectZoneOutcome(t, "一号仓的库区", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)

	second := registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) { spec.Usage = domain.ZoneUsagePicking })
	second.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-02")
	expectZoneOutcome(t, "二号仓下同编码的库区（用途不同也不冲突）", mustRegisterZone(t, handler, second), application.RegistrationRegistered)

	other := registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) { spec.Usage = domain.ZoneUsageDefective })
	other.Tenant = valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	expectZoneOutcome(t, "另一个租户同仓同编码的库区", mustRegisterZone(t, handler, other), application.RegistrationRegistered)

	if len(zones.rows) != 3 {
		t.Fatalf("册里有 %d 个库区，want 3 个互相独立的库区", len(zones.rows))
	}
}

// Covers: 停用与再启用各是一笔新修订，都答 REGISTERED；停用不拦此后的修订（停用期间照常改名），旧修订照留；用途在停用与再启用
// 之后仍不可改。
func TestRegisterZoneDeactivationAndReactivationAreNewRevisions(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)
	expectZoneOutcome(t, "修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)
	expectZoneOutcome(t, "停用", mustRegisterZone(t, handler, registerZoneCommand(t, 2, func(spec *domain.ZoneContentSpec) {
		spec.Deactivated = true
	})), application.RegistrationRegistered)
	expectZoneOutcome(t, "停用期间改名", mustRegisterZone(t, handler, registerZoneCommand(t, 3, func(spec *domain.ZoneContentSpec) {
		spec.Deactivated = true
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（停用中）")
	})), application.RegistrationRegistered)
	expectZoneRefusal(t, "停用期间改用途", mustRegisterZone(t, handler, registerZoneCommand(t, 4, func(spec *domain.ZoneContentSpec) {
		spec.Deactivated = true
		spec.Usage = domain.ZoneUsagePicking
	})), application.RegistrationRefusalImmutableField, domain.ZoneFieldUsage)
	expectZoneOutcome(t, "再启用", mustRegisterZone(t, handler, registerZoneCommand(t, 4, nil)), application.RegistrationRegistered)

	rows := zones.revisionsOf("SYN-ZONE-01")
	if len(rows) != 4 || rows[0].Content().Deactivated() || !rows[1].Content().Deactivated() ||
		!rows[2].Content().Deactivated() || rows[3].Content().Deactivated() {
		t.Fatalf("停用与再启用之后的册不对：四笔的停用标记该依次是 否、是、是、否")
	}
}

// Covers: 重放只比内容，不比登记责任方与登记时刻——换一个登记责任方、晚一些再投仍答 ALREADY_REGISTERED，在册那一笔的登记责任方
// 与登记时刻不变，也不再保存。
func TestRegisterZoneReplayByAnotherRegistrantKeepsTheOriginalRegistrant(t *testing.T) {
	zones, warehouses := zoneStores()
	clock := &fixedClock{at: registrationInstant}
	handler := application.NewRegisterZoneHandler(zones, warehouses, clock)
	expectZoneOutcome(t, "修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)

	clock.at = registrationInstant.Add(72 * time.Hour)
	another := registerZoneCommand(t, 1, nil)
	another.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	replay := mustRegisterZone(t, handler, another)
	expectZoneOutcome(t, "换登记责任方重放", replay, application.RegistrationAlreadyRegistered)

	rows := zones.revisionsOf("SYN-ZONE-01")
	if len(rows) != 1 || rows[0].Registrant().String() != "operator-1" || !rows[0].RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("重放之后在册的登记责任方 / 登记时刻变了：%q %v", rows[0].Registrant(), rows[0].RegisteredAt())
	}
	if zones.saves != 1 {
		t.Fatalf("重放保存了 %d 次，want 只有首笔那 1 次", zones.saves)
	}
	echoed, has := replay.Revision()
	if !has || echoed.Registrant().String() != "operator-1" {
		t.Fatalf("重放回显的登记责任方 = %q（%v），want 在册那一笔的 operator-1", echoed.Registrant(), has)
	}
}

// Covers: 重放与冲突要找的是任何一笔在册的修订，不只是最新那一笔——在册有 1、2，原样重投 1 答 ALREADY_REGISTERED，对 1 换内容答
// CONTENT_CONFLICT；两者都不写册。
func TestRegisterZoneJudgesReplayAgainstAnyRevisionOnTheRegister(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)
	expectZoneOutcome(t, "修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationRegistered)
	expectZoneOutcome(t, "修订 2", mustRegisterZone(t, handler, registerZoneCommand(t, 2, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（二）")
	})), application.RegistrationRegistered)

	expectZoneOutcome(t, "重投旧修订 1", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectZoneOutcome(t, "对旧修订 1 换内容", mustRegisterZone(t, handler, registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "另一个名字")
	})), application.RegistrationContentConflict)
	if zones.saves != 2 || len(zones.revisionsOf("SYN-ZONE-01")) != 2 {
		t.Fatalf("保存 %d 次、落册 %d 笔，want 2 次、2 笔", zones.saves, len(zones.revisionsOf("SYN-ZONE-01")))
	}
}

// 判定次序：先按（身份，修订号）找在册的那一笔，重放与冲突不再过后面的核。册上早就有的修订，哪怕它所指的仓此刻问不到、用途与最新
// 修订对不上，原样重投仍是重放，换内容仍是冲突，不能变成「查无所指」或「不可改的格」。
func TestRegisterZoneJudgesReplayAndConflictBeforeSequenceUsageAndReference(t *testing.T) {
	zones := newFakeZoneRegistry()
	warehouses := newFakeWarehouseLookup("SYN-TENANT-01")
	handler := newZoneHandler(zones, warehouses)
	seeded := registerZoneCommand(t, 1, nil)
	revision, err := domain.NewZoneRevision(seeded.Tenant, seeded.Warehouse, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	zones.seed(revision)

	expectZoneOutcome(t, "重放所指的仓问不到的在册修订", mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectZoneOutcome(t, "对它换用途", mustRegisterZone(t, handler, registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsagePicking
	})), application.RegistrationContentConflict)
	if zones.saves != 0 || warehouses.asked.Load() != 0 {
		t.Fatalf("重放与冲突保存了 %d 次、问了仓册 %d 次，want 都是 0", zones.saves, warehouses.asked.Load())
	}
}

// 未落册的修订依次核：修订号连续 → 用途没有改 → 指名的仓已登记。前面一格不成立就答前面那一格，不继续往后核。
func TestRegisterZoneChecksSequenceThenUsageThenTheWarehouse(t *testing.T) {
	zones := newFakeZoneRegistry()
	warehouses := newFakeWarehouseLookup("SYN-TENANT-01")
	handler := newZoneHandler(zones, warehouses)
	seeded := registerZoneCommand(t, 1, nil)
	revision, err := domain.NewZoneRevision(seeded.Tenant, seeded.Warehouse, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	zones.seed(revision)

	changeUsage := func(spec *domain.ZoneContentSpec) { spec.Usage = domain.ZoneUsagePicking }
	expectZoneRefusal(t, "跳号且用途改了、仓也问不到", mustRegisterZone(t, handler, registerZoneCommand(t, 3, changeUsage)),
		application.RegistrationRefusalRevisionOutOfSequence, domain.ZoneFieldInvalid)
	expectZoneRefusal(t, "用途改了且仓问不到", mustRegisterZone(t, handler, registerZoneCommand(t, 2, changeUsage)),
		application.RegistrationRefusalImmutableField, domain.ZoneFieldUsage)
	expectZoneRefusal(t, "只有仓问不到", mustRegisterZone(t, handler, registerZoneCommand(t, 2, nil)),
		application.RegistrationRefusalReferenceNotFound, domain.ZoneFieldWarehouseCode)
	if zones.saves != 0 {
		t.Fatalf("各种拒绝之后保存 %d 次，want 0", zones.saves)
	}
}

// 输入被拒是已形成的答案，不是技术失败：以上各格都经 mustRegisterZone 断言 error 为空。这里证反面——册读写不动才返回 error，
// 并且不折成任何一格业务结果（折成「从未登记」会让登记方从修订 1 重登一个本来就在册的库区，折成「查无所指的仓」会把登记方
// 打发去登记一个本来就在册的仓）。
func TestRegisterZoneReturnsAnErrorOnlyWhenTheRegistersCannotBeReadOrWritten(t *testing.T) {
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*fakeZoneRegistry, *fakeWarehouseLookup){
		"按修订号读不动":  func(zones *fakeZoneRegistry, _ *fakeWarehouseLookup) { zones.loadErr = boom },
		"读最新修订读不动": func(zones *fakeZoneRegistry, _ *fakeWarehouseLookup) { zones.loadLatestErr = boom },
		"读仓册读不动":   func(_ *fakeZoneRegistry, warehouses *fakeWarehouseLookup) { warehouses.err = boom },
		"保存不动":     func(zones *fakeZoneRegistry, _ *fakeWarehouseLookup) { zones.saveErr = boom },
		"保存答出集外的落点": func(zones *fakeZoneRegistry, _ *fakeWarehouseLookup) {
			zones.saveOutcome = ports.RegistrySaveOutcome(99)
		},
	} {
		zones, warehouses := zoneStores()
		arrange(zones, warehouses)
		result, err := newZoneHandler(zones, warehouses).Register(t.Context(), registerZoneCommand(t, 1, nil))
		if err == nil {
			t.Errorf("%s：err 为空，结果 = %s", name, result.Outcome())
		}
		if result.Outcome() != application.RegistrationOutcomeInvalid {
			t.Errorf("%s：结果 = %s，error 之外不该另带一格业务结果", name, result.Outcome())
		}
	}
}

// 登记口拒绝的是内容；身份与登记责任方没经构造器（零值）是接入侧装配错了，不是登记方的输入，按技术失败上抛。内容里缺格
// （零值内容）仍是内容不合并指出缺哪一格。
func TestRegisterZoneTreatsUnconstructedIdentityAsAnErrorAndUnconstructedContentAsInvalid(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)

	noTenant := registerZoneCommand(t, 1, nil)
	noTenant.Tenant = domain.TenantID{}
	noWarehouse := registerZoneCommand(t, 1, nil)
	noWarehouse.Warehouse = domain.WarehouseCode{}
	noZone := registerZoneCommand(t, 1, nil)
	noZone.Code = domain.ZoneCode{}
	noRegistrant := registerZoneCommand(t, 1, nil)
	noRegistrant.Registrant = domain.Registrant{}
	for name, command := range map[string]application.RegisterZoneCommand{
		"缺租户": noTenant, "缺仓编码": noWarehouse, "缺库区编码": noZone, "缺登记责任方": noRegistrant,
	} {
		if _, err := handler.Register(t.Context(), command); err == nil {
			t.Errorf("%s：应按技术失败上抛", name)
		}
	}

	noContent := registerZoneCommand(t, 1, nil)
	noContent.Content = domain.ZoneContent{}
	expectZoneRefusal(t, "零值内容", mustRegisterZone(t, handler, noContent),
		application.RegistrationRefusalContentInvalid, domain.ZoneFieldName)
	if zones.saves != 0 {
		t.Fatalf("保存了 %d 次，want 0", zones.saves)
	}
}

// 登记时刻取自注入的时钟，一次登记只读一次；回显的修订带它。
func TestRegisterZoneStampsTheRegistrationInstantFromTheClock(t *testing.T) {
	zones, warehouses := zoneStores()
	result := mustRegisterZone(t, newZoneHandler(zones, warehouses), registerZoneCommand(t, 1, nil))
	landed, has := result.Revision()
	if !has || !landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("登记时刻 = %v（%v），want 时钟给的 %v", landed.RegisteredAt(), has, registrationInstant.Truncate(time.Microsecond))
	}
	if stored := zones.revisionsOf("SYN-ZONE-01")[0]; !stored.RegisteredAt().Equal(landed.RegisteredAt()) {
		t.Fatalf("落册的登记时刻 %v 与回显的 %v 不一致", stored.RegisteredAt(), landed.RegisteredAt())
	}
}

// 仓的结果与库区的结果是同一个泛型结果的两个实例：各格只在回显的修订与「指出哪一格」的取值上不同，零值都是「没有结果」。
func TestWarehouseAndZoneResultsAreInstancesOfOneAlgebra(t *testing.T) {
	var (
		_ application.RegistrationResult[domain.WarehouseRevision, domain.WarehouseField] = application.RegisterWarehouseResult{}
		_ application.RegistrationResult[domain.ZoneRevision, domain.ZoneField]           = application.RegisterZoneResult{}
	)
	if (application.RegisterWarehouseResult{}).Outcome() != application.RegistrationOutcomeInvalid ||
		(application.RegisterZoneResult{}).Outcome() != application.RegistrationOutcomeInvalid {
		t.Errorf("零值结果该是没有结果")
	}
}

// 拒绝理由只在 NOT_ACCEPTED 时有值，回显的修订只在 REGISTERED 与 ALREADY_REGISTERED 时有：别的结果上读它们得零值。
func TestRegisterZoneResultCarriesDetailOnlyWhereItBelongs(t *testing.T) {
	zones, warehouses := zoneStores()
	handler := newZoneHandler(zones, warehouses)
	registered := mustRegisterZone(t, handler, registerZoneCommand(t, 1, nil))
	if registered.Refusal() != application.RegistrationRefusalNone || registered.Field() != domain.ZoneFieldInvalid || registered.LatestRevision() != 0 {
		t.Errorf("REGISTERED 带了拒绝细节：%s %q %d", registered.Refusal(), registered.Field(), registered.LatestRevision())
	}
	conflict := mustRegisterZone(t, handler, registerZoneCommand(t, 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "冲突的名字")
	}))
	if _, has := conflict.Revision(); has || conflict.Refusal() != application.RegistrationRefusalNone {
		t.Errorf("CONTENT_CONFLICT 不该带回显的修订或拒绝理由")
	}
	refused := mustRegisterZone(t, handler, registerZoneCommand(t, 5, nil))
	if _, has := refused.Revision(); has || !strings.Contains(refused.Refusal().String(), "SEQUENCE") {
		t.Errorf("NOT_ACCEPTED 不该带回显的修订")
	}
}
