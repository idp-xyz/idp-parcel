package cataloguepage

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// staleCursorReason 是 ADR-0144 决定一点名的理由散文：游标解不开，或摘要与本次的排序 / 筛选不符，
// 都说这一句——两种情形下操作者该做的是同一件事。
const staleCursorReason = "游标与本次的排序或筛选不符，请从第一页重取"

// Position 是一行在「排序维值 + 标识」上的位置（ADR-0144 决定一）。各格按本册声明的 ValueKind 规整成串：
// 读面编游标时用 FormatInteger / FormatInstant 写，取回时用 ParseInteger / ParseInstant 读。
type Position struct {
	Value    string
	Identity []string
}

// cursorBody 是游标解开后的内容：本次的排序、末行在排序维上的值与标识，以及本次条件的摘要。
type cursorBody struct {
	Sort     string   `json:"s"`
	Value    string   `json:"v"`
	Identity []string `json:"i"`
	Digest   string   `json:"d"`
}

// CursorAfter 把本页末行的位置编成下一页的游标。位置不合本册声明是读面的编程错误，照实报出来，
// 不编出一个下次解不开的游标。
func (query Query) CursorAfter(last Position) (string, error) {
	if query.catalogue == nil {
		return "", errors.New("cataloguepage: 查询对象不是 Decode 解出来的，编不出游标")
	}
	if err := query.catalogue.checkPosition(query.Sort, last); err != nil {
		return "", fmt.Errorf("cataloguepage: 末行位置不合册 %s 的声明：%w", query.catalogue.name, err)
	}
	body, err := json.Marshal(cursorBody{
		Sort:     query.Sort.String(),
		Value:    last.Value,
		Identity: last.Identity,
		Digest:   query.digest,
	})
	if err != nil {
		return "", fmt.Errorf("cataloguepage: 编游标：%w", err)
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func (catalogue *Catalogue) decodeCursor(encoded string, query Query) (Position, error) {
	stale := &MalformedQuery{Reason: staleCursorReason}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Position{}, stale
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var body cursorBody
	if decoder.Decode(&body) != nil || decoder.More() {
		return Position{}, stale
	}
	if body.Digest != query.digest || body.Sort != query.Sort.String() {
		return Position{}, stale
	}
	position := Position{Value: body.Value, Identity: body.Identity}
	if catalogue.checkPosition(query.Sort, position) != nil {
		return Position{}, stale
	}
	return position, nil
}

func (catalogue *Catalogue) checkPosition(sort Sort, position Position) error {
	if err := checkValue(catalogue.sorts[sort.Field], position.Value); err != nil {
		return fmt.Errorf("排序维 %s：%w", sort.Field, err)
	}
	if len(position.Identity) != len(catalogue.identity) {
		return fmt.Errorf("标识应有 %d 段，实有 %d 段", len(catalogue.identity), len(position.Identity))
	}
	for index, kind := range catalogue.identity {
		if err := checkValue(kind, position.Identity[index]); err != nil {
			return fmt.Errorf("标识第 %d 段：%w", index+1, err)
		}
	}
	return nil
}

func checkValue(kind ValueKind, value string) error {
	var err error
	switch kind {
	case Text:
	case Integer:
		_, err = ParseInteger(value)
	case Instant:
		_, err = ParseInstant(value)
	default:
		err = fmt.Errorf("值形状 %q 未知", kind)
	}
	return err
}

// digest 是本次条件（册名、选择器、排序、筛选、q）的摘要。各格按长度前缀写进散列，任何两组不同的条件
// 写出的字节都不同；筛选与选择器已由 canonical 规整，同一组条件只有一种写法。
func (catalogue *Catalogue) digest(query Query, selectors map[string][]string) string {
	hash := sha256.New()
	field := func(value string) {
		fmt.Fprintf(hash, "%d:%s", len(value), value)
	}
	group := func(groups map[string][]string) {
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		field(strconv.Itoa(len(keys)))
		for _, key := range keys {
			field(key)
			field(strconv.Itoa(len(groups[key])))
			for _, value := range groups[key] {
				field(value)
			}
		}
	}
	field(catalogue.name)
	group(selectors)
	field(query.Sort.String())
	group(query.Filters)
	field(query.Keyword)
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil)[:16])
}

// FormatInteger 是整数在游标里的写法：十进制，无正号、无前导零。
func FormatInteger(value int64) string {
	return strconv.FormatInt(value, 10)
}

// ParseInteger 只认 FormatInteger 的写法：同一个值只有一种写法，往返才稳，伪造的变体也进不来。
func ParseInteger(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || FormatInteger(parsed) != value {
		return 0, fmt.Errorf("%q 不是规整的十进制整数", value)
	}
	return parsed, nil
}

// FormatInstant 是时刻在游标里的写法：UTC 的 RFC 3339，秒以下按需。
func FormatInstant(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

// ParseInstant 只认 FormatInstant 的写法；带时区偏移或补了尾零的写法一律拒，理由同 ParseInteger。
func ParseInstant(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || FormatInstant(parsed) != value {
		return time.Time{}, fmt.Errorf("%q 不是规整的 UTC RFC 3339 时刻", value)
	}
	return parsed.UTC(), nil
}
