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

// 本文件证包材登记用例（UC-WF-001 的包材修订）：判定次序、结果代数、「一个仓都没登记」答未配置、内容门（数值为正、单位在目录里且类别对）与并发。
// 包材册与仓册用内存替身，真库上的重放、冲突与并发另在 adapters/postgres 的用例里证。租户与包材都是隔离合成值，只记 `S`。

// fakePackagingMaterialRegistry 是包材登记册的内存替身：同键同内容答已在册、同键异内容答冲突，与真适配器同判据。保存在锁里串行，等于主键唯一
// 约束；afterLookup 与 beforeSave 让并发用例逼出真实的竞态窗口。
type fakePackagingMaterialRegistry struct {
	mutex         sync.Mutex
	rows          map[string][]domain.PackagingMaterialRevision
	saves         int
	loadErr       error
	loadLatestErr error
	saveErr       error
	saveOutcome   ports.RegistrySaveOutcome
	afterLookup   func()
	beforeSave    func()
}

func newFakePackagingMaterialRegistry() *fakePackagingMaterialRegistry {
	return &fakePackagingMaterialRegistry{rows: make(map[string][]domain.PackagingMaterialRevision)}
}

func packagingMaterialKey(tenant domain.TenantID, code domain.PackagingMaterialCode) string {
	return tenant.String() + "/" + code.String()
}

