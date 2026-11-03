import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import type { GroupLegalEntityRecord, RegistrationNumberTypeRecord } from './api';
import {
  companionPartyDraftOf,
  companionPartyIdOf,
  effectivePartyIdOf,
  emptyLegalEntityDraft,
  identityCountryOptions,
  identityTypeOptions,
  legalEntityDraftUnsaved,
  legalEntityFieldPaths,
  legalEntityLocalProblems,
  legalEntityPayloadOf,
  pairedSavePlan,
  partyStepLanded,
  runPairedSave,
  suggestedRevision,
  withCompanionRegistered,
  type LegalEntityDraft,
} from './legal-entity-form';

// 本文件钉的是表单只做编码层的事（伞票 admin-write-faces/07 硬句）：修订号编成整数、墙钟时刻换成
// RFC 3339、可缺键缺席；空串、引用在不在册、修订连不连续，一律原样送上去让服务端答。载荷镜像受控 CLI
// register-parties 的 legalEntities 一项且**不带 tenantId**——在线 Intake 从认证结果取租户，载荷里带上
// 就是自报（register_party_identity.go 包注释）。
// 建档一体化那一族（票 company-subject-ux/02）钉的是两步串行的次序与停止条件，不钉任何一步答复的领域含义。

// 基线选的是已有档案：法人钉的参与方就是 partyId 那一格，与建档一体化之前的载荷同形。一并登记的用例各自改 partyMode。
function draft(over: Partial<LegalEntityDraft>): LegalEntityDraft {
  return {
    ...emptyLegalEntityDraft(),
    legalEntityId: 'SYN-LE-02',
    partyId: 'SYN-PARTY-SHIPPER-01',
    partyMode: 'existing',
    revision: '1',
    basis: 'SYN-REG-BASIS-LE-02',
    effectiveFrom: '2026-01-02T08:00',
    ...over,
  };
}

// Covers: 五格齐 → legalEntities 一项；revision 是 JSON 整数；effectiveFrom 按时区换成 UTC；顶层没有 tenantId。
test('草稿组成载荷：一项、整数修订、UTC 时刻、无租户格', () => {
  deepEqual(legalEntityPayloadOf(draft({}), 'Asia/Shanghai'), {
    legalEntities: [
      {
        legalEntityId: 'SYN-LE-02',
        partyId: 'SYN-PARTY-SHIPPER-01',
        revision: 1,
        basis: 'SYN-REG-BASIS-LE-02',
        effectiveFrom: '2026-01-02T00:00:00Z',
      },
    ],
  });
});

// Covers: 各格首尾空白去掉；空串照送（那是服务端要点名的格，不是本地要判的）；生效时刻留空则键缺席。
test('空白去掉、空串照送、生效时刻留空缺席', () => {
  const payload = legalEntityPayloadOf(
    draft({ legalEntityId: '  SYN-LE-02 ', partyId: '', basis: ' b ', effectiveFrom: '' }),
    'UTC',
  );
  deepEqual(payload.legalEntities[0], {
    legalEntityId: 'SYN-LE-02',
    partyId: '',
    revision: 1,
    basis: 'b',
  });
});

// Covers: 修订号编不进整数（空、小数、负、非数字）是编码层问题，本地报在它的路径上；能编进就不报；
// 时刻换不出来同样报。问题清零之前不该送。
test('本地只报编码层问题', () => {
  deepEqual(legalEntityLocalProblems(draft({}), 'UTC'), {});
  deepEqual(legalEntityLocalProblems(draft({ revision: '' }), 'UTC'), {
    'legalEntities[0].revision': ['修订号要填正整数'],
  });
  deepEqual(legalEntityLocalProblems(draft({ revision: '1.5' }), 'UTC'), {
    'legalEntities[0].revision': ['修订号要填正整数'],
  });
  deepEqual(legalEntityLocalProblems(draft({ revision: '0' }), 'UTC'), {
    'legalEntities[0].revision': ['修订号要填正整数'],
  });
  deepEqual(legalEntityLocalProblems(draft({ effectiveFrom: 'yesterday' }), 'UTC'), {
    'legalEntities[0].effectiveFrom': ['生效时刻要是可解析的日期时间'],
  });
});

