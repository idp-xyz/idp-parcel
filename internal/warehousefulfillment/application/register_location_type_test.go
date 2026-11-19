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

// 本文件证库位类型登记用例（UC-WF-001 的库位类型修订）：判定次序、结果代数、「一个仓都没登记」答未配置与并发。库位类型册与仓册用内存
// 替身，真库上的重放、冲突与并发另在 adapters/postgres 的用例里证。租户与库位类型都是隔离合成值，只记 `S`。

// fakeLocationTypeRegistry 是库位类型登记册的内存替身：同键同内容答已在册、同键异内容答冲突，与真适配器同判据。保存在锁里串行，等于主键
// 唯一约束；afterLookup 与 beforeSave 让并发用例逼出真实的竞态窗口。
type fakeLocationTypeRegistry struct {
	mutex         sync.Mutex
	rows          map[string][]domain.LocationTypeRevision
	saves         int
	loadErr       error
	loadLatestErr error
	saveErr       error
	saveOutcome   ports.RegistrySaveOutcome
	afterLookup   func()
	beforeSave    func()
}

func newFakeLocationTypeRegistry() *fakeLocationTypeRegistry {
	return &fakeLocationTypeRegistry{rows: make(map[string][]domain.LocationTypeRevision)}
}

func locationTypeKey(tenant domain.TenantID, code domain.LocationTypeCode) string {
	return tenant.String() + "/" + code.String()
}

