package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证总单聚合（ADR-0113）：首版保住 CONTEXT 词条点名的六件事、关联是集合不是列表、撤销 / 替代 /
// 关联重述各成新版本回指前版而原版本不动、已撤销或已替代不再形成新版本、重建门挡住领域造不出的行。
// 夹具全是合成引用（S 级），不含任何真实总单号。

var masterDocumentChangedAt = time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)

func mustAssociation(t *testing.T, kind domain.AssociatedObjectKind, reference string) domain.MasterDocumentAssociation {
	t.Helper()
	association, err := domain.NewMasterDocumentAssociation(kind, reference)
	if err != nil {
		t.Fatalf("association: %v", err)
	}
	return association
}

func masterDocumentSpec(t *testing.T) domain.MasterDocumentSpec {
	t.Helper()
	return domain.MasterDocumentSpec{
		TenantID: mustRef(t, domain.NewTenantID, "tenant-1"),
		Document: mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1"),
		Version:  mustRef(t, domain.NewMasterDocumentVersion, "MDV-1"),
		Issuer:   mustRef(t, domain.NewMasterDocumentIssuerReference, "party/carrier-x"),
		Scope:    mustRef(t, domain.NewTransportScopeReference, "SYN-LANE-1"),
		Associations: []domain.MasterDocumentAssociation{
			mustAssociation(t, domain.AssociatesParcel, "PCL-2"),
			mustAssociation(t, domain.AssociatesConsolidationUnit, "CU-1"),
			mustAssociation(t, domain.AssociatesParcel, "PCL-1"),
		},
	}
}

func mustMasterDocument(t *testing.T, spec domain.MasterDocumentSpec) domain.MasterDocument {
	t.Helper()
	document, err := domain.RegisterMasterDocument(spec)
	if err != nil {
		t.Fatalf("register master document: %v", err)
	}
	return document
}

func TestRegisteringAMasterDocumentKeepsTheThingsTheContextNames(t *testing.T) {
	spec := masterDocumentSpec(t)
	document := mustMasterDocument(t, spec)

	if document.Document() != spec.Document || document.Version() != spec.Version ||
		document.Issuer() != spec.Issuer || document.Scope() != spec.Scope || document.TenantID() != spec.TenantID {
		t.Fatal("总单身份、版本、签发方或范围没有原样保存")
	}
	if document.Standing() != domain.MasterDocumentInForce || !document.InForce() {
		t.Fatalf("首版状态 = %s", document.Standing())
	}
	if _, has := document.Supersedes(); has {
		t.Fatal("首版不回指任何前版")
	}
	if _, has := document.ChangedAt(); has {
		t.Fatal("首版没有改变时间")
	}
	if _, has := document.ReplacedBy(); has {
		t.Fatal("首版没有替代者")
	}
	if _, has := document.Commission(); has {
		t.Fatal("没给运输委托引用却交回了一个")
	}
	if _, has := document.Booking(); has {
		t.Fatal("没给订舱引用却交回了一个")
	}
	associations := document.Associations()
	if len(associations) != 3 ||
		associations[0].Kind() != domain.AssociatesConsolidationUnit || associations[0].Reference() != "CU-1" ||
		associations[1].Kind() != domain.AssociatesParcel || associations[1].Reference() != "PCL-1" ||
		associations[2].Kind() != domain.AssociatesParcel || associations[2].Reference() != "PCL-2" {
		t.Fatalf("关联应按（类别，引用）整理成集合：%+v", associations)
	}
	associations[0] = mustAssociation(t, domain.AssociatesFulfillmentSegment, "SEG-9")
	if document.Associations()[0].Kind() != domain.AssociatesConsolidationUnit {
		t.Fatal("Associations 交回的不是副本")
	}
}

func TestAMasterDocumentMayCarryCommissionAndBookingReferencesAndStartWithNoAssociations(t *testing.T) {
	spec := masterDocumentSpec(t)
	spec.Associations = nil
	spec.Commission = mustRef(t, domain.NewTransportCommissionReference, "COMM-1")
	spec.Booking = mustRef(t, domain.NewBookingReference, "BOOK-1")
	document := mustMasterDocument(t, spec)

	if commission, has := document.Commission(); !has || commission != spec.Commission {
		t.Fatalf("运输委托引用没有原样保存：%v %q", has, commission)
	}
	if booking, has := document.Booking(); !has || booking != spec.Booking {
		t.Fatalf("订舱引用没有原样保存：%v %q", has, booking)
	}
	if len(document.Associations()) != 0 {
		t.Fatal("零关联的首版应当成立——总单先签、集运单元后列是常态")
	}
}

