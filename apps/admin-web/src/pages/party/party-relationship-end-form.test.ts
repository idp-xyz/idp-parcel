import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { PartyRelationshipRecord } from './api';
import {
  endingAvailability,
  endingConfirmLines,
  endingConfirmMessage,
  endingLocalProblems,
  endingPayloadOf,
  endingRevisionOf,
  initialEndingDraft,
  relationshipSentence,
  wallTimeOf,
} from './party-relationship-end-form';

// 本文件钉关系详情「撤销」「到期」两个动作的纯逻辑（票 ps-external-mark-relations/17）：只有已生效的关系开得了，开不了时
// 说清为什么；到期另要区间有终点；落点修订是这一行的下一号；时刻按本地墙钟填、换成 RFC 3339 UTC 送；确认框复述关系、
// 生效边界（本地 + UTC）、依据与落点修订，并写明只影响边界之后的新决定、不可撤回。载荷镜像服务端批文一节一项、不带 tenantId。

const shanghai = 'Asia/Shanghai';

function row(over: Partial<PartyRelationshipRecord> = {}): PartyRelationshipRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    relationshipId: 'SYN-REL-AUTHORIZED-DELIVERY-01',
    revision: 1,
    holderId: 'SYN-PARTY-DELIVERER-01',
    holderName: '合成交付方一号',
    holderNameKnown: true,
    counterpartyId: 'SYN-PARTY-SHIPPER-01',
    counterpartyName: '合成货主一号',
    counterpartyNameKnown: true,
    role: 'AUTHORIZED_DELIVERING_PARTY',
    scope: 'customerAccount=SYN-ACCOUNT-01',
    basis: 'SYN-MANDATE-01',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-05T00:00:00Z',
    registeredAt: '2026-01-05T00:00:00Z',
    ...over,
  };
}

test('只有已生效的关系开得了两个动作，开不了时按状态词说为什么', () => {
  deepEqual(endingAvailability(row(), 'revocation'), { available: true });
  const revoked = endingAvailability(row({ status: 'REVOKED' }), 'revocation');
  equal(revoked.available, false);
  ok(!revoked.available && revoked.reason.includes('已撤销') && revoked.reason.includes('只有已生效的关系能撤销'));
  const candidate = endingAvailability(row({ status: 'CANDIDATE', effectiveEndsAt: '2026-06-01T00:00:00Z' }), 'expiration');
  ok(!candidate.available && candidate.reason.includes('候选关系') && candidate.reason.includes('到期'));
});

test('到期另要区间有终点：开区间的关系无期可到，指去撤销', () => {
  const open = endingAvailability(row(), 'expiration');
  ok(!open.available && open.reason.includes('撤销'));
  deepEqual(endingAvailability(row({ effectiveEndsAt: '2026-06-01T00:00:00Z' }), 'expiration'), { available: true });
});

test('初值：撤销预填此刻、到期预填区间终点，都按本地墙钟；依据空着', () => {
  const now = new Date('2026-10-25T02:03:04Z');
  deepEqual(initialEndingDraft(row(), 'revocation', now, shanghai), { basis: '', at: '2026-10-25T10:03:04' });
  deepEqual(initialEndingDraft(row({ effectiveEndsAt: '2026-06-01T00:00:00Z' }), 'expiration', now, shanghai), {
    basis: '',
    at: '2026-06-01T08:00:00',
  });
  equal(wallTimeOf('not a time', shanghai), '');
});

test('载荷一节一项：落点修订是这一行的下一号，时刻换成 RFC 3339 UTC，依据不裁空白、不带租户', () => {
  equal(endingRevisionOf(row({ revision: 3 })), 4);
  deepEqual(endingPayloadOf(row({ revision: 3 }), 'revocation', { basis: ' SYN-REVOCATION-01', at: '2026-10-25T10:00' }, shanghai), {
    revocations: [
      { relationshipId: 'SYN-REL-AUTHORIZED-DELIVERY-01', revision: 4, basis: ' SYN-REVOCATION-01', at: '2026-10-25T02:00:00Z' },
    ],
  });
  deepEqual(endingPayloadOf(row(), 'expiration', { basis: 'SYN-TERM-ENDED-01', at: '2026-06-01T08:00:00' }, shanghai), {
    expirations: [{ relationshipId: 'SYN-REL-AUTHORIZED-DELIVERY-01', revision: 2, basis: 'SYN-TERM-ENDED-01', at: '2026-06-01T00:00:00Z' }],
  });
  const unparsed = endingPayloadOf(row(), 'revocation', { basis: 'b', at: '' }, shanghai);
  equal('at' in unparsed.revocations[0], false);
});

test('本地只拦时刻不可解析；依据为空是服务端的话', () => {
  deepEqual(endingLocalProblems({ basis: '', at: '2026-10-25T10:00' }, shanghai), {});
  deepEqual(endingLocalProblems({ basis: 'b', at: '2026-13-45T10:00' }, shanghai), { at: ['时刻要是可解析的日期时间'] });
});

test('确认框复述关系一句话、生效边界本地 + UTC、依据与落点修订，并写明只影响边界之后的新决定、不可撤回', () => {
  const lines = endingConfirmLines(row(), 'revocation', { basis: 'SYN-REVOCATION-01', at: '2026-10-25T10:00' }, shanghai);
  deepEqual(lines, [
    '关系：合成交付方一号（SYN-PARTY-DELIVERER-01） —授权交付方→ 合成货主一号（SYN-PARTY-SHIPPER-01）（SYN-REL-AUTHORIZED-DELIVERY-01）',
    '生效边界：2026-10-25 10:00:00 UTC+8（2026-10-25 02:00:00 UTC）起不再支持新的商业决定',
    '撤销依据：SYN-REVOCATION-01',
    '落点修订：r2（这一行当前为 r1）',
  ]);
  ok(endingConfirmLines(row(), 'expiration', { basis: '', at: '' }, shanghai).some((line) => line.includes('时刻不可解析')));
  const message = endingConfirmMessage('expiration');
  ok(message.includes('只影响边界之后的新决定') && message.includes('已作出的决定不变') && message.includes('不可撤回'));
});

test('名称转不到时一句话里如实只写标识', () => {
  equal(
    relationshipSentence(row({ holderNameKnown: false, holderName: undefined })),
    'SYN-PARTY-DELIVERER-01 —授权交付方→ 合成货主一号（SYN-PARTY-SHIPPER-01）',
  );
});
