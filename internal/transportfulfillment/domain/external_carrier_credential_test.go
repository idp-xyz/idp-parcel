package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	credentialFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	credentialAt   = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
)

func credentialSpec(t *testing.T) domain.ExternalCarrierCredentialSpec {
	t.Helper()
	return domain.ExternalCarrierCredentialSpec{
		TenantID:      mustRef(t, domain.NewTenantID, "tenant-1"),
		Credential:    mustRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Version:       mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-1"),
		Assigner:      mustRef(t, domain.NewCredentialAssignerReference, "party/carrier-x"),
		Identifies:    mustIdentifiedObject(t, domain.IdentifiesCarriedObject, "PCL-1"),
		Applicability: mustApplicability(t, credentialFrom, time.Time{}),
	}
}

func mustRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func mustIdentifiedObject(t *testing.T, kind domain.IdentifiedObjectKind, reference string) domain.IdentifiedObject {
	t.Helper()
	object, err := domain.NewIdentifiedObject(kind, reference)
	if err != nil {
		t.Fatalf("identified object: %v", err)
	}
	return object
}

func mustApplicability(t *testing.T, from, until time.Time) domain.CredentialApplicability {
	t.Helper()
	applicability, err := domain.NewCredentialApplicability(from, until)
	if err != nil {
		t.Fatalf("applicability: %v", err)
	}
	return applicability
}

func TestRegisteringACredentialKeepsTheFiveThingsTheContextNames(t *testing.T) {
	spec := credentialSpec(t)
	credential, err := domain.RegisterExternalCarrierCredential(spec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if credential.Assigner() != spec.Assigner || credential.Identifies() != spec.Identifies ||
		credential.Version() != spec.Version || credential.Credential() != spec.Credential {
		t.Fatal("分配方、真实标识对象、版本或凭证身份没有原样保存")
	}
	if !credential.Applicability().From().Equal(credentialFrom) {
		t.Fatalf("适用起点 = %s", credential.Applicability().From())
	}
	if _, closed := credential.Applicability().Until(); closed {
		t.Fatal("开放的适用范围不该有终点")
	}
	if credential.Standing() != domain.CredentialApplicable || !credential.Applicable() {
		t.Fatalf("首版状态 = %s", credential.Standing())
	}
	if _, has := credential.Supersedes(); has {
		t.Fatal("首版不回指任何前版")
	}
	object, isObject := credential.Identifies().CarriedObject()
	if !isObject || object.String() != "PCL-1" {
		t.Fatalf("标识载运对象的凭证要能交回该对象：%v %s", isObject, object)
	}
}

func TestACredentialIdentifyingACommissionDoesNotYieldACarriedObject(t *testing.T) {
	spec := credentialSpec(t)
	spec.Identifies = mustIdentifiedObject(t, domain.IdentifiesTransportCommission, "COMM-9")
	credential, err := domain.RegisterExternalCarrierCredential(spec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, isObject := credential.Identifies().CarriedObject(); isObject {
		t.Fatal("标识运输委托的凭证不得被解释成某个载运对象——那正是「不全部解释为包裹运单号」那条")
	}
}

func TestRegisterRefusesMissingPartsAndInvalidRanges(t *testing.T) {
	cases := map[string]func(*domain.ExternalCarrierCredentialSpec){
		"缺租户": func(spec *domain.ExternalCarrierCredentialSpec) { spec.TenantID = domain.TenantID{} },
		"缺凭证身份": func(spec *domain.ExternalCarrierCredentialSpec) {
			spec.Credential = domain.ExternalCarrierCredentialReference{}
		},
		"缺版本": func(spec *domain.ExternalCarrierCredentialSpec) {
			spec.Version = domain.ExternalCarrierCredentialVersion{}
		},
		"缺分配方":  func(spec *domain.ExternalCarrierCredentialSpec) { spec.Assigner = domain.CredentialAssignerReference{} },
		"缺标识对象": func(spec *domain.ExternalCarrierCredentialSpec) { spec.Identifies = domain.IdentifiedObject{} },
		"缺适用起点": func(spec *domain.ExternalCarrierCredentialSpec) {
			spec.Applicability = domain.CredentialApplicability{}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := credentialSpec(t)
			mutate(&spec)
			if _, err := domain.RegisterExternalCarrierCredential(spec); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
				t.Fatalf("err = %v，want ErrInvalidExternalCarrierCredential", err)
			}
		})
	}

	if _, err := domain.NewCredentialApplicability(credentialFrom, credentialFrom); err == nil {
		t.Fatal("终点不晚于起点的适用范围该被拒")
	}
	if _, err := domain.NewCredentialApplicability(time.Time{}, credentialAt); err == nil {
		t.Fatal("没有起点的适用范围该被拒")
	}
	if _, err := domain.NewIdentifiedObject(domain.IdentifiedObjectKindInvalid, "X"); err == nil {
		t.Fatal("类别不在封闭集合内的标识对象该被拒")
	}
	if _, err := domain.NewIdentifiedObject(domain.IdentifiesBooking, " "); err == nil {
		t.Fatal("空引用的标识对象该被拒")
	}
}

