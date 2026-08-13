package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证客户资料版本登记册的行为：版本往返不丢留痕清单、
// 作用域隔离由 SQL 条件承担、版本只形成一次由主键拦住、无事务拒、回滚无痕。

var versionFormedAt = time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

// TestASourceDataVersionRoundTripsWholly 证两种基准形态整份往返：更正版本带前版链、
// 包裹范围与适用时间；基线补充版本带委托范围、无适用时间（缺失本身是客户事实）。
func TestASourceDataVersionRoundTripsWholly(t *testing.T) {
	repository, transactor := newSourceDataVersions(t)
	ctx := t.Context()

	correction := correctionVersion(t, "version-2")
	mustAppend(t, transactor, ctx, repository, shipmentIdentity(t, "tenant-1", "customer-1"), correction)

	found, exists, err := repository.FindVersion(ctx, shipmentIdentity(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewSourceDataVersionID, "version-2"))
	if err != nil {
		t.Fatalf("取回版本：%v", err)
	}
	if !exists {
		t.Fatal("已追加的版本读不回来")
	}
	if found.VersionID() != correction.VersionID() ||
		found.Intent() != domain.CorrectionIntent ||
		found.Reason() != correction.Reason() ||
		found.Requester() != correction.Requester() ||
		found.Decider() != correction.Decider() ||
		found.Authority() != correction.Authority() ||
		found.Request() != correction.Request() ||
		!found.FormedAt().Equal(correction.FormedAt()) {
		t.Fatalf("留痕清单往返变形：%+v", found)
	}
	prior, corrects := found.Basis().PriorVersion()
	if !corrects || prior.String() != "version-1" {
		t.Fatal("前版链没有读回来")
	}
	parcel, scoped := found.Scope().DeclaredParcelID()
	if !scoped || parcel.String() != "parcel-1" {
		t.Fatal("包裹范围没有读回来")
	}
	if !found.HasEffectiveAt() || !found.EffectiveAt().Equal(correction.EffectiveAt()) {
		t.Fatal("适用时间没有读回来")
	}

	supplement := supplementVersion(t, "version-3")
	mustAppend(t, transactor, ctx, repository, shipmentIdentity(t, "tenant-1", "customer-1"), supplement)
	foundSupplement, exists, err := repository.FindVersion(ctx, shipmentIdentity(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewSourceDataVersionID, "version-3"))
	if err != nil || !exists {
		t.Fatalf("取回补充版本：%v exists=%v", err, exists)
	}
	if !foundSupplement.Basis().OnAcceptanceBaseline() {
		t.Fatal("基线基准没有读回来")
	}
	if _, scoped := foundSupplement.Scope().DeclaredParcelID(); scoped {
		t.Fatal("委托级范围读成了包裹级")
	}
	if foundSupplement.HasEffectiveAt() {
		t.Fatal("客户没声明适用时间，读回却有了——缺失被顶替")
	}
}

// TestOtherScopesAreInvisibleForVersions 证否定结果不泄露其他作用域是否存在该版本。
func TestOtherScopesAreInvisibleForVersions(t *testing.T) {
	repository, transactor := newSourceDataVersions(t)
	ctx := t.Context()

	mustAppend(t, transactor, ctx, repository,
		shipmentIdentity(t, "tenant-1", "customer-1"), correctionVersion(t, "version-2"))

	elsewhere := map[string]domain.SourceIdentity{
		"另一个租户":   shipmentIdentity(t, "tenant-b", "customer-1"),
		"另一个客户账户": shipmentIdentity(t, "tenant-1", "customer-b"),
	}
	for name, scope := range elsewhere {
		_, exists, err := repository.FindVersion(ctx, scope,
			mustBuild(t, domain.NewSourceDataVersionID, "version-2"))
		if err != nil {
			t.Fatalf("%s：查询出错 %v", name, err)
		}
		if exists {
			t.Errorf("%s：读到了不属于该作用域的版本", name)
		}
	}
}

// TestAppendingTwiceKeepsTheFirstVersion 证版本只形成一次：同键重复追加译成`已记录`
// （业务答案不是 error，事务保持可用），先到者的留痕清单原样保留。
func TestAppendingTwiceKeepsTheFirstVersion(t *testing.T) {
	repository, transactor := newSourceDataVersions(t)
	ctx := t.Context()

	first := correctionVersion(t, "version-2")
	mustAppend(t, transactor, ctx, repository, shipmentIdentity(t, "tenant-1", "customer-1"), first)

	// 同版本号另一份内容：重复追加必须让先到者原样活着，绝不覆盖。
	second := correctionVersionWithReason(t, "version-2", "reason-late-writer")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Append(txCtx, shipmentIdentity(t, "tenant-1", "customer-1"), second)
		if err != nil {
			return err
		}
		if outcome != ports.SourceDataVersionAlreadyRecorded {
			t.Fatalf("outcome = %s, want ALREADY_RECORDED", outcome)
		}
		// 撞键后事务仍可用：同一个事务里立刻读回先到者。
		found, exists, err := repository.FindVersion(txCtx, shipmentIdentity(t, "tenant-1", "customer-1"),
			mustBuild(t, domain.NewSourceDataVersionID, "version-2"))
		if err != nil || !exists {
			t.Fatalf("撞键后同事务读回失败：%v exists=%v", err, exists)
		}
		if found.Reason() != first.Reason() {
			t.Fatal("后到者覆盖了先到者的留痕清单")
		}
		return nil
	})
}

// TestVersionWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
func TestVersionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newSourceDataVersions(t)
	ctx := t.Context()

	if _, err := repository.Append(ctx, shipmentIdentity(t, "tenant-1", "customer-1"),
		correctionVersion(t, "version-2")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestVersionRollbackLeavesNothingBehind 证追加与它所在的事务同生共死。
func TestVersionRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor := newSourceDataVersions(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Append(txCtx, shipmentIdentity(t, "tenant-1", "customer-1"),
			correctionVersion(t, "version-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindVersion(ctx, shipmentIdentity(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewSourceDataVersionID, "version-2"))
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后版本仍在")
	}
}

// ---- 夹具 ----

func newSourceDataVersions(t *testing.T) (*adapter.SourceDataVersions, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewSourceDataVersions(db)
	if err != nil {
		t.Fatalf("构造版本登记册：%v", err)
	}
	return repository, db.Transactor()
}

func mustAppend(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.SourceDataVersions,
	identity domain.SourceIdentity,
	version domain.CustomerSourceDataVersion,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := repository.Append(txCtx, identity, version)
		if err != nil {
			return err
		}
		if outcome != ports.SourceDataVersionAppended {
			t.Fatalf("append outcome = %s", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("事务内追加失败：%v", err)
	}
}

func shipmentIdentity(t *testing.T, tenant, customer string) domain.SourceIdentity {
	t.Helper()
	return requestScopedIdentity(t, tenant, customer, "req-key-1")
}

func amendmentFingerprint(t *testing.T, key string) domain.SourceSubmissionFingerprint {
	t.Helper()
	built, err := domain.NewSourceSubmissionFingerprint(
		requestScopedIdentity(t, "tenant-1", "customer-1", key),
		mustBuild(t, domain.NewPayloadDigest, "digest-"+key),
		versionFormedAt.Add(-time.Hour),
		versionFormedAt.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("修订来源指纹：%v", err)
	}
	return built
}

func correctionVersion(t *testing.T, versionID string) domain.CustomerSourceDataVersion {
	t.Helper()
	return correctionVersionWithReason(t, versionID, "reason-address-typo")
}

func correctionVersionWithReason(t *testing.T, versionID, reason string) domain.CustomerSourceDataVersion {
	t.Helper()
	scope, err := domain.NewParcelScopedSourceData(
		mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		mustBuild(t, domain.NewSourceDataGroupReference, "recipient-address"),
	)
	if err != nil {
		t.Fatalf("包裹级范围：%v", err)
	}
	basis, err := domain.NewAmendmentOfVersion(mustBuild(t, domain.NewSourceDataVersionID, "version-1"))
	if err != nil {
		t.Fatalf("前版基准：%v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID:   mustBuild(t, domain.NewSourceDataVersionID, versionID),
		Scope:       scope,
		Basis:       basis,
		Intent:      domain.CorrectionIntent,
		Request:     amendmentFingerprint(t, "amend-"+versionID+"-"+reason),
		Reason:      mustBuild(t, domain.NewAmendmentReasonReference, reason),
		Requester:   mustBuild(t, domain.NewRequesterReference, "customer-operator-1"),
		Decider:     mustBuild(t, domain.NewDeciderReference, "customer-1"),
		Authority:   mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "authority-snapshot-1"),
		EffectiveAt: versionFormedAt.Add(-30 * time.Minute),
		FormedAt:    versionFormedAt,
	})
	if err != nil {
		t.Fatalf("形成更正版本：%v", err)
	}
	return version
}

func supplementVersion(t *testing.T, versionID string) domain.CustomerSourceDataVersion {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		mustBuild(t, domain.NewSourceDataGroupReference, "sender-contact"),
	)
	if err != nil {
		t.Fatalf("委托级范围：%v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, versionID),
		Scope:     scope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   amendmentFingerprint(t, "amend-"+versionID),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "reason-missing-contact"),
		Requester: mustBuild(t, domain.NewRequesterReference, "customer-operator-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "customer-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "authority-snapshot-1"),
		FormedAt:  versionFormedAt,
	})
	if err != nil {
		t.Fatalf("形成补充版本：%v", err)
	}
	return version
}
