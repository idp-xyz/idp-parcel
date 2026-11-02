package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func numberedShipmentDocument(t *testing.T, raw string) domain.ShipmentDocumentNumbering {
	t.Helper()
	numbering, err := domain.NumberedShipment(mustBuild(t, domain.NewShipmentDocumentNumber, raw))
	if err != nil {
		t.Fatalf("单据号：%v", err)
	}
	return numbering
}

// Covers: ADR-0355 决定三、十一——单据号与建单同一笔落在委托行上，按来源身份读回与查阅列表、单份都带它；没记号的那一行
// （单据号上线之前建的委托）读回是「上线前开立」，不是零值，也不是空号。
//
// Covers: `AT-PS-178`。
func TestTheDocumentNumberIsReadBackWithTheRequestAndTheViews(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	numbered := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-numbered", "request-numbered", submittedAtFixture)
	numbered, err := numbered.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000001"))
	if err != nil {
		t.Fatalf("记号：%v", err)
	}
	identity := numbered.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, identity, numbered); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, identity, numbered)
	})
	legacy := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-legacy", "request-legacy", submittedAtFixture.Add(-1))
	mustInsert(t, transactor, ctx, repository, legacy)

	found, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if number, present := found.DocumentNumbering().Number(); !present || number.String() != "P20260906000001" {
		t.Fatalf("读回的单据号 = %q（%v）", number, present)
	}
	if found.Revision() != 1 {
		t.Fatalf("记号推进了委托版本：revision = %d", found.Revision())
	}
	foundLegacy, _, err := repository.FindBySourceIdentity(ctx, legacy.CurrentSubmissionVersion().SourceSubmission().Identity())
	if err != nil {
		t.Fatalf("读回旧行：%v", err)
	}
	if reason, absent := foundLegacy.DocumentNumbering().Absence(); !absent || reason != domain.IssuedBeforeDocumentNumbering {
		t.Fatalf("旧行的无号情形 = %v（%v）", reason, absent)
	}
	// 读回的旧委托这一格已有结果，不能再被补上号。
	if _, err := foundLegacy.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000002")); !errors.Is(err, domain.ErrShipmentDocumentNumberingAlreadyRecorded) {
		t.Fatalf("旧委托被补号：%v", err)
	}

	listed, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 10)
	if err != nil || len(listed) != 2 {
		t.Fatalf("列表：%d 行，%v", len(listed), err)
	}
	if number, present := listed[0].DocumentNumbering.Number(); !present || number.String() != "P20260906000001" {
		t.Fatalf("列表首行单据号 = %q（%v）", number, present)
	}
	if reason, _ := listed[1].DocumentNumbering.Absence(); reason != domain.IssuedBeforeDocumentNumbering {
		t.Fatalf("列表旧行无号情形 = %v", reason)
	}
	// 按单据号检索（运单号）：作用域内逐字命中那一份；作用域外的账户与没有这个号同答空。
	byNumber, err := views.ListVisibleByDocumentNumber(ctx, viewScope(t, "tenant-1", "customer-1"), mustBuild(t, domain.NewShipmentDocumentNumber, "P20260906000001"))
	if err != nil || len(byNumber) != 1 || byNumber[0].ShipmentRequestID != numbered.ShipmentRequestID() {
		t.Fatalf("按单据号检索：%+v %v", byNumber, err)
	}
	for name, probe := range map[string]struct {
		scope  domain.AuthorizedQueryScope
		number string
	}{
		"other account": {viewScope(t, "tenant-1", "customer-2"), "P20260906000001"},
		"other tenant":  {viewScope(t, "tenant-2", "customer-1"), "P20260906000001"},
		"case folded":   {viewScope(t, "tenant-1", "customer-1"), "p20260906000001"},
	} {
		found, err := views.ListVisibleByDocumentNumber(ctx, probe.scope, mustBuild(t, domain.NewShipmentDocumentNumber, probe.number))
		if err != nil || len(found) != 0 {
			t.Fatalf("%s：%+v %v", name, found, err)
		}
	}
	detail, visible, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"), numbered.ShipmentRequestID())
	if err != nil || !visible {
		t.Fatalf("单份：%v %v", visible, err)
	}
	if number, _ := detail.DocumentNumbering.Number(); number.String() != "P20260906000001" {
		t.Fatalf("单份单据号 = %q", number)
	}
}

