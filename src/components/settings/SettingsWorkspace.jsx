import React from 'react';
import { ArrowLeft, BarChart3, Blocks, ClipboardList, KeyRound, Search, Settings2, Users } from 'lucide-react';
import { Button } from './SettingsUI';
import UserManagement from './UserManagement';
import DiagnosticLogsView from './DiagnosticLogsView';
import UsageDashboard from './UsageDashboard';
import ModelProviderSettings from './ModelProviderSettings';
import SearchProviderSettings from './SearchProviderSettings';
import SkillSettings from './SkillSettings';
import { SettingsNavigation, useSettingsNavigation } from './SettingsNavigation';

const pages = [
  { id: 'models', label: '模型与连接', icon: KeyRound, group: '工作台功能', description: '管理可用模型，为文稿写作选择合适的服务。' },
  { id: 'search', label: '联网搜索', icon: Search, group: '工作台功能', description: '配置 Agent 使用的联网搜索服务。' },
  { id: 'skills', label: '工具扩展', icon: Blocks, group: '工作台功能', description: '安装和管理 Skills，为 Agent 补充专门的工作方法。' },
  { id: 'users', label: '用户与访问', icon: Users, group: '系统管理', description: '管理用户、角色与注册规则。' },
  { id: 'usage', label: '用量与费用', icon: BarChart3, group: '系统管理', description: '查看调用用量，配置模型价格和估算费用。' },
  { id: 'diagnostics', label: '诊断日志', icon: ClipboardList, group: '系统管理', description: '查看模型与工具运行错误，定位工作台问题。' },
];

export default function SettingsWorkspace({ ide, dialog, onClose, currentUser }) {
  const [page, setPage] = React.useState('models');
  const heading = React.useRef(null);
  const content = React.useRef(null);
  const { value, notice } = useSettingsNavigation(dialog);
  const active = pages.find(item => item.id === page);
  React.useEffect(() => {
    const origin = document.activeElement;
    return () => { if (origin?.isConnected) origin.focus(); };
  }, []);
  React.useEffect(() => { content.current?.scrollTo(0, 0); heading.current?.focus(); }, [page]);
  if (currentUser?.role !== 'admin') return null;
  const navigate = (next) => { if (next !== page) value.attemptLeave(() => setPage(next)); };
  return (
    <SettingsNavigation.Provider value={value}>
      <section aria-label="工作台设置" className="settings-workspace" data-testid="settings-workspace">
        <header className="settings-workspace-header">
          <div className="settings-brand"><Settings2 size={19} /><span>工作台设置</span><span className="settings-admin-label">管理员</span></div>
          <Button variant="secondary" onClick={() => value.attemptLeave(onClose)} data-testid="settings-return">
            <ArrowLeft size={16} /> 返回工作台
          </Button>
        </header>
        <label className="settings-mobile-navigation">设置分类
          <select aria-label="设置分类" value={page} onChange={e => navigate(e.target.value)}>{pages.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</select>
        </label>
        <div className="settings-workspace-body">
          <nav className="settings-navigation custom-scrollbar" aria-label="设置分类">
            {['工作台功能', '系统管理'].map(group => (
              <div className="settings-nav-group" key={group}>
                <p>{group}</p>
                {pages.filter(item => item.group === group).map(({ id, label, icon: Icon }) => (
                  <button type="button" key={id} aria-current={id === page ? 'page' : undefined}
                    data-testid={id === 'diagnostics' ? 'open-diagnostics-settings' : `settings-nav-${id}`}
                    onClick={() => navigate(id)} className="settings-nav-item motion-row">
                    <Icon size={17} /><span>{label}</span>
                  </button>
                ))}
              </div>
            ))}
            <p className="settings-nav-caption">Perseus Studio</p>
          </nav>
          <main ref={content} className="settings-content custom-scrollbar" aria-labelledby="settings-page-title">
            <div className="settings-page">
              <div className="settings-page-heading"><h1 id="settings-page-title" ref={heading} tabIndex={-1}>{active.label}</h1><p>{active.description}</p></div>
              {notice && <p role="status" className="settings-feedback">{notice}</p>}
              {ide.settingsError && <p role="alert" className="settings-feedback settings-feedback-error">{ide.settingsError}</p>}
              {page === 'models' && <ModelProviderSettings ide={ide} dialog={dialog} />}
              {page === 'search' && <SearchProviderSettings ide={ide} dialog={dialog} />}
              {page === 'skills' && <SkillSettings ide={ide} dialog={dialog} />}
              {page === 'users' && <UserManagement dialog={dialog} currentUser={currentUser} />}
              {page === 'usage' && <UsageDashboard dialog={dialog} currentUser={currentUser} />}
              {page === 'diagnostics' && <DiagnosticLogsView dialog={dialog} currentUser={currentUser} />}
            </div>
          </main>
        </div>
      </section>
    </SettingsNavigation.Provider>
  );
}
