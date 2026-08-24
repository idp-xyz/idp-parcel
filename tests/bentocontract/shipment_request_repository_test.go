package bentocontract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/repository"
	"go.idp.xyz/idp-bento-go/testkit"

	psadapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件取 `PBC-02`:Parcel 强类型复合键 Repository(委托聚合仓储,键为四维来源身份)在真实
// PostgreSQL 16 上通过 Bento Repository 合同——插入、加载、乐观版本冲突与作用域隔离。
//
// 绑定方式按 ADR-0031 的逐符号实测:框架合同收的是 Loader / Inserter / VersionedUpdater 三个
// 分开的接口,不要求本仓 `Save` 改名或改签名。本文件的三个薄壳只做「端口自己的封闭代数 →
// 框架哨兵错误」的转译,不含任何绕开适配器的读写。
//
// 场景聚合一律经重建门(ADR-0028/0030)在 Revision=1 上构造:合同把 expectedRevision 单独递给
// Update,而本仓的预期版本由聚合自己携带(`request.Revision()`,ADR-0028)——两个来源必须指
// 同一个数,壳里以显式检查钉住,不静默偏袒任何一侧。

var contractSubmittedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

func TestShipmentRequestRepositoryContract(t *testing.T) {
	testkit.RunRepositoryContract(t, func(t *testing.T) testkit.RepositoryContractScenario[domain.SourceIdentity, domain.ShipmentRequest] {
		db := frameworkDB(t)
		requests, err := psadapter.NewShipmentRequests(db)
		if err != nil {
			t.Fatalf("构造委托仓储:%v", err)
		}

		key := contractIdentity(t, "tenant-1", "key-1")
		otherKey := contractIdentity(t, "tenant-2", "key-1")

		return testkit.RepositoryContractScenario[domain.SourceIdentity, domain.ShipmentRequest]{
			Transactor:          db.Transactor(),
			Loader:              shipmentRequestLoader{requests: requests},
			Inserter:            shipmentRequestInserter{requests: requests},
			Updater:             shipmentRequestUpdater{requests: requests},
			Key:                 key,
			OtherScopeKey:       otherKey,
			Aggregate:           rehydrated(t, submittedSpec(t, key, "request-1", "version-1", "task-1")),
			UpdatedAggregate:    supplementedOnce(t, key),
			OtherScopeAggregate: rehydrated(t, submittedSpec(t, otherKey, "request-2", "version-9", "task-9")),
			Equal:               equalShipmentRequestContent,
		}
	})
}

type shipmentRequestLoader struct{ requests *psadapter.ShipmentRequests }

func (loader shipmentRequestLoader) Load(
	ctx context.Context,
	key domain.SourceIdentity,
) (repository.Loaded[domain.ShipmentRequest], error) {
	request, found, err := loader.requests.FindBySourceIdentity(ctx, key)
	if err != nil {
		return repository.Loaded[domain.ShipmentRequest]{}, err
	}
	if !found {
		return repository.Loaded[domain.ShipmentRequest]{}, repository.ErrNotFound
	}
	return repository.Loaded[domain.ShipmentRequest]{
		Aggregate: request,
		Revision:  repository.Revision(request.Revision()),
	}, nil
}

type shipmentRequestInserter struct{ requests *psadapter.ShipmentRequests }

func (inserter shipmentRequestInserter) Insert(
	ctx context.Context,
	request domain.ShipmentRequest,
) (repository.Revision, error) {
	// 键不另传:委托聚合的来源身份就在它的当前提交版本里,取它即合同说的「强类型复合键」。
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	outcome, err := inserter.requests.Insert(ctx, identity, request)
	if err != nil {
		return 0, err
	}
	switch outcome {
	case ports.ShipmentRequestInserted:
		// 适配器把首版写死为 1(Insert 的 SQL 字面量),这里如实转译,不回查一遍充数。
		return 1, nil
	case ports.ShipmentRequestAlreadyExists:
		return 0, repository.ErrAlreadyExists
	default:
		return 0, fmt.Errorf("委托插入结果 %d 不在封闭代数里", outcome)
	}
}

type shipmentRequestUpdater struct{ requests *psadapter.ShipmentRequests }

func (updater shipmentRequestUpdater) Update(
	ctx context.Context,
	key domain.SourceIdentity,
	request domain.ShipmentRequest,
	expectedRevision repository.Revision,
) (repository.Revision, error) {
	// 预期版本有两个来源:合同单独递的 expectedRevision 与聚合携带的 request.Revision()。
	// 本仓的 Save 只认后者(ADR-0028),场景构造保证两者一致;不一致说明场景造错了,报错让
	// 合同当场失败,不得静默取其一。
	if request.Revision() != int64(expectedRevision) {
		return 0, fmt.Errorf("场景聚合携带版本 %d 与合同预期 %d 不一致", request.Revision(), expectedRevision)
	}
	outcome, err := updater.requests.Save(ctx, key, request)
	if err != nil {
		return 0, err
	}
	switch outcome {
	case ports.ShipmentRequestSaved:
		// Save 的 SQL 按预期版本加一(SET revision = expected + 1),推进量是确定的。
		return expectedRevision + 1, nil
	case ports.ShipmentRequestRevisionConflict:
		return 0, repository.ErrConflict
	default:
		return 0, fmt.Errorf("委托保存结果 %d 不在封闭代数里", outcome)
	}
}

