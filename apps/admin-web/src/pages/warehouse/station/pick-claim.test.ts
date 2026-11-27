import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { operatorIdOf, type OidcSession } from '../../../auth/oidc';
import type { WaveRecord, WaveTaskRecord } from '../api';
import { buildGuidedPayload } from '../guided-forms';
import { allWarehouseGuidedForms } from '../guided-schemas';
import { operatorLabel, operatorNamesOf } from '../../operator-access/operator-names';
import { waveClaimNote } from './pick-claim';
import { pickOutcomeLabels, queueNotes, taskGoneAfter } from './pick-flow';
import { cellsOfWaves, loadPickQueue, taskAssignee } from './pick-queue';

const ME = 'https://issuer.example.invalid#picker-me';
const OTHER = 'https://issuer.example.invalid#picker-other';

const task = (taskId: string, locationCode: string, status = 'OPEN'): WaveTaskRecord => ({
  taskId,
  requestId: `OBR-${taskId}`,
  ownerAccount: 'OWNER-1',
  locationCode,
  goodsCode: 'SKU-1',
  unit: '件',
  stockState: 'GOOD',
  quantity: 2,
  picked: 0,
  status,
});

const wave = (waveId: string, assignee: string | undefined, tasks: WaveTaskRecord[]): WaveRecord => ({
  waveId,
  warehouseCode: 'WH-1',
  operator: 'op',
  prefetch: false,
  status: 'RELEASED',
  releasedAt: '2026-11-27T00:00:00Z',
  ...(assignee === undefined ? {} : { assignee }),
  requests: tasks.map((item) => item.requestId),
  withdrawnRequests: [],
  tasks,
});

const waves = [wave('W-MINE', ME, [task('T-1', 'L-1')]), wave('W-OTHER', OTHER, [task('T-2', 'L-2')]), wave('W-FREE', undefined, [task('T-3', 'L-3')])];

// Covers: 看本仓全部波次时，派给了别人的波次的格不摆，自己的与没人领的照摆；指名了那个波次就照摆；
// 认不出自己是谁时不筛。
test('看全部波次时不摆派给别人的波次', () => {
  deepEqual(cellsOfWaves(waves, { warehouse: 'WH-1', wave: null, me: ME }).map((cell) => cell.taskId), ['T-1', 'T-3']);
  deepEqual(cellsOfWaves(waves, { warehouse: 'WH-1', wave: 'W-OTHER', me: ME }).map((cell) => cell.taskId), ['T-2']);
  equal(cellsOfWaves(waves, { warehouse: 'WH-1', wave: null, me: null }).length, 3);
  equal(cellsOfWaves(waves, { warehouse: 'WH-1', wave: null }).length, 3);
});

// Covers: 按库区派：单独派了人的库区认那个人，没单独派的库区照整个波次的指派走。看全部波次时，别人的库区的格不摆，
// 自己的库区的格照摆——哪怕整个波次派给了别人；这样的波次算进「有格派给了别人」。
test('按库区派：别人的库区的格不摆，自己的库区照摆', async () => {
  const inZone = (taskId: string, locationCode: string, zoneCode: string): WaveTaskRecord => ({ ...task(taskId, locationCode), zoneCode });
  const zoned: WaveRecord = {
    ...wave('W-ZONED', OTHER, [inZone('T-A', 'L-A', 'ZONE-A'), inZone('T-B', 'L-B', 'ZONE-B'), task('T-C', 'L-C')]),
    zoneAssignees: [{ zoneCode: 'ZONE-A', assignee: ME }],
  };
  equal(taskAssignee(zoned, { zoneCode: 'ZONE-A' }), ME);
  equal(taskAssignee(zoned, { zoneCode: 'ZONE-B' }), OTHER);
  equal(taskAssignee(zoned, {}), OTHER);
  equal(taskAssignee({ zoneAssignees: [] }, { zoneCode: 'ZONE-A' }), undefined);
  deepEqual(cellsOfWaves([zoned], { warehouse: 'WH-1', wave: null, me: ME }).map((cell) => cell.taskId), ['T-A']);
  deepEqual(cellsOfWaves([zoned], { warehouse: 'WH-1', wave: null, me: OTHER }).map((cell) => cell.taskId), ['T-B', 'T-C']);
  equal(cellsOfWaves([zoned], { warehouse: 'WH-1', wave: 'W-ZONED', me: ME }).length, 3);
  const answer = await loadPickQueue({
    warehouse: 'WH-1',
    wave: null,
    me: ME,
    list: async () => ({ kind: 'outcome', status: 200, body: { outcome: 'WAVES_LISTED', waves: [zoned], page: { next: null } as never } }),
    one: async () => ({ kind: 'outcome', status: 200, body: { outcome: 'WAVE_NOT_FOUND' } }),
  });
  ok(answer.kind === 'queue' && answer.queue.assignedElsewhere === 1 && answer.queue.cells.length === 1);
});

