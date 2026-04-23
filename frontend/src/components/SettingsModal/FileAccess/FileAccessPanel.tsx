import React, { useState, useEffect, useRef } from 'react';
import { FileAccessConfig, FileAccessPolicy } from './types';

const FileAccessPanel: React.FC = () => {
  const [config, setConfig] = useState<FileAccessConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [expandedPolicy, setExpandedPolicy] = useState<string | null>(null);
  const [editingPolicy, setEditingPolicy] = useState<FileAccessPolicy | null>(null);

  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const initialLoadCompleteRef = useRef(false);
  const prevConfigRef = useRef<string>('');

  useEffect(() => {
    loadConfig();
  }, []);

  // 自动保存（防抖）
  useEffect(() => {
    if (!initialLoadCompleteRef.current) return;
    if (!config) return;

    const configStr = JSON.stringify(config);
    if (configStr === prevConfigRef.current) return;
    prevConfigRef.current = configStr;

    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }

    setSaveStatus('saving');
    saveTimeoutRef.current = setTimeout(async () => {
      try {
        // @ts-ignore
        await window.go.main.App.SaveFileAccessConfig(config);
        setSaveStatus('saved');
        setTimeout(() => setSaveStatus('idle'), 3000);
      } catch (err) {
        console.error('自动保存失败:', err);
        setSaveStatus('error');
      }
    }, 500);

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current);
      }
    };
  }, [config]);

  const loadConfig = async () => {
    setLoading(true);
    try {
      // @ts-ignore
      const result = await window.go.main.App.GetFileAccessConfig();
      setConfig(result);
      prevConfigRef.current = JSON.stringify(result);
      initialLoadCompleteRef.current = true;
    } catch (err) {
      console.error('加载配置失败:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleAddPolicy = () => {
    if (!config) return;
    const newPolicy: FileAccessPolicy = {
      id: `policy-${Date.now()}`,
      name: '新策略',
      ip_ranges: [],
      read_paths: [],
      write_paths: [],
      denied_paths: [],
      allowed_local_dirs: ['/tmp/opscopilot-mcp/'],
      max_read_bytes: 10 * 1024 * 1024,
      max_write_bytes: 5 * 1024 * 1024,
    };
    setConfig({ ...config, policies: [...config.policies, newPolicy] });
    setEditingPolicy(newPolicy);
  };

  const handleDeletePolicy = (policyId: string) => {
    if (!config) return;
    setConfig({ ...config, policies: config.policies.filter(p => p.id !== policyId) });
  };

  const handleUpdatePolicy = (updatedPolicy: FileAccessPolicy) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.map(p => p.id === updatedPolicy.id ? updatedPolicy : p),
    });
    setEditingPolicy(null);
  };

  const formatBytes = (bytes: number) => {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)}MB`;
    if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)}KB`;
    return `${bytes}B`;
  };

  if (loading) {
    return <div style={styles.loading}>加载中...</div>;
  }

  if (!config) {
    return <div style={styles.loading}>无法加载配置</div>;
  }

  return (
    <div style={styles.container}>
      {/* 说明 */}
      <div style={styles.section}>
        <div style={styles.sectionTitle}>文件访问控制</div>
        <div style={styles.sectionDesc}>
          控制 AI Agent 通过 file_download / file_upload 工具操作文件的权限范围。
          读取路径和写入路径基于远程路径前缀匹配，拒绝路径优先级最高。
        </div>
      </div>

      {/* 策略列表 */}
      <div style={styles.section}>
        <div style={styles.toolbar}>
          <span style={styles.sectionTitle}>策略列表</span>
          <button onClick={handleAddPolicy} style={styles.primaryBtn}>
            + 添加策略
          </button>
        </div>

        <div style={styles.policyList}>
          {config.policies.map((policy) => (
            <div key={policy.id} style={styles.policyItem}>
              {/* 策略头部 */}
              <div
                onClick={() => setExpandedPolicy(expandedPolicy === policy.id ? null : policy.id)}
                style={styles.policyHeader}
              >
                <div style={styles.policyInfo}>
                  <div style={styles.policyName}>{policy.name}</div>
                  <div style={styles.policyMeta}>
                    <span>IP: {policy.ip_ranges.length > 0 ? policy.ip_ranges.join(', ') : '未配置'}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>读取: {policy.read_paths.length}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>写入: {policy.write_paths.length}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>拒绝: {policy.denied_paths.length}</span>
                  </div>
                  <div style={styles.policyMetaSecondary}>
                    <span>本地目录: {policy.allowed_local_dirs.join(', ')}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>下载上限: {formatBytes(policy.max_read_bytes)}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>上传上限: {formatBytes(policy.max_write_bytes)}</span>
                  </div>
                </div>
                <div style={styles.policyActions}>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setEditingPolicy(policy);
                    }}
                    style={styles.editBtn}
                  >
                    编辑
                  </button>
                  {policy.id !== 'default' && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeletePolicy(policy.id);
                      }}
                      style={styles.deleteBtn}
                    >
                      删除
                    </button>
                  )}
                  <span style={styles.expandIcon}>
                    {expandedPolicy === policy.id ? '▾' : '▸'}
                  </span>
                </div>
              </div>

              {/* 策略详情 */}
              {expandedPolicy === policy.id && (
                <div style={styles.policyContent}>
                  <div style={styles.detailGrid}>
                    <div style={styles.detailSection}>
                      <div style={styles.detailTitle}>读取路径</div>
                      {policy.read_paths.length === 0 ? (
                        <div style={styles.emptyText}>无</div>
                      ) : (
                        <div style={styles.pathList}>
                          {policy.read_paths.map((p, i) => (
                            <code key={i} style={styles.pathTag}>{p}</code>
                          ))}
                        </div>
                      )}
                    </div>
                    <div style={styles.detailSection}>
                      <div style={styles.detailTitle}>写入路径</div>
                      {policy.write_paths.length === 0 ? (
                        <div style={styles.emptyText}>无（管理员需显式配置）</div>
                      ) : (
                        <div style={styles.pathList}>
                          {policy.write_paths.map((p, i) => (
                            <code key={i} style={styles.pathTag}>{p}</code>
                          ))}
                        </div>
                      )}
                    </div>
                    <div style={styles.detailSection}>
                      <div style={styles.detailTitle}>拒绝路径</div>
                      {policy.denied_paths.length === 0 ? (
                        <div style={styles.emptyText}>无</div>
                      ) : (
                        <div style={styles.pathList}>
                          {policy.denied_paths.map((p, i) => (
                            <code key={i} style={{ ...styles.pathTag, backgroundColor: '#3a2020', color: '#f06060' }}>{p}</code>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* 保存状态 */}
      {saveStatus !== 'idle' && (
        <div style={{
          ...styles.saveStatus,
          ...(saveStatus === 'saving' ? styles.saveStatusSaving : {}),
          ...(saveStatus === 'saved' ? styles.saveStatusSaved : {}),
          ...(saveStatus === 'error' ? styles.saveStatusError : {}),
        }}>
          {saveStatus === 'saving' && '保存中...'}
          {saveStatus === 'saved' && '已保存'}
          {saveStatus === 'error' && '保存失败'}
        </div>
      )}

      {/* 策略编辑模态框 */}
      {editingPolicy && (
        <FileAccessPolicyEditor
          policy={editingPolicy}
          onSave={handleUpdatePolicy}
          onCancel={() => setEditingPolicy(null)}
        />
      )}
    </div>
  );
};

// 策略编辑器组件
const FileAccessPolicyEditor: React.FC<{
  policy: FileAccessPolicy;
  onSave: (policy: FileAccessPolicy) => void;
  onCancel: () => void;
}> = ({ policy, onSave, onCancel }) => {
  const [editing, setEditing] = useState<FileAccessPolicy>({ ...policy });
  const [newIPRange, setNewIPRange] = useState('');
  const [newReadPath, setNewReadPath] = useState('');
  const [newWritePath, setNewWritePath] = useState('');
  const [newDeniedPath, setNewDeniedPath] = useState('');
  const [newLocalDir, setNewLocalDir] = useState('');

  const addTag = (field: keyof Pick<FileAccessPolicy, 'ip_ranges' | 'read_paths' | 'write_paths' | 'denied_paths' | 'allowed_local_dirs'>, value: string, setter: (v: string) => void) => {
    if (!value.trim()) return;
    setEditing({ ...editing, [field]: [...editing[field], value.trim()] });
    setter('');
  };

  const removeTag = (field: keyof Pick<FileAccessPolicy, 'ip_ranges' | 'read_paths' | 'write_paths' | 'denied_paths' | 'allowed_local_dirs'>, index: number) => {
    setEditing({
      ...editing,
      [field]: (editing[field] as string[]).filter((_, i) => i !== index),
    });
  };

  return (
    <div style={editorStyles.overlay}>
      <div style={editorStyles.modal}>
        <div style={editorStyles.header}>
          <h3 style={editorStyles.title}>编辑文件访问策略</h3>
          <button onClick={onCancel} style={editorStyles.closeBtn}>×</button>
        </div>

        <div style={editorStyles.body}>
          {/* 基本信息 */}
          <div style={editorStyles.field}>
            <label style={editorStyles.label}>策略名称</label>
            <input
              type="text"
              value={editing.name}
              onChange={(e) => setEditing({ ...editing, name: e.target.value })}
              style={editorStyles.input}
            />
          </div>

          {/* IP 段 */}
          <TagEditor
            label="IP 范围"
            hint="支持 CIDR 格式（如 192.168.1.0/24）或 * 表示所有"
            tags={editing.ip_ranges}
            newValue={newIPRange}
            onNewValueChange={setNewIPRange}
            onAdd={(v) => addTag('ip_ranges', v, setNewIPRange)}
            onRemove={(i) => removeTag('ip_ranges', i)}
          />

          {/* 读取路径 */}
          <TagEditor
            label="读取路径"
            hint="允许下载的远程路径前缀"
            tags={editing.read_paths}
            newValue={newReadPath}
            onNewValueChange={setNewReadPath}
            onAdd={(v) => addTag('read_paths', v, setNewReadPath)}
            onRemove={(i) => removeTag('read_paths', i)}
            placeholder="/var/log/"
          />

          {/* 写入路径 */}
          <TagEditor
            label="写入路径"
            hint="允许上传的远程路径前缀（默认为空，需显式配置）"
            tags={editing.write_paths}
            newValue={newWritePath}
            onNewValueChange={setNewWritePath}
            onAdd={(v) => addTag('write_paths', v, setNewWritePath)}
            onRemove={(i) => removeTag('write_paths', i)}
            placeholder="/tmp/"
            tagColor="#5a4a2e"
            tagTextColor="#f0c060"
          />

          {/* 拒绝路径 */}
          <TagEditor
            label="拒绝路径"
            hint="即使读取路径匹配也拒绝的路径（优先级最高）"
            tags={editing.denied_paths}
            newValue={newDeniedPath}
            onNewValueChange={setNewDeniedPath}
            onAdd={(v) => addTag('denied_paths', v, setNewDeniedPath)}
            onRemove={(i) => removeTag('denied_paths', i)}
            placeholder="/etc/shadow"
            tagColor="#3a2020"
            tagTextColor="#f06060"
          />

          {/* 本地目录 */}
          <TagEditor
            label="允许的本地目录"
            hint="Agent 文件操作只能在这些目录内进行"
            tags={editing.allowed_local_dirs}
            newValue={newLocalDir}
            onNewValueChange={setNewLocalDir}
            onAdd={(v) => addTag('allowed_local_dirs', v, setNewLocalDir)}
            onRemove={(i) => removeTag('allowed_local_dirs', i)}
            placeholder="/tmp/opscopilot-mcp/"
          />

          {/* 大小限制 */}
          <div style={editorStyles.fieldRow}>
            <div style={editorStyles.field}>
              <label style={editorStyles.label}>下载上限 (MB)</label>
              <input
                type="number"
                min="1"
                max="100"
                value={Math.round(editing.max_read_bytes / (1024 * 1024))}
                onChange={(e) => setEditing({
                  ...editing,
                  max_read_bytes: Math.max(1, parseInt(e.target.value) || 10) * 1024 * 1024,
                })}
                style={editorStyles.input}
              />
            </div>
            <div style={editorStyles.field}>
              <label style={editorStyles.label}>上传上限 (MB)</label>
              <input
                type="number"
                min="1"
                max="100"
                value={Math.round(editing.max_write_bytes / (1024 * 1024))}
                onChange={(e) => setEditing({
                  ...editing,
                  max_write_bytes: Math.max(1, parseInt(e.target.value) || 5) * 1024 * 1024,
                })}
                style={editorStyles.input}
              />
            </div>
          </div>
        </div>

        <div style={editorStyles.footer}>
          <button onClick={onCancel} style={editorStyles.cancelBtn}>取消</button>
          <button onClick={() => onSave(editing)} style={editorStyles.saveBtn}>保存</button>
        </div>
      </div>
    </div>
  );
};

// 标签编辑器子组件
const TagEditor: React.FC<{
  label: string;
  hint: string;
  tags: string[];
  newValue: string;
  onNewValueChange: (v: string) => void;
  onAdd: (v: string) => void;
  onRemove: (i: number) => void;
  placeholder?: string;
  tagColor?: string;
  tagTextColor?: string;
}> = ({ label, hint, tags, newValue, onNewValueChange, onAdd, onRemove, placeholder, tagColor, tagTextColor }) => (
  <div style={editorStyles.field}>
    <label style={editorStyles.label}>{label}</label>
    <div style={editorStyles.hint}>{hint}</div>
    <div style={editorStyles.inputRow}>
      <input
        type="text"
        value={newValue}
        onChange={(e) => onNewValueChange(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && onAdd(newValue)}
        placeholder={placeholder || ''}
        style={editorStyles.input}
      />
      <button onClick={() => onAdd(newValue)} style={editorStyles.addBtn}>添加</button>
    </div>
    <div style={editorStyles.tagList}>
      {tags.map((tag, idx) => (
        <span
          key={idx}
          style={{
            ...editorStyles.tag,
            ...(tagColor ? { backgroundColor: tagColor } : {}),
            ...(tagTextColor ? { color: tagTextColor } : {}),
          }}
        >
          {tag}
          <button onClick={() => onRemove(idx)} style={editorStyles.tagRemove}>×</button>
        </span>
      ))}
    </div>
  </div>
);

// 主面板样式
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
    padding: '8px 0',
  },
  loading: {
    padding: '40px',
    textAlign: 'center',
    color: '#888',
    fontSize: '14px',
  },
  section: {
    padding: '16px',
    backgroundColor: '#1e1e1e',
    borderRadius: '6px',
    border: '1px solid #3c3c3c',
  },
  sectionTitle: {
    color: '#fff',
    fontSize: '14px',
    fontWeight: 600,
  },
  sectionDesc: {
    color: '#888',
    fontSize: '12px',
    marginTop: '8px',
    lineHeight: 1.5,
  },
  toolbar: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  primaryBtn: {
    padding: '8px 16px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    fontWeight: 500,
  },
  policyList: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    marginTop: '12px',
  },
  policyItem: {
    border: '1px solid #3c3c3c',
    borderRadius: '6px',
    overflow: 'hidden',
    backgroundColor: '#252526',
  },
  policyHeader: {
    padding: '12px 16px',
    backgroundColor: '#2d2d2d',
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  policyInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    flex: 1,
  },
  policyName: {
    color: '#fff',
    fontSize: '13px',
    fontWeight: 500,
  },
  policyMeta: {
    color: '#888',
    fontSize: '11px',
    display: 'flex',
    flexWrap: 'wrap',
    gap: '2px',
  },
  policyMetaSecondary: {
    color: '#666',
    fontSize: '11px',
    display: 'flex',
    flexWrap: 'wrap',
    gap: '2px',
  },
  metaSeparator: {
    margin: '0 6px',
  },
  policyActions: {
    display: 'flex',
    gap: '8px',
    alignItems: 'center',
  },
  editBtn: {
    padding: '4px 10px',
    borderRadius: '4px',
    border: '1px solid #5a5a5a',
    backgroundColor: '#3c3c3c',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '12px',
  },
  deleteBtn: {
    padding: '4px 10px',
    borderRadius: '4px',
    border: '1px solid #5a3a3a',
    backgroundColor: 'transparent',
    color: '#f44336',
    cursor: 'pointer',
    fontSize: '12px',
  },
  expandIcon: {
    color: '#888',
    fontSize: '12px',
    marginLeft: '4px',
  },
  policyContent: {
    padding: '16px',
    borderTop: '1px solid #3c3c3c',
    backgroundColor: '#1e1e1e',
  },
  detailGrid: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
  },
  detailSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
  },
  detailTitle: {
    color: '#ccc',
    fontSize: '12px',
    fontWeight: 500,
  },
  pathList: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '6px',
  },
  pathTag: {
    padding: '2px 8px',
    backgroundColor: '#2d3a4a',
    borderRadius: '3px',
    color: '#9cdcfe',
    fontSize: '12px',
    fontFamily: 'var(--font-mono)',
  },
  emptyText: {
    color: '#666',
    fontSize: '12px',
  },
  saveStatus: {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    padding: '8px 16px',
    borderRadius: '20px',
    fontSize: '12px',
    fontWeight: 500,
    zIndex: 1000,
    transition: 'opacity 0.3s',
  },
  saveStatusSaving: {
    backgroundColor: '#2d3a4a',
    color: '#9cdcfe',
  },
  saveStatusSaved: {
    backgroundColor: '#1a2a24',
    color: '#4caf50',
  },
  saveStatusError: {
    backgroundColor: '#2a1818',
    color: '#f44336',
  },
};