func (fake *fakePackagingMaterialRegistry) LoadPackagingMaterialRevision(
	_ context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
	revision int,
) (domain.PackagingMaterialRevision, bool, error) {
	if fake.loadErr != nil {
		return domain.PackagingMaterialRevision{}, false, fake.loadErr
	}
	fake.mutex.Lock()
	var found domain.PackagingMaterialRevision
	var has bool
	for _, row := range fake.rows[packagingMaterialKey(tenant, code)] {
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

func (fake *fakePackagingMaterialRegistry) LoadLatestPackagingMaterial(
	_ context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
) (domain.PackagingMaterialRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.PackagingMaterialRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var latest domain.PackagingMaterialRevision
	var has bool
	for _, row := range fake.rows[packagingMaterialKey(tenant, code)] {
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has, nil
}

func (fake *fakePackagingMaterialRegistry) SavePackagingMaterialRevision(
	_ context.Context,
	revision domain.PackagingMaterialRevision,
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
	key := packagingMaterialKey(revision.Tenant(), revision.Code())
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
func (fake *fakePackagingMaterialRegistry) seed(revision domain.PackagingMaterialRevision) {
	key := packagingMaterialKey(revision.Tenant(), revision.Code())
	fake.rows[key] = append(fake.rows[key], revision)
}

func (fake *fakePackagingMaterialRegistry) revisionsOf(code string) []domain.PackagingMaterialRevision {
	return fake.rows["SYN-TENANT-01/"+code]
}

func measureOf(t *testing.T, text string) domain.Measure {
	t.Helper()
	return valueOf(t, domain.NewMeasure, text)
}

func packagingMaterialContentWith(t *testing.T, edit func(*domain.PackagingMaterialContentSpec)) domain.PackagingMaterialContent {
	t.Helper()
	spec := domain.PackagingMaterialContentSpec{
		Name:           valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱"),
		Length:         measureOf(t, "30"),
		Width:          measureOf(t, "20"),
		Height:         measureOf(t, "15.5"),
		DimensionsUnit: valueOf(t, domain.NewUnitCode, "CM"),
		Weight:         measureOf(t, "0.25"),
		WeightUnit:     valueOf(t, domain.NewUnitCode, "KG"),
	}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewPackagingMaterialContent(spec)
	if err != nil {
		t.Fatalf("NewPackagingMaterialContent: %v", err)
	}
	return content
}

func registerPackagingMaterialCommand(t *testing.T, revision int, edit func(*domain.PackagingMaterialContentSpec)) application.RegisterPackagingMaterialCommand {
	t.Helper()
	return application.RegisterPackagingMaterialCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:       valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
		Revision:   revision,
		Content:    packagingMaterialContentWith(t, edit),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

// packagingMaterialStores 是一组登记用例的替身：租户 SYN-TENANT-01 已登记过仓。
func packagingMaterialStores() (*fakePackagingMaterialRegistry, *fakeTenantWarehouses) {
	return newFakePackagingMaterialRegistry(), newFakeTenantWarehouses("SYN-TENANT-01")
}

func newPackagingMaterialHandler(registry *fakePackagingMaterialRegistry, warehouses *fakeTenantWarehouses) *application.RegisterPackagingMaterialHandler {
	return application.NewRegisterPackagingMaterialHandler(registry, warehouses, &fixedClock{at: registrationInstant})
}

func mustRegisterPackagingMaterial(
	t *testing.T,
	handler *application.RegisterPackagingMaterialHandler,
	command application.RegisterPackagingMaterialCommand,
) application.RegisterPackagingMaterialResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register 修订 %d：业务格不该走 error：%v", command.Revision, err)
	}
	return result
}

func expectPackagingMaterialOutcome(t *testing.T, label string, result application.RegisterPackagingMaterialResult, want application.RegistrationOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s（拒绝理由 %s，指向 %s），want %s", label, result.Outcome(), result.Refusal(), result.Field(), want)
	}
}

func expectPackagingMaterialRefusal(
	t *testing.T,
	label string,
	result application.RegisterPackagingMaterialResult,
	want application.RegistrationRefusal,
	wantField domain.PackagingMaterialField,
) {
	t.Helper()
	expectPackagingMaterialOutcome(t, label, result, application.RegistrationNotAccepted)
	if result.Refusal() != want || result.Field() != wantField {
		t.Fatalf("%s：拒绝理由 = %s 指向 %q，want %s 指向 %q", label, result.Refusal(), result.Field(), want, wantField)
	}
}

// Covers: AT-WF-001（包材的部分）——首次登记答 REGISTERED 且修订 1 落册；原样重投答 ALREADY_REGISTERED、册里仍只有一份；同一修订号换内容答
// CONTENT_CONFLICT、在册不变；首笔修订号给 2 答 NOT_ACCEPTED/REVISION_OUT_OF_SEQUENCE 并回显 0；已有修订 1 时给 3 同样答错位并回显 1。
func TestRegisterPackagingMaterialFirstRevisionReplayConflictAndOutOfSequence(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)

	first := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil))
	expectPackagingMaterialOutcome(t, "首次登记", first, application.RegistrationRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("回显的修订 = %+v（%v），want 修订 1、登记责任方 operator-1、时钟给的登记时刻", landed, has)
	}
	if len(registry.revisionsOf("SYN-BOX-01")) != 1 {
		t.Fatalf("落册 %d 笔，want 1", len(registry.revisionsOf("SYN-BOX-01")))
	}

	replay := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil))
	expectPackagingMaterialOutcome(t, "原样重投", replay, application.RegistrationAlreadyRegistered)
	if registry.saves != 1 || len(registry.revisionsOf("SYN-BOX-01")) != 1 {
		t.Fatalf("重放之后保存 %d 次、落册 %d 笔，want 1 次、1 笔", registry.saves, len(registry.revisionsOf("SYN-BOX-01")))
	}

	conflict := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱（改名）")
	}))
	expectPackagingMaterialOutcome(t, "同一修订号换内容", conflict, application.RegistrationContentConflict)
	if got := registry.revisionsOf("SYN-BOX-01")[0].Content().Name().String(); got != "合成一号纸箱" {
		t.Fatalf("冲突之后在册的名称 = %q，在册的不该变", got)
	}

	skipped := registerPackagingMaterialCommand(t, 2, nil)
	skipped.Code = valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-02")
	result := mustRegisterPackagingMaterial(t, handler, skipped)
	expectPackagingMaterialRefusal(t, "首笔给 2", result, application.RegistrationRefusalRevisionOutOfSequence, domain.PackagingMaterialFieldInvalid)
	if result.LatestRevision() != 0 || len(registry.revisionsOf("SYN-BOX-02")) != 0 {
		t.Fatalf("首笔错位回显 %d、落册 %d 笔，want 回显 0、不落册", result.LatestRevision(), len(registry.revisionsOf("SYN-BOX-02")))
	}

	result = mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 3, nil))
	expectPackagingMaterialRefusal(t, "已有修订 1 时给 3", result, application.RegistrationRefusalRevisionOutOfSequence, domain.PackagingMaterialFieldInvalid)
	if result.LatestRevision() != 1 || len(registry.revisionsOf("SYN-BOX-01")) != 1 {
		t.Fatalf("跳号回显 %d、落册 %d 笔，want 回显 1、仍是 1 笔", result.LatestRevision(), len(registry.revisionsOf("SYN-BOX-01")))
	}
}

// 修订号小于 1 不是「最新加一」：与跳号同答错位并回显在册最新修订号，不另造一格；一个仓都没登记时仍先答未配置（次序在核修订连续之前）。
func TestRegisterPackagingMaterialTreatsANonPositiveRevisionNumberAsOutOfSequence(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)

	for _, number := range []int{0, -1} {
		result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, number, nil))
		expectPackagingMaterialRefusal(t, fmt.Sprintf("空册给 %d", number), result, application.RegistrationRefusalRevisionOutOfSequence, domain.PackagingMaterialFieldInvalid)
		if result.LatestRevision() != 0 {
			t.Fatalf("空册给 %d 回显 %d，want 0", number, result.LatestRevision())
		}
	}
	expectPackagingMaterialOutcome(t, "修订 1", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 0, nil))
	expectPackagingMaterialRefusal(t, "已有修订 1 时给 0", result, application.RegistrationRefusalRevisionOutOfSequence, domain.PackagingMaterialFieldInvalid)
	if result.LatestRevision() != 1 || registry.saves != 1 {
		t.Fatalf("回显 %d、保存 %d 次，want 回显 1、保存 1 次", result.LatestRevision(), registry.saves)
	}

	bare, none := newFakePackagingMaterialRegistry(), newFakeTenantWarehouses()
	for _, number := range []int{0, -1} {
		result := mustRegisterPackagingMaterial(t, newPackagingMaterialHandler(bare, none), registerPackagingMaterialCommand(t, number, nil))
		expectPackagingMaterialOutcome(t, fmt.Sprintf("一个仓都没登记时给 %d", number), result, application.RegistrationNotConfigured)
	}
}

