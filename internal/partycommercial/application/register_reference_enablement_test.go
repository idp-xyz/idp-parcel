package application_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// fakeReferenceEnablementRegistry 是启用范围登记册的内存替身：同键同内容重放、异内容冲突，Load 交回最高修订
// （判据同真适配器，真库行为另在 adapters/postgres 证）。
type fakeReferenceEnablementRegistry struct {
	rows    []domain.ReferenceEnablementRegistration
	loadErr error
	saves   int
}

func (fake *fakeReferenceEnablementRegistry) SaveReferenceEnablement(
	_ context.Context,
	registration domain.ReferenceEnablementRegistration,
) (ports.ReferenceEnablementSaveOutcome, error) {
	fake.saves++
	for _, existing := range fake.rows {
		if existing.Tenant() == registration.Tenant() && existing.Kind() == registration.Kind() &&
			existing.Code() == registration.Code() && existing.Revision() == registration.Revision() {
			if existing.SameContent(registration) {
				return ports.ReferenceEnablementAlreadyRegistered, nil
			}
			return ports.ReferenceEnablementContentConflict, nil
		}
	}
	fake.rows = append(fake.rows, registration)
	return ports.ReferenceEnablementSaved, nil
}

func (fake *fakeReferenceEnablementRegistry) LoadLatestReferenceEnablement(
	_ context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
	code domain.ReferenceCode,
) (domain.ReferenceEnablementRegistration, bool, error) {
	if fake.loadErr != nil {
		return domain.ReferenceEnablementRegistration{}, false, fake.loadErr
	}
	var latest domain.ReferenceEnablementRegistration
	found := false
	for _, row := range fake.rows {
		if row.Tenant() == tenant && row.Kind() == kind && row.Code() == code &&
			(!found || row.Revision() > latest.Revision()) {
			latest, found = row, true
		}
	}
	return latest, found, nil
}

func (fake *fakeReferenceEnablementRegistry) ListReferenceEnablements(
	_ context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
) ([]domain.ReferenceEnablementRegistration, error) {
	if fake.loadErr != nil {
		return nil, fake.loadErr
	}
	var listed []domain.ReferenceEnablementRegistration
	for _, row := range fake.rows {
		if row.Tenant() == tenant && row.Kind() == kind {
			listed = append(listed, row)
		}
	}
	sort.Slice(listed, func(i, j int) bool {
		if listed[i].Code().String() != listed[j].Code().String() {
			return listed[i].Code().String() < listed[j].Code().String()
		}
		return listed[i].Revision() < listed[j].Revision()
	})
	return listed, nil
}

type fixedEnablementClock struct{ now time.Time }

func (clock fixedEnablementClock) Now() time.Time { return clock.now }

var enablementNow = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

func enablementCommand(
	t *testing.T,
	tenant string,
	kind domain.ReferenceKind,
	code string,
	revision int,
	status domain.ReferenceEnablementStatus,
	effectiveFrom time.Time,
) application.RegisterReferenceEnablementCommand {
	t.Helper()
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	referenceCode, err := domain.NewReferenceCode(code)
	if err != nil {
		t.Fatal(err)
	}
	registrant, err := domain.NewOperatorSubjectReference("https://idp.example#operator-1")
	if err != nil {
		t.Fatal(err)
	}
	return application.RegisterReferenceEnablementCommand{
		Tenant: tenantID, Kind: kind, Code: referenceCode, Revision: revision,
		Status: status, EffectiveFrom: effectiveFrom, Registrant: registrant,
	}
}

func registerEnablement(
	t *testing.T,
	handler *application.RegisterReferenceEnablementHandler,
	command application.RegisterReferenceEnablementCommand,
) application.ReferenceEnablementResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return result
}