// Covers: 队列如实带出有几个波次因为派给了别人没摆，屏上说清并指路；拣完了的、没派的不算；波次下拉带着各自派给了谁。
test('队列带出没摆的波次数，屏上说清', async () => {
  const listed = [...waves, wave('W-OTHER-DONE', OTHER, [task('T-9', 'L-9', 'DONE')])];
  const answer = await loadPickQueue({
    warehouse: 'WH-1',
    wave: null,
    me: ME,
    list: async () => ({ kind: 'outcome', status: 200, body: { outcome: 'WAVES_LISTED', waves: listed, page: { next: null } as never } }),
    one: async () => ({ kind: 'outcome', status: 200, body: { outcome: 'WAVE_NOT_FOUND' } }),
  });
  ok(answer.kind === 'queue');
  if (answer.kind !== 'queue') return;
  equal(answer.queue.assignedElsewhere, 1);
  equal(answer.queue.cells.length, 2);
  deepEqual(
    answer.queue.waves.map((entry) => [entry.waveId, entry.assignee]),
    [
      ['W-MINE', ME],
      ['W-OTHER', OTHER],
      ['W-FREE', undefined],
    ],
  );
  const notes = queueNotes(answer.queue, null);
  ok(notes.some((note) => note.includes('另有 1 个波次派给了别人') && note.includes('换波次')));
  deepEqual(queueNotes({ ...answer.queue, assignedElsewhere: 0 }, null), []);
});

// Covers: 后端因为波次派给了别人拒掉一笔拣货：这一格从这个人的队列里去掉，屏上说清货放回原处。
test('拣到派给别人的波次：这一格去掉', () => {
  ok(taskGoneAfter('WAVE_ASSIGNED_TO_OTHER'));
  ok(pickOutcomeLabels.WAVE_ASSIGNED_TO_OTHER.includes('派给了别人') && pickOutcomeLabels.WAVE_ASSIGNED_TO_OTHER.includes('放回原处'));
});

// Covers: 领波次之后的那一句：领到了、本来就是自己的、已经派给了谁、拣完了、没有这个波次、别的仓、没送到，各说各的。
test('领波次之后的话', () => {
  const answered = (outcome: string, assignee?: string) => ({ kind: 'outcome', status: 200, body: { outcome, ...(assignee ? { assignee } : {}) } }) as const;
  equal(waveClaimNote(answered('ASSIGNED', ME)), '领到了，这个波次归你拣。');
  ok(waveClaimNote(answered('EXISTING_RESULT', ME)).includes('本来就是你的'));
  ok(waveClaimNote(answered('WAVE_ASSIGNED_TO_OTHER', OTHER)).includes('已经派给了 picker-other'));
  // 那个人登了姓名就说姓名
  ok(waveClaimNote(answered('WAVE_ASSIGNED_TO_OTHER', OTHER), operatorNamesOf([{ operator: OTHER, name: '合成乙' }])).includes('已经派给了 合成乙'));
  ok(waveClaimNote(answered('WAVE_ASSIGNED_TO_OTHER')).includes('已经派给了 别人'));
  ok(waveClaimNote(answered('STATUS_MISMATCH')).includes('不用领了'));
  ok(waveClaimNote(answered('WAVE_NOT_FOUND')).includes('没有这个波次'));
  ok(waveClaimNote(answered('CONTENT_INVALID')).includes('不属于这台设备绑的仓'));
  ok(waveClaimNote(answered('SOMETHING_NEW')).includes('SOMETHING_NEW'));
  ok(waveClaimNote({ kind: 'transport', message: '断了' }).startsWith('没领成'));
});

// Covers: 拣货人没登姓名时只摆 sub 那一截；操作者标识取身份令牌的发行方与 sub，缺一格不猜。
test('拣货人的示法与「我是谁」', () => {
  equal(operatorLabel(OTHER), 'picker-other');
  equal(operatorLabel('plain-operator'), 'plain-operator');
  equal(operatorLabel('issuer#'), 'issuer#');
  equal(operatorLabel(undefined), null);
  equal(operatorLabel(''), null);
  const session = (claims: Record<string, unknown>) => ({ claims }) as OidcSession;
  equal(operatorIdOf(session({ iss: 'https://issuer.example.invalid', sub: 'picker-me' })), ME);
  equal(operatorIdOf(session({ sub: 'picker-me' })), null);
  equal(operatorIdOf(session({ iss: 'https://issuer.example.invalid', sub: '' })), null);
  equal(operatorIdOf(null), null);
});

// Covers: 指派拣货人的报文：不填拣货人就不带那个键（不带才是取消指派，空串后端不收）。
test('指派拣货人的报文', () => {
  const form = allWarehouseGuidedForms['wave-assign'];
  const assigned = buildGuidedPayload(form, { waveId: 'W-1', assignee: OTHER });
  deepEqual(assigned.ok ? assigned.payload : null, { waveId: 'W-1', assignee: OTHER });
  const cleared = buildGuidedPayload(form, { waveId: 'W-1', assignee: '' });
  deepEqual(cleared.ok ? cleared.payload : null, { waveId: 'W-1' });
});
