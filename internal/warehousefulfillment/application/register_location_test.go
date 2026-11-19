package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证库位登记用例（UC-WF-001 的库位修订）：判定次序、结果代数、指名的仓、库区与库位类型须已登记、「新指名」的库位类型不得已停用，
// 以及并发。库位册、仓册、库区册与库位类型册用内存替身，真库上的重放、冲突与并发另在 adapters/postgres 的用例里证。租户、仓、库区、
// 库位与库位类型都是隔离合成值，只记 `S`。

// fakeLocationRegistry 是库位登记册的内存替身：同键同内容答已在册、同键异内容答冲突，与真适配器同判据。保存在锁里串行，等于主键
// 唯一约束；afterLookup 与 beforeSave 让并发用例逼出真实的竞态窗口。
type fakeLocationRegistry struct {
	mutex         sync.Mutex
	rows          map[string][]domain.LocationRevision
	saves         int
	loadErr       error
	loadLatestErr error
	saveErr       error
	saveOutcome   ports.RegistrySaveOutcome
	afterLookup   func()
	beforeSave    func()
}

func newFakeLocationRegistry() *fakeLocationRegistry {
	return &fakeLocationRegistry{rows: make(map[string][]domain.LocationRevision)}
}

func locationKey(tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode) string {
	return tenant.String() + "/" + warehouse.String() + "/" + code.String()
}

func (fake *fakeLocationRegistry) LoadLocationRevision(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
	revision int,
) (domain.LocationRevision, bool, error) {
	if fake.loadErr != nil {
		return domain.LocationRevision{}, false, fake.loadErr
	}
	fake.mutex.Lock()
	var found domain.LocationRevision
	var has bool
	for _, row := range fake.rows[locationKey(tenant, warehouse, code)] {
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

func (fake *fakeLocationRegistry) LoadLatestLocation(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) (domain.LocationRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.LocationRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var latest domain.LocationRevision
	var has bool
	for _, row := range fake.rows[locationKey(tenant, warehouse, code)] {
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has, nil
}

func (fake *fakeLocationRegistry) SaveLocationRevision(
	_ context.Context,
	revision domain.LocationRevision,
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
	key := locationKey(revision.Tenant(), revision.Warehouse(), revision.Code())
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
func (fake *fakeLocationRegistry) seed(revision domain.LocationRevision) {
	key := locationKey(revision.Tenant(), revision.Warehouse(), revision.Code())
	fake.rows[key] = append(fake.rows[key], revision)
}

func (fake *fakeLocationRegistry) revisionsOf(location string) []domain.LocationRevision {
	return fake.rows["SYN-TENANT-01/SYN-WH-01/"+location]
}

// trackedZones 在库区册之前包一层：数问过几次，并可以让读不动。库位只拿库区的窄读口，所以这里只实现读方法。
type trackedZones struct {
	inner ports.ZoneLookup
	asked int
	err   error
}

func (tracked *trackedZones) LoadLatestZone(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	tracked.asked++
	if tracked.err != nil {
		return domain.ZoneRevision{}, false, tracked.err
	}
	return tracked.inner.LoadLatestZone(ctx, tenant, warehouse, code)
}

// trackedLocationTypes 在库位类型册之前包一层：数问过几次，可以让读不动，并可以在读完之后插进一段（afterLoad），演「读到没停用、
// 落册之前被别人停用」。
type trackedLocationTypes struct {
	inner     ports.LocationTypeLookup
	asked     int
	err       error
	afterLoad func()
}

func (tracked *trackedLocationTypes) LoadLatestLocationType(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) (domain.LocationTypeRevision, bool, error) {
	tracked.asked++
	if tracked.err != nil {
		return domain.LocationTypeRevision{}, false, tracked.err
	}
	revision, found, err := tracked.inner.LoadLatestLocationType(ctx, tenant, code)
	if tracked.afterLoad != nil {
		tracked.afterLoad()
	}
	return revision, found, err
}

// locationFixture 是一组登记用例的替身：租户 SYN-TENANT-01 的仓 SYN-WH-01 已登记，仓下有库区 SYN-ZONE-01，租户有一个在用的库位类型
// SYN-LT-01。别的用例按需再放。
type locationFixture struct {
	locations  *fakeLocationRegistry
	warehouses *fakeWarehouseLookup
	zoneBook   *fakeZoneRegistry
	zones      *trackedZones
	typeBook   *fakeLocationTypeRegistry
	types      *trackedLocationTypes
}

func newLocationFixture(t *testing.T) *locationFixture {
	t.Helper()
	fixture := &locationFixture{
		locations:  newFakeLocationRegistry(),
		warehouses: newFakeWarehouseLookup("SYN-TENANT-01", "SYN-WH-01"),
		zoneBook:   newFakeZoneRegistry(),
		typeBook:   newFakeLocationTypeRegistry(),
	}
	fixture.zones = &trackedZones{inner: fixture.zoneBook}
	fixture.types = &trackedLocationTypes{inner: fixture.typeBook}
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", false)
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 1, false)
	return fixture
}

func (fixture *locationFixture) putZone(t *testing.T, tenant, warehouse, code string, deactivated bool) {
	t.Helper()
	revision, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, tenant), valueOf(t, domain.NewWarehouseCode, warehouse), valueOf(t, domain.NewZoneCode, code),
		1, zoneContentWith(t, func(spec *domain.ZoneContentSpec) { spec.Deactivated = deactivated }),
		valueOf(t, domain.NewRegistrant, "operator-z"), registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	fixture.zoneBook.seed(revision)
}

// putLocationType 放一笔库位类型修订；停用一个类型就是放它的下一个修订号、停用标记为真。
func (fixture *locationFixture) putLocationType(t *testing.T, tenant, code string, revision int, deactivated bool) {
	t.Helper()
	built, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, tenant), valueOf(t, domain.NewLocationTypeCode, code), revision,
		locationTypeContentWith(t, func(spec *domain.LocationTypeContentSpec) { spec.Deactivated = deactivated }),
		valueOf(t, domain.NewRegistrant, "operator-t"), registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	fixture.typeBook.seed(built)
}

func (fixture *locationFixture) handler() *application.RegisterLocationHandler {
	return application.NewRegisterLocationHandler(
		fixture.locations, fixture.warehouses, fixture.zones, fixture.types, &fixedClock{at: registrationInstant})
}

// lookupsAsked 是别的册被问过的总次数，重放与冲突、错位与不可改的用例据此证明「没有走到后面的核」。
func (fixture *locationFixture) lookupsAsked() int {
	return fixture.warehouses.asked + fixture.zones.asked + fixture.types.asked
}

// 下面几个是内容的写法：用例只改它关心的格，缺省是只带所属库区 SYN-ZONE-01、不带库位类型与优先级、不停用。

func inZone(t *testing.T, code string) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) { spec.ZoneCode = valueOf(t, domain.NewZoneCode, code) }
}

