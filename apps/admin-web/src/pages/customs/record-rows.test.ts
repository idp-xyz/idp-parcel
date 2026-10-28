import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import type { ExternalResultBody, SubmissionVersionBody } from './api';
import { externalResultRows, judgementRows, submissionRows } from './record-rows';

// 钉的是哪一格不得被译顺：分层外部结果各占各的列、缺层写「尚无结果」、归属不上的响应不被猜版本。
// 夹具全是合成串。

const at = '2026-08-14T09:00:00Z';

function version(over: Partial<SubmissionVersionBody> = {}): SubmissionVersionBody {
  return {
    version: 'SUBV-1', unit: 'UNIT-1', procedure: 'proc-1', contentDigest: 'dg', members: ['PCL-1'],
    dossier: 'dossier-1', roles: 'roles-1', readinessBasis: 'b', authority: 'auth-1', fixedAt: at,
    isCurrent: true, attempts: [], ...over,
  };
}

function result(over: Partial<ExternalResultBody>): ExternalResultBody {
  return { source: 'RES-1', unattributable: false, layerConflict: false, correctionInForce: false, recordedAt: at, ...over };
}

// Covers: CONTEXT「各层必须分层保存」— 只有放行层结果时，监管接收与业务受理两列仍写尚无结果，不由放行推出。
test('提交版本行：缺失的层写尚无结果，不由别的层推导', () => {
  const [row] = submissionRows(
    [version()],
    [result({ layer: 'RELEASE_RESULT', submissionVersion: 'SUBV-1', rawSemantics: '放行' })],
  );
  equal(row.values.received, '尚无结果');
  equal(row.values.accepted, '尚无结果');
  equal(row.values.attempts, '尚无提交尝试');
});

// Covers: 迁移 0025/0030 — 已登记待发送尝试没有发送事实，不写成失败；技术回执另列。
test('提交尝试：待发送尝试写尚无发送事实，技术回执单独成列', () => {
  const [row] = submissionRows(
    [version({
      attempts: [
        { sequence: 1, target: 'SYN-BROKER', result: 'REGISTERED_PENDING_SEND', receipts: [] },
        {
          sequence: 2, target: 'SYN-BROKER', result: 'ACKNOWLEDGED', sentAt: at,
          sendFact: { sentAt: at, outcome: 'ACKNOWLEDGED' },
          receipts: [{ source: 'r1', outcome: 'ACKNOWLEDGED', occurredAt: at, receivedAt: at }],
        },
      ],
    })],
    [],
  );
  const lines = row.values.attempts.split('\n');
  ok(lines[0].includes('已登记待发送') && lines[0].includes('尚无发送事实'));
  ok(lines[1].includes('发送事实 已确认'));
  ok(row.values.receipts.split('\n')[1].includes('技术成功'));
});

// Covers: 留存不猜 — 归属不上的响应显示未能归属，放行事实带范围与条件。
test('外部结果：归属不上不猜版本；放行带范围与条件', () => {
  const rows = externalResultRows([
    result({ source: 'LOST', unattributable: true, claimedVersion: 'SUBV-9', rawSemantics: 'x' }),
    result({
      source: 'REL', layer: 'RELEASE_RESULT', submissionVersion: 'SUBV-1',
      release: { kind: 'CONDITIONAL', authority: 'CUSTOMS', scope: 'declaration/part', condition: '补证' },
    }),
  ]);
  equal(rows[0].values.layer, '未能归属');
  ok(rows[0].values.version.includes('SUBV-9'));
  equal(rows[1].values.layer, '放行结果');
  ok(rows[1].values.release.includes('附条件放行') && rows[1].values.release.includes('条件 补证'));
});

// Covers: UC-CC-013 — 整次答复与逐动作答复分列，逐动作带依据来源。
test('判断册行：整次答复与逐动作答复分列', () => {
  const [row] = judgementRows([{
    judgement: 'J-1', object: 'PCL-1', intent: 'HANDOVER', node: 'A', onwardNode: 'B', actionAt: at,
    crossZone: 'CROSSING', answer: 'ADMITTED', zones: [], judgedAt: at,
    findings: [{ action: 'CROSS_CUSTOMS_MOVEMENT', answer: 'ADMITTED', bases: [{ cause: 'RELEASE_OUTCOME', sources: ['RES-REL'] }] }],
  }]);
  equal(row.values.answer, '准入');
  equal(row.values.route, 'A → B');
  ok(row.values.findings.includes('跨关务区域移动：准入') && row.values.findings.includes('RES-REL'));
});
