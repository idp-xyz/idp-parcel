// 没有单独全屏工位的十种作业。拣货、复核、打包、收货、发货交接、上架移库、盘点清点各有自己的工位；
// 这十种仍用原来的引导表单，搬进全屏的「更多作业」，旧作业台页不再摆表单。

export const moreJobIds = [
  'warehouse-measurement',
  'unclaimed-arrival',
  'return-receipt',
  'form-conversion',
  'condition-conversion',
  'shortage-registration',
  'processing-execution',
  'relabel-execution',
  'unboxing',
  'stock-return',
] as const;

export type MoreJobId = (typeof moreJobIds)[number];
