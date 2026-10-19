package accessidentity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrIncompleteAdmissionScopeMapping 表示准入范围对照的一行缺件，建不成。
var ErrIncompleteAdmissionScopeMapping = errors.New("access identity: admission scope mapping is incomplete")

// ErrAdmissionRequirementUnknown 表示对照登的能力与事实类型不是任何一口会提的准入要求。词表归产品策略、随认证方定
// （ADR-0180 决定四）；拼错的一行登进去，只会让那个租户撞上与「没登」同一张脸的不在准入范围，所以在登记时拒。
var ErrAdmissionRequirementUnknown = errors.New("access identity: admission requirement is not one any face asks for")

// ErrAdmissionScopeMappingEndDoesNotMatch 表示一笔结束被挂到了它没结束的那一行对照上。
var ErrAdmissionScopeMappingEndDoesNotMatch = errors.New("access identity: mapping end belongs to another mapping")

// ErrConflictingAdmissionScopeMappings 表示同一个键在同一时刻有两行对照都生效。登记口在写入侧拒重叠（ADR-0180
// 决定二），读到它说明有绕过登记口的直插；不任选一行——选哪一行都可能把这笔写放进它不在的试点范围。
var ErrConflictingAdmissionScopeMappings = errors.New("access identity: two admission scope mappings are effective for the same key")

// checkAdmissionRequirement 认的是各族认证方今天会提的准入要求：运营决定口取能力面 CapabilityOperationDecision 与
// 决定种类，外部结果与资金事实口取 ExternalFactAdmissionCapability 与外部事实类型（ADR-0180 决定四）。别的能力面
// 要判准入时，它的词随那一面的票加进来。
//
// 首方客户族在铸造侧判准入的口取 CustomerCommandAdmissionCapability 与那一口的客户面能力词；不在铸造侧判的口（委托提交）
// 的词登不进来。
func checkAdmissionRequirement(requirement AdmissionRequirement) error {
	switch requirement.Capability {
	case CapabilityOperationDecision.String():
		if _, err := ParseDecisionKind(requirement.FactKind); err == nil {
			return nil
		}
	case ExternalFactAdmissionCapability:
		if _, err := ParseExternalFactType(requirement.FactKind); err == nil {
			return nil
		}
	case CustomerCommandAdmissionCapability:
		if capability, err := ParseCustomerCapability(requirement.FactKind); err == nil && customerCommandAdmission(capability) != nil {
			return nil
		}
	}
	return ErrAdmissionRequirementUnknown
}

// AdmissionScopeMapping 是准入范围对照的一行（ADR-0180 决定一、二）：某租户在（能力、事实类型）上的生产写，在生效
// 区间内落在治理登记册的哪个对象范围。对照只做坐标换算：「谁在写生产」的唯一权威仍是治理登记册的生产权威区间。
// 对象范围是那本册里的不透明引用，本册只存、不解释（ADR-0180 越权风险点 1）。
//
// 行带自己的登记标识，结束另登一行，理由同 OperatorGrant：被结束的那一行是那段时间里它确实有效的证据。键是（租户、
// 能力、事实类型），任一时刻至多一行生效——Admits 看不到这笔写改的是哪个对象，一键对几个范围、任一覆盖即放行，会把
// 试点范围外对象上的写也放进来。
type AdmissionScopeMapping struct {
	tenantID    string
	mappingID   string
	requirement AdmissionRequirement
	objectScope string
	interval    EffectiveInterval
	basis       string
}