// Covers: ADR-0355 决定十一——别的上下文的页面按一组委托标识取回运单号：作用域内的那几份带号或带无号情形交回，作用域外的
// （别的账户、别的租户）与查无此标识的都不出现，重复的标识只命中一行；空组不读也不错。
//
// Covers: `AT-PS-180`。
func TestASetOfShipmentRequestIDsIsLookedUpWithinTheScope(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	numbered := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-numbered", "request-numbered", submittedAtFixture)
	numbered, err := numbered.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000001"))
	if err != nil {
		t.Fatal(err)
	}
	identity := numbered.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, identity, numbered); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, identity, numbered)
	})
	legacy := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-legacy", "request-legacy", submittedAtFixture.Add(-1))
	mustInsert(t, transactor, ctx, repository, legacy)
	otherAccount := scopedSubmittedRequest(t, "tenant-1", "customer-2", "key-other-account", "request-other-account", submittedAtFixture)
	mustInsert(t, transactor, ctx, repository, otherAccount)
	otherTenant := scopedSubmittedRequest(t, "tenant-2", "customer-1", "key-other-tenant", "request-other-tenant", submittedAtFixture)
	mustInsert(t, transactor, ctx, repository, otherTenant)

	ids := []domain.ShipmentRequestID{
		numbered.ShipmentRequestID(),
		legacy.ShipmentRequestID(),
		otherAccount.ShipmentRequestID(),
		otherTenant.ShipmentRequestID(),
		mustBuild(t, domain.NewShipmentRequestID, "request-missing"),
		numbered.ShipmentRequestID(),
	}
	found, err := views.ListVisibleByIDs(ctx, viewScope(t, "tenant-1", "customer-1"), ids)
	if err != nil || len(found) != 2 {
		t.Fatalf("按标识集合：%d 行，%v", len(found), err)
	}
	if found[0].ShipmentRequestID != numbered.ShipmentRequestID() || found[1].ShipmentRequestID != legacy.ShipmentRequestID() {
		t.Fatalf("交回的行不是作用域内那两份：%v、%v", found[0].ShipmentRequestID, found[1].ShipmentRequestID)
	}
	if number, present := found[0].DocumentNumbering.Number(); !present || number.String() != "P20260906000001" {
		t.Fatalf("有号那一份的单据号 = %q（%v）", number, present)
	}
	if reason, absent := found[1].DocumentNumbering.Absence(); !absent || reason != domain.IssuedBeforeDocumentNumbering {
		t.Fatalf("上线前那一份的无号情形 = %v（%v）", reason, absent)
	}

	wider, err := views.ListVisibleByIDs(ctx, viewScope(t, "tenant-1", "customer-1", "customer-2"), ids)
	if err != nil || len(wider) != 3 {
		t.Fatalf("可见账户放宽到两个：%d 行，%v", len(wider), err)
	}
	if empty, err := views.ListVisibleByIDs(ctx, viewScope(t, "tenant-1", "customer-1"), nil); err != nil || len(empty) != 0 {
		t.Fatalf("空组：%v %v", empty, err)
	}
}

// Covers: ADR-0355 决定七——开立时单据编号时区未配置的委托记下这个原因，读回与查阅都如实说是这一种，不并成上线前开立。
//
// Covers: `AT-PS-178`。
func TestATimeZoneNotConfiguredAbsenceIsReadBackAsItself(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	unconfigured, err := domain.UnnumberedShipment(domain.DocumentNumberingTimeZoneNotConfigured)
	if err != nil {
		t.Fatal(err)
	}
	request := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-1", "request-1", submittedAtFixture)
	request, err = request.RecordDocumentNumbering(unconfigured)
	if err != nil {
		t.Fatal(err)
	}
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, identity, request); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, identity, request)
	})

	found, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if reason, _ := found.DocumentNumbering().Absence(); reason != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("读回的无号情形 = %v", reason)
	}
	listed, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("列表：%v %v", listed, err)
	}
	if reason, _ := listed[0].DocumentNumbering.Absence(); reason != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("列表无号情形 = %v", reason)
	}
}

