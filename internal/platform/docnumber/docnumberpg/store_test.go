package docnumberpg_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真库取 ADR-0143 原列、ADR-0355 决定四并入的那几条证据：取号组件参与调用方的开立事务（事务外即拒、回滚
// 即释放）、并发取号不撞、按租户隔离、按模板推出的周期重置、换模板后同周期流水接着数；外加规则册的登记、重放与读，
// 以及预览不占号。同一开立意图重放答原号由 04–09 各自在单据上验——号存在单据上，重放读回，不再取号。

const tenant = "SYN-TENANT-01"

var declarations = []docnumber.TypeDeclaration{
	{Code: "DAILY_DOC", DefaultTemplate: "D{yyyy}{mm}{dd}-{seq:4}", DateTimeZone: docnumber.DateTimeZoneFromTenant},
	{Code: "MONTHLY_DOC", DefaultTemplate: "M{yyyy}{mm}-{seq:4}", DateTimeZone: docnumber.DateTimeZoneFromTenant},
	{Code: "YEARLY_DOC", DefaultTemplate: "Y{yyyy}-{seq:4}", DateTimeZone: docnumber.DateTimeZoneFromTenant},
	{Code: "CONTINUOUS_DOC", DefaultTemplate: "C{seq:6}", DateTimeZone: docnumber.DateTimeZoneFromTenant},
}

type movableClock struct {
	mu sync.Mutex
	at time.Time
}

func (clock *movableClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.at
}

func (clock *movableClock) set(at time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.at = at
}

type fixture struct {
	db        *bentopg.DB
	clock     *movableClock
	registrar *docnumber.Registrar
	reader    *docnumber.Reader
	assigner  *docnumber.Assigner
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := docnumber.NewCatalogue(declarations)
	if err != nil {
		t.Fatal(err)
	}
	store, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := &movableClock{at: time.Date(2026, time.November, 2, 2, 0, 0, 0, time.UTC)}
	secrets := docnumber.SecretTable{}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := docnumber.NewReader(docnumber.ReaderDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: store, Sequences: sequences, Transactions: store, Secrets: secrets, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{db: db, clock: clock, registrar: registrar, reader: reader, assigner: assigner}
}

func (f fixture) registerZone(t *testing.T, tenantID, zone string) {
	t.Helper()
	err := f.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := f.registrar.RegisterTimeZone(ctx, docnumber.RegisterTimeZoneCommand{
			Tenant: tenantID, TimeZone: zone, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
	if err != nil {
		t.Fatalf("register numbering time zone: %v", err)
	}
}

func (f fixture) registerRule(t *testing.T, code docnumber.TypeCode, template string, effectiveFrom time.Time) docnumber.RuleRegistration {
	t.Helper()
	var result docnumber.RuleRegistration
	err := f.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		registered, err := f.registrar.RegisterRule(ctx, docnumber.RegisterRuleCommand{
			Tenant: tenant, Type: code, Template: template, EffectiveFrom: effectiveFrom, RegisteredBy: "SYN-TEST",
		})
		result = registered
		return err
	})
	if err != nil {
		t.Fatalf("register rule: %v", err)
	}
	return result
}

func neverUsed(context.Context, string) (bool, error) { return false, nil }

// issue 在一笔事务里取一个号并提交。
func (f fixture) issue(t *testing.T, tenantID string, code docnumber.TypeCode) docnumber.Assignment {
	t.Helper()
	var assignment docnumber.Assignment
	err := f.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		assigned, err := f.assigner.Assign(ctx, docnumber.IssueRequest{Tenant: tenantID, Type: code, IsUsed: neverUsed})
		assignment = assigned
		return err
	})
	if err != nil {
		t.Fatalf("issue %s: %v", code, err)
	}
	return assignment
}

// Covers: ADR-0355 决定三——取号参与调用方的开立事务：事务外即拒；事务回滚，号码随之释放，下一笔取到同一个号；
// 提交之后流水往前走。
func TestAssignmentParticipatesInTheIssuingTransaction(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	if _, err := f.assigner.Assign(t.Context(), docnumber.IssueRequest{Tenant: tenant, Type: "DAILY_DOC", IsUsed: neverUsed}); !errors.Is(err, docnumber.ErrTransactionRequired) {
		t.Fatalf("outside a transaction: %v", err)
	}

	rolledBack := errors.New("the issuing transaction rolls back")
	var released docnumber.Assignment
	err := f.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		assigned, err := f.assigner.Assign(ctx, docnumber.IssueRequest{Tenant: tenant, Type: "DAILY_DOC", IsUsed: neverUsed})
		if err != nil {
			return err
		}
		released = assigned
		return rolledBack
	})
	if !errors.Is(err, rolledBack) || released.Number != "D20261102-0001" {
		t.Fatalf("rolled back issue: %+v %v", released, err)
	}
	if again := f.issue(t, tenant, "DAILY_DOC"); again.Number != "D20261102-0001" {
		t.Fatalf("rollback did not release the number: %+v", again)
	}
	if next := f.issue(t, tenant, "DAILY_DOC"); next.Number != "D20261102-0002" {
		t.Fatalf("committed numbers do not advance: %+v", next)
	}
}

