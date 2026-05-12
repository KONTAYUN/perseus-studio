import React from 'react';
import { BookOpen, FileWarning, Users, X } from 'lucide-react';
import { SectionTitle } from './SettingsUI';
import UserManagement from './UserManagement';
import DiagnosticLogsView from './DiagnosticLogsView';
import ModelProviderSettings from './ModelProviderSettings';
import SearchProviderSettings from './SearchProviderSettings';
import SkillSettings from './SkillSettings';

export default function SettingsModal({ ide, dialog, onClose, currentUser }) {
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

  return (
    <div
      className={`fixed inset-0 z-[100] flex items-center justify-center bg-black/40 p-4 backdrop-blur-md transition-opacity duration-300 ease-apple dark:bg-black/60 ${
        isClosing || !hasEntered ? 'opacity-0' : 'opacity-100'
      }`}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) closeWithAnimation();
      }}
    >
      <div
        className={`settings-modal-card ui-overlay-surface flex max-h-[88vh] w-full max-w-6xl flex-col overflow-hidden border border-white/40 bg-white/90 backdrop-blur-3xl transition-all duration-500 ease-apple dark:border-white/10 dark:bg-[#1C1C1E]/95 ${
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
          <button
            onClick={closeWithAnimation}
            className="p-2 bg-gray-100 dark:bg-white/5 text-gray-500 hover:text-gray-800 dark:hover:text-gray-200 hover:bg-gray-200 dark:hover:bg-white/10 rounded-full transition-colors"
          >
            <X size={20} />
          </button>
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

          {settingsView === 'main' && (
          <>
          {currentUser?.role === 'admin' && (
          <section className="settings-section-appear rounded-[24px] border border-gray-100 bg-white/55 p-5 dark:border-white/10 dark:bg-white/[0.03]">
            <SectionTitle icon={<Users size={15} />} title="用户管理" description="管理系统用户、角色和注册设置。" />
            <button
              type="button"
              onClick={() => setSettingsView('users')}
              className="settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left transition-all duration-300 ease-apple hover:-translate-y-0.5 hover:border-blue-200 hover:bg-blue-50/40 hover:shadow-lg hover:shadow-blue-500/10 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">管理用户</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  查看和管理所有用户、角色和注册开关
                </div>
              </div>
              <Users size={18} className="text-blue-500" />
            </button>
          </section>
          )}

          <section className="settings-section-appear rounded-[24px] border border-gray-100 bg-white/55 p-5 dark:border-white/10 dark:bg-white/[0.03]">
            <SectionTitle icon={<BookOpen size={15} />} title="Skills" description="安装和管理本地 Skill。AI 只会在需要时读取启用的 Skill。" />
            <button
              type="button"
              onClick={() => setSettingsView('skills')}
              className="settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left transition-all duration-300 ease-apple hover:-translate-y-0.5 hover:border-blue-200 hover:bg-blue-50/40 hover:shadow-lg hover:shadow-blue-500/10 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">管理 Skills</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  已安装 {ide.skills.length} 个，启用 {ide.skills.filter((item) => item.enabled).length} 个
                </div>
              </div>
              <BookOpen size={18} className="text-blue-500" />
            </button>
          </section>

          <ModelProviderSettings ide={ide} />

          <SearchProviderSettings ide={ide} />

          <section className="settings-section-appear rounded-[24px] border border-gray-100 bg-white/55 p-5 dark:border-white/10 dark:bg-white/[0.03]">
            <SectionTitle
              icon={<FileWarning size={15} />}
              title="诊断日志"
              description={currentUser?.role === 'admin' ? '汇总查看用户报错摘要，定位模型调用和工具执行问题。' : '查看最近模型调用和工具执行错误，导出后发给管理员排查。'}
            />
            <button
              data-testid="open-diagnostics-settings"
              type="button"
              onClick={() => setSettingsView('diagnostics')}
              className="settings-nav-card flex w-full items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 text-left transition-all duration-300 ease-apple hover:-translate-y-0.5 hover:border-blue-200 hover:bg-blue-50/40 hover:shadow-lg hover:shadow-blue-500/10 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25 dark:hover:bg-blue-500/10"
            >
              <div>
                <div className="font-bold text-gray-800 dark:text-gray-100">查看诊断日志</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {currentUser?.role === 'admin' ? '管理员可查看全部用户最近报错，不包含 API Key、完整提示词或文件内容' : '仅包含最近错误摘要，不包含 API Key、完整提示词或文件内容'}
                </div>
              </div>
              <FileWarning size={18} className="text-blue-500" />
            </button>
          </section>
          </>
          )}
          </div>
        </div>

        <div className="px-7 py-5 bg-gray-50/50 dark:bg-[#252526]/50 border-t border-gray-100 dark:border-gray-800 flex justify-end sticky bottom-0">
          <button
            onClick={closeWithAnimation}
            className="px-6 py-2.5 text-sm font-bold text-white bg-blue-500 hover:bg-blue-600 rounded-xl shadow-lg shadow-blue-500/30 hover:scale-105 active:scale-95 transition-all"
          >
            完成
          </button>
        </div>
      </div>
    </div>
  );
}