func TestIdentifiedObjectKindRoundTripsThroughItsClosedSet(t *testing.T) {
	for _, kind := range []domain.IdentifiedObjectKind{
		domain.IdentifiesTransportCommission, domain.IdentifiesBooking, domain.IdentifiesTransportSchedule,
		domain.IdentifiesCarriedObject, domain.IdentifiesFulfillmentSegment,
	} {
		parsed, err := domain.ParseIdentifiedObjectKind(kind.String())
		if err != nil || parsed != kind {
			t.Fatalf("%s 解析回来 = %s（%v）", kind, parsed, err)
		}
	}
	if _, err := domain.ParseIdentifiedObjectKind("PARCEL"); err == nil {
		t.Fatal("封闭集合外的类别词该被拒——不从单号格式或别的词推断")
	}
}

func TestRevokeExpireAndSupersedeOnlyChangeApplicabilityAndKeepHistory(t *testing.T) {
	first, err := domain.RegisterExternalCarrierCredential(credentialSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	second := mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-2")
	replacement := mustRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z000")

	revoked, err := first.Revoke(credentialAt, second)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Standing() != domain.CredentialRevoked || revoked.Applicable() {
		t.Fatalf("作废后的状态 = %s", revoked.Standing())
	}
	until, closed := revoked.Applicability().Until()
	if !closed || !until.Equal(credentialAt) {
		t.Fatalf("作废只该收掉适用范围的终点：%v %s", closed, until)
	}
	if prior, has := revoked.Supersedes(); !has || prior != first.Version() {
		t.Fatal("新版本必须回指被它改变的那一版")
	}
	if revoked.Version() != second || revoked.Identifies() != first.Identifies() || revoked.Assigner() != first.Assigner() {
		t.Fatal("作废不得改写分配方、标识对象，且要落在新版本上")
	}
	if first.Standing() != domain.CredentialApplicable {
		t.Fatal("原版本是值，不得被改写——历史凭证不删")
	}

	expired, err := first.Expire(credentialAt, second)
	if err != nil || expired.Standing() != domain.CredentialExpired {
		t.Fatalf("expire: %v %s", err, expired.Standing())
	}

	superseded, err := first.Supersede(credentialAt, replacement, second)
	if err != nil || superseded.Standing() != domain.CredentialSuperseded {
		t.Fatalf("supersede: %v %s", err, superseded.Standing())
	}
	if by, has := superseded.ReplacedBy(); !has || by != replacement {
		t.Fatal("替代必须记下替代它的凭证")
	}
	if _, has := revoked.ReplacedBy(); has {
		t.Fatal("作废没有替代者")
	}
}

func TestApplicabilityChangesRefuseWhatWouldRewriteHistory(t *testing.T) {
	first, err := domain.RegisterExternalCarrierCredential(credentialSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	second := mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-2")
	third := mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-3")

	if _, err := first.Revoke(credentialAt, first.Version()); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
		t.Fatalf("沿用原版本号就是覆盖：%v", err)
	}
	if _, err := first.Revoke(credentialFrom.Add(-time.Hour), second); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
		t.Fatalf("早于适用起点的改变会让未知期间倒着长：%v", err)
	}
	if _, err := first.Revoke(time.Time{}, second); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
		t.Fatalf("没有业务时间的改变：%v", err)
	}
	if _, err := first.Supersede(credentialAt, first.Credential(), second); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
		t.Fatalf("自己替代自己：%v", err)
	}

	revoked, err := first.Revoke(credentialAt, second)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := revoked.Expire(credentialAt.Add(time.Hour), third); !errors.Is(err, domain.ErrCredentialNoLongerApplicable) {
		t.Fatalf("已不适用的凭证不能再次改变适用关系：%v", err)
	}

	bounded := credentialSpec(t)
	bounded.Applicability = mustApplicability(t, credentialFrom, credentialAt)
	planned, err := domain.RegisterExternalCarrierCredential(bounded)
	if err != nil {
		t.Fatalf("register bounded: %v", err)
	}
	if _, err := planned.Revoke(credentialAt.Add(time.Hour), second); !errors.Is(err, domain.ErrInvalidExternalCarrierCredential) {
		t.Fatalf("在既定终点之后再作废等于改写已经结束的区间：%v", err)
	}
	if !planned.Applicability().Covers(credentialFrom) || planned.Applicability().Covers(credentialAt) {
		t.Fatal("适用范围是左闭右开区间")
	}
}

