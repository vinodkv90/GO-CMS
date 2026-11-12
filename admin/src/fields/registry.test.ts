import { describe, expect, it } from 'vitest';
import { getFieldSettings, validateFieldDraft } from './registry';
import type { ContentField, ContentType } from '../types';

const owner: ContentType = { id: 'posts', uid: 'posts', name: 'Posts', kind: 'collection', fields: [
  { name: 'Title', field_key: 'title', field_type: 'short_text', configuration: {} },
] };

describe('field capability registry', () => {
  it('shows only settings supported by the selected field type and server', () => {
    const types = getFieldSettings('short_text', {}, { short_text: { settings: ['required', 'placeholder', 'min_length'] } });
    expect(types.map((setting) => setting.key)).toEqual(['required', 'placeholder', 'min_length']);
  });

  it('validates UID sources and requires actual component definitions', () => {
    const uid: ContentField = { name: 'Slug', field_key: 'slug', field_type: 'uid', configuration: { target_field: 'title' } };
    expect(validateFieldDraft(uid, owner, [])).toEqual([]);
    const component: ContentField = { name: 'SEO', field_key: 'seo', field_type: 'component', configuration: { component: 'shared.seo' } };
    expect(validateFieldDraft(component, owner, [])).toContain('Choose existing component definitions.');
  });
});
