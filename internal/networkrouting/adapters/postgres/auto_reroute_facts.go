package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// AutoRerouteFactsCatalog 是自动改路四条件事实目录的存取口（审计票 05，W10）。
// 它拥有按判断键版本化的事实陈述，不做任何条件评估——评估在领域
// （EvaluateAutoRerouteConditions），这里只翻译。
type AutoRerouteFactsCatalog struct {
	db *bentopg.DB
}

func NewAutoRerouteFactsCatalog(db *bentopg.DB) (*AutoRerouteFactsCatalog, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &AutoRerouteFactsCatalog{db: db}, nil
}

// 读写两个端口都编译期钉住（先例：NetworkCatalog 对登记端口的钉法）。
var (
	_ ports.AutoRerouteFactsView     = (*AutoRerouteFactsCatalog)(nil)
	_ ports.AutoRerouteFactsRegistry = (*AutoRerouteFactsCatalog)(nil)
)

// LoadAutoRerouteFacts 按判断键取当前陈述（最大版本，历史链先例：availability_
// adjustment）。三格：行在场即事实；零行即**这个判断键从未登记过事实**（未配置，
// 第二格）——复核编排据此整段不做改路评估；error 只表示依赖故障或行数据坏了。
func (catalog *AutoRerouteFactsCatalog) LoadAutoRerouteFacts(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
) (domain.AutoRerouteFacts, bool, error) {
	none := domain.AutoRerouteFacts{}
	// 身份不成立时不读任何权威（判断键同一条纪律）。
	if !key.MinimumIdentityEstablished() {
		return none, false, fmt.Errorf("load auto reroute facts: judgment key is incomplete")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load auto reroute facts: %w", err)
	}

	var policyAllows, atControlledNode, onlyUnexecuted bool
	var restrictionsRaw, responsibilitiesRaw []byte
	err = querier.QueryRow(ctx, `
SELECT policy_allows_automatic, at_controlled_node, only_unexecuted_affected,
       unresolved_restrictions, outstanding_responsibilities
  FROM network_routing.auto_reroute_facts
 WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3
   AND acceptance_baseline = $4 AND declared_parcel_id = $5 AND service_purpose = $6
 ORDER BY version DESC
 LIMIT 1`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
	).Scan(&policyAllows, &atControlledNode, &onlyUnexecuted, &restrictionsRaw, &responsibilitiesRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load auto reroute facts: %w", err)
	}

	restrictions, err := decodeRestrictionList(restrictionsRaw)
	if err != nil {
		return none, false, fmt.Errorf("load auto reroute facts: unresolved restrictions: %w", err)
	}
	responsibilities, err := decodeResponsibilityList(responsibilitiesRaw)
	if err != nil {
		return none, false, fmt.Errorf("load auto reroute facts: outstanding responsibilities: %w", err)
	}
	return domain.AutoRerouteFacts{
		PolicyAllowsAutomatic:       policyAllows,
		AtControlledNode:            atControlledNode,
		OnlyUnexecutedAffected:      onlyUnexecuted,
		UnresolvedRestrictions:      restrictions,
		OutstandingResponsibilities: responsibilities,
	}, true, nil
}

// RegisterAutoRerouteFacts 登记一版事实陈述。`已登记`用 ON CONFLICT DO NOTHING 加
// 零行判定翻译，不捕 23505——撞键的 INSERT 会把整个事务打进中止态，而登记编排拿到
// `已登记`还要在同一事务里读回既有版本比对幂等与冲突（先例：ReachabilityJudgments）。
//
// 行完整性在这里也守一遍（键完整、版本为正、依据与时刻非空）：这些格坏行一旦进库，
// 读口只能以「数据坏了」报障，挡在写入侧坏处最小。内容级受理门在登记用例。
func (catalog *AutoRerouteFactsCatalog) RegisterAutoRerouteFacts(
	ctx context.Context,
	record ports.AutoRerouteFactsRecord,
) (ports.AutoRerouteFactsSaveOutcome, error) {
	invalid := ports.AutoRerouteFactsSaveOutcomeInvalid
	if !record.Key.MinimumIdentityEstablished() {
		return invalid, fmt.Errorf("register auto reroute facts: judgment key is incomplete")
	}
	if record.Version < 1 {
		return invalid, fmt.Errorf("register auto reroute facts: version must be positive")
	}
	if record.StrategyBasis == "" {
		return invalid, fmt.Errorf("register auto reroute facts: strategy basis is required")
	}
	if record.RegisteredAt.IsZero() {
		return invalid, fmt.Errorf("register auto reroute facts: registered time is required")
	}
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return invalid, fmt.Errorf("register auto reroute facts: %w", err)
	}

	restrictions, err := encodeReferenceList(referenceStrings(record.Facts.UnresolvedRestrictions))
	if err != nil {
		return invalid, fmt.Errorf("register auto reroute facts: unresolved restrictions: %w", err)
	}
	responsibilities, err := encodeReferenceList(responsibilityStrings(record.Facts.OutstandingResponsibilities))
	if err != nil {
		return invalid, fmt.Errorf("register auto reroute facts: outstanding responsibilities: %w", err)
	}

	key := record.Key
	tag, err := executor.Exec(ctx, `
INSERT INTO network_routing.auto_reroute_facts
    (tenant_id, customer_account_id, shipment_request_id, acceptance_baseline,
     declared_parcel_id, service_purpose, version,
     policy_allows_automatic, at_controlled_node, only_unexecuted_affected,
     unresolved_restrictions, outstanding_responsibilities, strategy_basis, registered_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT ON CONSTRAINT auto_reroute_facts_pkey DO NOTHING`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
		record.Version,
		record.Facts.PolicyAllowsAutomatic, record.Facts.AtControlledNode,
		record.Facts.OnlyUnexecutedAffected,
		restrictions, responsibilities, record.StrategyBasis, record.RegisteredAt.UTC(),
	)
	if err != nil {
		return invalid, fmt.Errorf("register auto reroute facts: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AutoRerouteFactsAlreadyRegistered, nil
	}
	return ports.AutoRerouteFactsRegistered, nil
}

