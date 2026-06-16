import React from 'react';
import { BarChart3, BookOpen, ClipboardList, Users, X } from 'lucide-react';
import { Button, IconButton, SectionTitle, SettingsSection } from './SettingsUI';
import UserManagement from './UserManagement';
import DiagnosticLogsView from './DiagnosticLogsView';
import UsageDashboard from './UsageDashboard';
import ModelProviderSettings from './ModelProviderSettings';
import SearchProviderSettings from './SearchProviderSettings';
import SkillSettings from './SkillSettings';

export default function SettingsModal({ ide, dialog, onClose, currentUser }) {
  const isAdmin = currentUser?.role === 'admin';
  const [hasEntered, setHasEntered] = React.useState(false);
  const [isClosing, setIsClosing] = React.useState(false);
  const [settingsView, setSettingsView] = React.useState('main');

  React.useEffect(() => {
    const frame = requestAnimationFrame(() => setHasEntered(true));
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') closeWithAnimation();
    };
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, []);

  const closeWithAnimation = () => {
    setIsClosing(true);
    window.setTimeout(onClose, 160);
  };

  if (!isAdmin) return null;

  return (
    <div
      className={`modal-backdrop fixed inset-0 z-[100] flex items-center justify-center bg-black/40 p-4 backdrop-blur-md transition-opacity duration-300 ease-apple dark:bg-black/60 ${
        isClosing || !hasEntered ? 'opacity-0' : 'opacity-100'
      }`}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) closeWithAnimation();
      }}
    >
      <div
        className={`settings-modal-card ui-overlay-surface flex h-[88vh] w-full max-w-6xl flex-col overflow-hidden border border-white/40 bg-white/90 backdrop-blur-3xl transition-[opacity,transform,box-shadow,border-color,background-color] duration-500 ease-apple dark:border-white/10 dark:bg-[#1C1C1E]/95 ${
          isClosing || !hasEntered ? 'translate-y-3 scale-[0.975] opacity-0' : 'translate-y-0 scale-100 opacity-100'
        }`}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-gray-100 px-7 py-5 dark:border-gray-800">
          <div>
            <h2 className="text-xl font-bold text-gray-800 dark:text-gray-100">偏好设置</h2>
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              API Key 仅保存在后端，前端只显示脱敏值。
            </p>
          </div>
          <IconButton
            onClick={closeWithAnimation}
            title="关闭设置"
            size="md"
          >
            <X size={20} />
          </IconButton>
        </div>

        <div className="flex-1 overflow-y-auto p-7 custom-scrollbar">
          <div key={settingsView} className="settings-view-enter space-y-6">
          {ide.settingsError && (
            <div className="settings-card rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
              {ide.settingsError}
            </div>
          )}

          {settingsView === 'skills' && (
            <SkillSettings ide={ide} dialog={dialog} onBack={() => setSettingsView('main')} />
          )}

          {settingsView === 'users' && (
            <UserManagement dialog={dialog} onBack={() => setSettingsView('main')} currentUser={currentUser} />
          )}

          {settingsView === 'diagnostics' && (
            <DiagnosticLogsView dialog={dialog} onBack={() => setSettingsView('main')} currentUser={currentUser} />
          )}

          {settingsView === 'usage' && (
            <UsageDashboard dialog={dialog} onBack={() => setSettingsView('main')} currentUser={currentUser} />
          )}

          {settingsView === 'main' && (
          <>
          <SettingsSection>
            <SectionTitle icon={<Users size={15} />} title="用户管理" description="管理系统用户、角色和注册设置。" />
            <button
              type="button"
              onClick={() => setSettingsView('users')}
              className="motion-row settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left hover:border-blue-200 hover:bg-blue-50/40 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">管理用户</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  查看和管理所有用户、角色和注册开关
                </div>
              </div>
              <Users size={18} className="text-blue-500" />
            </button>
          </SettingsSection>

          <SettingsSection>
            <SectionTitle icon={<BookOpen size={15} />} title="Skills" description="安装和管理本地 Skill。AI 只会在需要时读取启用的 Skill。" />
            <button
              type="button"
              onClick={() => setSettingsView('skills')}
              className="motion-row settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left hover:border-blue-200 hover:bg-blue-50/40 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">管理 Skills</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  已安装 {ide.skills.length} 个，启用 {ide.skills.filter((item) => item.enabled).length} 个
                </div>
              </div>
              <BookOpen size={18} className="text-blue-500" />
            </button>
          </SettingsSection>

          <ModelProviderSettings ide={ide} />

          <SearchProviderSettings ide={ide} />

          <SettingsSection>
            <SectionTitle
              icon={<BarChart3 size={15} />}
              title="用量统计"
              description="按用户、模型和时间查看管理员 API 配额消耗。"
            />
            <button
              type="button"
              onClick={() => setSettingsView('usage')}
              className="motion-row settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left hover:border-blue-200 hover:bg-blue-50/40 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">查看用户用量</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  统计请求数、tokens、失败次数和已配置模型价格的估算费用
                </div>
              </div>
              <BarChart3 size={18} className="text-blue-500" />
            </button>
          </SettingsSection>

          <SettingsSection>
            <SectionTitle
              icon={<ClipboardList size={15} />}
              title="诊断日志"
              description="汇总查看用户报错摘要，定位模型调用和工具执行问题。"
            />
            <button
              data-testid="open-diagnostics-settings"
              type="button"
              onClick={() => setSettingsView('diagnostics')}
              className="motion-row settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left hover:border-blue-200 hover:bg-blue-50/40 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">查看诊断日志</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  管理员可查看全部用户最近报错，不包含 API Key、完整提示词或文件内容
                </div>
              </div>
              <ClipboardList size={18} className="text-blue-500" />
            </button>
          </SettingsSection>
          </>
          )}
          </div>
        </div>

        <div className="px-7 py-5 bg-gray-50/50 dark:bg-[#252526]/50 border-t border-gray-100 dark:border-gray-800 flex justify-end sticky bottom-0">
          <Button
            onClick={closeWithAnimation}
            size="md"
          >
            完成
          </Button>
        </div>
      </div>
    </div>
  );
}