func (fake *fakeLocationTypeRegistry) LoadLocationTypeRevision(
	_ context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
	revision int,
) (domain.LocationTypeRevision, bool, error) {
	if fake.loadErr != nil {
		return domain.LocationTypeRevision{}, false, fake.loadErr
	}
	fake.mutex.Lock()
	var found domain.LocationTypeRevision
	var has bool
	for _, row := range fake.rows[locationTypeKey(tenant, code)] {
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

func (fake *fakeLocationTypeRegistry) LoadLatestLocationType(
	_ context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) (domain.LocationTypeRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.LocationTypeRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var latest domain.LocationTypeRevision
	var has bool
	for _, row := range fake.rows[locationTypeKey(tenant, code)] {
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has, nil
}

func (fake *fakeLocationTypeRegistry) SaveLocationTypeRevision(
	_ context.Context,
	revision domain.LocationTypeRevision,
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
	key := locationTypeKey(revision.Tenant(), revision.Code())
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
func (fake *fakeLocationTypeRegistry) seed(revision domain.LocationTypeRevision) {
	key := locationTypeKey(revision.Tenant(), revision.Code())
	fake.rows[key] = append(fake.rows[key], revision)
}

func (fake *fakeLocationTypeRegistry) revisionsOf(code string) []domain.LocationTypeRevision {
	return fake.rows["SYN-TENANT-01/"+code]
}

// fakeTenantWarehouses 是「本租户有没有登记过仓」的内存替身，按租户记。asked 数问过几次。
type fakeTenantWarehouses struct {
	registered map[string]bool
	err        error
	asked      int
}

func newFakeTenantWarehouses(tenants ...string) *fakeTenantWarehouses {
	fake := &fakeTenantWarehouses{registered: map[string]bool{}}
	for _, tenant := range tenants {
		fake.registered[tenant] = true
	}
	return fake
}

func (fake *fakeTenantWarehouses) HasAnyWarehouse(_ context.Context, tenant domain.TenantID) (bool, error) {
	fake.asked++
	if fake.err != nil {
		return false, fake.err
	}
	return fake.registered[tenant.String()], nil
}

func locationTypeContentWith(t *testing.T, edit func(*domain.LocationTypeContentSpec)) domain.LocationTypeContent {
	t.Helper()
	spec := domain.LocationTypeContentSpec{Name: valueOf(t, domain.NewLocationTypeName, "合成货架位")}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationTypeContent(spec)
	if err != nil {
		t.Fatalf("NewLocationTypeContent: %v", err)
	}
	return content
}

func registerLocationTypeCommand(t *testing.T, revision int, edit func(*domain.LocationTypeContentSpec)) application.RegisterLocationTypeCommand {
	t.Helper()
	return application.RegisterLocationTypeCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:       valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"),
		Revision:   revision,
		Content:    locationTypeContentWith(t, edit),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

// locationTypeStores 是一组登记用例的替身：租户 SYN-TENANT-01 已登记过仓。
func locationTypeStores() (*fakeLocationTypeRegistry, *fakeTenantWarehouses) {
	return newFakeLocationTypeRegistry(), newFakeTenantWarehouses("SYN-TENANT-01")
}

func newLocationTypeHandler(registry *fakeLocationTypeRegistry, warehouses *fakeTenantWarehouses) *application.RegisterLocationTypeHandler {
	return application.NewRegisterLocationTypeHandler(registry, warehouses, &fixedClock{at: registrationInstant})
}

func mustRegisterLocationType(
	t *testing.T,
	handler *application.RegisterLocationTypeHandler,
	command application.RegisterLocationTypeCommand,
) application.RegisterLocationTypeResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register 修订 %d：业务格不该走 error：%v", command.Revision, err)
	}
	return result
}

func expectLocationTypeOutcome(t *testing.T, label string, result application.RegisterLocationTypeResult, want application.RegistrationOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s（拒绝理由 %s，指向 %s），want %s", label, result.Outcome(), result.Refusal(), result.Field(), want)
	}
}

func expectLocationTypeRefusal(
	t *testing.T,
	label string,
	result application.RegisterLocationTypeResult,
	want application.RegistrationRefusal,
	wantField domain.LocationTypeField,
) {
	t.Helper()
	expectLocationTypeOutcome(t, label, result, application.RegistrationNotAccepted)
	if result.Refusal() != want || result.Field() != wantField {
		t.Fatalf("%s：拒绝理由 = %s 指向 %q，want %s 指向 %q", label, result.Refusal(), result.Field(), want, wantField)
	}
}

// Covers: AT-WF-001（库位类型的部分）——首次登记答 REGISTERED 且修订 1 落册；原样重投答 ALREADY_REGISTERED、册里仍只有一份；同一修订号
// 换内容答 CONTENT_CONFLICT、在册不变；首笔修订号给 2 答 NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 并回显 0；已有修订 1 时给 3 同样答
// 错位并回显 1。
func TestRegisterLocationTypeFirstRevisionReplayConflictAndOutOfSequence(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)

	first := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil))
	expectLocationTypeOutcome(t, "首次登记", first, application.RegistrationRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("回显的修订 = %+v（%v），want 修订 1、登记责任方 operator-1、时钟给的登记时刻", landed, has)
	}
	if len(registry.revisionsOf("SYN-LT-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(registry.revisionsOf("SYN-LT-01")))
	}

	replay := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil))
	expectLocationTypeOutcome(t, "原样重投", replay, application.RegistrationAlreadyRegistered)
	if registry.saves != 1 || len(registry.revisionsOf("SYN-LT-01")) != 1 {
		t.Fatalf("重放之后保存 %d 次、落册 %d 笔，want 1 次、1 笔", registry.saves, len(registry.revisionsOf("SYN-LT-01")))
	}

	conflict := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（改名）")
	}))
	expectLocationTypeOutcome(t, "同一修订号换内容", conflict, application.RegistrationContentConflict)
	if got := registry.revisionsOf("SYN-LT-01")[0].Content().Name().String(); got != "合成货架位" {
		t.Fatalf("冲突之后在册的名称 = %q，在册的不该变", got)
	}

	skipped := registerLocationTypeCommand(t, 2, nil)
	skipped.Code = valueOf(t, domain.NewLocationTypeCode, "SYN-LT-02")
	result := mustRegisterLocationType(t, handler, skipped)
	expectLocationTypeRefusal(t, "首笔给 2", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationTypeFieldInvalid)
	if result.LatestRevision() != 0 || len(registry.revisionsOf("SYN-LT-02")) != 0 {
		t.Fatalf("首笔错位回显 %d、落册 %d 笔，want 回显 0、不落册", result.LatestRevision(), len(registry.revisionsOf("SYN-LT-02")))
	}

	result = mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 3, nil))
	expectLocationTypeRefusal(t, "已有修订 1 时给 3", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationTypeFieldInvalid)
	if result.LatestRevision() != 1 || len(registry.revisionsOf("SYN-LT-01")) != 1 {
		t.Fatalf("跳号回显 %d、落册 %d 笔，want 回显 1、仍是 1 笔", result.LatestRevision(), len(registry.revisionsOf("SYN-LT-01")))
	}
}