// Covers: 修订号编不进整数时载荷不造假数——那一格缺席，由问题表拦住不送。
test('修订号编不进整数时载荷缺席该格', () => {
  const payload = legalEntityPayloadOf(draft({ revision: 'x' }), 'UTC');
  equal('revision' in payload.legalEntities[0], false);
});

// Covers: 建议修订号——标识在已取回列表里 → 最新修订 + 1；不在 → 1；标识空白 → 1。它只是建议，服务端仍判连续。
test('建议修订号取列表最新修订加一，不在册为一', () => {
  const rows: GroupLegalEntityRecord[] = [
    {
      tenantId: 'SYN-TENANT-01',
      legalEntityId: 'SYN-LE-01',
      kind: 'RESPONSIBLE_LEGAL_ENTITY',
      partyId: 'SYN-PARTY-OPERATOR-01',
      partyNameKnown: true,
      partyName: 'x',
      status: 'EFFECTIVE',
      revision: 3,
      basis: 'b',
      effectiveFrom: '2026-01-02T00:00:00Z',
      registeredAt: '2026-01-02T00:00:00Z',
      identityLayerRegistered: false,
    },
  ];
  equal(suggestedRevision(rows, 'SYN-LE-01'), 4);
  equal(suggestedRevision(rows, ' SYN-LE-01 '), 4);
  equal(suggestedRevision(rows, 'SYN-LE-99'), 1);
  equal(suggestedRevision(rows, ''), 1);
  equal(suggestedRevision(null, 'SYN-LE-01'), 1);
});

// Covers: 表单认领的路径表与载荷键一一对应——服务端点名到这些路径的问题各有一格接住。
test('认领的路径表', () => {
  deepEqual([...legalEntityFieldPaths], [
    'legalEntities[0].legalEntityId',
    'legalEntities[0].partyId',
    'legalEntities[0].revision',
    'legalEntities[0].basis',
    'legalEntities[0].effectiveFrom',
    'legalEntities[0].registrationCountry',
    'legalEntities[0].lifetimeRegistrationNumbers',
    'legalEntities[0].identityCorrectionBasis',
  ]);
});

// Covers: 身份层三格（票 legal-entity-profile/04 第 1 项）——填了就去首尾空白带上，号按行成项、次序照填；
// 两格都空的行当作没填；三格留空则键缺席而不是空串（空串会被 Intake 当成「给了」而答形状错）。
test('身份层三格：填了照带，留空缺席', () => {
  const filled = legalEntityPayloadOf(
    draft({
      registrationCountry: ' SG ',
      lifetimeNumbers: [
        { typeCode: ' SYN-UEN ', number: ' 200000001Z ' },
        { typeCode: '', number: '  ' },
        { typeCode: 'SYN-ALT', number: '2' },
      ],
      identityCorrectionBasis: ' SYN-CORRECTION-01 ',
    }),
    'UTC',
  ).legalEntities[0];
  equal(filled.registrationCountry, 'SG');
  deepEqual(filled.lifetimeRegistrationNumbers, [
    { typeCode: 'SYN-UEN', number: '200000001Z' },
    { typeCode: 'SYN-ALT', number: '2' },
  ]);
  equal(filled.identityCorrectionBasis, 'SYN-CORRECTION-01');

  const blank = legalEntityPayloadOf(
    draft({ registrationCountry: '  ', lifetimeNumbers: [{ typeCode: ' ', number: '' }], identityCorrectionBasis: ' ' }),
    'UTC',
  ).legalEntities[0];
  equal('registrationCountry' in blank, false);
  equal('lifetimeRegistrationNumbers' in blank, false);
  equal('identityCorrectionBasis' in blank, false);
});

