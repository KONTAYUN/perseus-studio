import React from 'react';
import { ArrowLeft, Ban, CheckCircle, KeyRound, Plus, ShieldCheck, Trash2, Users } from 'lucide-react';
import { apiGet, apiPost, apiPatch, apiDelete } from '../../api/client';
import {
  Badge,
  Button,
  CardActions,
  EmptyCard,
  Field,
  FormHeader,
  IconButton,
  Input,
  ProviderCard,
  SectionTitle,
  Select,
  SettingsSection,
  Switch,
  formatDate,
} from './SettingsUI';

export default function UserManagement({ dialog, onBack, currentUser }) {
  const [users, setUsers] = React.useState([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState('');
  const [allowRegistration, setAllowRegistration] = React.useState(true);
  const [createForm, setCreateForm] = React.useState({ username: '', password: '', role: 'user' });
  const [isCreating, setIsCreating] = React.useState(false);
  const [resetPasswordId, setResetPasswordId] = React.useState('');
  const [newPassword, setNewPassword] = React.useState('');

  const loadData = React.useCallback(async () => {
    try {
      const [usersData, settingsData] = await Promise.all([
        apiGet('/api/admin/users'),
        apiGet('/api/admin/settings'),
      ]);
      setUsers(usersData.items || []);
      setAllowRegistration(settingsData.allow_registration);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { loadData(); }, [loadData]);

  async function handleCreateUser(e) {
    e.preventDefault();
    if (!createForm.username.trim() || !createForm.password.trim()) return;
    setIsCreating(true);
    setError('');
    try {
      await apiPost('/api/admin/users', createForm);
      setCreateForm({ username: '', password: '', role: 'user' });
      await loadData();
    } catch (err) {
      setError(err.message);
    } finally {
      setIsCreating(false);
    }
  }

  async function handleToggleRole(user) {
    const newRole = user.role === 'admin' ? 'user' : 'admin';
    const action = newRole === 'user' ? '降级为普通用户' : '提升为管理员';
    const ok = await dialog?.confirm?.({
      title: '变更用户角色',
      message: `确认将 ${user.username} ${action}？角色变更后该用户需要重新登录。`,
      confirmLabel: '确认变更',
    });
    if (!ok) return;
    setError('');
    try {
      await apiPatch('/api/admin/users/' + encodeURIComponent(user.id), { role: newRole });
      await loadData();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleToggleDisabled(user) {
    const action = user.disabled ? '启用' : '禁用';
    const ok = await dialog?.confirm?.({
      title: `${action}用户`,
      message: `确认${action}用户 ${user.username}？`,
      confirmLabel: action,
      tone: user.disabled ? 'blue' : 'red',
    });
    if (!ok) return;
    setError('');
    try {
      const endpoint = user.disabled ? 'enable' : 'disable';
      await apiPost('/api/admin/users/' + encodeURIComponent(user.id) + '/' + endpoint);
      await loadData();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleResetPassword(userId) {
    if (!newPassword.trim()) return;
    setError('');
    try {
      await apiPost('/api/admin/users/' + encodeURIComponent(userId) + '/reset-password', { password: newPassword });
      setResetPasswordId('');
      setNewPassword('');
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleDeleteUser(user) {
    const ok = await dialog?.confirm?.({
      title: '删除用户',
      message: `确认删除用户 ${user.username}？此操作将删除用户账号记录，但保留用户数据目录。`,
      confirmLabel: '删除用户',
      tone: 'red',
    });
    if (!ok) return;
    setError('');
    try {
      await apiDelete('/api/admin/users/' + encodeURIComponent(user.id));
      await loadData();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleToggleRegistration() {
    setError('');
    try {
      await apiPatch('/api/admin/settings', { allow_registration: !allowRegistration });
      setAllowRegistration(!allowRegistration);
    } catch (err) {
      setError(err.message);
    }
  }

  const isSelf = (user) => user.id === currentUser?.id;

  if (loading) {
    return (
      <SettingsSection className="settings-view-enter">
        <div className="text-sm text-gray-500 dark:text-gray-400">加载中...</div>
      </SettingsSection>
    );
  }

  return (
    <SettingsSection className="settings-view-enter">
      <Button
        type="button"
        variant="ghost"
        size="xs"
        onClick={onBack}
        className="mb-5 px-2 py-1 text-gray-400"
      >
        <ArrowLeft size={14} /> 返回设置
      </Button>

      <SectionTitle icon={<Users size={15} />} title="用户管理" description="管理系统用户、角色和注册设置。" />

      {error && (
        <div className="settings-expand-enter mb-4 rounded-xl bg-red-50 px-4 py-2.5 text-xs font-bold text-red-600 dark:bg-red-500/10 dark:text-red-300">
          {error}
        </div>
      )}

      <div className="settings-card mb-5 flex items-center justify-between rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#2C2C2E]/75">
        <div>
          <div className="text-sm font-bold text-gray-800 dark:text-gray-100">允许新用户注册</div>
          <div className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {allowRegistration ? '任何人可在登录页注册新账号' : '仅管理员可创建新用户'}
          </div>
        </div>
        <Switch
          checked={allowRegistration}
          onChange={handleToggleRegistration}
          title={allowRegistration ? '关闭注册' : '开启注册'}
        />
      </div>

      <div className="mb-3 text-xs font-bold text-gray-500 dark:text-gray-400">
        所有用户 ({users.length})
      </div>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px] items-start">
        <div className="space-y-3">
          {users.length === 0 && <EmptyCard text="暂无用户。" />}
          {users.map((user) => (
            <ProviderCard key={user.id}>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-bold text-gray-800 dark:text-gray-100 truncate">
                    {user.username}
                  </span>
                  <Badge tone={user.role === 'admin' ? 'blue' : 'gray'}>{user.role}</Badge>
                  {user.disabled ? <Badge tone="red">已禁用</Badge> : null}
                  {isSelf(user) && <Badge tone="green">当前</Badge>}
                </div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  创建于 {formatDate(user.created_at)}
                </div>
                {resetPasswordId === user.id && (
                  <form
                    className="settings-expand-enter mt-2 flex items-center gap-2"
                    onSubmit={(e) => { e.preventDefault(); handleResetPassword(user.id); }}
                  >
                    <Input
                      type="password"
                      placeholder="新密码"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      className="h-8 w-40 text-xs"
                      autoFocus
                    />
                    <Button
                      type="submit"
                      size="xs"
                    >
                      确认
                    </Button>
                    <Button
                      type="button"
                      onClick={() => { setResetPasswordId(''); setNewPassword(''); }}
                      variant="ghost"
                      size="xs"
                      className="px-2 py-1 text-gray-400"
                    >
                      取消
                    </Button>
                  </form>
                )}
              </div>
              <CardActions>
                <IconButton
                  title={user.role === 'admin' ? '降级为 user' : '提升为 admin'}
                  onClick={() => handleToggleRole(user)}
                  disabled={isSelf(user)}
                >
                  <ShieldCheck size={15} />
                </IconButton>
                <IconButton
                  title={user.disabled ? '启用用户' : '禁用用户'}
                  onClick={() => handleToggleDisabled(user)}
                  disabled={isSelf(user)}
                >
                  {user.disabled ? <CheckCircle size={15} /> : <Ban size={15} />}
                </IconButton>
                <IconButton
                  title="重置密码"
                  onClick={() => { setResetPasswordId(user.id); setNewPassword(''); }}
                >
                  <KeyRound size={15} />
                </IconButton>
                <IconButton
                  title="删除用户"
                  onClick={() => handleDeleteUser(user)}
                  danger
                  disabled={isSelf(user)}
                >
                  <Trash2 size={15} />
                </IconButton>
              </CardActions>
            </ProviderCard>
          ))}
        </div>

        <div className="settings-card h-fit rounded-2xl border border-gray-100 bg-white/80 p-5 dark:border-white/10 dark:bg-[#2C2C2E]/75">
          <FormHeader title="创建用户" />
          <form onSubmit={handleCreateUser}>
            <Field label="用户名">
              <Input
                type="text"
                placeholder="username"
                value={createForm.username}
                onChange={(e) => setCreateForm((f) => ({ ...f, username: e.target.value }))}
                required
              />
            </Field>
            <Field label="密码">
              <Input
                type="password"
                placeholder="password"
                value={createForm.password}
                onChange={(e) => setCreateForm((f) => ({ ...f, password: e.target.value }))}
                required
              />
            </Field>
            <Field label="角色">
              <Select
                value={createForm.role}
                onChange={(e) => setCreateForm((f) => ({ ...f, role: e.target.value }))}
              >
                <option value="user">user</option>
                <option value="admin">admin</option>
              </Select>
            </Field>
            <Button
              type="submit"
              disabled={isCreating}
              className="mt-5"
              fullWidth
            >
              <Plus size={15} />
              {isCreating ? '创建中...' : '创建用户'}
            </Button>
          </form>
        </div>
      </div>
    </SettingsSection>
  );
}
