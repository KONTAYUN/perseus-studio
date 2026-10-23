import React from 'react';
import { ArrowLeft, Ban, CheckCircle, KeyRound, Plus, ShieldCheck, Trash2, UserCheck, Users } from 'lucide-react';
import { apiGet, apiPost, apiPatch, apiDelete } from '../../api/client';
import { useSettingsGuard } from './SettingsNavigation';
import SettingsRowMenu from './SettingsRowMenu';
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
  const [defaultUserRole, setDefaultUserRole] = React.useState('pending');
  const [createForm, setCreateForm] = React.useState({ username: '', password: '', role: 'user' });
  const [isCreating, setIsCreating] = React.useState(false);
  const [resetPasswordId, setResetPasswordId] = React.useState('');
  const [newPassword, setNewPassword] = React.useState('');
  const [showCreate, setShowCreate] = React.useState(false);
  const [isResetting, setIsResetting] = React.useState(false);
  const submitting = React.useRef(false);
  const discard = () => { setCreateForm({ username: '', password: '', role: 'user' }); setNewPassword(''); setResetPasswordId(''); };
  const navigation = useSettingsGuard({ dirty: Boolean(newPassword) || (showCreate && Boolean(createForm.username || createForm.password || createForm.role !== 'user')),
    busy: isCreating || isResetting, discard });

  const loadData = React.useCallback(async () => {
    try {
      const [usersData, settingsData] = await Promise.all([
        apiGet('/api/admin/users'),
        apiGet('/api/admin/settings'),
      ]);
      setUsers(usersData.items || []);
      setAllowRegistration(settingsData.allow_registration);
      setDefaultUserRole(settingsData.default_user_role || 'pending');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { loadData(); }, [loadData]);

  async function handleCreateUser(e) {
    e.preventDefault();
    if (submitting.current || !createForm.username.trim() || !createForm.password.trim()) return;
    submitting.current = true;
    setIsCreating(true);
    setError('');
    try {
      await apiPost('/api/admin/users', createForm);
      setCreateForm({ username: '', password: '', role: 'user' });
      setShowCreate(false);
      await loadData();
    } catch (err) {
      setError(err.message);
    } finally {
      setIsCreating(false);
      submitting.current = false;
    }
  }

  async function handleChangeRole(user, newRole) {
    if (!newRole || newRole === user.role) return;
    const action = roleActionLabel(newRole);
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

  async function handleToggleRole(user) {
    const newRole = user.role === 'admin' ? 'user' : 'admin';
    await handleChangeRole(user, newRole);
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
    if (submitting.current || !newPassword.trim()) return;
    submitting.current = true; setIsResetting(true);
    setError('');
    try {
      await apiPost('/api/admin/users/' + encodeURIComponent(userId) + '/reset-password', { password: newPassword });
      setResetPasswordId('');
      setNewPassword('');
    } catch (err) {
      setError(err.message);
    } finally { submitting.current = false; setIsResetting(false); }
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

  async function handleDefaultRoleChange(role) {
    setError('');
    try {
      const settings = await apiPatch('/api/admin/settings', { default_user_role: role });
      setDefaultUserRole(settings.default_user_role || role);
      setAllowRegistration(settings.allow_registration);
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
      {onBack && <Button
        type="button"
        variant="ghost"
        size="xs"
        onClick={onBack}
        className="mb-5 px-2 py-1 text-gray-400"
      >
        <ArrowLeft size={14} /> 返回设置
      </Button>}

      <div className="settings-list-toolbar"><h2 className="text-sm font-semibold">{showCreate ? '创建用户' : '系统用户'}</h2>
        {!showCreate && <Button onClick={() => navigation.attemptLeave(() => setShowCreate(true))}><Plus size={15} />创建用户</Button>}
      </div>

      {error && (
        <div className="settings-expand-enter mb-4 rounded-xl bg-red-50 px-4 py-2.5 text-xs font-bold text-red-600 dark:bg-red-500/10 dark:text-red-300">
          {error}
        </div>
      )}

      {!showCreate && <div className="settings-card mb-5 space-y-4 rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#2C2C2E]/75">
        <div className="flex items-center justify-between gap-4">
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
        <div className="border-t border-gray-100 pt-4 dark:border-white/10">
          <Field label="新用户默认角色">
            <Select
              value={defaultUserRole}
              onChange={(e) => handleDefaultRoleChange(e.target.value)}
            >
              <option value="pending">待激活</option>
              <option value="user">用户</option>
              <option value="admin">管理员</option>
            </Select>
          </Field>
        </div>
      </div>}

      {!showCreate && <div className="mb-3 text-xs font-bold text-gray-500 dark:text-gray-400">
        所有用户 ({users.length})
      </div>}
      <div>
        {!showCreate && <div className="space-y-3">
          {users.length === 0 && <EmptyCard text="暂无用户。" />}
          {users.map((user) => (
            <ProviderCard key={user.id}>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-bold text-gray-800 dark:text-gray-100 truncate">
                    {user.username}
                  </span>
                  <Badge tone={roleTone(user.role)}>{roleLabel(user.role)}</Badge>
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
                      onClick={() => navigation.attemptLeave(() => { setResetPasswordId(''); setNewPassword(''); })}
                      variant="ghost"
                      size="xs"
                      className="px-2 py-1 text-gray-400"
                    >
                      取消
                    </Button>
                  </form>
                )}
              </div>
              <SettingsRowMenu label={`管理用户 ${user.username}`} text="管理" actions={[
                { label: user.role === 'admin' ? '降级为用户' : '提升为管理员', disabled: isSelf(user), run: () => handleToggleRole(user) },
                { label: user.role === 'pending' ? '激活为用户' : '设为待激活', disabled: isSelf(user), run: () => handleChangeRole(user, user.role === 'pending' ? 'user' : 'pending') },
                { label: user.disabled ? '启用用户' : '禁用用户', disabled: isSelf(user), run: () => handleToggleDisabled(user) },
                { label: '重置密码', run: () => navigation.attemptLeave(() => { setResetPasswordId(user.id); setNewPassword(''); }) },
                { label: '删除用户', danger: true, disabled: isSelf(user), run: () => handleDeleteUser(user) },
              ]} />
            </ProviderCard>
          ))}
        </div>}

        {showCreate && <div className="settings-editor">
          <form onSubmit={handleCreateUser}>
            <fieldset disabled={isCreating}>
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
                <option value="pending">待激活</option>
                <option value="user">用户</option>
                <option value="admin">管理员</option>
              </Select>
            </Field>
            </fieldset>
            <div className="settings-form-actions">
            <Button variant="secondary" disabled={isCreating} onClick={() => navigation.attemptLeave(() => { discard(); setShowCreate(false); })}>取消</Button>
            <Button type="submit" disabled={isCreating}>
              <Plus size={15} />
              {isCreating ? '创建中...' : '创建用户'}
            </Button>
            </div>
          </form>
        </div>}
      </div>
    </SettingsSection>
  );
}

function roleLabel(role) {
  if (role === 'admin') return '管理员';
  if (role === 'pending') return '待激活';
  return '用户';
}

function roleTone(role) {
  if (role === 'admin') return 'blue';
  if (role === 'pending') return 'yellow';
  return 'gray';
}

function roleActionLabel(role) {
  if (role === 'admin') return '提升为管理员';
  if (role === 'pending') return '设为待激活';
  return '激活为用户';
}
