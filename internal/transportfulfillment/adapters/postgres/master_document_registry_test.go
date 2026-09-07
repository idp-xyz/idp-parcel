package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证总单登记册（ADR-0113，迁移 0017）：登记内容与关联集原样往返、撤销 / 替代 /
// 关联重述各成新版本而原版本与原关联一字不动、当前版按回指派生、第二个首版与重放都答`已登记`、写口无事务
// 即拒、库内 CHECK 与外键挡住领域造不出的行。夹具全为合成登记（S 级），不含任何真实总单号。

var (
	masterDocumentRecordedAt = time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	masterDocumentChangedAt  = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
)

func newMasterDocuments(t *testing.T) (*adapter.MasterDocuments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewMasterDocuments(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	return repository, db.Transactor(), pool
}

func masterDocumentAssociation(t *testing.T, kind domain.AssociatedObjectKind, reference string) domain.MasterDocumentAssociation {
	t.Helper()
	association, err := domain.NewMasterDocumentAssociation(kind, reference)
	if err != nil {
		t.Fatalf("关联夹具：%v", err)
	}
	return association
}

type masterDocumentFixtureOptions struct {
	document, version   string
	commission, booking string
	associations        []domain.MasterDocumentAssociation
}

func masterDocumentRecord(t *testing.T, options masterDocumentFixtureOptions) ports.MasterDocumentRecord {
	t.Helper()
	spec := domain.MasterDocumentSpec{
		TenantID:     segmentRef(t, domain.NewTenantID, "tenant-1"),
		Document:     segmentRef(t, domain.NewMasterDocumentReference, options.document),
		Version:      segmentRef(t, domain.NewMasterDocumentVersion, options.version),
		Issuer:       segmentRef(t, domain.NewMasterDocumentIssuerReference, "party/carrier-x"),
		Scope:        segmentRef(t, domain.NewTransportScopeReference, "SYN-LANE-1"),
		Associations: options.associations,
	}
	if options.commission != "" {
		spec.Commission = segmentRef(t, domain.NewTransportCommissionReference, options.commission)
	}
	if options.booking != "" {
		spec.Booking = segmentRef(t, domain.NewBookingReference, options.booking)
	}
	document, err := domain.RegisterMasterDocument(spec)
	if err != nil {
		t.Fatalf("形成总单夹具：%v", err)
	}
	return masterDocumentRecordOf(document, masterDocumentRecordedAt)
}

func masterDocumentRecordOf(document domain.MasterDocument, recordedAt time.Time) ports.MasterDocumentRecord {
	return ports.MasterDocumentRecord{
		Key: ports.MasterDocumentKey{
			TenantID: document.TenantID(),
			Document: document.Document(),
			Version:  document.Version(),
		},
		Document:   document,
		RecordedAt: recordedAt,
	}
}

func mustSaveMasterDocument(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.MasterDocuments,
	record ports.MasterDocumentRecord,
) ports.MasterDocumentSaveOutcome {
	t.Helper()
	var outcome ports.MasterDocumentSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	return outcome
}

func TestAFirstMasterDocumentVersionRoundTripsWithItsAssociations(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	record := masterDocumentRecord(t, masterDocumentFixtureOptions{
		document: "SYN-MAWB-001", version: "MDV-1", commission: "COMM-1", booking: "BOOK-1",
		associations: []domain.MasterDocumentAssociation{
			masterDocumentAssociation(t, domain.AssociatesParcel, "PCL-2"),
			masterDocumentAssociation(t, domain.AssociatesConsolidationUnit, "CU-1"),
			masterDocumentAssociation(t, domain.AssociatesParcel, "PCL-1"),
		},
	})
	if outcome := mustSaveMasterDocument(t, transactor, ctx, repository, record); outcome != ports.MasterDocumentSaved {
		t.Fatalf("首登 outcome = %s", outcome)
	}

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if !found.Document.Equal(record.Document) {
		t.Fatalf("登记内容没有原样带回：%+v", found.Document)
	}
	if found.Document.Issuer().String() != "party/carrier-x" || found.Document.Scope().String() != "SYN-LANE-1" {
		t.Fatalf("签发方或范围没有原样带回：%q / %q", found.Document.Issuer(), found.Document.Scope())
	}
	if commission, has := found.Document.Commission(); !has || commission.String() != "COMM-1" {
		t.Fatalf("运输委托引用没有原样带回：%v %q", has, commission)
	}
	if booking, has := found.Document.Booking(); !has || booking.String() != "BOOK-1" {
		t.Fatalf("订舱引用没有原样带回：%v %q", has, booking)
	}
	associations := found.Document.Associations()
	if len(associations) != 3 || associations[0].Reference() != "CU-1" || associations[1].Reference() != "PCL-1" || associations[2].Reference() != "PCL-2" {
		t.Fatalf("关联集没有原样带回：%+v", associations)
	}
	if !found.RecordedAt.Equal(record.RecordedAt) {
		t.Fatalf("登记时刻没有原样带回：%s", found.RecordedAt)
	}

	current, exists, err := repository.FindCurrent(ctx, record.Key.TenantID, record.Key.Document)
	if err != nil || !exists || current.Key.Version != record.Key.Version {
		t.Fatalf("唯一一版就是当前版：%v exists=%v version=%q", err, exists, current.Key.Version)
	}
}

func TestAMasterDocumentWithoutOptionalReferencesOrAssociationsRoundTrips(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	record := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-002", version: "MDV-1"})
	mustSaveMasterDocument(t, transactor, ctx, repository, record)

	found, _, err := repository.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	if _, has := found.Document.Commission(); has {
		t.Fatal("没登运输委托引用却带回了一个")
	}
	if _, has := found.Document.Booking(); has {
		t.Fatal("没登订舱引用却带回了一个")
	}
	if len(found.Document.Associations()) != 0 {
		t.Fatal("零关联的首版装回来却有关联")
	}
}

func TestSavingTheSameVersionTwiceAndASecondFirstVersionBothAnswerAlreadyRegistered(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	first := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-003", version: "MDV-1"})
	mustSaveMasterDocument(t, transactor, ctx, repository, first)

	if outcome := mustSaveMasterDocument(t, transactor, ctx, repository, first); outcome != ports.MasterDocumentAlreadyRegistered {
		t.Fatalf("撞主键应答已登记：%s", outcome)
	}
	secondRoot := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-003", version: "MDV-1B"})
	if outcome := mustSaveMasterDocument(t, transactor, ctx, repository, secondRoot); outcome != ports.MasterDocumentAlreadyRegistered {
		t.Fatalf("第二个首版应撞 one_root_per_document 答已登记：%s", outcome)
	}
	if _, exists, _ := repository.FindByKey(ctx, secondRoot.Key); exists {
		t.Fatal("第二个首版落进去了——链的根不再唯一")
	}
}

func TestRevocationFormsANewVersionAndLeavesTheOriginalAndItsAssociationsUntouched(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	first := masterDocumentRecord(t, masterDocumentFixtureOptions{
		document: "SYN-MAWB-004", version: "MDV-1",
		associations: []domain.MasterDocumentAssociation{masterDocumentAssociation(t, domain.AssociatesConsolidationUnit, "CU-1")},
	})
	mustSaveMasterDocument(t, transactor, ctx, repository, first)

	revoked, err := first.Document.Revoke(masterDocumentChangedAt, segmentRef(t, domain.NewMasterDocumentVersion, "MDV-2"))
	if err != nil {
		t.Fatalf("撤销：%v", err)
	}
	if outcome := mustSaveMasterDocument(t, transactor, ctx, repository, masterDocumentRecordOf(revoked, masterDocumentRecordedAt.Add(time.Hour))); outcome != ports.MasterDocumentSaved {
		t.Fatalf("撤销版应落库：%s", outcome)
	}

	current, exists, err := repository.FindCurrent(ctx, first.Key.TenantID, first.Key.Document)
	if err != nil || !exists {
		t.Fatalf("当前版：%v exists=%v", err, exists)
	}
	if current.Key.Version != revoked.Version() || current.Document.Standing() != domain.MasterDocumentRevoked {
		t.Fatalf("当前版应是未被回指的撤销版：%q %s", current.Key.Version, current.Document.Standing())
	}
	if prior, has := current.Document.Supersedes(); !has || prior != first.Key.Version {
		t.Fatalf("撤销版应回指首版：%q has=%v", prior, has)
	}
	if changedAt, has := current.Document.ChangedAt(); !has || !changedAt.Equal(masterDocumentChangedAt) {
		t.Fatalf("改变时间没有原样带回：%s has=%v", changedAt, has)
	}
	if len(current.Document.Associations()) != 1 {
		t.Fatal("撤销版应原样带过关联集")
	}

	original, exists, err := repository.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("原版本应保留：%v exists=%v", err, exists)
	}
	if !original.Document.InForce() || len(original.Document.Associations()) != 1 {
		t.Fatal("原版本或原关联被回写——只插不改被破了")
	}
}

