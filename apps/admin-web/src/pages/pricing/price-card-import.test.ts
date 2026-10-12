import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import type { ModuleInfo } from '../../navigation';
import { priceCardUploadField, priceCardUploadForm } from './api';
import {
  draftActionNoteOf,
  draftActionOf,
  draftListViewState,
  draftReferencePayload,
  draftSubmittable,
  priceCardApprovalOutcomeLabels,
  priceCardPublicationOutcomeLabels,
  sortPriceCardProblems,
  withProgressedDraft,
} from './price-card-import';
import type { PriceCardDraftRow, PriceCardProblem } from './api';

function problem(over: Partial<PriceCardProblem>): PriceCardProblem {
  return { sheet: 'card', row: 1, column: 'planId', code: 'BLANK', message: '空', ...over };
}

test('逐格问题按表、行、列排，同一格保持原顺序', () => {
  const sorted = sortPriceCardProblems([
    problem({ sheet: 'rates', row: 3, column: 'b', code: 'SECOND', message: '后' }),
    problem({ sheet: 'card', row: 2, column: 'a', code: 'CARD', message: '卡' }),
    problem({ sheet: 'rates', row: 3, column: 'a', code: 'COL', message: '列' }),
    problem({ sheet: 'rates', row: 3, column: 'b', code: 'FIRST', message: '先' }),
  ]);
  deepEqual(
    sorted.map((item) => item.code),
    ['CARD', 'COL', 'SECOND', 'FIRST'],
  );
});

test('上传表单只有一个 file 格，文件名原样带上', () => {
  const form = priceCardUploadForm(new Blob(['bytes']), 'card.xlsx');
  equal([...form.keys()].join(','), priceCardUploadField);
  const file = form.get(priceCardUploadField);
  okFile(file);
  equal(file.name, 'card.xlsx');
});

test('存草稿只在这一份文件的预览形成答案之后才开，未配置与传输失败不算看过预览', () => {
  equal(draftSubmittable(null), false);
  equal(draftSubmittable({ kind: 'unconfigured' }), false);
  equal(draftSubmittable({ kind: 'transport', message: '请求未到达' }), false);
  equal(draftSubmittable({ kind: 'noAnswer', status: 503, code: 'STORE_UNAVAILABLE' }), false);
  equal(draftSubmittable({ kind: 'outcome', status: 200, body: { outcome: 'HAS_PROBLEMS' } }), true);
});

function okFile(value: FormDataEntryValue | null): asserts value is File {
  if (!(value instanceof File)) {
    throw new Error('file 格不是文件');
  }
}

// ---- 草稿签（票 price-card-import/06）----

// 期望的答案原名取自 internal/parcelpricing/application 的 ApprovePriceCardDraftOutcome 与 PublishPriceCardDraftOutcome
// ——票 04 判断项 3、4 写明这些名字已随两口上线、本签逐格接——不从本模块的词表反推。
const approvalOutcomes = [
  'DRAFT_APPROVED',
  'DRAFT_NOT_FOUND',
  'NOT_CONFIGURED',
  'NEEDS_ANOTHER_APPROVER',
  'APPROVER_NOT_QUALIFIED',
  'DRAFT_NOT_VALIDATED',
  'DRAFT_ALREADY_APPROVED',
  'DRAFT_ALREADY_PUBLISHED',
  'DRAFT_CHANGED',
];
const publicationOutcomes = [
  'DRAFT_PUBLISHED',
  'DRAFT_NOT_FOUND',
  'DRAFT_NOT_APPROVED',
  'DRAFT_ALREADY_PUBLISHED',
  'PUBLICATION_NOT_LANDED',
];

function draftRow(over: Partial<PriceCardDraftRow>): PriceCardDraftRow {
  return {
    plan: { id: 'SYN-PLAN-01', version: 'v1' },
    status: 'VALIDATED',
    sourceFile: { name: 'card.xlsx', sha256: 'a'.repeat(64) },
    submitter: 'https://id.syn.example/dex#SYN-OPERATOR-01',
    submittedAt: '2026-10-09T03:00:00Z',
    problems: [],
    ...over,
  };
}

