// 选册 / 选签 chip 的一份类名（票 admin-web-group-legal-entities/13 第 4 条，10 N4）。此前 MultiRegistrationPanel 与
// 业务参与方页登记签各留一份同字副本，改一处漏一处；抬到这里各页按名导入。它只是一串 class，不是组件——chip 的
// 语义（选中哪一册）由调用方的按钮承载，这里不替它决定 aria 或事件。

/** 选中态描边与文字换成强调色；未选中态用边框色与弱文字，悬停显底色。 */
export const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;
