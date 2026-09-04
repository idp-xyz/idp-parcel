package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ErrInvalidPublishedRecord 表示抓取所得公布记录立不住：没有字节、没有抓取时刻、没有来源
// 地址，或来源没有声明公布日期。
var ErrInvalidPublishedRecord = errors.New("parcel pricing: invalid published record")

// PublishedRecord 是连接器从来源抓回的一份公布记录原文，连同使它成为取值凭证的三件：内容
// 摘要、抓取时刻、来源地址（ADR-0099 决定六）。
//
// 摘要由构造器对字节自己算，不收外面给的：ADR-0092 要求「摘要在收到本体那一刻算」，做进构造器
// 之后调用方交不出一个与字节不符的摘要，也漏不掉——漏算就永久失去「我们收到过什么」的证据。
//
// 字节在这里只是过路：本体不进领域快照也不进业务库，交由工件存放端口，未配置时如实答未配置
// （ADR-0092 决定二）。凭证等级不依赖本体在不在——摘要、时刻、地址三件齐了就是 VERIFIABLE。
type PublishedRecord struct {
	body          []byte
	contentDigest string
	fetchedAt     time.Time
	sourceLocator string
	publishedOn   time.Time
}

// NewPublishedRecord 收字节、抓取时刻、来源地址与来源声明的公布日期。四件缺一即拒：空字节
// 没有可摘要的内容；缺时刻或地址的记录事后无从再次复核；缺公布日期的记录放不到时间线上。
func NewPublishedRecord(body []byte, fetchedAt time.Time, sourceLocator string, publishedOn time.Time) (PublishedRecord, error) {
	if len(body) == 0 || fetchedAt.IsZero() || !trimmed(sourceLocator) || publishedOn.IsZero() {
		return PublishedRecord{}, ErrInvalidPublishedRecord
	}
	sum := sha256.Sum256(body)
	return PublishedRecord{
		body:          append([]byte(nil), body...),
		contentDigest: "sha256:" + hex.EncodeToString(sum[:]),
		fetchedAt:     fetchedAt.UTC(),
		sourceLocator: sourceLocator,
		publishedOn:   publishedOn.UTC(),
	}, nil
}

// Body 交出原文字节的副本：记录一经构造不再变，摘要才始终对得上。
func (record PublishedRecord) Body() []byte { return append([]byte(nil), record.body...) }

// ContentDigest 是原文字节的 SHA-256，形如 `sha256:<hex>`。
func (record PublishedRecord) ContentDigest() string { return record.contentDigest }
func (record PublishedRecord) FetchedAt() time.Time  { return record.fetchedAt }
func (record PublishedRecord) SourceLocator() string { return record.sourceLocator }

// PublishedOn 是来源自己声明的公布日期，不是抓取日期：两者可以相差数日，版本号取前者。
func (record PublishedRecord) PublishedOn() time.Time { return record.publishedOn }

func (record PublishedRecord) valid() bool {
	return len(record.body) > 0 && record.contentDigest != "" && !record.fetchedAt.IsZero() &&
		trimmed(record.sourceLocator) && !record.publishedOn.IsZero()
}

// EvidenceReference 是这份记录作为一期取值凭证的写法：`sha256:… @ 抓取时刻 ← 来源地址`，本体
// 另有存放处时追加 ` → 存放定位符`。
//
// 定位符可缺（ADR-0092 决定二）：未配置存放时为空，不追加，摘要照旧在——凭证证明「收到过
// 什么」，定位符只回答「此刻在哪」，两者分开写才表达得出「收到了但没处放」。原地引用（存放
// 定位符就是来源地址）也不追加：同一个地址写两遍不多出任何信息。
func (record PublishedRecord) EvidenceReference(storageLocator string) string {
	reference := record.contentDigest + " @ " + record.fetchedAt.Format(time.RFC3339) + " ← " + record.sourceLocator
	if storageLocator != "" && storageLocator != record.sourceLocator {
		reference += " → " + storageLocator
	}
	return reference
}
