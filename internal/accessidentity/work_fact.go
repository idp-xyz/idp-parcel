package accessidentity

import (
	"errors"
	"strings"
	"time"
)

// ErrWorkScopeRequired 表示「作业事实登记」这一格没带作业范围：它的授予按租户 × 作业范围登记（ADR-0149 决定二），
// 不带范围的一笔等于把这个租户所有节点与场外作业范围一并授出。经 NewWorkFactRegistrationGrant 建。
var ErrWorkScopeRequired = errors.New("access identity: work fact registration grant needs a work scope")

// ErrWorkScopeKindUnknown 表示给出的作业范围种类不在 ADR-0149 决定二列出的那两种里。
var ErrWorkScopeKindUnknown = errors.New("access identity: work scope kind is unknown")

// ErrInvalidWorkScope 表示作业范围缺引用。
var ErrInvalidWorkScope = errors.New("access identity: work scope needs a reference")

// WorkScopeKind 是作业范围的种类：节点，或场外作业范围（ADR-0149 决定二）。
//
// 种类是册的列，由产品定；某租户有哪些节点、哪些场外作业范围，是租户取值。
type WorkScopeKind string

const (
	WorkScopeNode    WorkScopeKind = "NODE"
	WorkScopeOffsite WorkScopeKind = "OFFSITE"
)

// ParseWorkScopeKind 按字面取值认作业范围种类，大小写不折叠：册里存的就是这些字面量。
func ParseWorkScopeKind(value string) (WorkScopeKind, error) {
	switch kind := WorkScopeKind(value); kind {
	case WorkScopeNode, WorkScopeOffsite:
		return kind, nil
	}
	return "", ErrWorkScopeKindUnknown
}

func (kind WorkScopeKind) String() string { return string(kind) }

// WorkScope 是一个作业范围：种类加该租户给它的引用（节点或场外作业范围的编码）。引用只比相等，本包不解释它——
// 节点与场外作业范围之间没有本包认得的上下级，按包含关系推就得替消费方的网络模型下判断。
type WorkScope struct {
	kind      WorkScopeKind
	reference string
}

func NewWorkScope(kind WorkScopeKind, reference string) (WorkScope, error) {
	if _, err := ParseWorkScopeKind(string(kind)); err != nil {
		return WorkScope{}, err
	}
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return WorkScope{}, ErrInvalidWorkScope
	}
	return WorkScope{kind: kind, reference: trimmed}, nil
}

func (scope WorkScope) Kind() WorkScopeKind { return scope.kind }
func (scope WorkScope) Reference() string   { return scope.reference }

// valid 只认构造出来的范围：零值与包外拼出来的种类都不算。
func (scope WorkScope) valid() bool {
	_, err := ParseWorkScopeKind(string(scope.kind))
	return err == nil && scope.reference != ""
}

// NewWorkFactRegistrationGrant 登一笔「作业事实登记」授予：能力面固定为 CapabilityWorkFactRegistration，作业范围必填。
// 其余各件的纪律同 NewOperatorGrant。
func NewWorkFactRegistrationGrant(
	tenantID string,
	grantID string,
	subject OperatorSubject,
	scope WorkScope,
	interval EffectiveInterval,
	basis string,
) (OperatorGrant, error) {
	if !scope.valid() {
		return OperatorGrant{}, ErrWorkScopeRequired
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return OperatorGrant{}, ErrIncompleteOperatorRegistration
	}
	return OperatorGrant{
		tenantID:  tenant,
		grantID:   id,
		subject:   subject,
		face:      CapabilityWorkFactRegistration,
		workScope: scope,
		interval:  interval,
		basis:     reference,
	}, nil
}

// WorkScope 交回「作业事实登记」授予的作业范围；别的能力面上为零值。
func (grant OperatorGrant) WorkScope() WorkScope { return grant.workScope }

// HoldsWorkFactAt 答这个操作者在 at 那一刻是否持有 scope 这一作业范围上的「作业事实登记」生效授予。范围按种类与引用
// 逐字相等。
func (standing OperatorStanding) HoldsWorkFactAt(scope WorkScope, at time.Time) bool {
	if !scope.valid() {
		return false
	}
	for _, recorded := range standing.grants {
		if recorded.grant.face == CapabilityWorkFactRegistration && recorded.grant.workScope == scope && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}
