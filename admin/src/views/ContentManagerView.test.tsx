import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '../components/ui/Toast';
import { matchesAllowedTypes, isVideoMediaMime } from '../components/media/MediaPickerModal';
import { contentTypesApi, entriesApi, mediaApi } from '../api/endpoints';
import type { ContentEntry, ContentType } from '../types';
import { ContentManagerView } from './ContentManagerView';

vi.mock('../api/endpoints', () => ({
  contentTypesApi: {
    list: vi.fn(),
    update: vi.fn(),
  },
  entriesApi: {
    list: vi.fn(),
    get: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    publish: vi.fn(),
    unpublish: vi.fn(),
    delete: vi.fn(),
    versions: vi.fn(),
    restore: vi.fn(),
  },
  mediaApi: {
    list: vi.fn(),
    listFolders: vi.fn(),
    tree: vi.fn(),
    get: vi.fn(),
  },
}));

const calloutComponent: ContentType = {
  id: 'comp-callout',
  uid: 'blocks.callout',
  name: 'Callout',
  kind: 'component',
  fields: [
    { name: 'Heading', field_key: 'heading', field_type: 'short_text', configuration: {} },
    { name: 'Body', field_key: 'body', field_type: 'rich_text', configuration: {} },
  ],
};

const tagType: ContentType = {
  id: 'type-tag',
  uid: 'e2e-tag',
  name: 'E2E Tag',
  kind: 'collection',
  fields: [
    { name: 'Name', field_key: 'name', field_type: 'short_text', configuration: { required: true } },
  ],
};

const showcaseType: ContentType = {
  id: 'type-showcase',
  uid: 'e2e-showcase',
  name: 'E2E Showcase',
  kind: 'collection',
  fields: [
    { name: 'Title', field_key: 'title', field_type: 'short_text', configuration: { required: true } },
    { name: 'Slug', field_key: 'slug', field_type: 'uid', configuration: { target_field: 'title', unique: true } },
    { name: 'Body', field_key: 'body', field_type: 'rich_text', configuration: {} },
    { name: 'Content Blocks', field_key: 'content_blocks', field_type: 'blocks', configuration: {} },
    { name: 'API Secret', field_key: 'api_secret', field_type: 'secret', configuration: { write_only: true } },
    { name: 'Scheduled At', field_key: 'scheduled_at', field_type: 'datetime', configuration: {} },
    { name: 'Extra JSON', field_key: 'extra_json', field_type: 'json', configuration: {} },
    { name: 'Primary Tag', field_key: 'primary_tag', field_type: 'relation_single', configuration: { target_type: 'e2e-tag' } },
    { name: 'Callouts', field_key: 'callouts', field_type: 'repeatable_component', configuration: { component: 'blocks.callout' } },
    { name: 'Page Sections', field_key: 'page_sections', field_type: 'dynamic_zone', configuration: { components: ['blocks.callout'] } },
  ],
};

const savedEntryV1: ContentEntry = {
  id: 'entry-v1',
  content_type_id: 'type-showcase',
  slug: 'hello-world',
  draft_data: {
    body: '<p>Rich <strong>HTML content</strong></p>',
    callouts: [{ body: '<p></p>', heading: 'One' }],
    content_blocks: { content: [{ content: [{ text: 'Block', type: 'text' }], type: 'paragraph' }], type: 'doc' },
    extra_json: { env: 'prod', tier: 1 },
    page_sections: [{ __component: 'blocks.callout', body: '<p></p>', heading: 'DZ' }],
    primary_tag: 'tag-1',
    scheduled_at: '2026-11-12T05:00:00.000Z',
    slug: 'hello-world',
    title: 'Hello World',
  },
  status: 'draft',
  draft_version: 1,
  published_version: 0,
  created_at: '2026-11-12T05:00:00Z',
  updated_at: '2026-11-12T05:00:00Z',
};