// Covers: AT-WF-001（包材的部分）最后一行——并发提交同一修订号、内容相同的两笔，一笔答 REGISTERED、一笔答 ALREADY_REGISTERED；册里只有一份，
// 答重放的那一笔回显的是已落册的那一份（登记责任方是先落册者的）。内容不同时一笔落册、另一笔答 CONTENT_CONFLICT。两笔都停在「已查过在册」
// 之后再各自保存，逼的是真实的竞态窗口。
func TestRegisterPackagingMaterialConcurrentSubmissionsOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		wantLoser  application.RegistrationOutcome
	}{
		"内容相同": {secondName: "合成一号纸箱", wantLoser: application.RegistrationAlreadyRegistered},
		"内容不同": {secondName: "合成一号纸箱（另一份）", wantLoser: application.RegistrationContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses := packagingMaterialStores()
			meeting := newRendezvous(2)
			registry.afterLookup = meeting.arrive
			handler := newPackagingMaterialHandler(registry, warehouses)

			commands := []application.RegisterPackagingMaterialCommand{
				registerPackagingMaterialCommand(t, 1, nil),
				registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
					spec.Name = valueOf(t, domain.NewPackagingMaterialName, test.secondName)
				}),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")

			results := make([]application.RegisterPackagingMaterialResult, len(commands))
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
			landed, others := 0, []application.RegisterPackagingMaterialResult{}
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
			rows := registry.revisionsOf("SYN-BOX-01")
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
// 后一个窗口里靠登记册的主键，保存按内容答已在册或冲突。两个窗口的答案必须一样（仓那一册里处理过的同一个竞态，包材同样要守）。
func TestRegisterPackagingMaterialJudgesAConcurrentLandingByContentInBothRaceWindows(t *testing.T) {
	for windowName, install := range map[string]func(*fakePackagingMaterialRegistry, func()){
		"查册之后": func(registry *fakePackagingMaterialRegistry, land func()) { registry.afterLookup = land },
		"保存之前": func(registry *fakePackagingMaterialRegistry, land func()) { registry.beforeSave = land },
	} {
		for contentName, test := range map[string]struct {
			competitorName string
			want           application.RegistrationOutcome
		}{
			"内容相同": {"合成一号纸箱", application.RegistrationAlreadyRegistered},
			"内容不同": {"合成一号纸箱（别人先落的）", application.RegistrationContentConflict},
		} {
			t.Run(windowName+"/"+contentName, func(t *testing.T) {
				registry, warehouses := packagingMaterialStores()
				competitorCommand := registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
					spec.Name = valueOf(t, domain.NewPackagingMaterialName, test.competitorName)
				})
				competitorCommand.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
				competitor, err := domain.NewPackagingMaterialRevision(
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

				result := mustRegisterPackagingMaterial(t, newPackagingMaterialHandler(registry, warehouses), registerPackagingMaterialCommand(t, 1, nil))
				expectPackagingMaterialOutcome(t, "别人先落册之后的这一笔", result, test.want)
				if !landed || len(registry.revisionsOf("SYN-BOX-01")) != 1 {
					t.Fatalf("竞争者落册 %v、册里 %d 笔，want 已落、只有 1 笔", landed, len(registry.revisionsOf("SYN-BOX-01")))
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

// Covers: AT-WF-004（包材的部分）——尺寸单位不在内置计量单位目录；重量给了长度类的单位；尺寸给了重量类的单位；重量为零；某一边尺寸为零：各答
// NOT_ACCEPTED/CONTENT_INVALID，field 指出那一格，不落册，不取默认值，不代为折算（册里没有、保存没发生）。大小写不同的码不在目录里（按码逐字比），
// 体积与件数类的单位两处都不收，零的各种写法（0、0.0、000）都是零。
func TestRegisterPackagingMaterialRefusesContentThatFailsTheCatalogueAndPositivityGates(t *testing.T) {
	unit := func(code string) domain.UnitCode { return valueOf(t, domain.NewUnitCode, code) }
	for name, test := range map[string]struct {
		edit func(*domain.PackagingMaterialContentSpec)
		want domain.PackagingMaterialField
	}{
		"尺寸单位不在目录里":   {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("CUBIT") }, domain.PackagingMaterialFieldDimensionsUnit},
		"尺寸单位的大小写不对":  {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("cm") }, domain.PackagingMaterialFieldDimensionsUnit},
		"尺寸单位带空白":     {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("CM ") }, domain.PackagingMaterialFieldDimensionsUnit},
		"尺寸给了重量类的单位":  {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("KG") }, domain.PackagingMaterialFieldDimensionsUnit},
		"尺寸给了体积类的单位":  {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("M3") }, domain.PackagingMaterialFieldDimensionsUnit},
		"尺寸给了件数类的单位":  {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = unit("PCS") }, domain.PackagingMaterialFieldDimensionsUnit},
		"重量单位不在目录里":   {func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = unit("STONE") }, domain.PackagingMaterialFieldWeightUnit},
		"重量单位的大小写不对":  {func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = unit("kg") }, domain.PackagingMaterialFieldWeightUnit},
		"重量给了长度类的单位":  {func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = unit("CM") }, domain.PackagingMaterialFieldWeightUnit},
		"重量给了体积类的单位":  {func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = unit("L") }, domain.PackagingMaterialFieldWeightUnit},
		"重量为零":        {func(spec *domain.PackagingMaterialContentSpec) { spec.Weight = measureOf(t, "0") }, domain.PackagingMaterialFieldWeightValue},
		"重量为 0.0":     {func(spec *domain.PackagingMaterialContentSpec) { spec.Weight = measureOf(t, "0.0") }, domain.PackagingMaterialFieldWeightValue},
		"长为零":         {func(spec *domain.PackagingMaterialContentSpec) { spec.Length = measureOf(t, "0") }, domain.PackagingMaterialFieldDimensionsLength},
		"宽为 0.000000": {func(spec *domain.PackagingMaterialContentSpec) { spec.Width = measureOf(t, "0.000000") }, domain.PackagingMaterialFieldDimensionsWidth},
		"高为 000":      {func(spec *domain.PackagingMaterialContentSpec) { spec.Height = measureOf(t, "000") }, domain.PackagingMaterialFieldDimensionsHeight},
		"长宽都为零时先指出长": {func(spec *domain.PackagingMaterialContentSpec) {
			spec.Length, spec.Width = measureOf(t, "0"), measureOf(t, "0")
		}, domain.PackagingMaterialFieldDimensionsLength},
		"零值与坏单位并存时按报文次序先指出尺寸单位": {func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit, spec.Weight = unit("KG"), measureOf(t, "0")
		}, domain.PackagingMaterialFieldDimensionsUnit},
	} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses := packagingMaterialStores()
			handler := newPackagingMaterialHandler(registry, warehouses)

			result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, test.edit))
			expectPackagingMaterialRefusal(t, name, result, application.RegistrationRefusalContentInvalid, test.want)
			if _, has := result.Revision(); has || result.LatestRevision() != 0 {
				t.Fatalf("内容不合的答复带了多余的细节")
			}
			if registry.saves != 0 || len(registry.rows) != 0 {
				t.Fatalf("内容不合之后保存 %d 次、册里 %d 个包材，want 不落册", registry.saves, len(registry.rows))
			}
		})
	}
}

