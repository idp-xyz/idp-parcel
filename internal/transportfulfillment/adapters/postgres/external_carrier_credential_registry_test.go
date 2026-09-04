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

// 本文件对真实 PostgreSQL 16 证外部承运凭证登记册（label-channel/18）：五件登记内容原样往返、
// 作废/失效/替代各成新版本而原版本一字不动、当前版按回指派生、解析口按「适用状态 × 适用范围
// × 标识对象类别」三道答`已解析`或`未知`且从不答`未配置`、写口无事务即拒、库内 CHECK 挡住领域
// 造不出的行。夹具全为合成登记（S 级），不含任何承运商的真实凭证格式。

var (
	credentialFrom    = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	credentialNow     = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	credentialChanged = time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
)

type credentialClock struct{ at time.Time }

func (clock credentialClock) Now() time.Time { return clock.at }

func newExternalCarrierCredentials(t *testing.T) (*adapter.ExternalCarrierCredentials, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewExternalCarrierCredentials(db, credentialClock{at: credentialNow})
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	return repository, db.Transactor(), pool
}

type credentialFixtureOptions struct {
	credential, version, reference string
	kind                           domain.IdentifiedObjectKind
	until                          time.Time
}

func credentialRecord(t *testing.T, options credentialFixtureOptions) ports.ExternalCarrierCredentialRecord {
	t.Helper()
	if options.kind == domain.IdentifiedObjectKindInvalid {
		options.kind = domain.IdentifiesCarriedObject
	}
	if options.reference == "" {
		options.reference = "PCL-1"
	}
	identifies, err := domain.NewIdentifiedObject(options.kind, options.reference)
	if err != nil {
		t.Fatalf("标识对象夹具：%v", err)
	}
	applicability, err := domain.NewCredentialApplicability(credentialFrom, options.until)
	if err != nil {
		t.Fatalf("适用范围夹具：%v", err)
	}
	spec := domain.ExternalCarrierCredentialSpec{
		TenantID:      segmentRef(t, domain.NewTenantID, "tenant-1"),
		Credential:    segmentRef(t, domain.NewExternalCarrierCredentialReference, options.credential),
		Version:       segmentRef(t, domain.NewExternalCarrierCredentialVersion, options.version),
		Assigner:      segmentRef(t, domain.NewCredentialAssignerReference, "carrier-x"),
		Identifies:    identifies,
		Applicability: applicability,
	}
	credential, err := domain.RegisterExternalCarrierCredential(spec)
	if err != nil {
		t.Fatalf("形成凭证夹具：%v", err)
	}
	return ports.ExternalCarrierCredentialRecord{
		Key:        ports.ExternalCarrierCredentialKey{TenantID: spec.TenantID, Credential: spec.Credential, Version: spec.Version},
		Credential: credential,
		RecordedAt: credentialFrom.Add(time.Minute),
	}
}

func recordOf(t *testing.T, credential domain.ExternalCarrierCredential, recordedAt time.Time) ports.ExternalCarrierCredentialRecord {
	t.Helper()
	return ports.ExternalCarrierCredentialRecord{
		Key: ports.ExternalCarrierCredentialKey{
			TenantID:   credential.TenantID(),
			Credential: credential.Credential(),
			Version:    credential.Version(),
		},
		Credential: credential,
		RecordedAt: recordedAt,
	}
}

func mustSaveCredential(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ExternalCarrierCredentials,
	record ports.ExternalCarrierCredentialRecord,
) ports.ExternalCarrierCredentialSaveOutcome {
	t.Helper()
	var outcome ports.ExternalCarrierCredentialSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	return outcome
}

func TestAFirstCredentialVersionRoundTripsAndResolvesToItsCarriedObject(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	ctx := t.Context()
	record := credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z001", version: "ECV-1"})
	if outcome := mustSaveCredential(t, transactor, ctx, repository, record); outcome != ports.ExternalCarrierCredentialSaved {
		t.Fatalf("首登 outcome = %s", outcome)
	}

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if !found.Credential.Equal(record.Credential) {
		t.Fatal("五件登记内容没有原样带回")
	}
	if found.Credential.Assigner().String() != "carrier-x" || found.Credential.Identifies().Reference() != "PCL-1" {
		t.Fatalf("分配方或标识对象没有原样带回：%q / %q", found.Credential.Assigner(), found.Credential.Identifies().Reference())
	}
	if _, closed := found.Credential.Applicability().Until(); closed {
		t.Fatal("开放的适用范围装回来却有了终点")
	}
	if !found.RecordedAt.Equal(record.RecordedAt) {
		t.Fatalf("登记时刻没有原样带回：%s", found.RecordedAt)
	}

	object, resolution, err := repository.ResolveCredential(ctx, record.Key.TenantID, record.Key.Credential)
	if err != nil || resolution != ports.CredentialResolved || object.String() != "PCL-1" {
		t.Fatalf("解析：%v %s %q，want RESOLVED PCL-1", err, resolution, object)
	}
}

