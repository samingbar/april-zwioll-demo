import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  Circle,
  Clock3,
  Cloud,
  Copy,
  Database,
  GitBranch,
  ImageIcon,
  Layers3,
  Loader2,
  Play,
  Radio,
  RefreshCw,
  Search,
  Server,
  Sparkles,
  TerminalSquare,
  UploadCloud,
  Workflow,
  XCircle
} from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | string;

type TraceStep = {
  key: string;
  order: number;
  label: string;
  detail: string;
  activityName?: string;
  status: StepStatus;
  timestamp?: string;
  durationMs?: number;
  errorMessage?: string;
};

type ActivityLogEntry = {
  time: string;
  activityName: string;
  status: string;
  durationMs?: number;
  message?: string;
};

type Rendition = {
  name: string;
  uri: string;
  width: number;
  height: number;
};

type TraceRecord = {
  traceId: string;
  eventId: string;
  eventType: string;
  topic: string;
  workflowId: string;
  runId: string;
  correlationId: string;
  assetId: string;
  listingId: string;
  sourceUrl: string;
  destinationUrl: string;
  contentType: string;
  sizeBytes: number;
  requestedRenditions: string[];
  status: string;
  currentStage: string;
  steps: TraceStep[];
  activityLog: ActivityLogEntry[];
  renditions: Rendition[];
  publishedUrl: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
};

type EventRecord = {
  eventId: string;
  traceId: string;
  assetId: string;
  topic: string;
  eventType: string;
  status: string;
  correlationId: string;
  createdAt: string;
  updatedAt: string;
};

type DLQRecord = {
  id: string;
  eventId?: string;
  traceId?: string;
  assetId?: string;
  topic?: string;
  reason: string;
  rawMessage: string;
  createdAt: string;
};

type MetricsSnapshot = {
  eventsConsumed: number;
  workflowsStarted: number;
  completed: number;
  failed: number;
  dlqCount: number;
  averageLagSeconds: number;
};

type SystemStatus = {
  environment: string;
  kafkaSim: string;
  temporalDev: string;
  worker: string;
  api: string;
};

type Snapshot = {
  system: SystemStatus;
  metrics: MetricsSnapshot;
  events: EventRecord[];
  traces: TraceRecord[];
  dlq: DLQRecord[];
};

const emptySnapshot: Snapshot = {
  system: {
    environment: 'Local',
    kafkaSim: 'In-process',
    temporalDev: 'localhost:7233',
    worker: 'sparkle-image-processing',
    api: ':8080'
  },
  metrics: {
    eventsConsumed: 0,
    workflowsStarted: 0,
    completed: 0,
    failed: 0,
    dlqCount: 0,
    averageLagSeconds: 0
  },
  events: [],
  traces: [],
  dlq: []
};

const navItems = [
  { label: 'Trace', icon: Activity, active: true },
  { label: 'Events', icon: Database },
  { label: 'Workflows', icon: GitBranch },
  { label: 'Read Model', icon: Server },
  { label: 'DLQ', icon: AlertTriangle }
];

