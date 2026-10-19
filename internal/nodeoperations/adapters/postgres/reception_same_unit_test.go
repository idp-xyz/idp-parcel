package postgres_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证同一作业实物在同一节点不另起控制起点的存储面（ADR-0199 决定四，票 no-reception-source/04）：按实物
// 与节点找各链当前有效版本、这一问取的实物锁、「并入已有收寄」那一种记录的往返与库面形状，以及读收寄册的各面读不到并入行。
// 夹具全为合成（S 级）。

const sameUnitTenant = "SYN-TENANT-NRS04"

func sameUnitSource(t *testing.T, unit, node, claim string) domain.DeliverySource {
	t.Helper()
	source := domain.DeliverySource{
		Unit:        ref(t, domain.NewHandlingUnitID, unit),
		Mark:        "SYN-BARCODE-NRS04",
		Claim:       claim,
		DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-NRS04"),
		Node:        ref(t, domain.NewNodeReference, node),
		OccurredAt:  receivedAt,
	}
	switch claim {
	case "RECEIVED":
		source.Evidence = ref(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRS04")
	case "REFUSED":
		source.Refusal = "SYN-REFUSED-NRS04"
	}
	return source
}

// judgedAt 拼一行判断：形成与待识别带收寄与控制（形成另带关联 association），未收寄与待确认只带来源文档。来源文档的实物与节点
// 与收寄上的同值，除非调用方随后改它。
func judgedAt(t *testing.T, tenant, sourceID string, kind ports.ReceptionRecordKind, unit, node, version string) ports.ReceptionRecord {
	t.Helper()
	tenantID := ref(t, domain.NewTenantID, tenant)
	record := ports.ReceptionRecord{
		Key:             ports.ReceptionKey{TenantID: tenantID, SourceID: sourceID},
		ContentDigest:   "SYN-DIGEST-" + sourceID,
		Kind:            kind,
		JudgmentVersion: ref(t, domain.NewIntakeResultVersion, version),
		RecordedAt:      receivedAt.Add(time.Minute),
	}
	switch kind {
	case ports.RecordIntakeFormed, ports.RecordPendingIdentification:
		spec := domain.NodeIntakeSpec{
			TenantID:    tenantID,
			Unit:        ref(t, domain.NewHandlingUnitID, unit),
			Node:        ref(t, domain.NewNodeReference, node),
			DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-NRS04"),
			Evidence:    ref(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRS04"),
			Version:     record.JudgmentVersion,
			ReceivedAt:  receivedAt,
		}
		if kind == ports.RecordIntakeFormed {
			spec.Association = ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRS04")
			record.ServiceResultCheck = ports.ServiceResultsChecked
		}
		intake, err := domain.FormNodeIntake(spec)
		if err != nil {
			t.Fatalf("构造收寄：%v", err)
		}
		control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
			TenantID:      tenantID,
			Unit:          spec.Unit,
			Node:          spec.Node,
			Kind:          domain.EstablishedByNodeIntake,
			Basis:         ref(t, domain.NewControlBasisReference, "NODE-INTAKE/"+version),
			EstablishedAt: receivedAt,
		})
		if err != nil {
			t.Fatalf("构造控制：%v", err)
		}
		record.Intake, record.Control = intake, control
		record.Source = sameUnitSource(t, unit, node, "RECEIVED")
	case ports.RecordIntakeNotFormed:
		record.RefusalReason = "SYN-REFUSED-NRS04"
		record.Source = sameUnitSource(t, unit, node, "REFUSED")
	case ports.RecordReceptionUndecided:
		record.Source = sameUnitSource(t, unit, node, "SCAN_ONLY")
	}
	return record
}

// mergedRow 是一行「并入已有收寄」：来源文档写这次送站的实物与节点，指名并入的那一版。
func mergedRow(t *testing.T, tenant, sourceID, unit, node, mergedInto string) ports.ReceptionRecord {
	t.Helper()
	return ports.ReceptionRecord{
		Key:           ports.ReceptionKey{TenantID: ref(t, domain.NewTenantID, tenant), SourceID: sourceID},
		ContentDigest: "SYN-DIGEST-" + sourceID,
		Kind:          ports.RecordMergedIntoExistingIntake,
		MergedInto:    ref(t, domain.NewIntakeResultVersion, mergedInto),
		Source:        sameUnitSource(t, unit, node, "SCAN_ONLY"),
		RecordedAt:    receivedAt.Add(2 * time.Minute),
	}
}

