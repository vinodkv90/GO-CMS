import React, { useCallback, useEffect, useState } from 'react';
import {
  ClipboardList,
  Download,
  Eye,
  Filter,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
} from 'lucide-react';
import { auditApi } from '../../api/endpoints';
import { ApiError } from '../../api/client';
import type { AuditLogEntry, AuditLogFilter } from '../../types';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { Badge } from '../../components/ui/Badge';
import { DataTable, type DataTableColumn } from '../../components/ui/DataTable';
import { Pagination } from '../../components/ui/Pagination';
import { EmptyState } from '../../components/ui/EmptyState';
import { Alert } from '../../components/ui/Alert';
import { Modal } from '../../components/ui/Modal';
import { useToast } from '../../components/ui/Toast';

interface FilterDraft {
  actor_id: string;
  action: string;
  resource: string;
  result: string;
  from: string;
  to: string;
}

const EMPTY_DRAFT: FilterDraft = {
  actor_id: '',
  action: '',
  resource: '',
  result: '',
  from: '',
  to: '',
};

const PAGE_LIMIT = 25;

function formatTimestamp(iso: string): string {
  try {
    const dt = new Date(iso);
    if (Number.isNaN(dt.getTime())) return iso;
    return dt.toLocaleString();
  } catch {
    return iso;
  }
}

function resultBadgeVariant(result?: string): 'success' | 'error' | 'neutral' {
  const norm = (result || '').toLowerCase();
  if (norm === 'success') return 'success';
  if (norm === 'failure' || norm === 'error' || norm === 'denied') return 'error';
  return 'neutral';
}

