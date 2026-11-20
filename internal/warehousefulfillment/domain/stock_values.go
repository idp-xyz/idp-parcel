package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrUnknownStockWord 是库存状态或流水原因不在封闭集里。封闭集之外的词是接入层的坏载荷，到不了记数。
var ErrUnknownStockWord = errors.New("warehouse fulfillment: unknown closed-set word")

// ErrInvalidExpiryDate 是效期不是一个真实存在的公历日。只收到日，不带时刻与时区（CONTEXT「效期」）。
var ErrInvalidExpiryDate = errors.New("warehouse fulfillment: invalid expiry date")

// StockState 是库存状态，产品内置的封闭三格：良品、待检、次品（CONTEXT「库存状态」）。零值不是其中任何一格。
type StockState uint8

const (
	StockStateInvalid StockState = iota
	StockStateGood
	StockStatePendingInspection
	StockStateDefective
)

func (state StockState) String() string {
	switch state {
	case StockStateGood:
		return "GOOD"
	case StockStatePendingInspection:
		return "PENDING_INSPECTION"
	case StockStateDefective:
		return "DEFECTIVE"
	default:
		return ""
	}
}

// NewStockState 按字面认库存状态，只认 `GOOD`、`PENDING_INSPECTION` 与 `DEFECTIVE`。
func NewStockState(word string) (StockState, error) {
	for _, state := range []StockState{StockStateGood, StockStatePendingInspection, StockStateDefective} {
		if state.String() == word {
			return state, nil
		}
	}
	return StockStateInvalid, fmt.Errorf("%w: stock state", ErrUnknownStockWord)
}

// AllowedIn 回答这一状态能不能放进这种用途的库区（CONTEXT「库存账与守恒」：存储与拣选只放良品，次品库区只放次品，
// 收货暂存、退货暂存与待出库三种状态都可以放）。用途不在封闭集里时一律不相容：不能把不认识的用途当成能放。
func (state StockState) AllowedIn(usage ZoneUsage) bool {
	switch usage {
	case ZoneUsageStorage, ZoneUsagePicking:
		return state == StockStateGood
	case ZoneUsageDefective:
		return state == StockStateDefective
	case ZoneUsageReceivingStaging, ZoneUsageReturnStaging, ZoneUsageOutboundStaging:
		return state == StockStateGood || state == StockStatePendingInspection || state == StockStateDefective
	default:
		return false
	}
}

// StockReasonClass 是流水原因的类。行的形状随类，不随每一种原因各写一套（CONTEXT「库存账与守恒」）。
type StockReasonClass uint8

const (
	StockReasonClassInvalid StockReasonClass = iota
	StockReasonIncrease
	StockReasonDecrease
	StockReasonConserving
	StockReasonOffsetting
)

// StockMovementReason 是库存流水的原因，产品内置封闭集（CONTEXT「库存流水」）。零值不是其中任何一种。
type StockMovementReason uint8

const (
	StockMovementReasonInvalid StockMovementReason = iota
	StockReasonReceipt
	StockReasonReturnReceipt
	StockReasonTakeOnImport
	StockReasonInternalMove
	StockReasonUnitConversion
	StockReasonStateChange
	StockReasonRelabel
	StockReasonProcessing
	StockReasonDisassembly
	StockReasonUnboxing
	StockReasonOutboundConfirmation
	StockReasonCountGain
	StockReasonCountLoss
	StockReasonAdjustmentIncrease
	StockReasonAdjustmentDecrease
	StockReasonDestruction
)

func (reason StockMovementReason) String() string {
	switch reason {
	case StockReasonReceipt:
		return "RECEIPT"
	case StockReasonReturnReceipt:
		return "RETURN_RECEIPT"
	case StockReasonTakeOnImport:
		return "TAKE_ON_IMPORT"
	case StockReasonInternalMove:
		return "INTERNAL_MOVE"
	case StockReasonUnitConversion:
		return "UNIT_CONVERSION"
	case StockReasonStateChange:
		return "STATE_CHANGE"
	case StockReasonRelabel:
		return "RELABEL"
	case StockReasonProcessing:
		return "PROCESSING"
	case StockReasonDisassembly:
		return "DISASSEMBLY"
	case StockReasonUnboxing:
		return "UNBOXING"
	case StockReasonOutboundConfirmation:
		return "OUTBOUND_CONFIRMATION"
	case StockReasonCountGain:
		return "COUNT_GAIN"
	case StockReasonCountLoss:
		return "COUNT_LOSS"
	case StockReasonAdjustmentIncrease:
		return "ADJUSTMENT_INCREASE"
	case StockReasonAdjustmentDecrease:
		return "ADJUSTMENT_DECREASE"
	case StockReasonDestruction:
		return "DESTRUCTION"
	default:
		return ""
	}
}

