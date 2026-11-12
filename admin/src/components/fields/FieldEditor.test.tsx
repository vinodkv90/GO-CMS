import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CustomFieldsProvider } from '../../fields/CustomFieldContext';
import { FieldEditor } from './FieldEditor';
import type { ContentField } from '../../types';

describe('schema field renderer', () => {
  it('renders scalar values using the field type and reports edits', () => {
    const field: ContentField = { name: 'Title', field_key: 'title', field_type: 'short_text', configuration: { required: true, placeholder: 'Post title', max_length: 80 } };
    const onChange = vi.fn();
    render(<FieldEditor field={field} value="Old title" onChange={onChange} data={{}} definitions={[]} />);
    const input = screen.getByRole('textbox', { name: /title/i });
    expect(input).toHaveAttribute('placeholder', 'Post title');
    expect(input).toBeRequired();
    expect(screen.getByText('9 / 80 characters')).toBeInTheDocument();
    fireEvent.change(input, { target: { value: 'New title' } });
    expect(onChange).toHaveBeenCalledWith('New title');
  });

  it('renders boolean fields using a Strapi switch toggle', () => {
    const field: ContentField = { name: 'Featured', field_key: 'featured', field_type: 'boolean', configuration: {} };
    const onChange = vi.fn();
    render(<FieldEditor field={field} value={false} onChange={onChange} data={{}} definitions={[]} />);
    const toggle = screen.getByRole('switch', { name: /featured/i });
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(true);
  });

  it('renders UID regenerate action and clears date fields cleanly', () => {
    const uidField: ContentField = { name: 'Slug', field_key: 'slug', field_type: 'uid', configuration: { target_field: 'title' } };
    const onUidChange = vi.fn();
    render(<FieldEditor field={uidField} value="custom-slug" onChange={onUidChange} data={{ title: 'Hello Strapi 5' }} definitions={[]} />);
    fireEvent.click(screen.getByRole('button', { name: /regenerate uid/i }));
    expect(onUidChange).toHaveBeenCalledWith('hello-strapi-5');

    const dateField: ContentField = { name: 'Published Date', field_key: 'pub_date', field_type: 'date', configuration: {} };
    const onDateChange = vi.fn();
    render(<FieldEditor field={dateField} value="2026-11-11" onChange={onDateChange} data={{}} definitions={[]} />);
    fireEvent.click(screen.getByRole('button', { name: /clear value/i }));
    expect(onDateChange).toHaveBeenCalledWith(null);
  });

  it('renders registered plugin custom fields when enabled', () => {
    const field = { name: 'Accent Color', field_key: 'accent', field_type: 'color_picker', configuration: {} } as unknown as ContentField;
    const onChange = vi.fn();
    render(
      <CustomFieldsProvider value={[{ type: 'color_picker', label: 'Color Picker', plugin_name: 'Color Plugin' }]}>
        <FieldEditor field={field} value="#0ea5e9" onChange={onChange} data={{}} definitions={[]} />
      </CustomFieldsProvider>,
    );
    expect(screen.getByText('Color Plugin')).toBeInTheDocument();
    const input = screen.getByRole('textbox', { name: /accent color/i });
    fireEvent.change(input, { target: { value: '#10b981' } });
    expect(onChange).toHaveBeenCalledWith('#10b981');
  });

  it('never silently converts an unknown field to text', () => {
    const field = { name: 'Legacy', field_key: 'legacy', field_type: 'future_type', configuration: {} } as unknown as ContentField;
    render(<FieldEditor field={field} value="keep" onChange={vi.fn()} data={{}} definitions={[]} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Unsupported field type: future_type');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });
});
