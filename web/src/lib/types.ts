// Mirrors the JSON emitted by internal/server/api. Keep in sync with dto.go,
// store/metrics.go, store/alerts.go and store/settings.go.

export interface Meta {
  site_name: string;
  version: string;
  public_enabled: boolean;
  admin: boolean;
  keep_minute_days: number;
  keep_hour_days: number;
}

export interface NodeInfo {
  mem_total: number;
  disk_total: number;
  os: string;
  kernel: string;
  arch: string;
  cpu_model: string;
  cpu_cores: number;
  virt: string;
  agent_version: string;
}

export interface NodeStatus {
  cpu: number;
  load1: number;
  load5: number;
  load15: number;
  mem_total: number;
  mem_used: number;
  swap_total: number;
  swap_used: number;
  disk_total: number;
  disk_used: number;
  net_in: number;
  net_out: number;
  tcp: number;
  udp: number;
  procs: number;
  uptime: number;
}

export type TrafficMode = "sum" | "up" | "down" | "max";

export interface Traffic {
  up: number;
  down: number;
  used: number;
  limit: number;
  mode: TrafficMode;
  reset_day: number;
  period_start: number;
  next_reset_at: number;
  reset_utc_offset: number;
}

export interface PublicNode {
  has_reported: boolean;
  visibility: { system: boolean; expiry: boolean; traffic: boolean; ping: boolean };
  uuid: string;
  name: string;
  group: string;
  public_note: string;
  public_tags: string;
  expire_at: number;
  online: boolean;
  last_seen: number;
  info: NodeInfo;
  status: NodeStatus | null;
  network: NetworkProbe[];
  traffic: Traffic;
}

export interface NetworkProbe {
  target: string;
  avg_ms: number;
  failure_rate: number;
  fresh: boolean;
  latency_fresh: boolean;
  samples?: number;
  successes?: number;
  history?: NetworkBucket[];
}

export interface NetworkBucket {
  t: number;
  avg_ms: number;
  samples: number;
  successes: number;
}

export interface Asset {
  provider: string;
  region: string;
  tags: string;
  price: number;
  currency: string;
  billing_months: number;
  auto_renew: boolean;
}

export interface CostSummary {
  currency: string;
  nodes: number;
  monthly: number;
  annual: number;
}

export interface AdminNode extends PublicNode, Asset {
  id: number;
  sort: number;
  hidden: boolean;
  note: string;
  last_ip: string;
  traffic_limit: number;
  traffic_mode: TrafficMode;
  traffic_reset_day: number;
  created_at: number;
}

export interface NodeInput extends Asset {
  name: string;
  group: string;
  sort: number;
  hidden: boolean;
  note: string;
  public_note: string;
  public_tags: string;
  expire_at: number;
  traffic_limit: number;
  traffic_mode: TrafficMode;
  traffic_reset_day: number;
}

export interface Metric {
  t: number;
  cpu: number;
  load1: number;
  mem_used: number;
  swap_used: number;
  disk_used: number;
  net_in: number;
  net_out: number;
  tcp: number;
  udp: number;
  procs: number;
}

export interface PingSeries {
  name: string;
  points: { t: number; ms: number; loss: number }[];
}

export interface Settings {
  agent_idle_interval: number;
  probe_targets: string;
  probe_cards: string;
  probe_interval: number;
  public_system: boolean;
  public_expiry: boolean;
  public_traffic: boolean;
  public_ping: boolean;
  site_name: string;
  public_enabled: boolean;
  public_url: string;
  lang: "en" | "zh";
}

export interface Me {
  username: string;
  totp_enabled: boolean;
  default_password: boolean;
}

export type RuleKind = "offline" | "cpu" | "mem" | "disk" | "traffic" | "expire" | "latency" | "packet_loss";

export interface Rule {
  target: string;
  id: number;
  name: string;
  kind: RuleKind;
  threshold: number;
  duration: number;
  node_ids: number[];
  enabled: boolean;
  created_at: number;
}

export interface TelegramConfig {
  bot_token: string;
  chat_id: string;
  api_base?: string;
}

export interface WebhookConfig {
  url: string;
  method?: "GET" | "POST";
  headers?: Record<string, string>;
  body?: string;
}

export type Channel =
  | { id: number; name: string; type: "telegram"; config: TelegramConfig; enabled: boolean; created_at: number }
  | { id: number; name: string; type: "webhook"; config: WebhookConfig; enabled: boolean; created_at: number };

export interface AlertEvent {
  id: number;
  rule_id: number;
  node_id: number;
  kind: RuleKind;
  firing: boolean;
  message: string;
  created_at: number;
}

export interface DeliveryAttempt {
  number: number;
  started_at: number;
  finished_at: number;
  status: string;
  error: string;
}

export interface Delivery {
  id: number;
  channel_id: number;
  kind: string;
  firing: boolean;
  status: "pending" | "retrying" | "sending" | "succeeded" | "failed" | "canceled";
  attempts: number;
  max_attempts: number;
  created_at: number;
  updated_at: number;
  next_attempt_at: number;
  last_error: string;
  history: DeliveryAttempt[];
}

export interface AvailabilitySpan {
  start: number;
  end: number;
  state: "online" | "offline" | "unknown";
}

export interface AvailabilitySummary {
  from: number;
  to: number;
  online_seconds: number;
  offline_seconds: number;
  unknown_seconds: number;
  uptime_percent: number | null;
  coverage_percent: number;
  outage_count: number;
  longest_outage_seconds: number;
  timeline: AvailabilitySpan[];
  timeline_truncated: boolean;
}

export interface AdminSession {
  id: string;
  created_at: number;
  expires_at: number;
  ip: string;
  user_agent: string;
  current: boolean;
}

export interface SecurityAuditEvent {
  id: number;
  created_at: number;
  action: string;
  target: string;
  status: number;
  ip: string;
}

export interface SecurityAuditPage {
  items: SecurityAuditEvent[];
  next_before: number;
}
