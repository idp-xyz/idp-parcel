package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"time"
)

// publicationShellDigestVersion 标出壳摘要的算法版本；摘要串是「版本:十六进制」，自带版本，同内容摘要的写法。
const publicationShellDigestVersion = "PCS-1"

// ShellDigest 是载体版本壳的摘要：适用范围、有效区间与指名引用。内容摘要只盖正文，不盖这几格（见
// CanonicalizePublicationContent），而待批准期间壳变了正文没变也是合法的就地修订（SameSubmissionAs）——批准与发布要绑定
// 调用方看到的东西，就得连壳一并绑，否则内容不变、范围或区间被换掉的载体会照样批过去。
//
// 身份四元不入：它是键，键不同的两份不会撞在一起。时刻取微秒并按 UTC 写：载体册以微秒存时刻，录入答复里算出的串
// 与之后读回算出的串必须相等。引用按载体上记下的顺序写，与 SameSubmissionAs 比对引用的口径一致。
func (draft PublicationDraft) ShellDigest() string {
	hasher := sha256.New()
	writeShellField(hasher, draft.shell.scope.String())
	writeShellField(hasher, shellInstant(draft.shell.effective.StartsAt()))
	if end, bounded := draft.shell.effective.EndsAt(); bounded {
		writeShellField(hasher, shellInstant(end))
	} else {
		writeShellField(hasher, "")
	}
	for _, reference := range draft.shell.references {
		writeShellField(hasher, reference.Kind().String())
		writeShellField(hasher, reference.ObjectID().String())
	}
	return publicationShellDigestVersion + ":" + hex.EncodeToString(hasher.Sum(nil))
}

func shellInstant(instant time.Time) string {
	return instant.UTC().Round(time.Microsecond).Format(time.RFC3339Nano)
}

// writeShellField 带长度前缀写一格，两格相邻拼接不会撞出同一串。
func writeShellField(hasher hash.Hash, value string) {
	_, _ = fmt.Fprintf(hasher, "%d:%s;", len(value), value)
}
