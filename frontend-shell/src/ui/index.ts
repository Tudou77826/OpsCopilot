export { default as TerminalComponent } from './Terminal/Terminal';
export type { TerminalProps, TerminalRef } from './Terminal/Terminal';
export { default as FlexLayoutAdapter } from './FlexLayout/FlexLayoutAdapter';
export type { FlexLayoutAdapterProps, TerminalSession } from './FlexLayout/FlexLayoutAdapter';
export * from './appearance';
export * from './appearanceTypes';
export * from './ports';
export * from './terminalSchemes';
export * from './timestampParser';
export * from './types';
export * from './Terminal/highlightTypes';
export * from './Terminal/terminalAppearance';
export { assessPattern } from './Terminal/highlight/regexSafety';
export { ToastProvider, useToast } from './feedback/Toast';
export type { ToastContextValue, ToastType } from './feedback/Toast';
export { default as ConfirmDialogInternal, confirmDialog } from './feedback/ConfirmDialog';
export type { ConfirmChoice, ConfirmOptions } from './feedback/ConfirmDialog';
export { default as logoUniversal } from './assets/logo-universal.png';
// 阶段 3：会话树 / 连接表单 / 快捷命令（连接管理与快捷命令纵切）
export { default as SessionManager } from './session/SessionManager';
export type { SessionNode } from './ports';
export { default as EditSavedSessionModal } from './session/EditSavedSessionModal';
export { default as SharedSessionPanel } from './session/SharedSessionPanel';
export { default as ConnectionConfigForm } from './connection/ConnectionConfigForm';
export { default as SmartConnectModal } from './connection/SmartConnectModal';
export { default as QuickCommandPanel } from './quickcmd/QuickCommandPanel';
export { default as CommandEditModal } from './quickcmd/CommandEditModal';
export { useQuickCommands, MemoryAdapter } from './quickcmd/useQuickCommands';
export type { QuickCommandStorageAdapter, QuickCommandHost } from './ports';
export type {
  SessionManagerRuntime,
  SharedSessionRuntime,
  SharedSessionEntry,
  SharedConnectResult,
} from './ports';
// 阶段 4：文件传输纵切
export { default as FilesPanel } from './filetransfer/FilesPanel';
export type {
  FileTransferHost,
  FileTransferProgress,
  FileTransferDone,
  FileDropHandler,
} from './filetransfer/FilesPanel';
// 阶段 5：结构化脚本
export { default as ScriptRecordingPanel } from './script/ScriptRecordingPanel';
export { default as ScriptListPanel } from './script/ScriptListPanel';
export { default as ScriptEditorModal } from './script/ScriptEditorModal';
export { default as VariableInputDialog } from './script/VariableInputDialog';
export type {
  ScriptRuntime,
  ScriptData,
  ScriptStep,
  ScriptCommand,
  ScriptVariable,
  ScriptRecordingStatus,
} from './script/types';
// 阶段 5：Shell 设置切片
export { default as HighlightRulesModal } from './settings/HighlightRulesModal';
export { default as Switch } from './settings/Switch';
export { default as ThemeChoiceCard } from './settings/ThemeChoiceCard';
export { default as TerminalAppearanceCard } from './settings/TerminalAppearanceCard';
export { default as ShellSettingsModal } from './settings/ShellSettingsModal';
export type { ShellSettings, ShellSettingsRuntime, ShellSettingsModalProps } from './settings/ShellSettingsModal';
// 迭代 A：AI 接入配置（独立端口，密钥只在后台感知）
export { default as AIConfigCard } from './settings/AIConfigCard';
export type { AIConfigStatus, AIConfigUpdateInput, AIConfigRuntime, AIConfigCardProps } from './settings/AIConfigCard';
// 迭代 B：命令生成（Ctrl+K）与快捷键工具
export { default as CommandQueryOverlay } from './command/CommandQueryOverlay';
export type { CommandQueryResult } from './command/CommandQueryOverlay';
export { isEditableTarget, eventToShortcut, matchesShortcut } from './command/shortcut';
export type { CommandGeneratorRuntime, ConnectIntentParserRuntime } from './ports';
// 迭代 C：AI 诊断（中性事件契约）
export { default as DiagnosePanel } from './diagnose/DiagnosePanel';
export type { DiagnoseEvent, DiagnoseRuntime, DiagnoseStartOptions, DiagnoseArchiveInput, DiagnoseCaseRuntime, DiagnosePanelProps } from './diagnose/DiagnosePanel';
export * from './settings/settingsStyles';