func currentJudgmentsAt(
	t *testing.T,
	repository *adapter.Receptions,
	transactor bentoapp.Transactor,
	tenant, unit, node string,
) []string {
	t.Helper()
	var found []ports.ReceptionRecord
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		records, err := repository.FindCurrentJudgmentsAtNode(txCtx, ref(t, domain.NewTenantID, tenant),
			ref(t, domain.NewHandlingUnitID, unit), ref(t, domain.NewNodeReference, node))
		found = records
		return err
	}); err != nil {
		t.Fatalf("按实物与节点找 %s@%s：%v", unit, node, err)
	}
	return sourceIDsOf(found)
}

// Covers: ADR-0199 决定四「当前收寄」的读口——按（租户，作业实物，节点）交回各条判断链的当前有效版本：形成与待识别按收寄上的
// 实物与节点认（冲突版来源文档里是后到那件，按收寄认在先到那件名下），未收寄与待确认按来源文档认；被接续的、别的节点的、别的
// 实物的、别的租户的、并入行与没有判断版本的旧行都不在其中。
func TestCurrentJudgmentsAtANodeAreTheChainHeadsOfThatUnitThere(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	const unit, node = "SYN-UNIT-NRS04", "SYN-NODE-NRS04"

	formed := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-01", ports.RecordIntakeFormed, unit, node, "SYN-INTAKEV-NRS04-01")
	conflict := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-02", ports.RecordPendingIdentification, unit, node, "SYN-INTAKEV-NRS04-02")
	conflict.Predecessor, conflict.IdentityConflict = formed.JudgmentVersion, true
	conflict.Source = sameUnitSource(t, "SYN-UNIT-NRS04-LATE", node, "RECEIVED")
	undecided := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-03", ports.RecordReceptionUndecided, unit, node, "SYN-INTAKEV-NRS04-03")
	refused := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-04", ports.RecordIntakeNotFormed, unit, node, "SYN-INTAKEV-NRS04-04")
	elsewhere := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-05", ports.RecordIntakeFormed, unit, "SYN-NODE-NRS04-OTHER", "SYN-INTAKEV-NRS04-05")
	lateUnit := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-06", ports.RecordPendingIdentification, "SYN-UNIT-NRS04-LATE", node, "SYN-INTAKEV-NRS04-06")
	merged := mergedRow(t, sameUnitTenant, "SYN-SCAN-NRS04-07", unit, node, conflict.JudgmentVersion.String())
	unversioned := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-08", ports.RecordIntakeNotFormed, unit, node, "SYN-UNUSED")
	unversioned.JudgmentVersion = domain.IntakeResultVersion{}
	foreign := judgedAt(t, "SYN-TENANT-NRS04-B", "SYN-SCAN-NRS04-09", ports.RecordIntakeFormed, unit, node, "SYN-INTAKEV-NRS04-09")
	saveReceptions(t, transactor, ctx, repository,
		formed, conflict, undecided, refused, elsewhere, lateUnit, merged, unversioned, foreign)

	if got, want := currentJudgmentsAt(t, repository, transactor, sameUnitTenant, unit, node),
		[]string{"SYN-SCAN-NRS04-02", "SYN-SCAN-NRS04-03", "SYN-SCAN-NRS04-04"}; !slices.Equal(got, want) {
		t.Fatalf("%s@%s 的当前各链 = %v, want %v", unit, node, got, want)
	}
	if got, want := currentJudgmentsAt(t, repository, transactor, sameUnitTenant, "SYN-UNIT-NRS04-LATE", node),
		[]string{"SYN-SCAN-NRS04-06"}; !slices.Equal(got, want) {
		t.Fatalf("后到那件的当前各链 = %v, want %v——冲突版记在先到那件名下", got, want)
	}
	if got := currentJudgmentsAt(t, repository, transactor, sameUnitTenant, "SYN-UNIT-NRS04-NONE", node); len(got) != 0 {
		t.Fatalf("没收过的实物读到 %v", got)
	}
}