func TestSupersessionCarriesTheReplacementAndRestatementSwapsOnlyTheAssociations(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	first := masterDocumentRecord(t, masterDocumentFixtureOptions{
		document: "SYN-MAWB-005", version: "MDV-1",
		associations: []domain.MasterDocumentAssociation{masterDocumentAssociation(t, domain.AssociatesParcel, "PCL-1")},
	})
	mustSaveMasterDocument(t, transactor, ctx, repository, first)

	restated, err := first.Document.RestateAssociations(masterDocumentChangedAt, []domain.MasterDocumentAssociation{
		masterDocumentAssociation(t, domain.AssociatesParcel, "PCL-1"),
		masterDocumentAssociation(t, domain.AssociatesFulfillmentSegment, "SEG-1"),
	}, segmentRef(t, domain.NewMasterDocumentVersion, "MDV-2"))
	if err != nil {
		t.Fatalf("关联重述：%v", err)
	}
	mustSaveMasterDocument(t, transactor, ctx, repository, masterDocumentRecordOf(restated, masterDocumentRecordedAt.Add(time.Hour)))

	replacement := segmentRef(t, domain.NewMasterDocumentReference, "SYN-MAWB-005B")
	superseded, err := restated.Supersede(masterDocumentChangedAt.Add(time.Hour), replacement, segmentRef(t, domain.NewMasterDocumentVersion, "MDV-3"))
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	mustSaveMasterDocument(t, transactor, ctx, repository, masterDocumentRecordOf(superseded, masterDocumentRecordedAt.Add(2*time.Hour)))

	versions, err := repository.ListVersions(ctx, first.Key.TenantID, first.Key.Document)
	if err != nil {
		t.Fatalf("列版本：%v", err)
	}
	if len(versions) != 3 || versions[0].Key.Version.String() != "MDV-1" || versions[1].Key.Version.String() != "MDV-2" || versions[2].Key.Version.String() != "MDV-3" {
		t.Fatalf("版本应按登记先后三条：%+v", versions)
	}
	if !versions[1].Document.InForce() || len(versions[1].Document.Associations()) != 2 {
		t.Fatalf("重述版应仍有效且带两条关联：%s %d", versions[1].Document.Standing(), len(versions[1].Document.Associations()))
	}
	if len(versions[0].Document.Associations()) != 1 {
		t.Fatal("首版的关联集被重述改动了")
	}
	replacedBy, has := versions[2].Document.ReplacedBy()
	if !has || replacedBy != replacement || versions[2].Document.Standing() != domain.MasterDocumentSuperseded {
		t.Fatalf("替代者没有原样带回：%q has=%v %s", replacedBy, has, versions[2].Document.Standing())
	}
	if prior, _ := versions[2].Document.Supersedes(); prior.String() != "MDV-2" {
		t.Fatalf("替代版应回指重述版：%q", prior)
	}
	current, _, _ := repository.FindCurrent(ctx, first.Key.TenantID, first.Key.Document)
	if current.Key.Version.String() != "MDV-3" {
		t.Fatalf("当前版应是链尾：%q", current.Key.Version)
	}
}

