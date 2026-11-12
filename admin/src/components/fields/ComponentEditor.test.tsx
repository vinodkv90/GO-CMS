import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ContentField, ContentType } from '../../types';
import { FieldEditor } from './FieldEditor';

const heroComponent: ContentType = {
  id: 'comp-1',
  uid: 'sections.hero',
  name: 'Hero Section',
  kind: 'component',
  fields: [
    { name: 'Heading', field_key: 'heading', field_type: 'short_text', configuration: { required: true } },
  ],
};

const ctaComponent: ContentType = {
  id: 'comp-2',
  uid: 'blocks.cta',
  name: 'Call To Action',
  kind: 'component',
  fields: [
    { name: 'Label', field_key: 'label', field_type: 'short_text', configuration: {} },
  ],
};

describe('ComponentEditor & Dynamic Zone', () => {
  it('renders empty state, adds repeatable component items, and shows invalid badge on child errors', () => {
    const field: ContentField = {
      name: 'Highlights',
      field_key: 'highlights',
      field_type: 'repeatable_component',
      configuration: { component: 'sections.hero', repeatable: true },
    };
    const onChange = vi.fn();
    const { rerender } = render(
      <FieldEditor field={field} value={[]} onChange={onChange} data={{}} definitions={[heroComponent]} />,
    );

    expect(screen.getByText(/no entry yet/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /add an entry/i }));
    expect(onChange).toHaveBeenCalledWith([{}]);

    rerender(
      <FieldEditor
        field={field}
        value={[{ heading: '' }]}
        onChange={onChange}
        data={{}}
        definitions={[heroComponent]}
        errors={{ 'highlights[0].heading': 'Heading is required' }}
      />,
    );

    expect(screen.getByText('Invalid')).toBeInTheDocument();
    expect(screen.getByText('Heading is required')).toBeInTheDocument();
  });

  it('opens categorized dynamic zone picker and adds component with __component discriminator', () => {
    const field: ContentField = {
      name: 'Page Sections',
      field_key: 'sections',
      field_type: 'dynamic_zone',
      configuration: { components: ['sections.hero', 'blocks.cta'] },
    };
    const onChange = vi.fn();
    render(
      <FieldEditor
        field={field}
        value={[]}
        onChange={onChange}
        data={{}}
        definitions={[heroComponent, ctaComponent]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /add a component to page sections/i }));
    expect(screen.getByText('blocks')).toBeInTheDocument();
    expect(screen.getByText('sections')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /call to action/i }));
    expect(onChange).toHaveBeenCalledWith([{ __component: 'blocks.cta' }]);
  });
});

