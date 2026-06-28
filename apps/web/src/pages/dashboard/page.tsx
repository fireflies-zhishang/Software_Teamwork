import { Activity, FileUp, MessageSquareText, Rows3 } from 'lucide-react'

const modules = [
  {
    title: '知识管理',
    description: '知识库、文档上传、处理状态、切片详情与检索配置。',
    metric: '4 条主流程',
    icon: FileUp,
  },
  {
    title: '智能问答',
    description: '多会话聊天、SSE 流式输出、引用溯源和思考过程。',
    metric: 'SSE ready',
    icon: MessageSquareText,
  },
  {
    title: '报告生成',
    description: '大纲编辑、章节流式生成、富文本编辑和 DOCX 下载。',
    metric: '向导式',
    icon: Rows3,
  },
]

const workflowSteps = [
  '从 upstream/develop 创建个人功能分支',
  '在 apps/web/src 内开发前端功能',
  '运行 bun run --cwd apps/web check 和 build',
  '从 fork 分支发起 PR 到 upstream/develop',
]

export function DashboardPage() {
  return (
    <div className="dashboard">
      <section className="intro-section">
        <div>
          <p className="eyebrow">基础骨架</p>
          <h2>前端应用已按 apps/web/src 建立</h2>
          <p>
            当前骨架包含应用入口、Provider、Query Client、布局、模块导航和基础页面，后续页面按
            features 与 pages 分层扩展。
          </p>
        </div>
        <div className="intro-stat" aria-label="当前质量基线">
          <Activity aria-hidden="true" />
          <span>check + build</span>
        </div>
      </section>

      <section className="module-grid" aria-label="前端模块">
        {modules.map((module) => (
          <article className="module-card" key={module.title}>
            <module.icon aria-hidden="true" size={22} />
            <div>
              <h3>{module.title}</h3>
              <p>{module.description}</p>
            </div>
            <span>{module.metric}</span>
          </article>
        ))}
      </section>

      <section className="workflow-section">
        <h2>协作流程</h2>
        <ol>
          {workflowSteps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </section>
    </div>
  )
}
