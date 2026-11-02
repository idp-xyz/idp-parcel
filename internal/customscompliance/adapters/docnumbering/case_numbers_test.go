package docnumbering_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccdocnumbering "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/docnumbering"
	ccidentity "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/identity"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真库上把建案编排、案件库、本包的取号适配与编号组件的取号编排接成一条，证关务案件单据号的那几条证据
// （ADR-0355 决定三、七、八；票 document-numbering/07）：同一建案意图重放答原号、并发建案不撞号、租户隔离、按租户
// 单据编号时区的日界重置、事务回滚释放号码、编号规则撞号。时钟可拨，日界不靠运气。
//
// 回滚按 EstablishCaseResult.RollsBack 办，与 parcel-api 建案口的事务壳同一条约定；那只壳本身由 parcel-api 的装配用例
// 在生产装配上再证一遍。断言一律在事务闭包外。

const (
	tenantA = "SYN-TENANT-CC07-A"
	tenantB = "SYN-TENANT-CC07-B"
)

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

type alwaysRequired struct{}

func (alwaysRequired) JudgeCaseRequirement(context.Context, domain.TenantID, domain.RegulatoryJurisdictionReference,
	domain.ManifestDirection, domain.CustomsProcedureReference) (ports.CaseRequirementJudgment, bool, error) {
	return ports.CaseRequirementJudgment{Required: true, Basis: "SYN-BASIS-CC07"}, true, nil
}

type noHandoff struct{}

func (noHandoff) HandOffCase(context.Context, ports.CustomsCaseHandoffIntent) error { return nil }

type harness struct {
	pool      *pgxpool.Pool
	db        *bentopg.DB
	clock     *movableClock
	registrar *docnumber.Registrar
	cases     *ccpostgres.CustomsCases
	handler   *application.EstablishCaseHandler
}

func newHarness(t *testing.T) harness {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
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
	// 2026-11-02 10:00（上海）。
	clock := &movableClock{at: time.Date(2026, time.November, 2, 2, 0, 0, 0, time.UTC)}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: store, Secrets: docnumber.SecretTable{}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: store, Sequences: sequences, Transactions: store, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	cases, err := ccpostgres.NewCustomsCases(db)
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := ccdocnumbering.NewCaseNumbers(assigner, cases)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := ccidentity.NewCaseIdentities()
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewEstablishCaseHandler(application.EstablishCaseDeps{
		Requirement: alwaysRequired{},
		Store:       cases,
		Identity:    identities,
		Numbers:     numbers,
		Downstream:  noHandoff{},
		Clock:       clock,
	})
	return harness{pool: pool, db: db, clock: clock, registrar: registrar, cases: cases, handler: handler}
}

func (h harness) registerZone(t *testing.T, tenant string) {
	t.Helper()
	err := h.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := h.registrar.RegisterTimeZone(ctx, docnumber.RegisterTimeZoneCommand{
			Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
	if err != nil {
		t.Fatalf("登单据编号时区：%v", err)
	}
}

func command(t *testing.T, tenant, obligation string, parcels ...string) application.EstablishCaseCommand {
	t.Helper()
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	jurisdiction, _ := domain.NewRegulatoryJurisdictionReference("SYN-JUR-CN")
	procedure, _ := domain.NewCustomsProcedureReference("SYN-PROC-CN-EXPORT")
	scope, err := domain.NewObligationScopeReference(obligation)
	if err != nil {
		t.Fatal(err)
	}
	associations := make([]domain.CaseParcelAssociation, 0, len(parcels))
	for _, parcel := range parcels {
		associations = append(associations, domain.CaseParcelAssociation{Parcel: parcel, Customer: "SYN-CUST-01", SourceRef: "PS/" + parcel})
	}
	return application.EstablishCaseCommand{
		TenantID: tenantID, Jurisdiction: jurisdiction, Direction: domain.ExportManifest, Procedure: procedure,
		Obligation: scope, Parcels: associations,
	}
}

var errForcedRollback = errors.New("test: the establishing transaction rolls back")

// establish 在一笔事务里建案：答复要求回滚就回滚（与建案口的事务壳同一条约定），forceRollback 时无论如何回滚。
func (h harness) establish(t *testing.T, cmd application.EstablishCaseCommand, forceRollback bool) (application.EstablishCaseResult, error) {
	var result application.EstablishCaseResult
	err := h.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		handled, err := h.handler.Handle(ctx, cmd)
		if err != nil {
			return err
		}
		result = handled
		if forceRollback || handled.RollsBack() {
			return errForcedRollback
		}
		return nil
	})
	if errors.Is(err, errForcedRollback) {
		err = nil
	}
	return result, err
}

func numberOf(t *testing.T, result application.EstablishCaseResult) string {
	t.Helper()
	customsCase, present := result.Case()
	if !present {
		t.Fatalf("答复 %v 没带案件", result.Outcome())
	}
	number, numbered := customsCase.DocumentNumber()
	if !numbered {
		t.Fatalf("案件没有单据号（%v）", customsCase.DocumentNumberAbsence())
	}
	return number
}

