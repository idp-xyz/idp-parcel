// Package referenceconfig 拥有随产品版本发布的参考配置（ADR-0147）：判断方法与公开标准的实例数据，
// 租户显式采用才生效（ADR-0146 决定三）。
//
// 放在仓库根而不是某个上下文包里，理由同 migrations：它是随产品版本发布的数据，`//go:embed` 取不到
// 包目录之外的文件；不可改写的发布清单与引用串的解析也因此只有一处。本包只管「有哪几版、原文是什么、
// 依据格里怎么引」，原文的形状与领域构造门归采用它的那本登记册的入口。
package referenceconfig

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 嵌入清单列上下文目录；新上下文的首份参考配置与本行、发布清单同一笔提交落——引用一个尚无文件的
// 目录会让干净检出编译不过，与 migrations 的嵌入行同一条纪律。
//
//go:embed all:party-commercial
var assets embed.FS

var (
	ErrInvalidReference    = errors.New("reference configuration: invalid reference")
	ErrNotReleased         = errors.New("reference configuration: not released")
	ErrAlteredAfterRelease = errors.New("reference configuration: content differs from its released digest")
)

// citationShapeVersion 是引用串自身的形状版本（ADR-0147 决定三），与引用之间以冒号相接——写法同摘要串的
// `PSC-1`、`PCC-1`。串里要多带一格时换 REFCFG-2，旧串照旧可读。
const citationShapeVersion = "REFCFG-1"

// 标识恰三段：上下文 / 目录 / 键。上下文与目录是小写短横线名；键取该目录的自然键（如国家 / 地区码），
// 允许大写。
var identifierShape = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*/[a-z0-9]+(?:-[a-z0-9]+)*/[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$`)

// 版本从 1 起、不带前导零：同一版只有一种写法，依据格里才不会出现两串指同一份原文。
var versionShape = regexp.MustCompile(`^[1-9][0-9]*$`)

type release struct {
	identifier string
	version    int
	digest     string
}

// releases 是发布清单：每一版原文的 sha256。进 main 即发布，此后那份文件一字不改；更正或扩充另立
// 新版本，并在此加一行。
var releases = []release{
	{"party-commercial/registration-number-types/CN", 1, "46087f2adfc18dc277d4dc534c2254663de5e13f8415cc8b180a553cb0fa300c"},
	{"party-commercial/registration-number-types/SG", 1, "928d06b9ee6b96ded103aad5bc636c5e40e408a2620e67b026811cd0bb96767c"},
}

// Reference 指名一份参考配置的一个版本。
type Reference struct {
	identifier string
	version    int
}

func NewReference(identifier string, version int) (Reference, error) {
	if !identifierShape.MatchString(identifier) || version < 1 {
		return Reference{}, fmt.Errorf("%w: %q@%d", ErrInvalidReference, identifier, version)
	}
	return Reference{identifier: identifier, version: version}, nil
}

// ParseReference 读「标识@版本」。
func ParseReference(text string) (Reference, error) {
	identifier, version, found := strings.Cut(text, "@")
	if !found || !versionShape.MatchString(version) {
		return Reference{}, fmt.Errorf("%w: %q", ErrInvalidReference, text)
	}
	number, err := strconv.Atoi(version)
	if err != nil {
		return Reference{}, fmt.Errorf("%w: %q", ErrInvalidReference, text)
	}
	return NewReference(identifier, number)
}

// ParseCitation 从依据格反解引用串。不带前缀或形状不对的串答 false：依据格里别的依据照旧是别的依据，
// 不当成一次坏掉的采用。
func ParseCitation(text string) (Reference, bool) {
	rest, found := strings.CutPrefix(text, citationShapeVersion+":")
	if !found {
		return Reference{}, false
	}
	reference, err := ParseReference(rest)
	if err != nil {
		return Reference{}, false
	}
	return reference, true
}

func (reference Reference) Identifier() string {
	return reference.identifier
}

func (reference Reference) Version() int {
	return reference.version
}

func (reference Reference) String() string {
	return reference.identifier + "@" + strconv.Itoa(reference.version)
}

// Citation 交回写进登记依据格的引用串。
func (reference Reference) Citation() string {
	return citationShapeVersion + ":" + reference.String()
}

// Open 交回一份已发布参考配置的原文。未发布答 ErrNotReleased；原文与发布时的摘要不符答
// ErrAlteredAfterRelease——已采用的租户引的是发布时那一版，改过的原文不能冒它的名。
func Open(reference Reference) ([]byte, error) {
	for _, entry := range releases {
		if entry.identifier != reference.identifier || entry.version != reference.version {
			continue
		}
		raw, err := assets.ReadFile(reference.String() + ".json")
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrNotReleased, reference, err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != entry.digest {
			return nil, fmt.Errorf("%w: %s", ErrAlteredAfterRelease, reference)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotReleased, reference)
}

// Released 按标识、版本列出全部已发布版本。
func Released() []Reference {
	references := make([]Reference, 0, len(releases))
	for _, entry := range releases {
		references = append(references, Reference{identifier: entry.identifier, version: entry.version})
	}
	sort.Slice(references, func(i, j int) bool {
		if references[i].identifier != references[j].identifier {
			return references[i].identifier < references[j].identifier
		}
		return references[i].version < references[j].version
	})
	return references
}