export const AuditLogsSettingsView: React.FC = () => {
  const toast = useToast();

  const [draft, setDraft] = useState<FilterDraft>(EMPTY_DRAFT);
  const [applied, setApplied] = useState<FilterDraft>(EMPTY_DRAFT);
  const [offset, setOffset] = useState(0);

  const [rows, setRows] = useState<AuditLogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isExporting, setIsExporting] = useState(false);

  const [selectedEntry, setSelectedEntry] = useState<AuditLogEntry | null>(null);

  const buildFilter = useCallback(
    (source: FilterDraft, nextOffset: number): AuditLogFilter => ({
      actor_id: source.actor_id.trim() || undefined,
      action: source.action.trim() || undefined,
      resource: source.resource.trim() || undefined,
      result: source.result.trim() || undefined,
      from: source.from.trim() || undefined,
      to: source.to.trim() || undefined,
      limit: PAGE_LIMIT,
      offset: nextOffset,
    }),
    [],
  );

  const loadLogs = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await auditApi.list(buildFilter(applied, offset));
      setRows(Array.isArray(res.data) ? res.data : []);
      setTotal(res.meta?.total ?? 0);
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : 'Failed to load audit logs. Please check permissions and connectivity.';
      setError(msg);
    } finally {
      setIsLoading(false);
    }
  }, [applied, offset, buildFilter]);

  useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  const handleApplyFilters = (e: React.FormEvent) => {
    e.preventDefault();
    setOffset(0);
    setApplied({ ...draft });
  };

  const handleResetFilters = () => {
    setDraft(EMPTY_DRAFT);
    setApplied(EMPTY_DRAFT);
    setOffset(0);
  };

  const handleExportCsv = async () => {
    setIsExporting(true);
    try {
      const filter = buildFilter(applied, 0);
      delete filter.limit;
      delete filter.offset;
      const blob = await auditApi.exportCsv(filter);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      toast.success('Audit export downloaded', 'Filtered audit log records saved as CSV.');
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Unable to export audit logs';
      toast.error('Export failed', msg);
    } finally {
      setIsExporting(false);
    }
  };

  const hasActiveFilters = Boolean(
    applied.actor_id ||
      applied.action ||
      applied.resource ||
      applied.result ||
      applied.from ||
      applied.to,
  );

  const columns: DataTableColumn<AuditLogEntry>[] = [
    {
      key: 'created_at',
      header: 'Timestamp',
      render: (row) => (
        <span className="text-xs text-slate-700 font-medium whitespace-nowrap" title={row.created_at}>
          {formatTimestamp(row.created_at)}
        </span>
      ),
    },
    {
      key: 'actor',
      header: 'Actor',
      render: (row) => (
        <div className="flex items-center gap-2 min-w-0">
          <Badge variant="neutral" className="font-mono text-[11px] shrink-0">
            {row.actor_type || 'system'}
          </Badge>
          <span className="text-xs font-mono text-slate-700 truncate max-w-[140px]" title={row.actor_id}>
            {row.actor_id || 'system'}
          </span>
        </div>
      ),
    },
    {
      key: 'action',
      header: 'Action',
      render: (row) => (
        <div className="flex items-center gap-2 flex-wrap">
          <code className="text-xs font-semibold text-slate-900 bg-slate-100 px-1.5 py-0.5 rounded">
            {row.action}
          </code>
          {row.result && (
            <Badge variant={resultBadgeVariant(row.result)} className="text-[11px]">
              {row.result}
            </Badge>
          )}
        </div>
      ),
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (row) => (
        <div className="space-y-0.5 min-w-0">
          <div className="text-xs font-medium text-slate-800 truncate">{row.resource}</div>
          {row.target_id && (
            <div className="text-[11px] font-mono text-slate-400 truncate max-w-[160px]" title={row.target_id}>
              {row.target_id}
            </div>
          )}
        </div>
      ),
    },
    {
      key: 'ip_address',
      header: 'IP Address',
      render: (row) => (
        <span className="text-xs font-mono text-slate-600">{row.ip_address || '—'}</span>
      ),
    },
    {
      key: 'details',
      header: '',
      className: 'text-right',
      render: (row) => (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            setSelectedEntry(row);
          }}
          aria-label={`Inspect audit entry ${row.action}`}
        >
          <Eye className="w-3.5 h-3.5" />
          <span>Inspect</span>
        </Button>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Alert variant="info" title="Security & Compliance Audit Trail">
        Administrative actions, authentication attempts, RBAC changes, token operations, and
        plugin events are persisted to <code className="font-mono">cms_audit_logs</code>. Sensitive
        fields (passwords, secrets, tokens, API keys) are automatically scrubbed to{' '}
        <code className="font-mono">[REDACTED]</code> before storage.
      </Alert>

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Filter className="w-4 h-4 text-sky-600" />
              <span>Filter Events</span>
            </span>
          }
          description="Narrow logs by actor, action name, target resource, outcome, or UTC date window."
          action={
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void loadLogs()}
                disabled={isLoading}
              >
                <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin' : ''}`} />
                <span>Refresh</span>
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => void handleExportCsv()}
                isLoading={isExporting}
              >
                <Download className="w-3.5 h-3.5" />
                <span>Export CSV</span>
              </Button>
            </div>
          }
        />
        <CardContent>
          <form onSubmit={handleApplyFilters} className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Input
                label="Actor ID"
                placeholder="e.g. user UUID or system"
                value={draft.actor_id}
                onChange={(e) => setDraft((prev) => ({ ...prev, actor_id: e.target.value }))}
              />
              <Input
                label="Action"
                placeholder="e.g. admin_login, token.create"
                value={draft.action}
                onChange={(e) => setDraft((prev) => ({ ...prev, action: e.target.value }))}
              />
              <Input
                label="Resource"
                placeholder="e.g. auth:session, settings:email"
                value={draft.resource}
                onChange={(e) => setDraft((prev) => ({ ...prev, resource: e.target.value }))}
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-4 gap-4 items-end">
              <Select
                label="Result"
                value={draft.result}
                onChange={(e) => setDraft((prev) => ({ ...prev, result: e.target.value }))}
                options={[
                  { label: 'All outcomes', value: '' },
                  { label: 'Success', value: 'success' },
                  { label: 'Failure', value: 'failure' },
                ]}
              />
              <Input
                label="From Date"
                type="date"
                value={draft.from}
                onChange={(e) => setDraft((prev) => ({ ...prev, from: e.target.value }))}
              />
              <Input
                label="To Date"
                type="date"
                value={draft.to}
                onChange={(e) => setDraft((prev) => ({ ...prev, to: e.target.value }))}
              />
              <div className="flex items-center gap-2">
                <Button type="submit" size="sm" className="flex-1">
                  Apply Filters
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleResetFilters}
                  title="Reset all filters"
                >
                  <RotateCcw className="w-3.5 h-3.5" />
                  <span>Reset</span>
                </Button>
              </div>
            </div>
          </form>
        </CardContent>
      </Card>

      {error && (
        <Alert
          variant="error"
          title="Unable to load audit logs"
          action={
            <Button type="button" size="sm" variant="outline" onClick={() => void loadLogs()}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <ClipboardList className="w-4 h-4 text-sky-600" />
              <span>Audit Log Stream</span>
            </span>
          }
          description={`Showing persisted system, authentication, settings, and plugin activity (${total} matching records).`}
        />
        <DataTable
          columns={columns}
          rows={rows}
          rowKey={(row) => row.id}
          isLoading={isLoading}
          onRowClick={(row) => setSelectedEntry(row)}
          emptyState={
            <EmptyState
              icon={<ShieldCheck className="w-5 h-5" />}
              title={hasActiveFilters ? 'No audit events match filters' : 'No audit events recorded yet'}
              description={
                hasActiveFilters
                  ? 'Try broadening your actor, action, resource, or date window filters.'
                  : 'Security, login, settings, and plugin events will appear here automatically.'
              }
              action={
                hasActiveFilters ? (
                  <Button type="button" variant="outline" size="sm" onClick={handleResetFilters}>
                    Reset Filters
                  </Button>
                ) : undefined
              }
            />
          }
        />
        {!isLoading && total > 0 && (
          <Pagination
            total={total}
            limit={PAGE_LIMIT}
            offset={offset}
            onPageChange={(nextOffset) => setOffset(nextOffset)}
          />
        )}
      </Card>

      <Modal
        isOpen={selectedEntry !== null}
        onClose={() => setSelectedEntry(null)}
        title="Audit Event Details"
        description={selectedEntry ? `Event ID: ${selectedEntry.id}` : undefined}
        maxWidth="xl"
      >
        {selectedEntry && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3 text-xs bg-slate-50 border border-slate-200 rounded-xl p-4">
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  Action
                </p>
                <p className="font-mono font-semibold text-slate-900 mt-0.5">
                  {selectedEntry.action}
                </p>
              </div>
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  Result
                </p>
                <div className="mt-0.5">
                  <Badge variant={resultBadgeVariant(selectedEntry.result)}>
                    {selectedEntry.result || 'recorded'}
                  </Badge>
                </div>
              </div>
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  Actor
                </p>
                <p className="font-mono text-slate-800 mt-0.5 break-all">
                  {selectedEntry.actor_type || 'system'} / {selectedEntry.actor_id || 'system'}
                </p>
              </div>
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  IP Address
                </p>
                <p className="font-mono text-slate-800 mt-0.5">
                  {selectedEntry.ip_address || '—'}
                </p>
              </div>
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  Resource
                </p>
                <p className="font-mono text-slate-800 mt-0.5 break-all">
                  {selectedEntry.resource}
                  {selectedEntry.target_id ? ` (${selectedEntry.target_id})` : ''}
                </p>
              </div>
              <div>
                <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                  Timestamp (UTC)
                </p>
                <p className="font-mono text-slate-800 mt-0.5">{selectedEntry.created_at}</p>
              </div>
              {selectedEntry.user_agent && (
                <div className="col-span-2">
                  <p className="text-slate-400 uppercase font-semibold tracking-wider text-[10px]">
                    User Agent
                  </p>
                  <p className="font-mono text-slate-700 mt-0.5 break-all">
                    {selectedEntry.user_agent}
                  </p>
                </div>
              )}
            </div>

            <div>
              <p className="text-xs font-semibold text-slate-700 uppercase tracking-wider mb-1.5">
                Redacted Metadata Payload
              </p>
              <pre className="bg-slate-900 text-slate-100 rounded-xl p-4 text-xs font-mono overflow-x-auto max-h-72">
                {JSON.stringify(selectedEntry.metadata ?? {}, null, 2)}
              </pre>
            </div>

            <div className="flex justify-end pt-2">
              <Button type="button" variant="outline" onClick={() => setSelectedEntry(null)}>
                Close
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};

