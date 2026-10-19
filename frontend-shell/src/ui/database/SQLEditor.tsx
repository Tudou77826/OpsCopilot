import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
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
  history,
  historyKeymap,
  indentWithTab,
} from '@codemirror/commands';
import { sql, MySQL } from '@codemirror/lang-sql';
import {
  bracketMatching,
  HighlightStyle,
  syntaxHighlighting,
} from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { autocompletion, completionKeymap } from '@codemirror/autocomplete';

export interface SQLEditorRef {
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
  const latest = useRef({ onChange, onRun, onRunScript });
  latest.current = { onChange, onRun, onRunScript };
  const dialect = useRef(new Compartment());
  useImperativeHandle(
    ref,
    () => ({
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
              borderColor: 'var(--border)',
            },
          }),
        ],
      }),
    });
    view.current = editor;
    return () => {
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
      ref={parent}
      onKeyDown={(e) => e.stopPropagation()}
    />
  );
});
