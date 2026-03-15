import React, { useState, useEffect } from 'react';
import { WhitelistConfig, Policy, Command, RiskAssessment } from './types';

interface CommandWhitelistPanelProps {
  onSave?: () => void;
}

const CommandWhitelistPanel: React.FC<CommandWhitelistPanelProps> = ({ onSave }) => {
  const [config, setConfig] = useState<WhitelistConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState('');
  const [expandedPolicy, setExpandedPolicy] = useState<string | null>(null);
  const [editingPolicy, setEditingPolicy] = useState<Policy | null>(null);
  const [testingCommand, setTestingCommand] = useState('');
  const [testResult, setTestResult] = useState<RiskAssessment | null>(null);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    setLoading(true);
    try {
      // @ts-ignore
      const result = await window.go.main.App.GetCommandWhitelist();
      setConfig(result);
    } catch (err) {
      setMsg(`加载配置失败: ${err}`);
      // 使用默认配置
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setMsg('');
    try {
      // @ts-ignore
      await window.go.main.App.SaveCommandWhitelist(config);
      setMsg('保存成功');
      onSave?.();
    } catch (err) {
      setMsg(`保存失败: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const handleToggleLLMCheck = () => {
    if (!config) return;
    setConfig({ ...config, llm_check_enabled: !config.llm_check_enabled });
  };

  const handleAddPolicy = () => {
    if (!config) return;
    const newPolicy: Policy = {
      id: `policy-${Date.now()}`,
      name: '新策略',
      description: '',
      ip_ranges: [],
      commands: [],
    };
    setConfig({
      ...config,
      policies: [...config.policies, newPolicy],
    });
    setEditingPolicy(newPolicy);
  };

  const handleDeletePolicy = (policyId: string) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.filter(p => p.id !== policyId),
    });
  };

  const handleUpdatePolicy = (updatedPolicy: Policy) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.map(p => p.id === updatedPolicy.id ? updatedPolicy : p),
    });
    setEditingPolicy(null);
  };

  const handleTestCommand = async () => {
    if (!testingCommand.trim()) return;
    setTesting(true);
    setTestResult(null);
    try {
      // @ts-ignore
      const result = await window.go.main.App.AssessCommandRisk(testingCommand);
      setTestResult(result);
    } catch (err) {
      setTestResult({
        is_risky: true,
        risk_level: 'high',
        reason: `评估失败: ${err}`,
        suggestions: '',
      });
    } finally {
      setTesting(false);
    }
  };

  const toggleCommand = (policyId: string, cmdIndex: number) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.map(p => {
        if (p.id === policyId) {
          const newCommands = [...p.commands];
          newCommands[cmdIndex] = { ...newCommands[cmdIndex], enabled: !newCommands[cmdIndex].enabled };
          return { ...p, commands: newCommands };
        }
        return p;
      }),
    });
  };

  const getRiskColor = (level: string) => {
    switch (level) {
      case 'low': return '#4caf50';
      case 'medium': return '#ff9800';
      case 'high': return '#f44336';
      default: return '#9e9e9e';
    }
  };

  const getCategoryColor = (category: string) => {
    return category === 'read_only' ? '#4caf50' : '#ff9800';
  };

  if (loading) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  if (!config) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>无法加载配置</div>;
  }

  return (
    <div style={{ padding: '16px' }}>
      {/* 全局设置 */}
      <div style={{
        marginBottom: '20px',
        padding: '16px',
        background: '#f5f5f5',
        borderRadius: '8px',
      }}>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}>
          <div>
            <span style={{ fontWeight: 'bold' }}>LLM 风险检查</span>
            <span style={{
              marginLeft: '8px',
              color: '#666',
              fontSize: '13px',
            }}>
              对未知命令使用 LLM 进行风险评估
            </span>
          </div>
          <button
            onClick={handleToggleLLMCheck}
            style={{
              padding: '8px 16px',
              background: config.llm_check_enabled ? '#4caf50' : '#ccc',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            {config.llm_check_enabled ? '已启用' : '已禁用'}
          </button>
        </div>
      </div>

      {/* 命令测试区域 */}
      <div style={{
        marginBottom: '20px',
        padding: '16px',
        background: '#fff3e0',
        borderRadius: '8px',
        border: '1px solid #ffe0b2',
      }}>
        <div style={{ fontWeight: 'bold', marginBottom: '12px' }}>
          🔍 命令风险测试
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <input
            type="text"
            value={testingCommand}
            onChange={(e) => setTestingCommand(e.target.value)}
            placeholder="输入命令进行风险评估..."
            style={{
              flex: 1,
              padding: '8px 12px',
              border: '1px solid #ddd',
              borderRadius: '4px',
              fontSize: '14px',
            }}
          />
          <button
            onClick={handleTestCommand}
            disabled={testing || !testingCommand.trim()}
            style={{
              padding: '8px 16px',
              background: '#2196f3',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: testing ? 'wait' : 'pointer',
            }}
          >
            {testing ? '评估中...' : '评估'}
          </button>
        </div>
        {testResult && (
          <div style={{
            marginTop: '12px',
            padding: '12px',
            background: '#fff',
            borderRadius: '4px',
            border: `1px solid ${getRiskColor(testResult.risk_level)}`,
          }}>
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              marginBottom: '8px',
            }}>
              <span style={{
                padding: '2px 8px',
                background: getRiskColor(testResult.risk_level),
                color: 'white',
                borderRadius: '4px',
                fontSize: '12px',
              }}>
                {testResult.risk_level.toUpperCase()}
              </span>
              <span>{testResult.reason}</span>
            </div>
            {testResult.suggestions && (
              <div style={{ color: '#666', fontSize: '13px' }}>
                💡 {testResult.suggestions}
              </div>
            )}
          </div>
        )}
      </div>

      {/* 策略列表 */}
      <div style={{ marginBottom: '16px' }}>
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <h3 style={{ margin: 0 }}>策略列表</h3>
          <button
            onClick={handleAddPolicy}
            style={{
              padding: '8px 16px',
              background: '#2196f3',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            + 添加策略
          </button>
        </div>
      </div>

      {config.policies.map((policy) => (
        <div
          key={policy.id}
          style={{
            marginBottom: '12px',
            border: '1px solid #e0e0e0',
            borderRadius: '8px',
            overflow: 'hidden',
          }}
        >
          {/* 策略头部 */}
          <div
            onClick={() => setExpandedPolicy(expandedPolicy === policy.id ? null : policy.id)}
            style={{
              padding: '12px 16px',
              background: '#fafafa',
              cursor: 'pointer',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
            }}
          >
            <div>
              <div style={{ fontWeight: 'bold' }}>{policy.name}</div>
              <div style={{ fontSize: '12px', color: '#666' }}>
                IP 段: {policy.ip_ranges.length > 0 ? policy.ip_ranges.join(', ') : '未配置'}
                <span style={{ margin: '0 8px' }}>|</span>
                命令: {policy.commands.filter(c => c.enabled).length}/{policy.commands.length}
              </div>
            </div>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setEditingPolicy(policy);
                }}
                style={{
                  padding: '4px 8px',
                  background: '#fff',
                  border: '1px solid #ddd',
                  borderRadius: '4px',
                  cursor: 'pointer',
                }}
              >
                编辑
              </button>
              {policy.id !== 'default' && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDeletePolicy(policy.id);
                  }}
                  style={{
                    padding: '4px 8px',
                    background: '#fff',
                    border: '1px solid #f44336',
                    color: '#f44336',
                    borderRadius: '4px',
                    cursor: 'pointer',
                  }}
                >
                  删除
                </button>
              )}
              <span style={{ fontSize: '16px' }}>
                {expandedPolicy === policy.id ? '▼' : '▶'}
              </span>
            </div>
          </div>

          {/* 策略详情（展开时显示） */}
          {expandedPolicy === policy.id && (
            <div style={{ padding: '16px', background: '#fff' }}>
              <div style={{ fontSize: '14px', color: '#666', marginBottom: '12px' }}>
                {policy.description}
              </div>
              <div style={{ marginBottom: '8px', fontWeight: 'bold' }}>
                命令规则:
              </div>
              {policy.commands.length === 0 ? (
                <div style={{ color: '#999', padding: '8px' }}>
                  暂无命令规则
                </div>
              ) : (
                <div style={{
                  maxHeight: '200px',
                  overflowY: 'auto',
                  border: '1px solid #eee',
                  borderRadius: '4px',
                }}>
                  {policy.commands.map((cmd, idx) => (
                    <div
                      key={idx}
                      style={{
                        padding: '8px 12px',
                        borderBottom: idx < policy.commands.length - 1 ? '1px solid #f0f0f0' : 'none',
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        background: cmd.enabled ? '#fff' : '#f9f9f9',
                      }}
                    >
                      <div style={{ flex: 1 }}>
                        <code style={{ fontSize: '13px' }}>{cmd.pattern}</code>
                        <span style={{
                          marginLeft: '8px',
                          padding: '2px 6px',
                          background: getCategoryColor(cmd.category),
                          color: 'white',
                          borderRadius: '3px',
                          fontSize: '11px',
                        }}>
                          {cmd.category === 'read_only' ? '只读' : '写入'}
                        </span>
                        <span style={{ marginLeft: '8px', color: '#666', fontSize: '12px' }}>
                          {cmd.description}
                        </span>
                      </div>
                      <button
                        onClick={() => toggleCommand(policy.id, idx)}
                        style={{
                          padding: '4px 8px',
                          background: cmd.enabled ? '#4caf50' : '#ccc',
                          color: 'white',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: 'pointer',
                          fontSize: '12px',
                        }}
                      >
                        {cmd.enabled ? '启用' : '禁用'}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      ))}

      {/* 保存按钮 */}
      <div style={{
        marginTop: '24px',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        alignItems: 'center',
      }}>
        {msg && (
          <span style={{
            color: msg.includes('成功') ? '#4caf50' : '#f44336',
            fontSize: '14px',
          }}>
            {msg}
          </span>
        )}
        <button
          onClick={handleSave}
          disabled={saving}
          style={{
            padding: '10px 24px',
            background: '#4caf50',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: saving ? 'wait' : 'pointer',
            fontSize: '14px',
          }}
        >
          {saving ? '保存中...' : '保存配置'}
        </button>
      </div>

      {/* 策略编辑模态框 */}
      {editingPolicy && (
        <PolicyEditor
          policy={editingPolicy}
          onSave={handleUpdatePolicy}
          onCancel={() => setEditingPolicy(null)}
        />
      )}
    </div>
  );
};

// 简单的策略编辑器组件
const PolicyEditor: React.FC<{
  policy: Policy;
  onSave: (policy: Policy) => void;
  onCancel: () => void;
}> = ({ policy, onSave, onCancel }) => {
  const [editing, setEditing] = useState<Policy>({ ...policy });
  const [newIPRange, setNewIPRange] = useState('');
  const [newCommand, setNewCommand] = useState<Partial<Command>>({
    pattern: '',
    category: 'read_only',
    description: '',
    enabled: true,
  });

  const handleAddIPRange = () => {
    if (!newIPRange.trim()) return;
    setEditing({
      ...editing,
      ip_ranges: [...editing.ip_ranges, newIPRange.trim()],
    });
    setNewIPRange('');
  };

  const handleRemoveIPRange = (index: number) => {
    setEditing({
      ...editing,
      ip_ranges: editing.ip_ranges.filter((_, i) => i !== index),
    });
  };

  const handleAddCommand = () => {
    if (!newCommand.pattern?.trim()) return;
    setEditing({
      ...editing,
      commands: [...editing.commands, newCommand as Command],
    });
    setNewCommand({
      pattern: '',
      category: 'read_only',
      description: '',
      enabled: true,
    });
  };

  const handleRemoveCommand = (index: number) => {
    setEditing({
      ...editing,
      commands: editing.commands.filter((_, i) => i !== index),
    });
  };

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(0,0,0,0.5)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 1000,
    }}>
      <div style={{
        background: 'white',
        borderRadius: '8px',
        padding: '24px',
        width: '600px',
        maxHeight: '80vh',
        overflowY: 'auto',
      }}>
        <h3 style={{ marginTop: 0 }}>编辑策略</h3>

        {/* 基本信息 */}
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', marginBottom: '4px', fontWeight: 'bold' }}>
            策略名称
          </label>
          <input
            type="text"
            value={editing.name}
            onChange={(e) => setEditing({ ...editing, name: e.target.value })}
            style={{
              width: '100%',
              padding: '8px',
              border: '1px solid #ddd',
              borderRadius: '4px',
            }}
          />
        </div>

        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', marginBottom: '4px', fontWeight: 'bold' }}>
            描述
          </label>
          <input
            type="text"
            value={editing.description}
            onChange={(e) => setEditing({ ...editing, description: e.target.value })}
            style={{
              width: '100%',
              padding: '8px',
              border: '1px solid #ddd',
              borderRadius: '4px',
            }}
          />
        </div>

        {/* IP 段配置 */}
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', marginBottom: '8px', fontWeight: 'bold' }}>
            IP 段（每行一个，支持 CIDR 格式如 192.168.1.0/24 或 * 表示所有）
          </label>
          <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
            <input
              type="text"
              value={newIPRange}
              onChange={(e) => setNewIPRange(e.target.value)}
              placeholder="例如: 192.168.1.0/24 或 *"
              style={{
                flex: 1,
                padding: '8px',
                border: '1px solid #ddd',
                borderRadius: '4px',
              }}
            />
            <button
              onClick={handleAddIPRange}
              style={{
                padding: '8px 16px',
                background: '#2196f3',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
              }}
            >
              添加
            </button>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
            {editing.ip_ranges.map((ip, idx) => (
              <span
                key={idx}
                style={{
                  padding: '4px 8px',
                  background: '#e3f2fd',
                  borderRadius: '4px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                }}
              >
                {ip}
                <button
                  onClick={() => handleRemoveIPRange(idx)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: '#f44336',
                  }}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        </div>

        {/* 命令规则 */}
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', marginBottom: '8px', fontWeight: 'bold' }}>
            命令规则
          </label>
          <div style={{
            border: '1px solid #eee',
            borderRadius: '4px',
            marginBottom: '12px',
            maxHeight: '200px',
            overflowY: 'auto',
          }}>
            {editing.commands.map((cmd, idx) => (
              <div
                key={idx}
                style={{
                  padding: '8px 12px',
                  borderBottom: idx < editing.commands.length - 1 ? '1px solid #f0f0f0' : 'none',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                }}
              >
                <div>
                  <code style={{ fontSize: '12px' }}>{cmd.pattern}</code>
                  <span style={{
                    marginLeft: '8px',
                    fontSize: '11px',
                    padding: '2px 4px',
                    background: cmd.category === 'read_only' ? '#e8f5e9' : '#fff3e0',
                    color: cmd.category === 'read_only' ? '#2e7d32' : '#e65100',
                    borderRadius: '3px',
                  }}>
                    {cmd.category === 'read_only' ? '只读' : '写入'}
                  </span>
                </div>
                <button
                  onClick={() => handleRemoveCommand(idx)}
                  style={{
                    padding: '2px 8px',
                    background: '#fff',
                    border: '1px solid #f44336',
                    color: '#f44336',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '12px',
                  }}
                >
                  删除
                </button>
              </div>
            ))}
            {editing.commands.length === 0 && (
              <div style={{ padding: '12px', color: '#999', textAlign: 'center' }}>
                暂无命令规则
              </div>
            )}
          </div>

          {/* 添加新命令 */}
          <div style={{
            background: '#f9f9f9',
            padding: '12px',
            borderRadius: '4px',
          }}>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
              <input
                type="text"
                value={newCommand.pattern || ''}
                onChange={(e) => setNewCommand({ ...newCommand, pattern: e.target.value })}
                placeholder="正则表达式，如: ^ls(\\s|$)"
                style={{
                  flex: 1,
                  padding: '8px',
                  border: '1px solid #ddd',
                  borderRadius: '4px',
                  fontFamily: 'monospace',
                }}
              />
              <select
                value={newCommand.category || 'read_only'}
                onChange={(e) => setNewCommand({ ...newCommand, category: e.target.value as 'read_only' | 'write' })}
                style={{
                  padding: '8px',
                  border: '1px solid #ddd',
                  borderRadius: '4px',
                }}
              >
                <option value="read_only">只读</option>
                <option value="write">写入</option>
              </select>
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text"
                value={newCommand.description || ''}
                onChange={(e) => setNewCommand({ ...newCommand, description: e.target.value })}
                placeholder="命令描述"
                style={{
                  flex: 1,
                  padding: '8px',
                  border: '1px solid #ddd',
                  borderRadius: '4px',
                }}
              />
              <button
                onClick={handleAddCommand}
                disabled={!newCommand.pattern?.trim()}
                style={{
                  padding: '8px 16px',
                  background: newCommand.pattern?.trim() ? '#4caf50' : '#ccc',
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: newCommand.pattern?.trim() ? 'pointer' : 'not-allowed',
                }}
              >
                添加命令
              </button>
            </div>
          </div>
        </div>

        {/* 操作按钮 */}
        <div style={{
          display: 'flex',
          justifyContent: 'flex-end',
          gap: '12px',
          marginTop: '24px',
        }}>
          <button
            onClick={onCancel}
            style={{
              padding: '10px 20px',
              background: '#fff',
              border: '1px solid #ddd',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            取消
          </button>
          <button
            onClick={() => onSave(editing)}
            style={{
              padding: '10px 20px',
              background: '#2196f3',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            保存
          </button>
        </div>
      </div>
    </div>
  );
};

export default CommandWhitelistPanel;