// 修订号小于 1 不是「最新加一」：与跳号同答错位并回显在册最新修订号，不另造一格；一个仓都没登记时仍先答未配置（次序在核修订连续之前）。
func TestRegisterLocationTypeTreatsANonPositiveRevisionNumberAsOutOfSequence(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)

	for _, number := range []int{0, -1} {
		result := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, number, nil))
		expectLocationTypeRefusal(t, fmt.Sprintf("空册给 %d", number), result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationTypeFieldInvalid)
		if result.LatestRevision() != 0 {
			t.Fatalf("空册给 %d 回显 %d，want 0", number, result.LatestRevision())
		}
	}
	expectLocationTypeOutcome(t, "修订 1", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)
	result := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 0, nil))
	expectLocationTypeRefusal(t, "已有修订 1 时给 0", result, application.RegistrationRefusalRevisionOutOfSequence, domain.LocationTypeFieldInvalid)
	if result.LatestRevision() != 1 || registry.saves != 1 {
		t.Fatalf("回显 %d、保存 %d 次，want 回显 1、保存 1 次", result.LatestRevision(), registry.saves)
	}

	bare, none := newFakeLocationTypeRegistry(), newFakeTenantWarehouses()
	for _, number := range []int{0, -1} {
		result := mustRegisterLocationType(t, newLocationTypeHandler(bare, none), registerLocationTypeCommand(t, number, nil))
		expectLocationTypeOutcome(t, fmt.Sprintf("一个仓都没登记时给 %d", number), result, application.RegistrationNotConfigured)
	}
}

