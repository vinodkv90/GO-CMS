import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SettingsLayout } from './SettingsLayout';

const groups = [
  {
    id: 'global',
    label: 'Global Settings',
    items: [
      { id: 'overview', label: 'Overview' },
      { id: 'general', label: 'General' },
    ],
  },
];

describe('SettingsLayout', () => {
  it('switches sections immediately when clean', () => {
    const onSelectSection = vi.fn();
    render(
      <SettingsLayout
        groups={groups}
        activeSection="overview"
        onSelectSection={onSelectSection}
        title="Overview"
        isDirty={false}
      >
        <div>Body</div>
      </SettingsLayout>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'General' }));
    expect(onSelectSection).toHaveBeenCalledWith('general');
  });

  it('prompts confirmation before leaving when form has unsaved changes', () => {
    const onSelectSection = vi.fn();
    const onDiscard = vi.fn();

    render(
      <SettingsLayout
        groups={groups}
        activeSection="general"
        onSelectSection={onSelectSection}
        title="General"
        isDirty={true}
        onSave={vi.fn()}
        onDiscard={onDiscard}
      >
        <div>Form</div>
      </SettingsLayout>,
    );

    expect(screen.getByText('Unsaved changes')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Overview' }));

    expect(onSelectSection).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Discard & leave' }));
    expect(onDiscard).toHaveBeenCalledTimes(1);
    expect(onSelectSection).toHaveBeenCalledWith('overview');
  });
});

