package docnumber

import (
	"errors"
	"testing"
)

// Covers: ADR-0355 决定六——各上下文的声明经 NewCatalogue 汇总；同一类型只许声明一次，类型代码只收一种写法，时区来路
// 必须声明，默认模板过与租户登记同一道模板检查，且不许用加扰（加扰密钥是租户取值、无默认）。
func TestCatalogueRefusesInconsistentDeclarations(t *testing.T) {
	valid := TypeDeclaration{Code: "SHIPMENT", DefaultTemplate: "P{yyyy}{mm}{dd}{seq:6}", DateTimeZone: DateTimeZoneFromTenant}
	cases := []struct {
		name   string
		groups [][]TypeDeclaration
	}{
		{"declared twice across contexts", [][]TypeDeclaration{{valid}, {valid}}},
		{"lower case code", [][]TypeDeclaration{{{Code: "shipment", DefaultTemplate: valid.DefaultTemplate, DateTimeZone: DateTimeZoneFromTenant}}}},
		{"blank code", [][]TypeDeclaration{{{Code: "", DefaultTemplate: valid.DefaultTemplate, DateTimeZone: DateTimeZoneFromTenant}}}},
		{"no time zone source", [][]TypeDeclaration{{{Code: "SHIPMENT", DefaultTemplate: valid.DefaultTemplate}}}},
		{"default without sequence", [][]TypeDeclaration{{{Code: "SHIPMENT", DefaultTemplate: "P{yyyy}{mm}{dd}{rand:6}", DateTimeZone: DateTimeZoneFromTenant}}}},
		{"default with scrambled sequence", [][]TypeDeclaration{{{Code: "SHIPMENT", DefaultTemplate: "P{yyyy}{seqx:6}", DateTimeZone: DateTimeZoneFromTenant}}}},
		{"nothing declared", nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewCatalogue(testCase.groups...); !errors.Is(err, ErrInvalidDeclaration) {
				t.Fatalf("want ErrInvalidDeclaration, got %v", err)
			}
		})
	}

	catalogue, err := NewCatalogue(testDeclarations[:1], testDeclarations[1:])
	if err != nil {
		t.Fatalf("valid declarations refused: %v", err)
	}
	if got := catalogue.Declarations(); len(got) != len(testDeclarations) || got[0].Code != "SHIPMENT" || got[2].Code != "CUSTOMER_STATEMENT" {
		t.Fatalf("declarations not kept in aggregation order: %+v", got)
	}
	if _, found := catalogue.Declaration("CLAIM"); found {
		t.Fatal("an undeclared type was found")
	}
}

// Covers: ADR-0355 决定五——模板须能被平台 Parse、含流水令牌；编排给不出取值的令牌（上下文值、校验位）、渲染出空白
// 或过长的号都拒；加扰与其余令牌照收，并认得出用了加扰。
func TestTemplateCheck(t *testing.T) {
	cases := []struct {
		template  string
		want      error
		scrambled bool
	}{
		{"P{yyyy}{mm}{dd}{seq:6}", nil, false},
		{"{type}-{yy}{q}-{seq36:4}-{rand:2}", nil, false},
		{"FY{fyyy}/{seq}", nil, false},
		{"X{yyyy}{seqx:6}", nil, true},
		{"X{seqx36:1}", nil, true},
		{"X{seqx:1}", nil, true},
		{"", ErrTemplateUnparseable, false},
		{"P{yyyy}{bogus}{seq}", ErrTemplateUnparseable, false},
		{"P{yyyy}{mm}{dd}", ErrTemplateWithoutSequence, false},
		{"P{rand:8}", ErrTemplateWithoutSequence, false},
		{"{site}-{seq:5}", ErrTemplateNeedsUnsuppliedValue, false},
		{"{le}{seq:5}", ErrTemplateNeedsUnsuppliedValue, false},
		{"P{seq:6}{check}", ErrTemplateNeedsUnsuppliedValue, false},
		{"P {seq:6}", ErrTemplateUnrenderable, false},
		{"P\t{seq:6}", ErrTemplateUnrenderable, false},
		{"P{seq:50}", ErrTemplateUnrenderable, false},
		{"P{seqx:30}", ErrTemplateUnrenderable, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.template, func(t *testing.T) {
			checked, err := checkTemplate("SHIPMENT", testCase.template)
			if testCase.want == nil {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if checked.scrambled != testCase.scrambled {
					t.Fatalf("scrambled = %v, want %v", checked.scrambled, testCase.scrambled)
				}
				return
			}
			if !errors.Is(err, testCase.want) {
				t.Fatalf("want %v, got %v", testCase.want, err)
			}
		})
	}
}

// Covers: 加扰密钥从实例配置注入、无默认（ADR-0355 决定五）——不设即一个租户都没配；设了的表要合规：短于下限、
// 空租户键、重复键、非对象、尾随内容都拒，读不动也拒。
func TestScrambleSecretsComeFromInstanceConfiguration(t *testing.T) {
	table, configured, err := LoadScrambleSecrets(func(string) string { return "" }, nil)
	if err != nil || configured || table.Tenants() != 0 {
		t.Fatalf("unset should mean no secrets: %v %v %d", err, configured, table.Tenants())
	}
	if _, found := table.ScrambleSecret(testTenant); found {
		t.Fatal("a secret appeared without configuration")
	}

	getenv := func(name string) string {
		if name == ScrambleSecretsFileEnv {
			return "/secrets/document-numbers.json"
		}
		return ""
	}
	table, configured, err = LoadScrambleSecrets(getenv, func(path string) ([]byte, error) {
		if path != "/secrets/document-numbers.json" {
			t.Fatalf("read %s", path)
		}
		return []byte(`{"` + testTenant + `":"` + testSecret + `"}`), nil
	})
	if err != nil || !configured {
		t.Fatalf("configured table refused: %v", err)
	}
	if secret, found := table.ScrambleSecret(testTenant); !found || secret != testSecret {
		t.Fatalf("secret not served: %q %v", secret, found)
	}
	if _, found := table.ScrambleSecret("SYN-TENANT-02"); found {
		t.Fatal("another tenant got a secret")
	}

	if _, _, err := LoadScrambleSecrets(getenv, func(string) ([]byte, error) { return nil, errors.New("gone") }); !errors.Is(err, ErrInvalidScrambleSecrets) {
		t.Fatalf("unreadable file: %v", err)
	}
	for _, raw := range []string{
		`{"SYN-TENANT-01":"short"}`,
		`{"":"` + testSecret + `"}`,
		`{" SYN-TENANT-01":"` + testSecret + `"}`,
		`{"SYN-TENANT-01":"` + testSecret + `","SYN-TENANT-01":"` + testSecret + `x"}`,
		`{"SYN-TENANT-01":7}`,
		`["SYN-TENANT-01"]`,
		`{"SYN-TENANT-01":"` + testSecret + `"} {}`,
		``,
	} {
		if _, err := ParseScrambleSecrets([]byte(raw)); !errors.Is(err, ErrInvalidScrambleSecrets) {
			t.Errorf("%q accepted: %v", raw, err)
		}
	}
	if table, err := ParseScrambleSecrets([]byte(`{}`)); err != nil || table.Tenants() != 0 {
		t.Fatalf("empty object: %v", err)
	}
}
