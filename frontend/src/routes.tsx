import { createBrowserRouter,Navigate, Outlet } from 'react-router-dom'

import { AdminGuard } from '@/auth/AdminGuard'
import { RequireAuth } from '@/auth/RequireAuth'
import { TenantGuard } from '@/auth/TenantGuard'
import { GlobalLayout } from '@/layouts/GlobalLayout'
import { CallbackPage } from '@/pages/CallbackPage'
import { EscalationPoliciesPage } from '@/pages/EscalationPoliciesPage'
import { PolicyEditorPage } from '@/pages/PolicyEditorPage'
import { ForbiddenPage } from '@/pages/ForbiddenPage'
import { IncidentListPage } from '@/pages/IncidentListPage'
import { ProfilePage } from '@/pages/ProfilePage'
import { RootRedirectPage } from '@/pages/RootRedirectPage'
import { SchedulesPage } from '@/pages/SchedulesPage'
import { SelectTeamPage } from '@/pages/SelectTeamPage'
import { SilentRenewPage } from '@/pages/SilentRenewPage'
import { MembersPage } from '@/pages/settings/MembersPage'
import { NotificationConfigPage } from '@/pages/settings/NotificationConfigPage'
import { WebhookTokensPage } from '@/pages/settings/WebhookTokensPage'

export const router = createBrowserRouter([
  // Public routes — no auth required
  { path: '/callback', element: <CallbackPage /> },
  { path: '/silent-renew', element: <SilentRenewPage /> },
  { path: '/403', element: <ForbiddenPage /> },

  // Protected routes
  {
    element: (
      <RequireAuth>
        <Outlet />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <RootRedirectPage /> },
      { path: 'select-team', element: <SelectTeamPage /> },

      // Tenant-scoped routes
      {
        path: ':tenant',
        element: (
          <TenantGuard>
            <GlobalLayout />
          </TenantGuard>
        ),
        children: [
          { index: true, element: <Navigate to="incidents" replace /> },
          { path: 'incidents', element: <IncidentListPage /> },
          { path: 'schedules', element: <SchedulesPage /> },
          { path: 'escalations', element: <EscalationPoliciesPage /> },
          { path: 'escalations/new', element: <PolicyEditorPage /> },
          { path: 'escalations/:policyId/edit', element: <PolicyEditorPage /> },
          {
            path: 'settings',
            children: [
              // Profile is reachable by every member — kept outside AdminGuard.
              { path: 'profile', element: <ProfilePage /> },
              // Admin-only settings sections share a single guard via the pathless route.
              {
                element: (
                  <AdminGuard>
                    <Outlet />
                  </AdminGuard>
                ),
                children: [
                  { index: true, element: <Navigate to="webhook-tokens" replace /> },
                  { path: 'webhook-tokens', element: <WebhookTokensPage /> },
                  { path: 'notifications', element: <NotificationConfigPage /> },
                  { path: 'members', element: <MembersPage /> },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
])