const pricingModule: ModuleInfo = { title: '价卡目录', owner: '小包计价（parcel-pricing）', source: '价卡出处' };

// Covers: 两口的答案代数逐格有中文，与服务端原名一一对上，不多不少。
test('批准与发布两口的每一格答案都有中文，不多不少', () => {
  deepEqual(Object.keys(priceCardApprovalOutcomeLabels).sort(), [...approvalOutcomes].sort());
  deepEqual(Object.keys(priceCardPublicationOutcomeLabels).sort(), [...publicationOutcomes].sort());
});

// Covers: 要做的 2「动作按钮只在对应状态出现」。
test('动作只在对应状态出现：已校验给批准，已批准给发布，草稿、已发布与没见过的状态不给', () => {
  equal(draftActionOf('VALIDATED'), 'approve');
  equal(draftActionOf('APPROVED'), 'publish');
  equal(draftActionOf('DRAFT'), null);
  equal(draftActionOf('PUBLISHED'), null);
  equal(draftActionOf('SOMETHING_NEW'), null);
});

// Covers: 两口载荷只收草稿引用（票 04 第 5 条）；身份在信封里，夹带身份格服务端即拒。
test('两口的载荷只有草稿引用，不带任何身份格', () => {
  deepEqual(draftReferencePayload({ id: 'SYN-PLAN-01', version: 'v2' }), { planId: 'SYN-PLAN-01', planVersion: 'v2' });
});

// Covers: 推进成功带回的行就地换上；负向答案、渠道那一格不改表。
test('推进成功带回的那一行就地换上，其余行与没带行的答案都不改表', () => {
  const validated = draftRow({});
  const other = draftRow({ plan: { id: 'SYN-PLAN-02', version: 'v1' } });
  const approved = draftRow({
    status: 'APPROVED',
    approver: 'https://id.syn.example/dex#SYN-OPERATOR-02',
    approvedAt: '2026-10-09T04:00:00Z',
  });
  deepEqual(
    withProgressedDraft([validated, other], { kind: 'outcome', status: 200, body: { outcome: 'DRAFT_APPROVED', draft: approved } }),
    [approved, other],
  );
  deepEqual(
    withProgressedDraft([validated, other], { kind: 'outcome', status: 200, body: { outcome: 'NEEDS_ANOTHER_APPROVER' } }),
    [validated, other],
  );
  deepEqual(withProgressedDraft([validated, other], { kind: 'notGranted' }), [validated, other]);
});

// Covers: 要做的 2「未配置的审批职责规则、主体相同、授予不足」各是答案，原名与中文并列；没收录的原名过线。
test('批准门的负向各格如实呈现为答案：审批职责规则未配置、主体相同、授予不足', () => {
  const note = (outcome: string) => draftActionNoteOf('approve', { kind: 'outcome', status: 200, body: { outcome } });
  const notConfigured = note('NOT_CONFIGURED');
  equal(notConfigured.tone, 'answered');
  match(notConfigured.text, /^批准（HTTP 200）：NOT_CONFIGURED —— 审批职责规则未配置/);
  match(note('NEEDS_ANOTHER_APPROVER').text, /NEEDS_ANOTHER_APPROVER —— 主体相同/);
  match(note('APPROVER_NOT_QUALIFIED').text, /APPROVER_NOT_QUALIFIED —— 授予不足/);
  match(note('SOMETHING_NEW').text, /SOMETHING_NEW —— SOMETHING_NEW$/);
});

// Covers: 要做的 2「登记用例的各格答复」——发布交给了登记时，登记那一格原名与中文接在后面；没交给登记就不接。
test('发布交给了登记时，登记那一格的原名与中文接在发布答案之后', () => {
  const landed = draftActionNoteOf('publish', {
    kind: 'outcome',
    status: 201,
    body: { outcome: 'DRAFT_PUBLISHED', registration: 'RECORDED' },
  });
  equal(landed.text, '发布（HTTP 201）：DRAFT_PUBLISHED —— 已发布（登记落定，草稿转已发布）；登记：RECORDED —— 已入册');
  const conflict = draftActionNoteOf('publish', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'PUBLICATION_NOT_LANDED', registration: 'CONTENT_CONFLICT' },
  });
  match(conflict.text, /PUBLICATION_NOT_LANDED —— 登记没有落定.*；登记：CONTENT_CONFLICT —— 版本内容冲突/);
  const notApproved = draftActionNoteOf('publish', { kind: 'outcome', status: 200, body: { outcome: 'DRAFT_NOT_APPROVED' } });
  ok(!/登记：/.test(notApproved.text));
});