func TestRegistrationRefusesIncompleteOrIllFormedInput(t *testing.T) {
	cases := map[string]func(*domain.MasterDocumentSpec){
		"缺签发方": func(spec *domain.MasterDocumentSpec) { spec.Issuer = domain.MasterDocumentIssuerReference{} },
		"缺范围":  func(spec *domain.MasterDocumentSpec) { spec.Scope = domain.TransportScopeReference{} },
		"缺版本":  func(spec *domain.MasterDocumentSpec) { spec.Version = domain.MasterDocumentVersion{} },
		"缺总单引用": func(spec *domain.MasterDocumentSpec) {
			spec.Document = domain.MasterDocumentReference{}
		},
		"同一关联给了两遍": func(spec *domain.MasterDocumentSpec) {
			spec.Associations = append(spec.Associations, mustAssociation(t, domain.AssociatesParcel, "PCL-1"))
		},
		"零值关联": func(spec *domain.MasterDocumentSpec) {
			spec.Associations = append(spec.Associations, domain.MasterDocumentAssociation{})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := masterDocumentSpec(t)
			mutate(&spec)
			if _, err := domain.RegisterMasterDocument(spec); !errors.Is(err, domain.ErrInvalidMasterDocument) {
				t.Fatalf("err = %v, want ErrInvalidMasterDocument", err)
			}
		})
	}
}

func TestAssociationConstructionRefusesUnknownKindsAndBlankReferences(t *testing.T) {
	if _, err := domain.NewMasterDocumentAssociation(domain.AssociatedObjectKindInvalid, "X"); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("类别无效应拒：%v", err)
	}
	if _, err := domain.NewMasterDocumentAssociation(domain.AssociatesParcel, "  "); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("引用空白应拒：%v", err)
	}
	if _, err := domain.ParseAssociatedObjectKind("BAG"); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("词不在封闭集合内应拒：%v", err)
	}
	for _, kind := range []domain.AssociatedObjectKind{domain.AssociatesConsolidationUnit, domain.AssociatesParcel, domain.AssociatesFulfillmentSegment} {
		parsed, err := domain.ParseAssociatedObjectKind(kind.String())
		if err != nil || parsed != kind {
			t.Fatalf("%s 应能往返：%v %s", kind, err, parsed)
		}
	}
}

func TestRevocationFormsANewVersionAndLeavesTheOriginalUntouched(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")

	revoked, err := first.Revoke(masterDocumentChangedAt, second)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Standing() != domain.MasterDocumentRevoked || revoked.InForce() {
		t.Fatalf("撤销版状态 = %s", revoked.Standing())
	}
	if revoked.Version() != second {
		t.Fatalf("撤销版版本 = %q", revoked.Version())
	}
	if prior, has := revoked.Supersedes(); !has || prior != first.Version() {
		t.Fatalf("撤销版应回指首版：%v %q", has, prior)
	}
	if changedAt, has := revoked.ChangedAt(); !has || !changedAt.Equal(masterDocumentChangedAt) {
		t.Fatalf("改变时间 = %s has=%v", changedAt, has)
	}
	if _, has := revoked.ReplacedBy(); has {
		t.Fatal("撤销没有替代者")
	}
	if len(revoked.Associations()) != 3 || revoked.Issuer() != first.Issuer() || revoked.Scope() != first.Scope() {
		t.Fatal("撤销版应原样带过签发方、范围与关联集")
	}
	if !first.InForce() || first.Version().String() != "MDV-1" {
		t.Fatal("原版本被改动——值语义被破了")
	}
}

func TestSupersessionCarriesTheReplacementAndRefusesSelfOrBlank(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")
	replacement := mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1B")

	superseded, err := first.Supersede(masterDocumentChangedAt, replacement, second)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if superseded.Standing() != domain.MasterDocumentSuperseded {
		t.Fatalf("状态 = %s", superseded.Standing())
	}
	if replacedBy, has := superseded.ReplacedBy(); !has || replacedBy != replacement {
		t.Fatalf("替代者没有带上：%v %q", has, replacedBy)
	}
	if _, err := first.Supersede(masterDocumentChangedAt, first.Document(), second); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("替代者是自己应拒：%v", err)
	}
	if _, err := first.Supersede(masterDocumentChangedAt, domain.MasterDocumentReference{}, second); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("替代者空白应拒：%v", err)
	}
}

