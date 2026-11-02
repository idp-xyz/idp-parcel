package docnumber

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-platform-go/numbering"
)

// utcEvening 是 UTC 的 11 月 1 日晚上，在 Asia/Shanghai 已是 11 月 2 日：日期按哪个时区算，一眼看得出。
var utcEvening = time.Date(2026, time.November, 1, 17, 30, 0, 0, time.UTC)

type assignerFixture struct {
	book      *memoryBook
	sequences *memorySequences
	secrets   SecretTable
	scope     transactionScope
	clock     fixedClock
}

func newAssignerFixture() *assignerFixture {
	return &assignerFixture{
		book:      &memoryBook{},
		sequences: &memorySequences{},
		secrets:   SecretTable{},
		scope:     transactionScope{inside: true},
		clock:     fixedClock{at: utcEvening},
	}
}

func (fixture *assignerFixture) assigner(t *testing.T) *Assigner {
	t.Helper()
	assigner, err := NewAssigner(AssignerDeps{
		Catalogue: testCatalogue(), Book: fixture.book, Sequences: fixture.sequences,
		Transactions: fixture.scope, Secrets: fixture.secrets, Clock: fixture.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return assigner
}

func (fixture *assignerFixture) registerZone(zone string) {
	fixture.book.zones = append(fixture.book.zones, TimeZoneVersion{
		Tenant: testTenant, TimeZone: zone, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		RegisteredBy: "SYN", RegisteredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
}

func neverUsed(context.Context, string) (bool, error) { return false, nil }

// Covers: ADR-0355 决定七——开立方没有业务时区、租户也没登单据编号时区时答「单据编号时区未配置」，不拿 UTC 顶，
// 序列一个值也不动；登了之后日期按它算。
func TestAssignWithoutNumberingTimeZoneAnswersNotConfigured(t *testing.T) {
	fixture := newAssignerFixture()
	assignment, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Outcome != NumberingTimeZoneNotConfigured || assignment.Number != "" || assignment.TimeZone != "" {
		t.Fatalf("want not configured without a number, got %+v", assignment)
	}
	if len(fixture.sequences.keys) != 0 {
		t.Fatalf("sequence touched without a time zone: %v", fixture.sequences.keys)
	}

	fixture.registerZone("Asia/Shanghai")
	assignment, err = fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Outcome != NumberAssigned || assignment.Number != "P20261102000001" || assignment.TimeZone != "Asia/Shanghai" {
		t.Fatalf("want the Shanghai date, got %+v", assignment)
	}
	if assignment.Source != SourceProductDefault || !assignment.IssuedAt.Equal(utcEvening) {
		t.Fatalf("source or issuing moment: %+v", assignment)
	}
	if key := fixture.sequences.keys[0]; key != (numbering.SequenceKey{Scope: testTenant, DocType: "SHIPMENT", Period: "20261102"}) {
		t.Fatalf("sequence key %+v", key)
	}
}

// Covers: ADR-0355 决定七——开立方交来业务时区就用它，盖过单据编号时区；声明说开立方没有业务时区的类型交来一个是
// 编程错误；交来的不是 IANA 名也拒。
func TestAssignPrefersTheIssuerBusinessTimeZone(t *testing.T) {
	fixture := newAssignerFixture()
	fixture.registerZone("Asia/Shanghai")
	assigner := fixture.assigner(t)

	assignment, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "DELIVERY_PRE_ADVICE", IssuerTimeZone: "America/Los_Angeles", IsUsed: neverUsed})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Number != "PA-20261101-00001" || assignment.TimeZone != "America/Los_Angeles" {
		t.Fatalf("want the issuer's date, got %+v", assignment)
	}
	fallback, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "DELIVERY_PRE_ADVICE", IsUsed: neverUsed})
	if err != nil || fallback.Number != "PA-20261102-00001" {
		t.Fatalf("issuer without a zone should fall back to the numbering time zone: %+v %v", fallback, err)
	}

	if _, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IssuerTimeZone: "Asia/Shanghai", IsUsed: neverUsed}); !errors.Is(err, ErrIssuerTimeZoneNotDeclared) {
		t.Fatalf("undeclared issuer zone accepted: %v", err)
	}
	for _, zone := range []string{"Local", "Mars/Olympus"} {
		if _, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "DELIVERY_PRE_ADVICE", IssuerTimeZone: zone, IsUsed: neverUsed}); !errors.Is(err, ErrInvalidIssueRequest) {
			t.Fatalf("issuer zone %q accepted: %v", zone, err)
		}
	}
}

