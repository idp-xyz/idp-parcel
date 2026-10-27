package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果（票 docs-consistency/code/90；TF CONTEXT「揽收、派送与交付」）。
//
// 更正是一条独立的口，两侧（揽收、派送）各一个处理器，形同而不合并：揽收一侧更正成揽收到手还要经对象级揽收登记形成场外
// 揽收、进段、交 parcel-shipment 采认，派送一侧更正成妥投不形成别的事实。两个处理器共用本文件的答复格。
//
// 谁来提交由接入面定，不由处理器区分：执行方在作业事实面后补（更正随原事实的族，ADR-0151 决定四），复核人在运营决定面认定
// （ADR-0297）。两面交进来的是同一条命令，运营决定面另带提交操作者作为操作证据（DecidedBy）。

// AttemptResultCorrectionOutcome 是一次更正的应用处理结果。各格按恢复动作分（ADR-0029）：`来源未受理`是改请求，`并非待确认`是
// 承认那一格已是确定结果（不更正），`冲突`是同一格已被更正成别的说法，`未决`才是等依赖。
type AttemptResultCorrectionOutcome uint8

const (
	AttemptResultCorrectionOutcomeInvalid AttemptResultCorrectionOutcome = iota
	AttemptResultCorrected
	AttemptResultCorrectionExisting
	AttemptResultCorrectionConflict
	AttemptResultNotPending
	AttemptResultCorrectionNotAccepted
	AttemptResultCorrectionUndecided
)

func (outcome AttemptResultCorrectionOutcome) String() string {
	switch outcome {
	case AttemptResultCorrected:
		return "RESULT_CORRECTED"
	case AttemptResultCorrectionExisting:
		return "EXISTING_CORRECTION"
	case AttemptResultCorrectionConflict:
		return "SOURCE_CONFLICT"
	case AttemptResultNotPending:
		return "RESULT_NOT_PENDING"
	case AttemptResultCorrectionNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case AttemptResultCorrectionUndecided:
		return "CORRECTION_UNDECIDED"
	default:
		return ""
	}
}

// AttemptResultCorrectionUndecidedReason 指名更正停在哪一步等谁。封闭集合：新依赖故障必须补格。
type AttemptResultCorrectionUndecidedReason uint8

const (
	AttemptResultCorrectionUndecidedReasonNone AttemptResultCorrectionUndecidedReason = iota
	AttemptResultCorrectionStoreUnavailable
	// AttemptResultCorrectionPickupRegistrationUnsettled 只在揽收一侧：更正成揽收到手时经对象级揽收登记形成场外揽收，
	// 那一条路答了未决。
	AttemptResultCorrectionPickupRegistrationUnsettled
)

func (reason AttemptResultCorrectionUndecidedReason) String() string {
	switch reason {
	case AttemptResultCorrectionStoreUnavailable:
		return "CORRECTION_STORE_UNAVAILABLE"
	case AttemptResultCorrectionPickupRegistrationUnsettled:
		return "PICKUP_REGISTRATION_UNSETTLED"
	default:
		return ""
	}
}

func attemptResultCorrectionContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// correctionContent 是同一格重放比对的锚：更正说了什么，不含登记时刻与操作证据。时刻折到登记册精度再写，与库里读回的
// 同形；控制依据只在更正成揽收到手时非空。
func correctionContent(outcome, basis, source, control string, occurredAt, correctedAt time.Time) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		outcome, basis, source, control,
		domain.AtRegistryPrecision(occurredAt).Format(time.RFC3339Nano),
		domain.AtRegistryPrecision(correctedAt).Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func isNotPending(err error) bool {
	return errors.Is(err, domain.ErrAttemptResultNotPending)
}
