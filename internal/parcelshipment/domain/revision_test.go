package domain_test

import (
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// TestNoStateTransitionMovesTheAggregateRevision 守 ADR-0028「状态转移一律不动版本」。
//
// 版本由仓储在写入成功后推进：一次保存对应一次推进，而两次转移之间只保存一次。转移各自加一
// 会让版本跳号，框架合同当场不成立；转移把版本刷回零更坏——随后一次按预期版本的写入要么被
// 当成并发冲突失败，要么覆盖掉别人的写入。
//
// 要守的风险不是「忘了带」，是日后某个转移改成重新构造一个 `ShipmentRequest{…}`。所以本条
// 不手工列举转移，而是反射枚举出全部转移再断言每一条都被下表盖到：ADR-0028 点名否掉了
// 「六处都要记得」，理由是它守不到第七个转移出现那天。
//
// 它直到重建入口落地才写得出来：版本非零的聚合只有那扇门造得出，`SubmitShipmentRequest`
// 产出的永远是零，而零过一遍转移还是零——那样的断言恒真，绿得毫无意义。
func TestNoStateTransitionMovesTheAggregateRevision(t *testing.T) {
	transitions := map[string]func(*testing.T, domain.ShipmentRequest) (domain.ShipmentRequest, error){
		"Decide": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.Decide(decisionSpec(t, everyGroupPassingFor(t, "parcel-1")))
		},
		"RejectByAuthority": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.RejectByAuthority(activeRejectionSpec(t))
		},
		"WithdrawByCustomer": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.WithdrawByCustomer(withdrawalSpec(t))
		},
		"CompleteManualReview": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.CompleteManualReview(reviewCompletion(t))
		},
		"RecordProcessingAttempt": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.RecordProcessingAttempt(
				internalAttempt(t, "COMMERCIAL_BASIS_UNAVAILABLE", firstAttemptAt),
			)
		},
		// 资料修订只对`已接受`开放，所以这一条要先越过决定边界。中间那次 Decide 同样不许动
		// 版本，链起来测反而比单测更接近真实调用序列。
		"AmendCustomerSourceData": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			accepted, err := request.Decide(decisionSpec(t, everyGroupPassingFor(t, "parcel-1")))
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			version, err := domain.FormCustomerSourceDataVersion(amendmentSpec(t))
			if err != nil {
				t.Fatalf("form customer source data version: %v", err)
			}
			return accepted.AmendCustomerSourceData(version)
		},
	}

	for _, name := range transitionMethodsOn(t, domain.ShipmentRequest{}) {
		if _, covered := transitions[name]; !covered {
			t.Errorf("ShipmentRequest.%s 是一条状态转移，本用例却没在守它不动聚合版本；"+
				"新转移要在表里加一行——ADR-0028 明否「六处都要记得」，它守不到第七处", name)
		}
	}

	for name, apply := range transitions {
		t.Run(name, func(t *testing.T) {
			start, err := domain.RehydrateShipmentRequest(submittedSnapshot(t))
			if err != nil {
				t.Fatalf("rehydrate: %v", err)
			}
			if start.Revision() == 0 {
				t.Fatal("前置不成立：起点聚合版本为零，「转移不动版本」这条断言会恒真")
			}

			moved, err := apply(t, start)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if moved.Revision() != start.Revision() {
				t.Fatalf("revision %d → %d；转移动了聚合版本，「一次保存对应一次推进」的合同就此不成立",
					start.Revision(), moved.Revision())
			}
		})
	}
}

// transitionMethodsOn 反射列出聚合上的全部状态转移：返回 `(聚合, error)` 的导出方法。
//
// 判据是签名而不是一份名字清单，这样第七个转移一写下来就会被列进来，随即因为表里没有对应行
// 而变红。名字清单做不到——它与它本该守住的东西一起腐。
func transitionMethodsOn(t *testing.T, aggregate domain.ShipmentRequest) []string {
	t.Helper()
	aggregateType := reflect.TypeOf(aggregate)
	errorType := reflect.TypeOf((*error)(nil)).Elem()

	var names []string
	for index := 0; index < aggregateType.NumMethod(); index++ {
		method := aggregateType.Method(index)
		if method.Type.NumOut() != 2 {
			continue
		}
		if method.Type.Out(0) != aggregateType || method.Type.Out(1) != errorType {
			continue
		}
		names = append(names, method.Name)
	}
	if len(names) == 0 {
		t.Fatal("没有反射到任何状态转移；本用例会永远空过")
	}
	return names
}
