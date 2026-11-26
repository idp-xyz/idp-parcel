package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// SplitOutboundPart 是拆出来的一份。键是这一份自己的来源请求键或决定标识。
type SplitOutboundPart struct {
	Key   domain.RequestKey
	Lines []domain.OutboundLine
}

// SplitOutboundCommand 拆一份尚未分配的出库请求。代录不带货主，货主取自被拆的那份。
type SplitOutboundCommand struct {
	Tenant   domain.TenantID
	Owner    domain.OwnerAccountID
	Origin   domain.OutboundOrigin
	Parent   domain.OutboundRequestID
	Operator domain.Registrant
	Basis    string
	Parts    []SplitOutboundPart
}

// SplitResult 是一次拆单。Children 只在拆成或重放时有。
type SplitResult struct {
	outcome  OutboundOutcome
	parent   domain.OutboundRequest
	children []domain.OutboundRequest
	has      bool
}

func (result SplitResult) Outcome() OutboundOutcome { return result.outcome }
func (result SplitResult) Parent() (domain.OutboundRequest, bool) {
	return result.parent, result.has
}
func (result SplitResult) Children() []domain.OutboundRequest {
	return append([]domain.OutboundRequest(nil), result.children...)
}

func (handler *OutboundHandler) Split(ctx context.Context, command SplitOutboundCommand) (SplitResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split outbound: %w", err)
	}
	if !configured {
		return SplitResult{outcome: OutboundPendingPolicy}, nil
	}
	parent, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Parent)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split outbound: %w", err)
	}
	if !found {
		return SplitResult{outcome: OutboundNotFound}, nil
	}
	if command.Origin == domain.OutboundOriginOperator {
		if command.Owner.String() != "" || command.Basis == "" || command.Operator.String() == "" {
			return SplitResult{outcome: OutboundContentInvalid, parent: parent, has: true}, nil
		}
	} else if command.Origin != domain.OutboundOriginCustomer || command.Owner != parent.Owner() || command.Basis != "" {
		return SplitResult{outcome: OutboundContentInvalid, parent: parent, has: true}, nil
	}
	if parent.Status() == domain.OutboundSplit {
		children, err := handler.requests.ListChildren(ctx, command.Tenant, parent.ID())
		if err != nil {
			return SplitResult{}, fmt.Errorf("split outbound: %w", err)
		}
		if sameSplit(children, command.Parts) {
			return SplitResult{outcome: OutboundExisting, parent: parent, children: children, has: true}, nil
		}
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	if !splittable(parent.Status()) {
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	if !partsConserve(parent.Lines(), command.Parts) {
		return SplitResult{outcome: OutboundContentInvalid, parent: parent, has: true}, nil
	}
	for _, part := range command.Parts {
		existing, found, err := handler.requests.LoadByKey(ctx, command.Tenant, command.Origin, part.Key)
		if err != nil {
			return SplitResult{}, fmt.Errorf("split outbound: %w", err)
		}
		if found {
			_ = existing
			return SplitResult{outcome: OutboundConflict, parent: parent, has: true}, nil
		}
	}
	released, err := handler.requests.ReleaseForSplit(ctx, command.Tenant, parent.ID())
	if err != nil {
		return SplitResult{}, fmt.Errorf("split outbound: %w", err)
	}
	if !released {
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	// 货主对这一张说过的话跟着这一张的货走：受理之后代录的同意盖过提交时带的，拆出的每一份都按它记。
	// 货主的默认不带过去——那不是对这一张说的，子请求没写明时各自按当时的默认读。
	said := parent.PartialShipment()
	consent, consented, err := handler.requests.LatestConsent(ctx, command.Tenant, parent.ID())
	if err != nil {
		return SplitResult{}, fmt.Errorf("split outbound: %w", err)
	}
	if consented {
		said = consent.Rule
	}
	owner := parent.Owner()
	children := make([]domain.OutboundRequest, 0, len(command.Parts))
	for _, part := range command.Parts {
		opened, err := handler.Accept(ctx, AcceptOutboundCommand{
			Tenant: command.Tenant, Owner: owner, Warehouse: parent.Warehouse(), Origin: command.Origin, Key: part.Key,
			Dispatch: parent.Dispatch(), Service: parent.Service(), Recipient: parent.Recipient(), Address: parent.Address(),
			Basis: command.Basis, Operator: command.Operator, Lines: part.Lines, Parent: parent.ID(),
			PlatformStocking: parent.PlatformStocking(), PartialShipment: said,
		})
		if err != nil {
			return SplitResult{}, err
		}
		if opened.Outcome() != OutboundReserved && opened.Outcome() != OutboundPartiallyReserved && opened.Outcome() != OutboundPendingPolicy && opened.Outcome() != OutboundRejected && opened.Outcome() != OutboundHandedOff {
			return SplitResult{outcome: opened.Outcome(), parent: parent, has: true}, nil
		}
		child, ok := opened.Request()
		if !ok {
			return SplitResult{}, fmt.Errorf("split outbound: 新请求没有落册")
		}
		children = append(children, child)
	}
	parent, found, err = handler.requests.LoadByID(ctx, command.Tenant, parent.ID())
	if err != nil || !found {
		return SplitResult{}, fmt.Errorf("split outbound: %w", err)
	}
	return SplitResult{outcome: OutboundSplitResult, parent: parent, children: children, has: true}, nil
}

func splittable(status domain.OutboundStatus) bool {
	return status == domain.OutboundReserved || status == domain.OutboundPartiallyReserved || status == domain.OutboundPendingPolicy
}

func partsConserve(original []domain.OutboundLine, parts []SplitOutboundPart) bool {
	if len(parts) < 2 {
		return false
	}
	want := map[string]int64{}
	for _, line := range original {
		want[lineKey(line)] += line.Quantity()
	}
	got := map[string]int64{}
	seen := map[string]struct{}{}
	for _, part := range parts {
		if part.Key.String() == "" || len(part.Lines) == 0 {
			return false
		}
		if _, ok := seen[part.Key.String()]; ok {
			return false
		}
		seen[part.Key.String()] = struct{}{}
		for _, line := range part.Lines {
			got[lineKey(line)] += line.Quantity()
		}
	}
	if len(want) != len(got) {
		return false
	}
	for key, quantity := range want {
		if got[key] != quantity {
			return false
		}
	}
	return true
}

func sameSplit(children []domain.OutboundRequest, parts []SplitOutboundPart) bool {
	if len(children) != len(parts) {
		return false
	}
	for _, part := range parts {
		matched := false
		for _, child := range children {
			if child.Key() != part.Key {
				continue
			}
			if len(child.Lines()) != len(part.Lines) {
				return false
			}
			for index := range part.Lines {
				if child.Lines()[index].Goods() != part.Lines[index].Goods() || child.Lines()[index].Unit() != part.Lines[index].Unit() ||
					child.Lines()[index].State() != part.Lines[index].State() || child.Lines()[index].Quantity() != part.Lines[index].Quantity() {
					return false
				}
			}
			matched = true
		}
		if !matched {
			return false
		}
	}
	return true
}

func lineKey(line domain.OutboundLine) string {
	return line.Goods().String() + "\x1f" + line.Unit().String() + "\x1f" + line.State().String() + "\x1f" +
		line.Alias().String() + "\x1f" + line.Batch().String() + "\x1f" + line.Expiry().String()
}
