import { useEffect, useRef, useState } from 'react';
import { EditorContent, useEditor } from '@tiptap/react';
import type { JSONContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import Underline from '@tiptap/extension-underline';
import { MediaPickerModal } from '../media/MediaPickerModal';
import { buttonClass, configFor, inputClass, isRecord, safeAssetURL } from './contracts';
import type { FieldEditorProps } from './contracts';
import { useValidity } from './useValidity';

const SafeImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      src: {
        default: null,
        parseHTML: (element) => safeAssetURL(element.getAttribute('src') || undefined) || null,
        renderHTML: (attributes) => ({ src: safeAssetURL(attributes.src) || undefined }),
      },
    };
  },
}).configure({ allowBase64: false, HTMLAttributes: { loading: 'lazy', referrerpolicy: 'no-referrer' } });

const extensions = [StarterKit.configure({ link: false, underline: false }), SafeImage, Link.configure({ openOnClick: false, autolink: true, defaultProtocol: 'https', protocols: ['http', 'https', 'mailto'], HTMLAttributes: { rel: 'noopener noreferrer nofollow' } }), Underline];
const emptyDocument: JSONContent = { type: 'doc', content: [{ type: 'paragraph' }] };

interface Props extends FieldEditorProps { id: string; format: 'html' | 'json'; describedBy?: string; invalid?: boolean }