// Covers: 只填了类型或只填了号的行编不成 Intake 要的一项，本地按行号报在号表路径上；整行空或整行齐都不报。
// 国家在不在目录、号合不合格式不在本地判。
test('半填的号行是编码层问题，按行报', () => {
  deepEqual(
    legalEntityLocalProblems(
      draft({
        registrationCountry: 'NOT-A-COUNTRY',
        lifetimeNumbers: [
          { typeCode: 'SYN-UEN', number: '' },
          { typeCode: '', number: '' },
          { typeCode: 'SYN-ALT', number: 'anything' },
          { typeCode: ' ', number: '3' },
        ],
      }),
      'UTC',
    ),
    { 'legalEntities[0].lifetimeRegistrationNumbers': ['第 1 行：类型与号要成对填', '第 4 行：类型与号要成对填'] },
  );
});

function numberType(over: Partial<RegistrationNumberTypeRecord>): RegistrationNumberTypeRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    countryCode: 'SG',
    typeCode: 'SYN-UEN',
    revision: 1,
    typeName: '合成终身号',
    layer: 'IDENTITY',
    formatPattern: '^[0-9A-Z]+$',
    basis: 'SYN-RNT-BASIS-01',
    status: 'EFFECTIVE',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 国家候选取身份层类型的国家码、去重、按码排序；资料层类型的国家不进候选。
test('国家候选：身份层国家去重排序', () => {
  deepEqual(
    identityCountryOptions([
      numberType({ countryCode: 'SG' }),
      numberType({ countryCode: 'CN', typeCode: 'SYN-USCC' }),
      numberType({ countryCode: 'SG', typeCode: 'SYN-ALT' }),
      numberType({ countryCode: 'MY', typeCode: 'SYN-GST', layer: 'PROFILE' }),
    ]),
    [
      { value: 'CN', label: 'CN' },
      { value: 'SG', label: 'SG' },
    ],
  );
});

// Covers: 类型候选按国家（去首尾空白后比）只列身份层；已停用的照列并括注，拒不拒由服务端判；没选国家即无候选。
test('类型候选：按国家列身份层，停用照列并括注', () => {
  const types = [
    numberType({}),
    numberType({ typeCode: 'SYN-OLD', typeName: '旧号', deactivatedAt: '2026-06-01T00:00:00Z' }),
    numberType({ typeCode: 'SYN-GST', typeName: '资料层号', layer: 'PROFILE' }),
    numberType({ countryCode: 'CN', typeCode: 'SYN-USCC' }),
  ];
  deepEqual(identityTypeOptions(types, ' SG '), [
    { value: 'SYN-UEN', label: 'SYN-UEN · 合成终身号' },
    { value: 'SYN-OLD', label: 'SYN-OLD · 旧号（已停用）' },
  ]);
  deepEqual(identityTypeOptions(types, ''), []);
});

// Covers: 「有没有没提交的输入」（票 group-legal-entities-uiux/04 离开前确认用）：空表单、只有空白、只有空的注册号行都不算；
// 任一格填了就算。落册之后（landed 是送出的那份）内容没再动就不算——修订号格不比，送出那份带的是建议值；落册后再改任一格又算。
test('未提交草稿：空与空白不算，落册后未改不算', () => {
  const empty = emptyLegalEntityDraft();
  equal(legalEntityDraftUnsaved(empty, null), false);
  equal(legalEntityDraftUnsaved({ ...empty, basis: '   ' }, null), false);
  equal(legalEntityDraftUnsaved({ ...empty, lifetimeNumbers: [{ typeCode: '', number: '' }, { typeCode: ' ', number: '' }] }, null), false);
  equal(legalEntityDraftUnsaved({ ...empty, legalEntityId: 'LE-01' }, null), true);
  equal(legalEntityDraftUnsaved({ ...empty, lifetimeNumbers: [{ typeCode: '', number: '9100' }] }, null), true);
  equal(legalEntityDraftUnsaved({ ...empty, revision: '3' }, null), true);

  const filled = { ...empty, legalEntityId: 'LE-01', basis: 'B-1' };
  equal(legalEntityDraftUnsaved(filled, { ...filled, revision: '1' }), false);
  equal(legalEntityDraftUnsaved({ ...filled, basis: 'B-2' }, { ...filled, revision: '1' }), true);
});

