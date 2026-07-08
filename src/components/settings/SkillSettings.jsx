import React from 'react';
import { ArrowLeft, Check, Pencil, Plus, Trash2, Upload } from 'lucide-react';
import {
  Badge,
  Button,
  CardActions,
  EmptyCard,
  IconButton,
  ProviderCard,
  SettingsSection,
  SectionTitle,
  formatDate,
} from './SettingsUI';

export default function SkillSettings({ ide, dialog, onBack }) {
  const [candidates, setCandidates] = React.useState(null);
  const [selectedCandidateIds, setSelectedCandidateIds] = React.useState(new Set());
  const [selectedSkillId, setSelectedSkillId] = React.useState('');
  const [selectedSkillDetail, setSelectedSkillDetail] = React.useState(null);
  const [skillPreview, setSkillPreview] = React.useState(null);
  const [view, setView] = React.useState('list');
  const selectedSkill = ide.skills.find((item) => item.id === selectedSkillId) || null;
  const showUploadMessage = React.useCallback((message) => {
    if (message) ide.showToast?.(message);
  }, [ide.showToast]);

  const openDetail = async (item) => {
    setSelectedSkillId(item.id);
    setView('detail');
    setSkillPreview(null);
    const [detail, preview] = await Promise.all([ide.getSkill(item.id), ide.readSkill(item.id)]);
    setSelectedSkillDetail(detail);
    setSkillPreview(preview);
  };

  const uploadSkill = async (file) => {
    if (!file) return;
    showUploadMessage('');
    setCandidates(null);
    const result = await ide.uploadSkillZip(file);
    if (result.candidates?.length > 0) {
      setCandidates(result);
      setSelectedCandidateIds(new Set(result.candidates.map((item) => item.id)));
      showUploadMessage(result.message || '发现多个 Skill，请选择安装。');
    } else {
      showUploadMessage(result.message || 'Skill 已安装。');
    }
  };

  const installCandidates = async () => {
    if (!candidates) return;
    const ids = Array.from(selectedCandidateIds);
    await ide.installSelectedSkills(candidates.upload_id, ids);
    setCandidates(null);
    setSelectedCandidateIds(new Set());
    showUploadMessage('已安装选中的 Skill。');
  };

  const renameSkill = async (item) => {
    const nextName = await dialog?.prompt?.({
      title: '重命名 Skill',
      message: '输入新的 Skill 名称。',
      initialValue: item.name,
      confirmLabel: '保存名称',
    });
    if (!nextName || nextName.trim() === item.name) return;
    await ide.updateSkill(item.id, { name: nextName.trim(), description: item.description, visibility: item.visibility, enabled: item.enabled });
    if (selectedSkillId === item.id) {
      const detail = await ide.getSkill(item.id);
      setSelectedSkillDetail(detail);
    }
  };

  const deleteSkill = async (item) => {
    const ok = await dialog?.confirm?.({
      title: '删除 Skill',
      message: `确定删除 Skill「${item.name}」吗？这只会删除 Skill 记录和安装目录，不会影响 workspace。`,
      confirmLabel: '删除 Skill',
      tone: 'red',
    });
    if (!ok) return;
    await ide.deleteSkill(item.id);
    if (selectedSkillId === item.id) {
      setSelectedSkillId('');
      setSelectedSkillDetail(null);
      setSkillPreview(null);
      setView('list');
    }
  };

  const toggleSkill = async (item) => {
    await ide.updateSkill(item.id, { name: item.name, description: item.description, visibility: item.visibility, enabled: !item.enabled });
    if (selectedSkillId === item.id) {
      const detail = await ide.getSkill(item.id);
      setSelectedSkillDetail(detail);
    }
  };

  const toggleVisibility = async (item) => {
    await ide.updateSkill(item.id, {
      name: item.name,
      description: item.description,
      visibility: item.visibility === 'public' ? 'private' : 'public',
      enabled: item.enabled,
    });
    if (selectedSkillId === item.id) {
      const detail = await ide.getSkill(item.id);
      setSelectedSkillDetail(detail);
    }
  };

  if (view === 'detail') {
    return (
      <SkillDetailView
        skill={selectedSkillDetail?.skill || selectedSkill}
        tree={selectedSkillDetail?.tree || []}
        preview={skillPreview}
        onBack={() => setView('list')}
        onToggle={toggleSkill}
        onToggleVisibility={toggleVisibility}
        onRename={renameSkill}
        onDelete={deleteSkill}
      />
    );
  }

  return (
    <SkillsListView
      ide={ide}
      onBack={onBack}
      onUpload={uploadSkill}
      candidates={candidates}
      selectedCandidateIds={selectedCandidateIds}
      setSelectedCandidateIds={setSelectedCandidateIds}
      onInstallCandidates={installCandidates}
      onOpenDetail={openDetail}
      onToggle={toggleSkill}
      onToggleVisibility={toggleVisibility}
      onRename={renameSkill}
      onDelete={deleteSkill}
    />
  );
}