func TestRestatingAssociationsKeepsTheDocumentInForceUnderTheSameIdentity(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")
	restatedSet := []domain.MasterDocumentAssociation{
		mustAssociation(t, domain.AssociatesConsolidationUnit, "CU-1"),
		mustAssociation(t, domain.AssociatesConsolidationUnit, "CU-2"),
	}

	restated, err := first.RestateAssociations(masterDocumentChangedAt, restatedSet, second)
	if err != nil {
		t.Fatalf("restate: %v", err)
	}
	if !restated.InForce() || restated.Document() != first.Document() {
		t.Fatal("关联重述不改变状态，也不改变身份")
	}
	if prior, has := restated.Supersedes(); !has || prior != first.Version() {
		t.Fatalf("重述版应回指前版：%v %q", has, prior)
	}
	if changedAt, has := restated.ChangedAt(); !has || !changedAt.Equal(masterDocumentChangedAt) {
		t.Fatalf("改变时间 = %s has=%v", changedAt, has)
	}
	if associations := restated.Associations(); len(associations) != 2 || associations[1].Reference() != "CU-2" {
		t.Fatalf("重述后的关联集走样：%+v", associations)
	}
	if len(first.Associations()) != 3 {
		t.Fatal("原版本的关联集被改动")
	}

	t.Run("restating the same set has nothing to record", func(t *testing.T) {
		sameSetOtherOrder := []domain.MasterDocumentAssociation{
			mustAssociation(t, domain.AssociatesParcel, "PCL-1"),
			mustAssociation(t, domain.AssociatesParcel, "PCL-2"),
			mustAssociation(t, domain.AssociatesConsolidationUnit, "CU-1"),
		}
		if _, err := first.RestateAssociations(masterDocumentChangedAt, sameSetOtherOrder, second); !errors.Is(err, domain.ErrInvalidMasterDocument) {
			t.Fatalf("关联集未变应拒：%v", err)
		}
	})
}

func TestNoNewVersionFormsOnARevokedOrSupersededDocument(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")
	third := mustRef(t, domain.NewMasterDocumentVersion, "MDV-3")
	revoked, _ := first.Revoke(masterDocumentChangedAt, second)
	superseded, _ := first.Supersede(masterDocumentChangedAt, mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1B"), second)

	for name, closed := range map[string]domain.MasterDocument{"已撤销": revoked, "已替代": superseded} {
		t.Run(name, func(t *testing.T) {
			if _, err := closed.Revoke(masterDocumentChangedAt.Add(time.Hour), third); !errors.Is(err, domain.ErrMasterDocumentNoLongerInForce) {
				t.Fatalf("再撤销：%v", err)
			}
			if _, err := closed.Supersede(masterDocumentChangedAt.Add(time.Hour), mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1C"), third); !errors.Is(err, domain.ErrMasterDocumentNoLongerInForce) {
				t.Fatalf("再替代：%v", err)
			}
			if _, err := closed.RestateAssociations(masterDocumentChangedAt.Add(time.Hour), nil, third); !errors.Is(err, domain.ErrMasterDocumentNoLongerInForce) {
				t.Fatalf("再重述：%v", err)
			}
		})
	}
}

func TestAChangeRefusesTheCurrentVersionNumberAndAZeroBusinessTime(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	if _, err := first.Revoke(masterDocumentChangedAt, first.Version()); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("沿用当前版本号就是覆盖，应拒：%v", err)
	}
	if _, err := first.Revoke(time.Time{}, mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("没有改变的业务时间应拒：%v", err)
	}
}

func TestEqualComparesBusinessContentNotAssociationOrder(t *testing.T) {
	spec := masterDocumentSpec(t)
	left := mustMasterDocument(t, spec)
	reordered := spec
	reordered.Associations = []domain.MasterDocumentAssociation{spec.Associations[2], spec.Associations[0], spec.Associations[1]}
	right := mustMasterDocument(t, reordered)
	if !left.Equal(right) {
		t.Fatal("同一关联集换个顺序给应相等——关联是集合")
	}
	otherScope := spec
	otherScope.Scope = mustRef(t, domain.NewTransportScopeReference, "SYN-LANE-2")
	if left.Equal(mustMasterDocument(t, otherScope)) {
		t.Fatal("范围不同却相等")
	}
	fewer := spec
	fewer.Associations = spec.Associations[:2]
	if left.Equal(mustMasterDocument(t, fewer)) {
		t.Fatal("关联集不同却相等")
	}
}

func TestClosedWordSetsRoundTrip(t *testing.T) {
	for _, standing := range []domain.MasterDocumentStanding{domain.MasterDocumentInForce, domain.MasterDocumentRevoked, domain.MasterDocumentSuperseded} {
		parsed, err := domain.ParseMasterDocumentStanding(standing.String())
		if err != nil || parsed != standing {
			t.Fatalf("%s 应能往返：%v %s", standing, err, parsed)
		}
	}
	if _, err := domain.ParseMasterDocumentStanding("ACTIVE"); !errors.Is(err, domain.ErrInvalidMasterDocument) {
		t.Fatalf("状态词不在集合内应拒：%v", err)
	}
	words := map[string]bool{}
	for _, revision := range []domain.MasterDocumentRevision{domain.MasterDocumentRevocation, domain.MasterDocumentSupersession, domain.MasterDocumentAssociationRestatement} {
		if revision.String() == "" || words[revision.String()] {
			t.Fatalf("改变词 %d 没有唯一的封闭词：%q", revision, revision.String())
		}
		words[revision.String()] = true
	}
	if domain.MasterDocumentRevisionInvalid.String() != "" {
		t.Fatal("零值改变词不该有名字")
	}
}