// FindAutoRerouteFacts 按（键+版本）精确取一版，供登记编排比对。键回填用入参而不从行
// 重建——行里的六维就是入参写进去的，重建一遍只是多一次可失败的翻译。
func (catalog *AutoRerouteFactsCatalog) FindAutoRerouteFacts(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	version int,
) (ports.AutoRerouteFactsRecord, bool, error) {
	none := ports.AutoRerouteFactsRecord{}
	if !key.MinimumIdentityEstablished() {
		return none, false, fmt.Errorf("find auto reroute facts: judgment key is incomplete")
	}
	if version < 1 {
		return none, false, fmt.Errorf("find auto reroute facts: version must be positive")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find auto reroute facts: %w", err)
	}

	record := ports.AutoRerouteFactsRecord{Key: key, Version: version}
	var restrictionsRaw, responsibilitiesRaw []byte
	err = querier.QueryRow(ctx, `
SELECT policy_allows_automatic, at_controlled_node, only_unexecuted_affected,
       unresolved_restrictions, outstanding_responsibilities, strategy_basis, registered_at
  FROM network_routing.auto_reroute_facts
 WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3
   AND acceptance_baseline = $4 AND declared_parcel_id = $5 AND service_purpose = $6
   AND version = $7`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
		version,
	).Scan(
		&record.Facts.PolicyAllowsAutomatic, &record.Facts.AtControlledNode,
		&record.Facts.OnlyUnexecutedAffected,
		&restrictionsRaw, &responsibilitiesRaw, &record.StrategyBasis, &record.RegisteredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find auto reroute facts: %w", err)
	}

	record.Facts.UnresolvedRestrictions, err = decodeRestrictionList(restrictionsRaw)
	if err != nil {
		return none, false, fmt.Errorf("find auto reroute facts: unresolved restrictions: %w", err)
	}
	record.Facts.OutstandingResponsibilities, err = decodeResponsibilityList(responsibilitiesRaw)
	if err != nil {
		return none, false, fmt.Errorf("find auto reroute facts: outstanding responsibilities: %w", err)
	}
	return record, true, nil
}

// encodeReferenceList 编码引用清单。空白元素在这里也挡一遍：写进去的空白读回时会在
// 领域构造器上炸成「数据坏了」，挡在写入侧坏处最小。
func encodeReferenceList(values []string) ([]byte, error) {
	for _, value := range values {
		if value == "" {
			return nil, fmt.Errorf("reference must not be blank")
		}
	}
	if values == nil {
		values = []string{}
	}
	return json.Marshal(values)
}

func referenceStrings(references []domain.RestrictionReference) []string {
	values := make([]string, 0, len(references))
	for _, reference := range references {
		values = append(values, reference.String())
	}
	return values
}

func responsibilityStrings(references []domain.ResponsibilityReference) []string {
	values := make([]string, 0, len(references))
	for _, reference := range references {
		values = append(values, reference.String())
	}
	return values
}

func decodeRestrictionList(raw []byte) ([]domain.RestrictionReference, error) {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	references := make([]domain.RestrictionReference, 0, len(values))
	for _, value := range values {
		reference, err := domain.NewRestrictionReference(value)
		if err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	return references, nil
}

func decodeResponsibilityList(raw []byte) ([]domain.ResponsibilityReference, error) {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	references := make([]domain.ResponsibilityReference, 0, len(values))
	for _, value := range values {
		reference, err := domain.NewResponsibilityReference(value)
		if err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	return references, nil
}
