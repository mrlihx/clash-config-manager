import {EditorView, EditorState, basicSetup, keymap, indentWithTab, indentUnit, HighlightStyle, syntaxHighlighting, tags, yaml} from './editor-vendor.js';

let editor;

export function destroyYamlEditor() {
  editor?.destroy();
  editor = undefined;
}

// CodeMirror 只在浏览器本地编辑内容，隐藏字段接入现有表单保存流程。
export function mountYamlEditor(form, value, {name = 'yaml', label = '完整 YAML 配置', readonly = false} = {}) {
  destroyYamlEditor();
  const host = form.querySelector('[data-yaml-editor]');
  const field = document.createElement('input');
  field.type = 'hidden';
  field.name = name;
  field.value = value;
  host.append(field);
  const highlighting = HighlightStyle.define([
    {tag: tags.propertyName, class: 'yaml-key'},
    {tag: tags.string, class: 'yaml-string'},
    {tag: [tags.number, tags.bool, tags.null], class: 'yaml-value'},
    {tag: tags.comment, class: 'yaml-comment'},
    {tag: [tags.punctuation, tags.meta], class: 'yaml-punctuation'},
  ]);
  editor = new EditorView({
    parent: host,
    doc: value,
    extensions: [
      basicSetup,
      yaml(),
      indentUnit.of('  '),
      EditorState.tabSize.of(2),
      keymap.of([indentWithTab]),
      syntaxHighlighting(highlighting),
      EditorView.contentAttributes.of({'aria-label': label}),
      EditorState.readOnly.of(readonly),
      EditorView.editable.of(!readonly),
      EditorView.updateListener.of(update => {
        if (update.docChanged) field.value = update.state.doc.toString();
      }),
      EditorView.theme({
        '&': {height: 'min(58vh, 620px)', fontSize: '13px'},
        '.cm-scroller': {overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'},
        '.cm-content': {padding: '12px 0', minWidth: '100%'},
        '.cm-line': {padding: '0 12px'},
        '&.cm-focused': {outline: 'none'},
      }),
    ],
  });
}
