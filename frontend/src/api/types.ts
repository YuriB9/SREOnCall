// ─── Incidents ───────────────────────────────────────────────────────────────

export type IncidentStatus = 'open' | 'acknowledged' | 'resolved'
export type IncidentSeverity = 'critical' | 'high' | 'medium' | 'low'

export interface Incident {
  id: string
  tenant: string
  title: string
  status: IncidentStatus
  severity: IncidentSeverity
  source: string
  created_at: string
  updated_at: string
  acknowledged_by?: string
  resolved_at?: string
}

export interface IncidentListResponse {
  incidents: Incident[]
  total: number
  page: number
  page_size: number
}

export interface Alert {
  id: string
  incident_id: string
  fingerprint: string
  labels: Record<string, string>
  status: 'firing' | 'resolved'
  started_at: string
  resolved_at?: string
}

export interface Comment {
  id: string
  incident_id: string
  author: string
  text: string
  created_at: string
}

export type HistoryEventKind =
  | 'created'
  | 'status_changed'
  | 'acknowledged'
  | 'resolved'
  | 'escalated'
  | 'comment'

export interface HistoryEntry {
  id: string
  incident_id: string
  kind: HistoryEventKind
  actor?: string
  detail?: string
  occurred_at: string
}

// ─── Schedules ────────────────────────────────────────────────────────────────

export interface RotationMember {
  user_id: string
  username: string
  display_name: string
}

export interface Schedule {
  id: string
  tenant: string
  name: string
  timezone: string
  members: RotationMember[]
  created_at: string
}

export interface ShiftWindow {
  schedule_id: string
  user_id: string
  username: string
  display_name: string
  start: string
  end: string
  is_override: boolean
}

export interface OnCallNow {
  schedule_id: string
  schedule_name: string
  user_id: string
  username: string
  display_name: string
  escalation_level: number
  shift_start: string
  shift_end: string
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

export interface EscalationStep {
  order: number
  schedule_id: string
  schedule_name?: string
  timeout_minutes: number
}

export interface EscalationPolicy {
  id: string
  tenant: string
  name: string
  is_default: boolean
  steps: EscalationStep[]
  created_at: string
  updated_at: string
}

// ─── Tenant Settings ──────────────────────────────────────────────────────────

export interface WebhookToken {
  id: string
  tenant: string
  source_label: string
  created_at: string
  created_by: string
}

export interface WebhookTokenCreated extends WebhookToken {
  token: string
}

export interface NotificationConfig {
  tenant: string
  mattermost_webhook_url: string
  mattermost_channel: string
  smtp_from: string
}

export interface Member {
  user_id: string
  username: string
  display_name: string
  email: string
  role: 'member' | 'admin'
}

// ─── User Profile ─────────────────────────────────────────────────────────────

export type NotificationChannel = 'email' | 'mattermost'

export interface UserContacts {
  user_id: string
  mattermost_username: string
  email: string
  enabled_channels: NotificationChannel[]
}
