// Reusable enterprise audit timeline. Presentational: the parent fetches
// enriched audit events (category/severity/actor/description supplied by the
// gateway) and hands them to this component, which renders a severity-coded,
// filterable, drill-down timeline. Shared by Admin CA and Admin Bank.
import { useMemo, useState } from "react";
import {
  Activity,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Loader2,
  RefreshCw,
  Search,
  ShieldAlert,
  ShieldCheck,
} from "lucide-react";

export type AuditSeverity = "info" | "warning" | "critical";
export type AuditCategory =
  | "key_issuance"
  | "cert_lifecycle"
  | "authentication"
  | "resource_access"
  | "admin_action";
export type AuditOutcome = "success" | "denied";
export type AuditSource = "ca" | "kdc" | "bank";

// The view model every source maps to. `toAuditVM` builds it from an enriched
// audit item returned by the gateway.
export interface AuditEventVM {
  id: string;
  source?: AuditSource;
  timestamp: string; // ISO 8601
  action: string;
  category: AuditCategory;
  severity: AuditSeverity;
  outcome: AuditOutcome;
  actorDisplay: string;
  actorType?: string;
  description: string;
  target?: string;
  reason?: string;
  requestId?: string;
  metadata?: unknown;
}

// Maps a gateway-enriched audit item (with category/severity/actor/description)
// to the timeline view model. Tolerates the small field-name differences
// between the CA/KDC/Bank list responses.
export function toAuditVM(raw: any, index = 0): AuditEventVM {
  const actor = raw.actor && typeof raw.actor === "object" ? raw.actor : undefined;
  return {
    id: raw.id ?? raw.event_id ?? `${raw.source ?? "ev"}-${index}`,
    source: raw.source,
    timestamp: raw.timestamp ?? raw.performed_at ?? raw.created_at ?? "",
    action: raw.action ?? "",
    category: raw.category ?? "cert_lifecycle",
    severity: raw.severity ?? "info",
    outcome: raw.outcome ?? "success",
    actorDisplay:
      raw.actor_display ?? actor?.display ?? raw.performed_by ?? raw.client_id ?? raw.user_id ?? "—",
    actorType: raw.actor_type ?? actor?.type,
    description: raw.description ?? raw.action ?? "",
    target: raw.target ?? raw.serial_number ?? raw.cert_serial ?? raw.transaction_id ?? "",
    reason: raw.reason ?? "",
    requestId: raw.request_id ?? raw.metadata?.request_id ?? "",
    metadata: raw.metadata,
  };
}

const SEVERITY_ORDER: AuditSeverity[] = ["info", "warning", "critical"];

const CATEGORY_LABEL: Record<AuditCategory, string> = {
  key_issuance: "Key issuance",
  cert_lifecycle: "Cert lifecycle",
  authentication: "Authentication",
  resource_access: "Resource access",
  admin_action: "Admin action",
};

function severityClasses(severity: AuditSeverity): string {
  switch (severity) {
    case "critical":
      return "border-red-500/30 bg-red-500/10 text-red-400";
    case "warning":
      return "border-amber-500/30 bg-amber-500/10 text-amber-300";
    default:
      return "border-emerald-500/25 bg-emerald-500/10 text-emerald-400";
  }
}

function SeverityIcon({ severity }: { severity: AuditSeverity }) {
  if (severity === "critical") return <ShieldAlert className="w-3.5 h-3.5" />;
  if (severity === "warning") return <AlertTriangle className="w-3.5 h-3.5" />;
  return <ShieldCheck className="w-3.5 h-3.5" />;
}

