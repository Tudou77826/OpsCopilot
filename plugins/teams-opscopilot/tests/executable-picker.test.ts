import test from 'node:test'
import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { join } from 'node:path'
import { chooseExecutable, pickerInitialization, pickerScript } from '../src/executable-picker.js'

test('picker enables modern Windows visuals before creating windows and uses an owner', () => {
  assert.ok(pickerScript.indexOf('EnableVisualStyles()') < pickerScript.indexOf('New-Object System.Windows.Forms.Form'))
  assert.ok(pickerScript.indexOf('[OpsPickerDpi]::Enable()') < pickerScript.indexOf('Add-Type -AssemblyName System.Windows.Forms'))
  assert.match(pickerScript, /AutoUpgradeEnabled=\$true/)
  assert.match(pickerScript, /TopMost=\$true/)
  assert.match(pickerScript, /ShowDialog\(\$owner\)/)
  assert.match(pickerScript, /finally[\s\S]*\$dialog.Dispose\(\)[\s\S]*\$owner.Dispose\(\)/)
})

test('real Windows helper creates a Per-Monitor V2 window without opening a dialog', { skip: process.platform !== 'win32', timeout: 20000 }, async () => {
  const probe = `${pickerInitialization}
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class PickerDpiProbe {
  [DllImport("user32.dll")] private static extern IntPtr GetWindowDpiAwarenessContext(IntPtr window);
  [DllImport("user32.dll")] private static extern bool AreDpiAwarenessContextsEqual(IntPtr a, IntPtr b);
  public static bool IsPerMonitorV2(IntPtr window) {
    return AreDpiAwarenessContextsEqual(GetWindowDpiAwarenessContext(window), new IntPtr(-4));
  }
}
'@
$form=New-Object System.Windows.Forms.Form
try { [Console]::Write([PickerDpiProbe]::IsPerMonitorV2($form.Handle)) }
finally { $form.Dispose() }
`
  const { stdout } = await promisify(execFile)(join(process.env.SystemRoot || 'C:\\Windows', 'System32/WindowsPowerShell/v1.0/powershell.exe'),
    ['-NoProfile', '-NonInteractive', '-STA', '-EncodedCommand', Buffer.from(probe, 'utf16le').toString('base64')],
    { windowsHide: true, shell: false, timeout: 15000, encoding: 'utf8' })
  assert.equal(stdout.trim(), 'True')
})

test('picker returns Unicode paths, keeps the console hidden and passes cancellation', { skip: process.platform !== 'win32' }, async () => {
  const signal = new AbortController().signal
  const run = (async (_exe: string, args: string[], options: any) => {
    assert.equal(Buffer.from(args.at(-1)!, 'base64').toString('utf16le'), pickerScript)
    assert.equal(options.windowsHide, true)
    assert.equal(options.shell, false)
    assert.equal(options.signal, signal)
    return { stdout: '\uFEFFC:\\应用 目录\\OpsCopilot.exe\r\n', stderr: '' }
  }) as NonNullable<Parameters<typeof chooseExecutable>[1]>
  assert.deepEqual(await chooseExecutable(signal, run), { executable: 'C:\\应用 目录\\OpsCopilot.exe', cancelled: false })
})

test('picker distinguishes cancel from launch failure', { skip: process.platform !== 'win32' }, async () => {
  const signal = new AbortController().signal
  const cancel = (async () => ({ stdout: '', stderr: '' })) as NonNullable<Parameters<typeof chooseExecutable>[1]>
  assert.deepEqual(await chooseExecutable(signal, cancel), { executable: '', cancelled: true })
  const fail = (async () => { throw new Error('spawn failure') }) as NonNullable<Parameters<typeof chooseExecutable>[1]>
  await assert.rejects(chooseExecutable(signal, fail), { code: 'OPERATION_FAILED' })
})