// Covers: AT-WF-004（包材的部分）——登记时不折算单位、不取默认值：30 CM 与 0.3 M 是两份不同的内容，各自按原样落册；目录里的各个长度类与重量类的单位
// 都收。
func TestRegisterPackagingMaterialKeepsTheUnitAsGivenAndAcceptsEveryLengthAndWeightUnit(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)

	expectPackagingMaterialOutcome(t, "30 CM", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	meters := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 2, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureOf(t, "0.3"), measureOf(t, "0.2"), measureOf(t, "0.155")
		spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "M")
	}))
	expectPackagingMaterialOutcome(t, "0.3 M 是新修订，不是 30 CM 的重放", meters, application.RegistrationRegistered)
	rows := registry.revisionsOf("SYN-BOX-01")
	if len(rows) != 2 || rows[0].Content().Length().String() != "30" || rows[0].Content().DimensionsUnit().String() != "CM" ||
		rows[1].Content().Length().String() != "0.3" || rows[1].Content().DimensionsUnit().String() != "M" {
		t.Fatalf("册里的两笔没有按原样落册：%+v", rows)
	}

	next := 3
	for _, lengthUnit := range []string{"MM", "CM", "M", "IN", "FT", "YD"} {
		result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, next, func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, lengthUnit)
		}))
		expectPackagingMaterialOutcome(t, "长度单位 "+lengthUnit, result, application.RegistrationRegistered)
		next++
	}
	for _, weightUnit := range []string{"MG", "G", "KG", "T", "LB", "OZ"} {
		result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, next, func(spec *domain.PackagingMaterialContentSpec) {
			spec.WeightUnit = valueOf(t, domain.NewUnitCode, weightUnit)
		}))
		expectPackagingMaterialOutcome(t, "重量单位 "+weightUnit, result, application.RegistrationRegistered)
		next++
	}
}