// Covers: ADR-0355 决定三「同一开立意图重放答原号——号存在单据上，重放读回，不再取号」；按产品默认模板
// `CC-{yyyy}{mm}{dd}-{seq:4}` 在租户单据编号时区下出号。点名 `AT-CC-459`。
func TestAReplayedEstablishmentAnswersTheOriginalNumber(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)

	first, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || first.Outcome() != application.CaseEstablished {
		t.Fatalf("建案：%v %v", first.Outcome(), err)
	}
	if got := numberOf(t, first); got != "CC-20261102-0001" {
		t.Fatalf("号 = %s", got)
	}
	replay, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || replay.Outcome() != application.CaseExisting || numberOf(t, replay) != "CC-20261102-0001" {
		t.Fatalf("重放：%v %v", replay.Outcome(), err)
	}
	next, err := h.establish(t, command(t, tenantA, "SYN-OBL-2", "P-1"), false)
	if err != nil || numberOf(t, next) != "CC-20261102-0002" {
		t.Fatalf("重放占了号：下一个案件 %v %v", next.Outcome(), err)
	}
}

// Covers: ADR-0355 决定三、八——并发建案不撞号、不留空号：不同监管范围各取各的号且号段连续；同一范围并发重放只落一个
// 案件，输掉的几笔答原案件原号并回滚释放取过的流水。点名 `AT-CC-462`（并发半边）。
func TestConcurrentEstablishmentsNeverCollideOrLeaveGaps(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)

	const distinct, racers = 8, 4
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		numbers []string
		raced   []string
		errs    []error
	)
	for i := range distinct {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := h.establish(t, command(t, tenantA, fmt.Sprintf("SYN-OBL-D%02d", i), "P-1"), false)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || result.Outcome() != application.CaseEstablished {
				errs = append(errs, fmt.Errorf("distinct %d: %v %v", i, result.Outcome(), err))
				return
			}
			customsCase, _ := result.Case()
			number, _ := customsCase.DocumentNumber()
			numbers = append(numbers, number)
		}()
	}
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := h.establish(t, command(t, tenantA, "SYN-OBL-SAME", "P-1"), false)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || (result.Outcome() != application.CaseEstablished && result.Outcome() != application.CaseExisting) {
				errs = append(errs, fmt.Errorf("racer %d: %v %v", i, result.Outcome(), err))
				return
			}
			customsCase, _ := result.Case()
			number, _ := customsCase.DocumentNumber()
			raced = append(raced, number)
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("并发建案出错：%v", errs)
	}
	for _, number := range raced[1:] {
		if number != raced[0] {
			t.Fatalf("同一范围并发重放答了不同的号：%v", raced)
		}
	}
	all := append(append([]string(nil), numbers...), raced[0])
	sort.Strings(all)
	for i, number := range all {
		if want := fmt.Sprintf("CC-20261102-%04d", i+1); number != want {
			t.Fatalf("号段不连续或撞号：%v", all)
		}
	}
	after, err := h.establish(t, command(t, tenantA, "SYN-OBL-AFTER", "P-1"), false)
	if err != nil || numberOf(t, after) != fmt.Sprintf("CC-20261102-%04d", distinct+2) {
		t.Fatalf("并发输家没释放取过的流水：%v %v", after.Outcome(), err)
	}
}

// Covers: ADR-0355 决定五——流水按（租户，类型，周期）计，不跨租户；租户各登各的单据编号时区。点名 `AT-CC-462`（租户隔离半边）。
func TestCaseNumbersAreCountedPerTenant(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)
	h.registerZone(t, tenantB)

	for i, tenant := range []string{tenantA, tenantA, tenantB} {
		if _, err := h.establish(t, command(t, tenant, fmt.Sprintf("SYN-OBL-%d", i), "P-1"), false); err != nil {
			t.Fatal(err)
		}
	}
	b, err := h.establish(t, command(t, tenantB, "SYN-OBL-B-LAST", "P-1"), false)
	if err != nil || numberOf(t, b) != "CC-20261102-0002" {
		t.Fatalf("租户 B 的流水受了租户 A 的影响：%v %v", b.Outcome(), err)
	}
}

// Covers: ADR-0355 决定七——日期与重置周期按服务端此刻在租户单据编号时区下算：上海的午夜一过，日期换、流水从 1 起。
// 点名 `AT-CC-462`（周期半边）。
func TestCaseNumbersResetAtTheNumberingTimeZoneMidnight(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)

	h.clock.set(time.Date(2026, time.November, 2, 15, 59, 0, 0, time.UTC)) // 上海 23:59
	before, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || numberOf(t, before) != "CC-20261102-0001" {
		t.Fatalf("午夜前：%v %v", before.Outcome(), err)
	}
	h.clock.set(time.Date(2026, time.November, 2, 16, 0, 30, 0, time.UTC)) // 上海次日 00:00:30，UTC 仍是 11-02
	after, err := h.establish(t, command(t, tenantA, "SYN-OBL-2", "P-1"), false)
	if err != nil || numberOf(t, after) != "CC-20261103-0001" {
		t.Fatalf("午夜后：%v %v", after.Outcome(), err)
	}
}

