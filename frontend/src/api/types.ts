// ─── Incidents ───────────────────────────────────────────────────────────────

export type IncidentStatus = 'open' | 'acknowledged' | 'resolved'
export type IncidentSeverity = 'critical' | 'high' | 'warning' | 'info'

export interface Incident {
  id: string
  tenant_id: string
  title: string
  status: IncidentStatus
  severity: IncidentSeverity
  labels?: Record<string, string>
  created_at: string
  updated_at: string
  acknowledged_by?: string | null
  acknowledged_at?: string | null
  resolved_at?: string | null
}

export interface IncidentListResponse {
  incidents: Incident[]
  next_cursor: string
}

export interface Alert {
  id: string
  incident_id: string
  fingerprint: string
  source: string
  group_key?: string
  status: 'firing' | 'resolved'
  attached_at: string
}

export interface Comment {
  id: string
  incident_id: string
  author_id: string
  body: string
  created_at: string
}

export type HistoryEventKind = 'status_change' | 'label_change' | 'comment_added'

export interface HistoryEntry {
  id: string
  incident_id: string
  kind: HistoryEventKind
  author?: string
  old_value?: string
  new_value?: string
  occurred_at: string
}

// ─── Schedules ────────────────────────────────────────────────────────────────

export interface Schedule {
  id: string
  tenant_id: string
  name: string
  timezone: string
  rotation: string[]
  shift_duration: string
  start_date: string
  created_at: string
  updated_at: string
}

export interface ShiftWindow {
  user_id: string
  start_at: string
  end_at: string
  is_override: boolean
}

export interface OnCallNow {
  user_id: string
  username: string
  starts_at: string
  ends_at: string
}

export interface Override {
  id: string
  schedule_id: string
  user_id: string
  username: string
  display_name: string
  start: string
  end: string
  created_by: string
  created_at: string
}

// ─── Escalation Policies ──────────────────────────────────────────────────────

export interface PolicyTier {
  id?: string
  policy_id?: string
  tier_number: number
  timeout_seconds: number
  notify_schedule_id: string
}

export interface EscalationPolicy {
  id: string
  tenant_id: string
  name: string
  tiers: PolicyTier[]
  created_at: string
}

export interface TenantEscalationConfig {
  tenant_id: string
  default_policy_id: string | null
  updated_at: string
}

// ─── Tenant Settings ──────────────────────────────────────────────────────────

export interface WebhookToken {
  id: string
  tenant_id: string
  source: string
  created_at: string
}

export interface WebhookTokenCreated extends WebhookToken {
  token: string
}

export interface NotificationConfig {
  tenant_id: string
  mattermost_webhook_url: string
  mattermost_channel: string
  smtp_from: string
}

export interface Member {
  user_id: string
  preferred_username: string
  role: 'member' | 'admin'
}

// ─── User Profile ─────────────────────────────────────────────────────────────

export type NotificationChannel = 'email' | 'mattermost'

export interface UserContacts {
  id?: string
  user_id: string
  tenant_id?: string
  mattermost_username: string
  email: string
  enabled_channels: NotificationChannel[]
  created_at?: string
  updated_at?: string
}
