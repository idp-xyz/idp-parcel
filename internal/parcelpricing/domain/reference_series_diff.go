package domain

import (
	"errors"
	"sort"
	"time"
)

// ErrSeriesPeriodDiffAcrossSeries 表示要比对的两版不属于同一条序列（租户、种类或序列标识
// 有一处不同）。那不是「差异很大」，是问错了对象：逐期差异只在同一条序列的两版之间有意义。
var ErrSeriesPeriodDiffAcrossSeries = errors.New("parcel pricing: series period diff needs two versions of the same series")

// SeriesPeriodChangeKind 是一期在两版之间的变化种类（封闭集）。
type SeriesPeriodChangeKind string

const (
	// SeriesPeriodUnchanged：同起点的期次止点、取值、凭证三样都没动。
	SeriesPeriodUnchanged SeriesPeriodChangeKind = "UNCHANGED"
	// SeriesPeriodChanged：同起点的期次至少动了一样，动了哪样看细项。
	SeriesPeriodChanged SeriesPeriodChangeKind = "CHANGED"
	// SeriesPeriodAdded：拟登版本有、基准版本没有的起点。
	SeriesPeriodAdded SeriesPeriodChangeKind = "ADDED"
	// SeriesPeriodRemoved：基准版本有、拟登版本没有的起点。
	SeriesPeriodRemoved SeriesPeriodChangeKind = "REMOVED"
)

func (kind SeriesPeriodChangeKind) String() string { return string(kind) }

// SeriesPeriodChange 是一期的比对结果。基准侧与拟登侧各自可缺席（新增无基准、移除无拟登），
// 三个细项布尔只在 CHANGED 上有意义——续办动作不同（抄错的数、补凭证、挪期界），合成一个
// 布尔会让看的人分不出改的是什么。
type SeriesPeriodChange struct {
	kind            SeriesPeriodChangeKind
	startsAt        time.Time
	base            *SeriesPeriodValue
	proposed        *SeriesPeriodValue
	valueChanged    bool
	endChanged      bool
	evidenceChanged bool
}

func (change SeriesPeriodChange) Kind() SeriesPeriodChangeKind { return change.kind }
func (change SeriesPeriodChange) StartsAt() time.Time          { return change.startsAt }

// Base 只在基准版本有这一起点时给出。
func (change SeriesPeriodChange) Base() (SeriesPeriodValue, bool) {
	if change.base == nil {
		return SeriesPeriodValue{}, false
	}
	return *change.base, true
}

// Proposed 只在拟登版本有这一起点时给出。
func (change SeriesPeriodChange) Proposed() (SeriesPeriodValue, bool) {
	if change.proposed == nil {
		return SeriesPeriodValue{}, false
	}
	return *change.proposed, true
}

func (change SeriesPeriodChange) ValueChanged() bool    { return change.valueChanged }
func (change SeriesPeriodChange) EndChanged() bool      { return change.endChanged }
func (change SeriesPeriodChange) EvidenceChanged() bool { return change.evidenceChanged }

// DiffSeriesPeriods 把同一条序列的两版期次表按起点对齐逐期比对。每一版都整版重述该序列
// 自起点以来的全部期次（CONTEXT），因此起点是两版之间唯一稳定的对齐键：同起点比止点、取值
// 与凭证，只在一侧出现的起点是新增或移除。结果按起点升序，一起点一条，两侧合并。
//
// 它不裁哪一版在用、不算摘要、不判哪一版对——那些各有自己的门；这里只把两张表并排说清楚，
// 供登记前预览呈现。
func DiffSeriesPeriods(base, proposed ReferenceSeriesRegistration) ([]SeriesPeriodChange, error) {
	if !base.valid() || !proposed.valid() {
		return nil, ErrInvalidReferenceSeriesRegistration
	}
	if base.tenant != proposed.tenant || base.kind != proposed.kind || base.reference.id != proposed.reference.id {
		return nil, ErrSeriesPeriodDiffAcrossSeries
	}

	// 键取时刻的纳秒数而不是 time.Time 本身：同一瞬间在不同 Location 下是两个不相等的
	// map 键，而重建自快照的期次不一定带着构造时的 UTC。
	byStart := make(map[int64]*SeriesPeriodChange, len(base.periods)+len(proposed.periods))
	for index := range base.periods {
		period := base.periods[index]
		byStart[period.startsAt.UnixNano()] = &SeriesPeriodChange{
			kind:     SeriesPeriodRemoved,
			startsAt: period.startsAt.UTC(),
			base:     &period,
		}
	}
	for index := range proposed.periods {
		period := proposed.periods[index]
		change, known := byStart[period.startsAt.UnixNano()]
		if !known {
			byStart[period.startsAt.UnixNano()] = &SeriesPeriodChange{
				kind:     SeriesPeriodAdded,
				startsAt: period.startsAt.UTC(),
				proposed: &period,
			}
			continue
		}
		change.proposed = &period
		change.valueChanged = !change.base.value.Equal(period.value)
		change.endChanged = !change.base.endsAt.Equal(period.endsAt)
		change.evidenceChanged = change.base.evidence != period.evidence
		change.kind = SeriesPeriodUnchanged
		if change.valueChanged || change.endChanged || change.evidenceChanged {
			change.kind = SeriesPeriodChanged
		}
	}

	changes := make([]SeriesPeriodChange, 0, len(byStart))
	for _, change := range byStart {
		changes = append(changes, *change)
	}
	sort.Slice(changes, func(left, right int) bool {
		return changes[left].startsAt.Before(changes[right].startsAt)
	})
	return changes, nil
}
