package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 生产装配要往两个窄口里插的就是 PC 的参与方身份登记册。编译期钉住，形状不合当场编不过——靠装配时才发现，
// 表现出来是一格看不出原因的`未决`。
var (
	_ adapter.BusinessPartySource      = (*pcpostgres.PartyIdentityRegistrations)(nil)
	_ adapter.LegalEntitySource        = (*pcpostgres.PartyIdentityRegistrations)(nil)
	_ tfports.CarrierIdentityDirectory = (*adapter.CarrierIdentityDirectory)(nil)
)

type partySourceDouble struct {
	registered map[string]bool
	err        error
	tenants    []string
}

func (double *partySourceDouble) LoadLatestBusinessParty(
	_ context.Context,
	tenant pcdomain.TenantID,
	party pcdomain.PartyID,
) (pcdomain.BusinessPartyRegistration, bool, error) {
	double.tenants = append(double.tenants, tenant.String())
	if double.err != nil {
		return pcdomain.BusinessPartyRegistration{}, false, double.err
	}
	return pcdomain.BusinessPartyRegistration{}, double.registered[party.String()], nil
}

type entitySourceDouble struct {
	registered map[string]bool
	err        error
}

func (double *entitySourceDouble) LoadLatestLegalEntity(
	_ context.Context,
	_ pcdomain.TenantID,
	entity pcdomain.LegalEntityReference,
) (pcdomain.LegalEntityRegistration, bool, error) {
	if double.err != nil {
		return pcdomain.LegalEntityRegistration{}, false, double.err
	}
	return pcdomain.LegalEntityRegistration{}, double.registered[entity.String()], nil
}

func newDirectory(t *testing.T, parties *partySourceDouble, entities *entitySourceDouble) *adapter.CarrierIdentityDirectory {
	t.Helper()
	directory, err := adapter.NewCarrierIdentityDirectory(parties, entities)
	if err != nil {
		t.Fatalf("build directory: %v", err)
	}
	return directory
}

func subject(t *testing.T, kind tfdomain.CarrierSubjectKind, reference string) tfdomain.CarrierSubject {
	t.Helper()
	built, err := tfdomain.NewCarrierSubject(kind, reference)
	if err != nil {
		t.Fatalf("carrier subject: %v", err)
	}
	return built
}

func tenant(t *testing.T) tfdomain.TenantID {
	t.Helper()
	built, err := tfdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return built
}

// Covers: CONTEXT Boundaries「实际承运商判断的判断值只引用 party-commercial 已登记的参与方或运营法人身份，
// 本上下文不为承运方铸身份」——两支各查各的册：外部参与方查参与方册，自营法人查法人册。
func TestEachSubjectBranchIsLookedUpInItsOwnRegister(t *testing.T) {
	parties := &partySourceDouble{registered: map[string]bool{"party/carrier-x": true}}
	entities := &entitySourceDouble{registered: map[string]bool{"LE-OPERATOR": true}}
	directory := newDirectory(t, parties, entities)

	cases := map[string]struct {
		subject tfdomain.CarrierSubject
		want    bool
	}{
		"在册外部参与方":     {subject: subject(t, tfdomain.ExternalCarrierParty, "party/carrier-x"), want: true},
		"未在册外部参与方":    {subject: subject(t, tfdomain.ExternalCarrierParty, "party/carrier-y"), want: false},
		"在册运营法人":      {subject: subject(t, tfdomain.OwnOperatingLegalEntity, "LE-OPERATOR"), want: true},
		"未在册运营法人":     {subject: subject(t, tfdomain.OwnOperatingLegalEntity, "LE-OTHER"), want: false},
		"法人编号拿去查参与方册": {subject: subject(t, tfdomain.ExternalCarrierParty, "LE-OPERATOR"), want: false},
		"参与方编号拿去查法人册": {subject: subject(t, tfdomain.OwnOperatingLegalEntity, "party/carrier-x"), want: false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := directory.IdentityRegistered(context.Background(), tenant(t), test.subject)
			if err != nil {
				t.Fatalf("lookup: %v", err)
			}
			if got != test.want {
				t.Fatalf("registered = %v, want %v", got, test.want)
			}
		})
	}
	// 租户按原值传到 PC 一侧：跨越租户必须在签名上看得见（ADR-0003），适配器不替换也不省略它。
	for _, seen := range parties.tenants {
		if seen != "tenant-1" {
			t.Fatalf("传到参与方册的租户是 %q", seen)
		}
	}
}

// 读不到一律上抛，绝不折成「未登记」（ADR-0029）：折了，一次故障就会变成一版待确认（承运主体身份未登记）。
func TestReadFailuresAreNeverFoldedIntoNotRegistered(t *testing.T) {
	unavailable := errors.New("登记册不可读")
	cases := map[string]struct {
		directory *adapter.CarrierIdentityDirectory
		subject   tfdomain.CarrierSubject
	}{
		"参与方册读不通": {
			directory: newDirectory(t, &partySourceDouble{err: unavailable}, &entitySourceDouble{}),
			subject:   subject(t, tfdomain.ExternalCarrierParty, "party/carrier-x"),
		},
		"法人册读不通": {
			directory: newDirectory(t, &partySourceDouble{}, &entitySourceDouble{err: unavailable}),
			subject:   subject(t, tfdomain.OwnOperatingLegalEntity, "LE-OPERATOR"),
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			registered, err := test.directory.IdentityRegistered(context.Background(), tenant(t), test.subject)
			if !errors.Is(err, unavailable) {
				t.Fatalf("error = %v, want wrapped %v", err, unavailable)
			}
			if registered {
				t.Fatal("读不到却答了在册")
			}
		})
	}
}

// 分支之外的取值是编程错误不是业务答案：零值主体不该走到任何一本册子上。
func TestAnUntranslatableSubjectIsRefusedBeforeAnyRegisterIsAsked(t *testing.T) {
	parties := &partySourceDouble{err: errors.New("不该被问到")}
	directory := newDirectory(t, parties, &entitySourceDouble{err: errors.New("不该被问到")})

	_, err := directory.IdentityRegistered(context.Background(), tenant(t), tfdomain.CarrierSubject{})
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("error = %v, want ErrUntranslatableAnswer", err)
	}
	if len(parties.tenants) != 0 {
		t.Fatal("零值主体仍去问了参与方册")
	}
}

func TestTheDirectoryRefusesToBuildWithoutBothRegisters(t *testing.T) {
	if _, err := adapter.NewCarrierIdentityDirectory(nil, &entitySourceDouble{}); err == nil {
		t.Fatal("没有参与方册也装得起来——外部承运方那一支从此答不出")
	}
	if _, err := adapter.NewCarrierIdentityDirectory(&partySourceDouble{}, nil); err == nil {
		t.Fatal("没有法人册也装得起来——自营那一支从此答不出")
	}
}
