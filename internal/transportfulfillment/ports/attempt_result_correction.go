package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果的登记册（票 docs-consistency/code/90）。
//
// 更正只追加：首次提交的结果行（待确认那一版）一个字不改，更正行回指它、带新的确定结果、所凭的后补来源与更正时刻。
// 一格待确认至多一条更正——对已确定判断的再更正不在这条路上（domain.ErrAttemptResultNotPending）。

// PickupResultCorrectionRecord 是一次揽收结果更正越过提交边界留下的东西。
//
// Control 只在更正成揽收到手时在：它是更正时所报的控制依据，**只为重放比对留**——控制事实本身是对象级揽收登记上的
// OffsitePickup（那里才是控制依据的权威），这里再存一份是为了同一更正换了说法的重放认得出冲突，而不必去比一条可能
// 已被它自己的更正改过的登记。DecidedBy 是作出这一判断的操作者（「发行方#sub」），只在运营决定面提交时在；它是操作
// 证据，只进审计，不进重放比对，也不进任何门（ADR-0303 的待遇）。
type PickupResultCorrectionRecord struct {
	Key        PickupAttemptKey
	Correction domain.AttemptObjectResultCorrection
	Control    domain.TransportControlReference
	DecidedBy  domain.RegistrantReference
	RecordedAt time.Time
}

// PickupResultCorrectionTarget 是被更正的那一格连同它的尝试：首次提交的结果、尝试本体与已落的更正（若有）。
type PickupResultCorrectionTarget struct {
	Attempt       domain.FulfillmentAttempt
	Result        domain.AttemptObjectResult
	Correction    PickupResultCorrectionRecord
	HasCorrection bool
}

type PickupResultCorrectionSaveOutcome uint8

const (
	PickupResultCorrectionSaveOutcomeInvalid PickupResultCorrectionSaveOutcome = iota
	PickupResultCorrectionSaved
	PickupResultCorrectionAlreadyRecorded
)

// PickupResultCorrections 按（尝试提交的来源身份，对象）找回并追加揽收结果更正。
//
// 按来源身份而不按尝试身份指名：揽收结果行按尝试提交的来源身份落（pickup_attempt_result 的主键），同一尝试身份是否只对
// 一份提交今天没有约束，按尝试身份指名会指不准。
//
// LoadForCorrection 在调用方的事务里锁住这格结果，直到事务结束：同一格的并发更正排队，后到的读到先到的更正，不会各自
// 看见「还没更正」。更正随后经对象级揽收登记形成场外揽收的那一条路（揽收到手时）也在同一事务里，锁序是：这格结果行 →
// 对象级揽收登记键与段册（enterFulfillmentSegment 自己的顺序）。没有任何路径先取段册或登记键再回来取结果行：结果行只在
// 这一处加锁，而尝试登记口写结果行是在父行刚落下的同一笔事务里、不持别的锁。
type PickupResultCorrections interface {
	LoadForCorrection(
		ctx context.Context,
		key PickupAttemptKey,
		object domain.CarriedObjectReference,
	) (PickupResultCorrectionTarget, bool, error)
	Save(ctx context.Context, record PickupResultCorrectionRecord) (PickupResultCorrectionSaveOutcome, error)
}

// DeliveryResultCorrectionRecord 是一次派送结果更正越过提交边界留下的东西；DecidedBy 的待遇同
// PickupResultCorrectionRecord。更正成妥投不形成任何别的事实：有效交付仍由交付登记带着 POD 另登。
type DeliveryResultCorrectionRecord struct {
	Key        DeliveryAttemptKey
	Correction domain.DeliveryAttemptResultCorrection
	DecidedBy  domain.RegistrantReference
	RecordedAt time.Time
}

type DeliveryResultCorrectionTarget struct {
	Attempt       domain.FulfillmentAttempt
	Result        domain.DeliveryAttemptResult
	Correction    DeliveryResultCorrectionRecord
	HasCorrection bool
}

type DeliveryResultCorrectionSaveOutcome uint8

const (
	DeliveryResultCorrectionSaveOutcomeInvalid DeliveryResultCorrectionSaveOutcome = iota
	DeliveryResultCorrectionSaved
	DeliveryResultCorrectionAlreadyRecorded
)

// DeliveryResultCorrections 按（尝试身份，对象）找回并追加派送结果更正；加锁与锁序同 PickupResultCorrections，派送这一侧
// 更正之后不再触碰别的登记册，锁序只有这一格结果行。
type DeliveryResultCorrections interface {
	LoadForCorrection(
		ctx context.Context,
		key DeliveryAttemptKey,
		object domain.CarriedObjectReference,
	) (DeliveryResultCorrectionTarget, bool, error)
	Save(ctx context.Context, record DeliveryResultCorrectionRecord) (DeliveryResultCorrectionSaveOutcome, error)
}

// 复核查阅「还在待确认的对象」的行（揽收或派送一侧各一种，Side 区分）：待确认那一版的来源与缺口，加上复核的人下一步要指名的键——
// 揽收一侧按尝试提交的来源身份加对象，派送一侧按尝试身份加对象。已凭后补来源更正成确定结果的不再在册上列出。
const (
	PendingAttemptSidePickup   = "PICKUP"
	PendingAttemptSideDelivery = "DELIVERY"
)

type PendingAttemptResultRow struct {
	Side string
	// SourceID 只在揽收一侧：更正口按它指名被更正的那一格（尝试登记口里的 sourceId）。
	SourceID   string
	Attempt    string
	Object     string
	Task       string
	ExecutedBy string
	Place      string
	// Source 是待确认所凭的已有来源，Gap 是缺口或冲突（UC-TF-002「揽收结果待确认」、UC-TF-006「尝试待确认」要保存的两样）。
	Source     string
	Gap        string
	OccurredAt time.Time
}