// Covers: 记号只写两列都还空着的那一行、只在事务里写；判撞的「是否已用」按租户查、只在事务里查；（租户，单据号）唯一约束
// 是最后一道保险，跨租户同号不算撞（ADR-0355 决定三、八）。
func TestRecordingAndUsedNumberChecksStayInsideTheTransactionAndTheTenant(t *testing.T) {
	_, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	first := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-1", "request-1", submittedAtFixture)
	first, err := first.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000001"))
	if err != nil {
		t.Fatal(err)
	}
	identity := first.CurrentSubmissionVersion().SourceSubmission().Identity()
	tenant := identity.TenantID()

	if err := repository.RecordShipmentDocumentNumbering(ctx, identity, first); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外记号应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.ShipmentDocumentNumberUsed(ctx, tenant, "P20260906000001"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外判撞应返回 ErrTransactionRequired，实得：%v", err)
	}

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, identity, first); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, identity, first)
	})
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.RecordShipmentDocumentNumbering(txCtx, identity, first)
	})
	if err == nil {
		t.Fatal("已有号的委托被再记一次")
	}

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		used, err := repository.ShipmentDocumentNumberUsed(txCtx, tenant, "P20260906000001")
		if err != nil || !used {
			t.Errorf("本租户已用的号答未用：%v %v", used, err)
		}
		other, err := repository.ShipmentDocumentNumberUsed(txCtx, mustBuild(t, domain.NewTenantID, "tenant-2"), "P20260906000001")
		if err != nil || other {
			t.Errorf("别的租户的号算成本租户已用：%v %v", other, err)
		}
		return nil
	})

	// 同租户同号：唯一约束拦下，整笔回滚、第二份委托不留。
	duplicate := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-2", "request-2", submittedAtFixture)
	duplicate, err = duplicate.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000001"))
	if err != nil {
		t.Fatal(err)
	}
	duplicateIdentity := duplicate.CurrentSubmissionVersion().SourceSubmission().Identity()
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, duplicateIdentity, duplicate); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, duplicateIdentity, duplicate)
	})
	if err == nil {
		t.Fatal("同租户同号被唯一约束放过")
	}
	if _, found, _ := repository.FindBySourceIdentity(ctx, duplicateIdentity); found {
		t.Fatal("撞唯一约束的那一笔没有整笔回滚")
	}

	// 别的租户同号：各自有号，不撞。
	neighbour := scopedSubmittedRequest(t, "tenant-2", "customer-1", "key-1", "request-1", submittedAtFixture)
	neighbour, err = neighbour.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000001"))
	if err != nil {
		t.Fatal(err)
	}
	neighbourIdentity := neighbour.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Insert(txCtx, neighbourIdentity, neighbour); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(txCtx, neighbourIdentity, neighbour)
	})
}

// Covers: ADR-0355 决定十一——释放未受理册的运营查阅随行带出所指委托的单据号；册行指的委托读不到时那一行没有这一格，
// 不拿「上线前开立」替它说。
func TestTheRefusalCatalogueCarriesTheDocumentNumberOfTheRequestEachRowNames(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	repository, err := adapter.NewShipmentRequests(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	request := scopedSubmittedRequest(t, "tenant-1", "customer-1", "key-1", "request-1", submittedAtFixture)
	request, err = request.RecordDocumentNumbering(numberedShipmentDocument(t, "P20260906000009"))
	if err != nil {
		t.Fatal(err)
	}
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	rig.runInTx(t, func(ctx context.Context) error {
		if _, err := repository.Insert(ctx, identity, request); err != nil {
			return err
		}
		return repository.RecordShipmentDocumentNumbering(ctx, identity, request)
	})
	rig.record(t, refusalFor(t, "tenant-1", "request-1", "version-1", refusalRecordedAt, syntheticScope(t)))
	rig.record(t, refusalFor(t, "tenant-1", "request-unknown", "version-1", refusalRecordedAt.Add(time.Minute), syntheticScope(t)))

	listed, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), psTenant(t, "tenant-1"), 10)
	if err != nil || len(listed.Refusals) != 2 {
		t.Fatalf("查阅：%d 行 %v", len(listed.Refusals), err)
	}
	numbering, found := listed.DocumentNumbers[mustBuild(t, domain.NewShipmentRequestID, "request-1")]
	if number, _ := numbering.Number(); !found || number.String() != "P20260906000009" {
		t.Fatalf("册行所指委托的单据号 = %q（%v）", number, found)
	}
	if _, found := listed.DocumentNumbers[mustBuild(t, domain.NewShipmentRequestID, "request-unknown")]; found {
		t.Fatal("读不到的委托也带了单据号这一格")
	}
}
