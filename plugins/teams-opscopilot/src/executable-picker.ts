import { execFile, type ExecFileOptionsWithStringEncoding } from 'node:child_process'
import { promisify } from 'node:util'
import { join } from 'node:path'
import { OpsError } from './errors.js'

// Never interpolate browser input into this script.
export const pickerInitialization = `
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[System.Text.Encoding]::UTF8
Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
public static class OpsPickerDpi {
  [DllImport("user32.dll", SetLastError=true)]
  private static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
  public static void Enable() {
    // The helper owns this STA thread until exit. Do not inherit PowerShell's
    // DPI-unaware default or change the Teams host's process-wide DPI mode.
    if (SetThreadDpiAwarenessContext(new IntPtr(-4)) == IntPtr.Zero)
      throw new Win32Exception(Marshal.GetLastWin32Error());
  }
}
'@
[OpsPickerDpi]::Enable()
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Application]::EnableVisualStyles()
`

// The owner brings the native dialog forward without showing a helper console.
export const pickerScript = `${pickerInitialization}
$owner=New-Object System.Windows.Forms.Form
$dialog=New-Object System.Windows.Forms.OpenFileDialog
try {
  $owner.Text='选择本地 OpsCopilot.exe'
  $owner.Size=New-Object System.Drawing.Size(1,1)
  $owner.StartPosition='CenterScreen'
  $owner.ShowInTaskbar=$false
  $owner.TopMost=$true
  $owner.Opacity=0
  $owner.Show()
  $owner.Visible=$true
  $owner.Activate()
  $dialog.Title='选择本地 OpsCopilot.exe'
  $dialog.Filter='OpsCopilot (*.exe)|*.exe'
  $dialog.CheckFileExists=$true
  $dialog.Multiselect=$false
  $dialog.AutoUpgradeEnabled=$true
  $dialog.InitialDirectory=[Environment]::GetFolderPath('MyDocuments')
  if($dialog.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) {
    [Console]::Write($dialog.FileName)
  }
} finally {
  $dialog.Dispose()
  $owner.Dispose()
}
`

const runFile = promisify(execFile)
type PickerRunner = (file: string, args: string[], options: ExecFileOptionsWithStringEncoding) => Promise<{ stdout: string; stderr: string }>
export async function chooseExecutable(signal: AbortSignal, run: PickerRunner = runFile): Promise<{ executable: string; cancelled: boolean }> {
  if (process.platform !== 'win32') throw new OpsError('UNSUPPORTED_CAPABILITY', '首版仅支持 Windows 本地 Ops')
  try {
    const { stdout } = await run(join(process.env.SystemRoot || 'C:\\Windows', 'System32/WindowsPowerShell/v1.0/powershell.exe'),
      ['-NoProfile', '-NonInteractive', '-STA', '-EncodedCommand', Buffer.from(pickerScript, 'utf16le').toString('base64')],
      { windowsHide: true, shell: false, timeout: 120000, maxBuffer: 16384, signal, encoding: 'utf8' })
    const executable = stdout.trim().replace(/^\uFEFF/, '')
    return { executable, cancelled: !executable }
  } catch {
    throw new OpsError('OPERATION_FAILED', '文件选择框未能完成或已超时，请重新点击浏览')
  }
}
