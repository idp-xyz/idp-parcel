import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ExternalResultBody, SubmissionVersionBody } from './api';
import {
  caseNumberNote,
  caseNumberOf,
  caseNumberReader,
  caseProgress,
  caseRows,
  externalResultRows,
  judgementRows,
  submissionRows,
  unitRows,
} from './record-rows';
import type { CustomsCaseBody } from './api';

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
  ok(lines[0].includes('待发送') && lines[0].includes('尚无发送事实'));
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
  const row0 = {
    judgement: 'J-1', object: 'PCL-1', intent: 'HANDOVER', node: 'A', onwardNode: 'B', actionAt: at,
    crossZone: 'CROSSING', answer: 'ADMITTED', zones: [], judgedAt: at,
    findings: [{ action: 'CROSS_CUSTOMS_MOVEMENT', answer: 'ADMITTED', bases: [{ cause: 'RELEASED_IN_FULL', sources: ['RES-REL'] }] }],
  };
  const [row] = judgementRows([row0]);
  equal(row.values.answer, '可放行');
  equal(row.values.route, 'A → B');
  equal(row.values.routeCodes, 'A → B');
  // 网点按页面给的查名换成名称，原编码留在 routeCodes。
  const [named] = judgementRows([{ ...row0 }], (node) => ({ A: '上海枢纽' })[node] ?? node);
  equal(named.values.route, '上海枢纽 → B');
  equal(named.values.routeCodes, 'A → B');
  ok(row.values.findings.includes('跨关区移动：可放行') && row.values.findings.includes('RES-REL'));
  // 依据种类经词典译中文、不露原码；依据来源是引用，原样列。
  ok(row.values.findings.includes('海关已全部放行') && !row.values.findings.includes('RELEASED_IN_FULL'), row.values.findings);
});

// Covers: 清关单的「最新进展」取已有事实里最靠后的一层；没有批次就是待申报，只有发送就是已发送，有放行就是已放行，关案盖过一切。
test('清关单最新进展：按已有事实取最靠后的一层', () => {
  const base: CustomsCaseBody = {
    case: 'CASE-1', jurisdiction: 'J', direction: 'EXPORT', procedure: 'proc-1', obligation: 'obl', establishedAt: at,
    parcels: [], roles: [], units: [{ unit: 'UNIT-1', procedure: 'proc-1', members: ['PCL-1'], formedAt: at }],
  };
  equal(caseProgress({ ...base, units: [] }, [], []), 'CUSTOMS_PENDING_DECLARATION');
  equal(caseProgress(base, [version({ attempts: [{ sequence: 1, target: 't', result: 'REGISTERED_PENDING_SEND', receipts: [] }] })], []), 'DECLARATION_PENDING_SEND');
  const sent = version({ attempts: [{ sequence: 1, target: 't', result: 'ACKNOWLEDGED', sendFact: { sentAt: at, outcome: 'ACKNOWLEDGED' }, receipts: [] }] });
  equal(caseProgress(base, [sent], []), 'DECLARATION_SENT');
  equal(caseProgress(base, [sent], [result({ layer: 'REGULATORY_RECEIPT', submissionVersion: 'SUBV-1' })]), 'REGULATORY_RECEIPT');
  equal(
    caseProgress(base, [sent], [result({ layer: 'RELEASE_RESULT', submissionVersion: 'SUBV-1', release: { kind: 'FULL', authority: 'a', scope: 'UNIT-1' } })]),
    'CUSTOMS_RELEASED',
  );
  equal(
    caseProgress(base, [sent], [result({ layer: 'RELEASE_RESULT', scope: 'UNIT-1', release: { kind: 'PARTIAL', authority: 'a', scope: 'UNIT-1' } })]),
    'PARTIAL',
  );
  equal(
    caseProgress({ ...base, closure: { decidedBy: 'x', cutoffAt: at, verifiedAt: at, closedAt: at, itemCount: 1, reopenCount: 0 } }, [sent], []),
    'CUSTOMS_CASE_CLOSED',
  );
});

