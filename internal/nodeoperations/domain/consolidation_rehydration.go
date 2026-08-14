package domain

import (
	"errors"
	"time"
)

// ErrInvalidRehydratedConsolidation 是集运单元从持久化行重建失败的信号。
// OpenConsolidationUnit 只开空开放实例，读回已封装/已关闭/带成员必须走这扇门：
// 三相与关闭时刻、封装快照互证，不把行数据直接当成合法聚合。
var ErrInvalidRehydratedConsolidation = errors.New("node operations: rehydrated consolidation unit violates its invariants")

// 三相在库面的封闭取值。适配器写行、重建门读行共用这一份，不在 SQL 与 Go 之间
// 各立一套标签。
const (
	ConsolidationPhaseOpen   = "OPEN"
	ConsolidationPhaseSealed = "SEALED"
	ConsolidationPhaseClosed = "CLOSED"
)

// RehydrateSealedSnapshotSpec 是一行封装快照在库里的样子。
type RehydrateSealedSnapshotSpec struct {
	Members  []HandlingUnitID
	Seal     SealReference
	Basis    WorkBasisReference
	SealedAt time.Time
}

// RehydrateConsolidationUnitSpec 是从行数据重建一个集运单元所需的全部字段。
// Phase 取 OPEN / SEALED / CLOSED；ClosedAt 只在已关闭时非零。
type RehydrateConsolidationUnitSpec struct {
	ID        ConsolidationUnitID
	Asset     CarrierAssetReference
	Phase     string
	Members   []HandlingUnitID
	Snapshots []RehydrateSealedSnapshotSpec
	ClosedAt  time.Time
}

// RehydrateConsolidationUnit 验三相与在场件：关闭必有时刻、封装必有至少一份快照
// 且当前成员与最后一份快照一致、开放/封装不得带关闭时刻。历史快照在开封后仍保留。
func RehydrateConsolidationUnit(spec RehydrateConsolidationUnitSpec) (*ConsolidationUnit, error) {
	if !spec.ID.valid() || !spec.Asset.valid() {
		return nil, ErrInvalidRehydratedConsolidation
	}
	phase, err := unitPhaseFrom(spec.Phase)
	if err != nil {
		return nil, err
	}

	members := make(map[HandlingUnitID]bool, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() || members[member] {
			return nil, ErrInvalidRehydratedConsolidation
		}
		members[member] = true
	}

	snapshots := make([]SealedSnapshot, 0, len(spec.Snapshots))
	for _, raw := range spec.Snapshots {
		snapshot, err := rehydrateSealedSnapshot(raw)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}

	switch phase {
	case unitSealed:
		if !spec.ClosedAt.IsZero() || len(snapshots) == 0 {
			return nil, ErrInvalidRehydratedConsolidation
		}
		if !sameMemberSet(members, snapshots[len(snapshots)-1].members) {
			return nil, ErrInvalidRehydratedConsolidation
		}
	case unitClosed:
		if spec.ClosedAt.IsZero() {
			return nil, ErrInvalidRehydratedConsolidation
		}
	case unitOpen:
		if !spec.ClosedAt.IsZero() {
			return nil, ErrInvalidRehydratedConsolidation
		}
	}

	closedAt := time.Time{}
	if phase == unitClosed {
		closedAt = spec.ClosedAt.UTC()
	}
	return &ConsolidationUnit{
		id:        spec.ID,
		asset:     spec.Asset,
		phase:     phase,
		members:   members,
		snapshots: snapshots,
		closedAt:  closedAt,
	}, nil
}

func rehydrateSealedSnapshot(spec RehydrateSealedSnapshotSpec) (SealedSnapshot, error) {
	if !spec.Seal.valid() || !spec.Basis.valid() || spec.SealedAt.IsZero() || len(spec.Members) == 0 {
		return SealedSnapshot{}, ErrInvalidRehydratedConsolidation
	}
	seen := make(map[HandlingUnitID]struct{}, len(spec.Members))
	members := make([]HandlingUnitID, 0, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() {
			return SealedSnapshot{}, ErrInvalidRehydratedConsolidation
		}
		if _, exists := seen[member]; exists {
			return SealedSnapshot{}, ErrInvalidRehydratedConsolidation
		}
		seen[member] = struct{}{}
		members = append(members, member)
	}
	return SealedSnapshot{
		members:  members,
		seal:     spec.Seal,
		basis:    spec.Basis,
		sealedAt: spec.SealedAt.UTC(),
	}, nil
}

func unitPhaseFrom(raw string) (unitPhase, error) {
	switch raw {
	case ConsolidationPhaseOpen:
		return unitOpen, nil
	case ConsolidationPhaseSealed:
		return unitSealed, nil
	case ConsolidationPhaseClosed:
		return unitClosed, nil
	default:
		return 0, ErrInvalidRehydratedConsolidation
	}
}

func sameMemberSet(current map[HandlingUnitID]bool, snapshot []HandlingUnitID) bool {
	if len(current) != len(snapshot) {
		return false
	}
	for _, member := range snapshot {
		if !current[member] {
			return false
		}
	}
	return true
}