func typedAs(t *testing.T, code string) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) {
		spec.LocationType = valueOf(t, domain.NewLocationTypeCode, code)
	}
}

func prioritisedAs(value int32) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) { spec.Priority = domain.NewLocationPriority(value) }
}

func deactivatedLocation(spec *domain.LocationContentSpec) { spec.Deactivated = true }

func combined(edits ...func(*domain.LocationContentSpec)) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) {
		for _, edit := range edits {
			edit(spec)
		}
	}
}

func locationContentWith(t *testing.T, edit func(*domain.LocationContentSpec)) domain.LocationContent {
	t.Helper()
	spec := domain.LocationContentSpec{ZoneCode: valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationContent(spec)
	if err != nil {
		t.Fatalf("NewLocationContent: %v", err)
	}
	return content
}

func registerLocationCommand(t *testing.T, revision int, edit func(*domain.LocationContentSpec)) application.RegisterLocationCommand {
	t.Helper()
	return application.RegisterLocationCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Warehouse:  valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Code:       valueOf(t, domain.NewLocationCode, "SYN-LOC-01"),
		Revision:   revision,
		Content:    locationContentWith(t, edit),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

// seedLocation 把一笔修订直接放进库位册。
func (fixture *locationFixture) seedLocation(t *testing.T, revision int, edit func(*domain.LocationContentSpec)) {
	t.Helper()
	command := registerLocationCommand(t, revision, edit)
	built, err := domain.NewLocationRevision(
		command.Tenant, command.Warehouse, command.Code, revision, command.Content, command.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	fixture.locations.seed(built)
}

func mustRegisterLocation(
	t *testing.T,
	handler *application.RegisterLocationHandler,
	command application.RegisterLocationCommand,
) application.RegisterLocationResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register 修订 %d：业务格不该走 error：%v", command.Revision, err)
	}
	return result
}

func expectLocationOutcome(t *testing.T, label string, result application.RegisterLocationResult, want application.RegistrationOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s（拒绝理由 %s，指向 %s），want %s", label, result.Outcome(), result.Refusal(), result.Field(), want)
	}
}

func expectLocationRefusal(
	t *testing.T,
	label string,
	result application.RegisterLocationResult,
	want application.RegistrationRefusal,
	wantField domain.LocationField,
) {
	t.Helper()
	expectLocationOutcome(t, label, result, application.RegistrationNotAccepted)
	if result.Refusal() != want || result.Field() != wantField {
		t.Fatalf("%s：拒绝理由 = %s 指向 %q，want %s 指向 %q", label, result.Refusal(), result.Field(), want, wantField)
	}
}

// Covers: AT-WF-001（库位的部分）——首次登记答 REGISTERED 且修订 1 落册；原样重投答 ALREADY_REGISTERED、册里仍只有一份；同一修订号
// 换内容答 CONTENT_CONFLICT、在册不变；首笔修订号给 2 答 NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 并回显 0；已有修订 1 时给 3 同样答
// 错位并回显 1。
func TestRegisterLocationFirstRevisionReplayConflictAndOutOfSequence(t *testing.T) {
	fixture := newLocationFixture(t)
	handler := fixture.handler()
	first := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(7))))
	expectLocationOutcome(t, "首次登记", first, application.RegistrationRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("回显的修订 = %+v（%v），want 修订 1、登记责任方 operator-1、时钟给的登记时刻", landed, has)
	}
	if len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(fixture.locations.revisionsOf("SYN-LOC-01")))
	}

	replay := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(7))))
	expectLocationOutcome(t, "原样重投", replay, application.RegistrationAlreadyRegistered)
	if fixture.locations.saves != 1 || len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 {
		t.Fatalf("重放之后保存 %d 次、落册 %d 笔，want 1 次、1 笔", fixture.locations.saves, len(fixture.locations.revisionsOf("SYN-LOC-01")))
	}

	conflict := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(8))))
	expectLocationOutcome(t, "同一修订号换内容", conflict, application.RegistrationContentConflict)
	if got := fixture.locations.revisionsOf("SYN-LOC-01")[0].Content().Priority().Value(); got != 7 {
		t.Fatalf("冲突之后在册的优先级 = %d，在册的不该变", got)
	}

	skipped := registerLocationCommand(t, 2, nil)
	skipped.Code = valueOf(t, domain.NewLocationCode, "SYN-LOC-02")
	result := mustRegisterLocation(t, handler, skipped)
	expectLocationRefusal(t, "首笔给 2", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationFieldInvalid)
	if result.LatestRevision() != 0 || len(fixture.locations.revisionsOf("SYN-LOC-02")) != 0 {
		t.Fatalf("首笔错位回显 %d、落册 %d 笔，want 回显 0、不落册", result.LatestRevision(), len(fixture.locations.revisionsOf("SYN-LOC-02")))
	}

	result = mustRegisterLocation(t, handler, registerLocationCommand(t, 3, nil))
	expectLocationRefusal(t, "已有修订 1 时给 3", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationFieldInvalid)
	if result.LatestRevision() != 1 || len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 {
		t.Fatalf("跳号回显 %d、落册 %d 笔，want 回显 1、仍是 1 笔", result.LatestRevision(), len(fixture.locations.revisionsOf("SYN-LOC-01")))
	}
}

