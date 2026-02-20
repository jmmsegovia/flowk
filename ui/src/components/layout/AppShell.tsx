import { PropsWithChildren, useCallback, useMemo, useState } from 'react';
import { Link, NavLink } from 'react-router-dom';
import { Workflow, BookOpen } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import FlowImportsTree from '../flow/FlowImportsTree';
import ToastViewport from '../common/ToastViewport';

const navLinkClassName = ({ isActive }: { isActive: boolean }) =>
  isActive ? 'button' : 'button button--secondary';

function AppShell({ children }: PropsWithChildren) {
  const { t, i18n } = useTranslation();
  const languages = useMemo(
    () => [
      { code: 'en', label: 'EN' },
      { code: 'es', label: 'ES' }
    ],
    []
  );

  const changeLanguage = useCallback(
    (nextLanguage: string) => {
      i18n.changeLanguage(nextLanguage);
      if (typeof window !== 'undefined') {
        window.localStorage.setItem('appLanguage', nextLanguage);
      }
    },
    [i18n]
  );

  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);

  return (
    <div className={`app-shell ${isSidebarCollapsed ? 'sidebar-collapsed' : ''}`}>
      <aside className="app-shell__sidebar">
        <button
          className="sidebar-toggle"
          onClick={() => setIsSidebarCollapsed(!isSidebarCollapsed)}
          title={isSidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {isSidebarCollapsed ? '»' : '«'}
        </button>
        <div className="app-shell__sidebar-content">
          <Link className="app-shell__logo" to="/flows" aria-label={t('nav.flows')}>
            <img src="/images/logo_dark_mode.png" alt="FlowK" />
          </Link>
          <nav className="sidebar-nav">
            <ul>
              <li>
                <NavLink to="/flows" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
                  <Workflow size={20} />
                  <span>{t('nav.flows')}</span>
                </NavLink>
              </li>
            </ul>
          </nav>
          <FlowImportsTree />
          <div className="sidebar-bottom">
            <div className="sidebar-language">
              <p className="sidebar-language__label">{t('app.languageLabel')}</p>
              <div className="sidebar-language__options">
                {languages.map((option) => (
                  <button
                    key={option.code}
                    type="button"
                    className={option.code === i18n.language ? 'active' : ''}
                    onClick={() => changeLanguage(option.code)}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </div>
            <nav className="sidebar-nav">
              <ul>
                <li>
                  <NavLink to="/actions/guide" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
                    <BookOpen size={20} />
                    <span>{t('nav.actions')}</span>
                  </NavLink>
                </li>
              </ul>
            </nav>
          </div>
        </div>
      </aside>
      <main className="app-shell__content">{children}</main>
      <ToastViewport />
    </div>
  );
}

export default AppShell;
