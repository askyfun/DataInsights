import type { CSSProperties } from 'react';

export type DropZoneType = 'dimension' | 'metric' | 'filter';

/**
 * 拖放区的类型配色：维度蓝 / 指标绿 / 过滤橙。
 * 维度组、指标组、过滤字段组共用，保证三个区域的落点高亮与边框色同源不漂移。
 * accent 走全站语义色变量（:root 的 --dr-dim / --dr-metric / --dr-filter），
 * 不允许在本文件另写一份 HEX —— 旧版这里写过 #1890ff，与字段标签的 antd blue
 * （#1677ff）形成两个蓝，同一页面「维度」有两种颜色。
 * hoverBackground 是落点高亮的浅底，目前只有这三处消费，故就近以字面量登记。
 */
const ZONE_PALETTE: Record<DropZoneType, { accent: string; hoverBackground: string }> = {
  dimension: { accent: 'var(--dr-dim)', hoverBackground: '#e6f4ff' },
  metric: { accent: 'var(--dr-metric)', hoverBackground: '#f6ffed' },
  filter: { accent: 'var(--dr-filter)', hoverBackground: '#fff7e6' },
};

/** 该类型的强调色（边框高亮 / 标签文字色）。 */
export const getDropZoneAccent = (zoneType: DropZoneType): string => ZONE_PALETTE[zoneType].accent;

/**
 * dnd-kit droppable 的 id 命名约定。
 * 调用场景：FieldDropZone（维度/指标组）与 FilterDropZone（过滤字段组）注册落点。
 * 两个组件共用同一函数，避免新落点手写 id 与现有 id 撞名后互相抢命中。
 */
export const dropZoneId = (zoneType: DropZoneType, groupIndex = 0): string =>
  `dropzone-${zoneType}-${groupIndex}`;

/** 拖放区容器样式：空态与有字段时形状一致，落点悬停时按类型换底色与描边。 */
export const dropZoneSurfaceStyle = (zoneType: DropZoneType, isOver: boolean): CSSProperties => {
  const palette = ZONE_PALETTE[zoneType];
  return {
    // 26 = 单行字段标签（Tag 高 20） + 上下各 2px 内边距 + 2px 呼吸；空态文字行高
    // 同量级，故空态与有字段态高度一致。原为 32，是按旧标签高 28 定的，标签收紧后
    // 会白留一圈纵向空白。
    minHeight: 26,
    padding: '2px 8px',
    // 底色与描边取自页面表面系统的"下沉层"（--dr-sunken / --dr-border-strong，
    // 定义在 styles/index.css 的 :root）：槽位读起来是卡面上"挖出的坑"，
    // 与卡体的白面形成明度差，一眼能认出"这里可以放东西"。
    // 色值只用变量引用，避免同一套灰阶在本文件里再写一份。
    backgroundColor: isOver ? palette.hoverBackground : 'var(--dr-sunken)',
    border: `1px dashed ${isOver ? palette.accent : 'var(--dr-border-strong)'}`,
    borderRadius: 6,
    transition: 'all 0.2s ease',
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    flexWrap: 'wrap',
  };
};