// Covers: AT-WF-001（库位类型的部分）最后一行——并发提交同一修订号、内容相同的两笔，一笔答 REGISTERED、一笔答 ALREADY_REGISTERED；
// 册里只有一份，答重放的那一笔回显的是已落册的那一份（登记责任方是先落册者的）。内容不同时一笔落册、另一笔答 CONTENT_CONFLICT。
// 两笔都停在「已查过在册」之后再各自保存，逼的是真实的竞态窗口。
func TestRegisterLocationTypeConcurrentSubmissionsOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		wantLoser  application.RegistrationOutcome
	}{
		"内容相同": {secondName: "合成货架位", wantLoser: application.RegistrationAlreadyRegistered},
		"内容不同": {secondName: "合成货架位（另一份）", wantLoser: application.RegistrationContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses := locationTypeStores()
			meeting := newRendezvous(2)
			registry.afterLookup = meeting.arrive
			handler := newLocationTypeHandler(registry, warehouses)

			commands := []application.RegisterLocationTypeCommand{
				registerLocationTypeCommand(t, 1, nil),
				registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
					spec.Name = valueOf(t, domain.NewLocationTypeName, test.secondName)
				}),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")

			results := make([]application.RegisterLocationTypeResult, len(commands))
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
			landed, others := 0, []application.RegisterLocationTypeResult{}
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
			rows := registry.revisionsOf("SYN-LT-01")
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
// 后一个窗口里靠登记册的主键，保存按内容答已在册或冲突。两个窗口的答案必须一样（仓那一册里处理过的同一个竞态，库位类型同样要守）。
func TestRegisterLocationTypeJudgesAConcurrentLandingByContentInBothRaceWindows(t *testing.T) {
	for windowName, install := range map[string]func(*fakeLocationTypeRegistry, func()){
		"查册之后": func(registry *fakeLocationTypeRegistry, land func()) { registry.afterLookup = land },
		"保存之前": func(registry *fakeLocationTypeRegistry, land func()) { registry.beforeSave = land },
	} {
		for contentName, test := range map[string]struct {
			competitorName string
			want           application.RegistrationOutcome
		}{
			"内容相同": {"合成货架位", application.RegistrationAlreadyRegistered},
			"内容不同": {"合成货架位（别人先落的）", application.RegistrationContentConflict},
		} {
			t.Run(windowName+"/"+contentName, func(t *testing.T) {
				registry, warehouses := locationTypeStores()
				competitorCommand := registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
					spec.Name = valueOf(t, domain.NewLocationTypeName, test.competitorName)
				})
				competitorCommand.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
				competitor, err := domain.NewLocationTypeRevision(
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

				result := mustRegisterLocationType(t, newLocationTypeHandler(registry, warehouses), registerLocationTypeCommand(t, 1, nil))
				expectLocationTypeOutcome(t, "别人先落册之后的这一笔", result, test.want)
				if !landed || len(registry.revisionsOf("SYN-LT-01")) != 1 {
					t.Fatalf("竞争者落册 %v、册里 %d 笔，want 已落、只有 1 笔", landed, len(registry.revisionsOf("SYN-LT-01")))
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

// Covers: AT-WF-009（库位类型的部分）——一个仓都没登记的租户：登记库位类型答 NOT_CONFIGURED，不带拒绝理由、指出的格与最新修订号，不落册、
// 不回显修订；它不是拒绝，所以不是 NOT_ACCEPTED。之后登记了仓，同一修订原样重交照常受理（那一笔当初没有落册，重交不是重放）。
func TestRegisterLocationTypeBeforeAnyWarehouseIsNotConfiguredAndTheSameRevisionLandsAfterwards(t *testing.T) {
	registry := newFakeLocationTypeRegistry()
	warehouses := newFakeTenantWarehouses()
	handler := newLocationTypeHandler(registry, warehouses)

	result := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil))
	expectLocationTypeOutcome(t, "一个仓都没登记", result, application.RegistrationNotConfigured)
	if _, has := result.Revision(); has || result.Refusal() != application.RegistrationRefusalNone ||
		result.Field() != domain.LocationTypeFieldInvalid || result.LatestRevision() != 0 {
		t.Fatalf("NOT_CONFIGURED 带了多余的细节：%+v", result)
	}
	if registry.saves != 0 || len(registry.rows) != 0 {
		t.Fatalf("未配置之后保存 %d 次、册里 %d 个库位类型，want 不落册", registry.saves, len(registry.rows))
	}

	warehouses.registered["SYN-TENANT-01"] = true
	expectLocationTypeOutcome(t, "登记仓之后原样重交", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)
	if len(registry.revisionsOf("SYN-LT-01")) != 1 {
		t.Fatalf("重交之后落册 %d 笔，want 1", len(registry.revisionsOf("SYN-LT-01")))
	}
}

// Covers: AT-WF-009（库位类型的部分）——别的租户登记了仓不算本租户的仓：本租户一个仓都没登记时仍答 NOT_CONFIGURED，与别的租户有没有仓无关，
// 不泄露别的租户的任何事。
func TestRegisterLocationTypeIgnoresWarehousesOfOtherTenants(t *testing.T) {
	registry := newFakeLocationTypeRegistry()
	warehouses := newFakeTenantWarehouses("SYN-TENANT-02")
	handler := newLocationTypeHandler(registry, warehouses)

	expectLocationTypeOutcome(t, "别的租户登记了仓", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationNotConfigured)

	other := registerLocationTypeCommand(t, 1, nil)
	other.Tenant = valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	expectLocationTypeOutcome(t, "登记了仓的那个租户自己", mustRegisterLocationType(t, handler, other), application.RegistrationRegistered)
}

// Covers: 重放先于 NOT_CONFIGURED 与修订连续——先按（身份，修订号）找在册的那一笔，重放与冲突不再过后面的核。册上早就有的
// 修订，哪怕此刻本租户一个仓都问不到，原样重投仍是重放，换内容仍是冲突，不能变成未配置；仓册一次都没被问到。
func TestRegisterLocationTypeJudgesReplayAndConflictBeforeNotConfiguredAndSequence(t *testing.T) {
	registry := newFakeLocationTypeRegistry()
	warehouses := newFakeTenantWarehouses()
	handler := newLocationTypeHandler(registry, warehouses)
	seeded := registerLocationTypeCommand(t, 1, nil)
	revision, err := domain.NewLocationTypeRevision(seeded.Tenant, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	registry.seed(revision)

	expectLocationTypeOutcome(t, "一个仓都问不到时重放在册的修订", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectLocationTypeOutcome(t, "对它换内容", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "另一个名字")
	})), application.RegistrationContentConflict)
	if registry.saves != 0 || warehouses.asked != 0 {
		t.Fatalf("重放与冲突保存了 %d 次、问了仓册 %d 次，want 都是 0", registry.saves, warehouses.asked)
	}
}

// 未落册的修订依次核：一个仓都没登记 → 修订号连续。前面一格不成立就答前面那一格，不继续往后核：册上有修订 1、本租户一个仓都问不到时，
// 给修订 3 答未配置而不是错位。
func TestRegisterLocationTypeChecksNotConfiguredThenSequence(t *testing.T) {
	registry := newFakeLocationTypeRegistry()
	warehouses := newFakeTenantWarehouses()
	handler := newLocationTypeHandler(registry, warehouses)
	seeded := registerLocationTypeCommand(t, 1, nil)
	revision, err := domain.NewLocationTypeRevision(seeded.Tenant, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	registry.seed(revision)

	expectLocationTypeOutcome(t, "没有仓且跳号", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 3, nil)), application.RegistrationNotConfigured)
	warehouses.registered["SYN-TENANT-01"] = true
	expectLocationTypeRefusal(t, "有仓且跳号", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 3, nil)),
		application.RegistrationRefusalRevisionOutOfSequence, domain.LocationTypeFieldInvalid)
	if registry.saves != 0 {
		t.Fatalf("各种未受理之后保存 %d 次，want 0", registry.saves)
	}
}