export function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>(emptySnapshot);
  const [selectedTraceId, setSelectedTraceId] = useState<string>('');
  const [query, setQuery] = useState('');
  const [busyAction, setBusyAction] = useState<string>('');

  useEffect(() => {
    void refresh();
    const stream = new EventSource('/api/stream');
    stream.addEventListener('snapshot', (event) => {
      const next = normalizeSnapshot(JSON.parse((event as MessageEvent).data) as Partial<Snapshot>);
      setSnapshot(next);
      setSelectedTraceId((current) => current || next.traces[0]?.traceId || '');
    });
    stream.onerror = () => {
      stream.close();
    };
    return () => stream.close();
  }, []);

  useEffect(() => {
    if (!selectedTraceId && snapshot.traces[0]) {
      setSelectedTraceId(snapshot.traces[0].traceId);
    }
  }, [selectedTraceId, snapshot.traces]);

  const filteredEvents = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return snapshot.events;
    return snapshot.events.filter((event) =>
      [event.eventId, event.assetId, event.topic, event.status, event.correlationId]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    );
  }, [snapshot.events, query]);

  const selectedTrace = useMemo(() => {
    return snapshot.traces.find((trace) => trace.traceId === selectedTraceId) || snapshot.traces[0];
  }, [selectedTraceId, snapshot.traces]);

  async function refresh() {
    const response = await fetch('/api/snapshot');
    if (response.ok) {
      const next = normalizeSnapshot((await response.json()) as Partial<Snapshot>);
      setSnapshot(next);
      setSelectedTraceId((current) => current || next.traces[0]?.traceId || '');
    }
  }

  async function runAction(name: string, path: string) {
    setBusyAction(name);
    try {
      await fetch(path, { method: 'POST' });
      await refresh();
    } finally {
      setBusyAction('');
    }
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <Sparkles size={31} strokeWidth={1.8} />
          <div>
            <strong>Sparkle</strong>
            <span>Trace</span>
          </div>
        </div>
        <nav>
          {navItems.map((item) => (
            <button key={item.label} className={item.active ? 'active' : ''} title={item.label}>
              <item.icon size={19} />
              <span>{item.label}</span>
            </button>
          ))}
        </nav>
        <div className="version">
          <TerminalSquare size={18} />
          <span>v0.1.0-demo</span>
        </div>
      </aside>

      <main className="main">
        <header className="topbar">
          <div className="env-label">Environment</div>
          <StatusPill icon={Cloud} label={snapshot.system.environment} />
          <StatusPill icon={Radio} label="Kafka Sim" detail={snapshot.system.kafkaSim} />
          <StatusPill icon={Workflow} label="Temporal Dev" detail={snapshot.system.temporalDev} />
          <StatusPill icon={Layers3} label="Worker" detail={snapshot.system.worker} />
          <StatusPill icon={Server} label="API" detail={snapshot.system.api} />
          <div className="topbar-spacer" />
          <button className="icon-button" onClick={refresh} title="Refresh">
            <RefreshCw size={18} />
          </button>
        </header>

        <section className="action-strip">
          <div>
            <h1>Sparkle Trace</h1>
            <p>End-to-end visibility from event ingestion through durable workflow execution and read-model projection.</p>
          </div>
          <div className="actions">
            <button onClick={() => runAction('sample', '/api/demo/sample')} disabled={busyAction !== ''}>
              {busyAction === 'sample' ? <Loader2 className="spin" size={17} /> : <Play size={17} />}
              Publish sample
            </button>
            <button onClick={() => runAction('burst', '/api/demo/burst?count=5')} disabled={busyAction !== ''}>
              <Activity size={17} />
              Publish burst
            </button>
            <button className="danger-light" onClick={() => runAction('failure', '/api/demo/failure')} disabled={busyAction !== ''}>
              <AlertTriangle size={17} />
              Simulate failure
            </button>
          </div>
        </section>

        <section className="dashboard-grid">
          <EventStream events={filteredEvents} query={query} setQuery={setQuery} selectedTraceId={selectedTrace?.traceId || ''} onSelect={setSelectedTraceId} />
          <TraceTimeline trace={selectedTrace} />
          <TraceDetails trace={selectedTrace} dlq={snapshot.dlq ?? []} />
        </section>

        <section className="metric-row">
          <MetricCard icon={Activity} label="Events Consumed" value={snapshot.metrics.eventsConsumed.toString()} tone="teal" delta="+ local stream" />
          <MetricCard icon={Play} label="Workflows Started" value={snapshot.metrics.workflowsStarted.toString()} tone="green" delta="deterministic IDs" />
          <MetricCard icon={Clock3} label="Avg Projection Lag" value={`${snapshot.metrics.averageLagSeconds.toFixed(2)}s`} tone="amber" delta="event to visible" />
          <MetricCard icon={AlertTriangle} label="DLQ Count" value={snapshot.metrics.dlqCount.toString()} tone="red" delta={`${snapshot.metrics.failed} failed traces`} />
        </section>

        <PizzaTracker trace={selectedTrace} />
      </main>
    </div>
  );
}

function normalizeSnapshot(snapshot: Partial<Snapshot>): Snapshot {
  return {
    system: { ...emptySnapshot.system, ...(snapshot.system ?? {}) },
    metrics: { ...emptySnapshot.metrics, ...(snapshot.metrics ?? {}) },
    events: Array.isArray(snapshot.events) ? snapshot.events : [],
    traces: Array.isArray(snapshot.traces) ? snapshot.traces : [],
    dlq: Array.isArray(snapshot.dlq) ? snapshot.dlq : []
  };
}

function StatusPill({ icon: Icon, label, detail }: { icon: typeof Cloud; label: string; detail?: string }) {
  return (
    <div className="status-pill" title={detail || label}>
      <Icon size={17} />
      <span>{label}</span>
      <i />
    </div>
  );
}

