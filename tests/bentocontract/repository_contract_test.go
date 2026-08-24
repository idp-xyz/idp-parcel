package bentocontract

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"go.idp.xyz/idp-bento-go/repository"
	"go.idp.xyz/idp-bento-go/testkit"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件取 `PBC-02`：Parcel 强类型复合键仓储通过框架 Repository 合同——插入、加载、
// 乐观版本冲突、重复插入、作用域隔离、回滚与严格写入全部由框架的 RunRepositoryContract
// 在真实 PostgreSQL 16 上驱动。
//
// 受测的是真实适配器 adapters/postgres.ShipmentRequests。它的端口语言与框架合同的
// 语言不同且**不该**相同：端口按 ADR-0031 说封闭结果代数（`已存在`/`版本冲突`是业务
// 答案不是 error），预期版本由聚合自己携带（request.Revision()）；框架合同说哨兵错误
// 与显式 expectedRevision。下面的场景壳只做这一层翻译——这正是 bento-gate-reeval
// 票 03 步骤 1 复核结论所指的绑定方式，端口自身的签名一个都不用改。
func TestShipmentRequestRepositoryContract(t *testing.T) {
	testkit.RunRepositoryContract(t,
		func(t *testing.T) testkit.RepositoryContractScenario[domain.SourceIdentity, domain.ShipmentRequest] {
			db, _ := parcelDB(t)
			requests, err := pspostgres.NewShipmentRequests(db)
			if err != nil {
				t.Fatalf("构造委托仓储：%v", err)
			}
			shell := shipmentRequestContractShell{t: t, requests: requests}

			base := submittedContractRequest(t,
				contractScope{tenant: "tenant-a", customer: "customer-a"}, "contract-key-a", "contract-request-a")
			// 另一作用域换租户：作用域隔离的考点是「即使外部键相同也必须隔离」，
			// 所以两份聚合共用同一个来源请求键。
			otherScope := submittedContractRequest(t,
				contractScope{tenant: "tenant-b", customer: "customer-b"}, "contract-key-a", "contract-request-b")

			return testkit.RepositoryContractScenario[domain.SourceIdentity, domain.ShipmentRequest]{
				Transactor:          db.Transactor(),
				Loader:              shell,
				Inserter:            shell,
				Updater:             shell,
				Key:                 requestSourceIdentity(base),
				OtherScopeKey:       requestSourceIdentity(otherScope),
				Aggregate:           base,
				UpdatedAggregate:    withProcessingAttempt(t, base),
				OtherScopeAggregate: otherScope,
				// 比较经 contractRequestSpec 摊平后按结构逐字段进行；Revision 归一为 1，
				// 因为合同对版本另有专门断言（Loaded.Revision），相等性只问内容。
				Equal: func(got, want domain.ShipmentRequest) bool {
					return reflect.DeepEqual(
						contractRequestSpec(t, got, 1),
						contractRequestSpec(t, want, 1),
					)
				},
			}
		})
}

// shipmentRequestContractShell 把 ports.ShipmentRequestRepository 装进框架合同的三个
// 接口（Loader/Inserter/VersionedUpdater）。
type shipmentRequestContractShell struct {
	t        *testing.T
	requests *pspostgres.ShipmentRequests
}

func (shell shipmentRequestContractShell) Load(
	ctx context.Context,
	key domain.SourceIdentity,
) (repository.Loaded[domain.ShipmentRequest], error) {
	request, found, err := shell.requests.FindBySourceIdentity(ctx, key)
	if err != nil {
		return repository.Loaded[domain.ShipmentRequest]{}, err
	}
	// 端口的否定结果是 found=false（统一不可见，不区分「不存在」与「别人的」）；
	// 合同要的哨兵 ErrNotFound 语义相同，只是拼写不同。
	if !found {
		return repository.Loaded[domain.ShipmentRequest]{}, repository.ErrNotFound
	}
	return repository.Loaded[domain.ShipmentRequest]{
		Aggregate: request,
		Revision:  repository.Revision(request.Revision()),
	}, nil
}

func (shell shipmentRequestContractShell) Insert(
	ctx context.Context,
	aggregate domain.ShipmentRequest,
) (repository.Revision, error) {
	outcome, err := shell.requests.Insert(ctx, requestSourceIdentity(aggregate), aggregate)
	if err != nil {
		return 0, err
	}
	switch outcome {
	case ports.ShipmentRequestInserted:
		// 建单只发生一次，适配器恒写 revision 1。
		return 1, nil
	case ports.ShipmentRequestAlreadyExists:
		return 0, repository.ErrAlreadyExists
	default:
		return 0, fmt.Errorf("建单结果 %d 不在封闭集合内", uint8(outcome))
	}
}

func (shell shipmentRequestContractShell) Update(
	ctx context.Context,
	key domain.SourceIdentity,
	aggregate domain.ShipmentRequest,
	expectedRevision repository.Revision,
) (repository.Revision, error) {
	// 端口不收独立的预期版本——那是聚合自己携带的（「聚合只记自己是从哪一版读出来的」，
	// ADR-0028/0031）。合同给的 expectedRevision 因此要盖回聚合：经重建门把同一份内容
	// 立在指定版本上，Save 的 WHERE 就按它比对。
	restamped, err := domain.RehydrateShipmentRequest(
		contractRequestSpec(shell.t, aggregate, int64(expectedRevision)))
	if err != nil {
		return 0, fmt.Errorf("按预期版本重立聚合：%w", err)
	}
	outcome, err := shell.requests.Save(ctx, key, restamped)
	if err != nil {
		return 0, err
	}
	switch outcome {
	case ports.ShipmentRequestSaved:
		// 一次保存推进一格：SET revision = 预期 + 1。
		return expectedRevision + 1, nil
	case ports.ShipmentRequestRevisionConflict:
		return 0, repository.ErrConflict
	default:
		return 0, fmt.Errorf("保存结果 %d 不在封闭集合内", uint8(outcome))
	}
}