// Covers: 停用与再启用各是一笔新修订，都答 REGISTERED；停用不拦此后的修订（停用期间照常改名），旧修订照留；库位类型没有登记后不改的格，
// 所以名称随时可改。
func TestRegisterLocationTypeDeactivationAndReactivationAreNewRevisions(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)
	expectLocationTypeOutcome(t, "修订 1", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)
	expectLocationTypeOutcome(t, "停用", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 2, func(spec *domain.LocationTypeContentSpec) {
		spec.Deactivated = true
	})), application.RegistrationRegistered)
	expectLocationTypeOutcome(t, "停用期间改名", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 3, func(spec *domain.LocationTypeContentSpec) {
		spec.Deactivated = true
		spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（停用中）")
	})), application.RegistrationRegistered)
	expectLocationTypeOutcome(t, "再启用", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 4, nil)), application.RegistrationRegistered)

	rows := registry.revisionsOf("SYN-LT-01")
	if len(rows) != 4 || rows[0].Content().Deactivated() || !rows[1].Content().Deactivated() ||
		!rows[2].Content().Deactivated() || rows[3].Content().Deactivated() {
		t.Fatalf("停用与再启用之后的册不对：四笔的停用标记该依次是 否、是、是、否")
	}
}

// Covers: 重放只比内容，不比登记责任方与登记时刻——换一个登记责任方、晚一些再投仍答 ALREADY_REGISTERED，在册那一笔的登记责任方与登记时刻
// 不变，也不再保存。
func TestRegisterLocationTypeReplayByAnotherRegistrantKeepsTheOriginalRegistrant(t *testing.T) {
	registry, warehouses := locationTypeStores()
	clock := &fixedClock{at: registrationInstant}
	handler := application.NewRegisterLocationTypeHandler(registry, warehouses, clock)
	expectLocationTypeOutcome(t, "修订 1", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)

	clock.at = registrationInstant.Add(72 * time.Hour)
	another := registerLocationTypeCommand(t, 1, nil)
	another.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	replay := mustRegisterLocationType(t, handler, another)
	expectLocationTypeOutcome(t, "换登记责任方重放", replay, application.RegistrationAlreadyRegistered)

	rows := registry.revisionsOf("SYN-LT-01")
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

// Covers: 重放与冲突要找的是任何一笔在册的修订，不只是最新那一笔——在册有 1、2，原样重投 1 答 ALREADY_REGISTERED，对 1 换内容答
// CONTENT_CONFLICT；两者都不写册。
func TestRegisterLocationTypeJudgesReplayAgainstAnyRevisionOnTheRegister(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)
	expectLocationTypeOutcome(t, "修订 1", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)
	expectLocationTypeOutcome(t, "修订 2", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 2, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（二）")
	})), application.RegistrationRegistered)

	expectLocationTypeOutcome(t, "重投旧修订 1", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectLocationTypeOutcome(t, "对旧修订 1 换内容", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "另一个名字")
	})), application.RegistrationContentConflict)
	if registry.saves != 2 || len(registry.revisionsOf("SYN-LT-01")) != 2 {
		t.Fatalf("保存 %d 次、落册 %d 笔，want 2 次、2 笔", registry.saves, len(registry.revisionsOf("SYN-LT-01")))
	}
}

