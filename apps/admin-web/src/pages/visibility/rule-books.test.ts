import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { labelFor } from '../../vocabulary/status';
import { registrationSnapshotHints, visibilityCatalogueKindLabels } from './presentation';
import {
  customerText,
  deadlineText,
  disclosureKinds,
  judgmentKinds,
  disclosureCellText,
  lifecycleAt,
  ruleCountSummary,
  ruleRowMatches,
  ruleRowsOf,
} from './rule-books';

// 本文件钉的是设置区两张规则册页的行模型与人话：码经词典译中文、相对时限折成「72 小时内」、生效与否按生效期判、
// 客户显名称读不到显编码。数据取演示租户登记的那几本册（scripts/demo-seeds/data/visibility）的形状。

const now = new Date('2026-10-28T12:00:00Z');

test('相对时限折成人话，认不得的原样', () => {
  equal(deadlineText('72:00:00'), '72 小时内');
  equal(deadlineText('3 days 04:00:00'), '3 天 4 小时内');
  equal(deadlineText('72h'), '72 小时内');
  equal(deadlineText('1h30m'), '1 小时 30 分钟内');
  equal(deadlineText('P3D'), 'P3D');
  equal(deadlineText(''), '');
});

test('生效与否按生效期判', () => {
  equal(lifecycleAt('2026-01-05T00:00:00Z', undefined, now), 'EFFECTIVE');
  equal(lifecycleAt('2026-01-05T00:00:00Z', '2026-03-01T00:00:00Z', now), 'EXPIRED');
  equal(lifecycleAt('2027-01-01T00:00:00Z', undefined, now), 'REGISTERED');
  equal(labelFor('EFFECTIVE'), '已生效');
  equal(labelFor('EXPIRED'), '已到期');
  equal(labelFor('REGISTERED'), '待生效');
});

test('演示租户登记的码全部有中文', () => {
  for (const code of [
    'node-intake',
    'handover-handed-over',
    'movement-departure',
    'external-result-release-full',
    'effective-delivery',
    'PICKED_UP',
    'IN_TRANSIT',
    'DEPARTED',
    'CUSTOMS_CLEARED',
    'DELIVERED',
    'DELIVERY_FAILED',
    'CUSTOMS_HOLD',
    'ETA_GAP',
    'SOURCE_FACT_CONFLICT',
    'CARRIER_CONFIRMED',
    'HEURISTIC',
    'MONITOR_DERIVED',
    'SOURCE_CONFIRMED',
    'AUTO_ESTABLISH',
    'ATTACH_TO_EXISTING',
    'MANUAL_REVIEW',
    'NO_CASE',
    'EMAIL',
    'CUSTOMER_NOTIFIED',
    'SHOWN',
    'PENDING_CONFIRMATION',
    'NOT_DISCLOSED',
  ]) {
    equal(/[A-Za-z]/.test(labelFor(code)), false, `${code} → ${labelFor(code)}`);
  }
});

test('状态归类行：环节与动态说中文，状态格留码给胶囊，原码进审计', () => {
  const [row] = ruleRowsOf(
    {
      outcome: 'VISIBILITY_CATALOGUES_LISTED',
      kind: 'MILESTONE_MAPPING',
      catalogues: [
        {
          version: 'SYN-VE-MAP-V1',
          effectiveFrom: '2026-01-05T00:00:00Z',
          approvedBy: 'SYN-OPS-TRACKING-LEAD',
          entries: [{ source: 'NODE_OPERATIONS', factKind: 'node-intake', milestone: 'PICKED_UP' }],
        },
      ],
    },
    new Map(),
    now,
  );
  equal(row.cells.source, '网点');
  equal(row.cells.fact, '已收寄');
  equal(row.cells.milestone, 'PICKED_UP');
  equal(row.lifecycle, 'EFFECTIVE');
  equal(row.audit.find((entry) => entry.label === '版本号')?.value, 'SYN-VE-MAP-V1');
  equal(ruleRowMatches(row, '已揽收'), true);
  equal(ruleRowMatches(row, 'syn-ve-map'), true);
  equal(ruleRowMatches(row, '已签收'), false);
});

test('对客户显示行：客户显名称、读不到显编码，四项按态说', () => {
  const body = {
    outcome: 'VISIBILITY_CATALOGUES_LISTED' as const,
    kind: 'DISCLOSURE_POLICY' as const,
    catalogues: [
      {
        version: 'SYN-VE-DISCLOSE-V1',
        effectiveFrom: '2026-01-05T00:00:00Z',
        effectiveTo: '2026-03-01T00:00:00Z',
        approvedBy: 'SYN-OPS-TRACKING-LEAD',
        entries: [
          {
            customer: 'SYN-ACCOUNT-01',
            milestones: { state: 'SHOWN', content: 'SYN-VE-CONTENT-PUBLIC-MILESTONES-V1' },
            eta: { state: 'PENDING_CONFIRMATION' },
            final: { state: 'PENDING_CONFIRMATION' },
            note: { state: 'NOT_DISCLOSED' },
          },
        ],
      },
    ],
  };
  const [named] = ruleRowsOf(body, new Map([['SYN-ACCOUNT-01', '合成货主一号']]), now);
  equal(named.cells.customer, '合成货主一号');
  equal(named.cells.milestones, '展示');
  equal(named.audit.find((entry) => entry.label === '物流状态文案')?.value, 'SYN-VE-CONTENT-PUBLIC-MILESTONES-V1');
  equal(named.cells.eta, '待确认');
  equal(named.lifecycle, 'EXPIRED');
  const [coded] = ruleRowsOf(body, new Map(), now);
  equal(coded.cells.customer, 'SYN-ACCOUNT-01');
  equal(customerText('X', new Map()), 'X');
  equal(disclosureCellText({ state: 'NOT_DISCLOSED' }), '不披露');
});

test('通知时限与动态矛盾两册没有生效期，计数只报条数', () => {
  const rows = ruleRowsOf(
    {
      outcome: 'VISIBILITY_CATALOGUES_LISTED',
      kind: 'NOTIFICATION_POLICY',
      catalogues: [
        {
          policy: 'SYN-VE-DISCLOSE-V1',
          channel: 'EMAIL',
          deadlineAfter: '72:00:00',
          obligation: 'CUSTOMER_NOTIFIED',
          approvedBy: 'SYN-OPS-TRACKING-LEAD',
        },
      ],
    },
    new Map(),
    now,
  );
  deepEqual(
    { channel: rows[0].cells.channel, deadline: rows[0].cells.deadline, obligation: rows[0].cells.obligation },
    { channel: '邮件', deadline: '72 小时内', obligation: '通知送达客户' },
  );
  equal(rows[0].lifecycle, undefined);
  equal(ruleCountSummary('NOTIFICATION_POLICY', rows), '通知时限 1 条');
});

test('签名与填写提示每册都有，提示不出现系统术语', () => {
  for (const kind of [...judgmentKinds, ...disclosureKinds]) {
    equal(visibilityCatalogueKindLabels[kind].length > 0, true, kind);
    const hint = registrationSnapshotHints[kind];
    for (const word of ['服务端', '租户', '快照', '译装', '受控登记口']) {
      equal(hint.includes(word), false, `${kind} 的提示含「${word}」`);
    }
  }
});
