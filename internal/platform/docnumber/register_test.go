package docnumber

import (
	"errors"
	"testing"
	"time"
)

func newRegistrar(t *testing.T, book *memoryBook, secrets SecretTable) *Registrar {
	t.Helper()
	registrar, err := NewRegistrar(RegistrarDeps{Catalogue: testCatalogue(), Book: book, Secrets: secrets, Clock: fixedClock{at: utcEvening}})
	if err != nil {
		t.Fatal(err)
	}
	return registrar
}

var effectiveFrom = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))

// Covers: ADR-0355 决定五、六——规则版本只增不改：首登落一版，同内容重放答已存在，同一生效时刻异内容答已被占，
// 册上那一版不变；生效时刻与登记时刻收成库里的精度与 UTC。
func TestRegisterRuleAppendsReplaysAndRefusesToOverwrite(t *testing.T) {
	book := &memoryBook{}
	registrar := newRegistrar(t, book, SecretTable{})
	command := RegisterRuleCommand{
		Tenant: testTenant, Type: "SHIPMENT", Template: "SYN{yy}{mm}{dd}{seq:5}",
		EffectiveFrom: effectiveFrom.Add(123 * time.Nanosecond), RegisteredBy: "OPERATOR/syn#01",
	}

	first, err := registrar.RegisterRule(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != VersionRegistered || !first.Version.EffectiveFrom.Equal(effectiveFrom) || first.Version.EffectiveFrom.Location() != time.UTC {
		t.Fatalf("first registration: %+v", first)
	}
	if !first.Version.RegisteredAt.Equal(utcEvening) || first.Version.RegisteredBy != "OPERATOR/syn#01" {
		t.Fatalf("registrant and moment: %+v", first.Version)
	}

	command.RegisteredBy = "OPERATOR/syn#02"
	replay, err := registrar.RegisterRule(t.Context(), command)
	if err != nil || replay.Outcome != VersionExisting || replay.Version.RegisteredBy != "OPERATOR/syn#01" {
		t.Fatalf("replay: %+v %v", replay, err)
	}

	command.Template = "SYN{yy}{mm}{dd}{seq:6}"
	taken, err := registrar.RegisterRule(t.Context(), command)
	if err != nil || taken.Outcome != EffectiveFromTaken || taken.Version.Template != "SYN{yy}{mm}{dd}{seq:5}" {
		t.Fatalf("taken: %+v %v", taken, err)
	}
	if len(book.rules) != 1 {
		t.Fatalf("book holds %d versions", len(book.rules))
	}
}

// Covers: ADR-0355 决定五——登记校验：类型须已声明、模板须过模板检查，用了加扰而该租户没配密钥即拒；缺租户、登记人或
// 生效时刻不成立。拒了册上什么也不落。
func TestRegisterRuleRejections(t *testing.T) {
	book := &memoryBook{}
	registrar := newRegistrar(t, book, SecretTable{"SYN-TENANT-02": testSecret})
	base := RegisterRuleCommand{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{seq:6}", EffectiveFrom: effectiveFrom, RegisteredBy: "SYN"}

	cases := []struct {
		name   string
		mutate func(*RegisterRuleCommand)
		reason error
	}{
		{"undeclared type", func(c *RegisterRuleCommand) { c.Type = "CLAIM" }, ErrTypeNotDeclared},
		{"unparseable", func(c *RegisterRuleCommand) { c.Template = "P{nope}{seq}" }, ErrTemplateUnparseable},
		{"no sequence", func(c *RegisterRuleCommand) { c.Template = "P{yyyy}{rand:6}" }, ErrTemplateWithoutSequence},
		{"unsupplied context", func(c *RegisterRuleCommand) { c.Template = "{region}{seq:6}" }, ErrTemplateNeedsUnsuppliedValue},
		{"repeats next year", func(c *RegisterRuleCommand) { c.Template = "M{mm}-{seq:2}" }, ErrTemplateRepeatsAcrossPeriods},
		{"scrambled without secret", func(c *RegisterRuleCommand) { c.Template = "P{yyyy}{seqx:6}" }, ErrScrambleSecretNotConfigured},
		{"no tenant", func(c *RegisterRuleCommand) { c.Tenant = " " }, ErrRegistrationIncomplete},
		{"no registrant", func(c *RegisterRuleCommand) { c.RegisteredBy = "" }, ErrRegistrationIncomplete},
		{"no effective moment", func(c *RegisterRuleCommand) { c.EffectiveFrom = time.Time{} }, ErrRegistrationIncomplete},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := base
			testCase.mutate(&command)
			_, err := registrar.RegisterRule(t.Context(), command)
			if !errors.Is(err, ErrRegistrationRejected) || !errors.Is(err, testCase.reason) {
				t.Fatalf("want rejection %v, got %v", testCase.reason, err)
			}
			if RejectionReason(err) != testCase.reason {
				t.Fatalf("reason %v, want %v", RejectionReason(err), testCase.reason)
			}
		})
	}
	if len(book.rules) != 0 {
		t.Fatalf("a rejected registration landed: %+v", book.rules)
	}

	scrambled := base
	scrambled.Tenant, scrambled.Template = "SYN-TENANT-02", "P{yyyy}{seqx:6}"
	if result, err := registrar.RegisterRule(t.Context(), scrambled); err != nil || result.Outcome != VersionRegistered {
		t.Fatalf("scrambled with a secret refused: %+v %v", result, err)
	}
}

// Covers: ADR-0355 决定七——单据编号时区是租户取值、只收 IANA 名：空串、Local 与不认识的名字都拒；同一生效时刻的
// 重放与改名语义同规则版本。
func TestRegisterTimeZone(t *testing.T) {
	book := &memoryBook{}
	registrar := newRegistrar(t, book, SecretTable{})
	command := RegisterTimeZoneCommand{Tenant: testTenant, TimeZone: "Asia/Shanghai", EffectiveFrom: effectiveFrom, RegisteredBy: "SYN"}

	for _, zone := range []string{"", "Local", " Asia/Shanghai", "Mars/Olympus"} {
		bad := command
		bad.TimeZone = zone
		if _, err := registrar.RegisterTimeZone(t.Context(), bad); !errors.Is(err, ErrTimeZoneNotRecognized) {
			t.Errorf("%q accepted: %v", zone, err)
		}
	}
	first, err := registrar.RegisterTimeZone(t.Context(), command)
	if err != nil || first.Outcome != VersionRegistered {
		t.Fatalf("first: %+v %v", first, err)
	}
	replay, err := registrar.RegisterTimeZone(t.Context(), command)
	if err != nil || replay.Outcome != VersionExisting {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	command.TimeZone = "Asia/Singapore"
	taken, err := registrar.RegisterTimeZone(t.Context(), command)
	if err != nil || taken.Outcome != EffectiveFromTaken || taken.Version.TimeZone != "Asia/Shanghai" {
		t.Fatalf("taken: %+v %v", taken, err)
	}
}