// 库位类型各仓共用、按租户登记：同一个编码在另一个租户名下是另一个库位类型，各自从修订 1 起；只差大小写的编码也是另一个库位类型。
func TestRegisterLocationTypeKeepsTypesOfDifferentTenantsAndSpellingsApart(t *testing.T) {
	registry := newFakeLocationTypeRegistry()
	warehouses := newFakeTenantWarehouses("SYN-TENANT-01", "SYN-TENANT-02")
	handler := newLocationTypeHandler(registry, warehouses)

	expectLocationTypeOutcome(t, "本租户", mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil)), application.RegistrationRegistered)
	other := registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "另一租户的货架位")
	})
	other.Tenant = valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	expectLocationTypeOutcome(t, "另一个租户同编码、不同内容", mustRegisterLocationType(t, handler, other), application.RegistrationRegistered)
	lower := registerLocationTypeCommand(t, 1, nil)
	lower.Code = valueOf(t, domain.NewLocationTypeCode, "syn-lt-01")
	expectLocationTypeOutcome(t, "只差大小写的编码", mustRegisterLocationType(t, handler, lower), application.RegistrationRegistered)

	if len(registry.rows) != 3 {
		t.Fatalf("册里有 %d 个库位类型，want 3 个互相独立的库位类型", len(registry.rows))
	}
}

// 输入被拒是已形成的答案，不是技术失败：以上各格都经 mustRegisterLocationType 断言 error 为空。这里证反面——册读写不动才返回 error，
// 并且不折成任何一格业务结果（折成「一个仓都没登记」会让登记方被打发去登记一个本来就在册的仓）。
func TestRegisterLocationTypeReturnsAnErrorOnlyWhenTheRegistersCannotBeReadOrWritten(t *testing.T) {
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*fakeLocationTypeRegistry, *fakeTenantWarehouses){
		"按修订号读不动":  func(registry *fakeLocationTypeRegistry, _ *fakeTenantWarehouses) { registry.loadErr = boom },
		"读最新修订读不动": func(registry *fakeLocationTypeRegistry, _ *fakeTenantWarehouses) { registry.loadLatestErr = boom },
		"读仓册读不动":   func(_ *fakeLocationTypeRegistry, warehouses *fakeTenantWarehouses) { warehouses.err = boom },
		"保存不动":     func(registry *fakeLocationTypeRegistry, _ *fakeTenantWarehouses) { registry.saveErr = boom },
		"保存答出集外的落点": func(registry *fakeLocationTypeRegistry, _ *fakeTenantWarehouses) {
			registry.saveOutcome = ports.RegistrySaveOutcome(99)
		},
	} {
		registry, warehouses := locationTypeStores()
		arrange(registry, warehouses)
		result, err := newLocationTypeHandler(registry, warehouses).Register(t.Context(), registerLocationTypeCommand(t, 1, nil))
		if err == nil {
			t.Errorf("%s：err 为空，结果 = %s", name, result.Outcome())
		}
		if result.Outcome() != application.RegistrationOutcomeInvalid {
			t.Errorf("%s：结果 = %s，error 之外不该另带一格业务结果", name, result.Outcome())
		}
	}

	// 修订号立不住的命令也要问仓册：读仓册读不动同样走 error，不折成未配置。
	registry, warehouses := locationTypeStores()
	warehouses.err = boom
	if _, err := newLocationTypeHandler(registry, warehouses).Register(t.Context(), registerLocationTypeCommand(t, 0, nil)); err == nil {
		t.Errorf("修订号为 0 时读仓册读不动：应按技术失败上抛")
	}
}

