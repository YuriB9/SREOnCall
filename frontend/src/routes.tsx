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
import { TenantSettingsPage } from '@/pages/TenantSettingsPage'

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
      { path: 'profile', element: <ProfilePage /> },

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
            element: (
              <AdminGuard>
                <TenantSettingsPage />
              </AdminGuard>
            ),
          },
        ],
      },
    ],
  },
])