// Covers: ADR-0355 决定三——建案事务回滚，号码随之释放：下一个案件拿到同一个号，库里没有那个案件。点名 `AT-CC-462`（回滚半边）。
func TestARolledBackEstablishmentReleasesItsNumber(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)

	rolledBack, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), true)
	if err != nil || numberOf(t, rolledBack) != "CC-20261102-0001" {
		t.Fatalf("回滚的那一笔：%v %v", rolledBack.Outcome(), err)
	}
	again, err := h.establish(t, command(t, tenantA, "SYN-OBL-2", "P-1"), false)
	if err != nil || numberOf(t, again) != "CC-20261102-0001" {
		t.Fatalf("回滚没释放号码：%v %v", again.Outcome(), err)
	}
}

// Covers: ADR-0355 决定八——渲染出的号已被用过就跳过；跳到上限仍撞，这一次不落单、答编号规则撞号并回滚，取过的流水
// 一个不留；腾出一个号后，下一次建案从同一个流水起取到它。点名 `AT-CC-461`。
func TestANumberingCollisionLandsNothingAndReleasesTheSequence(t *testing.T) {
	h := newHarness(t)
	h.registerZone(t, tenantA)

	// 用另一套规则下已经发出去的号占满 CC-20261102-0001 起的整整一段（换格式前的旧号恰好同形）。
	for i := 1; i <= docnumber.MaxSkippedNumbers; i++ {
		if _, err := h.pool.Exec(t.Context(),
			`INSERT INTO customs_compliance.customs_case
				(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at, document_number)
			 VALUES ($1, 'SYN-JUR-OLD', 'EXPORT', 'SYN-PROC-OLD', $2, $2,
			         '[{"parcel":"P-OLD","customer":"SYN-CUST-01","sourceRef":"PS/P-OLD"}]', '[]', now(), $3)`,
			tenantA, fmt.Sprintf("SYN-OLD-%02d", i), fmt.Sprintf("CC-20261102-%04d", i)); err != nil {
			t.Fatalf("占号：%v", err)
		}
	}

	collision, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || collision.Outcome() != application.CaseNumberingRuleCollision || !collision.RollsBack() {
		t.Fatalf("撞号：%v %v", collision.Outcome(), err)
	}
	if _, found, err := h.cases.FindByKey(t.Context(), ports.CustomsCaseKey{
		TenantID: command(t, tenantA, "SYN-OBL-1").TenantID, Jurisdiction: command(t, tenantA, "SYN-OBL-1").Jurisdiction,
		Direction: domain.ExportManifest, Procedure: command(t, tenantA, "SYN-OBL-1").Procedure,
		Obligation: command(t, tenantA, "SYN-OBL-1").Obligation,
	}); err != nil || found {
		t.Fatalf("撞号还落了案件：%v %v", err, found)
	}

	if _, err := h.pool.Exec(t.Context(),
		`DELETE FROM customs_compliance.customs_case WHERE tenant_id = $1 AND document_number = 'CC-20261102-0007'`, tenantA); err != nil {
		t.Fatal(err)
	}
	freed, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || numberOf(t, freed) != "CC-20261102-0007" {
		t.Fatalf("撞号回滚没释放流水（应从 1 起跳到腾出的 0007）：%v %v", freed.Outcome(), err)
	}
}

// Covers: ADR-0355 决定七——租户没登单据编号时区时，案件照常建立、不带号并记下情形，序列一个值也不动；事后补登时区
// 不给已建立的案件补号，之后建立的才有号，且从 1 起。点名 `AT-CC-460`。
func TestACaseWithoutANumberingTimeZoneStaysUnnumbered(t *testing.T) {
	h := newHarness(t)

	unnumbered, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || unnumbered.Outcome() != application.CaseEstablished {
		t.Fatalf("没登时区：%v %v", unnumbered.Outcome(), err)
	}
	customsCase, _ := unnumbered.Case()
	if _, present := customsCase.DocumentNumber(); present || customsCase.DocumentNumberAbsence() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("没登时区的案件 = %v", customsCase.DocumentNumberAbsence())
	}

	h.registerZone(t, tenantA)
	replay, err := h.establish(t, command(t, tenantA, "SYN-OBL-1", "P-1"), false)
	if err != nil || replay.Outcome() != application.CaseExisting {
		t.Fatalf("重放：%v %v", replay.Outcome(), err)
	}
	if replayed, _ := replay.Case(); replayed.DocumentNumberAbsence() != domain.NumberingTimeZoneNotConfigured {
		t.Fatal("补登时区之后旧案件被补了号")
	}
	numbered, err := h.establish(t, command(t, tenantA, "SYN-OBL-2", "P-1"), false)
	if err != nil || numberOf(t, numbered) != "CC-20261102-0001" {
		t.Fatalf("补登之后：%v %v", numbered.Outcome(), err)
	}
}
