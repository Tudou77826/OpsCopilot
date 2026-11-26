import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { TbX } from 'react-icons/tb';
import { EditorState, Compartment } from '@codemirror/state';
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  drawSelection,
} from '@codemirror/view';
import {
  defaultKeymap,
  deleteLine,
  history,
  historyKeymap,
  indentWithTab,
} from '@codemirror/commands';
import { formatSQL } from './sqlFormat';
import { sql, MySQL } from '@codemirror/lang-sql';
import {
  bracketMatching,
  HighlightStyle,
  syntaxHighlighting,
} from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { autocompletion, completionKeymap } from '@codemirror/autocomplete';

const CTRL_Y_HINT_KEY = 'opscopilot.db.hideCtrlYHint';

function isCtrlYHintHidden(): boolean {
  try {
    return window.localStorage.getItem(CTRL_Y_HINT_KEY) === '1';
  } catch {
    return false;
  }
}

function persistHideCtrlYHint(): void {
  try {
    window.localStorage.setItem(CTRL_Y_HINT_KEY, '1');
  } catch {
    // ignore storage errors
  }
}

export interface SQLEditorRef {
  value(): string;
  selection(): string;
  cursorOffset(): number;
  focus(): void;
  insert(text: string): void;
}
export const SQLEditor = forwardRef<
  SQLEditorRef,
  {
    value: string;
    onChange(value: string): void;
    onRun(): void;
    onRunScript(): void;
    schema?: Record<string, string[]>;
  }