// NewStockMovementReason 按字面认流水原因。
func NewStockMovementReason(word string) (StockMovementReason, error) {
	for _, reason := range []StockMovementReason{
		StockReasonReceipt,
		StockReasonReturnReceipt,
		StockReasonTakeOnImport,
		StockReasonInternalMove,
		StockReasonUnitConversion,
		StockReasonStateChange,
		StockReasonRelabel,
		StockReasonProcessing,
		StockReasonDisassembly,
		StockReasonUnboxing,
		StockReasonOutboundConfirmation,
		StockReasonCountGain,
		StockReasonCountLoss,
		StockReasonAdjustmentIncrease,
		StockReasonAdjustmentDecrease,
		StockReasonDestruction,
	} {
		if reason.String() == word {
			return reason, nil
		}
	}
	return StockMovementReasonInvalid, fmt.Errorf("%w: stock movement reason", ErrUnknownStockWord)
}

// Class 把原因归到四类。开箱归相抵：它是仓内按实点相抵的形态变化；这一段还没有整箱库存的行，记数口另拒，不在这里改类。
func (reason StockMovementReason) Class() StockReasonClass {
	switch reason {
	case StockReasonReceipt, StockReasonReturnReceipt, StockReasonTakeOnImport, StockReasonCountGain, StockReasonAdjustmentIncrease:
		return StockReasonIncrease
	case StockReasonOutboundConfirmation, StockReasonCountLoss, StockReasonAdjustmentDecrease, StockReasonDestruction:
		return StockReasonDecrease
	case StockReasonInternalMove, StockReasonStateChange, StockReasonRelabel, StockReasonUnitConversion:
		return StockReasonConserving
	case StockReasonProcessing, StockReasonDisassembly, StockReasonUnboxing:
		return StockReasonOffsetting
	default:
		return StockReasonClassInvalid
	}
}

// FormsInboundBatch 回答这种原因的记入行会不会形成新的入库批次（CONTEXT「入库批次」）。
// 开箱、盘盈、调增与不改汇总的四种都不形成：开箱沿用该箱的批次，其余要么点名已有批次，要么根本不新起一批。
func (reason StockMovementReason) FormsInboundBatch() bool {
	switch reason {
	case StockReasonReceipt, StockReasonReturnReceipt, StockReasonTakeOnImport, StockReasonProcessing, StockReasonDisassembly:
		return true
	default:
		return false
	}
}

// SourceFactReference 是一笔流水指向的来源事实，逐字保存。判重位置是（租户，原因，来源事实引用）。
type SourceFactReference struct{ requiredValue }

func NewSourceFactReference(value string) (SourceFactReference, error) {
	required, err := newRequiredValue("source fact reference", value)
	return SourceFactReference{required}, err
}

// StockMovementID 是库存流水的标识，由账签发，逐字保存。
type StockMovementID struct{ requiredValue }

func NewStockMovementID(value string) (StockMovementID, error) {
	required, err := newRequiredValue("stock movement ID", value)
	return StockMovementID{required}, err
}

// InboundBatchID 是入库批次的标识，由账签发，逐字保存。零值是没指名。
type InboundBatchID struct{ requiredValue }

func NewInboundBatchID(value string) (InboundBatchID, error) {
	required, err := newRequiredValue("inbound batch ID", value)
	return InboundBatchID{required}, err
}

// BatchNumber 是生产方或货主给的生产批号，逐字保存。零值就是没有：不跟踪批号的商品这一维为空（CONTEXT「批号」）。
// 给了的不得是空白；空白在构造处拒，不把「给了空白」存成「没有」。
type BatchNumber struct {
	value string
}

func NewBatchNumber(value string) (BatchNumber, error) {
	required, err := newRequiredValue("batch number", value)
	if err != nil {
		return BatchNumber{}, err
	}
	return BatchNumber{value: required.String()}, nil
}

func (number BatchNumber) String() string {
	return number.value
}

// Given 回答这一格有没有批号。
func (number BatchNumber) Given() bool {
	return number.value != ""
}

// ExpiryDate 是失效日期，只到日，不带时刻与时区（CONTEXT「效期」）。零值就是没有。
// 它按日文字本身比较，好进一格的键：用时刻会把时区与单调钟带进「同一天」。
type ExpiryDate struct {
	day string
}

// NewExpiryDate 只收 `YYYY-MM-DD` 且是真实存在的公历日。空白、存不下的字节、格式不对与不存在的日期都拒。
func NewExpiryDate(text string) (ExpiryDate, error) {
	if strings.TrimSpace(text) == "" {
		return ExpiryDate{}, fmt.Errorf("%w: expiry date", ErrBlankValue)
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return ExpiryDate{}, fmt.Errorf("%w: expiry date", ErrUnstorableValue)
	}
	parsed, err := time.Parse(time.DateOnly, text)
	if err != nil || parsed.Format(time.DateOnly) != text {
		return ExpiryDate{}, fmt.Errorf("%w: expiry date", ErrInvalidExpiryDate)
	}
	return ExpiryDate{day: text}, nil
}

func (date ExpiryDate) String() string {
	return date.day
}

// Given 回答这一格有没有效期。
func (date ExpiryDate) Given() bool {
	return date.day != ""
}
