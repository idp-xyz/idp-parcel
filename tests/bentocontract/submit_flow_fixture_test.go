package bentocontract

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psadapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件装配 `PBC-04`/`PBC-05`/`PBC-07` 共用的真实提交管线:真实的 UC-PS-001 编排
// （SubmitShipmentRequestHandler）接真实 PostgreSQL 适配器,事务边界按简报「事务边界」
// 的两段拍——来源保全独立提交,建单与「委托已提交」信封同事务原子。
//
// 两个边界壳只切事务,不含任何业务判断:生产接线（仍阻断在 Bento 闸门之后）要复用的
// 正是这两条边界,而不是这两个壳本身。归属权威与标识来源是隔离合成 `S` 替身——它们替的
// 是 Parcel 自有端口,不是框架合同（简报「实现准入检查」明文这不属「本地框架替身」）。

// submitFlowSubmittedAt 是提交管线的固定时钟读数。归属替身的有效区间锚在它上面,
// 门禁评估因此确定性通过;信封的 RecordedAt 另取一个错开的读数,两个时间位才分得出谁是谁。
var submitFlowSubmittedAt = contractSubmittedAt

// submitFlowRecordedAt 是意图适配器的固定时钟读数,与提交时刻显式错开——
// 「记录时间与领域发生时间分别填写」要靠两个不同的值才证得出。
var submitFlowRecordedAt = contractSubmittedAt.Add(5 * time.Minute)

type fixedContractClock struct{ at time.Time }

func (clock fixedContractClock) Now() time.Time { return clock.at }

// countingTransactor 数提交事务开了几次。PBC-07 的「不触发无条件自动重放」要拿它作证:
// 一次失败的 Handle 之后计数必须停在 1。
type countingTransactor struct {
	inner bentoapp.Transactor
	calls atomic.Int32
}

func (transactor *countingTransactor) WithinTransaction(
	ctx context.Context,
	fn bentoapp.TxFunc,
) error {
	transactor.calls.Add(1)
	return transactor.inner.WithinTransaction(ctx, fn)
}

// preservationBoundary 给来源保全的每笔写入各开一个事务（简报「事务边界」第一段）:
// 保全一经提交就不随后续步骤回滚,「后续解析或依赖失败不能删除该记录」由边界位置兑现。
type preservationBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.SourceSubmissionRepository
}

func (boundary preservationBoundary) FindPreserved(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool, error) {
	return boundary.inner.FindPreserved(ctx, identity)
}

func (boundary preservationBoundary) Preserve(
	ctx context.Context,
	submission domain.SourceSubmissionFingerprint,
) error {
	return boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return boundary.inner.Preserve(txCtx, submission)
	})
}

func (boundary preservationBoundary) AppendObservation(
	ctx context.Context,
	observed domain.SourceSubmissionFingerprint,
) error {
	return boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return boundary.inner.AppendObservation(txCtx, observed)
	})
}

// submissionBoundary 是简报「事务边界」的第二段:建单与「委托已提交」信封在同一个事务里
// 成立或一起消失。已存在时本事务没有写下任何东西,把答案交回编排按重放规则重答,不入队
// 第二份意图。
type submissionBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.ShipmentRequestRepository
	handoff    ports.ShipmentRequestSubmittedHandoff
}

func (boundary submissionBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

func (boundary submissionBoundary) Insert(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	var outcome ports.ShipmentRequestInsertOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		inserted, err := boundary.inner.Insert(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = inserted
		if inserted != ports.ShipmentRequestInserted {
			return nil
		}
		return boundary.handoff.HandOffShipmentRequestSubmitted(txCtx, ports.ShipmentRequestSubmittedHandoffIntent{
			Identity: identity,
			Request:  request,
		})
	})
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, err
	}
	return outcome, nil
}

func (boundary submissionBoundary) Save(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	var outcome ports.ShipmentRequestSaveOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := boundary.inner.Save(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = saved
		return nil
	})
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// contractProductionOwnership 是归属权威的隔离合成 `S` 替身:对任何拟受理范围答
// 「本产品即当前唯一生产权威、准入开放」,有效区间锚在固定时钟上。命名夹具值,
// 不是生产默认——生产归属只能来自真实权威接线。
type contractProductionOwnership struct{ anchor time.Time }

func (authority contractProductionOwnership) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	interval, err := domain.NewOwnershipValidityInterval(
		authority.anchor.Add(-time.Hour),
		authority.anchor.Add(24*time.Hour),
	)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	decisionID, err := domain.NewProductionOwnershipDecisionID("PBC-OWN-DEC-01")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	ruleVersion, err := domain.NewProductionOwnershipRuleVersion("PBC-OWN-RULE-01")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	revision, err := domain.NewProductionOwnershipRevision("PBC-OWN-REV-01")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       decisionID,
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      ruleVersion,
		AsOf:             authority.anchor.Add(-time.Minute),
		Validity:         interval,
		Revision:         revision,
		DecisionAt:       authority.anchor.Add(-time.Minute),
	})
}

// contractSubmissionIdentities 按序签发版本与任务标识。并发用例的多个协程都可能取号,
// 计数器因此是原子的;只有赢下建单的那一份进库。
type contractSubmissionIdentities struct{ counter atomic.Int64 }

func (identities *contractSubmissionIdentities) NextSubmissionVersionID(
	_ context.Context,
) (domain.SubmissionVersionID, error) {
	return domain.NewSubmissionVersionID(fmt.Sprintf("PBC-VER-%03d", identities.counter.Add(1)))
}

