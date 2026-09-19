import type { ReactNode } from 'react';

export interface PageHeaderProps {
  /** 页头图标，渲染在白底描边方块内；与顶栏品牌标记同族语汇 */
  icon?: ReactNode;
  title: ReactNode;
  /** 副标题：一句话说清这页能做什么 */
  description?: ReactNode;
  /** 右侧操作区（主操作放最右） */
  extra?: ReactNode;
  /** 面包屑，详情页需要；占满整行渲染在标题上方 */
  breadcrumb?: ReactNode;
}

/**
 * 页面级标题区。
 * 调用场景：所有一级页面与详情页的顶部（`/charts`、`/datasets`、`/datasources`、`/share/:token` …）。
 * 主要逻辑：渲染在**灰画布上而非卡片内** —— 标题坐在画布上、内容落在白卡里，
 * 两者形成明确的前后关系。这是"白底白卡、面无层次"的根治手段：
 * 只要标题还在卡片里，卡片内就同时存在背景白、卡头白、标题区白三层同色。
 *
 * 布局契约：`.dr-page` 的 `gap` 负责与下方卡片的间距，本组件自身不加外边距。
 */
const PageHeader: React.FC<PageHeaderProps> = ({ icon, title, description, extra, breadcrumb }) => (
  <div className="dr-page-header">
    {breadcrumb ? <div className="dr-page-header__crumb">{breadcrumb}</div> : null}
    <div className="dr-page-header__main">
      {icon ? <span className="dr-page-header__icon">{icon}</span> : null}
      <div className="dr-page-header__text">
        <h1 className="dr-page-header__title">{title}</h1>
        {description ? <p className="dr-page-header__desc">{description}</p> : null}
      </div>
    </div>
    {extra ? <div className="dr-page-header__extra">{extra}</div> : null}
  </div>
);

export default PageHeader;