// Covers: ADR-0345 决定三——首笔是修订 1，其后连续；同键同内容重放答已登记、不再落笔，同键异内容答内容冲突、册面不动。
func TestReferenceEnablementRegistersReplaysAndConflicts(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	enable := enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, enablementNow)

	if got := registerEnablement(t, handler, enable).Outcome(); got != application.ReferenceEnablementRegistered {
		t.Fatalf("首笔应答已登记，实际 %s", got)
	}
	if got := registerEnablement(t, handler, enable).Outcome(); got != application.ReferenceEnablementAlreadyRegistered {
		t.Fatalf("同键同内容应答重复，实际 %s", got)
	}
	conflicting := enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementDisabled, enablementNow)
	if got := registerEnablement(t, handler, conflicting).Outcome(); got != application.ReferenceEnablementContentConflict {
		t.Fatalf("同键异内容应答内容冲突，实际 %s", got)
	}
	if len(registry.rows) != 1 || registry.rows[0].Status() != domain.ReferenceEnablementEnabled {
		t.Fatalf("冲突不得覆盖册面：%+v", registry.rows)
	}

	// 另一位操作者重放同一个意思：在册的仍是先到的那一行。
	other := enable
	other.Registrant, _ = domain.NewOperatorSubjectReference("https://idp.example#operator-2")
	if got := registerEnablement(t, handler, other).Outcome(); got != application.ReferenceEnablementAlreadyRegistered {
		t.Fatalf("登记责任方不进重放比对，实际 %s", got)
	}
	if registry.rows[0].Registrant().String() != "https://idp.example#operator-1" {
		t.Fatalf("在册的应是先到的登记责任方，实际 %s", registry.rows[0].Registrant())
	}
}

// Covers: ADR-0345 决定三——停用与再启用都是新修订追加，历史原样在册。
func TestReferenceEnablementDisableThenEnableAgainAppends(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	day := func(n int) time.Time { return enablementNow.AddDate(0, 0, n) }

	for revision, step := range []struct {
		status domain.ReferenceEnablementStatus
		at     time.Time
	}{
		{domain.ReferenceEnablementEnabled, day(-10)},
		{domain.ReferenceEnablementDisabled, day(-5)},
		{domain.ReferenceEnablementEnabled, day(-1)},
	} {
		command := enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCurrency, "USD", revision+1, step.status, step.at)
		if got := registerEnablement(t, handler, command).Outcome(); got != application.ReferenceEnablementRegistered {
			t.Fatalf("第 %d 笔应答已登记，实际 %s", revision+1, got)
		}
	}
	if len(registry.rows) != 3 {
		t.Fatalf("三笔都追加，册上应有三行，实际 %d", len(registry.rows))
	}

	query := application.NewQueryReferenceEnablementsHandler(registry, fixedEnablementClock{enablementNow})
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	asOf, entries, err := query.ListReferenceEnablements(t.Context(), tenant, domain.ReferenceKindCurrency)
	if err != nil {
		t.Fatal(err)
	}
	if !asOf.Equal(enablementNow) || len(entries) != 1 {
		t.Fatalf("asOf=%v entries=%d", asOf, len(entries))
	}
	entry := entries[0]
	if !entry.HasStanding || entry.Standing.Status() != domain.ReferenceEnablementEnabled || entry.Standing.Revision() != 3 {
		t.Fatalf("停用后再启用，此刻应是启用（修订 3）：%+v", entry.Standing)
	}
	if len(entry.History) != 3 || entry.History[0].Revision() != 1 || entry.History[2].Revision() != 3 {
		t.Fatalf("历史应带全部三笔且按修订升序：%+v", entry.History)
	}
}

