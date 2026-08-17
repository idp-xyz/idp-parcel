package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrNetworkDefinitionUnresolvable 表示这个范围登记了网络定义，而本进程产不出它的事实族
// ——解析层（候选生成、过滤与排序，属 `PAR-NET-14`）不在。
//
// 它必须响亮上抛，不能退成`未配置`也不能退成一份空事实（ADR-0053 第四条第三格）：退成
// `未配置`会让运维去催租户登记一份**已经登记过**的东西；退成空事实则让领域评出`无当前
// 有效路由`，把「这个构建缺解析层」讲成「这个网络里什么都没有」。装配缺口要看得见。
var ErrNetworkDefinitionUnresolvable = errors.New(
	"network routing postgres: a network definition is registered but this build has no resolver for it")

// NetworkDefinitions 是网络定义登记册的读口，同时实现两个证据视图。
//
// 它只回答「这个范围有没有网络定义」这一件事（ADR-0053）。九族事实是解析层对每次判断的
// 推导结果，不在本表里；定义原语的模式也留待真有定义可登时再设计。今天本表没有写入方，
// 因此生产上恒答`未配置`——那是首发唯一走得到的真实分支，不是占位。
type NetworkDefinitions struct {
	db *bentopg.DB
}

func NewNetworkDefinitions(db *bentopg.DB) (*NetworkDefinitions, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &NetworkDefinitions{db: db}, nil
}

var _ ports.NetworkEvidenceView = (*NetworkDefinitions)(nil)
var _ ports.InitialRouteEvidenceView = (*NetworkDefinitions)(nil)

// LoadNetworkEvidence 为可达性判断取证据。三格见 ports.NetworkEvidenceView。
func (repository *NetworkDefinitions) LoadNetworkEvidence(
	ctx context.Context,
	key domain.ReachabilityJudgmentKey,
) (ports.NetworkEvidence, bool, error) {
	if _, registered, err := repository.revisionFor(
		ctx, key.TenantID, key.ServicePurpose,
	); err != nil || !registered {
		return ports.NetworkEvidence{}, false, err
	}
	return ports.NetworkEvidence{}, false, ErrNetworkDefinitionUnresolvable
}

// LoadInitialRouteEvidence 为初始路由与复核取证据。三格同上。
func (repository *NetworkDefinitions) LoadInitialRouteEvidence(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteEvidence, bool, error) {
	if _, registered, err := repository.revisionFor(
		ctx, key.TenantID, key.ServicePurpose,
	); err != nil || !registered {
		return ports.InitialRouteEvidence{}, false, err
	}
	return ports.InitialRouteEvidence{}, false, ErrNetworkDefinitionUnresolvable
}

// revisionFor 按（租户+服务目的）查登记。查无即未登记，不是错误——`未配置`是一个如实的
// 答案，端口为它专设了一格。
//
// 修订随定义同行取回而不由调用方传入：ADR-0052 要求同一次取回的全部事实族共用同一版，
// 而两次取回之间视图一变，事实与它标的修订就不再同源。
func (repository *NetworkDefinitions) revisionFor(
	ctx context.Context,
	tenant domain.TenantID,
	purpose domain.ServicePurpose,
) (domain.NetworkViewRevision, bool, error) {
	none := domain.NetworkViewRevision{}
	if tenant.String() == "" || purpose.String() == "" {
		return none, false, fmt.Errorf("load network definition: tenant and service purpose are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load network definition: %w", err)
	}

	var raw string
	err = querier.QueryRow(ctx,
		`SELECT view_revision
		   FROM network_routing.network_definition
		  WHERE tenant_id = $1 AND service_purpose = $2`,
		tenant.String(), purpose.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load network definition: %w", err)
	}
	revision, err := domain.NewNetworkViewRevision(raw)
	if err != nil {
		return none, false, fmt.Errorf("load network definition: %w", err)
	}
	return revision, true, nil
}
