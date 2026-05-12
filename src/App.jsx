import React from 'react';
import AppHeader from './components/layout/AppHeader';
import AppDialog, { useAppDialog } from './components/common/AppDialog';
import MainPanels from './components/layout/MainPanels';
import SettingsModal from './components/settings/SettingsModal';
import TaskPanel from './components/tasks/TaskPanel';
import { apiGet, apiPost } from './api/client';
import useIdeState from './hooks/useIdeState';
import useTasks from './hooks/useTasks';

export default function App() {
  const tasksRef = React.useRef(null);
  const ideRef = React.useRef(null);
  const [auth, setAuth] = React.useState({ loading: true, authenticated: false, user: null, hasUsers: false, registrationOpen: true });
  const [loginUsername, setLoginUsername] = React.useState('');
  const [loginPassword, setLoginPassword] = React.useState('');
  const [loginError, setLoginError] = React.useState('');
  const [isLoggingIn, setIsLoggingIn] = React.useState(false);
  const [isRegisterMode, setIsRegisterMode] = React.useState(false);
  const appDialog = useAppDialog();
  const ide = useIdeState({
    currentUser: auth.user,
    enabled: auth.authenticated,
    dialog: appDialog.api,
    onTasksChanged: (task) => {
      tasksRef.current?.upsertTask(task);
      tasksRef.current?.loadTasks();
    },
  });
  const tasks = useTasks({
    enabled: auth.authenticated,
    onFinished: async (finishedTasks) => {
      const latestIde = ideRef.current;
      if (!latestIde) return;
      await latestIde.loadTree();
      await latestIde.loadConversations();
      for (const task of finishedTasks) {
        await refreshAfterFinishedTask(task, latestIde);
      }
    },
  });

  React.useEffect(() => {
    tasksRef.current = tasks;
  }, [tasks]);

  React.useEffect(() => {
    ideRef.current = ide;
  }, [ide]);

  React.useEffect(() => {
    checkAuth();
    const onUnauthorized = () => {
      setAuth((current) => ({ ...current, loading: false, authenticated: false, user: null }));
    };
    window.addEventListener('perseus:unauthorized', onUnauthorized);
    return () => window.removeEventListener('perseus:unauthorized', onUnauthorized);
  }, []);

  async function checkAuth() {
    try {
      const data = await apiGet('/api/auth/me');
      if (data.authenticated) {
        setAuth({ loading: false, authenticated: true, user: data.user, hasUsers: true, registrationOpen: false });
      } else {
        const shouldRegister = !data.has_users;
        setAuth({ loading: false, authenticated: false, user: null, hasUsers: data.has_users, registrationOpen: data.registration_open });
        setIsRegisterMode(shouldRegister);
      }
    } catch (error) {
      console.error('checkAuth failed', error);
      setAuth({ loading: false, authenticated: false, user: null, hasUsers: false, registrationOpen: true });
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setLoginError('');
    setIsLoggingIn(true);
    try {
      const endpoint = isRegisterMode ? '/api/auth/register' : '/api/auth/login';
      const data = await apiPost(endpoint, { username: loginUsername, password: loginPassword });
      if (data.authenticated) {
        setAuth({ loading: false, authenticated: true, user: data.user, hasUsers: true, registrationOpen: false });
        setLoginUsername('');
        setLoginPassword('');
      }
    } catch (error) {
      setLoginError(error.message || (isRegisterMode ? '注册失败' : '登录失败'));
    } finally {
      setIsLoggingIn(false);
    }
  }

  async function handleLogout() {
    await apiPost('/api/auth/logout', {});
    setAuth((current) => ({ ...current, authenticated: false, user: null }));
    setLoginUsername('');
    setLoginPassword('');
    checkAuth();
  }

  if (auth.loading) {
    return (
      <div className="h-screen w-screen bg-[#121212] text-gray-200 flex items-center justify-center text-sm">
        正在进入 Perseus Studio...
      </div>
    );
  }

  if (!auth.authenticated) {
    return (
      <LoginScreen
        username={loginUsername}
        password={loginPassword}
        error={loginError}
        isLoggingIn={isLoggingIn}
        isRegisterMode={isRegisterMode}
        registrationOpen={auth.registrationOpen}
        hasUsers={auth.hasUsers}
        onUsernameChange={setLoginUsername}
        onPasswordChange={setLoginPassword}
        onSubmit={handleSubmit}
        onToggleMode={() => {
          setIsRegisterMode((v) => !v);
          setLoginError('');
        }}
      />
    );
  }

  return (
    <div
      className={`app-shell h-screen w-screen flex flex-col overflow-hidden transition-colors duration-500 font-sans selection:bg-blue-200 dark:selection:bg-blue-900 ${
        ide.isDarkMode ? 'dark bg-[#121212] text-gray-200' : 'bg-[#F5F5F7] text-gray-800'
      }`}
    >
      <style>{`
        .custom-scrollbar::-webkit-scrollbar { width: 6px; height: 6px; }
        .custom-scrollbar::-webkit-scrollbar-track { background: transparent; }
        .custom-scrollbar::-webkit-scrollbar-thumb { background: rgba(156, 163, 175, 0.4); border-radius: 10px; }
        .dark .custom-scrollbar::-webkit-scrollbar-thumb { background: rgba(107, 114, 128, 0.4); }
        .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: rgba(107, 114, 128, 0.7); }
        .dark .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: rgba(156, 163, 175, 0.7); }
        .scrollbar-rounded-clip {
          clip-path: inset(0 round 0 0 14px 14px);
          scrollbar-gutter: stable;
        }
        .editor-panel-root { container-type: inline-size; }
        @container (max-width: 360px) {
          .editor-tab-close { display: none; }
        }
      `}</style>

      <AppHeader
        isDarkMode={ide.isDarkMode}
        layoutColumns={ide.layoutColumns}
        onLayoutChange={ide.setLayoutColumns}
        onAddPanel={ide.addPanel}
        onCloseDraggedPanel={ide.closeDraggedPanel}
        onPanelTrashOver={ide.setIsPanelTrashOver}
        onOpenSettings={() => ide.setIsSettingsOpen(true)}
        onOpenTasks={tasks.openTaskPanel}
        onToggleDarkMode={() => ide.setIsDarkMode((value) => !value)}
        onLogout={handleLogout}
        taskCount={tasks.activeTaskCount}
        taskDoneUnread={tasks.hasUnreadFinishedTask}
        isPanelDragging={ide.dragType === 'reorder'}
        isPanelTrashOver={ide.isPanelTrashOver}
        panelAddMenuOpen={ide.panelAddMenuOpen}
        setPanelAddMenuOpen={ide.setPanelAddMenuOpen}
        panels={ide.panels}
        currentUser={auth.user}
      />

      <MainPanels ide={{ ...ide, tasks }} />

      {ide.isSettingsOpen && (
        <SettingsModal ide={ide} dialog={appDialog.api} onClose={() => ide.setIsSettingsOpen(false)} currentUser={auth.user} />
      )}

      <TaskPanel tasks={tasks} />
      <AppDialog dialog={appDialog.dialog} onClose={appDialog.close} />
    </div>
  );
}

function LoginScreen({ username, password, error, isLoggingIn, isRegisterMode, registrationOpen, hasUsers, onUsernameChange, onPasswordChange, onSubmit, onToggleMode }) {
  const isFirstUser = !hasUsers;
  const title = isFirstUser ? '创建管理员账号' : isRegisterMode ? '注册账号' : '登录';
  const subtitle = isFirstUser ? '首次使用，请创建一个管理员账号。' : isRegisterMode ? '注册新账号加入 Perseus Studio。' : '请输入用户名和密码。';
  const buttonText = isLoggingIn ? (isRegisterMode ? '正在注册...' : '正在登录...') : isFirstUser ? '创建并进入' : isRegisterMode ? '注册' : '进入';

  return (
    <div className="h-screen w-screen bg-[#121212] text-gray-100 flex items-center justify-center px-6">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm rounded-2xl border border-white/10 bg-[#1C1C1E] p-6 shadow-2xl shadow-black/40"
      >
        <div className="mb-6">
          <div className="text-xl font-bold">Perseus Studio</div>
          <div className="mt-1 text-sm text-gray-400">{subtitle}</div>
        </div>
        <input
          autoFocus
          type="text"
          value={username}
          onChange={(event) => onUsernameChange(event.target.value)}
          className="h-11 w-full rounded-xl border border-white/10 bg-black/20 px-3 text-sm outline-none transition focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
          placeholder="用户名"
          autoComplete="username"
        />
        <input
          type="password"
          value={password}
          onChange={(event) => onPasswordChange(event.target.value)}
          className="mt-3 h-11 w-full rounded-xl border border-white/10 bg-black/20 px-3 text-sm outline-none transition focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
          placeholder="密码"
          autoComplete={isRegisterMode ? 'new-password' : 'current-password'}
        />
        {error && <div className="mt-3 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300">{error}</div>}
        <button
          type="submit"
          disabled={isLoggingIn || !username || !password}
          className="mt-5 h-10 w-full rounded-xl bg-blue-500 text-sm font-semibold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:bg-gray-700 disabled:text-gray-400"
        >
          {buttonText}
        </button>
        {!isFirstUser && registrationOpen && (
          <button
            type="button"
            onClick={onToggleMode}
            className="mt-3 w-full text-center text-xs text-gray-500 hover:text-gray-300 transition"
          >
            {isRegisterMode ? '已有账号？去登录' : '没有账号？注册一个'}
          </button>
        )}
      </form>
    </div>
  );
}

async function refreshAfterFinishedTask(task, ide) {
  const conversationId = getTaskConversationId(task);
  if (!conversationId) return;
  try {
    const data = await apiGet(`/api/conversations/${encodeURIComponent(conversationId)}/messages`);
    const messages = data.items || [];
    const lastAssistant = [...messages].reverse().find((message) => message.role === 'assistant');
    await ide.refreshWorkspaceAfterAssistantTools(lastAssistant?.attachments || []);
    if (ide.currentConversationId === conversationId) {
      await ide.loadConversationMessages(conversationId);
    }
  } catch (error) {
    console.error('refreshAfterFinishedTask failed', error);
  }
}

function getTaskConversationId(task) {
  if (task?.result_json?.conversation_id) return task.result_json.conversation_id;
  try {
    const payload = JSON.parse(task?.payload_json || '{}');
    return payload.conversation_id || '';
  } catch {
    return '';
  }
}
