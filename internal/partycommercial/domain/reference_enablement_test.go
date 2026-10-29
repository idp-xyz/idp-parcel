package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var referenceEnablementBase = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func enablementRegistration(
	t *testing.T,
	kind domain.ReferenceKind,
	code string,
	revision int,
	status domain.ReferenceEnablementStatus,
	effectiveFrom time.Time,
) (domain.ReferenceEnablementRegistration, error) {
	t.Helper()
	return domain.NewReferenceEnablementRegistration(
		commercialValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		kind,
		commercialValue(t, domain.NewReferenceCode, code),
		revision,
		domain.ReferenceEnablementSpec{Status: status, EffectiveFrom: effectiveFrom},
		commercialValue(t, domain.NewOperatorSubjectReference, "https://idp.example#operator-1"),
		referenceEnablementBase,
	)
}

// Covers: `AT-PC-074`，ADR-0345 决定三「码必须在内置目录内」——目录里的码照常登记，目录外的码答 ErrReferenceCodeNotInCatalogue，
// 国家码与币种码各查各的目录（CN 不是币种，CNY 不是国家 / 地区）。
func TestEnablementCodeMustBeInTheBuiltInCatalogueOfItsKind(t *testing.T) {
	cases := []struct {
		name string
		kind domain.ReferenceKind
		code string
		want bool
	}{
		{"国家 / 地区码在目录内", domain.ReferenceKindCountry, "CN", true},
		{"币种码在目录内", domain.ReferenceKindCurrency, "USD", true},
		{"形状合法而目录外的国家码", domain.ReferenceKindCountry, "ZZ", false},
		{"形状合法而目录外的币种码", domain.ReferenceKindCurrency, "XXX", false},
		{"国家码不能当币种登", domain.ReferenceKindCurrency, "CN", false},
		{"币种码不能当国家登", domain.ReferenceKindCountry, "CNY", false},
		{"小写不是目录里的写法", domain.ReferenceKindCountry, "cn", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := enablementRegistration(t, tc.kind, tc.code, 1, domain.ReferenceEnablementEnabled, referenceEnablementBase)
			if tc.want && err != nil {
				t.Fatalf("目录内的码被拒：%v", err)
			}
			if !tc.want && !errors.Is(err, domain.ErrReferenceCodeNotInCatalogue) {
				t.Fatalf("目录外的码应答 ErrReferenceCodeNotInCatalogue，实际 %v", err)
			}
		})
	}
}

// Covers: 同上——读回的重建不核目录成员。目录随发版变，ISO 废止一个码之后，此前合法登过的行仍读得回。
func TestRehydrationDoesNotRecheckCatalogueMembership(t *testing.T) {
	row, err := domain.RehydrateReferenceEnablementRegistration(
		commercialValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		domain.ReferenceKindCurrency,
		commercialValue(t, domain.NewReferenceCode, "XXX"),
		1,
		domain.ReferenceEnablementSpec{Status: domain.ReferenceEnablementEnabled, EffectiveFrom: referenceEnablementBase},
		commercialValue(t, domain.NewOperatorSubjectReference, "https://idp.example#operator-1"),
		referenceEnablementBase,
	)
	if err != nil {
		t.Fatalf("读回不该核目录成员：%v", err)
	}
	if row.Code().String() != "XXX" {
		t.Fatalf("code = %q", row.Code())
	}
}

func TestEnablementRegistrationRefusesWhatCannotStand(t *testing.T) {
	cases := map[string]func() (domain.ReferenceEnablementRegistration, error){
		"修订号从 1 起": func() (domain.ReferenceEnablementRegistration, error) {
			return enablementRegistration(t, domain.ReferenceKindCountry, "CN", 0, domain.ReferenceEnablementEnabled, referenceEnablementBase)
		},
		"状态在封闭集内": func() (domain.ReferenceEnablementRegistration, error) {
			return enablementRegistration(t, domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementStatusInvalid, referenceEnablementBase)
		},
		"生效时点必给": func() (domain.ReferenceEnablementRegistration, error) {
			return enablementRegistration(t, domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, time.Time{})
		},
		"种类在封闭集内": func() (domain.ReferenceEnablementRegistration, error) {
			return enablementRegistration(t, domain.ReferenceKindInvalid, "CN", 1, domain.ReferenceEnablementEnabled, referenceEnablementBase)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := build(); !errors.Is(err, domain.ErrInvalidReferenceEnablement) {
				t.Fatalf("应答 ErrInvalidReferenceEnablement，实际 %v", err)
			}
		})
	}
}