func TestSupersedingTheSamePriorVersionTwiceAnswersAlreadyRegistered(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	ctx := t.Context()
	first := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-006", version: "MDV-1"})
	mustSaveMasterDocument(t, transactor, ctx, repository, first)

	revoked, _ := first.Document.Revoke(masterDocumentChangedAt, segmentRef(t, domain.NewMasterDocumentVersion, "MDV-2"))
	mustSaveMasterDocument(t, transactor, ctx, repository, masterDocumentRecordOf(revoked, masterDocumentRecordedAt.Add(time.Hour)))

	// 另一方拿同一个首版再改一次：链不允许一版被回指两次。
	restated, _ := first.Document.RestateAssociations(masterDocumentChangedAt, []domain.MasterDocumentAssociation{
		masterDocumentAssociation(t, domain.AssociatesParcel, "PCL-9"),
	}, segmentRef(t, domain.NewMasterDocumentVersion, "MDV-2B"))
	if outcome := mustSaveMasterDocument(t, transactor, ctx, repository, masterDocumentRecordOf(restated, masterDocumentRecordedAt.Add(2*time.Hour))); outcome != ports.MasterDocumentAlreadyRegistered {
		t.Fatalf("同一前版被回指两次应撞 supersedes_once 答已登记：%s", outcome)
	}
	versions, _ := repository.ListVersions(ctx, first.Key.TenantID, first.Key.Document)
	if len(versions) != 2 {
		t.Fatalf("链应仍是两版：%d", len(versions))
	}
}

func TestMasterDocumentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newMasterDocuments(t)
	_, err := repository.Save(t.Context(), masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-007", version: "MDV-1"}))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestMasterDocumentSaveRefusesAKeyThatDisagreesWithTheDocument(t *testing.T) {
	repository, transactor, _ := newMasterDocuments(t)
	record := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "SYN-MAWB-008", version: "MDV-1"})
	record.Key.Version = segmentRef(t, domain.NewMasterDocumentVersion, "MDV-other")
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := repository.Save(txCtx, record)
		return saveErr
	})
	if err == nil {
		t.Fatal("键与聚合不一致的写入落进去了")
	}
}

// TestMasterDocumentConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK、外键与部分唯一索引是第二道门：
// 状态、改变时间、前版、替代者的配合由重建门核，也由库面核——适配器绕不过任何一道。
func TestMasterDocumentConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	_, _, pool := newMasterDocuments(t)
	ctx := t.Context()
	base := `INSERT INTO transport_fulfillment.carrier_master_document
	    (tenant_id, master_document_ref, version, issuer_ref, scope_ref, commission_ref, booking_ref,
	     standing, changed_at, supersedes_version, replaced_by_document, recorded_at) VALUES `
	if _, err := pool.Exec(ctx, base+`('t','GOOD-1','v1','i','s',NULL,NULL,'IN_FORCE',NULL,NULL,NULL,now())`); err != nil {
		t.Fatalf("合法首版应落库：%v", err)
	}
	for name, values := range map[string]string{
		"首版却已撤销":      `('t','BAD-1','v1','i','s',NULL,NULL,'REVOKED',NULL,NULL,NULL,now())`,
		"回指却无改变时间":    `('t','GOOD-1','v2','i','s',NULL,NULL,'IN_FORCE',NULL,'v1',NULL,now())`,
		"有改变时间却不回指":   `('t','BAD-3','v1','i','s',NULL,NULL,'IN_FORCE',now(),NULL,NULL,now())`,
		"替代却没有替代者":    `('t','GOOD-1','v2','i','s',NULL,NULL,'SUPERSEDED',now(),'v1',NULL,now())`,
		"撤销却带着替代者":    `('t','GOOD-1','v2','i','s',NULL,NULL,'REVOKED',now(),'v1','OTHER',now())`,
		"替代者是自己":      `('t','GOOD-1','v2','i','s',NULL,NULL,'SUPERSEDED',now(),'v1','GOOD-1',now())`,
		"前版指向自己":      `('t','GOOD-1','v2','i','s',NULL,NULL,'REVOKED',now(),'v2',NULL,now())`,
		"前版不存在（外键）":   `('t','GOOD-1','v2','i','s',NULL,NULL,'REVOKED',now(),'v0',NULL,now())`,
		"第二个首版（部分唯一）": `('t','GOOD-1','v1B','i','s',NULL,NULL,'IN_FORCE',NULL,NULL,NULL,now())`,
		"状态不在封闭集合内":   `('t','BAD-9','v1','i','s',NULL,NULL,'ACTIVE',NULL,NULL,NULL,now())`,
		"签发方为空":       `('t','BAD-10','v1','  ','s',NULL,NULL,'IN_FORCE',NULL,NULL,NULL,now())`,
		"范围为空":        `('t','BAD-11','v1','i','',NULL,NULL,'IN_FORCE',NULL,NULL,NULL,now())`,
		"运输委托引用给了却空白": `('t','BAD-12','v1','i','s','  ',NULL,'IN_FORCE',NULL,NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}

	associationBase := `INSERT INTO transport_fulfillment.carrier_master_document_association
	    (tenant_id, master_document_ref, version, associated_kind, associated_ref) VALUES `
	if _, err := pool.Exec(ctx, associationBase+`('t','GOOD-1','v1','PARCEL','p-1')`); err != nil {
		t.Fatalf("合法关联应落库：%v", err)
	}
	for name, values := range map[string]string{
		"类别不在封闭集合内": `('t','GOOD-1','v1','BAG','b-1')`,
		"引用为空":      `('t','GOOD-1','v1','PARCEL','  ')`,
		"挂在不存在的版本上": `('t','GOOD-1','v9','PARCEL','p-2')`,
		"同一关联给了两遍":  `('t','GOOD-1','v1','PARCEL','p-1')`,
	} {
		t.Run("关联："+name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, associationBase+values); err == nil {
				t.Fatal("领域造不出的关联行落进去了")
			}
		})
	}
}
