import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { nounOf, objectNouns } from './objects';

// 本文件钉名词词典的两条纪律：每个词是中文、不为空；设计源第四节点名的几个对照在表里。

test('名词全部是中文、不为空', () => {
  for (const [key, word] of Object.entries(objectNouns)) {
    ok(word.trim() !== '', `${key} 为空`);
    equal(/[A-Za-z]/.test(word), false, `${key} 的词「${word}」含英文`);
  }
});

test('设计源点名的对照', () => {
  equal(nounOf('shipment'), '运单');
  equal(nounOf('customerReference'), '客户单号');
  equal(nounOf('parcelCount'), '件数');
  equal(nounOf('companyEntity'), '公司主体');
  equal(nounOf('changeRecord'), '变更记录');
  equal(nounOf('sourceDocument'), '来源单据');
  equal(nounOf('customsCase'), '清关单');
  equal(nounOf('trackingUpdates'), '物流动态');
  equal(nounOf('updatedAt'), '更新时间');
  equal(nounOf('scanDevice'), '扫描设备');
  equal(nounOf('disposition'), '货物处置');
  equal(nounOf('todo'), '待办事项');
});