function SkillsListView({
  ide,
  onBack,
  onUpload,
  candidates,
  selectedCandidateIds,
  setSelectedCandidateIds,
  onInstallCandidates,
  onOpenDetail,
  onToggle,
  onToggleVisibility,
  onRename,
  onDelete,
}) {
  return (
    <SettingsSection className="settings-view-enter">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft size={16} /> Skills
        </Button>
        <Button as="label" variant="primary" size="md" className="cursor-pointer">
          <input
            type="file"
            accept=".zip"
            className="hidden"
            onChange={(e) => onUpload(e.target.files?.[0])}
          />
          <Upload size={15} /> 上传 skill.zip
        </Button>
      </div>

      {candidates?.candidates?.length > 0 && (
        <div className="settings-expand-enter mb-4 rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
          <div className="mb-3 text-sm font-bold text-gray-800 dark:text-gray-100">选择要安装的 Skill</div>
          <div className="space-y-2">
            {candidates.candidates.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => {
                  const next = new Set(selectedCandidateIds);
                  if (next.has(item.id)) next.delete(item.id);
                  else next.add(item.id);
                  setSelectedCandidateIds(next);
                }}
                className="motion-row flex w-full min-w-0 items-center gap-3 rounded-xl border border-gray-100 bg-gray-50 p-3 text-left dark:border-white/10 dark:bg-white/5"
              >
                <span className={`candidate-check flex h-4 w-4 shrink-0 items-center justify-center rounded-md border ${selectedCandidateIds.has(item.id) ? 'border-blue-500 bg-blue-500 text-white' : 'border-gray-300 dark:border-gray-600'}`}>
                  {selectedCandidateIds.has(item.id) && <Check size={12} />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-bold text-gray-800 dark:text-gray-100">{item.name}</span>
                  <span className="block truncate text-xs text-gray-400">{item.source_path || 'zip 根目录'}</span>
                </span>
              </button>
            ))}
          </div>
          <Button
            type="button"
            onClick={onInstallCandidates}
            disabled={selectedCandidateIds.size === 0}
            className="mt-4"
          >
            安装选中项
          </Button>
        </div>
      )}

      <div className="space-y-3">
        {ide.skills.length === 0 ? (
          <EmptyCard text="还没有安装 Skill。上传包含 SKILL.md 的 zip 即可安装。" />
        ) : (
          ide.skills.map((item) => (
            <ProviderCard key={item.id}>
              <button type="button" onClick={() => onOpenDetail(item)} className="motion-row min-w-0 flex-1 rounded-xl text-left">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-bold text-gray-800 dark:text-gray-100">{item.name}</span>
                  <Badge tone={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '禁用'}</Badge>
                  <Badge tone={item.visibility === 'public' ? 'blue' : 'gray'}>{item.visibility === 'public' ? '公开' : '私密'}</Badge>
                </div>
                <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{item.description || '暂无简介'}</div>
                <div className="mt-1 text-xs text-gray-400">更新 {formatDate(item.updated_at)}</div>
              </button>
              <CardActions>
                <IconButton title={item.enabled ? '禁用' : '启用'} onClick={() => onToggle(item)}>
                  <Check size={14} />
                </IconButton>
                <Button type="button" variant="ghost" size="xs" onClick={() => onToggleVisibility(item)}>
                  {item.visibility === 'public' ? '设为私密' : '设为公开'}
                </Button>
                <IconButton title="重命名" onClick={() => onRename(item)}>
                  <Pencil size={14} />
                </IconButton>
                <IconButton title="删除" danger onClick={() => onDelete(item)}>
                  <Trash2 size={14} />
                </IconButton>
              </CardActions>
            </ProviderCard>
          ))
        )}
      </div>
    </SettingsSection>
  );
}

