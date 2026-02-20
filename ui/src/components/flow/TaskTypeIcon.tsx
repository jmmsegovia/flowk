import { CSSProperties, memo, ReactNode } from 'react';

interface TaskTypeIconProps {
  action?: string;
}

interface BadgeVariant {
  icon: ReactNode;
  color: string;
  background: string;
  label: string;
}

const iconBaseProps = {
  width: 20,
  height: 20,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  role: 'presentation' as const,
  focusable: 'false' as const,
  'aria-hidden': 'true' as const
} as const;

const icons: Record<string, ReactNode> = {
  cloud: (
    <svg {...iconBaseProps}>
      <path d="M6 15h11a3 3 0 0 0 0-6 4.5 4.5 0 0 0-8.3-1.5A3.5 3.5 0 0 0 6 15z" />
    </svg>
  ),
  database: (
    <svg {...iconBaseProps}>
      <ellipse cx="12" cy="6.5" rx="7" ry="3.5" />
      <path d="M5 6.5v8.5c0 1.9 3.1 3.5 7 3.5s7-1.6 7-3.5V6.5" />
      <path d="M5 12c0 1.9 3.1 3.5 7 3.5s7-1.6 7-3.5" />
    </svg>
  ),
  cluster: (
    <svg {...iconBaseProps}>
      <path d="M12 3 18.5 6.5v7L12 17 5.5 13.5v-7z" />
      <path d="M12 3v14" />
      <path d="m5.5 6.5 6.5 3.5 6.5-3.5" />
    </svg>
  ),
  code: (
    <svg {...iconBaseProps}>
      <path d="m8 7-4 5 4 5" />
      <path d="m16 7 4 5-4 5" />
    </svg>
  ),
  terminal: (
    <svg {...iconBaseProps}>
      <path d="m6 8 4 4-4 4" />
      <path d="M11 16h7" />
    </svg>
  ),
  nodes: (
    <svg {...iconBaseProps}>
      <circle cx="7" cy="7" r="3" />
      <circle cx="17" cy="5" r="2.5" />
      <circle cx="17" cy="15" r="3.2" />
      <path d="m9.5 8.5 4.5 4.5" />
      <path d="M10 6.3 14.5 5" />
    </svg>
  ),
  split: (
    <svg {...iconBaseProps}>
      <path d="M6 6.5 12 12l6-5.5" />
      <path d="M6 17.5 12 12l6 5.5" />
    </svg>
  ),
  diamond: (
    <svg {...iconBaseProps}>
      <path d="M12 4.5 19 12l-7 7.5L5 12z" />
    </svg>
  ),
  moon: (
    <svg {...iconBaseProps}>
      <path d="M15 4a7 7 0 1 0 5 11 7 7 0 0 1-5-11z" />
    </svg>
  ),
  printer: (
    <svg {...iconBaseProps}>
      <path d="M7 10V5h10v5" />
      <rect x="5" y="11" width="14" height="8" rx="2" />
      <path d="M7 15h10" />
      <circle cx="17" cy="13" r="0.8" />
    </svg>
  ),
  shield: (
    <svg {...iconBaseProps}>
      <path d="M12 4 19 7v5c0 4.2-3 7.7-7 8.8-4-1.1-7-4.6-7-8.8V7z" />
      <path d="M12 11v6" />
      <path d="m9 14 3-3 3 3" />
    </svg>
  ),
  key: (
    <svg {...iconBaseProps}>
      <circle cx="9" cy="15" r="3" />
      <path d="M12 15h7v-3" />
      <path d="M19 9v3" />
    </svg>
  ),
  antenna: (
    <svg {...iconBaseProps}>
      <path d="M12 6a5 5 0 0 1 5 5" />
      <path d="M12 6a5 5 0 0 0-5 5" />
      <path d="M12 9.5a1.5 1.5 0 1 1 0 3" />
      <path d="M12 13v5" />
    </svg>
  ),
  document: (
    <svg {...iconBaseProps}>
      <path d="M8 4h7l3 3v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z" />
      <path d="M15 4v4h4" />
      <path d="M9 13h6" />
      <path d="M9 17h4" />
    </svg>
  ),
  arrow: (
    <svg {...iconBaseProps}>
      <path d="M5 12h14" />
      <path d="m13 7 6 5-6 5" />
    </svg>
  ),
  loop: (
    <svg {...iconBaseProps}>
      <path d="M5 11V7h4" />
      <path d="M19 13v4h-4" />
      <path d="M9 7a7 7 0 0 1 11 3" />
      <path d="M15 17a7 7 0 0 1-11-3" />
    </svg>
  ),
  mail: (
    <svg {...iconBaseProps}>
      <rect width="20" height="16" x="2" y="4" rx="2" />
      <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
    </svg>
  ),
  slack: (
    <svg {...iconBaseProps}>
      <rect width="3" height="8" x="13" y="2" rx="1.5" />
      <path d="M19 10a3 3 0 1 1-3 3" />
      <rect width="8" height="3" x="11" y="13" rx="1.5" />
      <path d="M10 19a3 3 0 1 1 3-3" />
      <rect width="3" height="8" x="8" y="14" rx="1.5" />
      <path d="M5 14a3 3 0 1 1 3-3" />
      <rect width="8" height="3" x="5" y="8" rx="1.5" />
      <path d="M14 5a3 3 0 1 1-3 3" />
    </svg>
  ),
  telegram: (
    <svg {...iconBaseProps}>
      <path d="m22 2-7 20-4-9-9-4Z" />
      <path d="M22 2 11 13" />
    </svg>
  ),
  webhook: (
    <svg {...iconBaseProps}>
      <path d="M18 16.98h-5.99c-1.1 0-1.95.94-2.48 1.9A4 4 0 0 1 2 17c.01-.7.2-1.4.57-2" />
      <path d="m6 17 3.13-5.78c.53-.97.1-2.18-.5-3.1a4 4 0 1 1 6.89-4.06" />
      <path d="m12 6 3.13 5.73C15.66 12.7 16.9 13 18 13a4 4 0 0 1 0 8" />
    </svg>
  ),
  search: (
    <svg {...iconBaseProps}>
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  ),
  robot: (
    <svg {...iconBaseProps}>
      <rect width="18" height="10" x="3" y="11" rx="2" />
      <circle cx="12" cy="5" r="2" />
      <path d="M12 7v4" />
      <line x1="8" x2="8" y1="16" y2="16" />
      <line x1="16" x2="16" y1="16" y2="16" />
    </svg>
  ),
  calendar: (
    <svg {...iconBaseProps}>
      <rect width="18" height="18" x="3" y="4" rx="2" ry="2" />
      <line x1="16" x2="16" y1="2" y2="6" />
      <line x1="8" x2="8" y1="2" y2="6" />
      <line x1="3" x2="21" y1="10" y2="10" />
    </svg>
  ),
  user: (
    <svg {...iconBaseProps}>
      <path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2" />
      <circle cx="12" cy="7" r="4" />
    </svg>
  ),
  check: (
    <svg {...iconBaseProps}>
      <polyline points="20 6 9 17 4 12" />
    </svg>
  ),
  bell: (
    <svg {...iconBaseProps}>
      <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
      <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
    </svg>
  ),
  file: (
    <svg {...iconBaseProps}>
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
      <polyline points="14 2 14 8 20 8" />
    </svg>
  ),
  container: (
    <svg {...iconBaseProps}>
      <path d="m7.5 4.27 9 5.15" />
      <path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z" />
      <path d="m3.3 7 8.7 5 8.7-5" />
      <path d="M12 22v-9" />
    </svg>
  ),
  anchor: (
    <svg {...iconBaseProps}>
      <path d="M12 3a2 2 0 1 0 0 4a2 2 0 0 0 0-4z" />
      <path d="M12 7v9" />
      <path d="M7 12H3a9 9 0 0 0 9 9a9 9 0 0 0 9-9h-4" />
      <path d="M8.5 12a3.5 3.5 0 0 1 7 0" />
    </svg>
  )
};