func TestRehydrationRebuildsWithoutRecomputingAndRefusesInconsistentRows(t *testing.T) {
	spec := domain.RehydrateExternalCarrierCredentialSpec{
		TenantID:       mustRef(t, domain.NewTenantID, "tenant-1"),
		Credential:     mustRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Version:        mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-2"),
		Assigner:       mustRef(t, domain.NewCredentialAssignerReference, "party/carrier-x"),
		Identifies:     mustIdentifiedObject(t, domain.IdentifiesCarriedObject, "PCL-1"),
		EffectiveFrom:  credentialFrom,
		EffectiveUntil: credentialAt,
		Standing:       domain.CredentialSuperseded,
		ChangedAt:      credentialAt,
		Supersedes:     mustRef(t, domain.NewExternalCarrierCredentialVersion, "CREDV-1"),
		ReplacedBy:     mustRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z000"),
	}
	credential, err := domain.RehydrateExternalCarrierCredential(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if credential.Standing() != domain.CredentialSuperseded {
		t.Fatalf("standing = %s", credential.Standing())
	}
	if by, has := credential.ReplacedBy(); !has || by != spec.ReplacedBy {
		t.Fatal("替代者没有装回")
	}

	refusals := map[string]func(*domain.RehydrateExternalCarrierCredentialSpec){
		"已替代却没有替代者": func(s *domain.RehydrateExternalCarrierCredentialSpec) {
			s.ReplacedBy = domain.ExternalCarrierCredentialReference{}
		},
		"已作废却带着替代者": func(s *domain.RehydrateExternalCarrierCredentialSpec) { s.Standing = domain.CredentialRevoked },
		"不适用却没有终点": func(s *domain.RehydrateExternalCarrierCredentialSpec) {
			s.EffectiveUntil = time.Time{}
		},
		"不适用却不回指前版": func(s *domain.RehydrateExternalCarrierCredentialSpec) {
			s.Supersedes = domain.ExternalCarrierCredentialVersion{}
		},
		"回指自己": func(s *domain.RehydrateExternalCarrierCredentialSpec) { s.Supersedes = s.Version },
		"适用中却带着改变时间": func(s *domain.RehydrateExternalCarrierCredentialSpec) {
			s.Standing = domain.CredentialApplicable
			s.ReplacedBy = domain.ExternalCarrierCredentialReference{}
			s.Supersedes = domain.ExternalCarrierCredentialVersion{}
		},
		"状态不在封闭集合内": func(s *domain.RehydrateExternalCarrierCredentialSpec) { s.Standing = domain.CredentialStanding(99) },
	}
	for name, mutate := range refusals {
		t.Run(name, func(t *testing.T) {
			bad := spec
			mutate(&bad)
			if _, err := domain.RehydrateExternalCarrierCredential(bad); !errors.Is(err, domain.ErrInvalidRehydratedExternalCarrierCredential) {
				t.Fatalf("err = %v，want ErrInvalidRehydratedExternalCarrierCredential", err)
			}
		})
	}
}
