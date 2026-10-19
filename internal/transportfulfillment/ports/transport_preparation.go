package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是运输准备（ADR-0201 决定六至八，票 demo-chain-gaps/18）越过提交边界与上下文边界的形状。准备结果是一个新聚合，
// 它的端口与既有聚合的端口各立一处，不混进 ports.go（计划「跨票通则」X4）。

var (
	// ErrDeclaredDemandUntranslatable：提供方答了，却译不成本上下文的需求量快照——集外取值、申报的数与单位立不成
	// 快照，或本上下文的租户与对象在提供方的词汇里立不起来。与读不动分开：读不动过一会儿可能就好，这一类重问一万次
	// 也是同一个结果，要查的是两边的口径。编排把它原样上抛、不答未决（ADR-0029：确定性错误不进未决名单）。
	ErrDeclaredDemandUntranslatable = errors.New(
		"transport fulfillment: the declared measurement cannot be translated into a demand snapshot")
	// ErrTransportPreparationRecordUnreadable：库里同一身份那一份读回来了，却过不了重建门，或依据摘要的形状认不出。
	// 坏行不随重投变好；编排原样上抛等人看，不答未决，也不在坏行上分`已有结果`与`结果冲突`。
	ErrTransportPreparationRecordUnreadable = errors.New(
		"transport fulfillment: the stored transport preparation cannot be read back")
)

// DeclaredDemandSource 答一个载运对象收寄前的申报需求量（ADR-0201 决定八）。按（租户、对象）问：准备由计划段触发，
// 手上只有对象身份，没有委托或接受基线。客户没报、提供方此刻说不准按哪一版，都是快照里缺了那一维，不是 error。
// error 有两类：包住 ErrDeclaredDemandUntranslatable 的是译不过去，其余是读不动——提供方此刻答不出任何一格。
type DeclaredDemandSource interface {
	LoadDeclaredDemand(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) (domain.DemandSnapshot, error)
}

// CapacityPoolsOnSchedules 按班次交回登记册上的池（含预占）。只在已登记的班次上问：只经评估物化的那一班没有登过的池。
// 与 CapacityPoolStore 分开：那一口按池身份读写，准备评估只读、按班次成批问。
type CapacityPoolsOnSchedules interface {
	ListPoolsOnSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		schedules []domain.ScheduleReference,
	) ([]CapacityPoolRecord, error)
}

// TransportPreparationKey 是准备结果的身份：（租户、计划段引用、对象、规则版本）——同一段同一对象按同一规则只有一份
// 当前有效结果（ADR-0201 决定七）。计划段引用是 NR 的「版本#序位」，改路出新版本就是新引用、新结果。
type TransportPreparationKey struct {
	TenantID domain.TenantID
	Leg      domain.PlannedSegmentReference
	Object   domain.CarriedObjectReference
	Rule     domain.PreparationRuleVersion
}

// TransportPreparationRecord 是一份准备结果越过提交边界留下的东西。InputDigest 是依据的摘要，重放与冲突按它分。
type TransportPreparationRecord struct {
	Key         TransportPreparationKey
	InputDigest string
	Preparation domain.TransportPreparation
	RecordedAt  time.Time
}

// TransportPreparationSaveOutcome 是落一份结果的写入代数（ADR-0031）：撞键是业务答案不是错误，另一方先落了同一身份，
// 调用方读回赢家再分重放与冲突。
type TransportPreparationSaveOutcome uint8

const (
	TransportPreparationSaveOutcomeInvalid TransportPreparationSaveOutcome = iota
	TransportPreparationSaved
	TransportPreparationAlreadyRecorded
)

func (outcome TransportPreparationSaveOutcome) String() string {
	switch outcome {
	case TransportPreparationSaved:
		return "SAVED"
	case TransportPreparationAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// TransportPreparationStore 按身份找回并保存准备结果，只插不改：同一身份上不覆盖原结果（UC-TF-003「不得按最后到达
// 覆盖原结果」）。FindByKey 读回的那一行过不了重建门时，error 包住 ErrTransportPreparationRecordUnreadable，与读不动分开。
type TransportPreparationStore interface {
	FindByKey(ctx context.Context, key TransportPreparationKey) (TransportPreparationRecord, bool, error)
	Save(ctx context.Context, record TransportPreparationRecord) (TransportPreparationSaveOutcome, error)
}