// 编辑器模态框样式
const editorStyles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: 'rgba(0, 0, 0, 0.7)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 2100,
  },
  modal: {
    backgroundColor: '#252526',
    borderRadius: '8px',
    width: '580px',
    maxHeight: '80vh',
    display: 'flex',
    flexDirection: 'column',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
  },
  header: {
    padding: '16px 20px',
    borderBottom: '1px solid #3c3c3c',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: '#1e1e1e',
  },
  title: {
    margin: 0,
    fontSize: '15px',
    color: '#fff',
    fontWeight: 600,
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#ccc',
    fontSize: '20px',
    cursor: 'pointer',
    padding: '0',
    width: '28px',
    height: '28px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '4px',
  },
  body: {
    flex: 1,
    overflowY: 'auto',
    padding: '20px',
    display: 'flex',
    flexDirection: 'column',
    gap: '14px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
  },
  fieldRow: {
    display: 'flex',
    gap: '16px',
  },
  label: {
    color: '#ccc',
    fontSize: '13px',
    fontWeight: 500,
  },
  hint: {
    color: '#888',
    fontSize: '11px',
  },
  input: {
    padding: '8px 12px',
    borderRadius: '4px',
    border: '1px solid #3c3c3c',
    backgroundColor: '#1e1e1e',
    color: '#fff',
    outline: 'none',
    fontSize: '13px',
  },
  inputRow: {
    display: 'flex',
    gap: '8px',
  },
  addBtn: {
    padding: '8px 16px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    whiteSpace: 'nowrap',
  },
  tagList: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '6px',
    marginTop: '4px',
  },
  tag: {
    padding: '4px 8px',
    backgroundColor: '#2d3a4a',
    borderRadius: '4px',
    color: '#9cdcfe',
    fontSize: '12px',
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  tagRemove: {
    background: 'none',
    border: 'none',
    color: '#888',
    cursor: 'pointer',
    fontSize: '14px',
    padding: '0',
    lineHeight: 1,
  },
  footer: {
    padding: '16px 20px',
    borderTop: '1px solid #3c3c3c',
    display: 'flex',
    justifyContent: 'flex-end',
    gap: '12px',
    backgroundColor: '#1e1e1e',
  },
  cancelBtn: {
    padding: '8px 20px',
    borderRadius: '4px',
    border: '1px solid #5a5a5a',
    backgroundColor: 'transparent',
    color: '#ccc',
    cursor: 'pointer',
    fontSize: '13px',
  },
  saveBtn: {
    padding: '8px 20px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    fontWeight: 500,
  },
};

export default FileAccessPanel;