// Covers: ADR-0355 决定十一，点名 `AT-CC-459`、`AT-CC-460`（管理台半边）——清关单号一列显单据号、案件标识进技术详情；没有单据号的
// 清关单照旧显案件标识，技术详情按它记下的情形注明，不一律写成「上线前开立」；报关批次引到清关单处同样显单据号。
test('清关单号：有号显单据号，无号显案件标识并按情形注明', () => {
  const base: CustomsCaseBody = {
    case: 'CASE-1', jurisdiction: 'J', direction: 'EXPORT', procedure: 'proc-1', obligation: 'obl', establishedAt: at,
    parcels: [], roles: [], units: [{ unit: 'UNIT-1', procedure: 'proc-1', members: ['PCL-1'], formedAt: at }],
  };
  const numbered = { ...base, documentNumber: 'CC-20261102-0001' };
  const unconfigured: CustomsCaseBody = { ...base, case: 'CASE-2', documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' };
  const legacy: CustomsCaseBody = { ...base, case: 'CASE-3', documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' };

  equal(caseNumberOf(numbered), 'CC-20261102-0001');
  equal(caseNumberNote(numbered), '');
  equal(caseNumberOf(unconfigured), 'CASE-2');
  equal(caseNumberNote(unconfigured), '开立时单据编号时区未配置，无单据号');
  equal(caseNumberOf(legacy), 'CASE-3');
  equal(caseNumberNote(legacy), '上线前开立，无单据号');
  // 升级之前的服务端两格都不带：那时的清关单都是上线前开立的。
  equal(caseNumberNote({}), '上线前开立，无单据号');

  const [row, unconfiguredRow] = caseRows([numbered, unconfigured]);
  equal(row.values.number, 'CC-20261102-0001');
  equal(row.values.id, 'CASE-1');
  equal(row.values.numberNote, '');
  equal(unconfiguredRow.values.number, 'CASE-2');
  equal(unconfiguredRow.values.numberNote, '开立时单据编号时区未配置，无单据号');
  equal(unitRows([numbered])[0].values.caseRef, 'CC-20261102-0001');
  equal(unitRows([numbered])[0].values.caseId, 'CASE-1');
  equal(unitRows([unconfigured])[0].values.caseRef, 'CASE-2');
  equal(unitRows([unconfigured])[0].values.caseNumberNote, '开立时单据编号时区未配置，无单据号');
});

// Covers: 关闭核对等只带案件标识的读口按清关单那一页换号；清关单读口没答只让号这一格降级（显标识并注明读不到），
// 不在那一页里的案件照旧显标识。
test('引到清关单处：按清关单列表换号，列表读不到只降级号这一格', () => {
  const record: CustomsCaseBody = {
    case: 'CASE-1', documentNumber: 'CC-20261102-0001', jurisdiction: 'J', direction: 'EXPORT', procedure: 'proc-1',
    obligation: 'obl', establishedAt: at, parcels: [], roles: [], units: [],
  };
  const read = caseNumberReader([record, { ...record, case: 'CASE-2', documentNumber: undefined, documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' }]);
  deepEqual(read('CASE-1'), { number: 'CC-20261102-0001', note: '' });
  deepEqual(read('CASE-2'), { number: 'CASE-2', note: '上线前开立，无单据号' });
  deepEqual(read('CASE-ELSEWHERE'), { number: 'CASE-ELSEWHERE', note: '' });
  const unreadable = caseNumberReader(null)('CASE-1');
  equal(unreadable.number, 'CASE-1');
  ok(unreadable.note.includes('读不到'), unreadable.note);
});

// Covers: 产品形态「人工录入必须与接口来的分得开」——外部结果与发送事实在页面上各自说出来源。
test('来源：人工录入的结果与人工递交的发送带出谁与原件，接口来的说接口送达', () => {
  const rows = externalResultRows([
    result({ source: 'MANUAL/1', manualEntry: { enteredBy: 'SYN-OP-7', original: '海关放行通知书.pdf' } }),
    result({ source: 'RES-2' }),
  ]);
  equal(rows[0].values.entry, '人工录入｜SYN-OP-7 录入，原件：海关放行通知书.pdf');
  equal(rows[1].values.entry, '接口送达');

  const [row] = submissionRows(
    [version({
      attempts: [
        {
          sequence: 1, target: 'SYN-BROKER', result: 'ACKNOWLEDGED', sentAt: at, receipts: [],
          sendFact: {
            sentAt: at, outcome: 'ACKNOWLEDGED',
            manualLodgement: { lodgedBy: 'SYN-OP-7', original: '单一窗口回执截图.png', reference: 'ACK-778', recordedAt: at },
          },
        },
        { sequence: 2, target: 'SYN-BROKER', result: 'ACKNOWLEDGED', sentAt: at, receipts: [], sendFact: { sentAt: at, outcome: 'ACKNOWLEDGED' } },
      ],
    })],
    [],
  );
  const lines = row.values.attempts.split('\n');
  ok(lines[0].endsWith('，人工递交（SYN-OP-7 登记，原件：单一窗口回执截图.png，受理号 ACK-778）'), lines[0]);
  ok(!lines[1].includes('人工递交'), lines[1]);
});
