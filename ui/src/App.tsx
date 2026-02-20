import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import AppShell from './components/layout/AppShell';

const ActionDetailPage = lazy(() => import('./pages/ActionDetailPage'));
const ActionGuidePage = lazy(() => import('./pages/ActionGuidePage'));
const FlowBuilderPage = lazy(() => import('./pages/FlowBuilderPage'));
const FlowListPage = lazy(() => import('./pages/FlowListPage'));

const loadingFallback = (
  <div style={{ padding: '1.5rem' }}>
    <p className="text-muted">Loading...</p>
  </div>
);

function App() {
  return (
    <AppShell>
      <Suspense fallback={loadingFallback}>
        <Routes>
          <Route path="/actions/guide" element={<ActionGuidePage />} />
          <Route path="/actions/guide/:actionName" element={<ActionDetailPage />} />
          <Route path="/flows" element={<FlowListPage />} />
          <Route path="/flows/:flowId" element={<FlowBuilderPage />} />
          <Route path="*" element={<Navigate to="/flows" replace />} />
        </Routes>
      </Suspense>
    </AppShell>
  );
}

export default App;
