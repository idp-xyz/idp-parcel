package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 出发门禁补问读数的存取口（ADR-0305）。另立文件，理由同 departure_gate.go：本包按关注点分文件。

// DepartureGateReaskKey 是一次补问的键：哪一条出发的哪一版、替哪一件对象、第几次。同一条出发的同一件对象按 Sequence 从 1 往上数，
// 撞键就是另一方先补了这一次。
type DepartureGateReaskKey struct {
	TenantID domain.TenantID
	Fact     domain.MovementFactReference
	Version  domain.MovementFactVersion
	Object   domain.CarriedObjectReference
	Sequence int
}

// Departure 交回这次补问补的那一条出发在移动事实登记册上的键。
func (key DepartureGateReaskKey) Departure() MovementFactKey {
	return MovementFactKey{TenantID: key.TenantID, Fact: key.Fact, Version: key.Version}
}

// DepartureGateReaskRecord 是一次补问越过提交边界留下的东西：按原出发时点替 Key.Object 再问一次问来的读数，唤醒它的那一份控制
// 事实（Trigger，只供追溯，不进任何判断），与落册时刻。读数的形状与出发首版那一份同一个领域类型：补问走的是同一条读数路径。
type DepartureGateReaskRecord struct {
	Key        DepartureGateReaskKey
	Reading    domain.DepartureGateReading
	Trigger    string
	RecordedAt time.Time
}

// DepartureGateReaskSaveOutcome 是追加一次补问的结果。撞键是业务答案不是错误（ADR-0031）。
type DepartureGateReaskSaveOutcome uint8

const (
	DepartureGateReaskSaveOutcomeInvalid DepartureGateReaskSaveOutcome = iota
	DepartureGateReaskSaved
	DepartureGateReaskAlreadyRegistered
)

// ErrDepartureGateReaskRecordUnreadable：补问读数那一行读回来了，却过不了重建门。坏行不随重读变好，与读不动分开报，调用方据此不把
// 它答成未决（ADR-0029）。
var ErrDepartureGateReaskRecordUnreadable = errors.New("transport fulfillment: a stored departure gate reask cannot be read back")

// DepartureGateReaskRegistry 按键找回、按出发列出并追加补问读数。
//
// **只插不改，本口没有也不会有 Update。** 出发首版那一份读数留在移动事实上不动（迁移 0026），补问一次追加一行；改写哪一行，「当时
// 关务怎么答」就查不回来了。
//
// FindByDeparture 交回这一条出发这一版上的全部补问，各件对象混在一起，按落册时刻、再按对象、再按 Sequence 排，重放时一字不变；
// 没有补问交回空切片，不是错误。error 有两类：包住 ErrDepartureGateReaskRecordUnreadable 的是某一行过不了重建门，其余是读不动。
// FindByKey 同此。
type DepartureGateReaskRegistry interface {
	FindByKey(ctx context.Context, key DepartureGateReaskKey) (DepartureGateReaskRecord, bool, error)
	FindByDeparture(ctx context.Context, departure MovementFactKey) ([]DepartureGateReaskRecord, error)
	Save(ctx context.Context, record DepartureGateReaskRecord) (DepartureGateReaskSaveOutcome, error)
}

// DepartureGateReaskHandoff 把一次补问交出去，与那一行同一笔入队：补问读数里的阻断由全程追踪形成信号（ADR-0305，ADR-0205
// 决定五同一条路），本上下文不立异常案件。
type DepartureGateReaskHandoff interface {
	HandOffDepartureGateReask(ctx context.Context, record DepartureGateReaskRecord) error
}
