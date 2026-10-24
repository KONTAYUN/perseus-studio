import React, { useEffect, useRef, useState } from 'react';
import { Check, FolderOpen, ImagePlus, Loader2, Upload, X } from 'lucide-react';
import { Button, Input, SegmentedControl } from '../../ui';
import { apiGet, apiPostForm } from '../../../api/client.js';
import { imageDisplayURL } from '../document/imagePaths.js';

const ACCEPT = 'image/png,image/jpeg,image/gif,image/webp,image/avif';
const MAX_BYTES = 16 * 1024 * 1024;

export default function DocumentImagePicker({ value, onChange, alt, onAltChange, onBusyChange, path }) {
  const input = useRef(null);
  const alive = useRef(true);
  const [mode, setMode] = useState('upload');
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [query, setQuery] = useState('');
  const [error, setError] = useState('');
  const [selectedName, setSelectedName] = useState('');
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => {
    if (mode !== 'workspace') return;
    const controller = new AbortController();
    setLoading(true); setError('');
    apiGet('/api/documents/images', { signal: controller.signal }).then(result => setItems(result.items || []))
      .catch(failure => { if (!controller.signal.aborted) setError(failure.message); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [mode]);

  async function upload(file) {
    if (!file || uploading) return;
    if (!ACCEPT.split(',').includes(file.type)) { setError('请选择 PNG、JPG、GIF、WebP 或 AVIF 图片。'); return; }
    if (file.size > MAX_BYTES) { setError('图片不能超过 16 MB，请压缩后重试。'); return; }
    setUploading(true); onBusyChange(true); setError('');
    try {
      const form = new FormData(); form.append('file', file);
      const result = await apiPostForm('/api/documents/images', form);
      if (alive.current) { onChange(result.src); setSelectedName(file.name); }
    } catch (failure) { if (alive.current) setError(failure.message); }
    finally { if (alive.current) { setUploading(false); onBusyChange(false); } }
  }
  const visible = items.filter(item => `${item.name} ${item.path}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()));
  const preview = imageDisplayURL(value, path);
  return <div className="document-image-picker">
    <SegmentedControl aria-label="图片来源" value={mode} onChange={setMode} options={[
      { value: 'upload', label: '上传图片' }, { value: 'workspace', label: '工作区图片' },
    ]} />
    <input ref={input} className="document-image-input" type="file" aria-label="上传图片文件" accept={ACCEPT} disabled={uploading}
      onChange={event => { upload(event.target.files?.[0]); event.target.value = ''; }} />
    {mode === 'upload' ? <div className="document-image-upload" data-dragging={dragging ? 'true' : 'false'}
      onDragOver={event => { event.preventDefault(); setDragging(true); }}
      onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget)) setDragging(false); }}
      onDrop={event => { event.preventDefault(); setDragging(false); upload(event.dataTransfer.files?.[0]); }}>
      {uploading ? <Loader2 size={25} className="animate-spin" /> : <ImagePlus size={27} strokeWidth={1.5} />}
      <strong>{uploading ? '正在上传图片…' : '将图片拖到这里'}</strong>
      <span>PNG、JPG、GIF、WebP、AVIF · 最大 16 MB</span>
      <Button variant="secondary" size="sm" disabled={uploading} onClick={() => input.current?.click()}><Upload size={14} />选择图片</Button>
    </div> : <div className="document-image-library">
      <Input aria-label="搜索工作区图片" placeholder="搜索图片名称…" value={query} onChange={event => setQuery(event.target.value)} />
      <div className="document-image-grid custom-scrollbar" aria-label="工作区图片列表" aria-busy={loading}>
        {loading ? <p className="document-image-empty" role="status"><Loader2 size={20} className="animate-spin" />正在加载图片…</p>
          : visible.length ? visible.map(item => <button type="button" key={item.path} className="motion-button document-image-item"
            aria-label={`选择 ${item.name}`} aria-pressed={value === item.src} disabled={uploading} onClick={() => { onChange(item.src); setSelectedName(item.name); }}>
            <span className="document-image-thumbnail"><img loading="lazy" src={imageDisplayURL(item.src)} alt="" />{value === item.src && <Check size={16} />}</span>
            <span title={item.path}>{item.name}</span>
          </button>) : <p className="document-image-empty"><FolderOpen size={23} strokeWidth={1.5} />{query ? '没有找到匹配的图片' : '工作区还没有图片'}<span>可以切换到“上传图片”添加。</span></p>}
      </div>
    </div>}
    {value && <div className="document-image-selection"><div className="document-image-selected-preview"><img src={preview || undefined} alt="所选图片预览" /></div>
      <div><strong>已选择图片</strong><span>{selectedName || (value.startsWith('http') ? '当前文稿中的图片' : decodeName(value))}</span></div>
      <Button variant="ghost" size="xs" aria-label="取消图片选择" disabled={uploading} onClick={() => onChange('')}><X size={15} /></Button>
    </div>}
    <label className="document-image-alt">图片说明 <span>可选，用于描述图片内容</span><Input aria-label="图片说明" placeholder="例如：产品首页的界面截图" value={alt} onChange={event => onAltChange(event.target.value)} /></label>
    {error && <p role="alert" className="document-image-error">{error}</p>}
  </div>;
}

function decodeName(value) {
  try { return decodeURIComponent(value.split('/').at(-1)); } catch { return '所选图片'; }
}