/** Tiptap's schema renders the document; HTML is never injected into a preview. */
export function RichTextEditor({ value, onChange, disabled, id, format, path = id, onValidityChange, describedBy, invalid, field }: Props) {
  const change = useRef(onChange);
  change.current = onChange;
  const lastEmitted = useRef<string | undefined>(undefined);
  const [contentError, setContentError] = useState<string>();
  const [urlError, setUrlError] = useState<string>();
  const [linkURL, setLinkURL] = useState('');
  const [imageURL, setImageURL] = useState('');
  const [imageAlt, setImageAlt] = useState('');
  const [mediaOpen, setMediaOpen] = useState(false);
  const serialized = JSON.stringify(value);
  const legacy = value != null && value !== '' && (format === 'json' ? !isRecord(value) || value.type !== 'doc' : typeof value !== 'string');
  const legacyMessage = legacy ? `Legacy ${format === 'json' ? 'blocks' : 'rich text'} value preserved. Expected ${format === 'json' ? 'a ProseMirror document object' : 'HTML text'}; no automatic conversion is performed.` : undefined;
  const editor = useEditor({
    extensions,
    content: emptyDocument,
    injectCSS: false,
    immediatelyRender: false,
    shouldRerenderOnTransaction: true,
    editable: !disabled && !legacy,
    editorProps: { attributes: { id, role: 'textbox', 'aria-multiline': 'true', 'aria-label': configFor(field).label || field.name || field.field_key, class: 'min-h-40 p-3 outline-none [&_ul]:list-disc [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:pl-6 [&_h1]:text-2xl [&_h2]:text-xl [&_h3]:text-lg [&_blockquote]:border-l-4 [&_blockquote]:pl-3 [&_pre]:bg-slate-100 [&_pre]:p-2 [&_a]:text-sky-700 [&_a]:underline [&_img]:max-w-full' } },
    onUpdate: ({ editor: instance }) => {
      const next = format === 'html' ? instance.getHTML() : instance.getJSON();
      lastEmitted.current = JSON.stringify(next);
      change.current(next);
    },
  }, [format]);

  useEffect(() => {
    if (!editor || legacy || serialized === lastEmitted.current) return;
    try {
      const content = value == null || value === '' ? emptyDocument : value;
      if (format === 'json') editor.schema.nodeFromJSON(content as JSONContent).check();
      editor.commands.setContent(content as JSONContent | string, { emitUpdate: false, errorOnInvalidContent: true });
      setContentError(undefined);
      lastEmitted.current = serialized;
    } catch {
      setContentError('This document contains unsupported or invalid content. Its stored value is preserved; repair it before editing.');
    }
  }, [editor, serialized, legacy, format, value]);
  useEffect(() => { editor?.setEditable(!disabled && !legacy && !contentError); }, [editor, disabled, legacy, contentError]);
  useEffect(() => { editor?.setOptions({ editorProps: { attributes: { id, role: 'textbox', 'aria-label': configFor(field).label || field.name || field.field_key, 'aria-multiline': 'true', 'aria-invalid': String(Boolean(invalid || contentError)), 'aria-describedby': describedBy || '' } } }); }, [editor, id, describedBy, invalid, contentError, field.name, field.field_key]);
  useValidity(path, disabled ? undefined : legacyMessage || contentError, onValidityChange);

  if (legacyMessage || contentError) return <p role="alert" className="text-sm text-amber-700">{legacyMessage || contentError}</p>;
  const locked = disabled || !editor;
  const allowed = configFor(field).allowed_blocks;
  const blockAllowed = (name: string) => !allowed?.length || allowed.includes(name);
  return <div className="overflow-hidden rounded-lg border border-slate-300 bg-white">
    <div role="toolbar" aria-label="Text formatting" className="flex flex-wrap gap-1 border-b border-slate-200 bg-slate-50 p-2" onMouseDown={(event) => event.preventDefault()}>
      <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('bold') || false} onClick={() => editor?.chain().focus().toggleBold().run()}>Bold</button>
      <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('italic') || false} onClick={() => editor?.chain().focus().toggleItalic().run()}>Italic</button>
      <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('underline') || false} onClick={() => editor?.chain().focus().toggleUnderline().run()}>Underline</button>
      <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('strike') || false} onClick={() => editor?.chain().focus().toggleStrike().run()}>Strike</button>
      {blockAllowed('paragraph') && <button type="button" className={buttonClass} disabled={locked} onClick={() => editor?.chain().focus().setParagraph().run()}>Paragraph</button>}
      {blockAllowed('heading') && ([1, 2, 3] as const).map((level) => <button key={level} type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('heading', { level }) || false} onClick={() => editor?.chain().focus().toggleHeading({ level }).run()}>H{level}</button>)}
      {blockAllowed('bulletList') && <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('bulletList') || false} onClick={() => editor?.chain().focus().toggleBulletList().run()}>Bullets</button>}
      {blockAllowed('orderedList') && <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('orderedList') || false} onClick={() => editor?.chain().focus().toggleOrderedList().run()}>Numbered list</button>}
      {blockAllowed('blockquote') && <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('blockquote') || false} onClick={() => editor?.chain().focus().toggleBlockquote().run()}>Quote</button>}
      {blockAllowed('codeBlock') && <button type="button" className={buttonClass} disabled={locked} aria-pressed={editor?.isActive('codeBlock') || false} onClick={() => editor?.chain().focus().toggleCodeBlock().run()}>Code</button>}
      <button type="button" className={buttonClass} disabled={locked || !editor?.can().undo()} onClick={() => editor?.chain().focus().undo().run()}>Undo</button>
      <button type="button" className={buttonClass} disabled={locked || !editor?.can().redo()} onClick={() => editor?.chain().focus().redo().run()}>Redo</button>
    </div>
    <EditorContent editor={editor} />
    <div className="space-y-2 border-t border-slate-200 p-2">
      <div className="flex flex-wrap gap-2"><input id={`${id}-link-url`} name={`${path}-link-url`} className={`${inputClass} flex-1`} aria-label="Link URL" placeholder="https://example.com" value={linkURL} disabled={locked} onChange={(event) => setLinkURL(event.target.value)} /><button type="button" className={buttonClass} disabled={locked} onClick={() => {
        if (!/^(https?:\/\/|mailto:)[^\s]+$/i.test(linkURL)) { setUrlError('Use an http, https or mailto link.'); return; }
        setUrlError(undefined); editor?.chain().focus().extendMarkRange('link').setLink({ href: linkURL }).run(); setLinkURL('');
      }}>Set link</button><button type="button" className={buttonClass} disabled={locked || !editor?.isActive('link')} onClick={() => editor?.chain().focus().unsetLink().run()}>Remove link</button></div>
      {blockAllowed('image') && <div className="flex flex-wrap gap-2"><input id={`${id}-image-url`} name={`${path}-image-url`} className={`${inputClass} flex-1`} aria-label="Image URL" placeholder="https://example.com/image.jpg" value={imageURL} disabled={locked} onChange={(event) => setImageURL(event.target.value)} /><input id={`${id}-image-alt`} name={`${path}-image-alt`} className={`${inputClass} flex-1`} aria-label="Image alternative text" placeholder="Image description" value={imageAlt} disabled={locked} onChange={(event) => setImageAlt(event.target.value)} /><button type="button" className={buttonClass} disabled={locked} onClick={() => {
        const src = safeAssetURL(imageURL);
        if (!src) { setUrlError('Use an http, https or site-relative image URL.'); return; }
        setUrlError(undefined); editor?.chain().focus().setImage({ src, alt: imageAlt }).run(); setImageURL(''); setImageAlt('');
      }}>Insert image</button><button type="button" className={buttonClass} disabled={locked} aria-expanded={mediaOpen} onClick={() => setMediaOpen(true)}>Browse media library</button></div>}
      {blockAllowed('image') && <MediaPickerModal isOpen={mediaOpen} onClose={() => setMediaOpen(false)} allowedTypes={['image']} onConfirm={(picked) => {
        const asset = picked[0];
        if (!asset) return;
        const src = safeAssetURL(asset.url);
        if (src) editor?.chain().focus().setImage({ src, alt: asset.metadata?.alt_text || asset.original_name }).run();
      }} />}
      {urlError && <p role="alert" className="text-xs text-rose-600">{urlError}</p>}
    </div>
  </div>;
}