func (identities *contractSubmissionIdentities) NextAcceptanceDecisionTaskID(
	_ context.Context,
) (domain.AcceptanceDecisionTaskID, error) {
	return domain.NewAcceptanceDecisionTaskID(fmt.Sprintf("PBC-TASK-%03d", identities.counter.Add(1)))
}

type submitFlowFixture struct {
	pool     *pgxpool.Pool
	store    *outbox.Store
	requests *psadapter.ShipmentRequests
	handler  *psapplication.SubmitShipmentRequestHandler
	submits  *countingTransactor
}

// newSubmitFlowFixture 起一个独立数据库并装配完整提交管线。wrapSubmitTransactor 给
// PBC-07 注入「提交结果不确定」的口子;传 nil 即直接用真实事务器。计数器始终在最外层,
// 数到的是「编排试图开提交事务的次数」。
func newSubmitFlowFixture(
	t *testing.T,
	wrapSubmitTransactor func(bentoapp.Transactor) bentoapp.Transactor,
) *submitFlowFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB:%v", err)
	}
	sources, err := psadapter.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源保全仓储:%v", err)
	}
	requests, err := psadapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储:%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store:%v", err)
	}
	handoff, err := psadapter.NewOutboxShipmentRequestSubmittedHandoff(
		db, store, fixedContractClock{at: submitFlowRecordedAt})
	if err != nil {
		t.Fatalf("构造意图适配器:%v", err)
	}

	submitBase := bentoapp.Transactor(db.Transactor())
	if wrapSubmitTransactor != nil {
		submitBase = wrapSubmitTransactor(submitBase)
	}
	submits := &countingTransactor{inner: submitBase}

	handler := psapplication.NewSubmitShipmentRequestHandler(
		preservationBoundary{transactor: db.Transactor(), inner: sources},
		submissionBoundary{transactor: submits, inner: requests, handoff: handoff},
		contractProductionOwnership{anchor: submitFlowSubmittedAt},
		&contractSubmissionIdentities{},
		fixedContractClock{at: submitFlowSubmittedAt},
	)

	return &submitFlowFixture{
		pool:     pool,
		store:    store,
		requests: requests,
		handler:  handler,
		submits:  submits,
	}
}

// submitCommand 造一份完整提交命令。摘要与时间可变,好让重放（同摘要异时间）与冲突
// （异摘要）各自成立;ExpectedRevision 与归属替身的修订一致,门禁因此确定性放行。
func (fixture *submitFlowFixture) submitCommand(
	t *testing.T,
	identity domain.SourceIdentity,
	requestID, digest string,
	occurredAt time.Time,
) psapplication.SubmitShipmentRequestCommand {
	t.Helper()
	scopeReference, err := domain.NewAdmissionScopeReference("PBC-SCOPE-01")
	if err != nil {
		t.Fatalf("准入范围引用:%v", err)
	}
	scopeDigest, err := domain.NewAdmissionScopeDigest("PBC-SCOPE-DIGEST-01")
	if err != nil {
		t.Fatalf("准入范围摘要:%v", err)
	}
	scope, err := domain.NewAdmissionScope(scopeReference, scopeDigest)
	if err != nil {
		t.Fatalf("准入范围:%v", err)
	}
	return psapplication.SubmitShipmentRequestCommand{
		Identity:          identity,
		PayloadDigest:     contractValue(t, domain.NewPayloadDigest, digest),
		OccurredAt:        occurredAt,
		ReceivedAt:        occurredAt.Add(time.Second),
		BatchID:           contractValue(t, domain.NewSubmissionBatchID, "PBC-BATCH-01"),
		ShipmentRequestID: contractValue(t, domain.NewShipmentRequestID, requestID),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			contractValue(t, domain.NewDeclaredParcelID, "PBC-PARCEL-01"),
			contractValue(t, domain.NewDeclaredParcelID, "PBC-PARCEL-02"),
		},
		AdmissionScope:   scope,
		ExpectedRevision: contractValue(t, domain.NewProductionOwnershipRevision, "PBC-OWN-REV-01"),
	}
}

// submittedFlowEventID 与适配器的认领公式同形:来源身份四维加类型段。
func submittedFlowEventID(identity domain.SourceIdentity) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/shipment-request-submitted"
}

const submittedFlowEventType = "parcel-shipment.shipment-request.submitted"

// requestRowCount 数库里这个来源身份名下有几行委托。
func (fixture *submitFlowFixture) requestRowCount(t *testing.T, identity domain.SourceIdentity) int {
	t.Helper()
	var count int
	err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1 AND customer_account_id = $2
		    AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计委托行数:%v", err)
	}
	return count
}

// submittedIntentCount 数「委托已提交」类型下的全部信封。按类型而不只按 EventID 数,
// 一个意外身份的第二份信封才逃不掉。
func (fixture *submitFlowFixture) submittedIntentCount(t *testing.T) int {
	t.Helper()
	var count int
	err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		submittedFlowEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计意图行数:%v", err)
	}
	return count
}

// observationCount 数同一来源身份被追加了几次观察——重放追加、冲突不追加。
func (fixture *submitFlowFixture) observationCount(t *testing.T, identity domain.SourceIdentity) int {
	t.Helper()
	var count int
	err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.source_submission_observation
		  WHERE tenant_id = $1 AND customer_account_id = $2
		    AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计观察行数:%v", err)
	}
	return count
}
