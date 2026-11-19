package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// ActualMeasurementKey 是一条实际测量或一条作废的幂等键：租户加上现场给的来源身份。测量与作废共用一个来源身份空间：
// 变化事件的标识与下游续办的依据都由来源身份定，一条测量与一条作废带同一个来源身份，后到的那次变化会被当成已经办过。
// 这个空间由两层守：编排先取锁再读另一类，答出来源冲突这个明确的结果；登记册在库上以来源身份登记表兜底，编排没盖住时
// 写口交回错误，不记成功也不当成重放。
type ActualMeasurementKey struct {
	TenantID domain.TenantID
	SourceID string
}

// ActualMeasurementRecord 是登记册上的一条测量。内容摘要是落下时的那一份，记录时刻
// 由服务端给，不进摘要。
type ActualMeasurementRecord struct {
	Key           ActualMeasurementKey
	Measurement   domain.ActualMeasurement
	ContentDigest string
	RecordedAt    time.Time
}

// ActualMeasurementSaveOutcome 是一次写入越过提交边界之前的结果。已有记录不是错误：
// 调用方还要在同一个事务里读回原件，比内容摘要。
type ActualMeasurementSaveOutcome uint8

const (
	ActualMeasurementSaveOutcomeInvalid ActualMeasurementSaveOutcome = iota
	ActualMeasurementSaved
	ActualMeasurementAlreadyRecorded
)

func (outcome ActualMeasurementSaveOutcome) String() string {
	switch outcome {
	case ActualMeasurementSaved:
		return "SAVED"
	case ActualMeasurementAlreadyRecorded:
		return "ALREADY_RECORDED"
	case ActualMeasurementSaveOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// MeasurementInvalidationRecord 是登记册上的一条作废。
type MeasurementInvalidationRecord struct {
	Key           ActualMeasurementKey
	Invalidation  domain.MeasurementInvalidation
	ContentDigest string
	RecordedAt    time.Time
}

// MeasurementInvalidationSaveOutcome 把「这条来源已经写过」和「这条测量已经被另一条
// 作废指过」分开。后者不是重放：来源身份不同，读回哪一条都不是这次上报的答案。
type MeasurementInvalidationSaveOutcome uint8

const (
	MeasurementInvalidationSaveOutcomeInvalid MeasurementInvalidationSaveOutcome = iota
	MeasurementInvalidationSaved
	MeasurementInvalidationAlreadyRecorded
	MeasurementInvalidationTargetMissing
	MeasurementInvalidationAlreadyVoid
)

func (outcome MeasurementInvalidationSaveOutcome) String() string {
	switch outcome {
	case MeasurementInvalidationSaved:
		return "SAVED"
	case MeasurementInvalidationAlreadyRecorded:
		return "ALREADY_RECORDED"
	case MeasurementInvalidationTargetMissing:
		return "TARGET_MISSING"
	case MeasurementInvalidationAlreadyVoid:
		return "ALREADY_VOID"
	case MeasurementInvalidationSaveOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// ActualMeasurementStore 是实际测量登记册。ListStillValid 只答没有作废指过来的那些。
// 从多条里取出当前有效实测不在这只写口上，见 MeasurementSelectionAdoptionStore。
type ActualMeasurementStore interface {
	// SerializeSourceIdentity 在当前事务里取（租户，来源身份）的锁，事务结束即放；事务外调用被拒。测量与作废共用这一把：
	// 登记与作废在读另一类有没有同一来源身份之前先取它，并发到的一条测量与一条作废才不会都读到「没有」而各写各的。
	// 这把锁只为让编排答出来源冲突；锁没盖住时，写口自己在库上被来源身份登记表拦下，以错误交回。
	SerializeSourceIdentity(ctx context.Context, key ActualMeasurementKey) error
	FindMeasurement(ctx context.Context, key ActualMeasurementKey) (ActualMeasurementRecord, bool, error)
	SaveMeasurement(ctx context.Context, record ActualMeasurementRecord) (ActualMeasurementSaveOutcome, error)
	ListStillValid(
		ctx context.Context,
		tenant domain.TenantID,
		objectKind domain.MeasurementObjectKind,
		objectID string,
	) ([]ActualMeasurementRecord, error)
	FindInvalidation(ctx context.Context, key ActualMeasurementKey) (MeasurementInvalidationRecord, bool, error)
	SaveInvalidation(ctx context.Context, record MeasurementInvalidationRecord) (MeasurementInvalidationSaveOutcome, error)
}

// MeasurementSelectionAdoption 是租户采用的当前有效实测形态。没有这一行就是没采用。
type MeasurementSelectionAdoption struct {
	TenantID   domain.TenantID
	Form       domain.MeasurementSelectionForm
	Basis      string
	RecordedAt time.Time
}

// MeasurementSelectionSaveOutcome 把重放和「同一租户已经按另一份依据采用过」分开。
// 已经写下的那一行不改。
type MeasurementSelectionSaveOutcome uint8

const (
	MeasurementSelectionSaveOutcomeInvalid MeasurementSelectionSaveOutcome = iota
	MeasurementSelectionSaved
	MeasurementSelectionAlreadyRecorded
	MeasurementSelectionConflicts
)

func (outcome MeasurementSelectionSaveOutcome) String() string {
	switch outcome {
	case MeasurementSelectionSaved:
		return "SAVED"
	case MeasurementSelectionAlreadyRecorded:
		return "ALREADY_RECORDED"
	case MeasurementSelectionConflicts:
		return "CONFLICTS"
	case MeasurementSelectionSaveOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// MeasurementSelectionAdoptionStore 是当前有效实测形态的采用登记册。一租户一行。
type MeasurementSelectionAdoptionStore interface {
	FindAdoption(ctx context.Context, tenant domain.TenantID) (MeasurementSelectionAdoption, bool, error)
	SaveAdoption(ctx context.Context, adoption MeasurementSelectionAdoption) (MeasurementSelectionSaveOutcome, error)
}

// CurrentParcelAssociation 在包裹与作业实物的当前关联上双向列。
// 只读各条判断链的当前有效版本，被接续过的那一版上的关联不再算。
type CurrentParcelAssociation interface {
	ListHandlingUnits(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelAssociationReference,
	) ([]domain.HandlingUnitID, error)
	ListParcels(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
	) ([]domain.ParcelAssociationReference, error)
}

// MeasurementChanged 是一次实际测量登记或作废已经写下。来源身份是这次变化自己的键，不是被作废的那一条。
type MeasurementChanged struct {
	Tenant     domain.TenantID
	SourceID   string
	ObjectKind domain.MeasurementObjectKind
	ObjectID   string
	OccurredAt time.Time
}

// MeasurementChangeNotice 把变化交给下游。同一来源重投发的是同一封。
type MeasurementChangeNotice interface {
	NoticeMeasurementChanged(ctx context.Context, change MeasurementChanged) error
}