func rehydratedMasterDocumentSpec(t *testing.T) domain.RehydrateMasterDocumentSpec {
	t.Helper()
	spec := masterDocumentSpec(t)
	return domain.RehydrateMasterDocumentSpec{
		TenantID:     spec.TenantID,
		Document:     spec.Document,
		Version:      spec.Version,
		Issuer:       spec.Issuer,
		Scope:        spec.Scope,
		Associations: spec.Associations,
		Standing:     domain.MasterDocumentInForce,
	}
}

func TestRehydrationRebuildsWhatTheDoorsProduce(t *testing.T) {
	first := mustMasterDocument(t, masterDocumentSpec(t))
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")
	superseded, _ := first.Supersede(masterDocumentChangedAt, mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1B"), second)

	rebuiltFirst, err := domain.RehydrateMasterDocument(rehydratedMasterDocumentSpec(t))
	if err != nil || !rebuiltFirst.Equal(first) {
		t.Fatalf("首版重建：%v equal=%v", err, rebuiltFirst.Equal(first))
	}
	spec := rehydratedMasterDocumentSpec(t)
	spec.Version = second
	spec.Standing = domain.MasterDocumentSuperseded
	spec.ChangedAt = masterDocumentChangedAt
	spec.Supersedes = first.Version()
	spec.ReplacedBy = mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1B")
	rebuiltSuperseded, err := domain.RehydrateMasterDocument(spec)
	if err != nil || !rebuiltSuperseded.Equal(superseded) {
		t.Fatalf("替代版重建：%v equal=%v", err, rebuiltSuperseded.Equal(superseded))
	}
}

func TestRehydrationRefusesRowsTheDomainCannotProduce(t *testing.T) {
	second := mustRef(t, domain.NewMasterDocumentVersion, "MDV-2")
	cases := map[string]func(*domain.RehydrateMasterDocumentSpec){
		"状态不在封闭集合内": func(spec *domain.RehydrateMasterDocumentSpec) { spec.Standing = domain.MasterDocumentStandingInvalid },
		"首版却已撤销":    func(spec *domain.RehydrateMasterDocumentSpec) { spec.Standing = domain.MasterDocumentRevoked },
		"回指却无改变时间": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Version = second
			spec.Supersedes = mustRef(t, domain.NewMasterDocumentVersion, "MDV-1")
		},
		"有改变时间却不回指": func(spec *domain.RehydrateMasterDocumentSpec) { spec.ChangedAt = masterDocumentChangedAt },
		"前版指向自己": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Supersedes = spec.Version
			spec.ChangedAt = masterDocumentChangedAt
		},
		"已替代却无替代者": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Version = second
			spec.Supersedes = mustRef(t, domain.NewMasterDocumentVersion, "MDV-1")
			spec.ChangedAt = masterDocumentChangedAt
			spec.Standing = domain.MasterDocumentSuperseded
		},
		"已撤销却带替代者": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Version = second
			spec.Supersedes = mustRef(t, domain.NewMasterDocumentVersion, "MDV-1")
			spec.ChangedAt = masterDocumentChangedAt
			spec.Standing = domain.MasterDocumentRevoked
			spec.ReplacedBy = mustRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-1B")
		},
		"替代者是自己": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Version = second
			spec.Supersedes = mustRef(t, domain.NewMasterDocumentVersion, "MDV-1")
			spec.ChangedAt = masterDocumentChangedAt
			spec.Standing = domain.MasterDocumentSuperseded
			spec.ReplacedBy = spec.Document
		},
		"关联重复": func(spec *domain.RehydrateMasterDocumentSpec) {
			spec.Associations = append(spec.Associations, mustAssociation(t, domain.AssociatesParcel, "PCL-1"))
		},
		"缺签发方": func(spec *domain.RehydrateMasterDocumentSpec) { spec.Issuer = domain.MasterDocumentIssuerReference{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := rehydratedMasterDocumentSpec(t)
			mutate(&spec)
			if _, err := domain.RehydrateMasterDocument(spec); !errors.Is(err, domain.ErrInvalidRehydratedMasterDocument) {
				t.Fatalf("err = %v, want ErrInvalidRehydratedMasterDocument", err)
			}
		})
	}
}
