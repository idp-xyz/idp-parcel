package domain

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

var (
	ErrInvalidAuthorizedDeliveryScope = errors.New("party commercial: invalid authorized delivery scope")
	ErrInvalidAuthorizedDeliveryQuery = errors.New("party commercial: invalid authorized delivery query")
)

// LogisticsNodeReference 指名一个物流节点。节点的网络身份属 network-routing，本上下文只拿它收窄授权交付关系的适用范围，
// 不核它在不在网络目录里。
type LogisticsNodeReference struct{ requiredValue }

func NewLogisticsNodeReference(value string) (LogisticsNodeReference, error) {
	required, err := newRequiredValue("logistics node reference", value)
	return LogisticsNodeReference{required}, err
}

// 授权交付关系的适用范围写进 CommercialScopeReference 时用的两个键。只在这里出现：拼与拆都经本文件，别处不拼串。
const (
	authorizedDeliveryScopeAccountKey = "customerAccount"
	authorizedDeliveryScopeNodeKey    = "node"
)

// AuthorizedDeliveryScope 是授权交付关系的适用范围：收到一个客户账户，可再收到一个节点（PC CONTEXT「参与方关系」）。
//
// 它写进 CommercialScopeReference，而不是给关系册另开两列：参与方关系「必须保存双方、角色、方向、适用范围、依据和
// 有效区间」，范围是其中一格，各角色共用这一格，别的角色的范围照旧是不透明引用。写法是 URL 查询串
// （`customerAccount=…`，可再接 `&node=…`），键按字典序、值按查询串转义，于是一个范围只有一种拼法——内容摘要与
// 重放判据比的是串，两种拼法会把同一段关系答成内容冲突。标识里自带 `&`、`=` 之类也拆得回来，不必另立禁用字符。
type AuthorizedDeliveryScope struct {
	account CustomerAccountID
	node    LogisticsNodeReference
}

// NewAuthorizedDeliveryScope 的 node 给 nil 即不收窄到节点。
func NewAuthorizedDeliveryScope(account CustomerAccountID, node *LogisticsNodeReference) (AuthorizedDeliveryScope, error) {
	if !account.valid() {
		return AuthorizedDeliveryScope{}, fmt.Errorf("%w: customer account is required", ErrInvalidAuthorizedDeliveryScope)
	}
	scope := AuthorizedDeliveryScope{account: account}
	if node != nil {
		if !node.valid() {
			return AuthorizedDeliveryScope{}, fmt.Errorf("%w: node is blank", ErrInvalidAuthorizedDeliveryScope)
		}
		scope.node = *node
	}
	return scope, nil
}

// ParseAuthorizedDeliveryScope 只认 Reference 拼出的那一种串：换序、重复键、多余键、空值或多余转义都拒。宽进会让同一个
// 范围有两种在册拼法，见 AuthorizedDeliveryScope 的头注。
func ParseAuthorizedDeliveryScope(reference CommercialScopeReference) (AuthorizedDeliveryScope, error) {
	raw := reference.String()
	values, err := url.ParseQuery(raw)
	if err != nil {
		return AuthorizedDeliveryScope{}, fmt.Errorf("%w: %v", ErrInvalidAuthorizedDeliveryScope, err)
	}
	for key, list := range values {
		if key != authorizedDeliveryScopeAccountKey && key != authorizedDeliveryScopeNodeKey {
			return AuthorizedDeliveryScope{}, fmt.Errorf("%w: unknown key %q", ErrInvalidAuthorizedDeliveryScope, key)
		}
		if len(list) != 1 {
			return AuthorizedDeliveryScope{}, fmt.Errorf("%w: key %q given %d times", ErrInvalidAuthorizedDeliveryScope, key, len(list))
		}
	}
	accounts, ok := values[authorizedDeliveryScopeAccountKey]
	if !ok {
		return AuthorizedDeliveryScope{}, fmt.Errorf("%w: customer account is required", ErrInvalidAuthorizedDeliveryScope)
	}
	account, err := NewCustomerAccountID(accounts[0])
	if err != nil {
		return AuthorizedDeliveryScope{}, fmt.Errorf("%w: %v", ErrInvalidAuthorizedDeliveryScope, err)
	}
	var node *LogisticsNodeReference
	if nodes, ok := values[authorizedDeliveryScopeNodeKey]; ok {
		ref, err := NewLogisticsNodeReference(nodes[0])
		if err != nil {
			return AuthorizedDeliveryScope{}, fmt.Errorf("%w: %v", ErrInvalidAuthorizedDeliveryScope, err)
		}
		node = &ref
	}
	scope, err := NewAuthorizedDeliveryScope(account, node)
	if err != nil {
		return AuthorizedDeliveryScope{}, err
	}
	if canonical := scope.Reference().String(); canonical != raw {
		return AuthorizedDeliveryScope{}, fmt.Errorf("%w: %q is not the canonical spelling %q", ErrInvalidAuthorizedDeliveryScope, raw, canonical)
	}
	return scope, nil
}

// Reference 拼出本范围在关系册上的那一种串。
func (scope AuthorizedDeliveryScope) Reference() CommercialScopeReference {
	values := url.Values{authorizedDeliveryScopeAccountKey: {scope.account.String()}}
	if scope.node.valid() {
		values.Set(authorizedDeliveryScopeNodeKey, scope.node.String())
	}
	return CommercialScopeReference{requiredValue{value: values.Encode()}}
}

func (scope AuthorizedDeliveryScope) CustomerAccount() CustomerAccountID {
	return scope.account
}