// Covers: ADR-0355 决定三、六——并发开立不撞：多笔事务同时取同一（租户，类型，周期）的号，提交的号两两不同、恰好是
// 一到 N。
func TestConcurrentAssignmentsNeverCollide(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	const issuers = 16
	numbers := make([]string, issuers)
	failures := make([]error, issuers)
	var wait sync.WaitGroup
	for index := range issuers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			failures[index] = f.db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
				assigned, err := f.assigner.Assign(ctx, docnumber.IssueRequest{Tenant: tenant, Type: "MONTHLY_DOC", IsUsed: neverUsed})
				numbers[index] = assigned.Number
				return err
			})
		}()
	}
	wait.Wait()
	for _, err := range failures {
		if err != nil {
			t.Fatalf("concurrent issue failed: %v", err)
		}
	}
	sort.Strings(numbers)
	for index, number := range numbers {
		if want := fmt.Sprintf("M202611-%04d", index+1); number != want {
			t.Fatalf("concurrent numbers %v, want 0001 through %04d without gaps or repeats", numbers, issuers)
		}
	}
}

// Covers: ADR-0355 决定六——序列按租户隔离：两个租户同一类型同一周期各从 1 数起，一个租户的号推不出另一个的量；
// 单据编号时区也按租户各登各的，没登的那个答未配置。
func TestSequencesAreIsolatedPerTenant(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	first := f.issue(t, tenant, "YEARLY_DOC")
	second := f.issue(t, tenant, "YEARLY_DOC")
	other := f.issue(t, "SYN-TENANT-02", "YEARLY_DOC")
	if other.Outcome != docnumber.NumberingTimeZoneNotConfigured {
		t.Fatalf("another tenant borrowed the numbering time zone: %+v", other)
	}
	f.registerZone(t, "SYN-TENANT-02", "Asia/Singapore")
	other = f.issue(t, "SYN-TENANT-02", "YEARLY_DOC")
	if first.Number != "Y2026-0001" || second.Number != "Y2026-0002" || other.Number != "Y2026-0001" {
		t.Fatalf("tenants share a sequence: %s %s %s", first.Number, second.Number, other.Number)
	}
}

// Covers: ADR-0355 决定五、七——周期由模板最细的日期令牌推出、按单据编号时区算：含日按日、含月按月、只有年按年、
// 无日期不重置。时刻跨过的是上海的午夜：UTC 16:00 正是上海的第二天零点。
func TestPeriodsResetByTheTemplateInTheNumberingTimeZone(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	steps := []struct {
		at   time.Time
		want map[docnumber.TypeCode]string
	}{
		{time.Date(2026, time.December, 31, 15, 59, 0, 0, time.UTC), map[docnumber.TypeCode]string{
			"DAILY_DOC": "D20261231-0001", "MONTHLY_DOC": "M202612-0001", "YEARLY_DOC": "Y2026-0001", "CONTINUOUS_DOC": "C000001",
		}},
		{time.Date(2026, time.December, 31, 15, 59, 30, 0, time.UTC), map[docnumber.TypeCode]string{
			"DAILY_DOC": "D20261231-0002", "MONTHLY_DOC": "M202612-0002", "YEARLY_DOC": "Y2026-0002", "CONTINUOUS_DOC": "C000002",
		}},
		{time.Date(2026, time.December, 31, 16, 0, 0, 0, time.UTC), map[docnumber.TypeCode]string{
			"DAILY_DOC": "D20270101-0001", "MONTHLY_DOC": "M202701-0001", "YEARLY_DOC": "Y2027-0001", "CONTINUOUS_DOC": "C000003",
		}},
		{time.Date(2027, time.January, 1, 16, 0, 0, 0, time.UTC), map[docnumber.TypeCode]string{
			"DAILY_DOC": "D20270102-0001", "MONTHLY_DOC": "M202701-0002", "YEARLY_DOC": "Y2027-0002", "CONTINUOUS_DOC": "C000004",
		}},
	}
	for _, step := range steps {
		f.clock.set(step.at)
		for code, want := range step.want {
			if got := f.issue(t, tenant, code); got.Number != want {
				t.Errorf("%s at %s: %s, want %s", code, step.at, got.Number, want)
			}
		}
	}
}