// Covers: AT-WF-001、AT-WF-004（包材的部分）「数值按数值比，不按写法比」「回显用去掉多余前导零与尾零的写法」——原样重投换一种数值的写法
// （12.5 与 12.50、015.5 与 15.5、30.00 与 30）答 ALREADY_REGISTERED，不是冲突；回显的是在册那一份，数值是去掉多余零的写法；单位换一个码
// （30 CM 换成 30 M）才是不同的内容，答冲突。
func TestRegisterPackagingMaterialJudgesNumbersByValueAndEchoesTheTrimmedWriting(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)

	original := registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureOf(t, "30.00"), measureOf(t, "020"), measureOf(t, "15.50")
		spec.Weight = measureOf(t, "0.250")
	})
	registered := mustRegisterPackagingMaterial(t, handler, original)
	expectPackagingMaterialOutcome(t, "首次登记", registered, application.RegistrationRegistered)
	echoed, _ := registered.Revision()
	if got := []string{echoed.Content().Length().String(), echoed.Content().Width().String(), echoed.Content().Height().String(), echoed.Content().Weight().String()}; strings.Join(got, ",") != "30,20,15.5,0.25" {
		t.Fatalf("回显的数值 = %v，want 去掉多余零的 30、20、15.5、0.25", got)
	}

	rewritten := registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureOf(t, "30"), measureOf(t, "20.000000"), measureOf(t, "015.5")
		spec.Weight = measureOf(t, "00.25")
	})
	replay := mustRegisterPackagingMaterial(t, handler, rewritten)
	expectPackagingMaterialOutcome(t, "换一种数值写法原样重投", replay, application.RegistrationAlreadyRegistered)
	if registry.saves != 1 {
		t.Fatalf("换写法的重放保存了 %d 次，want 只有首笔那 1 次", registry.saves)
	}

	otherUnit := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "M")
	}))
	expectPackagingMaterialOutcome(t, "尺寸单位换一个码", otherUnit, application.RegistrationContentConflict)
	otherWeightUnit := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.WeightUnit = valueOf(t, domain.NewUnitCode, "G")
	}))
	expectPackagingMaterialOutcome(t, "重量单位换一个码", otherWeightUnit, application.RegistrationContentConflict)
}

// Covers: AT-WF-009（包材的部分）——一个仓都没登记的租户：登记包材答 NOT_CONFIGURED，不带拒绝理由、指出的格与最新修订号，不落册、不回显修订；
// 它不是拒绝，所以不是 NOT_ACCEPTED。之后登记了仓，同一修订原样重交照常受理（那一笔当初没有落册，重交不是重放）。
func TestRegisterPackagingMaterialBeforeAnyWarehouseIsNotConfiguredAndTheSameRevisionLandsAfterwards(t *testing.T) {
	registry := newFakePackagingMaterialRegistry()
	warehouses := newFakeTenantWarehouses()
	handler := newPackagingMaterialHandler(registry, warehouses)

	result := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil))
	expectPackagingMaterialOutcome(t, "一个仓都没登记", result, application.RegistrationNotConfigured)
	if _, has := result.Revision(); has || result.Refusal() != application.RegistrationRefusalNone ||
		result.Field() != domain.PackagingMaterialFieldInvalid || result.LatestRevision() != 0 {
		t.Fatalf("NOT_CONFIGURED 带了多余的细节：%+v", result)
	}
	if registry.saves != 0 || len(registry.rows) != 0 {
		t.Fatalf("未配置之后保存 %d 次、册里 %d 个包材，want 不落册", registry.saves, len(registry.rows))
	}

	warehouses.registered["SYN-TENANT-01"] = true
	expectPackagingMaterialOutcome(t, "登记仓之后原样重交", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	if len(registry.revisionsOf("SYN-BOX-01")) != 1 {
		t.Fatalf("重交之后落册 %d 笔，want 1", len(registry.revisionsOf("SYN-BOX-01")))
	}
}