// Covers: ADR-0199 决定四「串行：按（租户，作业实物）取事务级咨询锁」——一笔事务握着某租户某件实物的锁时：同租户另一件实物、
// 另一租户同一件实物的这一问照常走完；同租户同一件实物的这一问等在锁上，换个节点问也等（锁在实物上，不在节点上）；按包裹读
// 别的实物关联用另一个锁空间，键串与实物标识逐字相同也不等。等不等由探针事务的 lock_timeout 量出来，断言都在事务闭包外。
func TestTheUnitLockSerializesOnlyTheSameTenantAndUnit(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	transactor := db.Transactor()
	tenant := ref(t, domain.NewTenantID, sameUnitTenant)
	unit := ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04")
	node := ref(t, domain.NewNodeReference, "SYN-NODE-NRS04")

	holding := make(chan struct{})
	release := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			if _, err := repository.FindCurrentJudgmentsAtNode(txCtx, tenant, unit, node); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	t.Cleanup(letGo)
	select {
	case <-holding:
	case err := <-held:
		t.Fatalf("握锁的一方没握住就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("握锁的一方没在时限内取到锁")
	}

	probe := func(ask func(context.Context) error) error {
		return transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			executor, err := db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.Exec(txCtx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
				return err
			}
			return ask(txCtx)
		})
	}
	current := func(tenant domain.TenantID, unit domain.HandlingUnitID, node domain.NodeReference) func(context.Context) error {
		return func(txCtx context.Context) error {
			_, err := repository.FindCurrentJudgmentsAtNode(txCtx, tenant, unit, node)
			return err
		}
	}
	if err := probe(current(tenant, ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04-OTHER"), node)); err != nil {
		t.Errorf("同租户另一件实物被挡住了：%v", err)
	}
	if err := probe(current(ref(t, domain.NewTenantID, "SYN-TENANT-NRS04-B"), unit, node)); err != nil {
		t.Errorf("另一租户同一件实物被挡住了：%v", err)
	}
	if err := probe(func(txCtx context.Context) error {
		_, err := repository.FindOtherUnitIntakes(txCtx, tenant, ref(t, domain.NewParcelAssociationReference, unit.String()),
			ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04-ASKING"))
		return err
	}); err != nil {
		t.Errorf("按包裹读别的实物关联被实物锁挡住了：%v", err)
	}
	var pgErr *pgconn.PgError
	for name, ask := range map[string]func(context.Context) error{
		"同一节点": current(tenant, unit, node),
		"另一节点": current(tenant, unit, ref(t, domain.NewNodeReference, "SYN-NODE-NRS04-OTHER")),
	} {
		if err := probe(ask); !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			t.Errorf("同租户同一件实物·%s err = %v, want 等在锁上直到 lock_timeout（55P03）", name, err)
		}
	}

	letGo()
	select {
	case err := <-held:
		if err != nil {
			t.Fatalf("握锁的一方：%v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("握锁的一方没在时限内收尾")
	}
}

// Covers: 按实物与节点找各链只在事务里成立——它取的事务级锁要握到收寄落册提交为止；没有事务时锁随单条语句放掉，读完到落册
// 之间不再串行，所以拒，不改用连接池。
func TestTheUnitLockedReadRefusesToRunOutsideATransaction(t *testing.T) {
	repository, _ := newReceptions(t)
	_, err := repository.FindCurrentJudgmentsAtNode(t.Context(),
		ref(t, domain.NewTenantID, sameUnitTenant),
		ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04"),
		ref(t, domain.NewNodeReference, "SYN-NODE-NRS04"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务按实物找应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0199 决定四「并入已有收寄」的存储面——并入行原样读回：种类、指名并入的那一版、来源文档；不带收寄与控制、判断
// 版本与前版。按判断版本取那一版读到的是被并入的判断，不是并入行。
func TestAMergeRecordIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	formed := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-01", ports.RecordIntakeFormed, "SYN-UNIT-NRS04", "SYN-NODE-NRS04", "SYN-INTAKEV-NRS04-01")
	merged := mergedRow(t, sameUnitTenant, "SYN-SCAN-NRS04-02", "SYN-UNIT-NRS04", "SYN-NODE-NRS04", formed.JudgmentVersion.String())
	saveReceptions(t, transactor, ctx, repository, formed, merged)

	found, exists, err := repository.FindByKey(ctx, merged.Key)
	if err != nil || !exists {
		t.Fatalf("读回并入行：exists=%v err=%v", exists, err)
	}
	if found.Kind != ports.RecordMergedIntoExistingIntake || found.MergedInto != formed.JudgmentVersion || found.Source != merged.Source {
		t.Fatalf("并入行读回 种类 %s 并入 %q 来源 %+v, want %s %q %+v", found.Kind, found.MergedInto, found.Source,
			merged.Kind, merged.MergedInto, merged.Source)
	}
	if found.JudgmentVersion.String() != "" || found.Predecessor.String() != "" || !found.Intake.ReceivedAt().IsZero() || found.Control.Active() {
		t.Fatalf("并入行读回带了判断版本、前版、收寄或控制：%+v", found)
	}
	if named, exists, err := repository.FindByJudgmentVersion(ctx, formed.Key.TenantID, formed.JudgmentVersion); err != nil || !exists || named.Key != formed.Key {
		t.Fatalf("按判断版本取 %q 读到 %q（exists=%v err=%v）, want 被并入的那一行", formed.JudgmentVersion, named.Key.SourceID, exists, err)
	}
}

// Covers: ADR-0199 决定四「并入已有收寄」的库面形状——绕过适配器直接写库的一行也过不了：并入行缺并入版本、带判断版本、带前版、
// 带收寄、缺来源文档、并入版本是空白，别的种类带并入版本，种类封闭集之外的取值；合形的并入行照常落册。
func TestTheReceptionTableRefusesAMisshapenMergeRecord(t *testing.T) {
	_, _, pool := newReceptionsWithPool(t)
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
		 recorded_at, judgment_version, predecessor_version, source_document, merged_version)
		VALUES ('SYN-TENANT-NRS04', $1, 'syn-digest', $2, $3::jsonb, $3::jsonb, '[]'::jsonb, false, '[]'::jsonb, now(),
		        $4, $5, $6::jsonb, $7)`
	const document = `{"shape":"NRS-1"}`
	for name, row := range map[string][]any{
		"并入行缺并入版本":   {"MERGED_INTO_EXISTING_INTAKE", nil, nil, nil, document, nil},
		"并入行带判断版本":   {"MERGED_INTO_EXISTING_INTAKE", nil, ptr("SYN-INTAKEV-1"), nil, document, ptr("SYN-INTAKEV-0")},
		"并入行带前版":     {"MERGED_INTO_EXISTING_INTAKE", nil, nil, ptr("SYN-INTAKEV-0"), document, ptr("SYN-INTAKEV-0")},
		"并入行带收寄":     {"MERGED_INTO_EXISTING_INTAKE", ptr(`{}`), nil, nil, document, ptr("SYN-INTAKEV-0")},
		"并入行缺来源文档":   {"MERGED_INTO_EXISTING_INTAKE", nil, nil, nil, nil, ptr("SYN-INTAKEV-0")},
		"并入版本是空白":    {"MERGED_INTO_EXISTING_INTAKE", nil, nil, nil, document, ptr("  ")},
		"待确认带并入版本":   {"RECEPTION_UNDECIDED", nil, ptr("SYN-INTAKEV-1"), nil, document, ptr("SYN-INTAKEV-0")},
		"种类封闭集之外的取值": {"MERGED", nil, nil, nil, document, ptr("SYN-INTAKEV-0")},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]any{"SYN-SCAN-NRS04-BAD"}, row...)
			_, err := pool.Exec(t.Context(), insert, args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("写入 err = %v, want CHECK 违例（23514）", err)
			}
		})
	}
	if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-NRS04-OK", "MERGED_INTO_EXISTING_INTAKE", nil, nil, nil, document, ptr("SYN-INTAKEV-0")); err != nil {
		t.Fatalf("合形的并入行被拒：%v", err)
	}
}

// Covers: ADR-0199 越权风险点「读收寄册的各面要么按种类筛，要么靠它不带收寄与控制自然读不到」，票 no-reception-source/04 逐面
// 核过：并入行不带收寄、控制、关联与候选，复核目录的收寄册与待识别册、方向依据身份一格的读口、按包裹读别的实物关联、按号找
// 待重核的待识别、包裹容纳读面都读不到它——不被读成又一次收寄、又一条关联或又一件待识别。
func TestAMergeRecordIsNotReadAsAnIntakeOrAnAssociation(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	identities, err := adapter.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	containment, err := adapter.NewParcelContainmentView(db)
	if err != nil {
		t.Fatalf("构造包裹容纳读口：%v", err)
	}
	consolidations, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	transactor := db.Transactor()
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, sameUnitTenant)

	formed := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-01", ports.RecordIntakeFormed, "SYN-UNIT-NRS04", "SYN-NODE-NRS04", "SYN-INTAKEV-NRS04-01")
	pending := judgedAt(t, sameUnitTenant, "SYN-SCAN-NRS04-02", ports.RecordPendingIdentification, "SYN-UNIT-NRS04-P", "SYN-NODE-NRS04", "SYN-INTAKEV-NRS04-02")
	pending.Candidates = []domain.ParcelAssociationReference{ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRS04-CANDIDATE")}
	intoFormed := mergedRow(t, sameUnitTenant, "SYN-SCAN-NRS04-03", "SYN-UNIT-NRS04", "SYN-NODE-NRS04", formed.JudgmentVersion.String())
	intoPending := mergedRow(t, sameUnitTenant, "SYN-SCAN-NRS04-04", "SYN-UNIT-NRS04-P", "SYN-NODE-NRS04", pending.JudgmentVersion.String())
	saveReceptions(t, transactor, ctx, receptions, formed, pending, intoFormed, intoPending)

	listed, err := catalogue.ListReceptions(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列收寄册：%v", err)
	}
	if got := receptionSourcesOf(listed); !slices.Equal(got, []string{"SYN-SCAN-NRS04-01", "SYN-SCAN-NRS04-02"}) {
		t.Errorf("收寄册列出 %v, want 只有两条判断", got)
	}
	unidentified, err := catalogue.ListUnidentifiedItems(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	if len(unidentified) != 1 || unidentified[0].SourceID != pending.Key.SourceID || unidentified[0].SucceededBy != "" {
		t.Errorf("待识别册 = %+v, want 只有那件待识别、仍是当前有效版本", unidentified)
	}
	for unit, want := range map[string]string{"SYN-UNIT-NRS04": "SYN-SCAN-NRS04-01", "SYN-UNIT-NRS04-P": "SYN-SCAN-NRS04-02"} {
		observations, err := identities.FindIntakeIdentities(ctx, tenant, ref(t, domain.NewHandlingUnitID, unit))
		if err != nil {
			t.Fatalf("读 %s 的收寄身份：%v", unit, err)
		}
		if len(observations) != 1 || observations[0].SourceID != want {
			t.Errorf("%s 的收寄身份 = %+v, want 只有 %s", unit, observations, want)
		}
	}
	var others []ports.ReceptionRecord
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		others, err = receptions.FindOtherUnitIntakes(txCtx, tenant, ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRS04"),
			ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04-ASKING"))
		return err
	})
	if got := sourceIDsOf(others); !slices.Equal(got, []string{"SYN-SCAN-NRS04-01"}) {
		t.Errorf("按包裹读别的实物关联 = %v, want 只有那条形成", got)
	}
	recheckable, err := receptions.FindRecheckableByMark(ctx, tenant, "SYN-BARCODE-NRS04")
	if err != nil {
		t.Fatalf("按号找待重核：%v", err)
	}
	if got := sourceIDsOf(recheckable); !slices.Equal(got, []string{"SYN-SCAN-NRS04-02"}) {
		t.Errorf("按号找待重核 = %v, want 只有那件待识别——并入行的来源文档带同一个号", got)
	}

	bag := openConsolidation(t, "SYN-BAG-NRS04", "SYN-ASSET-NRS04")
	saveConsolidation(t, transactor, ctx, consolidations, tenant, bag)
	for _, member := range []string{"SYN-UNIT-NRS04", "SYN-UNIT-NRS04-P"} {
		if err := bag.AddMember(ref(t, domain.NewHandlingUnitID, member)); err != nil {
			t.Fatalf("移入 %s：%v", member, err)
		}
	}
	updateConsolidation(t, transactor, ctx, consolidations, tenant, bag)
	for parcel, want := range map[string]ports.ParcelContainment{
		"SYN-PARCEL-NRS04":           ports.ParcelContained,
		"SYN-PARCEL-NRS04-CANDIDATE": ports.ParcelContainmentUnattributable,
	} {
		got, err := containment.LoadParcelContainment(ctx, tenant, ref(t, domain.NewParcelAssociationReference, parcel))
		if err != nil {
			t.Fatalf("读 %s 的容纳：%v", parcel, err)
		}
		if got != want {
			t.Errorf("%s 的容纳 = %s, want %s", parcel, got, want)
		}
	}
}

func receptionSourcesOf(rows []ports.ReceptionCatalogueRow) []string {
	sources := make([]string, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, row.SourceID)
	}
	slices.Sort(sources)
	return sources
}