// ——建档一体化（票 company-subject-ux/02）——

function outcome(name: string): ApiResult<RegistrationResponseBody> {
  return { kind: 'outcome', status: 200, body: { outcome: name } };
}

const together = (over: Partial<LegalEntityDraft> = {}): LegalEntityDraft =>
  draft({ partyMode: 'together', partyId: '', partyName: ' 合成新公司（演示） ', ...over });

// Covers: 一并登记时往来单位编码缺省取公司主体编码、手填了就用手填的，两头都裁空白；法人载荷钉的就是这个编码，选已有档案时
// 钉的是选中的那个。第一步落册后钉住登记成的编码，再改公司主体编码也不跟着变。
test('一并登记：往来单位编码缺省取公司主体编码，落册后钉住', () => {
  equal(companionPartyIdOf(together({ legalEntityId: ' SYN-LE-NEW ' })), 'SYN-LE-NEW');
  equal(companionPartyIdOf(together({ companionPartyId: ' SYN-PARTY-NEW ' })), 'SYN-PARTY-NEW');
  equal(companionPartyIdOf(together({ companionPartyId: '   ' })), 'SYN-LE-02');

  equal(effectivePartyIdOf(together()), 'SYN-LE-02');
  equal(legalEntityPayloadOf(together(), 'UTC').legalEntities[0].partyId, 'SYN-LE-02');
  equal(effectivePartyIdOf(draft({})), 'SYN-PARTY-SHIPPER-01');

  const pinned = withCompanionRegistered(together(), 'SYN-LE-02');
  equal(effectivePartyIdOf({ ...pinned, legalEntityId: 'SYN-LE-03' }), 'SYN-LE-02');
  equal(legalEntityPayloadOf({ ...pinned, legalEntityId: 'SYN-LE-03' }, 'UTC').legalEntities[0].legalEntityId, 'SYN-LE-03');
});

// Covers: 往来单位那一步的草稿——名称裁空白、来源单据与生效时间共用法人这一笔的、修订号固定 1（首笔，不替服务端算下一修订）。
test('一并登记：往来单位草稿借法人的来源单据与生效时间，修订号为 1', () => {
  deepEqual(companionPartyDraftOf(together()), {
    partyId: 'SYN-LE-02',
    name: '合成新公司（演示）',
    revision: '1',
    basis: 'SYN-REG-BASIS-LE-02',
    effectiveFrom: '2026-01-02T08:00',
  });
});

// Covers: 要不要先发第一步——一并登记且没落过才发；选已有档案不发；第一步落过不再发。
test('保存计划：只有一并登记且未落册才发往来单位那一步', () => {
  deepEqual(pairedSavePlan(together()), { party: true });
  deepEqual(pairedSavePlan(draft({})), { party: false });
  deepEqual(pairedSavePlan(withCompanionRegistered(together(), 'SYN-LE-02')), { party: false });
});

// Covers: 第一步算落了的只有 REGISTERED 与 ALREADY_REGISTERED 两格线上名；别的答案、403、没到达都不算。
test('第一步落册判据：已登记与已在册算，其余不算', () => {
  equal(partyStepLanded(outcome('REGISTERED')), true);
  equal(partyStepLanded(outcome('ALREADY_REGISTERED')), true);
  equal(partyStepLanded(outcome('CONTENT_CONFLICT')), false);
  equal(partyStepLanded(outcome('NOT_ACCEPTED')), false);
  equal(partyStepLanded({ kind: 'unconfigured', status: 403 } as ApiResult<RegistrationResponseBody>), false);
  equal(partyStepLanded({ kind: 'transport', message: 'offline' }), false);
});