// 修订号小于 1 不是「最新加一」：与跳号同答错位并回显在册最新修订号，不另造一格。
func TestRegisterLocationTreatsANonPositiveRevisionNumberAsOutOfSequence(t *testing.T) {
	fixture := newLocationFixture(t)
	handler := fixture.handler()

	for _, number := range []int{0, -1} {
		result := mustRegisterLocation(t, handler, registerLocationCommand(t, number, nil))
		expectLocationRefusal(t, fmt.Sprintf("空册给 %d", number), result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationFieldInvalid)
		if result.LatestRevision() != 0 {
			t.Fatalf("空册给 %d 回显 %d，want 0", number, result.LatestRevision())
		}
	}
	expectLocationOutcome(t, "修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)
	result := mustRegisterLocation(t, handler, registerLocationCommand(t, 0, nil))
	expectLocationRefusal(t, "已有修订 1 时给 0", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationFieldInvalid)
	if result.LatestRevision() != 1 || fixture.locations.saves != 1 {
		t.Fatalf("回显 %d、保存 %d 次，want 回显 1、保存 1 次", result.LatestRevision(), fixture.locations.saves)
	}
}

// Covers: AT-WF-001（库位的部分）最后一行——并发提交同一修订号、内容相同的两笔，一笔答 REGISTERED、一笔答 ALREADY_REGISTERED；册里只有
// 一份，答重放的那一笔回显的是已落册的那一份（登记责任方是先落册者的）。内容不同时一笔落册、另一笔答 CONTENT_CONFLICT。两笔都停在
// 「已查过在册」之后再各自保存，逼的是真实的竞态窗口。
func TestRegisterLocationConcurrentSubmissionsOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondPriority int32
		wantLoser      application.RegistrationOutcome
	}{
		"内容相同": {secondPriority: 5, wantLoser: application.RegistrationAlreadyRegistered},
		"内容不同": {secondPriority: 6, wantLoser: application.RegistrationContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newLocationFixture(t)
			meeting := newRendezvous(2)
			fixture.locations.afterLookup = meeting.arrive
			handler := fixture.handler()

			commands := []application.RegisterLocationCommand{
				registerLocationCommand(t, 1, prioritisedAs(5)),
				registerLocationCommand(t, 1, prioritisedAs(test.secondPriority)),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")

			results := make([]application.RegisterLocationResult, len(commands))
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
			landed, others := 0, []application.RegisterLocationResult{}
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
			rows := fixture.locations.revisionsOf("SYN-LOC-01")
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
// 后一个窗口里靠登记册的主键，保存按内容答已在册或冲突。两个窗口的答案必须一样（库区那一册里守的同一个竞态，库位同样要守）。
func TestRegisterLocationJudgesAConcurrentLandingByContentInBothRaceWindows(t *testing.T) {
	for windowName, install := range map[string]func(*fakeLocationRegistry, func()){
		"查册之后": func(locations *fakeLocationRegistry, land func()) { locations.afterLookup = land },
		"保存之前": func(locations *fakeLocationRegistry, land func()) { locations.beforeSave = land },
	} {
		for contentName, test := range map[string]struct {
			competitorPriority int32
			want               application.RegistrationOutcome
		}{
			"内容相同": {5, application.RegistrationAlreadyRegistered},
			"内容不同": {9, application.RegistrationContentConflict},
		} {
			t.Run(windowName+"/"+contentName, func(t *testing.T) {
				fixture := newLocationFixture(t)
				competitorCommand := registerLocationCommand(t, 1, prioritisedAs(test.competitorPriority))
				competitorCommand.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
				competitor, err := domain.NewLocationRevision(
					competitorCommand.Tenant, competitorCommand.Warehouse, competitorCommand.Code, 1, competitorCommand.Content,
					competitorCommand.Registrant, registrationInstant.Add(-time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				landed := false
				install(fixture.locations, func() {
					if !landed {
						landed = true
						fixture.locations.seed(competitor)
					}
				})

				result := mustRegisterLocation(t, fixture.handler(), registerLocationCommand(t, 1, prioritisedAs(5)))
				expectLocationOutcome(t, "别人先落册之后的这一笔", result, test.want)
				if !landed || len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 {
					t.Fatalf("竞争者落册 %v、册里 %d 笔，want 已落、只有 1 笔", landed, len(fixture.locations.revisionsOf("SYN-LOC-01")))
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

// Covers: AT-WF-002（库位的部分）——把库位改到另一个库区答 NOT_ACCEPTED/IMMUTABLE_FIELD 并指出 zoneCode，册不变；只改优先级答
// REGISTERED，修订号加一，旧修订照留；改完优先级之后再改库区仍被拒（拿最新那一笔比，与最新一笔的库区一致才受理）。被拒的库区
// 是在册的、同仓的——不可改先于查无所指，所以另一个库区在不在册都不影响这一答。
func TestRegisterLocationKeepsTheZoneAndLetsThePriorityChange(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-02", false)
	handler := fixture.handler()
	expectLocationOutcome(t, "修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(1))), application.RegistrationRegistered)

	for _, zone := range []string{"SYN-ZONE-02", "SYN-ZONE-NEVER"} {
		result := mustRegisterLocation(t, handler, registerLocationCommand(t, 2, combined(inZone(t, zone), prioritisedAs(1))))
		expectLocationRefusal(t, "改到库区 "+zone, result, application.RegistrationRefusalImmutableField, domain.LocationFieldZoneCode)
		if _, has := result.Revision(); has {
			t.Fatalf("被拒的结果不该带回显的修订")
		}
	}
	if len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 || fixture.locations.saves != 1 {
		t.Fatalf("被拒之后落册 %d 笔、保存 %d 次，want 册不变", len(fixture.locations.revisionsOf("SYN-LOC-01")), fixture.locations.saves)
	}

	reprioritised := mustRegisterLocation(t, handler, registerLocationCommand(t, 2, prioritisedAs(9)))
	expectLocationOutcome(t, "只改优先级", reprioritised, application.RegistrationRegistered)
	rows := fixture.locations.revisionsOf("SYN-LOC-01")
	if len(rows) != 2 || rows[0].Content().Priority().Value() != 1 || rows[1].Revision() != 2 ||
		rows[1].Content().Priority().Value() != 9 || rows[1].Content().ZoneCode().String() != "SYN-ZONE-01" {
		t.Fatalf("改优先级之后的册 = %d 笔，旧修订该照留、新修订占修订号 2", len(rows))
	}

	result := mustRegisterLocation(t, handler, registerLocationCommand(t, 3, combined(inZone(t, "SYN-ZONE-02"), prioritisedAs(9))))
	expectLocationRefusal(t, "改优先级之后再改库区", result, application.RegistrationRefusalImmutableField, domain.LocationFieldZoneCode)
}

// Covers: AT-WF-003（库位的部分）——库位指名另一个仓的库区答 NOT_ACCEPTED/REFERENCE_NOT_FOUND 并指出 zoneCode：库区编码在另一个仓下
// 存在也按查不到答；指名没登记的库位类型答 REFERENCE_NOT_FOUND 并指出 locationTypeCode；指名没登记的仓答 REFERENCE_NOT_FOUND 并指出
// warehouseCode。都不落册，不代建所指的对象。
func TestRegisterLocationRefusesWhatItNamesAndCannotFind(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses.registered["SYN-TENANT-01/SYN-WH-02"] = true
	// 库区 SYN-ZONE-02 只在二号仓下登记过。
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-02", false)
	handler := fixture.handler()

	anotherWarehouse := registerLocationCommand(t, 1, inZone(t, "SYN-ZONE-02"))
	expectLocationRefusal(t, "指名另一个仓的库区", mustRegisterLocation(t, handler, anotherWarehouse),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldZoneCode)

	neverZone := registerLocationCommand(t, 1, inZone(t, "SYN-ZONE-NEVER"))
	expectLocationRefusal(t, "指名没登记的库区", mustRegisterLocation(t, handler, neverZone),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldZoneCode)

	neverType := registerLocationCommand(t, 1, typedAs(t, "SYN-LT-NEVER"))
	expectLocationRefusal(t, "指名没登记的库位类型", mustRegisterLocation(t, handler, neverType),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldLocationTypeCode)

	neverWarehouse := registerLocationCommand(t, 1, nil)
	neverWarehouse.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-NEVER")
	expectLocationRefusal(t, "指名没登记的仓", mustRegisterLocation(t, handler, neverWarehouse),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldWarehouseCode)

	if fixture.locations.saves != 0 || len(fixture.locations.rows) != 0 {
		t.Fatalf("被拒之后保存 %d 次、册里 %d 个库位，want 不落册", fixture.locations.saves, len(fixture.locations.rows))
	}

	// 同一个库区编码放进这个仓之后，同一笔就受理了：证明上面是「查不到」，不是别的什么。
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-02", false)
	expectLocationOutcome(t, "库区在这个仓下登记之后", mustRegisterLocation(t, handler, anotherWarehouse), application.RegistrationRegistered)
}

// Covers: AT-WF-003（库位的部分）最后一行——指名别的租户的仓编码、库区编码、库位类型编码，都与没登记过的同答：各册按租户读，本租户
// 查不到就是查不到，答复里没有别的租户有没有它们的任何痕迹。
func TestRegisterLocationAnswersTheSameForOtherTenantsObjectsAsForMissingOnes(t *testing.T) {
	fixture := newLocationFixture(t)
	// 这几样只在租户 SYN-TENANT-02 名下登记过。
	fixture.warehouses.registered["SYN-TENANT-02/SYN-WH-OTHER"] = true
	fixture.putZone(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-OTHER", false)
	fixture.putLocationType(t, "SYN-TENANT-02", "SYN-LT-OTHER", 1, false)
	handler := fixture.handler()

	pairs := map[string][2]application.RegisterLocationCommand{}
	{
		other := registerLocationCommand(t, 1, nil)
		other.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-OTHER")
		never := registerLocationCommand(t, 1, nil)
		never.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-NEVER")
		pairs["仓编码"] = [2]application.RegisterLocationCommand{other, never}
	}
	pairs["库区编码"] = [2]application.RegisterLocationCommand{
		registerLocationCommand(t, 1, inZone(t, "SYN-ZONE-OTHER")), registerLocationCommand(t, 1, inZone(t, "SYN-ZONE-NEVER")),
	}
	pairs["库位类型编码"] = [2]application.RegisterLocationCommand{
		registerLocationCommand(t, 1, typedAs(t, "SYN-LT-OTHER")), registerLocationCommand(t, 1, typedAs(t, "SYN-LT-NEVER")),
	}
	for name, pair := range pairs {
		other, never := mustRegisterLocation(t, handler, pair[0]), mustRegisterLocation(t, handler, pair[1])
		if other != never {
			t.Errorf("指名别的租户的%s与指名没登记过的答复不同：%+v 与 %+v，泄露了是否存在", name, other, never)
		}
		if other.Outcome() != application.RegistrationNotAccepted || other.Refusal() != application.RegistrationRefusalReferenceNotFound {
			t.Errorf("指名别的租户的%s = %s/%s，want NOT_ACCEPTED/REFERENCE_NOT_FOUND", name, other.Outcome(), other.Refusal())
		}
	}
	if fixture.locations.saves != 0 {
		t.Fatalf("被拒之后保存 %d 次，want 不落册", fixture.locations.saves)
	}
}

// Covers: AT-WF-009（库位的部分）——一个仓都没登记的租户：登记库位答 NOT_ACCEPTED/REFERENCE_NOT_FOUND 并指出仓那一格，不落册；
// 之后登记了仓，同一修订原样重交照常受理（那一笔当初没有落册，重交不是重放）。
func TestRegisterLocationBeforeAnyWarehouseIsRegisteredIsRefusedAndTheSameRevisionLandsAfterwards(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses = newFakeWarehouseLookup("SYN-TENANT-01")
	handler := fixture.handler()

	result := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil))
	expectLocationRefusal(t, "一个仓都没登记", result, application.RegistrationRefusalReferenceNotFound, domain.LocationFieldWarehouseCode)
	if fixture.locations.saves != 0 {
		t.Fatalf("被拒之后保存了 %d 次，want 0", fixture.locations.saves)
	}

	fixture.warehouses.registered["SYN-TENANT-01/SYN-WH-01"] = true
	expectLocationOutcome(t, "登记仓之后原样重交", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)
}

// 判定次序：先按（身份，修订号）找在册的那一笔，重放与冲突不再过后面的核。册上早就有的修订，哪怕它所指的仓、库区、库位类型此刻都
// 问不到、库区与最新修订对不上，原样重投仍是重放，换内容仍是冲突，不能变成「查无所指」「不可改的格」或「所指已停用」。
func TestRegisterLocationJudgesReplayAndConflictBeforeEverythingElse(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses = newFakeWarehouseLookup("SYN-TENANT-01")
	fixture.zoneBook = newFakeZoneRegistry()
	fixture.zones.inner = fixture.zoneBook
	fixture.typeBook = newFakeLocationTypeRegistry()
	fixture.types.inner = fixture.typeBook
	fixture.seedLocation(t, 1, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(3)))
	handler := fixture.handler()

	expectLocationOutcome(t, "重放所指的各册都问不到的在册修订",
		mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(typedAs(t, "SYN-LT-01"), prioritisedAs(3)))),
		application.RegistrationAlreadyRegistered)
	expectLocationOutcome(t, "对它换库区",
		mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(inZone(t, "SYN-ZONE-02"), typedAs(t, "SYN-LT-01"), prioritisedAs(3)))),
		application.RegistrationContentConflict)
	expectLocationOutcome(t, "对它去掉库位类型",
		mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(3))), application.RegistrationContentConflict)
	if fixture.locations.saves != 0 || fixture.lookupsAsked() != 0 {
		t.Fatalf("重放与冲突保存了 %d 次、问了别的册 %d 次，want 都是 0", fixture.locations.saves, fixture.lookupsAsked())
	}
}

// 未落册的修订依次核：修订号连续 → 库区没有改 → 仓已登记 → 库区已登记且属于这个仓 → 库位类型已登记 → 新指名的库位类型没有停用。
// 前面一格不成立就答前面那一格，不继续往后核，也不去问后面的册。
func TestRegisterLocationChecksSequenceThenZoneThenWarehouseThenZoneThenTypeThenDeactivation(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses = newFakeWarehouseLookup("SYN-TENANT-01") // 仓问不到
	fixture.zoneBook = newFakeZoneRegistry()                     // 库区问不到
	fixture.zones.inner = fixture.zoneBook
	fixture.typeBook = newFakeLocationTypeRegistry() // 库位类型问不到
	fixture.types.inner = fixture.typeBook
	fixture.seedLocation(t, 1, nil)
	handler := fixture.handler()

	changeZoneAndName := combined(inZone(t, "SYN-ZONE-02"), typedAs(t, "SYN-LT-01"))
	expectLocationRefusal(t, "跳号且库区改了、各册都问不到", mustRegisterLocation(t, handler, registerLocationCommand(t, 3, changeZoneAndName)),
		application.RegistrationRefusalRevisionOutOfSequence, domain.LocationFieldInvalid)
	expectLocationRefusal(t, "库区改了且各册都问不到", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, changeZoneAndName)),
		application.RegistrationRefusalImmutableField, domain.LocationFieldZoneCode)
	if fixture.lookupsAsked() != 0 {
		t.Fatalf("错位与不可改之后问了别的册 %d 次，want 0", fixture.lookupsAsked())
	}

	named := registerLocationCommand(t, 2, typedAs(t, "SYN-LT-01"))
	expectLocationRefusal(t, "仓、库区、库位类型都问不到", mustRegisterLocation(t, handler, named),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldWarehouseCode)
	if fixture.zones.asked != 0 || fixture.types.asked != 0 {
		t.Fatalf("仓问不到之后问了库区 %d 次、库位类型 %d 次，want 0", fixture.zones.asked, fixture.types.asked)
	}

	fixture.warehouses.registered["SYN-TENANT-01/SYN-WH-01"] = true
	expectLocationRefusal(t, "仓在册、库区与库位类型问不到", mustRegisterLocation(t, handler, named),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldZoneCode)
	if fixture.types.asked != 0 {
		t.Fatalf("库区问不到之后问了库位类型 %d 次，want 0", fixture.types.asked)
	}

	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", false)
	expectLocationRefusal(t, "只有库位类型问不到", mustRegisterLocation(t, handler, named),
		application.RegistrationRefusalReferenceNotFound, domain.LocationFieldLocationTypeCode)

	// 查不到先于已停用：一个没登记的类型谈不上停用；登记了而且停用了，才答所指已停用。
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 1, true)
	expectLocationRefusal(t, "类型已停用", mustRegisterLocation(t, handler, named),
		application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	if fixture.locations.saves != 0 {
		t.Fatalf("各种拒绝之后保存 %d 次，want 0", fixture.locations.saves)
	}
}

// Covers: AT-WF-003（库位的部分）——库位新指名一个已停用的库位类型答 NOT_ACCEPTED/REFERENCE_DEACTIVATED 并指出 locationTypeCode，
// 不落册；沿用它原有的、后来被停用的库位类型，只改优先级，答 REGISTERED；这笔修订之后再改回去也照常。
func TestRegisterLocationRefusesANewlyNamedDeactivatedTypeAndKeepsAnExistingOneAfterItsDeactivation(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-OLD", 1, false)
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-GONE", 1, true)
	handler := fixture.handler()

	expectLocationRefusal(t, "新指名一个已停用的库位类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, typedAs(t, "SYN-LT-GONE"))),
		application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	if len(fixture.locations.rows) != 0 || fixture.locations.saves != 0 {
		t.Fatalf("被拒之后册里 %d 个库位、保存 %d 次，want 不落册", len(fixture.locations.rows), fixture.locations.saves)
	}

	expectLocationOutcome(t, "修订 1 指名在用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, combined(typedAs(t, "SYN-LT-OLD"), prioritisedAs(1)))),
		application.RegistrationRegistered)
	// 类型此后被停用（再登记一个停用的修订）。
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-OLD", 2, true)

	kept := mustRegisterLocation(t, handler, registerLocationCommand(t, 2, combined(typedAs(t, "SYN-LT-OLD"), prioritisedAs(2))))
	expectLocationOutcome(t, "沿用后来被停用的类型、只改优先级", kept, application.RegistrationRegistered)
	if landed, has := kept.Revision(); !has || landed.Revision() != 2 {
		t.Fatalf("回显 = %+v（%v），want 修订 2", landed, has)
	}
	expectLocationOutcome(t, "类型仍停用，再沿用并停用库位",
		mustRegisterLocation(t, handler, registerLocationCommand(t, 3, combined(typedAs(t, "SYN-LT-OLD"), prioritisedAs(2), deactivatedLocation))),
		application.RegistrationRegistered)
	if rows := fixture.locations.revisionsOf("SYN-LOC-01"); len(rows) != 3 {
		t.Fatalf("册里 %d 笔，want 3", len(rows))
	}
}

// Covers: AT-WF-003（库位的部分）「新指名」的三种情形各一条——首笔修订就指名；上一笔没带库位类型、这一笔带上；换成另一个类型编码。
// 三种都在类型此刻已停用时答 REFERENCE_DEACTIVATED。又证：上一笔带类型、这一笔去掉类型、再下一笔又指名同一个已停用的类型，算新指名。
func TestRegisterLocationJudgesWhatCountsAsNamingATypeNewly(t *testing.T) {
	t.Run("首笔修订", func(t *testing.T) {
		fixture := newLocationFixture(t)
		fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-GONE", 1, true)
		expectLocationRefusal(t, "首笔指名已停用的类型", mustRegisterLocation(t, fixture.handler(), registerLocationCommand(t, 1, typedAs(t, "SYN-LT-GONE"))),
			application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	})

	t.Run("上一笔没带类型", func(t *testing.T) {
		fixture := newLocationFixture(t)
		fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-GONE", 1, true)
		handler := fixture.handler()
		expectLocationOutcome(t, "修订 1 不带类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)
		expectLocationRefusal(t, "修订 2 带上已停用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, typedAs(t, "SYN-LT-GONE"))),
			application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	})

	t.Run("换成另一个类型编码", func(t *testing.T) {
		fixture := newLocationFixture(t)
		fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-GONE", 1, true)
		handler := fixture.handler()
		expectLocationOutcome(t, "修订 1 带在用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, typedAs(t, "SYN-LT-01"))), application.RegistrationRegistered)
		expectLocationRefusal(t, "修订 2 换成已停用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, typedAs(t, "SYN-LT-GONE"))),
			application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	})

	t.Run("类型编码只差大小写也是另一个类型", func(t *testing.T) {
		fixture := newLocationFixture(t)
		fixture.putLocationType(t, "SYN-TENANT-01", "syn-lt-01", 1, true)
		handler := fixture.handler()
		expectLocationOutcome(t, "修订 1 带在用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, typedAs(t, "SYN-LT-01"))), application.RegistrationRegistered)
		expectLocationRefusal(t, "修订 2 换成只差大小写的已停用类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, typedAs(t, "syn-lt-01"))),
			application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
	})

	t.Run("去掉类型之后再指名同一个已停用的类型", func(t *testing.T) {
		fixture := newLocationFixture(t)
		handler := fixture.handler()
		expectLocationOutcome(t, "修订 1 带在用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, typedAs(t, "SYN-LT-01"))), application.RegistrationRegistered)
		fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 2, true)
		expectLocationOutcome(t, "修订 2 去掉类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, nil)), application.RegistrationRegistered)
		expectLocationRefusal(t, "修订 3 又指名同一个已停用的类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 3, typedAs(t, "SYN-LT-01"))),
			application.RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode)
		// 类型再启用之后，指名它又可以了。
		fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 3, false)
		expectLocationOutcome(t, "类型再启用之后修订 3 指名它", mustRegisterLocation(t, handler, registerLocationCommand(t, 3, typedAs(t, "SYN-LT-01"))), application.RegistrationRegistered)
	})
}

// 库位类型在「读到没停用」与「落册」之间被别人停用：这一笔照常落册——停用只作用于此后的新指名，不为它给库位类型册加锁。
func TestRegisterLocationAcceptsATypeDeactivatedBetweenTheReadAndTheSave(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.types.afterLoad = func() { fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-01", 2, true) }

	result := mustRegisterLocation(t, fixture.handler(), registerLocationCommand(t, 1, typedAs(t, "SYN-LT-01")))
	expectLocationOutcome(t, "读到没停用之后类型被停用", result, application.RegistrationRegistered)
	if len(fixture.locations.revisionsOf("SYN-LOC-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(fixture.locations.revisionsOf("SYN-LOC-01")))
	}
}

// 没有库位类型与没有优先级的库位登记得进、读得回；优先级 0 与没有优先级是两份内容：同一修订号一笔带 0、一笔不带，是内容冲突；
// 上一笔带类型、这一笔不带，是一次内容变化（合法的新修订）。
func TestRegisterLocationKeepsAbsentAndPresentOptionalCellsApart(t *testing.T) {
	fixture := newLocationFixture(t)
	handler := fixture.handler()

	bare := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil))
	expectLocationOutcome(t, "只带所属库区", bare, application.RegistrationRegistered)
	landed, _ := bare.Revision()
	if _, has := landed.Content().LocationType(); has || landed.Content().Priority().Given() {
		t.Fatalf("没带类型与优先级的修订读回 = 类型 %v、优先级 %v，want 都没有", has, landed.Content().Priority().Given())
	}

	zeroOnSameRevision := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(0)))
	expectLocationOutcome(t, "同一修订号一笔带优先级 0、一笔不带", zeroOnSameRevision, application.RegistrationContentConflict)
	if rows := fixture.locations.revisionsOf("SYN-LOC-01"); len(rows) != 1 || rows[0].Content().Priority().Given() {
		t.Fatalf("冲突之后在册的优先级该仍是「没有」")
	}

	zero := mustRegisterLocation(t, handler, registerLocationCommand(t, 2, prioritisedAs(0)))
	expectLocationOutcome(t, "下一修订带优先级 0", zero, application.RegistrationRegistered)
	if got, _ := zero.Revision(); !got.Content().Priority().Given() || got.Content().Priority().Value() != 0 {
		t.Fatalf("优先级 0 读回 = %+v", got.Content().Priority())
	}
	expectLocationOutcome(t, "再把优先级去掉", mustRegisterLocation(t, handler, registerLocationCommand(t, 3, nil)), application.RegistrationRegistered)

	expectLocationOutcome(t, "负优先级与极值", mustRegisterLocation(t, handler, registerLocationCommand(t, 4, prioritisedAs(-2147483648))), application.RegistrationRegistered)

	// 带类型、再不带类型：去掉类型是合法的新修订。
	expectLocationOutcome(t, "带上类型", mustRegisterLocation(t, handler, registerLocationCommand(t, 5, typedAs(t, "SYN-LT-01"))), application.RegistrationRegistered)
	removed := mustRegisterLocation(t, handler, registerLocationCommand(t, 6, nil))
	expectLocationOutcome(t, "去掉类型", removed, application.RegistrationRegistered)
	got, _ := removed.Revision()
	if _, has := got.Content().LocationType(); has {
		t.Fatalf("去掉类型的修订读回仍带类型")
	}
}

// Covers: AT-WF-003 与 CONTEXT「主数据登记」——停用的仓、停用的库区也算已登记：停用只挡此后的移入目标，不挡在它名下登记库位。仓册里
// 读到的最新那一笔是停用的、库区册里读到的最新那一笔是停用的，库位照常受理，修订 1 与后续修订都一样。
func TestRegisterLocationUnderADeactivatedWarehouseAndZoneIsAccepted(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses.deactivated["SYN-TENANT-01/SYN-WH-01"] = true
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-D", true)
	handler := fixture.handler()

	expectLocationOutcome(t, "停用的仓与库区下登记库位", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, inZone(t, "SYN-ZONE-D"))), application.RegistrationRegistered)
	expectLocationOutcome(t, "停用的仓与库区下改优先级", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, combined(inZone(t, "SYN-ZONE-D"), prioritisedAs(4)))),
		application.RegistrationRegistered)
	if fixture.warehouses.asked == 0 || fixture.zones.asked == 0 {
		t.Fatalf("没有向仓册 %d 次、库区册 %d 次问过它们在不在册", fixture.warehouses.asked, fixture.zones.asked)
	}
}

// 停用与再启用库位各是一笔新修订，都答 REGISTERED；停用不拦此后的修订（停用期间照常改优先级），旧修订照留；所属库区在停用与再启用
// 之后仍不可改。
func TestRegisterLocationDeactivationAndReactivationAreNewRevisions(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-02", false)
	handler := fixture.handler()
	expectLocationOutcome(t, "修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)
	expectLocationOutcome(t, "停用", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, deactivatedLocation)), application.RegistrationRegistered)
	expectLocationOutcome(t, "停用期间改优先级", mustRegisterLocation(t, handler, registerLocationCommand(t, 3, combined(deactivatedLocation, prioritisedAs(2)))),
		application.RegistrationRegistered)
	expectLocationRefusal(t, "停用期间改库区", mustRegisterLocation(t, handler, registerLocationCommand(t, 4, combined(deactivatedLocation, prioritisedAs(2), inZone(t, "SYN-ZONE-02")))),
		application.RegistrationRefusalImmutableField, domain.LocationFieldZoneCode)
	expectLocationOutcome(t, "再启用", mustRegisterLocation(t, handler, registerLocationCommand(t, 4, prioritisedAs(2))), application.RegistrationRegistered)

	rows := fixture.locations.revisionsOf("SYN-LOC-01")
	if len(rows) != 4 || rows[0].Content().Deactivated() || !rows[1].Content().Deactivated() ||
		!rows[2].Content().Deactivated() || rows[3].Content().Deactivated() {
		t.Fatalf("停用与再启用之后的册不对：四笔的停用标记该依次是 否、是、是、否")
	}
}

// 库位编码在仓内才唯一：同一个库位编码在两个仓下是两个互相独立的库位，各自从修订 1 起；同一个仓、同一个编码在另一个租户名下也是
// 另一个库位；编码逐字保存，只差大小写的是另一个库位。
func TestRegisterLocationKeepsLocationsOfDifferentWarehousesTenantsAndSpellingsApart(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.warehouses.registered["SYN-TENANT-01/SYN-WH-02"] = true
	fixture.putZone(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-01", false)
	fixture.warehouses.registered["SYN-TENANT-02/SYN-WH-01"] = true
	fixture.putZone(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-01", false)
	handler := fixture.handler()

	expectLocationOutcome(t, "一号仓的库位", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)

	second := registerLocationCommand(t, 1, prioritisedAs(2))
	second.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-02")
	expectLocationOutcome(t, "二号仓下同编码的库位（内容不同也不冲突）", mustRegisterLocation(t, handler, second), application.RegistrationRegistered)

	other := registerLocationCommand(t, 1, prioritisedAs(3))
	other.Tenant = valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	expectLocationOutcome(t, "另一个租户同仓同编码的库位", mustRegisterLocation(t, handler, other), application.RegistrationRegistered)

	spelled := registerLocationCommand(t, 1, nil)
	spelled.Code = valueOf(t, domain.NewLocationCode, "syn-loc-01")
	expectLocationOutcome(t, "只差大小写的库位编码", mustRegisterLocation(t, handler, spelled), application.RegistrationRegistered)

	if len(fixture.locations.rows) != 4 {
		t.Fatalf("册里有 %d 个库位，want 4 个互相独立的库位", len(fixture.locations.rows))
	}
}

// Covers: 重放只比内容，不比登记责任方与登记时刻——换一个登记责任方、晚一些再投仍答 ALREADY_REGISTERED，在册那一笔的登记责任方
// 与登记时刻不变，也不再保存。
func TestRegisterLocationReplayByAnotherRegistrantKeepsTheOriginalRegistrant(t *testing.T) {
	fixture := newLocationFixture(t)
	clock := &fixedClock{at: registrationInstant}
	handler := application.NewRegisterLocationHandler(fixture.locations, fixture.warehouses, fixture.zones, fixture.types, clock)
	expectLocationOutcome(t, "修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil)), application.RegistrationRegistered)

	clock.at = registrationInstant.Add(72 * time.Hour)
	another := registerLocationCommand(t, 1, nil)
	another.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	replay := mustRegisterLocation(t, handler, another)
	expectLocationOutcome(t, "换登记责任方重放", replay, application.RegistrationAlreadyRegistered)

	rows := fixture.locations.revisionsOf("SYN-LOC-01")
	if len(rows) != 1 || rows[0].Registrant().String() != "operator-1" || !rows[0].RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("重放之后在册的登记责任方 / 登记时刻变了：%q %v", rows[0].Registrant(), rows[0].RegisteredAt())
	}
	if fixture.locations.saves != 1 {
		t.Fatalf("重放保存了 %d 次，want 只有首笔那 1 次", fixture.locations.saves)
	}
	echoed, has := replay.Revision()
	if !has || echoed.Registrant().String() != "operator-1" {
		t.Fatalf("重放回显的登记责任方 = %q（%v），want 在册那一笔的 operator-1", echoed.Registrant(), has)
	}
}

// 重放与冲突要找的是任何一笔在册的修订，不只是最新那一笔——在册有 1、2，原样重投 1 答 ALREADY_REGISTERED，对 1 换内容答
// CONTENT_CONFLICT；两者都不写册。
func TestRegisterLocationJudgesReplayAgainstAnyRevisionOnTheRegister(t *testing.T) {
	fixture := newLocationFixture(t)
	handler := fixture.handler()
	expectLocationOutcome(t, "修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(1))), application.RegistrationRegistered)
	expectLocationOutcome(t, "修订 2", mustRegisterLocation(t, handler, registerLocationCommand(t, 2, prioritisedAs(2))), application.RegistrationRegistered)

	expectLocationOutcome(t, "重投旧修订 1", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(1))), application.RegistrationAlreadyRegistered)
	expectLocationOutcome(t, "对旧修订 1 换内容", mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(3))), application.RegistrationContentConflict)
	if fixture.locations.saves != 2 || len(fixture.locations.revisionsOf("SYN-LOC-01")) != 2 {
		t.Fatalf("保存 %d 次、落册 %d 笔，want 2 次、2 笔", fixture.locations.saves, len(fixture.locations.revisionsOf("SYN-LOC-01")))
	}
}

// 输入被拒是已形成的答案，不是技术失败：以上各格都经 mustRegisterLocation 断言 error 为空。这里证反面——册读写不动才返回 error，
// 并且不折成任何一格业务结果（折成「从未登记」会让登记方从修订 1 重登一个本来就在册的库位，折成「查无所指」会把登记方打发去登记
// 一个本来就在册的仓、库区或库位类型）。
func TestRegisterLocationReturnsAnErrorOnlyWhenTheRegistersCannotBeReadOrWritten(t *testing.T) {
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*locationFixture){
		"按修订号读不动":   func(fixture *locationFixture) { fixture.locations.loadErr = boom },
		"读最新修订读不动":  func(fixture *locationFixture) { fixture.locations.loadLatestErr = boom },
		"读仓册读不动":    func(fixture *locationFixture) { fixture.warehouses.err = boom },
		"读库区册读不动":   func(fixture *locationFixture) { fixture.zones.err = boom },
		"读库位类型册读不动": func(fixture *locationFixture) { fixture.types.err = boom },
		"保存不动":      func(fixture *locationFixture) { fixture.locations.saveErr = boom },
		"保存答出集外的落点": func(fixture *locationFixture) { fixture.locations.saveOutcome = ports.RegistrySaveOutcome(99) },
	} {
		fixture := newLocationFixture(t)
		arrange(fixture)
		result, err := fixture.handler().Register(t.Context(), registerLocationCommand(t, 1, typedAs(t, "SYN-LT-01")))
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
func TestRegisterLocationTreatsUnconstructedIdentityAsAnErrorAndUnconstructedContentAsInvalid(t *testing.T) {
	fixture := newLocationFixture(t)
	handler := fixture.handler()

	noTenant := registerLocationCommand(t, 1, nil)
	noTenant.Tenant = domain.TenantID{}
	noWarehouse := registerLocationCommand(t, 1, nil)
	noWarehouse.Warehouse = domain.WarehouseCode{}
	noLocation := registerLocationCommand(t, 1, nil)
	noLocation.Code = domain.LocationCode{}
	noRegistrant := registerLocationCommand(t, 1, nil)
	noRegistrant.Registrant = domain.Registrant{}
	for name, command := range map[string]application.RegisterLocationCommand{
		"缺租户": noTenant, "缺仓编码": noWarehouse, "缺库位编码": noLocation, "缺登记责任方": noRegistrant,
	} {
		if _, err := handler.Register(t.Context(), command); err == nil {
			t.Errorf("%s：应按技术失败上抛", name)
		}
	}

	noContent := registerLocationCommand(t, 1, nil)
	noContent.Content = domain.LocationContent{}
	expectLocationRefusal(t, "零值内容", mustRegisterLocation(t, handler, noContent),
		application.RegistrationRefusalContentInvalid, domain.LocationFieldZoneCode)
	if fixture.locations.saves != 0 {
		t.Fatalf("保存了 %d 次，want 0", fixture.locations.saves)
	}
}

// 登记时刻取自注入的时钟，一次登记只读一次；回显的修订带它。
func TestRegisterLocationStampsTheRegistrationInstantFromTheClock(t *testing.T) {
	fixture := newLocationFixture(t)
	result := mustRegisterLocation(t, fixture.handler(), registerLocationCommand(t, 1, nil))
	landed, has := result.Revision()
	if !has || !landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("登记时刻 = %v（%v），want 时钟给的 %v", landed.RegisteredAt(), has, registrationInstant.Truncate(time.Microsecond))
	}
	if stored := fixture.locations.revisionsOf("SYN-LOC-01")[0]; !stored.RegisteredAt().Equal(landed.RegisteredAt()) {
		t.Fatalf("落册的登记时刻 %v 与回显的 %v 不一致", stored.RegisteredAt(), landed.RegisteredAt())
	}
}

// 库位的结果与仓、库区、库位类型的结果是同一个泛型结果的几个实例：各格只在回显的修订与「指出哪一格」的取值上不同，零值都是
// 「没有结果」。
func TestLocationResultIsAnInstanceOfTheSharedAlgebra(t *testing.T) {
	var _ application.RegistrationResult[domain.LocationRevision, domain.LocationField] = application.RegisterLocationResult{}
	if (application.RegisterLocationResult{}).Outcome() != application.RegistrationOutcomeInvalid {
		t.Errorf("零值结果该是没有结果")
	}
}

// 拒绝理由只在 NOT_ACCEPTED 时有值，指出的格随 IMMUTABLE_FIELD、REFERENCE_NOT_FOUND、REFERENCE_DEACTIVATED 带上，回显的修订只在
// REGISTERED 与 ALREADY_REGISTERED 时有：别的结果上读它们得零值。
func TestRegisterLocationResultCarriesDetailOnlyWhereItBelongs(t *testing.T) {
	fixture := newLocationFixture(t)
	fixture.putLocationType(t, "SYN-TENANT-01", "SYN-LT-GONE", 1, true)
	handler := fixture.handler()
	registered := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, nil))
	if registered.Refusal() != application.RegistrationRefusalNone || registered.Field() != domain.LocationFieldInvalid || registered.LatestRevision() != 0 {
		t.Errorf("REGISTERED 带了拒绝细节：%s %q %d", registered.Refusal(), registered.Field(), registered.LatestRevision())
	}
	conflict := mustRegisterLocation(t, handler, registerLocationCommand(t, 1, prioritisedAs(1)))
	if _, has := conflict.Revision(); has || conflict.Refusal() != application.RegistrationRefusalNone {
		t.Errorf("CONTENT_CONFLICT 不该带回显的修订或拒绝理由")
	}
	refused := mustRegisterLocation(t, handler, registerLocationCommand(t, 5, nil))
	if _, has := refused.Revision(); has || !strings.Contains(refused.Refusal().String(), "SEQUENCE") {
		t.Errorf("NOT_ACCEPTED 不该带回显的修订")
	}
	deactivated := mustRegisterLocation(t, handler, registerLocationCommand(t, 2, typedAs(t, "SYN-LT-GONE")))
	if _, has := deactivated.Revision(); has || deactivated.Refusal().String() != "REFERENCE_DEACTIVATED" ||
		deactivated.Field().String() != "locationTypeCode" || deactivated.LatestRevision() != 0 {
		t.Errorf("REFERENCE_DEACTIVATED 的细节不对：%s %q", deactivated.Refusal(), deactivated.Field())
	}
}
