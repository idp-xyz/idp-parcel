import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { codedProblemLine, fieldProblemText, itemProblemText } from './field-problem-text';

// 原话取自 PC publication_draft_payload 与领域错误的现行写法；空表单点预览的那几条是会话在演示环境里实测到的。

test('留空：领域的 blank value 说成必填', () => {
  deepEqual(fieldProblemText('party commercial: blank value: commercial object ID'), {
    text: '必填，不能留空',
    original: 'party commercial: blank value: commercial object ID',
  });
});

test('时刻留空：解析空串说成必填', () => {
  const raw = '须为 RFC 3339 时刻：parsing time "" as "2006-01-02T15:04:05.999999999Z07:00": cannot parse "" as "2006"';
  equal(fieldProblemText(raw).text, '必填，不能留空');
});

test('时刻：Go 解析错误说成格式不对并给例子', () => {
  const raw = '须为 RFC 3339 时刻：parsing time "2026-13-01" as "2006-01-02T15:04:05.999999999Z07:00": month out of range';
  const result = fieldProblemText(raw);
  equal(result.text, '时间格式不对：请填完整时刻，例如 2026-10-01T00:00:00+08:00');
  equal(result.original, raw);
});

test('只填一项：主语照留，英文键名不进中文', () => {
  equal(fieldProblemText('额度须恰一格在场：limitMinor 或 limitRatioBasisPoints').text, '额度只能填其中一项');
  equal(fieldProblemText('适用对象须恰一格在场：serviceProduct 或 customerContract').text, '适用对象只能填其中一项');
  equal(fieldProblemText('约定须恰一格在场：policy 或 inapplicabilityBasis').text, '约定只能填其中一项');
});

test('不在集合：说出是哪一类不在可选范围', () => {
  equal(fieldProblemText('集合外的期限种类 "FOO"').text, '期限种类不在可选范围内');
  equal(
    fieldProblemText('集合外的人工复核指令 "X"（只收 REQUIRED / NOT_REQUIRED）').text,
    '人工复核指令不在可选范围内',
  );
});

test('须在场：去掉括注说成必须填写；主语是英文键名时不拿它当主语', () => {
  equal(fieldProblemText('规则包正文（五维适用性与规则表）须在场').text, '规则包正文必须填写');
  equal(
    fieldProblemText('closed 须在场：缺格读「未声明」（false）还是「不允许」（true）要登记方自己说，不给默认').text,
    '这一格必须填写',
  );
});

test('专门的句子各有说法', () => {
  equal(fieldProblemText('金额额度不带基数；ratioBase 只随 limitRatioBasisPoints 在场').text, '按金额设额度时，不要选比例的基数');
  equal(fieldProblemText('比例额度须声明其基数 ratioBase：一个比例没有分母不是业务判断依据').text, '按比例设额度时，必须选比例的基数');
  equal(fieldProblemText('时长须为正整数天，收到 0').text, '天数必须是正整数');
  equal(fieldProblemText('判断顺序须为从 1 起的正整数，收到 0').text, '判断顺序必须是从 1 开始的正整数');
  equal(fieldProblemText('计不计费要协议自己说，缺键不代说').text, '请选择是否计费');
  equal(fieldProblemText('空清单或同一原因两条：party commercial: duplicate x').text, '至少填一条，同一原因不能出现两次');
});

test('认不出的领域错误给通用一句，原话收起', () => {
  deepEqual(fieldProblemText('party commercial: invalid credit limit: something'), {
    text: '填写的内容不合要求',
    original: 'party commercial: invalid credit limit: something',
  });
  deepEqual(fieldProblemText('json: cannot unmarshal string into Go value of type int'), {
    text: '填写的内容不合要求',
    original: 'json: cannot unmarshal string into Go value of type int',
  });
});

test('认不出但本就是中文白话的原话照样显，不重复给原话', () => {
  deepEqual(fieldProblemText('这一版已经发布过'), { text: '这一版已经发布过', original: null });
});