>(function SQLEditor({ value, onChange, onRun, onRunScript, schema }, ref) {
  const parent = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView>();
  const hintTimer = useRef<number | null>(null);
  const [showDeleteHint, setShowDeleteHint] = useState(false);
  const isMac =
    typeof navigator !== 'undefined' &&
    /mac/i.test(navigator.platform || navigator.userAgent);
  const triggerDeleteHint = () => {
    if (isCtrlYHintHidden()) return;
    setShowDeleteHint(true);
    if (hintTimer.current !== null) window.clearTimeout(hintTimer.current);
    hintTimer.current = window.setTimeout(() => {
      setShowDeleteHint(false);
      hintTimer.current = null;
    }, 5000);
  };
  const latest = useRef({ onChange, onRun, onRunScript, triggerDeleteHint });
  latest.current = { onChange, onRun, onRunScript, triggerDeleteHint };
  const dialect = useRef(new Compartment());
  useImperativeHandle(
    ref,
    () => ({
      value: () => view.current?.state.doc.toString() ?? '',
      cursorOffset: () => view.current?.state.selection.main.head ?? 0,
      selection: () => {
        const v = view.current;
        return v
          ? v.state.sliceDoc(
              v.state.selection.main.from,
              v.state.selection.main.to,
            )
          : '';
      },
      focus: () => view.current?.focus(),
      insert: (text: string) => {
        const editor = view.current;
        if (!editor) return;
        editor.dispatch({
          ...editor.state.replaceSelection(text),
          scrollIntoView: true,
        });
        editor.focus();
      },
    }),
    [],
  );
  useEffect(() => {
    const editor = new EditorView({
      parent: parent.current!,
      state: EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(),
          history(),
          highlightActiveLine(),
          drawSelection(),
          bracketMatching(),
          syntaxHighlighting(
            HighlightStyle.define([
              { tag: tags.keyword, color: 'var(--oc-db-sql-keyword)' },
              {
                tag: [tags.string, tags.special(tags.string)],
                color: 'var(--oc-db-sql-string)',
              },
              {
                tag: [tags.number, tags.bool, tags.null],
                color: 'var(--oc-db-sql-number)',
              },
              {
                tag: tags.comment,
                color: 'var(--text-muted)',
                fontStyle: 'italic',
              },
            ]),
          ),
          dialect.current.of(sql({ dialect: MySQL, schema })),
          autocompletion(),
          keymap.of([
            {
              key: 'Mod-Shift-Enter',
              run: () => {
                latest.current.onRunScript();
                return true;
              },
            },
            {
              key: 'Mod-Enter',
              run: () => {
                latest.current.onRun();
                return true;
              },
            },
            {
              key: 'Mod-y',
              run: (editorView) => {
                const deleted = deleteLine(editorView);
                if (deleted) latest.current.triggerDeleteHint();
                return deleted;
              },
            },
            {
              key: 'Ctrl-Alt-l',
              mac: 'Cmd-Alt-l',
              run: (editorView) => {
                const { from, to, empty } = editorView.state.selection.main;
                const rangeFrom = empty ? 0 : from;
                const rangeTo = empty ? editorView.state.doc.length : to;
                const source = editorView.state.sliceDoc(rangeFrom, rangeTo);
                const formatted = formatSQL(source);
                if (formatted !== source) {
                  editorView.dispatch({
                    changes: { from: rangeFrom, to: rangeTo, insert: formatted },
                  });
                }
                return true;
              },
            },
            {
              key: 'Shift-Alt-f',
              run: (editorView) => {
                const { from, to, empty } = editorView.state.selection.main;
                const rangeFrom = empty ? 0 : from;
                const rangeTo = empty ? editorView.state.doc.length : to;
                const source = editorView.state.sliceDoc(rangeFrom, rangeTo);
                const formatted = formatSQL(source);
                if (formatted !== source) {
                  editorView.dispatch({
                    changes: { from: rangeFrom, to: rangeTo, insert: formatted },
                  });
                }
                return true;
              },
            },
            ...defaultKeymap,
            ...historyKeymap,
            ...completionKeymap,
            indentWithTab,
          ]),
          EditorView.contentAttributes.of({
            'aria-label': 'SQL 编辑器',
            spellcheck: 'false',
          }),
          EditorView.editorAttributes.of((editor) => ({
            'data-has-selection': String(
              editor.state.selection.ranges.some((range) => !range.empty),
            ),
          })),
          EditorView.updateListener.of((update) => {
            if (update.docChanged)
              latest.current.onChange(update.state.doc.toString());
          }),
          EditorView.theme({
            '&': {
              height: '100%',
              fontSize: '13px',
              color: 'var(--text-primary)',
              backgroundColor: 'var(--bg-primary)',
            },
            '.cm-scroller': {
              fontFamily: 'var(--font-mono, Consolas, monospace)',
              overflow: 'auto',
            },
            '.cm-content': { padding: '12px 0' },
            '.cm-gutters': {
              backgroundColor: 'var(--bg-primary)',
              color: 'var(--text-muted)',
              border: 'none',
              paddingRight: '8px',
            },
            '.cm-activeLine, .cm-activeLineGutter': {
              backgroundColor: 'var(--bg-hover)',
            },
            '&[data-has-selection="true"] .cm-activeLine': {
              backgroundColor: 'transparent',
            },
            '.cm-cursor': { borderLeftColor: 'var(--text-primary)' },
            // Match CodeMirror's focused base-theme specificity so its default
            // selection color cannot override the application's theme.
            '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground':
              {
                background:
                  'color-mix(in srgb, var(--accent) 25%, transparent)',
              },
            '.cm-tooltip': {
              backgroundColor: 'var(--bg-elevated)',
              color: 'var(--text-primary)',
              border: '1px solid var(--border)',
              borderRadius: '7px',
              boxShadow:
                '0 8px 24px color-mix(in srgb, var(--bg-primary) 55%, transparent)',
              overflow: 'hidden',
            },
            '.cm-tooltip.cm-tooltip-autocomplete': {
              minWidth: '210px',
              marginTop: '4px',
            },
            '.cm-tooltip.cm-tooltip-autocomplete > ul': {
              padding: '4px',
              maxHeight: '220px',
              fontFamily: 'var(--font-mono, Consolas, monospace)',
              fontSize: '12px',
              scrollbarWidth: 'thin',
            },
            '.cm-tooltip.cm-tooltip-autocomplete > ul > li': {
              display: 'flex',
              alignItems: 'center',
              gap: '7px',
              height: '25px',
              lineHeight: '25px',
              padding: '0 8px',
              borderRadius: '4px',
              color: 'color-mix(in srgb, var(--text-primary) 82%, transparent)',
            },
            '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': {
              backgroundColor:
                'color-mix(in srgb, var(--text-primary) 12%, var(--bg-elevated))',
              color: 'var(--text-primary)',
            },
            '.cm-completionLabel': {
              flex: '1',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            },
            '.cm-completionDetail': {
              marginLeft: '8px',
              fontSize: '10px',
              fontStyle: 'normal',
              color: 'var(--text-muted)',
              opacity: '0.85',
            },
            '.cm-completionMatchedText': {
              textDecoration: 'none',
              color: 'color-mix(in srgb, var(--accent) 68%, var(--text-primary))',
              fontWeight: '700',
            },
            '.cm-completionIcon': {
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '16px',
              height: '16px',
              padding: '0',
              marginRight: '0',
              borderRadius: '3px',
              fontSize: '10px',
              fontWeight: '600',
              fontStyle: 'normal',
              opacity: '1',
              flexShrink: '0',
              backgroundColor:
                'color-mix(in srgb, var(--text-muted) 16%, transparent)',
              color: 'var(--text-secondary)',
            },
            '.cm-completionIcon::after': {
              content: '"·"',
            },
            '.cm-completionIcon-keyword': {
              backgroundColor:
                'color-mix(in srgb, var(--oc-db-sql-keyword) 20%, transparent)',
              color:
                'color-mix(in srgb, var(--oc-db-sql-keyword) 75%, var(--text-primary))',
            },
            '.cm-completionIcon-keyword::after': {
              content: '"K"',
            },
            '.cm-completionIcon-type, .cm-completionIcon-class': {
              backgroundColor:
                'color-mix(in srgb, var(--accent) 22%, transparent)',
              color: 'color-mix(in srgb, var(--accent) 72%, var(--text-primary))',
            },
            '.cm-completionIcon-type::after, .cm-completionIcon-class::after': {
              content: '"T"',
            },
            '.cm-completionIcon-property': {
              backgroundColor:
                'color-mix(in srgb, var(--oc-db-sql-string) 16%, transparent)',
              color: 'var(--oc-db-sql-string)',
            },
            '.cm-completionIcon-property::after': {
              content: '"C"',
            },
            '.cm-completionIcon-function': {
              backgroundColor:
                'color-mix(in srgb, var(--oc-db-sql-number) 16%, transparent)',
              color: 'var(--oc-db-sql-number)',
            },
            '.cm-completionIcon-function::after': {
              content: '"f"',
            },
          }),
        ],
      }),
    });
    view.current = editor;
    return () => {
      if (hintTimer.current !== null) window.clearTimeout(hintTimer.current);
      editor.destroy();
      view.current = undefined;
    };
  }, []);
  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value)
      v.dispatch({
        changes: { from: 0, to: v.state.doc.length, insert: value },
      });
  }, [value]);
  useEffect(() => {
    view.current?.dispatch({
      effects: dialect.current.reconfigure(sql({ dialect: MySQL, schema })),
    });
  }, [schema]);
  return (
    <div
      className="oc-db-editor"
      onKeyDown={(e) => e.stopPropagation()}
    >
      <div className="oc-db-editor-host" ref={parent} />
      {showDeleteHint && (
        <div className="oc-db-editor-hint" role="status">
          <span>
            {isMac ? 'Cmd+Y' : 'Ctrl+Y'} 已删除当前行 · 重做可用{' '}
            {isMac ? 'Cmd+Shift+Z' : 'Ctrl+Shift+Z'}
          </span>
          <button
            type="button"
            className="oc-db-editor-hint-mute"
            onClick={() => {
              persistHideCtrlYHint();
              setShowDeleteHint(false);
            }}
          >
            不再提示
          </button>
          <button
            type="button"
            className="oc-db-editor-hint-close"
            aria-label="关闭提示"
            title="关闭"
            onClick={() => setShowDeleteHint(false)}
          >
            <TbX size={13} />
          </button>
        </div>
      )}
    </div>
  );
});
