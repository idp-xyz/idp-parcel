package domain

import (
	"errors"
	"sort"
	"strings"
)

// 本文件是关务事实出处的领域词形（票 routing-first-cut/12，ADR-0148 决定一「端口取回
// 的事实带出处、与判断一并留痕」）：逐候选一条出处——customs-compliance 铸成的可重算
// 判断标识，加上它当时所依的口岸与申报路径目录版本引用。出处随证据取回、随判断记录
// 落库；它只是引用，不含判断内容本身，复核者按标识与版本重算得回当初那一份作答。

var errInvalidCustomsCitation = errors.New("network routing: invalid customs applicability citation")

// CustomsApplicabilityCitationSpec 是构造一条关务出处的全部输入。
type CustomsApplicabilityCitationSpec struct {
	Candidate CandidateID
	Judgment  string
	Versions  []string
}

// CustomsApplicabilityCitation 是逐候选的关务出处。Judgment 来自 CC 按
// 租户+时点+候选+目录版本铸成的判断标识；Versions 是那一版目录的版本引用（懂
// PORT:/PATH: 那族的写成几格是 CC 的事，这里照引用收）。
type CustomsApplicabilityCitation struct {
	candidate CandidateID
	judgment  string
	versions  []string
}

// NewCustomsApplicabilityCitation 校验并装配：候选必须成形、判断标识非空白、版本引用
// 逐条非空白且排序去重——出处是复核找回原判断的索引，坏一格都会把复核引到错地方。
func NewCustomsApplicabilityCitation(spec CustomsApplicabilityCitationSpec) (CustomsApplicabilityCitation, error) {
	if !spec.Candidate.valid() {
		return CustomsApplicabilityCitation{}, errInvalidCustomsCitation
	}
	if strings.TrimSpace(spec.Judgment) == "" {
		return CustomsApplicabilityCitation{}, errInvalidCustomsCitation
	}
	versions := append([]string(nil), spec.Versions...)
	for _, version := range versions {
		if strings.TrimSpace(version) == "" {
			return CustomsApplicabilityCitation{}, errInvalidCustomsCitation
		}
	}
	sort.Strings(versions)
	unique := versions[:0]
	for index, version := range versions {
		if index == 0 || version != versions[index-1] {
			unique = append(unique, version)
		}
	}
	return CustomsApplicabilityCitation{
		candidate: spec.Candidate,
		judgment:  spec.Judgment,
		versions:  unique,
	}, nil
}

func (citation CustomsApplicabilityCitation) Candidate() CandidateID {
	return citation.candidate
}

func (citation CustomsApplicabilityCitation) Judgment() string {
	return citation.judgment
}

func (citation CustomsApplicabilityCitation) Versions() []string {
	return append([]string(nil), citation.versions...)
}