// Covers: ADR-0355 决定三——事务外调用即失败，单据编号时区未配置那一格也不例外；请求不成立（缺租户、缺「是否已用」
// 查询、类型没声明）是编程错误。
func TestAssignRefusesOutsideTheIssuingTransactionAndMalformedRequests(t *testing.T) {
	fixture := newAssignerFixture()
	fixture.scope.inside = false
	if _, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed}); !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("outside a transaction: %v", err)
	}

	fixture = newAssignerFixture()
	assigner := fixture.assigner(t)
	for name, request := range map[string]IssueRequest{
		"no tenant":     {Type: "SHIPMENT", IsUsed: neverUsed},
		"no used query": {Tenant: testTenant, Type: "SHIPMENT"},
		"undeclared":    {Tenant: testTenant, Type: "CLAIM", IsUsed: neverUsed},
	} {
		if _, err := assigner.Assign(t.Context(), request); !errors.Is(err, ErrInvalidIssueRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Covers: ADR-0355 决定八——渲染出的号已用就跳过、取同一周期的下一个流水；连续跳过到上限仍撞答「编号规则撞号」，
// 一共取了上限加一个值。
func TestAssignSkipsUsedNumbersUpToTheLimit(t *testing.T) {
	fixture := newAssignerFixture()
	fixture.registerZone("Asia/Shanghai")
	assigner := fixture.assigner(t)

	used := map[string]bool{"P20261102000001": true, "P20261102000002": true}
	assignment, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: func(_ context.Context, number string) (bool, error) {
		return used[number], nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Outcome != NumberAssigned || assignment.Number != "P20261102000003" || assignment.Skipped != 2 {
		t.Fatalf("want the third number after two skips, got %+v", assignment)
	}

	before := len(fixture.sequences.keys)
	collision, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: func(context.Context, string) (bool, error) {
		return true, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if collision.Outcome != NumberingRuleCollision || collision.Number != "" || collision.Skipped != MaxSkippedNumbers {
		t.Fatalf("want a collision answer, got %+v", collision)
	}
	if drawn := len(fixture.sequences.keys) - before; drawn != MaxSkippedNumbers+1 {
		t.Fatalf("drew %d values, want %d", drawn, MaxSkippedNumbers+1)
	}

	failing := errors.New("lookup failed")
	if _, err := assigner.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: func(context.Context, string) (bool, error) {
		return false, failing
	}}); !errors.Is(err, failing) {
		t.Fatalf("lookup failure swallowed: %v", err)
	}
}

// Covers: ADR-0355 决定五——租户登了新版就从生效时刻起用它，之前仍用产品默认；序列键不含模板，重置粒度不变时换格式
// 流水接着数。
func TestAssignUsesTheTenantRuleFromItsEffectiveMoment(t *testing.T) {
	fixture := newAssignerFixture()
	fixture.registerZone("Asia/Shanghai")
	fixture.book.rules = append(fixture.book.rules, RuleVersion{
		Tenant: testTenant, Type: "SHIPMENT", Template: "SYN{yy}{mm}{dd}-{seq:4}",
		EffectiveFrom: utcEvening.Add(time.Hour), RegisteredBy: "SYN", RegisteredAt: utcEvening,
	})

	before, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed})
	if err != nil || before.Number != "P20261102000001" || before.Source != SourceProductDefault {
		t.Fatalf("before the effective moment: %+v %v", before, err)
	}
	fixture.clock.at = utcEvening.Add(2 * time.Hour)
	after, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed})
	if err != nil {
		t.Fatal(err)
	}
	if after.Number != "SYN261102-0002" || after.Source != SourceTenantRule || after.Template != "SYN{yy}{mm}{dd}-{seq:4}" {
		t.Fatalf("after the effective moment: %+v", after)
	}
}

// Covers: ADR-0355 越权风险点 1——加扰键照平台入口的拼法：同一模板、租户、类型、时刻与密钥，本编排出的号与平台
// `Assigner.Assign` 出的号逐字相同。
func TestScrambledNumbersMatchThePlatformAssigner(t *testing.T) {
	const template = "X{yyyy}{mm}{dd}-{seqx:6}"
	fixture := newAssignerFixture()
	fixture.registerZone("UTC")
	fixture.secrets = SecretTable{testTenant: testSecret}
	fixture.book.rules = append(fixture.book.rules, RuleVersion{
		Tenant: testTenant, Type: "SHIPMENT", Template: template,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening,
	})
	ours := fixture.assigner(t)

	registry, err := numbering.NewRegistry(map[string]numbering.DocTypeSpec{"SHIPMENT": {Template: template}})
	if err != nil {
		t.Fatal(err)
	}
	platform, err := numbering.NewAssigner(registry, &memorySequences{},
		numbering.WithNow(func() time.Time { return utcEvening }), numbering.WithScrambleSecret(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		mine, err := ours.Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed})
		if err != nil {
			t.Fatal(err)
		}
		theirs, err := platform.Assign(t.Context(), testTenant, "SHIPMENT")
		if err != nil {
			t.Fatal(err)
		}
		if mine.Number != theirs {
			t.Fatalf("scrambled number %s differs from the platform entry's %s", mine.Number, theirs)
		}
	}

	// 登记之后从实例配置里撤了密钥：部署故障，开立随之失败，而不是拿空密钥渲染。
	fixture.secrets = SecretTable{}
	if _, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed}); !errors.Is(err, ErrScrambleSecretNotConfigured) {
		t.Fatalf("missing secret at assignment: %v", err)
	}
}

// Covers: 模板令牌分月、按年与不重置的周期键——周期由生效模板最细的日期令牌推出，按单据编号时区算。
func TestAssignDerivesThePeriodFromTheEffectiveTemplate(t *testing.T) {
	cases := []struct {
		template string
		period   string
	}{
		{"M{yyyy}{mm}-{seq:4}", "202611"},
		{"Y{yyyy}-{seq:4}", "2026"},
		{"C{seq:8}", ""},
	}
	for _, testCase := range cases {
		fixture := newAssignerFixture()
		fixture.registerZone("Asia/Shanghai")
		fixture.book.rules = append(fixture.book.rules, RuleVersion{
			Tenant: testTenant, Type: "SHIPMENT", Template: testCase.template,
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening,
		})
		if _, err := fixture.assigner(t).Assign(t.Context(), IssueRequest{Tenant: testTenant, Type: "SHIPMENT", IsUsed: neverUsed}); err != nil {
			t.Fatal(err)
		}
		if got := fixture.sequences.keys[0].Period; got != testCase.period {
			t.Errorf("%s: period %q, want %q", testCase.template, got, testCase.period)
		}
	}
}
