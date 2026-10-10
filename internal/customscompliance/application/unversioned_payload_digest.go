package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件是各口在 CCC-1 之前的摘要算法，即「无版本」那一版。改动前入库的记录存的是它们算出的
// 不带前缀的摘要，存量不回写（ADR-0014），所以它们只用来跟这类已存摘要比（domain.CompareStoredDigest），
// 或拼出那一版的键去查旧行，不再写进新记录。函数体一个字节都不能改：改了，同内容的旧记录就会被答成
// 冲突，键上带指纹的两口还会把旧行读成「没有」再落一行。

// unversionedExternalResultDigest 是同一来源响应身份的内容比对锚：层、原始语义、声称版本、尝试
// 序号、范围与业务时间任一不同即是另一份内容；放行层再加放行三件——同一来源身份先说
// 全部放行后说部分放行是来源响应冲突，不是重放。不带放行三件时指纹与此前一字不变，
// 已入库的非放行层记录重放仍比得上。
func unversionedExternalResultDigest(command ReceiveExternalResultCommand) string {
	parts := []string{
		fmt.Sprintf("%d", command.Layer),
		command.RawSemantics,
		command.ClaimedVersion,
		fmt.Sprintf("%d", command.Attempt),
		command.Scope,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	if command.Release != nil {
		parts = append(parts,
			fmt.Sprintf("%d", command.Release.Kind),
			command.Release.Authority.String(),
			command.Release.Condition)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedDeclarationDigest 是同一逻辑申报目标的内容比对锚：组成、资料快照与角色快照任一
// 不同即是另一份内容。成员先排序——提交顺序不构成不同的内容。
func unversionedDeclarationDigest(command SubmitDeclarationCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Procedure,
		command.Dossier,
		command.Roles,
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedVerificationDigest 是核对内容的稳定指纹：三轴、关联依据、监管程序、资金事实版本。三维身份在键上，
// 不进指纹。拼接顺序是 Coverage、Delta、Validity、Basis、Procedure、FundsVersion，以 \x00 分隔后 sha256；程序与
// 资金版本两维是在存量为零时追在末尾的（票 sa-cc/22 裁决 2、sa-cc/19 裁决 3），所以不存在缺这两维的旧行。它是
// 0016 主键 `version_digest` 的一列，也是信封 ID 的一段。
func unversionedVerificationDigest(command VerifyDutyPaymentCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		strconv.Itoa(int(command.Coverage)),
		strconv.Itoa(int(command.Delta)),
		strconv.Itoa(int(command.Validity)),
		strings.TrimSpace(command.Basis),
		strings.TrimSpace(command.Procedure.String()),
		strings.TrimSpace(command.FundsVersion.String()),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedFactSetDigest 是事实集的稳定指纹：逐事实引用排序后拼接——同一集合无论装载顺序
// 如何指纹恒定，新事实到达自然换指纹。
func unversionedFactSetDigest(facts []domain.ExecutionFact) string {
	references := make([]string, 0, len(facts))
	for _, fact := range facts {
		references = append(references, fact.Fact().String())
	}
	sort.Strings(references)
	digest := sha256.Sum256([]byte(strings.Join(references, "\x00")))
	return hex.EncodeToString(digest[:])
}