func NewAdmissionScopeMapping(
	tenantID string,
	mappingID string,
	requirement AdmissionRequirement,
	objectScope string,
	interval EffectiveInterval,
	basis string,
) (AdmissionScopeMapping, error) {
	if err := checkAdmissionRequirement(requirement); err != nil {
		return AdmissionScopeMapping{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(mappingID)
	scope := strings.TrimSpace(objectScope)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || scope == "" || interval.startsAt.IsZero() || reference == "" {
		return AdmissionScopeMapping{}, ErrIncompleteAdmissionScopeMapping
	}
	return AdmissionScopeMapping{
		tenantID:    tenant,
		mappingID:   id,
		requirement: requirement,
		objectScope: scope,
		interval:    interval,
		basis:       reference,
	}, nil
}

func (mapping AdmissionScopeMapping) TenantID() string                  { return mapping.tenantID }
func (mapping AdmissionScopeMapping) MappingID() string                 { return mapping.mappingID }
func (mapping AdmissionScopeMapping) Requirement() AdmissionRequirement { return mapping.requirement }
func (mapping AdmissionScopeMapping) ObjectScope() string               { return mapping.objectScope }
func (mapping AdmissionScopeMapping) Interval() EffectiveInterval       { return mapping.interval }
func (mapping AdmissionScopeMapping) Basis() string                     { return mapping.basis }

// AdmissionScopeMappingEnd 结束一行对照：自结束时刻起（含该时刻）那一行不再生效。时刻与依据缺一不成立，理由同
// GrantRevocation。试点范围换版本时，旧行在切换时刻结束、新行自同一时刻起登，两行首尾相接不重叠。
type AdmissionScopeMappingEnd struct {
	tenantID  string
	mappingID string
	endedAt   time.Time
	basis     string
}

func NewAdmissionScopeMappingEnd(tenantID, mappingID string, endedAt time.Time, basis string) (AdmissionScopeMappingEnd, error) {
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(mappingID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || endedAt.IsZero() || reference == "" {
		return AdmissionScopeMappingEnd{}, ErrIncompleteAdmissionScopeMapping
	}
	return AdmissionScopeMappingEnd{tenantID: tenant, mappingID: id, endedAt: endedAt, basis: reference}, nil
}

func (end AdmissionScopeMappingEnd) TenantID() string   { return end.tenantID }
func (end AdmissionScopeMappingEnd) MappingID() string  { return end.mappingID }
func (end AdmissionScopeMappingEnd) EndedAt() time.Time { return end.endedAt }
func (end AdmissionScopeMappingEnd) Basis() string      { return end.basis }

// RecordedAdmissionScopeMapping 是册上的一行对照，连同它的结束（若有）。
type RecordedAdmissionScopeMapping struct {
	mapping AdmissionScopeMapping
	end     AdmissionScopeMappingEnd
	ended   bool
}

// NewRecordedAdmissionScopeMapping 的 end 取 nil 表示未结束。
func NewRecordedAdmissionScopeMapping(mapping AdmissionScopeMapping, end *AdmissionScopeMappingEnd) (RecordedAdmissionScopeMapping, error) {
	if end == nil {
		return RecordedAdmissionScopeMapping{mapping: mapping}, nil
	}
	if end.tenantID != mapping.tenantID || end.mappingID != mapping.mappingID {
		return RecordedAdmissionScopeMapping{}, ErrAdmissionScopeMappingEndDoesNotMatch
	}
	return RecordedAdmissionScopeMapping{mapping: mapping, end: *end, ended: true}, nil
}

func (recorded RecordedAdmissionScopeMapping) Mapping() AdmissionScopeMapping {
	return recorded.mapping
}

func (recorded RecordedAdmissionScopeMapping) End() (AdmissionScopeMappingEnd, bool) {
	return recorded.end, recorded.ended
}

// EffectiveAt 答这一行在 at 那一刻生不生效：在区间内，且还没到结束时刻。
func (recorded RecordedAdmissionScopeMapping) EffectiveAt(at time.Time) bool {
	if !recorded.mapping.interval.Contains(at) {
		return false
	}
	return !recorded.ended || at.Before(recorded.end.endedAt)
}

// OverlapsWith 答一行待登的对照与册上这一行在生效期上有没有交集；不同键的两行永不重叠。册上这一行只算到它的结束
// 时刻，所以「旧行结束、新行自结束时刻起登」不算重叠（ADR-0180 决定二：同键的生效区间重叠在登记时拒）。
func (recorded RecordedAdmissionScopeMapping) OverlapsWith(candidate AdmissionScopeMapping) bool {
	if recorded.mapping.tenantID != candidate.tenantID || recorded.mapping.requirement != candidate.requirement {
		return false
	}
	start, end, bounded := recorded.mapping.interval.startsAt, recorded.mapping.interval.endsAt, !recorded.mapping.interval.endsAt.IsZero()
	if recorded.ended && (!bounded || recorded.end.endedAt.Before(end)) {
		end, bounded = recorded.end.endedAt, true
	}
	if bounded && !end.After(start) {
		return false
	}
	candidateEnd, candidateBounded := candidate.interval.EndsAt()
	startsBeforeCandidateEnds := !candidateBounded || start.Before(candidateEnd)
	candidateStartsBeforeEnd := !bounded || candidate.interval.startsAt.Before(end)
	return startsBeforeCandidateEnds && candidateStartsBeforeEnd
}

// ResolveAdmissionObjectScope 从同一个键的各行对照里取 at 那一刻生效的那个对象范围。一行都不生效答 found=false；
// 两行同时生效答 ErrConflictingAdmissionScopeMappings，不任选一行。
func ResolveAdmissionObjectScope(mappings []RecordedAdmissionScopeMapping, at time.Time) (string, bool, error) {
	scope, found := "", false
	for _, recorded := range mappings {
		if !recorded.EffectiveAt(at) {
			continue
		}
		if found {
			return "", false, ErrConflictingAdmissionScopeMappings
		}
		scope, found = recorded.mapping.objectScope, true
	}
	return scope, found, nil
}

// AdmissionScopeMappingReader 是对照册的读口：按（租户、能力、事实类型）取册上这个键的全部行，连同结束。键上一行都没有
// 答空切片，不返回 error；读不动才返回 error。此刻哪一行生效不在这里判。
type AdmissionScopeMappingReader interface {
	FindAdmissionScopeMappings(ctx context.Context, tenantID string, requirement AdmissionRequirement) ([]RecordedAdmissionScopeMapping, error)
}

// AdmissionScopeMappingOutcome 是对照册登记口的答复。撞键从不覆盖：同内容重放答原结果，异内容答冲突，册上已有的那一行
// 原样不动。它不复用另几本册的答复类型，理由同 IntegrationClientRegistrationOutcome。
type AdmissionScopeMappingOutcome string

const (
	AdmissionScopeMappingRecorded          AdmissionScopeMappingOutcome = "RECORDED"
	AdmissionScopeMappingAlreadyRegistered AdmissionScopeMappingOutcome = "ALREADY_REGISTERED"
	AdmissionScopeMappingContentConflict   AdmissionScopeMappingOutcome = "CONTENT_CONFLICT"
	// AdmissionScopeMappingOverlaps 只由 RegisterAdmissionScopeMapping 答：同一个键在这一行的生效区间里已有别的行生效。
	// 恢复动作是先结束那一行，或把这一行的起点挪到它结束之后。
	AdmissionScopeMappingOverlaps AdmissionScopeMappingOutcome = "MAPPING_OVERLAPS"
	// AdmissionScopeMappingNotRegistered 只由 RegisterAdmissionScopeMappingEnd 答：本租户册上没有这一行。
	AdmissionScopeMappingNotRegistered AdmissionScopeMappingOutcome = "MAPPING_NOT_REGISTERED"
)

func (outcome AdmissionScopeMappingOutcome) String() string { return string(outcome) }

// AdmissionScopeMappingRegistrar 是对照册的登记口，受控批量口消费它。写入要落在调用方开的框架事务里，理由同
// OperatorRegistrar：重叠判定与写入要在同一事务里关在一起。
type AdmissionScopeMappingRegistrar interface {
	RegisterAdmissionScopeMapping(ctx context.Context, mapping AdmissionScopeMapping) (AdmissionScopeMappingOutcome, error)
	RegisterAdmissionScopeMappingEnd(ctx context.Context, end AdmissionScopeMappingEnd) (AdmissionScopeMappingOutcome, error)
}