describe('ContentManagerView dirty state tracking', () => {
  beforeEach(() => {
    vi.mocked(contentTypesApi.list).mockResolvedValue({ data: [showcaseType, tagType, calloutComponent] });
    vi.mocked(entriesApi.list).mockImplementation(async (uid) => {
      if (uid === 'e2e-tag') {
        return {
          data: [{
            id: 'tag-1',
            content_type_id: 'type-tag',
            draft_data: { name: 'Go' },
            status: 'published',
            draft_version: 1,
            published_version: 1,
            created_at: '2026-11-12T05:00:00Z',
            updated_at: '2026-11-12T05:00:00Z',
          }],
          meta: { total: 1 },
        };
      }
      return { data: [savedEntryV1], meta: { total: 1 } };
    });
    vi.mocked(entriesApi.get).mockResolvedValue(savedEntryV1);
    vi.mocked(entriesApi.create).mockResolvedValue(savedEntryV1);
    vi.mocked(entriesApi.update).mockResolvedValue({
      ...savedEntryV1,
      draft_version: 2,
      draft_data: { ...savedEntryV1.draft_data, title: 'Updated World' },
    });
    vi.mocked(mediaApi.list).mockResolvedValue({ data: [], meta: { total: 0 } });
  });

  it('does not mark an entry dirty immediately after opening saved v1, and clears after editing + saving', async () => {
    render(
      <ToastProvider>
        <ContentManagerView
          contentType={showcaseType}
          contentTypes={[showcaseType, tagType, calloutComponent]}
          onBackToDashboard={vi.fn()}
        />
      </ToastProvider>,
    );

    const rows = await screen.findAllByText('Hello World');
    fireEvent.click(rows[0]);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument();
    });

    await new Promise((r) => setTimeout(r, 100));

    expect(screen.queryByText('Unsaved draft')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled();

    const titleInput = screen.getByRole('textbox', { name: /^title/i });
    fireEvent.change(titleInput, { target: { value: 'Updated World' } });
    expect(screen.getByText('Unsaved draft')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }));
    await waitFor(() => {
      expect(entriesApi.update).toHaveBeenCalled();
    });
    expect(screen.queryByText('Unsaved draft')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled();
  });

  it('does not emit spurious onChange when mounting FieldEditors with saved v1 values', async () => {
    const { FieldEditor } = await import('../components/fields/FieldEditor');
    const changedKeys: Array<{ key: string; value: unknown }> = [];
    for (const field of showcaseType.fields || []) {
      render(
        <FieldEditor
          key={field.field_key}
          field={field}
          value={savedEntryV1.draft_data[field.field_key]}
          onChange={(val) => changedKeys.push({ key: field.field_key, value: val })}
          data={savedEntryV1.draft_data}
          definitions={[showcaseType, tagType, calloutComponent]}
        />,
      );
    }
    await new Promise((r) => setTimeout(r, 100));
    expect(changedKeys).toEqual([]);
  });

  it('treats HLS playlists as videos in MediaPickerModal MIME filtering', () => {
    expect(isVideoMediaMime('application/vnd.apple.mpegurl')).toBe(true);
    expect(isVideoMediaMime('application/x-mpegURL')).toBe(true);
    expect(isVideoMediaMime('video/mp4')).toBe(true);
    expect(isVideoMediaMime('image/png')).toBe(false);

    expect(matchesAllowedTypes('application/vnd.apple.mpegurl', ['image', 'video'])).toBe(true);
    expect(matchesAllowedTypes('application/x-mpegurl', ['videos'])).toBe(true);
    expect(matchesAllowedTypes('application/vnd.apple.mpegurl', ['file'])).toBe(false);
    expect(matchesAllowedTypes('application/pdf', ['file'])).toBe(true);
  });

  it('does not mark an entry dirty immediately after creating v1 via POST', async () => {
    render(
      <ToastProvider>
        <ContentManagerView
          contentType={showcaseType}
          contentTypes={[showcaseType, tagType, calloutComponent]}
          onBackToDashboard={vi.fn()}
        />
      </ToastProvider>,
    );

    fireEvent.click(await screen.findByRole('button', { name: /create new entry/i }));
    const titleInput = await screen.findByRole('textbox', { name: /title/i });
    fireEvent.change(titleInput, { target: { value: 'Hello World' } });

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(entriesApi.create).toHaveBeenCalled();
    });
    await new Promise((r) => setTimeout(r, 100));

    expect(screen.queryByText('Unsaved draft')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled();
  });
});