const buildVariant = (iconKey: string, color: string, background: string, label: string): BadgeVariant => ({
  icon: icons[iconKey],
  color,
  background,
  label
});

const DEFAULT_VARIANT = buildVariant('document', '#475569', '#f1f5f9', 'Task');

const ACTION_VARIANTS: Record<string, BadgeVariant> = {
  // Core / Control Flow
  SUBFLOW: buildVariant('nodes', '#f97316', '#fff7ed', 'Subflow'),
  PARALLEL: buildVariant('split', '#a855f7', '#faf5ff', 'Parallel'),
  EVALUATE: buildVariant('diamond', '#f59e0b', '#fffbeb', 'Evaluate'),
  SLEEP: buildVariant('moon', '#6366f1', '#eef2ff', 'Sleep'),
  FOR: buildVariant('loop', '#06b6d4', '#ecfeff', 'Loop'),
  VARIABLES: buildVariant('code', '#3b82f6', '#eff6ff', 'Variables'),
  WAIT_FOR_EVENT: buildVariant('calendar', '#8b5cf6', '#f5f3ff', 'Wait Event'),

  // Network / System
  HTTP_REQUEST: buildVariant('arrow', '#0ea5e9', '#f0f9ff', 'HTTP Request'),
  SHELL: buildVariant('terminal', '#334155', '#f8fafc', 'Shell'),
  DOCKER: buildVariant('container', '#2496ed', '#e6f3ff', 'Docker'),
  SSH: buildVariant('key', '#10b981', '#ecfdf5', 'SSH'),
  TELNET: buildVariant('antenna', '#0284c7', '#e0f2fe', 'Telnet'),
  PRINT: buildVariant('printer', '#64748b', '#f1f5f9', 'Print'),

  // Data / Storage
  GCLOUD_STORAGE: buildVariant('cloud', '#7c3aed', '#f3e8ff', 'GCloud'),
  DB_CASSANDRA_OPERATION: buildVariant('database', '#0d9488', '#f0fdfa', 'Cassandra'),
  DB_POSTGRES_OPERATION: buildVariant('database', '#2563eb', '#eff6ff', 'PostgreSQL'),
  DB_MYSQL_OPERATION: buildVariant('database', '#00758f', '#e0f7fa', 'MySQL'),
  BASE64: buildVariant('file', '#b45309', '#fffbeb', 'Base64'),
  PGP: buildVariant('shield', '#dc2626', '#fef2f2', 'PGP'),
  OAUTH2: buildVariant('key', '#f59e0b', '#fffbeb', 'OAuth2'),

  // Communications / Integrations
  GMAIL: buildVariant('mail', '#ea4335', '#fef2f2', 'Gmail'),
  SLACK: buildVariant('slack', '#4a154b', '#fdf4ff', 'Slack'),
  TELEGRAM: buildVariant('telegram', '#2aabee', '#f0f9ff', 'Telegram'),
  SEND_MESSAGE: buildVariant('bell', '#ec4899', '#fdf2f8', 'Notify'),

  // External Services
  KUBERNETES: buildVariant('cluster', '#3b82f6', '#eff6ff', 'Kubernetes'),
  HELM: buildVariant('anchor', '#0f766e', '#f0fdfa', 'Helm'),
  OPENAI: buildVariant('robot', '#10a37f', '#ecfdf5', 'OpenAI'),
  GOOGLE_SEARCH: buildVariant('search', '#4285f4', '#eff6ff', 'Search')
};

export const getVariantForAction = (action?: string): BadgeVariant => {
  if (!action || typeof action !== 'string') {
    return DEFAULT_VARIANT;
  }
  const normalized = action.trim().toUpperCase();
  // Handle some common aliases or sub-types if necessary
  if (normalized.includes('GMAIL')) return ACTION_VARIANTS.GMAIL;
  if (normalized.includes('SLACK')) return ACTION_VARIANTS.SLACK;
  if (normalized.includes('TELEGRAM')) return ACTION_VARIANTS.TELEGRAM;

  return ACTION_VARIANTS[normalized] ?? DEFAULT_VARIANT;
};

function TaskTypeIcon({ action }: TaskTypeIconProps) {
  const variant = getVariantForAction(action);
  const style = {
    '--task-node-badge-color': variant.color,
    '--task-node-badge-background': variant.background
  } as CSSProperties;

  return (
    <div className="task-node__badge" style={style} title={`${variant.label}${action ? ` · ${action}` : ''}`}>
      <div className="task-node__badge-inner">{variant.icon}</div>
    </div>
  );
}

export default memo(TaskTypeIcon);