// 登记口拒绝的是内容；身份与登记责任方没经构造器（零值）是接入侧装配错了，不是登记方的输入，按技术失败上抛。内容里缺格（零值内容）
// 仍是内容不合并指出缺哪一格。
func TestRegisterLocationTypeTreatsUnconstructedIdentityAsAnErrorAndUnconstructedContentAsInvalid(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)

	noTenant := registerLocationTypeCommand(t, 1, nil)
	noTenant.Tenant = domain.TenantID{}
	noCode := registerLocationTypeCommand(t, 1, nil)
	noCode.Code = domain.LocationTypeCode{}
	noRegistrant := registerLocationTypeCommand(t, 1, nil)
	noRegistrant.Registrant = domain.Registrant{}
	for name, command := range map[string]application.RegisterLocationTypeCommand{
		"缺租户": noTenant, "缺库位类型编码": noCode, "缺登记责任方": noRegistrant,
	} {
		if _, err := handler.Register(t.Context(), command); err == nil {
			t.Errorf("%s：应按技术失败上抛", name)
		}
	}

	noContent := registerLocationTypeCommand(t, 1, nil)
	noContent.Content = domain.LocationTypeContent{}
	expectLocationTypeRefusal(t, "零值内容", mustRegisterLocationType(t, handler, noContent),
		application.RegistrationRefusalContentInvalid, domain.LocationTypeFieldName)
	if registry.saves != 0 {
		t.Fatalf("保存了 %d 次，want 0", registry.saves)
	}
}

// 登记时刻取自注入的时钟，一次登记只读一次；回显的修订带它。
func TestRegisterLocationTypeStampsTheRegistrationInstantFromTheClock(t *testing.T) {
	registry, warehouses := locationTypeStores()
	result := mustRegisterLocationType(t, newLocationTypeHandler(registry, warehouses), registerLocationTypeCommand(t, 1, nil))
	landed, has := result.Revision()
	if !has || !landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("登记时刻 = %v（%v），want 时钟给的 %v", landed.RegisteredAt(), has, registrationInstant.Truncate(time.Microsecond))
	}
	if stored := registry.revisionsOf("SYN-LT-01")[0]; !stored.RegisteredAt().Equal(landed.RegisteredAt()) {
		t.Fatalf("落册的登记时刻 %v 与回显的 %v 不一致", stored.RegisteredAt(), landed.RegisteredAt())
	}
}

// 库位类型的结果与仓、库区的结果是同一个泛型结果的实例：各格只在回显的修订与「指出哪一格」的取值上不同，零值都是「没有结果」。
func TestLocationTypeResultIsAnInstanceOfTheSharedAlgebra(t *testing.T) {
	var _ application.RegistrationResult[domain.LocationTypeRevision, domain.LocationTypeField] = application.RegisterLocationTypeResult{}
	if (application.RegisterLocationTypeResult{}).Outcome() != application.RegistrationOutcomeInvalid {
		t.Errorf("零值结果该是没有结果")
	}
}

// 拒绝理由只在 NOT_ACCEPTED 时有值，回显的修订只在 REGISTERED 与 ALREADY_REGISTERED 时有：别的结果上读它们得零值。
func TestRegisterLocationTypeResultCarriesDetailOnlyWhereItBelongs(t *testing.T) {
	registry, warehouses := locationTypeStores()
	handler := newLocationTypeHandler(registry, warehouses)
	registered := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, nil))
	if registered.Refusal() != application.RegistrationRefusalNone || registered.Field() != domain.LocationTypeFieldInvalid || registered.LatestRevision() != 0 {
		t.Errorf("REGISTERED 带了拒绝细节：%s %q %d", registered.Refusal(), registered.Field(), registered.LatestRevision())
	}
	conflict := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 1, func(spec *domain.LocationTypeContentSpec) {
		spec.Name = valueOf(t, domain.NewLocationTypeName, "冲突的名字")
	}))
	if _, has := conflict.Revision(); has || conflict.Refusal() != application.RegistrationRefusalNone {
		t.Errorf("CONTENT_CONFLICT 不该带回显的修订或拒绝理由")
	}
	refused := mustRegisterLocationType(t, handler, registerLocationTypeCommand(t, 5, nil))
	if _, has := refused.Revision(); has || !strings.Contains(refused.Refusal().String(), "SEQUENCE") {
		t.Errorf("NOT_ACCEPTED 不该带回显的修订")
	}
}