test('前端本地校验的中文问题照原样显，夹着英文例子也不被当成原话', () => {
  deepEqual(fieldProblemText('时长须形如 P7D 或 P3DT12H'), { text: '时长须形如 P7D 或 P3DT12H', original: null });
});

test('清单逐项：下标说成第几项，原话连同路径收起', () => {
  deepEqual(itemProblemText('methods[0]', 'party commercial: blank value: delivery method'), {
    text: '第 1 项：必填，不能留空',
    original: 'methods[0]：party commercial: blank value: delivery method',
  });
  deepEqual(itemProblemText('materials[2]', '这一项重复了'), { text: '第 3 项：这一项重复了', original: null });
});

// Covers: AT-PC-070——认得的问题码按码译成中文，原话作原话那一格跟着走；译文与原话相同时不显第二遍。
test('按问题码译：码与参数给出中文，原话收进原话那一格', () => {
  const line = codedProblemLine({
    problem: '集合外的规则分类 "X"',
    code: 'NOT_IN_SET',
    params: { what: '规则分类', value: 'X' },
  });
  deepEqual(fieldProblemText(line), { text: '规则分类不在可选范围内', original: '集合外的规则分类 "X"' });
  equal(codedProblemLine({ problem: '必填，不能留空', code: 'REQUIRED' }), '必填，不能留空');
  equal(fieldProblemText(codedProblemLine({ problem: '必填，不能留空', code: 'REQUIRED' })).original, null);
});

// Covers: AT-PC-070——每个码的话术：参数缺省时用不带主语的说法；主语来自服务端参数，不靠原话的形状。
test('各问题码的话术', () => {
  const text = (code: string, params?: Record<string, string>) =>
    fieldProblemText(codedProblemLine({ problem: '原话', code, params })).text;
  equal(text('REQUIRED'), '必填，不能留空');
  equal(text('REQUIRED', { subject: '规则包正文' }), '规则包正文必须填写');
  equal(text('TIME_FORMAT'), '时间格式不对：请填完整时刻，例如 2026-10-01T00:00:00+08:00');
  equal(text('EXACTLY_ONE', { subject: '额度' }), '额度只能填其中一项');
  equal(text('EXACTLY_ONE'), '这几格只能填其中一项');
  equal(text('RATIO_BASE_NOT_ALLOWED'), '按金额设额度时，不要选比例的基数');
  equal(text('RATIO_BASE_REQUIRED'), '按比例设额度时，必须选比例的基数');
  equal(text('DAYS_NOT_POSITIVE', { value: '0' }), '天数必须是正整数');
  equal(text('ORDER_NOT_POSITIVE', { value: '0' }), '判断顺序必须是从 1 开始的正整数');
  equal(text('CHARGED_UNDECLARED'), '请选择是否计费');
  equal(text('LIST_EMPTY_OR_DUPLICATE'), '至少填一条，同一原因不能出现两次');
  equal(text('NOT_IN_SET'), '这一格不在可选范围内');
});

// Covers: AT-PC-070——没有码、兜底的 INVALID、前端不认得的新码都退回原话，再按原话的形状译；不会因为码不认得而吞掉问题。
test('码缺席、兜底或不认得时退回原话', () => {
  const raw = 'party commercial: blank value: commercial object ID';
  for (const code of [undefined, 'INVALID', 'SOMETHING_NEW']) {
    equal(codedProblemLine({ problem: raw, code }), raw);
  }
  equal(fieldProblemText(codedProblemLine({ problem: raw, code: 'SOMETHING_NEW' })).text, '必填，不能留空');
});

// Covers: AT-PC-070——itemProblemText 经带码的行照常给「第 n 项」前缀，原话连同路径收进原话那一格。
test('清单逐项的问题经带码的行仍带第 n 项前缀', () => {
  const line = codedProblemLine({ problem: '集合外的校验组 "G"', code: 'NOT_IN_SET', params: { what: '校验组', value: 'G' } });
  deepEqual(itemProblemText('applicableGroups[1]', line), {
    text: '第 2 项：校验组不在可选范围内',
    original: 'applicableGroups[1]：集合外的校验组 "G"',
  });
});