function SkillDetailView({ skill, tree, preview, onBack, onToggle, onToggleVisibility, onRename, onDelete }) {
  if (!skill) {
    return (
      <SettingsSection className="settings-view-enter">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft size={16} /> 返回
        </Button>
        <div className="mt-6 text-sm text-gray-400">未选择 Skill</div>
      </SettingsSection>
    );
  }
  return (
    <SettingsSection className="settings-view-enter">
      <div className="mb-5 flex items-start justify-between gap-4">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft size={16} /> 返回 Skills
        </Button>
        <CardActions>
          <IconButton title={skill.enabled ? '禁用' : '启用'} onClick={() => onToggle(skill)}>
            <Check size={14} />
          </IconButton>
          <Button type="button" variant="ghost" size="xs" onClick={() => onToggleVisibility(skill)}>
            {skill.visibility === 'public' ? '设为私密' : '设为公开'}
          </Button>
          <IconButton title="重命名" onClick={() => onRename(skill)}>
            <Pencil size={14} />
          </IconButton>
          <IconButton title="删除" danger onClick={() => onDelete(skill)}>
            <Trash2 size={14} />
          </IconButton>
        </CardActions>
      </div>

      <div className="settings-card min-w-0 rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
        <div className="flex min-w-0 items-center gap-2">
          <h3 className="truncate text-lg font-bold text-gray-800 dark:text-gray-100">{skill.name}</h3>
          <Badge tone={skill.enabled ? 'green' : 'gray'}>{skill.enabled ? '启用' : '禁用'}</Badge>
          <Badge tone={skill.visibility === 'public' ? 'blue' : 'gray'}>{skill.visibility === 'public' ? '公开' : '私密'}</Badge>
        </div>
        <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">{skill.description || '暂无简介'}</p>
        <div className="mt-3 space-y-1 text-xs text-gray-400">
          <div className="truncate">install_path: {skill.install_path}</div>
          <div className="truncate">source_path: {skill.source_path || '-'}</div>
        </div>
      </div>

      <div className="mt-4 grid min-h-0 gap-4 xl:grid-cols-[minmax(0,1fr)_260px]">
        <div className="settings-card min-h-[360px] overflow-hidden rounded-2xl border border-gray-100 bg-gray-950 dark:border-white/10">
          <pre className="h-[420px] overflow-auto p-4 text-xs leading-5 text-gray-200 custom-scrollbar">{preview?.content || '正在读取 SKILL.md...'}</pre>
        </div>
        <div className="settings-card rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
          <div className="mb-3 text-sm font-bold text-gray-800 dark:text-gray-100">目录结构</div>
          <div className="max-h-[420px] overflow-auto text-xs text-gray-500 custom-scrollbar dark:text-gray-400">
            {(tree || []).length === 0 ? '暂无目录信息' : tree.map((item) => <div key={item} className="truncate py-0.5">{item}</div>)}
          </div>
        </div>
      </div>
    </SettingsSection>
  );
}
