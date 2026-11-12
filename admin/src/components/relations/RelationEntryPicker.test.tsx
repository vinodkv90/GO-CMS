import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { entriesApi } from '../../api/endpoints';
import type { ContentEntry } from '../../types';
import { RelationEntryPicker } from './RelationEntryPicker';

vi.mock('../../api/endpoints', () => ({
  entriesApi: { list: vi.fn(), get: vi.fn() },
}));

const entry: ContentEntry = {
  id: 'entry-1', content_type_id: 'type-1', draft_data: { title: 'A related article' },
  status: 'published', draft_version: 1, published_version: 1,
  created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
};

describe('RelationEntryPicker', () => {
  beforeEach(() => {
    vi.mocked(entriesApi.list).mockResolvedValue({ data: [entry], meta: { total: 1 } });
    vi.mocked(entriesApi.get).mockResolvedValue(entry);
  });

  it('loads and shows relation options without requiring the search field to be focused', async () => {
    render(<RelationEntryPicker contentTypeUid="blog-post" multiple value={[]} onChange={vi.fn()} />);

    expect(await screen.findByRole('option', { name: 'A related article' })).toBeInTheDocument();
    expect(entriesApi.list).toHaveBeenCalledWith('blog-post', expect.objectContaining({ limit: 20, offset: 0, preview: true }));
  });

  it('selects an option and shows its human-readable label', async () => {
    const onChange = vi.fn();
    render(<RelationEntryPicker contentTypeUid="blog-post" multiple value={[]} onChange={onChange} />);

    fireEvent.click(await screen.findByRole('option', { name: 'A related article' }));

    expect(onChange).toHaveBeenCalledWith(['entry-1']);
    await waitFor(() => expect(screen.getByText('A related article')).toBeInTheDocument());
  });

  it('prevents duplicate selections and replaces single relation selection cleanly', async () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <RelationEntryPicker contentTypeUid="blog-post" multiple value={['entry-1']} onChange={onChange} />,
    );
    const opt = await screen.findByRole('option', { name: 'A related article' });
    expect(opt).toBeDisabled();

    rerender(<RelationEntryPicker contentTypeUid="blog-post" multiple={false} value={['entry-1']} onChange={onChange} />);
    expect(screen.getByRole('button', { name: /remove entry-1/i })).toBeInTheDocument();
  });
});
