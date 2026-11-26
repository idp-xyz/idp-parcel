import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { activeWaveStatuses, settledWaveStatuses, waveProgress, waveStatusLabel, waveTrack, withdrawableRequests } from './wave-flow';

test('状态的词：四种各有中文，陌生的词原样示出', () => {
  deepEqual(
    [...activeWaveStatuses, ...settledWaveStatuses].map(waveStatusLabel),
    ['已放出', '拣货中', '已完成', '已取消'],
  );
  equal(waveStatusLabel('ARCHIVED'), 'ARCHIVED');
});

test('进度条：已放出、拣货中、已完成各落在自己那一步', () => {
  equal(waveTrack('RELEASED').currentId, 'RELEASED');
  equal(waveTrack('PICKING').currentId, 'PICKING');
  equal(waveTrack('COMPLETED').currentId, 'COMPLETED');
  equal(waveTrack('PICKING').halt, undefined);
});

test('进度条：已取消是流程停下，答中性；陌生的词示在第一步上', () => {
  deepEqual(waveTrack('CANCELLED').halt, { label: '已取消', tone: 'neutral' });
  const unknown = waveTrack('ARCHIVED');
  equal(unknown.currentId, 'RELEASED');
  equal(unknown.currentLabel, 'ARCHIVED');
});

const tasks = (...statuses: string[]) => statuses.map((status) => ({ status }));

test('能撤出的出库单：没撤过、有任务等着拣、没有一条拣过', () => {
  const wave = {
    requests: ['R-OPEN', 'R-PART', 'R-SHORT', 'R-DONE', 'R-GONE', 'R-CUT', 'R-NONE'],
    withdrawnRequests: ['R-GONE'],
    tasks: [
      { requestId: 'R-OPEN', status: 'OPEN' },
      { requestId: 'R-OPEN', status: 'OPEN' },
      { requestId: 'R-PART', status: 'OPEN' },
      { requestId: 'R-PART', status: 'DONE' },
      { requestId: 'R-SHORT', status: 'OPEN' },
      { requestId: 'R-SHORT', status: 'SHORT' },
      { requestId: 'R-DONE', status: 'DONE' },
      { requestId: 'R-GONE', status: 'CANCELLED' },
      { requestId: 'R-CUT', status: 'CANCELLED' },
    ],
  };
  deepEqual(withdrawableRequests(wave), ['R-OPEN']);
  deepEqual(withdrawableRequests({ requests: [], withdrawnRequests: [], tasks: [] }), []);
});

test('拣货进度：有了结果的（拣满、少拣）占待执行、拣满、少拣三种的百分比，往下取整', () => {
  deepEqual(waveProgress(tasks('OPEN', 'DONE', 'SHORT')), { tasks: 3, open: 1, done: 1, short: 1, cancelled: 0, percent: 66 });
  equal(waveProgress(tasks('DONE', 'DONE')).percent, 100);
  equal(waveProgress(tasks('OPEN', 'OPEN')).percent, 0);
});

test('拣货进度：作废与迟到的任务不算进度，单独数', () => {
  deepEqual(waveProgress(tasks('DONE', 'CANCELLED', 'LATE')), { tasks: 1, open: 0, done: 1, short: 0, cancelled: 2, percent: 100 });
});

test('拣货进度：没有任务可算时是 0，不除以零', () => {
  deepEqual(waveProgress(tasks('CANCELLED')), { tasks: 0, open: 0, done: 0, short: 0, cancelled: 1, percent: 0 });
  deepEqual(waveProgress([]), { tasks: 0, open: 0, done: 0, short: 0, cancelled: 0, percent: 0 });
});
