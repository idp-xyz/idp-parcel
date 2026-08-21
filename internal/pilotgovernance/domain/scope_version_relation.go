package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidScopeRelation = errors.New("pilot governance: invalid scope version relation")

// ScopeVersionRelationKind 是覆盖关系边的封闭二值。两种都要能表达：只能登「承继」，
// 「确认不相干」就永远读不出来，保守第三态照样恒成立。
type ScopeVersionRelationKind uint8

const (
	ScopeVersionRelationKindInvalid ScopeVersionRelationKind = iota
	// ScopeInheritsSuspensions：后继版本承继前代版本的暂停——前代那条已生效未恢复的
	// 暂停对后继同样拦新准入。有向：反向不成立，除非另登一条反向边。
	ScopeInheritsSuspensions
	// ScopeUnrelated：两版互不相干——前代的暂停不及于后继。对称事实：任一方向登记
	// 一次即成立，与任一方向的承继相悖。
	ScopeUnrelated
)

func (kind ScopeVersionRelationKind) valid() bool {
	return kind == ScopeInheritsSuspensions || kind == ScopeUnrelated
}

func (kind ScopeVersionRelationKind) String() string {
	switch kind {
	case ScopeInheritsSuspensions:
		return "INHERITS_SUSPENSIONS"
	case ScopeUnrelated:
		return "UNRELATED"
	default:
		return ""
	}
}

// ScopeCoverageDeclaration 是评审命令里的一格：对前代版本声明边的种类。后继由所在
// 评审的范围版本担任——关系随范围版本升版那次 Go/No-Go 决定一并登记，不设独立登记路。
type ScopeCoverageDeclaration struct {
	Predecessor ScopeVersionReference
	Kind        ScopeVersionRelationKind
}

// ScopeVersionRelationSpec 是一条覆盖关系边所需的全部输入。
type ScopeVersionRelationSpec struct {
	Successor   ScopeVersionReference
	Predecessor ScopeVersionReference
	Kind        ScopeVersionRelationKind
	// Objective 与 Candidates 合成所属决定引用：登记它的那次 Go/No-Go。无决定就无
	// 关系登记——届时恒走保守暂停照旧，那是诚实答案不是缺陷。
	Objective    string
	Candidates   CandidateVersionSetID
	RegisteredAt time.Time
}

// ScopeVersionRelation 是登记册上的一条覆盖关系边。ScopeVersionReference 是脱敏引用
// 组合，从不透明串上推不出谱系——前缀、子串、版本号解析与时间序推断一律禁止；关系
// 只能被登记，不能被算出来，本类型是那份登记的形状。
type ScopeVersionRelation struct {
	successor    ScopeVersionReference
	predecessor  ScopeVersionReference
	kind         ScopeVersionRelationKind
	objective    string
	candidates   CandidateVersionSetID
	registeredAt time.Time
}

func RegisterScopeVersionRelation(spec ScopeVersionRelationSpec) (ScopeVersionRelation, error) {
	if !spec.Successor.valid() ||
		!spec.Predecessor.valid() ||
		!spec.Kind.valid() ||
		strings.TrimSpace(spec.Objective) == "" ||
		!spec.Candidates.valid() ||
		spec.RegisteredAt.IsZero() {
		return ScopeVersionRelation{}, ErrInvalidScopeRelation
	}
	// 自指边无意义：自己承继自己是废话，与自己互不相干是谎话，两种都拒。
	if spec.Successor == spec.Predecessor {
		return ScopeVersionRelation{}, ErrInvalidScopeRelation
	}
	return ScopeVersionRelation{
		successor:    spec.Successor,
		predecessor:  spec.Predecessor,
		kind:         spec.Kind,
		objective:    spec.Objective,
		candidates:   spec.Candidates,
		registeredAt: spec.RegisteredAt.UTC(),
	}, nil
}

func (relation ScopeVersionRelation) Successor() ScopeVersionReference {
	return relation.successor
}

func (relation ScopeVersionRelation) Predecessor() ScopeVersionReference {
	return relation.predecessor
}

func (relation ScopeVersionRelation) Kind() ScopeVersionRelationKind {
	return relation.kind
}

func (relation ScopeVersionRelation) Objective() string {
	return relation.objective
}

func (relation ScopeVersionRelation) Candidates() CandidateVersionSetID {
	return relation.candidates
}

func (relation ScopeVersionRelation) RegisteredAt() time.Time {
	return relation.registeredAt
}

// RelationComparison 是「同一对版本上既有边与新声明边」的比较结果封闭集合。
type RelationComparison uint8

const (
	RelationComparisonInvalid RelationComparison = iota
	// RelationRedundant：同一事实已在册——同有序对同种类，或对称的互不相干已从另一
	// 方向登过。重复声明不是错误，跳过即可。
	RelationRedundant
	// RelationsContradictory：相悖——同有序对不同种类，或互不相干（无论哪个方向）撞上
	// 承继。相悖声明必须在决定落库前被拦下：后到的决定改写不了先到的登记（治理记录
	// 不可覆盖），静默收下等于让关系登记变成绕过恢复决定的旁路。
	RelationsContradictory
	// RelationsIndependent：两条各自成立——只剩双向承继一种组合（两版互相承继对方的
	// 暂停是两条独立的有向事实）。
	RelationsIndependent
)

// CompareScopeVersionRelations 判既有边与声明边的相容性。两条边必须落在同一无序对上，
// 否则答 Invalid——不同对之间没有可比性。
func CompareScopeVersionRelations(existing, declared ScopeVersionRelation) RelationComparison {
	samePair := existing.successor == declared.successor &&
		existing.predecessor == declared.predecessor
	reversePair := existing.successor == declared.predecessor &&
		existing.predecessor == declared.successor
	switch {
	case samePair:
		if existing.kind == declared.kind {
			return RelationRedundant
		}
		return RelationsContradictory
	case reversePair:
		if existing.kind == ScopeUnrelated && declared.kind == ScopeUnrelated {
			return RelationRedundant
		}
		if existing.kind == ScopeUnrelated || declared.kind == ScopeUnrelated {
			return RelationsContradictory
		}
		return RelationsIndependent
	default:
		return RelationComparisonInvalid
	}
}

// ScopeVersionRelationConflict 指名一对相悖的边：声明的与在册的。
type ScopeVersionRelationConflict struct {
	Declared ScopeVersionRelation
	Existing ScopeVersionRelation
}