function formatIso(value: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return new Intl.DateTimeFormat("en-GB", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(d);
}

function shorten(value: string, keep = 8): string {
  return value.length > keep * 2 + 1
    ? `${value.slice(0, keep)}…${value.slice(-keep)}`
    : value;
}

export interface AuditTimelineProps {
  events: AuditEventVM[];
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
  onRefresh?: () => void;
  // Drill-down: reconstruct the full cross-service session for a trace id.
  onViewSession?: (requestId: string) => void;
  showSource?: boolean;
  emptyLabel?: string;
}

export function AuditTimeline({
  events,
  loading = false,
  error,
  onRetry,
  onRefresh,
  onViewSession,
  showSource = false,
  emptyLabel = "No audit events",
}: AuditTimelineProps) {
  const [severity, setSeverity] = useState<"all" | AuditSeverity>("all");
  const [category, setCategory] = useState<"all" | AuditCategory>("all");
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return events.filter((e) => {
      if (severity !== "all" && e.severity !== severity) return false;
      if (category !== "all" && e.category !== category) return false;
      if (q) {
        const hay = `${e.action} ${e.description} ${e.actorDisplay} ${e.target} ${e.reason} ${e.requestId}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }, [events, severity, category, query]);

  const counts = useMemo(() => {
    const c = { info: 0, warning: 0, critical: 0 };
    for (const e of events) c[e.severity]++;
    return c;
  }, [events]);

  return (
    <div className="flex flex-col gap-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-xs">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search action, actor, reason…"
            className="w-full h-9 rounded-md border border-border bg-card pl-9 pr-3 text-sm outline-none focus:border-cyan-500"
          />
        </div>

        <div className="flex rounded-md border border-border overflow-hidden">
          {(["all", ...SEVERITY_ORDER] as const).map((s) => (
            <button
              key={s}
              onClick={() => setSeverity(s)}
              className={`h-9 px-3 text-xs capitalize border-r border-border last:border-r-0 ${
                severity === s ? "bg-cyan-600 text-white" : "bg-card text-muted-foreground hover:text-foreground"
              }`}
            >
              {s === "all" ? "All" : s}
              {s !== "all" ? ` (${counts[s]})` : ""}
            </button>
          ))}
        </div>

        <select
          value={category}
          onChange={(e) => setCategory(e.target.value as "all" | AuditCategory)}
          className="h-9 rounded-md border border-border bg-card px-3 text-xs outline-none focus:border-cyan-500"
        >
          <option value="all">All categories</option>
          {(Object.keys(CATEGORY_LABEL) as AuditCategory[]).map((c) => (
            <option key={c} value={c}>
              {CATEGORY_LABEL[c]}
            </option>
          ))}
        </select>

        {onRefresh && (
          <button
            onClick={onRefresh}
            className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent"
            aria-label="Refresh"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
          </button>
        )}
      </div>

      {error && (
        <div className="flex items-center justify-between gap-3 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2">
          <p className="text-xs text-red-300">{error}</p>
          {onRetry && (
            <button onClick={onRetry} className="text-xs text-red-100 underline">
              Retry
            </button>
          )}
        </div>
      )}

      {/* Timeline */}
      <div className="rounded-lg border border-border bg-card divide-y divide-border">
        {loading ? (
          <div className="px-4 py-12 text-center text-muted-foreground">
            <Loader2 className="w-5 h-5 animate-spin mx-auto mb-2" /> Loading audit events
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-4 py-12 text-center text-muted-foreground">
            <Activity className="w-5 h-5 mx-auto mb-2 opacity-60" />
            {emptyLabel}
          </div>
        ) : (
          filtered.map((e) => {
            const open = expanded === e.id;
            return (
              <div key={e.id}>
                <button
                  onClick={() => setExpanded(open ? null : e.id)}
                  className="w-full flex items-start gap-3 px-4 py-3 text-left hover:bg-accent/30"
                >
                  <span className="mt-0.5 text-muted-foreground">
                    {open ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                  </span>
                  <span
                    className={`mt-0.5 inline-flex items-center gap-1 px-2 py-0.5 rounded-md border text-xs whitespace-nowrap ${severityClasses(
                      e.severity,
                    )}`}
                  >
                    <SeverityIcon severity={e.severity} />
                    {e.severity}
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm text-foreground">{e.description}</span>
                      {e.outcome === "denied" && (
                        <span className="text-xs text-red-300 border border-red-500/30 rounded px-1.5">denied</span>
                      )}
                    </div>
                    <div className="mt-1 flex items-center gap-2 flex-wrap text-xs text-muted-foreground">
                      <span className="font-mono">{e.action}</span>
                      <span>·</span>
                      <span>{CATEGORY_LABEL[e.category]}</span>
                      <span>·</span>
                      <span>{e.actorDisplay}</span>
                      {showSource && e.source && (
                        <>
                          <span>·</span>
                          <span className="uppercase">{e.source}</span>
                        </>
                      )}
                    </div>
                  </div>
                  <span className="mt-0.5 text-xs text-muted-foreground whitespace-nowrap">
                    {formatIso(e.timestamp)}
                  </span>
                </button>

                {open && (
                  <div className="px-11 pb-4 -mt-1 text-xs space-y-1.5">
                    <Detail label="Actor" value={`${e.actorDisplay}${e.actorType ? ` (${e.actorType})` : ""}`} />
                    {e.target && <Detail label="Target" value={e.target} mono />}
                    {e.reason && <Detail label="Reason" value={e.reason} />}
                    {e.requestId && <Detail label="Request ID" value={e.requestId} mono />}
                    {e.metadata != null && (
                      <div>
                        <span className="text-muted-foreground">Metadata: </span>
                        <pre className="mt-1 rounded-md border border-border bg-background p-2 overflow-auto text-[11px] text-muted-foreground">
                          {JSON.stringify(e.metadata, null, 2)}
                        </pre>
                      </div>
                    )}
                    {onViewSession && e.requestId && (
                      <button
                        onClick={() => onViewSession(e.requestId!)}
                        className="mt-1 inline-flex items-center gap-1.5 h-8 px-3 rounded-md border border-border text-xs hover:bg-accent"
                      >
                        <Activity className="w-3.5 h-3.5" />
                        View full session ({shorten(e.requestId, 6)})
                      </button>
                    )}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex gap-2">
      <span className="text-muted-foreground w-24 shrink-0">{label}:</span>
      <span className={`text-foreground break-all ${mono ? "font-mono" : ""}`}>{value}</span>
    </div>
  );
}