// Covers: ADR-0345 决定三——码取自内置目录，目录外答未受理并指名 CODE_NOT_IN_CATALOGUE，一个字节没写；
// 修订错位与生效时点早于前一修订各有各的理由。
func TestReferenceEnablementRefusalsAreNamedAndWriteNothing(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	registerEnablement(t, handler, enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, enablementNow))
	saves := registry.saves

	cases := []struct {
		name    string
		command application.RegisterReferenceEnablementCommand
		refusal application.ReferenceEnablementRefusal
	}{
		{"目录外的国家码", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "ZZ", 1, domain.ReferenceEnablementEnabled, enablementNow),
			application.ReferenceEnablementCodeNotInCatalogue},
		{"目录外的币种码", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCurrency, "XXX", 1, domain.ReferenceEnablementEnabled, enablementNow),
			application.ReferenceEnablementCodeNotInCatalogue},
		{"册上没有时首笔不是 1", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "SG", 2, domain.ReferenceEnablementEnabled, enablementNow),
			application.ReferenceEnablementRevisionOutOfSequence},
		{"跳号", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 3, domain.ReferenceEnablementDisabled, enablementNow),
			application.ReferenceEnablementRevisionOutOfSequence},
		{"生效时点早于前一修订", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementDisabled, enablementNow.Add(-time.Hour)),
			application.ReferenceEnablementEffectiveBeforePrior},
		{"没有生效时点", enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementDisabled, time.Time{}),
			application.ReferenceEnablementInputNotWellFormed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := registerEnablement(t, handler, tc.command)
			if result.Outcome() != application.ReferenceEnablementNotAccepted || result.Refusal() != tc.refusal {
				t.Fatalf("应答未受理 / %s，实际 %s / %s", tc.refusal, result.Outcome(), result.Refusal())
			}
			if result.Cause() == nil {
				t.Fatal("未受理要带原因散文")
			}
		})
	}
	if registry.saves != saves {
		t.Fatalf("被拒的输入一个字节没写：落笔次数 %d -> %d", saves, registry.saves)
	}
}

// Covers: ADR-0345 决定一——没登记就是未配置：此刻没有生效行的码（压根没登，或全是未来生效）读面答 HasStanding=false，
// 不折成启用或停用；册上没有行的码不出现在读面。
func TestReferenceEnablementWithoutAnEffectiveRowIsNotConfigured(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	registerEnablement(t, handler, enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "SG", 1, domain.ReferenceEnablementEnabled, enablementNow.AddDate(0, 0, 30)))

	query := application.NewQueryReferenceEnablementsHandler(registry, fixedEnablementClock{enablementNow})
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	_, entries, err := query.ListReferenceEnablements(t.Context(), tenant, domain.ReferenceKindCountry)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Code.String() != "SG" || entries[0].HasStanding {
		t.Fatalf("只有未来生效行的码应在读面里、但没有此刻的状态：%+v", entries)
	}
	_, none, _ := query.ListReferenceEnablements(t.Context(), tenant, domain.ReferenceKindCurrency)
	if len(none) != 0 {
		t.Fatalf("没登记过的种类读面为空册：%+v", none)
	}
}

// Covers: ADR-0345 决定四——册面按租户隔离：他租户登的行本租户读不到，同码各租户各登各的首笔。
func TestReferenceEnablementIsolatesTenants(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	registerEnablement(t, handler, enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, enablementNow))
	if got := registerEnablement(t, handler, enablementCommand(t, "SYN-TENANT-02", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementDisabled, enablementNow)).Outcome(); got != application.ReferenceEnablementRegistered {
		t.Fatalf("另一租户同码也是自己的首笔，实际 %s", got)
	}

	query := application.NewQueryReferenceEnablementsHandler(registry, fixedEnablementClock{enablementNow})
	other, _ := domain.NewTenantID("SYN-TENANT-03")
	if _, entries, _ := query.ListReferenceEnablements(t.Context(), other, domain.ReferenceKindCountry); len(entries) != 0 {
		t.Fatalf("第三个租户读不到别人的行：%+v", entries)
	}
}

func TestReferenceEnablementSurfacesRegistryFailureAsError(t *testing.T) {
	registry := &fakeReferenceEnablementRegistry{loadErr: errors.New("db down")}
	handler := application.NewRegisterReferenceEnablementHandler(registry, fixedEnablementClock{enablementNow})
	result, err := handler.Register(t.Context(), enablementCommand(t, "SYN-TENANT-01", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, enablementNow))
	if err == nil || result.Outcome() != application.ReferenceEnablementOutcomeInvalid {
		t.Fatalf("读册失败是技术失败，不得折成业务答案：%v / %s", err, result.Outcome())
	}
	query := application.NewQueryReferenceEnablementsHandler(registry, fixedEnablementClock{enablementNow})
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	if _, _, err := query.ListReferenceEnablements(t.Context(), tenant, domain.ReferenceKindCountry); err == nil {
		t.Fatal("读面读不回是错误，不是空册")
	}
}