func TestEnablementSourceIsOptionalAndBounded(t *testing.T) {
	blank, err := domain.NewReferenceEnablementSource("   ")
	if err != nil || blank.String() != "" {
		t.Fatalf("不写来源单据是合法的，实际 %q %v", blank, err)
	}
	trimmed, err := domain.NewReferenceEnablementSource("  MEMO-2026-07  ")
	if err != nil || trimmed.String() != "MEMO-2026-07" {
		t.Fatalf("来源单据去首尾空白：%q %v", trimmed, err)
	}
	if _, err := domain.NewReferenceEnablementSource(strings.Repeat("单", 201)); !errors.Is(err, domain.ErrInvalidReferenceEnablement) {
		t.Fatalf("超过 200 字应拒，实际 %v", err)
	}
	if _, err := domain.NewReferenceEnablementSource(strings.Repeat("单", 200)); err != nil {
		t.Fatalf("200 字整是边界上限，不该拒：%v", err)
	}
}

// Covers: `AT-PC-073`，ADR-0345 决定三「修订连续、生效时点不早于前一修订」。
func TestEnablementRevisionsFollowOneAnotherInOrder(t *testing.T) {
	first, err := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, referenceEnablementBase)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.FollowsFrom(domain.ReferenceEnablementRegistration{}, false); err != nil {
		t.Fatalf("首笔是 1，应接得上：%v", err)
	}

	second, _ := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementDisabled, referenceEnablementBase.Add(time.Hour))
	if err := second.FollowsFrom(first, true); err != nil {
		t.Fatalf("2 接 1，应接得上：%v", err)
	}

	skipped, _ := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 3, domain.ReferenceEnablementDisabled, referenceEnablementBase)
	if err := skipped.FollowsFrom(first, true); !errors.Is(err, domain.ErrReferenceEnablementOutOfSequence) {
		t.Fatalf("跳号应答 ErrReferenceEnablementOutOfSequence，实际 %v", err)
	}

	notFirst, _ := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementEnabled, referenceEnablementBase)
	if err := notFirst.FollowsFrom(domain.ReferenceEnablementRegistration{}, false); !errors.Is(err, domain.ErrReferenceEnablementOutOfSequence) {
		t.Fatalf("册上没有时首笔必须是 1，实际 %v", err)
	}

	earlier, _ := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementDisabled, referenceEnablementBase.Add(-time.Second))
	if err := earlier.FollowsFrom(first, true); !errors.Is(err, domain.ErrReferenceEnablementEffectiveBeforePrior) {
		t.Fatalf("生效时点早于前一修订应答 ErrReferenceEnablementEffectiveBeforePrior，实际 %v", err)
	}

	same, _ := enablementRegistration(t, domain.ReferenceKindCountry, "CN", 2, domain.ReferenceEnablementDisabled, referenceEnablementBase)
	if err := same.FollowsFrom(first, true); err != nil {
		t.Fatalf("生效时点相等是后到者取胜，不该拒：%v", err)
	}
}

