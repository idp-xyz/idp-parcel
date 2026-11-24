import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { agentOutboundSteps, outboundTrack, selfOutboundSteps } from './outbound-track';
import { returnReceiptOutcomeLabels } from './presentation';

test('卡在预占的两种状态落在预占这一步，并以状态本身的词标出', () => {
  deepEqual(outboundTrack('PARTIALLY_RESERVED', false), {
    steps: selfOutboundSteps,
    currentId: 'RESERVED',
    currentLabel: '部分预占',
  });
  equal(outboundTrack('PENDING_POLICY', false).currentLabel, '待定缺货策略');
});

test('待回库是流程中止，不画成走到了已预占', () => {
  const track = outboundTrack('AWAITING_RETURN', false);
  deepEqual(track.halt, { label: '已中止，待回库', tone: 'danger' });
  equal(track.currentLabel, undefined);
});

test('拆成新单是中性结束，取消与拒单是失败', () => {
  equal(outboundTrack('SPLIT', false).halt?.tone, 'neutral');
  equal(outboundTrack('CANCELLED', false).halt?.tone, 'danger');
  equal(outboundTrack('REJECTED', false).halt?.tone, 'danger');
});

test('代理仓的单打包后不显示成走过了波次、拣货与复核', () => {
  const packed = outboundTrack('PACKED', true);
  equal(packed.steps, agentOutboundSteps);
  equal(packed.currentId, 'PACKED');
  equal(outboundTrack('PACKED', false).steps, selfOutboundSteps);
});

test('待转出与已转出即便不知道仓种类也走代理仓那组步骤', () => {
  equal(outboundTrack('HANDED_OFF', false).steps, agentOutboundSteps);
  equal(outboundTrack('FORWARDED', false).currentId, 'FORWARDED');
});

test('步骤里没有的状态停在第一步，并原样标出状态', () => {
  deepEqual(outboundTrack('SOMETHING_NEW', false), {
    steps: selfOutboundSteps,
    currentId: 'RESERVED',
    currentLabel: 'SOMETHING_NEW',
  });
});

test('退件到仓的 COUNTED 不说成盘点的已盘点', () => {
  equal(returnReceiptOutcomeLabels.COUNTED, '已点收入暂存');
  equal(returnReceiptOutcomeLabels.UNCLAIMED, '认不出货主，转待认领');
});