// Covers: ADR-0355 决定五——租户登了新模板就从生效时刻起用它；重置粒度不变时序列键不变，同一周期流水接着数。
func TestTemplateChangeKeepsCountingWithinThePeriod(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	before := f.issue(t, tenant, "MONTHLY_DOC")
	registered := f.registerRule(t, "MONTHLY_DOC", "SYN-{yy}{mm}-{seq:5}", f.clock.Now().Add(time.Minute))
	if registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("register: %+v", registered)
	}
	stillDefault := f.issue(t, tenant, "MONTHLY_DOC")
	f.clock.set(f.clock.Now().Add(2 * time.Minute))
	after := f.issue(t, tenant, "MONTHLY_DOC")
	if before.Number != "M202611-0001" || stillDefault.Number != "M202611-0002" || after.Number != "SYN-2611-00003" || after.Source != docnumber.SourceTenantRule {
		t.Fatalf("numbers across the change: %s %s %+v", before.Number, stillDefault.Number, after)
	}
}

// Covers: ADR-0355 决定六——规则册只增不改：首登、同内容重放答已存在、同一生效时刻异内容答已被占；读口列出此刻生效的
// 规则与历次版本，别的租户读不到；写册在事务外即拒。
func TestRuleBookRegistersReplaysAndReads(t *testing.T) {
	f := newFixture(t)
	effectiveFrom := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	first := f.registerRule(t, "DAILY_DOC", "SYN{yy}{mm}{dd}{seq:5}", effectiveFrom)
	replay := f.registerRule(t, "DAILY_DOC", "SYN{yy}{mm}{dd}{seq:5}", effectiveFrom)
	taken := f.registerRule(t, "DAILY_DOC", "SYN{yy}{mm}{dd}{seq:6}", effectiveFrom)
	later := f.registerRule(t, "DAILY_DOC", "NEXT{yy}{mm}{dd}{seq:5}", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if first.Outcome != docnumber.VersionRegistered || replay.Outcome != docnumber.VersionExisting ||
		taken.Outcome != docnumber.EffectiveFromTaken || taken.Version.Template != "SYN{yy}{mm}{dd}{seq:5}" || later.Outcome != docnumber.VersionRegistered {
		t.Fatalf("outcomes: %s %s %s %s", first.Outcome, replay.Outcome, taken.Outcome, later.Outcome)
	}
	if !first.Version.EffectiveFrom.Equal(effectiveFrom) || first.Version.RegisteredBy != "SYN-TEST" || first.Version.RegisteredAt.IsZero() {
		t.Fatalf("stored version: %+v", first.Version)
	}

	view, err := f.reader.Read(t.Context(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if view.NumberingTimeZone != nil {
		t.Fatalf("numbering time zone appeared without registration: %+v", view.NumberingTimeZone)
	}
	daily := view.Types[0]
	if daily.Source != docnumber.SourceTenantRule || daily.Template != "SYN{yy}{mm}{dd}{seq:5}" || len(daily.Versions) != 2 || daily.Sample != "" {
		t.Fatalf("daily view: %+v", daily)
	}
	other, err := f.reader.Read(t.Context(), "SYN-TENANT-02")
	if err != nil {
		t.Fatal(err)
	}
	if other.Types[0].Source != docnumber.SourceProductDefault || len(other.Types[0].Versions) != 0 {
		t.Fatalf("another tenant sees this tenant's rules: %+v", other.Types[0])
	}

	_, err = f.registrar.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: tenant, Type: "DAILY_DOC", Template: "OUT{seq:5}", EffectiveFrom: effectiveFrom.Add(time.Hour), RegisteredBy: "SYN-TEST",
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("a write outside a transaction: %v", err)
	}
}

// Covers: ADR-0355 决定六——预览不读也不占序列：预览之后第一次取号仍是第一个号。
func TestPreviewDoesNotConsumeTheSequence(t *testing.T) {
	f := newFixture(t)
	f.registerZone(t, tenant, "Asia/Shanghai")

	for range 3 {
		preview, err := f.reader.Preview(t.Context(), docnumber.PreviewRequest{Tenant: tenant, Type: "DAILY_DOC", Template: "D{yyyy}{mm}{dd}-{seq:4}"})
		if err != nil || preview.Sample != "D20261102-0001" {
			t.Fatalf("preview: %+v %v", preview, err)
		}
	}
	if first := f.issue(t, tenant, "DAILY_DOC"); first.Number != "D20261102-0001" {
		t.Fatalf("preview consumed the sequence: %+v", first)
	}
}

// Covers: 框架合同对写参与者的要求——规则册的两个写口与「是否在开立事务里」那一问，在没有事务的上下文里都由
// RequireExecutor 拒绝，而不是改用连接池写下一行或答「在」。
func TestStoreWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := store.AppendRuleVersion(t.Context(), docnumber.RuleVersion{
		Tenant: tenant, Type: "DAILY_DOC", Template: "D{seq}", EffectiveFrom: at, RegisteredBy: "SYN-TEST", RegisteredAt: at,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记规则版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, _, err := store.AppendTimeZoneVersion(t.Context(), docnumber.TimeZoneVersion{
		Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: at, RegisteredBy: "SYN-TEST", RegisteredAt: at,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记单据编号时区应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := store.RequireTransaction(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务时 RequireTransaction 应返回 ErrTransactionRequired，实得：%v", err)
	}
}