// Covers: `AT-PC-075`，ADR-0345 决定三「当前状态由生效时点不晚于此刻的最新一行导出」——没有生效的行（含全是未来生效）答不出来，
// 就是未配置；停用后再启用，后到者取胜；同刻生效取修订号大的；未来生效的行在到点之前不说话。
func TestEnablementStandingIsTheLatestRowEffectiveByThatMoment(t *testing.T) {
	row := func(revision int, status domain.ReferenceEnablementStatus, at time.Time) domain.ReferenceEnablementRegistration {
		registration, err := enablementRegistration(t, domain.ReferenceKindCurrency, "USD", revision, status, at)
		if err != nil {
			t.Fatal(err)
		}
		return registration
	}
	day := func(offset int) time.Time { return referenceEnablementBase.AddDate(0, 0, offset) }

	if _, found := domain.EnablementStandingAt(nil, day(0)); found {
		t.Fatal("册上没有行就是未配置，不是启用也不是停用")
	}

	history := []domain.ReferenceEnablementRegistration{
		row(1, domain.ReferenceEnablementEnabled, day(0)),
		row(2, domain.ReferenceEnablementDisabled, day(10)),
		row(3, domain.ReferenceEnablementEnabled, day(20)),
		row(4, domain.ReferenceEnablementDisabled, day(20)),
	}
	cases := []struct {
		name    string
		at      time.Time
		found   bool
		status  domain.ReferenceEnablementStatus
		revised int
	}{
		{"第一行生效之前", day(-1), false, 0, 0},
		{"第一行生效当刻", day(0), true, domain.ReferenceEnablementEnabled, 1},
		{"停用之前仍是启用", day(9), true, domain.ReferenceEnablementEnabled, 1},
		{"停用生效", day(10), true, domain.ReferenceEnablementDisabled, 2},
		{"同刻生效的两行取修订号大的", day(20), true, domain.ReferenceEnablementDisabled, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			standing, found := domain.EnablementStandingAt(history, tc.at)
			if found != tc.found {
				t.Fatalf("found = %v，应为 %v", found, tc.found)
			}
			if !found {
				return
			}
			if standing.Status() != tc.status || standing.Revision() != tc.revised {
				t.Fatalf("得到 %s 修订 %d，应为 %s 修订 %d",
					standing.Status(), standing.Revision(), tc.status, tc.revised)
			}
		})
	}

	futureOnly := []domain.ReferenceEnablementRegistration{row(1, domain.ReferenceEnablementDisabled, day(30))}
	if _, found := domain.EnablementStandingAt(futureOnly, day(0)); found {
		t.Fatal("只有未来生效的行，此刻答不出来：未配置，不是停用")
	}

	shuffled := []domain.ReferenceEnablementRegistration{history[3], history[0], history[2], history[1]}
	standing, _ := domain.EnablementStandingAt(shuffled, day(15))
	if standing.Revision() != 2 {
		t.Fatalf("输入次序不影响结果：day15 应取修订 2，实际 %d", standing.Revision())
	}
}

// Covers: 重放比对只看状态、生效时点与来源单据——登记责任方与登记时间是审计，不进比对。
func TestEnablementSameContentIgnoresTheAuditFacts(t *testing.T) {
	build := func(operator string, recordedAt time.Time, source string, status domain.ReferenceEnablementStatus) domain.ReferenceEnablementRegistration {
		registration, err := domain.NewReferenceEnablementRegistration(
			commercialValue(t, domain.NewTenantID, "SYN-TENANT-01"),
			domain.ReferenceKindCountry,
			commercialValue(t, domain.NewReferenceCode, "CN"),
			1,
			domain.ReferenceEnablementSpec{
				Status:        status,
				EffectiveFrom: referenceEnablementBase,
				Source:        commercialValue(t, domain.NewReferenceEnablementSource, source),
			},
			commercialValue(t, domain.NewOperatorSubjectReference, operator),
			recordedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		return registration
	}
	original := build("idp#a", referenceEnablementBase, "MEMO-1", domain.ReferenceEnablementEnabled)
	if !original.SameContent(build("idp#b", referenceEnablementBase.Add(time.Hour), "MEMO-1", domain.ReferenceEnablementEnabled)) {
		t.Fatal("另一位操作者在另一时刻重放同一个意思，应算同内容")
	}
	if original.SameContent(build("idp#a", referenceEnablementBase, "MEMO-2", domain.ReferenceEnablementEnabled)) {
		t.Fatal("来源单据不同就是不同内容")
	}
	if original.SameContent(build("idp#a", referenceEnablementBase, "MEMO-1", domain.ReferenceEnablementDisabled)) {
		t.Fatal("状态不同就是不同内容")
	}
}