function EventStream({
  events,
  query,
  setQuery,
  selectedTraceId,
  onSelect
}: {
  events: EventRecord[];
  query: string;
  setQuery: (query: string) => void;
  selectedTraceId: string;
  onSelect: (traceId: string) => void;
}) {
  return (
    <section className="panel event-panel">
      <div className="panel-header">
        <div>
          <h2>Event Stream</h2>
          <span>{events.length} latest messages</span>
        </div>
        <div className="search">
          <Search size={15} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter" />
        </div>
      </div>
      <div className="table">
        <div className="thead">
          <span>Event ID</span>
          <span>Asset ID</span>
          <span>Topic</span>
          <span>Status</span>
        </div>
        {events.length === 0 ? (
          <div className="empty-row">No events</div>
        ) : (
          events.slice(0, 12).map((event) => (
            <button
              key={event.eventId}
              className={`trow ${selectedTraceId === event.traceId ? 'selected' : ''}`}
              onClick={() => onSelect(event.traceId)}
            >
              <span className="mono truncate">{event.eventId}</span>
              <span className="truncate">{event.assetId}</span>
              <span className="truncate">{event.topic}</span>
              <span className={`status-dot ${toneForStatus(event.status)}`}>
                {iconForStatus(event.status)}
                {event.status}
              </span>
            </button>
          ))
        )}
      </div>
    </section>
  );
}

function TraceTimeline({ trace }: { trace?: TraceRecord }) {
  return (
    <section className="panel timeline-panel">
      <div className="panel-header">
        <div>
          <h2>Trace Timeline</h2>
          <span>{trace?.workflowId || 'Waiting for workflow'}</span>
        </div>
        <div className="auto-refresh">
          <span />
          Auto-refresh
        </div>
      </div>
      <div className="timeline">
        {(trace?.steps || []).map((step) => (
          <div key={step.key} className={`timeline-step ${step.status}`}>
            <div className="timeline-marker">{markerForStep(step)}</div>
            <div className="timeline-body">
              <div className="step-title">
                <strong>{step.order}. {step.label}</strong>
                <small className={step.status}>{labelForStatus(step.status)}</small>
              </div>
              <div className="step-detail">{step.detail}</div>
              {step.errorMessage && <div className="step-error">{step.errorMessage}</div>}
            </div>
            <div className="step-time">
              <span>{formatTime(step.timestamp)}</span>
              <small>{step.durationMs ? `${step.durationMs} ms` : '-'}</small>
            </div>
          </div>
        ))}
        {!trace && <div className="empty-row">No trace selected</div>}
      </div>
    </section>
  );
}

function TraceDetails({ trace, dlq }: { trace?: TraceRecord; dlq: DLQRecord[] }) {
  const payload = trace
    ? {
        assetId: trace.assetId,
        bucket: 'sparkle-local',
        sourceUrl: trace.sourceUrl,
        contentType: trace.contentType,
        sizeBytes: trace.sizeBytes,
        renditions: trace.requestedRenditions
      }
    : {};
  const dlqEntry = trace ? dlq.find((entry) => entry.traceId === trace.traceId) : undefined;

  return (
    <section className="panel details-panel">
      <div className="panel-header">
        <div>
          <h2>Trace Details</h2>
          <span>{trace?.traceId || 'No active trace'}</span>
        </div>
        <Copy size={17} />
      </div>
      <dl className="detail-list">
        <Detail label="Workflow ID" value={trace?.workflowId} />
        <Detail label="Run ID" value={trace?.runId} />
        <Detail label="Asset ID" value={trace?.assetId} />
        <Detail label="Correlation ID" value={trace?.correlationId} />
        <Detail label="Workflow Type" value="ImageProcessingWorkflow" />
        <Detail label="Status" value={trace?.status} status={trace?.status} />
      </dl>
      <div className="payload-block">
        <h3>Payload Preview</h3>
        <pre>{JSON.stringify(payload, null, 2)}</pre>
      </div>
      <div className="retry-grid">
        <h3>Retry Policy</h3>
        <span>Initial interval</span><strong>1s</strong>
        <span>Backoff coefficient</span><strong>2.0</strong>
        <span>Maximum attempts</span><strong>3</strong>
        <span>Maximum interval</span><strong>10s</strong>
      </div>
      {dlqEntry && <div className="dlq-note"><AlertTriangle size={16} />{dlqEntry.reason}</div>}
      <div className="activity-log">
        <h3>Activity Log</h3>
        {(trace?.activityLog || []).slice(-8).map((entry, index) => (
          <div key={`${entry.activityName}-${index}`} className="log-row">
            <span>{formatTime(entry.time)}</span>
            <span>{entry.activityName}</span>
            <strong className={entry.status}>{entry.status}</strong>
          </div>
        ))}
      </div>
    </section>
  );
}

