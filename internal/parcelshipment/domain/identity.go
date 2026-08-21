package domain

import (
	"errors"
	"fmt"
	"strings"
)

var ErrBlankValue = errors.New("parcel shipment: blank value")

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

type CustomerAccountID struct{ requiredValue }

func NewCustomerAccountID(value string) (CustomerAccountID, error) {
	required, err := newRequiredValue("customer account ID", value)
	return CustomerAccountID{required}, err
}

type Source struct{ requiredValue }

func NewSource(value string) (Source, error) {
	required, err := newRequiredValue("source", value)
	return Source{required}, err
}

type SourceRequestKey struct{ requiredValue }

func NewSourceRequestKey(value string) (SourceRequestKey, error) {
	required, err := newRequiredValue("source request key", value)
	return SourceRequestKey{required}, err
}

// PayloadDigest 由 CanonicalizeSubmissionPayload 按版本化规范化形状产出，摘要串自带
// 版本前缀（ADR-0014）。类型本身不锁定版本：历史摘要按各自的前缀解读，跨版本的比较
// 纪律属引入下一个规范化版本的那笔工作。
type PayloadDigest struct{ requiredValue }

func NewPayloadDigest(value string) (PayloadDigest, error) {
	required, err := newRequiredValue("payload digest", value)
	return PayloadDigest{required}, err
}

type SubmissionBatchID struct{ requiredValue }

func NewSubmissionBatchID(value string) (SubmissionBatchID, error) {
	required, err := newRequiredValue("submission batch ID", value)
	return SubmissionBatchID{required}, err
}

type ShipmentRequestID struct{ requiredValue }

func NewShipmentRequestID(value string) (ShipmentRequestID, error) {
	required, err := newRequiredValue("shipment request ID", value)
	return ShipmentRequestID{required}, err
}

type DeclaredParcelID struct{ requiredValue }

func NewDeclaredParcelID(value string) (DeclaredParcelID, error) {
	required, err := newRequiredValue("declared parcel ID", value)
	return DeclaredParcelID{required}, err
}
