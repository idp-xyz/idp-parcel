import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type {
  CommercialPublicationPayload,
  PublicationDraftPublicationResponseBody,
  PublicationDraftResponseBody,
  PublicationPreviewResponseBody,
} from './publication-draft-api';
import {
  currentAnswer,
  draftReferenceOf,
  emptyFlow,
  flowSteps,
  isFormLocked,
  nextAction,
  payloadKey,
  problemsByField,
  reachedStep,
  unclaimedProblems,
  withApproval,
  withPreview,
  withPublication,
  withSubmission,
} from './publication-draft-flow';

// 本文件钉的是五步状态机的边（票 admin-write-faces/16，公共半边）：
//   1. 每一步的答复只对**同一份载荷**（同键）算数；改任何一格键就变，下游答复整链作废；
//   2. 哪些 outcome 让流程前进、哪些停在原地，名字从 Go 的结果代数原词抄，一格不多不少；
//   3. 逐格问题按 JSON 路径归到格上，表单没认领的格单独交回，不静默丢。
// 状态机不算摘要、不裁任何门：它只读服务端答复的 outcome 原词做分派（伞票 07 硬句）。

function payload(over: Partial<CommercialPublicationPayload> = {}): CommercialPublicationPayload {
  return {
    kind: 'CREDIT_POLICY',
    objectId: 'CP-SYN-1',
    version: 'v1',
    scope: 'tenant',
    effectiveStartsAt: '2026-09-01T00:00:00Z',
    creditPolicy: {
      legalEntity: 'LE-1',
      authorityLevel: 'L1',
      chargeType: 'FREIGHT',
      limitMinor: 0,
      effectiveStartsAt: '2026-09-01T00:00:00Z',
    },
    ...over,
  };
}

function outcome<Body extends { outcome: string }>(body: Body): ApiResult<Body> {
  return { kind: 'outcome', status: 200, body };
}

const unconfigured: ApiResult<never> = { kind: 'unconfigured' };

const key = payloadKey(payload());

function previewed(): ApiResult<PublicationPreviewResponseBody> {
  return outcome({ outcome: 'PREVIEWED', canonicalization: 'PCC-1', contentDigest: 'PCC-1:abc' });
}

function drafted(name: string): ApiResult<PublicationDraftResponseBody> {
  return outcome({ outcome: name, canonicalization: 'PCC-1', contentDigest: 'PCC-1:abc', status: 'PENDING_APPROVAL' });
}

function published(name: string): ApiResult<PublicationDraftPublicationResponseBody> {
  return outcome({ outcome: name });
}

// Covers: 键与书写顺序无关、与任何一格内容有关——预览之后改一格，键就变，下游按钮才收得回。
test('载荷键与书写顺序无关、与任何一格内容有关', () => {
  const a = payload();
  const reordered: CommercialPublicationPayload = {
    creditPolicy: {
      effectiveStartsAt: '2026-09-01T00:00:00Z',
      limitMinor: 0,
      chargeType: 'FREIGHT',
      authorityLevel: 'L1',
      legalEntity: 'LE-1',
    },
    effectiveStartsAt: '2026-09-01T00:00:00Z',
    scope: 'tenant',
    version: 'v1',
    objectId: 'CP-SYN-1',
    kind: 'CREDIT_POLICY',
  };
  equal(payloadKey(a), payloadKey(reordered));
  notEqual(payloadKey(a), payloadKey(payload({ creditPolicy: { ...a.creditPolicy!, limitMinor: 1 } })));
  // 零金额与缺席不是同一份：「授予零信用」与「没填额度」键不同。
  const { limitMinor: _dropped, ...withoutLimit } = a.creditPolicy!;
  notEqual(payloadKey(a), payloadKey(payload({ creditPolicy: withoutLimit })));
});

// Covers: 批准口与发布口只收载体引用——身份三元，壳上其余各格与正文一概不带。
test('载体引用只取身份三元', () => {
  deepEqual(draftReferenceOf(payload()), { kind: 'CREDIT_POLICY', objectId: 'CP-SYN-1', version: 'v1' });
});

// Covers: 五步的顺序与空流程的起点。
test('五步按序，空流程停在填表', () => {
  deepEqual([...flowSteps], ['form', 'preview', 'submit', 'approve', 'publish']);
  equal(reachedStep(emptyFlow(), key), 'form');
  equal(nextAction('form'), 'preview');
  equal(nextAction('preview'), 'submit');
  equal(nextAction('submit'), 'approve');
  equal(nextAction('approve'), 'publish');
  equal(nextAction('publish'), null);
});

// Covers: 只有 PREVIEWED 让流程离开填表；不受理、未配置、键不同都停在填表。
test('预览只在 PREVIEWED 且同键时前进', () => {
  equal(reachedStep(withPreview(emptyFlow(), key, previewed()), key), 'preview');
  equal(reachedStep(withPreview(emptyFlow(), key, outcome({ outcome: 'NOT_ACCEPTED', cause: '没接的册' })), key), 'form');
  equal(reachedStep(withPreview(emptyFlow(), key, unconfigured), key), 'form');
  equal(reachedStep(withPreview(emptyFlow(), 'another-key', previewed()), key), 'form');
});