// equalShipmentRequestContent 比内容不比版本:合同用 Equal 区分场景里的几份聚合,而 Load
// 回来的聚合携带库里的版本(随保存推进),场景聚合固定造在 1 上——比版本会把「读回即对」
// 误判成不等。
func equalShipmentRequestContent(got, want domain.ShipmentRequest) bool {
	return got.ShipmentRequestID() == want.ShipmentRequestID() &&
		got.BatchID() == want.BatchID() &&
		got.State() == want.State() &&
		got.CurrentSubmissionVersion().VersionID() == want.CurrentSubmissionVersion().VersionID() &&
		len(got.PriorSubmissionVersions()) == len(want.PriorSubmissionVersions()) &&
		got.SubmittedAt().Equal(want.SubmittedAt())
}

// contractIdentity 造一份四维来源身份。作用域隔离走租户维:同来源同键不同租户,合同要求
// 互相不可见。
func contractIdentity(t *testing.T, tenant, key string) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		contractValue(t, domain.NewTenantID, tenant),
		contractValue(t, domain.NewCustomerAccountID, "customer-1"),
		contractValue(t, domain.NewSource, "portal"),
		contractValue(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("来源身份:%v", err)
	}
	return identity
}

func contractValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func contractFingerprint(
	t *testing.T,
	identity domain.SourceIdentity,
	digest string,
	occurredAt time.Time,
) domain.SourceSubmissionFingerprint {
	t.Helper()
	fingerprint, err := domain.NewSourceSubmissionFingerprint(
		identity,
		contractValue(t, domain.NewPayloadDigest, digest),
		occurredAt,
		occurredAt.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹:%v", err)
	}
	return fingerprint
}

func contractVersionSpec(
	t *testing.T,
	identity domain.SourceIdentity,
	versionID, digest string,
	establishedAt time.Time,
) domain.RehydrateSubmissionVersionSpec {
	t.Helper()
	return domain.RehydrateSubmissionVersionSpec{
		VersionID:        contractValue(t, domain.NewSubmissionVersionID, versionID),
		SourceSubmission: contractFingerprint(t, identity, digest, establishedAt.Add(-time.Hour)),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			contractValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		},
		EstablishedAt: establishedAt,
	}
}

func contractTaskSpec(t *testing.T, taskID, versionID string, establishedAt time.Time) domain.RehydrateAcceptanceTaskSpec {
	t.Helper()
	return domain.RehydrateAcceptanceTaskSpec{
		TaskID:              contractValue(t, domain.NewAcceptanceDecisionTaskID, taskID),
		SubmissionVersionID: contractValue(t, domain.NewSubmissionVersionID, versionID),
		EstablishedAt:       establishedAt,
		State:               domain.AcceptanceTaskRunning,
	}
}

func submittedSpec(
	t *testing.T,
	identity domain.SourceIdentity,
	requestID, versionID, taskID string,
) domain.RehydrateShipmentRequestSpec {
	t.Helper()
	return domain.RehydrateShipmentRequestSpec{
		// 重建的聚合必然已持久化,版本从 1 起(ADR-0028);合同的两次 Update 恰好都在 1 上做。
		Revision:          1,
		ShipmentRequestID: contractValue(t, domain.NewShipmentRequestID, requestID),
		BatchID:           contractValue(t, domain.NewSubmissionBatchID, "batch-1"),
		State:             domain.ShipmentRequestSubmitted,
		SubmittedAt:       contractSubmittedAt,
		CurrentVersion:    contractVersionSpec(t, identity, versionID, "sha256:a", contractSubmittedAt),
		AcceptanceTask:    contractTaskSpec(t, taskID, versionID, contractSubmittedAt),
	}
}

func rehydrated(t *testing.T, spec domain.RehydrateShipmentRequestSpec) domain.ShipmentRequest {
	t.Helper()
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		t.Fatalf("重建场景聚合:%v", err)
	}
	return request
}

// supplementedOnce 造「同一份委托经一次受控补充后的样子」当 UpdatedAggregate:当前版本换新、
// 旧版本连同旧任务按形成顺序留在 Prior 侧(ADR-0045)。这是`已提交`下真实会发生的内容变化,
// 不是为合同硬造的第二份文档。
func supplementedOnce(t *testing.T, identity domain.SourceIdentity) domain.ShipmentRequest {
	t.Helper()
	spec := submittedSpec(t, identity, "request-1", "version-2", "task-2")
	spec.CurrentVersion = contractVersionSpec(t, identity, "version-2", "sha256:b", contractSubmittedAt.Add(time.Hour))
	spec.AcceptanceTask = contractTaskSpec(t, "task-2", "version-2", contractSubmittedAt.Add(time.Hour))
	spec.PriorVersions = []domain.RehydrateSubmissionVersionSpec{
		contractVersionSpec(t, identity, "version-1", "sha256:a", contractSubmittedAt),
	}
	// 被补充取代的历史任务不可能还在运行——重建门拒收 Running 的历史任务,这里如实造成已停止。
	priorTask := contractTaskSpec(t, "task-1", "version-1", contractSubmittedAt)
	priorTask.State = domain.AcceptanceTaskStopped
	spec.PriorTasks = []domain.RehydrateAcceptanceTaskSpec{priorTask}
	return rehydrated(t, spec)
}
