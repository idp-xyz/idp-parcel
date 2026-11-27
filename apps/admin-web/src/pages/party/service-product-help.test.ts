import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { serviceProductHelpSections } from './service-product-help';

test('服务产品帮助按登记顺序写全', () => {
  equal(serviceProductHelpSections.map((section) => section.title).join('、'), [
    '先发布一个版本',
    '版本生效之后，登记服务形态',
    '然后让别的册引用这一版',
    '详情上怎么读这一版',
  ].join('、'));
  const publish = serviceProductHelpSections[0];
  equal(publish.paragraphs[0].includes('填表、预览、存为待批准、批准、发布'), true);
  equal(publish.items?.some((item) => item.label === '范围叫法' && item.text.includes('不会被盖掉')), true);
  equal(publish.items?.some((item) => item.label === '交付条件' && item.text.includes('整节都会提交')), true);
  equal(serviceProductHelpSections[1].paragraphs.some((paragraph) => paragraph.includes('也不会被选中')), true);
  equal(serviceProductHelpSections[2].items?.some((item) => item.label === '客户与合同' && item.text.includes('不能放宽')), true);
  equal(serviceProductHelpSections[3].paragraphs.some((paragraph) => paragraph.includes('不是替你填上的默认值')), true);
});