function Detail({ label, value, status }: { label: string; value?: string; status?: string }) {
  return (
    <>
      <dt>{label}</dt>
      <dd className={status ? toneForStatus(status) : ''}>{value || '-'}</dd>
    </>
  );
}

function MetricCard({ icon: Icon, label, value, tone, delta }: { icon: typeof Activity; label: string; value: string; tone: string; delta: string }) {
  return (
    <section className={`metric-card ${tone}`}>
      <div className="metric-icon"><Icon size={23} /></div>
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
        <small>{delta}</small>
      </div>
      <div className="sparkline" />
    </section>
  );
}

function PizzaTracker({ trace }: { trace?: TraceRecord }) {
  const steps = [
    { key: 'event-accepted', label: 'Pending', icon: Clock3 },
    { key: 'downloading', label: 'Downloading', icon: Cloud },
    { key: 'processing', label: 'Processing', icon: Layers3 },
    { key: 'uploading', label: 'Uploading', icon: UploadCloud },
    { key: 'visible', label: 'Completed', icon: CheckCircle2 }
  ];
  const completed = trace?.steps.filter((step) => step.status === 'completed').length || 0;
  const progress = trace ? Math.round((completed / Math.max(trace.steps.length, 1)) * 100) : 0;

  return (
    <section className="pizza-panel">
      <div className="pizza-header">
        <div>
          <ImageIcon size={21} />
          <h2>Sparkle Pizza Tracker</h2>
        </div>
        <span>Asset: {trace?.assetId || '-'}</span>
        <span>Workflow: {trace?.workflowId || '-'}</span>
      </div>
      <div className="pizza-content">
        <div className="tracker-line">
          {steps.map((item, index) => {
            const step = trace?.steps.find((candidate) => candidate.key === item.key);
            const Icon = item.icon;
            return (
              <div className={`tracker-node ${step?.status || 'pending'}`} key={item.key}>
                <div className="node-icon"><Icon size={22} /></div>
                <strong>{item.label}</strong>
                <small>{labelForStatus(step?.status || 'pending')}</small>
                {index < steps.length - 1 && <ChevronRight className="connector" size={18} />}
              </div>
            );
          })}
        </div>
        <div className="progress-track"><span style={{ width: `${progress}%` }} /></div>
        <div className="preview-strip">
          {(trace?.renditions?.length ? trace.renditions : placeholderRenditions()).map((rendition, index) => (
            <div className={`thumbnail thumb-${index}`} key={`${rendition.name}-${index}`}>
              <span>{rendition.name}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function placeholderRenditions(): Rendition[] {
  return [
    { name: 'hero', uri: '', width: 2400, height: 1600 },
    { name: 'gallery', uri: '', width: 1800, height: 1200 },
    { name: 'thumbnail', uri: '', width: 640, height: 480 }
  ];
}

function iconForStatus(status: string) {
  const tone = toneForStatus(status);
  if (tone === 'green') return <CheckCircle2 size={14} />;
  if (tone === 'red') return <XCircle size={14} />;
  if (tone === 'amber') return <Clock3 size={14} />;
  return <Circle size={14} />;
}

function markerForStep(step: TraceStep) {
  if (step.status === 'completed') return <CheckCircle2 size={20} />;
  if (step.status === 'running') return <Loader2 className="spin" size={20} />;
  if (step.status === 'failed') return <XCircle size={20} />;
  return <Circle size={20} />;
}

function toneForStatus(status: string) {
  const normalized = status.toLowerCase();
  if (['completed', 'consumed', 'success'].includes(normalized)) return 'green';
  if (['running', 'queued', 'pending', 'duplicate'].includes(normalized)) return 'amber';
  if (['failed', 'dlq', 'error'].includes(normalized)) return 'red';
  return 'teal';
}

function labelForStatus(status: string) {
  if (status === 'completed') return 'Success';
  if (status === 'running') return 'In progress';
  if (status === 'failed') return 'Failed';
  return 'Pending';
}

function formatTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
