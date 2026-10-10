package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 本文件是各口在 TFC-1 之前的摘要算法，即「无版本」那一版。改动前入库的记录存的是它们算出的
// 不带前缀的摘要，存量不回写（ADR-0014），所以它们只用来跟这类已存摘要比（domain.CompareStoredDigest），
// 不再写进新记录。函数体一个字节都不能改：改了，同内容的旧记录就会被答成冲突。
//
// 交接更正口没有这一版：它的重放与撞键都走版本键，从不比摘要。

// unversionedDispositionDigest 折叠承接决定的全部业务内容：同键异指纹即冒名冲突。
func unversionedDispositionDigest(command AcceptRegulatoryDispositionCommand) string {
	objects := append([]string(nil), command.AcceptedObjects...)
	sort.Strings(objects)
	parts := []string{
		command.Basis.String(),
		strings.TrimSpace(command.MovementAction),
		command.Decision.String(),
		strings.Join(objects, ","),
		command.DeclineBasis,
		strings.TrimSpace(command.MovementAuthority),
		command.DecidedAt.UTC().Format(time.RFC3339Nano),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:8])
}

func unversionedCommissionDigest(command SubmitCommissionCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Provider,
		command.Agreement,
		command.Conditions,
		command.Role,
		command.Responsibility,
		command.SubmittedAt.UTC().Format(time.RFC3339Nano),
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedBookingDigest(command SubmitBookingCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Commission,
		fmt.Sprintf("%d", command.Quantity),
		command.Unit,
		command.RequestedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedAnswerDigest(command AnswerBookingCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Outcome),
		command.Acceptance,
		fmt.Sprintf("%d", command.Quantity),
		command.Basis,
		command.DecidedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedPickupContentDigest 是同一来源身份的内容比对锚：尝试身份、任务、到场时间与逐对象
// 结果（对象、走向、依据、控制、时间）任一不同即是另一份内容。对象行先排序——提交
// 顺序不构成不同的内容。
func unversionedPickupContentDigest(command PerformOffsitePickupCommand) string {
	lines := make([]string, 0, len(command.Objects))
	for _, submission := range command.Objects {
		lines = append(lines, strings.Join([]string{
			submission.Object.String(),
			fmt.Sprintf("%d", submission.Outcome),
			submission.Basis.String(),
			submission.Control.String(),
			submission.OccurredAt.UTC().Format(time.RFC3339Nano),
		}, "\x1f"))
	}
	sort.Strings(lines)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Attempt,
		command.Task,
		command.ArrivedAt.UTC().Format(time.RFC3339Nano),
	}, lines...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedOpportunityDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedDeliveryContentDigest 是同一（对象+尝试）首登的内容比对锚：方式、接收方与 POD 任一
// 不同即是另一份内容。
func unversionedDeliveryContentDigest(command RegisterEffectiveDeliveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Method,
		command.Recipient,
		command.Proof,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedPickupRegistrationDigest 是同一（对象+尝试）首登的内容比对锚：任务、地点、控制、
// 执行方与业务时间任一不同即是另一份内容。
func unversionedPickupRegistrationDigest(command RegisterOffsitePickupCommand) string {
	return unversionedPickupRegistrationContentDigest(command.Task, command.Place, command.Control, command.ExecutedBy, command.OccurredAt)
}

// unversionedPickupRegistrationContentDigest 让首登与更正版本的记录带同一种内容比对锚——它描述的是这一版**说了什么**，
// 而不是这一版怎么来的。于是重放与冲突在两个入口上是同一套判据：首登重放对上当前版是已有版本、
// 对不上是冲突；更正重放同理。
func unversionedPickupRegistrationContentDigest(task, place, control, executedBy string, occurredAt time.Time) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		task,
		place,
		control,
		executedBy,
		occurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedHandoverDigest 是同一判断版本的内容比对锚：裁决、双方、证据、规则、依据与业务时间
// 任一不同即是另一份内容。
func unversionedHandoverDigest(command RegisterTransportHandoverCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Verdict),
		command.ReleasedBy,
		command.ReceivedBy,
		command.ReleasingEvidence,
		command.ReceivingEvidence,
		command.Rule,
		command.Basis,
		command.JudgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedJourneyDigest 是同一处置键下的内容比对锚：旅程身份、成员与开始时间任一不同即是
// 另一条旅程。成员先排序——提交顺序不构成不同的旅程。
func unversionedJourneyDigest(command StartAlternateJourneyCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Journey,
		fmt.Sprintf("%d", command.BasisKind),
		command.StartedAt.UTC().Format(time.RFC3339Nano),
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}