// 记下每次发送的次序与内容的假发送器：第一步答 partyAnswers 里依次取，第二步答 entityAnswer。
function senders(partyAnswers: ApiResult<RegistrationResponseBody>[], entityAnswer: ApiResult<RegistrationResponseBody>) {
  const log: string[] = [];
  return {
    log,
    send: {
      party: async (companion: { partyId: string; name: string }) => {
        log.push(`party ${companion.partyId} ${companion.name}`);
        return partyAnswers.shift() ?? outcome('REGISTERED');
      },
      entity: async (sending: LegalEntityDraft) => {
        log.push(`entity ${sending.legalEntityId} -> ${effectivePartyIdOf(sending)}`);
        return entityAnswer;
      },
    },
  };
}

// Covers: 两步都成——先往来单位后法人，法人钉的是刚登记成的编码；两份答复各自原样交回，草稿钉上编码。
test('两步串行：先往来单位、后法人，两步都成', async () => {
  const { log, send } = senders([outcome('REGISTERED')], outcome('REGISTERED'));
  const result = await runPairedSave(together(), send);
  deepEqual(log, ['party SYN-LE-02 合成新公司（演示）', 'entity SYN-LE-02 -> SYN-LE-02']);
  deepEqual(result.party, outcome('REGISTERED'));
  deepEqual(result.entity, outcome('REGISTERED'));
  equal(result.draft.companionRegisteredPartyId, 'SYN-LE-02');
});

// Covers: 第一步失败——答复原样交回，第二步不发，草稿不钉编码（下次保存仍从第一步起）。
test('两步串行：第一步失败则第二步不发', async () => {
  const { log, send } = senders([outcome('CONTENT_CONFLICT')], outcome('REGISTERED'));
  const result = await runPairedSave(together(), send);
  deepEqual(log, ['party SYN-LE-02 合成新公司（演示）']);
  deepEqual(result.party, outcome('CONTENT_CONFLICT'));
  equal(result.entity, null);
  equal(result.draft.companionRegisteredPartyId, '');
  deepEqual(pairedSavePlan(result.draft), { party: true });
});

// Covers: 第一步成、第二步失败——草稿钉上编码；拿这份草稿再保存一轮，只发第二步、不再发第一步，法人仍钉原编码。
test('两步串行：第二步失败后再保存只重发第二步', async () => {
  const first = senders([outcome('REGISTERED')], outcome('NOT_ACCEPTED'));
  const failed = await runPairedSave(together(), first.send);
  deepEqual(first.log, ['party SYN-LE-02 合成新公司（演示）', 'entity SYN-LE-02 -> SYN-LE-02']);
  deepEqual(failed.entity, outcome('NOT_ACCEPTED'));
  equal(failed.draft.companionRegisteredPartyId, 'SYN-LE-02');

  // 操作者改了法人那边的格（这里改公司主体编码）再保存：只发第二步，钉的还是登记成的往来单位。
  const retry = senders([outcome('CONTENT_CONFLICT')], outcome('REGISTERED'));
  const landed = await runPairedSave({ ...failed.draft, legalEntityId: 'SYN-LE-02B' }, retry.send);
  deepEqual(retry.log, ['entity SYN-LE-02B -> SYN-LE-02']);
  equal(landed.party, null);
  deepEqual(landed.entity, outcome('REGISTERED'));
});

// Covers: 往来单位「已在册」（ALREADY_REGISTERED）视为第一步成功——第二步照发。
test('两步串行：往来单位已在册视为第一步成功', async () => {
  const { log, send } = senders([outcome('ALREADY_REGISTERED')], outcome('REGISTERED'));
  const result = await runPairedSave(together(), send);
  deepEqual(log, ['party SYN-LE-02 合成新公司（演示）', 'entity SYN-LE-02 -> SYN-LE-02']);
  equal(result.draft.companionRegisteredPartyId, 'SYN-LE-02');
});

// Covers: 选已有档案——不发第一步，法人钉选中的参与方；草稿原样交回。
test('两步串行：选已有档案只发法人', async () => {
  const { log, send } = senders([], outcome('REGISTERED'));
  const result = await runPairedSave(draft({}), send);
  deepEqual(log, ['entity SYN-LE-02 -> SYN-PARTY-SHIPPER-01']);
  equal(result.party, null);
  equal(result.draft.companionRegisteredPartyId, '');
});