// Node 交回范围收到的节点；不收窄到节点时答 false。
func (scope AuthorizedDeliveryScope) Node() (LogisticsNodeReference, bool) {
	return scope.node, scope.node.valid()
}

// Covers 回答这次送站（客户账户，开立节点）落不落在范围里。没收窄到节点的范围覆盖任何节点。
func (scope AuthorizedDeliveryScope) Covers(account CustomerAccountID, node LogisticsNodeReference) bool {
	if scope.account != account {
		return false
	}
	return !scope.node.valid() || scope.node == node
}

// AuthorizedDeliveryQuery 是授权交付判断的问法：（租户，交付方参与方，客户账户，开立节点，开立时刻），见 CONTEXT-MAP
// `party-commercial → node-operations`。开立节点是为收到节点的范围而问的；不收窄到节点的关系不看它。
type AuthorizedDeliveryQuery struct {
	Tenant          TenantID
	DeliveringParty PartyID
	CustomerAccount CustomerAccountID
	Node            LogisticsNodeReference
	At              time.Time
}

// Validate 拒缺格的问法。缺格不是「否」：问法坏了就说不出这次送站是谁替谁、在哪、何时，答哪一格都是替调用方编。
func (query AuthorizedDeliveryQuery) Validate() error {
	switch {
	case !query.Tenant.valid():
		return fmt.Errorf("%w: tenant is required", ErrInvalidAuthorizedDeliveryQuery)
	case !query.DeliveringParty.valid():
		return fmt.Errorf("%w: delivering party is required", ErrInvalidAuthorizedDeliveryQuery)
	case !query.CustomerAccount.valid():
		return fmt.Errorf("%w: customer account is required", ErrInvalidAuthorizedDeliveryQuery)
	case !query.Node.valid():
		return fmt.Errorf("%w: node is required", ErrInvalidAuthorizedDeliveryQuery)
	case query.At.IsZero():
		return fmt.Errorf("%w: instant is required", ErrInvalidAuthorizedDeliveryQuery)
	}
	return nil
}

// AuthorizedDeliveryAnswer 是授权交付判断的三种答复。读不动不是第四格，交回错误。
type AuthorizedDeliveryAnswer uint8

const (
	AuthorizedDeliveryAnswerInvalid AuthorizedDeliveryAnswer = iota
	DeliveryAuthorized
	DeliveryNotAuthorized
	DeliveryAuthorizationNotConfigured
)

func (answer AuthorizedDeliveryAnswer) String() string {
	switch answer {
	case DeliveryAuthorized:
		return "AUTHORIZED"
	case DeliveryNotAuthorized:
		return "NOT_AUTHORIZED"
	case DeliveryAuthorizationNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// AuthorizedDeliveryEvidence 是判断要读的册面，由读口在同一个快照里取齐。
//
//   - Account：问法里那个客户账户的最新登记修订。查无此账户时留零值；零值、或租户与账户对不上问法的，都按查无算。
//   - Held：交付方作为持有方的授权交付方关系，每段只取最新修订——撤销、到期与替代都是同一段关系的下一修订，取旧修订
//     会让已撤销的关系还答是。
//   - TenantConfigured：本租户登记过任何一条授权交付方关系，不论状态。ADR-0212「越权风险点」一节写明：「未配置」
//     按本租户一条都没登记来判，不按单个客户账户判。
type AuthorizedDeliveryEvidence struct {
	Account          CustomerAccount
	Held             []PartyRelationshipRegistration
	TenantConfigured bool
}

// JudgeAuthorizedDelivery 答授权交付判断。
//
//   - 是：交付方就是该客户账户所属参与方本身；或判断时刻有一条生效的授权交付方关系，持有方是交付方、相对方是该账户
//     所属参与方，适用范围覆盖这个客户账户（收到节点的，也覆盖开立节点）。
//   - 未配置：本租户一条授权交付方关系都没登记过，交付方又不是该客户账户所属参与方本身。
//   - 否：其余情形，含客户账户在本租户查无。
//
// 相对方要对上账户此刻所属的参与方：授权是账户所属参与方给的，账户日后改属别的参与方，旧的授权不跟过去。
func JudgeAuthorizedDelivery(query AuthorizedDeliveryQuery, evidence AuthorizedDeliveryEvidence) (AuthorizedDeliveryAnswer, error) {
	if err := query.Validate(); err != nil {
		return AuthorizedDeliveryAnswerInvalid, err
	}
	owner, known := evidence.accountOwner(query)
	if known && owner == query.DeliveringParty {
		return DeliveryAuthorized, nil
	}
	if !evidence.TenantConfigured {
		return DeliveryAuthorizationNotConfigured, nil
	}
	if !known {
		return DeliveryNotAuthorized, nil
	}
	for _, registration := range evidence.Held {
		if registration.Tenant() != query.Tenant {
			continue
		}
		relationship := registration.Relationship()
		if relationship.Role() != AuthorizedDeliveringPartyRole ||
			relationship.Holder() != query.DeliveringParty ||
			relationship.Counterparty() != owner ||
			!relationship.AppliesAt(query.At) {
			continue
		}
		if scope, ok := relationship.AuthorizedDeliveryScope(); ok && scope.Covers(query.CustomerAccount, query.Node) {
			return DeliveryAuthorized, nil
		}
	}
	return DeliveryNotAuthorized, nil
}

func (evidence AuthorizedDeliveryEvidence) accountOwner(query AuthorizedDeliveryQuery) (PartyID, bool) {
	account := evidence.Account
	if account.Tenant() != query.Tenant || account.ID() != query.CustomerAccount || !account.CustomerParty().valid() {
		return PartyID{}, false
	}
	return account.CustomerParty(), true
}
