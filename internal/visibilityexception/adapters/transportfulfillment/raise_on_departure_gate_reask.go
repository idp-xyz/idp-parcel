package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// ErrDepartureGateReaskNotVisible 表示按信封键还读不回那一次补问，或那一条出发的补问列不出来。可见性滞后或读不动是续办，重投会
// 改变结果。
var ErrDepartureGateReaskNotVisible = errors.New(
	"visibility exception transportfulfillment adapter: departure gate reask is not yet visible")

// DepartureGateReaskFinder 按键取回一次补问、按出发列出全部补问。由 TF 的 DepartureGateReasks 满足；本适配器不写 TF 的库。
type DepartureGateReaskFinder interface {
	FindByKey(ctx context.Context, key tfports.DepartureGateReaskKey) (tfports.DepartureGateReaskRecord, bool, error)
	FindByDeparture(ctx context.Context, departure tfports.MovementFactKey) ([]tfports.DepartureGateReaskRecord, error)
}

// RaiseOnDepartureGateReaskAdapter 是 veinbox.DepartureGateReaskConsumer 的真实处理方（ADR-0305）：运输履约替一件对象按原出发时点
// 补问了一次门禁，补问读数里关务判为阻断的那几件各形成一次出发门禁阻断信号，与出发首版读数那一路同一个判法
// （raiseDepartureGateSignals）。投影不在这里：对象进段之后那条出发的投影由控制事实信封上的补派生补（ADR-0224、ADR-0302）。
//
// 同一条出发更早的一份读数里已判过阻断的那件，不再形成：它已经在那一份上交过分诊——出发首版那一份由移动事实那一路交，更早的补问
// 由它自己那一封交。更早取补问册的次序（落册时刻、对象、次序）：同一条出发的补问信封与它同排一个分区，先落册的先到。
type RaiseOnDepartureGateReaskAdapter struct {
	reasks  DepartureGateReaskFinder
	facts   MovementFactFinder
	signals DepartureGateSignalRaiser
}

func NewRaiseOnDepartureGateReaskAdapter(
	reasks DepartureGateReaskFinder,
	facts MovementFactFinder,
	signals DepartureGateSignalRaiser,
) (*RaiseOnDepartureGateReaskAdapter, error) {
	if reasks == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: departure gate reask finder is nil")
	}
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: movement fact finder is nil")
	}
	if signals == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: departure gate signal raiser is nil")
	}
	return &RaiseOnDepartureGateReaskAdapter{reasks: reasks, facts: facts, signals: signals}, nil
}

var _ veinbox.RecordedDepartureGateReaskHandler = (*RaiseOnDepartureGateReaskAdapter)(nil)

// HandleRecordedDepartureGateReask 按信封键取回那一次补问与它补的那条出发，为补问读数里新判阻断的每一件形成信号。
//
// 一封信一笔事务：读不回、读不动或信号搁置即整封回滚重投；记录与键不符、某一行装不回是仓储不变量已破。补问只补在原版出发上
// （更正版沿用原读数），补的不是原版出发也算不变量已破。
func (adapter *RaiseOnDepartureGateReaskAdapter) HandleRecordedDepartureGateReask(
	ctx context.Context,
	recorded veinbox.RecordedDepartureGateReask,
) error {
	key, err := departureGateReaskKeyFor(recorded)
	if err != nil {
		return err
	}
	record, found, err := adapter.reasks.FindByKey(ctx, key)
	if errors.Is(err, tfports.ErrDepartureGateReaskRecordUnreadable) {
		return fmt.Errorf("%w: departure gate reask %q: %v", ErrMovementFactRecordInconsistent, recorded.Fact, err)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDepartureGateReaskNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: fact %q version %q object %q sequence %s",
			ErrDepartureGateReaskNotVisible, recorded.Fact, recorded.Version, recorded.Object, recorded.Sequence)
	}
	if record.Key != key {
		return fmt.Errorf("%w: departure gate reask %q", ErrMovementFactRecordInconsistent, recorded.Fact)
	}

	departure := key.Departure()
	factRecord, found, err := adapter.facts.FindByKey(ctx, departure)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMovementFactNotVisible, err)
	}
	// 补问那一行有外键挂在这一版出发上：补问读得到、出发却读不回，是仓储不变量已破，不是等谁。
	if !found || !movementFactRecordAgrees(factRecord, departure) {
		return fmt.Errorf("%w: fact %q version %q", ErrMovementFactRecordInconsistent, recorded.Fact, recorded.Version)
	}
	fact := factRecord.Fact
	first, read := fact.GateReading()
	if _, corrected := fact.Corrects(); fact.Kind() != tfdomain.DepartureFact || corrected || !read {
		return fmt.Errorf("%w: fact %q version %q is not an original departure", ErrMovementFactRecordInconsistent,
			recorded.Fact, recorded.Version)
	}

	all, err := adapter.reasks.FindByDeparture(ctx, departure)
	if errors.Is(err, tfports.ErrDepartureGateReaskRecordUnreadable) {
		return fmt.Errorf("%w: departure gate reasks of %q: %v", ErrMovementFactRecordInconsistent, recorded.Fact, err)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDepartureGateReaskNotVisible, err)
	}
	already := blockedObjects(nil, first)
	listed := false
	for _, earlier := range all {
		if earlier.Key == key {
			listed = true
			break
		}
		already = blockedObjects(already, earlier.Reading)
	}
	if !listed {
		return fmt.Errorf("%w: departure gate reask %q sequence %s is not listed for its departure",
			ErrMovementFactRecordInconsistent, recorded.Fact, recorded.Sequence)
	}
	return raiseDepartureGateSignals(ctx, adapter.signals, fact, record.Reading, already)
}

// blockedObjects 把 reading 里逐对象判为阻断的那几件并进 into（集运单元那一格不算，理由同 raiseDepartureGateSignals）。
func blockedObjects(
	into map[tfdomain.CarriedObjectReference]bool,
	reading tfdomain.DepartureGateReading,
) map[tfdomain.CarriedObjectReference]bool {
	if into == nil {
		into = map[tfdomain.CarriedObjectReference]bool{}
	}
	for _, finding := range reading.Findings() {
		if _, unit := finding.ConsolidationUnit(); !unit && finding.Answer() == tfdomain.DepartureGateBlocked {
			into[finding.Object()] = true
		}
	}
	return into
}

// departureGateReaskKeyFor 把信封五维译成 TF 补问登记册的键。译不出来是引用坏了，不是等谁。
func departureGateReaskKeyFor(recorded veinbox.RecordedDepartureGateReask) (tfports.DepartureGateReaskKey, error) {
	none := tfports.DepartureGateReaskKey{}
	departure, err := movementFactKeyFor(veinbox.RecordedMovementFact{
		TenantID: recorded.TenantID, Fact: recorded.Fact, Version: recorded.Version,
	})
	if err != nil {
		return none, err
	}
	object, err := tfdomain.NewCarriedObjectReference(recorded.Object)
	if err != nil {
		return none, fmt.Errorf("%w: reasked object: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	sequence, err := strconv.Atoi(recorded.Sequence)
	if err != nil || sequence < 1 {
		return none, fmt.Errorf("%w: reask sequence %q", ErrMovementFactUntranslatableAnswer, recorded.Sequence)
	}
	return tfports.DepartureGateReaskKey{
		TenantID: departure.TenantID,
		Fact:     departure.Fact,
		Version:  departure.Version,
		Object:   object,
		Sequence: sequence,
	}, nil
}