func TestSavingTheSameVersionTwiceAnswersAlreadyRegistered(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	ctx := t.Context()
	record := credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z002", version: "ECV-1"})
	mustSaveCredential(t, transactor, ctx, repository, record)
	if outcome := mustSaveCredential(t, transactor, ctx, repository, record); outcome != ports.ExternalCarrierCredentialAlreadyRegistered {
		t.Fatalf("撞键应答已登记：%s", outcome)
	}
}

func TestRevocationFormsANewVersionAndLeavesTheOriginalUntouched(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	ctx := t.Context()
	first := credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z003", version: "ECV-1"})
	mustSaveCredential(t, transactor, ctx, repository, first)

	revoked, err := first.Credential.Revoke(credentialChanged, segmentRef(t, domain.NewExternalCarrierCredentialVersion, "ECV-2"))
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	second := recordOf(t, revoked, first.RecordedAt.Add(time.Hour))
	if outcome := mustSaveCredential(t, transactor, ctx, repository, second); outcome != ports.ExternalCarrierCredentialSaved {
		t.Fatalf("作废版本应落库：%s", outcome)
	}

	current, exists, err := repository.FindCurrent(ctx, first.Key.TenantID, first.Key.Credential)
	if err != nil || !exists {
		t.Fatalf("当前版：%v exists=%v", err, exists)
	}
	if current.Key.Version != revoked.Version() || current.Credential.Standing() != domain.CredentialRevoked {
		t.Fatalf("当前版应是未被回指的作废版：%q %s", current.Key.Version, current.Credential.Standing())
	}
	if prior, has := current.Credential.Supersedes(); !has || prior != first.Key.Version {
		t.Fatalf("作废版应回指首版：%q has=%v", prior, has)
	}
	until, closed := current.Credential.Applicability().Until()
	if !closed || !until.Equal(credentialChanged) {
		t.Fatalf("终点应落在作废时刻：%s closed=%v", until, closed)
	}
	if changedAt, has := current.Credential.ChangedAt(); !has || !changedAt.Equal(credentialChanged) {
		t.Fatalf("改变时间没有原样带回：%s has=%v", changedAt, has)
	}

	original, exists, err := repository.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("原版本应保留：%v exists=%v", err, exists)
	}
	if !original.Credential.Applicable() {
		t.Fatal("原版本被回写成不适用——只插不改被破了")
	}

	if _, resolution, err := repository.ResolveCredential(ctx, first.Key.TenantID, first.Key.Credential); err != nil || resolution != ports.CredentialUnknown {
		t.Fatalf("已作废的凭证应答未知：%v %s", err, resolution)
	}
}

func TestSupersessionCarriesTheReplacementAndVersionsListInRegistrationOrder(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	ctx := t.Context()
	first := credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z004", version: "ECV-1"})
	mustSaveCredential(t, transactor, ctx, repository, first)

	replacement := segmentRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z004-B")
	superseded, err := first.Credential.Supersede(credentialChanged, replacement, segmentRef(t, domain.NewExternalCarrierCredentialVersion, "ECV-2"))
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	mustSaveCredential(t, transactor, ctx, repository, recordOf(t, superseded, first.RecordedAt.Add(time.Hour)))

	versions, err := repository.ListVersions(ctx, first.Key.TenantID, first.Key.Credential)
	if err != nil {
		t.Fatalf("列版本：%v", err)
	}
	if len(versions) != 2 || versions[0].Key.Version != first.Key.Version || versions[1].Key.Version != superseded.Version() {
		t.Fatalf("版本应按登记先后两条：%+v", versions)
	}
	replacedBy, has := versions[1].Credential.ReplacedBy()
	if !has || replacedBy != replacement {
		t.Fatalf("替代者没有原样带回：%q has=%v", replacedBy, has)
	}
	if _, has := versions[0].Credential.ReplacedBy(); has {
		t.Fatal("首版装回来却带了替代者")
	}
}