// Covers: 渠道那一格三态与故障各说各的；发布没形成答案时登记与否未知。
test('渠道那一格与故障各说各的：未配置、需要登录、没有授予、未形成答案', () => {
  const unconfigured = draftActionNoteOf('approve', { kind: 'unconfigured' });
  equal(unconfigured.tone, 'unconfigured');
  match(unconfigured.text, /403 ACCESS_CHANNEL_NOT_CONFIGURED/);
  const login = draftActionNoteOf('publish', { kind: 'loginRequired' });
  ok(login.tone === 'refused' && login.kind === 'loginRequired');
  match(login.text, /^发布：需要登录（401 OPERATOR_CREDENTIAL_REJECTED）。/);
  const grant = draftActionNoteOf('approve', { kind: 'notGranted' });
  ok(grant.tone === 'refused' && grant.kind === 'notGranted');
  const noAnswer = draftActionNoteOf('publish', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  equal(noAnswer.tone, 'problem');
  match(noAnswer.text, /登记与否未知/);
  ok(!/登记与否未知/.test(draftActionNoteOf('approve', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }).text));
});

// Covers: 要做的 3「四态如实，同 05」——空册是空态；未配置说的是草稿查阅读口、不是册为空；渠道另两态各成一态。
test('草稿册的页面态：空册是空态，未配置说的是草稿查阅读口而不是册为空，需要登录与没有授予各成一态', () => {
  const retry = () => {};
  equal(draftListViewState(null, retry, pricingModule).kind, 'loading');
  const listed = (drafts: PriceCardDraftRow[]) =>
    draftListViewState({ kind: 'outcome', status: 200, body: { outcome: 'PRICE_CARD_DRAFTS_LISTED', drafts } }, retry, pricingModule);
  equal(listed([]).kind, 'empty');
  equal(listed([draftRow({})]).kind, 'ready');
  const unconfigured = draftListViewState({ kind: 'unconfigured' }, retry, pricingModule);
  ok(unconfigured.kind === 'unconfigured');
  match(unconfigured.description ?? '', /GET \/pricing-price-card-draft-views/);
  match(unconfigured.description ?? '', /不是「册上没有草稿」/);
  equal(draftListViewState({ kind: 'loginRequired' }, retry, pricingModule).kind, 'loginRequired');
  equal(draftListViewState({ kind: 'notGranted' }, retry, pricingModule).kind, 'notGranted');
});

// Covers: 要做的 3「四态如实」——故障格说的是草稿册读不回，不把它叫成目录（草稿不进价卡目录）；别的码照目录页原样。
test('草稿册没形成答案时说的是草稿册，不是目录，仍可重试', () => {
  const retry = () => {};
  const failed = draftListViewState({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, retry, pricingModule);
  ok(failed.kind === 'error');
  match(failed.description ?? '', /草稿册读不回/);
  ok(!/目录/.test(failed.description ?? ''));
  equal(failed.onRetry, retry);
  const intake = draftListViewState({ kind: 'noAnswer', status: 500, code: 'INTAKE_FAILED' }, retry, pricingModule);
  ok(intake.kind === 'error');
  match(intake.description ?? '', /没有读草稿册/);
  ok(!/目录/.test(intake.description ?? ''));
  const identity = draftListViewState({ kind: 'noAnswer', status: 503, code: 'IDENTITY_DEPENDENCY_UNAVAILABLE' }, retry, pricingModule);
  ok(identity.kind === 'error');
  equal(identity.description, '服务端问题码：IDENTITY_DEPENDENCY_UNAVAILABLE');
});