// 判定次序里内容门排在最后：一个仓都没登记时，哪怕单位不在目录、数值为零，答的也是未配置；有仓但跳号时，答的是错位；这两种情形里内容门都没有
// 被走到。只有轮到内容门（有仓、修订号接得上）才答内容不合。
func TestRegisterPackagingMaterialChecksNotConfiguredThenSequenceThenContent(t *testing.T) {
	zeroLength := func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length = measureOf(t, "0")
		spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "FURLONG")
	}

	bare := newFakeTenantWarehouses()
	expectPackagingMaterialOutcome(t, "没有仓且内容不合",
		mustRegisterPackagingMaterial(t, newPackagingMaterialHandler(newFakePackagingMaterialRegistry(), bare), registerPackagingMaterialCommand(t, 1, zeroLength)),
		application.RegistrationNotConfigured)

	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)
	expectPackagingMaterialRefusal(t, "有仓、跳号且内容不合", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 2, zeroLength)),
		application.RegistrationRefusalRevisionOutOfSequence, domain.PackagingMaterialFieldInvalid)
	expectPackagingMaterialRefusal(t, "有仓、修订号接得上、内容不合", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, zeroLength)),
		application.RegistrationRefusalContentInvalid, domain.PackagingMaterialFieldDimensionsLength)
	if registry.saves != 0 {
		t.Fatalf("各种未受理之后保存 %d 次，want 0", registry.saves)
	}
}

// Covers: 重放先于 NOT_CONFIGURED、修订连续与内容核对——这一条用假登记册在应用层证判定次序：册上早就有的修订，哪怕此刻本租户一个仓都问不到，
// 原样重投仍是重放；对它换内容仍是冲突，哪怕换成的内容是数值为零、单位不在目录里（零值与坏单位不会把冲突误答成内容不合，也不会答未配置）；
// 仓册一次都没被问到。
func TestRegisterPackagingMaterialJudgesReplayAndConflictBeforeNotConfiguredSequenceAndContent(t *testing.T) {
	registry := newFakePackagingMaterialRegistry()
	warehouses := newFakeTenantWarehouses()
	handler := newPackagingMaterialHandler(registry, warehouses)
	seeded := registerPackagingMaterialCommand(t, 1, nil)
	revision, err := domain.NewPackagingMaterialRevision(seeded.Tenant, seeded.Code, 1, seeded.Content, seeded.Registrant, registrationInstant)
	if err != nil {
		t.Fatal(err)
	}
	registry.seed(revision)

	expectPackagingMaterialOutcome(t, "一个仓都问不到时重放在册的修订", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectPackagingMaterialOutcome(t, "换写法重放", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length = measureOf(t, "030.000")
	})), application.RegistrationAlreadyRegistered)
	expectPackagingMaterialOutcome(t, "对它换内容", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "另一个名字")
	})), application.RegistrationContentConflict)
	expectPackagingMaterialOutcome(t, "对它换成数值为零的内容", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Weight = measureOf(t, "0")
	})), application.RegistrationContentConflict)
	expectPackagingMaterialOutcome(t, "对它换成坏单位的内容", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.WeightUnit = valueOf(t, domain.NewUnitCode, "CM")
	})), application.RegistrationContentConflict)
	if registry.saves != 0 || warehouses.asked.Load() != 0 {
		t.Fatalf("重放与冲突保存了 %d 次、问了仓册 %d 次，want 都是 0", registry.saves, warehouses.asked.Load())
	}
}

// Covers: 停用与再启用各是一笔新修订，都答 REGISTERED；停用不拦此后的修订（停用期间照常改名与改尺寸重量），旧修订照留；包材没有登记后不改的格，
// 所以尺寸与重量随时可以以新修订改。
func TestRegisterPackagingMaterialDeactivationAndReactivationAreNewRevisions(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)
	expectPackagingMaterialOutcome(t, "修订 1", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	expectPackagingMaterialOutcome(t, "停用", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 2, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Deactivated = true
	})), application.RegistrationRegistered)
	expectPackagingMaterialOutcome(t, "停用期间改尺寸与重量", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 3, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Deactivated = true
		spec.Length, spec.Weight = measureOf(t, "31"), measureOf(t, "0.3")
	})), application.RegistrationRegistered)
	expectPackagingMaterialOutcome(t, "再启用", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 4, nil)), application.RegistrationRegistered)

	rows := registry.revisionsOf("SYN-BOX-01")
	if len(rows) != 4 || rows[0].Content().Deactivated() || !rows[1].Content().Deactivated() ||
		!rows[2].Content().Deactivated() || rows[3].Content().Deactivated() {
		t.Fatalf("停用与再启用之后的册不对：四笔的停用标记该依次是 否、是、是、否")
	}
	if rows[2].Content().Length().String() != "31" || rows[0].Content().Length().String() != "30" {
		t.Fatalf("改尺寸之后旧修订该照留：修订 1 的长 %s、修订 3 的长 %s", rows[0].Content().Length(), rows[2].Content().Length())
	}
}

