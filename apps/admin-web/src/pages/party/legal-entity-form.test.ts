import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { GroupLegalEntityRecord } from './api';
import {
  emptyLegalEntityDraft,
  legalEntityFieldPaths,
  legalEntityLocalProblems,
  legalEntityPayloadOf,
  suggestedRevision,
  type LegalEntityDraft,
} from './legal-entity-form';

// 本文件钉的是表单只做编码层的事（伞票 admin-write-faces/07 硬句）：修订号编成整数、墙钟时刻换成
// RFC 3339、可缺键缺席；空串、引用在不在册、修订连不连续，一律原样送上去让服务端答。载荷镜像受控 CLI
// register-parties 的 legalEntities 一项且**不带 tenantId**——在线 Intake 从认证结果取租户，载荷里带上
// 就是自报（register_party_identity.go 包注释）。

function draft(over: Partial<LegalEntityDraft>): LegalEntityDraft {
  return {
    ...emptyLegalEntityDraft(),
    legalEntityId: 'SYN-LE-02',
    partyId: 'SYN-PARTY-SHIPPER-01',
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
  ]);
});