func TestResolutionAnswersUnknownForCredentialsThatDoNotPointAtACarriedObjectNow(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")

	t.Run("从未登记", func(t *testing.T) {
		object, resolution, err := repository.ResolveCredential(ctx, tenant, segmentRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/never"))
		if err != nil || resolution != ports.CredentialUnknown || object.String() != "" {
			t.Fatalf("解析：%v %s %q，want UNKNOWN", err, resolution, object)
		}
	})

	t.Run("标识的是班次而不是载运对象", func(t *testing.T) {
		record := credentialRecord(t, credentialFixtureOptions{
			credential: "carrier-x/AWB-1", version: "ECV-1",
			kind: domain.IdentifiesTransportSchedule, reference: "SCH-1",
		})
		mustSaveCredential(t, transactor, ctx, repository, record)
		if _, resolution, err := repository.ResolveCredential(ctx, tenant, record.Key.Credential); err != nil || resolution != ports.CredentialUnknown {
			t.Fatalf("解析：%v %s，want UNKNOWN——凭证不解释为包裹的运单号", err, resolution)
		}
		found, _, _ := repository.FindByKey(ctx, record.Key)
		if found.Credential.Identifies().Kind() != domain.IdentifiesTransportSchedule {
			t.Fatalf("类别没有原样带回：%s", found.Credential.Identifies().Kind())
		}
	})

	t.Run("适用范围的终点已过而没人登失效", func(t *testing.T) {
		record := credentialRecord(t, credentialFixtureOptions{
			credential: "carrier-x/1Z005", version: "ECV-1", until: credentialNow.Add(-time.Hour),
		})
		mustSaveCredential(t, transactor, ctx, repository, record)
		if _, resolution, err := repository.ResolveCredential(ctx, tenant, record.Key.Credential); err != nil || resolution != ports.CredentialUnknown {
			t.Fatalf("解析：%v %s，want UNKNOWN——范围那一道要拦得住到期未登失效的凭证", err, resolution)
		}
	})

	t.Run("适用范围的终点还没到", func(t *testing.T) {
		record := credentialRecord(t, credentialFixtureOptions{
			credential: "carrier-x/1Z006", version: "ECV-1", until: credentialNow.Add(time.Hour),
		})
		mustSaveCredential(t, transactor, ctx, repository, record)
		if object, resolution, err := repository.ResolveCredential(ctx, tenant, record.Key.Credential); err != nil || resolution != ports.CredentialResolved || object.String() != "PCL-1" {
			t.Fatalf("解析：%v %s %q，want RESOLVED PCL-1", err, resolution, object)
		}
	})
}

func TestExternalCarrierCredentialWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newExternalCarrierCredentials(t)
	_, err := repository.Save(t.Context(), credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z007", version: "ECV-1"}))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestSaveRefusesAKeyThatDisagreesWithTheCredential(t *testing.T) {
	repository, transactor, _ := newExternalCarrierCredentials(t)
	record := credentialRecord(t, credentialFixtureOptions{credential: "carrier-x/1Z008", version: "ECV-1"})
	record.Key.Version = segmentRef(t, domain.NewExternalCarrierCredentialVersion, "ECV-other")
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := repository.Save(txCtx, record)
		return saveErr
	})
	if err == nil {
		t.Fatal("键与聚合不一致的写入落进去了")
	}
}

// TestExternalCarrierCredentialCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门：
// 状态、终点、前版、替代者四者的配合由重建门核，也由库面核——适配器绕不过任何一道。
func TestExternalCarrierCredentialCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	_, _, pool := newExternalCarrierCredentials(t)
	ctx := t.Context()
	base := `INSERT INTO transport_fulfillment.external_carrier_credential
	    (tenant_id, credential_ref, version, assigner_ref, identified_kind, identified_ref,
	     effective_from, effective_until, standing, changed_at, supersedes_version, replaced_by_credential, recorded_at) VALUES `
	for name, values := range map[string]string{
		"适用中却回指前版":  `('t','BAD-1','v2','a','CARRIED_OBJECT','o',now(),NULL,'APPLICABLE',NULL,'v1',NULL,now())`,
		"适用中却带改变时间": `('t','BAD-2','v1','a','CARRIED_OBJECT','o',now(),NULL,'APPLICABLE',now(),NULL,NULL,now())`,
		"作废却没有终点":   `('t','BAD-3','v2','a','CARRIED_OBJECT','o',now(),NULL,'REVOKED',now(),'v1',NULL,now())`,
		"作废却不回指前版":  `('t','BAD-4','v2','a','CARRIED_OBJECT','o',now(),now()+interval '1 hour','REVOKED',now(),NULL,NULL,now())`,
		"替代却没有替代者":  `('t','BAD-5','v2','a','CARRIED_OBJECT','o',now(),now()+interval '1 hour','SUPERSEDED',now(),'v1',NULL,now())`,
		"失效却带着替代者":  `('t','BAD-6','v2','a','CARRIED_OBJECT','o',now(),now()+interval '1 hour','EXPIRED',now(),'v1','BAD-6b',now())`,
		"替代者是自己":    `('t','BAD-7','v2','a','CARRIED_OBJECT','o',now(),now()+interval '1 hour','SUPERSEDED',now(),'v1','BAD-7',now())`,
		"前版指向自己":    `('t','BAD-8','v2','a','CARRIED_OBJECT','o',now(),now()+interval '1 hour','REVOKED',now(),'v2',NULL,now())`,
		"终点不晚于起点":   `('t','BAD-9','v1','a','CARRIED_OBJECT','o',now(),now(),'APPLICABLE',NULL,NULL,NULL,now())`,
		"类别不在封闭集合内": `('t','BAD-10','v1','a','TRACKING_NUMBER','o',now(),NULL,'APPLICABLE',NULL,NULL,NULL,now())`,
		"状态不在封闭集合内": `('t','BAD-11','v1','a','CARRIED_OBJECT','o',now(),NULL,'ACTIVE',NULL,NULL,NULL,now())`,
		"分配方为空":     `('t','BAD-12','v1','  ','CARRIED_OBJECT','o',now(),NULL,'APPLICABLE',NULL,NULL,NULL,now())`,
		"标识对象引用为空":  `('t','BAD-13','v1','a','CARRIED_OBJECT','',now(),NULL,'APPLICABLE',NULL,NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}