// Covers: 重放只比内容，不比登记责任方与登记时刻——换一个登记责任方、晚一些再投仍答 ALREADY_REGISTERED，在册那一笔的登记责任方与登记时刻
// 不变，也不再保存。
func TestRegisterPackagingMaterialReplayByAnotherRegistrantKeepsTheOriginalRegistrant(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	clock := &fixedClock{at: registrationInstant}
	handler := application.NewRegisterPackagingMaterialHandler(registry, warehouses, clock)
	expectPackagingMaterialOutcome(t, "修订 1", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)

	clock.at = registrationInstant.Add(72 * time.Hour)
	another := registerPackagingMaterialCommand(t, 1, nil)
	another.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	replay := mustRegisterPackagingMaterial(t, handler, another)
	expectPackagingMaterialOutcome(t, "换登记责任方重放", replay, application.RegistrationAlreadyRegistered)

	rows := registry.revisionsOf("SYN-BOX-01")
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
func TestRegisterPackagingMaterialJudgesReplayAgainstAnyRevisionOnTheRegister(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)
	expectPackagingMaterialOutcome(t, "修订 1", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	expectPackagingMaterialOutcome(t, "修订 2", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 2, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱（二）")
	})), application.RegistrationRegistered)

	expectPackagingMaterialOutcome(t, "重投旧修订 1", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationAlreadyRegistered)
	expectPackagingMaterialOutcome(t, "对旧修订 1 换内容", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "另一个名字")
	})), application.RegistrationContentConflict)
	if registry.saves != 2 || len(registry.revisionsOf("SYN-BOX-01")) != 2 {
		t.Fatalf("保存 %d 次、落册 %d 笔，want 2 次、2 笔", registry.saves, len(registry.revisionsOf("SYN-BOX-01")))
	}
}

// 包材各仓共用、按租户登记：同一个编码在另一个租户名下是另一个包材，各自从修订 1 起；只差大小写的编码也是另一个包材。
func TestRegisterPackagingMaterialKeepsMaterialsOfDifferentTenantsAndSpellingsApart(t *testing.T) {
	registry := newFakePackagingMaterialRegistry()
	warehouses := newFakeTenantWarehouses("SYN-TENANT-01", "SYN-TENANT-02")
	handler := newPackagingMaterialHandler(registry, warehouses)

	expectPackagingMaterialOutcome(t, "本租户", mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil)), application.RegistrationRegistered)
	other := registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "另一租户的纸箱")
	})
	other.Tenant = valueOf(t, domain.NewTenantID, "SYN-TENANT-02")
	expectPackagingMaterialOutcome(t, "另一个租户同编码、不同内容", mustRegisterPackagingMaterial(t, handler, other), application.RegistrationRegistered)
	lower := registerPackagingMaterialCommand(t, 1, nil)
	lower.Code = valueOf(t, domain.NewPackagingMaterialCode, "syn-box-01")
	expectPackagingMaterialOutcome(t, "只差大小写的编码", mustRegisterPackagingMaterial(t, handler, lower), application.RegistrationRegistered)

	if len(registry.rows) != 3 {
		t.Fatalf("册里有 %d 个包材，want 3 个互相独立的包材", len(registry.rows))
	}
}

// 输入被拒是已形成的答案，不是技术失败：以上各格都经 mustRegisterPackagingMaterial 断言 error 为空。这里证反面——册读写不动才返回 error，并且不折成任何
// 一格业务结果（折成「一个仓都没登记」会让登记方被打发去登记一个本来就在册的仓）。
func TestRegisterPackagingMaterialReturnsAnErrorOnlyWhenTheRegistersCannotBeReadOrWritten(t *testing.T) {
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*fakePackagingMaterialRegistry, *fakeTenantWarehouses){
		"按修订号读不动":  func(registry *fakePackagingMaterialRegistry, _ *fakeTenantWarehouses) { registry.loadErr = boom },
		"读最新修订读不动": func(registry *fakePackagingMaterialRegistry, _ *fakeTenantWarehouses) { registry.loadLatestErr = boom },
		"读仓册读不动":   func(_ *fakePackagingMaterialRegistry, warehouses *fakeTenantWarehouses) { warehouses.err = boom },
		"保存不动":     func(registry *fakePackagingMaterialRegistry, _ *fakeTenantWarehouses) { registry.saveErr = boom },
		"保存答出集外的落点": func(registry *fakePackagingMaterialRegistry, _ *fakeTenantWarehouses) {
			registry.saveOutcome = ports.RegistrySaveOutcome(99)
		},
	} {
		registry, warehouses := packagingMaterialStores()
		arrange(registry, warehouses)
		result, err := newPackagingMaterialHandler(registry, warehouses).Register(t.Context(), registerPackagingMaterialCommand(t, 1, nil))
		if err == nil {
			t.Errorf("%s：err 为空，结果 = %s", name, result.Outcome())
		}
		if result.Outcome() != application.RegistrationOutcomeInvalid {
			t.Errorf("%s：结果 = %s，error 之外不该另带一格业务结果", name, result.Outcome())
		}
	}

	// 修订号立不住的命令也要问仓册：读仓册读不动同样走 error，不折成未配置。
	registry, warehouses := packagingMaterialStores()
	warehouses.err = boom
	if _, err := newPackagingMaterialHandler(registry, warehouses).Register(t.Context(), registerPackagingMaterialCommand(t, 0, nil)); err == nil {
		t.Errorf("修订号为 0 时读仓册读不动：应按技术失败上抛")
	}
}