// Covers: 录入三格放行（本次落册 / 同份重放 / 待批准期间修订），内容已固定与不受理停在预览。
test('录入按 outcome 原词分派前进与停留', () => {
  const previewedFlow = withPreview(emptyFlow(), key, previewed());
  for (const name of ['DRAFT_SUBMITTED', 'DRAFT_REPLAYED', 'DRAFT_REVISED']) {
    equal(reachedStep(withSubmission(previewedFlow, key, drafted(name)), key), 'submit', name);
  }
  for (const name of ['CONTENT_FIXED', 'NOT_ACCEPTED']) {
    equal(reachedStep(withSubmission(previewedFlow, key, drafted(name)), key), 'preview', name);
  }
  equal(reachedStep(withSubmission(previewedFlow, key, unconfigured), key), 'preview');
});

// Covers: 批准两格放行；`已发布`直接到发布那一步（载体早已过了两格）；未配置、需换人、不合格、
// 不在册、被替换都停在存为待批准——它们的续办各不相同，状态机不替人挑，只把答复原样留在那一步。
test('批准按 outcome 原词分派：放行、越到发布、停留', () => {
  const submitted = withSubmission(withPreview(emptyFlow(), key, previewed()), key, drafted('DRAFT_SUBMITTED'));
  for (const name of ['DRAFT_APPROVED', 'DRAFT_ALREADY_APPROVED']) {
    equal(reachedStep(withApproval(submitted, key, drafted(name)), key), 'approve', name);
  }
  equal(reachedStep(withApproval(submitted, key, drafted('DRAFT_ALREADY_PUBLISHED')), key), 'publish');
  for (const name of ['NOT_CONFIGURED', 'NEEDS_ANOTHER_APPROVER', 'APPROVER_NOT_QUALIFIED', 'DRAFT_NOT_FOUND', 'DRAFT_CHANGED']) {
    equal(reachedStep(withApproval(submitted, key, drafted(name)), key), 'submit', name);
  }
});

// Covers: 发布两格放行；等待生效边界与发布未落定停在已批准——前者届期再来，后者照嵌回的用例答案办。
test('发布按 outcome 原词分派前进与停留', () => {
  const approved = withApproval(
    withSubmission(withPreview(emptyFlow(), key, previewed()), key, drafted('DRAFT_SUBMITTED')),
    key,
    drafted('DRAFT_APPROVED'),
  );
  for (const name of ['DRAFT_PUBLISHED', 'DRAFT_ALREADY_PUBLISHED']) {
    equal(reachedStep(withPublication(approved, key, published(name)), key), 'publish', name);
  }
  for (const name of ['DRAFT_AWAITS_EFFECTIVE_START', 'PUBLICATION_NOT_LANDED', 'DRAFT_NOT_APPROVED', 'DRAFT_NOT_FOUND']) {
    equal(reachedStep(withPublication(approved, key, published(name)), key), 'approve', name);
  }
});

// Covers: 一次新预览把下游整链清掉；录入清批准与发布；换键的预览不让旧链残留——载体已按新内容
// 就地修订过（DRAFT_REVISED），改回旧内容时旧的「已存为待批准」若还显着，说的就是一份不在册的东西。
test('每一步的答复覆盖下游，新预览清整链', () => {
  const full = withPublication(
    withApproval(
      withSubmission(withPreview(emptyFlow(), key, previewed()), key, drafted('DRAFT_SUBMITTED')),
      key,
      drafted('DRAFT_APPROVED'),
    ),
    key,
    published('DRAFT_PUBLISHED'),
  );
  equal(reachedStep(full, key), 'publish');

  const resubmitted = withSubmission(full, key, drafted('DRAFT_REVISED'));
  equal(resubmitted.approval, null);
  equal(resubmitted.publication, null);
  equal(reachedStep(resubmitted, key), 'submit');

  const otherKey = payloadKey(payload({ version: 'v2' }));
  const repreviewed = withPreview(full, otherKey, previewed());
  equal(repreviewed.submission, null);
  equal(repreviewed.approval, null);
  equal(repreviewed.publication, null);
  equal(reachedStep(repreviewed, otherKey), 'preview');
  equal(reachedStep(repreviewed, key), 'form');
});

// Covers: 答复只在同键时交回——键不同的旧摘要不显示，免得冒充眼前这一份的。
test('答复只在同键时交回', () => {
  const flow = withPreview(emptyFlow(), key, previewed());
  ok(currentAnswer(flow.preview, key) !== null);
  equal(currentAnswer(flow.preview, 'other'), null);
  equal(currentAnswer(null, key), null);
});

// Covers: 载体在册之后表单锁定（存为待批准 / 已批准 / 已发布）；填表与已预览可继续改。
test('载体在册后表单锁定', () => {
  equal(isFormLocked('form'), false);
  equal(isFormLocked('preview'), false);
  equal(isFormLocked('submit'), true);
  equal(isFormLocked('approve'), true);
  equal(isFormLocked('publish'), true);
});

// Covers: 逐格问题按 JSON 路径归组、同格多条保序；表单没认领的路径单独交回，不静默丢。
test('逐格问题按路径归组，未认领的单独交回', () => {
  const problems = [
    { field: 'creditPolicy.legalEntity', problem: '不得为空' },
    { field: 'creditPolicy.limit', problem: '额度须恰一格在场' },
    { field: 'creditPolicy.legalEntity', problem: '形状不对' },
    { field: 'references.SERVICE_PRODUCT', problem: '集合外' },
  ];
  const byField = problemsByField(problems);
  deepEqual(byField['creditPolicy.legalEntity'], ['不得为空', '形状不对']);
  deepEqual(byField['creditPolicy.limit'], ['额度须恰一格在场']);
  deepEqual(unclaimedProblems(byField, ['creditPolicy.legalEntity', 'creditPolicy.limit']), [
    { field: 'references.SERVICE_PRODUCT', problem: '集合外' },
  ]);
  deepEqual(problemsByField(undefined), {});
});
