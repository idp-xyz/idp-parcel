package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是各口在 NOC-1 之前的摘要算法，即「无版本」那一版。改动前入库的记录存的是它们算出的
// 不带前缀的摘要，存量不回写（ADR-0014），所以它们只用来跟这类已存摘要比（domain.CompareStoredDigest），
// 不再写进新记录。函数体一个字节都不能改：改了，同内容的旧记录就会被答成冲突。

// unversionedAcceptanceDigest 是同一事项决定的内容比对锚：决定、范围、授权与依据任一不同即是
// 另一个决定。范围先排序——提交顺序不构成不同的决定。
func unversionedAcceptanceDigest(command AcceptCollaborationCommand) string {
	units := append([]string(nil), command.AcceptedUnits...)
	sort.Strings(units)
	actions := make([]string, 0, len(command.AcceptedActions))
	for _, action := range command.AcceptedActions {
		actions = append(actions, fmt.Sprintf("%d", action))
	}
	sort.Strings(actions)
	digest := sha256.Sum256([]byte(strings.Join(append(append([]string{
		fmt.Sprintf("%d", command.Decision),
		command.Node,
		command.Authority,
		command.Basis,
	}, units...), actions...), "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedExecutionDigest 是同一（事项+实物+动作）登记的内容比对锚：证据与业务时间任一不同
// 即是另一份内容。
func unversionedExecutionDigest(command RecordExecutionCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Evidence,
		command.PerformedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedConsolidationDigest 是同一来源身份的内容比对锚：来源身份之外的一切都算内容——动作、
// 对象、这一格特有的参数（载具/成员/封签/依据）、执行方、证据与业务发生时间，任一不同
// 即是另一份内容，按 AT-NO-043 形成冲突而不是覆盖先到者。
func unversionedConsolidationDigest(
	action domain.ConsolidationActionKind,
	source domain.WorkFactSource,
	parts ...string,
) string {
	fields := append([]string{
		action.String(),
		source.PerformedBy().String(),
		source.Evidence().String(),
		source.OccurredAt().UTC().Format(time.RFC3339Nano),
	}, parts...)
	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedDeliveryContentDigest 是同一来源身份的内容比对锚：实物、标识观察、接收观察、节点与
// 业务时间任一不同即是另一份内容。
func unversionedDeliveryContentDigest(command ReceiveDeliveredUnitCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Unit.String(),
		command.Mark.Mark,
		fmt.Sprintf("%d", command.Claim),
		command.Node.String(),
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