// 登记口拒绝的是内容；身份与登记责任方没经构造器（零值）是接入侧装配错了，不是登记方的输入，按技术失败上抛。内容里缺格（零值内容）
// 仍是内容不合并指出缺哪一格。
func TestRegisterPackagingMaterialTreatsUnconstructedIdentityAsAnErrorAndUnconstructedContentAsInvalid(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)

	noTenant := registerPackagingMaterialCommand(t, 1, nil)
	noTenant.Tenant = domain.TenantID{}
	noCode := registerPackagingMaterialCommand(t, 1, nil)
	noCode.Code = domain.PackagingMaterialCode{}
	noRegistrant := registerPackagingMaterialCommand(t, 1, nil)
	noRegistrant.Registrant = domain.Registrant{}
	for name, command := range map[string]application.RegisterPackagingMaterialCommand{
		"缺租户": noTenant, "缺包材编码": noCode, "缺登记责任方": noRegistrant,
	} {
		if _, err := handler.Register(t.Context(), command); err == nil {
			t.Errorf("%s：应按技术失败上抛", name)
		}
	}

	noContent := registerPackagingMaterialCommand(t, 1, nil)
	noContent.Content = domain.PackagingMaterialContent{}
	expectPackagingMaterialRefusal(t, "零值内容", mustRegisterPackagingMaterial(t, handler, noContent),
		application.RegistrationRefusalContentInvalid, domain.PackagingMaterialFieldName)
	if registry.saves != 0 {
		t.Fatalf("保存了 %d 次，want 0", registry.saves)
	}
}

// 登记时刻取自注入的时钟，一次登记只读一次；回显的修订带它。
func TestRegisterPackagingMaterialStampsTheRegistrationInstantFromTheClock(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	result := mustRegisterPackagingMaterial(t, newPackagingMaterialHandler(registry, warehouses), registerPackagingMaterialCommand(t, 1, nil))
	landed, has := result.Revision()
	if !has || !landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("登记时刻 = %v（%v），want 时钟给的 %v", landed.RegisteredAt(), has, registrationInstant.Truncate(time.Microsecond))
	}
	if stored := registry.revisionsOf("SYN-BOX-01")[0]; !stored.RegisteredAt().Equal(landed.RegisteredAt()) {
		t.Fatalf("落册的登记时刻 %v 与回显的 %v 不一致", stored.RegisteredAt(), landed.RegisteredAt())
	}
}

// 包材的结果与仓、库区、库位类型的结果是同一个泛型结果的实例：各格只在回显的修订与「指出哪一格」的取值上不同，零值都是「没有结果」。
func TestPackagingMaterialResultIsAnInstanceOfTheSharedAlgebra(t *testing.T) {
	var _ application.RegistrationResult[domain.PackagingMaterialRevision, domain.PackagingMaterialField] = application.RegisterPackagingMaterialResult{}
	if (application.RegisterPackagingMaterialResult{}).Outcome() != application.RegistrationOutcomeInvalid {
		t.Errorf("零值结果该是没有结果")
	}
}

// 拒绝理由只在 NOT_ACCEPTED 时有值，回显的修订只在 REGISTERED 与 ALREADY_REGISTERED 时有：别的结果上读它们得零值。
func TestRegisterPackagingMaterialResultCarriesDetailOnlyWhereItBelongs(t *testing.T) {
	registry, warehouses := packagingMaterialStores()
	handler := newPackagingMaterialHandler(registry, warehouses)
	registered := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, nil))
	if registered.Refusal() != application.RegistrationRefusalNone || registered.Field() != domain.PackagingMaterialFieldInvalid || registered.LatestRevision() != 0 {
		t.Errorf("REGISTERED 带了拒绝细节：%s %q %d", registered.Refusal(), registered.Field(), registered.LatestRevision())
	}
	conflict := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 1, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Name = valueOf(t, domain.NewPackagingMaterialName, "冲突的名字")
	}))
	if _, has := conflict.Revision(); has || conflict.Refusal() != application.RegistrationRefusalNone {
		t.Errorf("CONTENT_CONFLICT 不该带回显的修订或拒绝理由")
	}
	refused := mustRegisterPackagingMaterial(t, handler, registerPackagingMaterialCommand(t, 5, nil))
	if _, has := refused.Revision(); has || !strings.Contains(refused.Refusal().String(), "SEQUENCE") {
		t.Errorf("NOT_ACCEPTED 不该带回显的修订")
	}
}
