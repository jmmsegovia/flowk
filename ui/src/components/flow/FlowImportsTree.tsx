import { useTranslation } from 'react-i18next';
import useFlowStore from '../../state/flowStore';

function FlowImportsTree() {
  const imports = useFlowStore((state) => state.importsTree);
  const focusOnTask = useFlowStore((state) => state.focusOnTask);
  const { t } = useTranslation();

  const handleClick = (taskId?: string) => {
    if (!taskId) {
      return;
    }
    focusOnTask(taskId);
  };

  if (!imports.length) {
    return null;
  }

  const handleMouseEnter = (e: React.MouseEvent<HTMLButtonElement>) => {
    const button = e.currentTarget;
    const span = button.querySelector('span');
    if (!span) return;

    const overflow = span.scrollWidth - button.clientWidth;
    if (overflow > 0) {
      // Add a small buffer and padding consideration
      const offset = overflow + 16;
      const duration = offset / 30; // Speed: 30px per second

      button.style.setProperty('--scroll-offset', `-${offset}px`);
      button.style.setProperty('--scroll-duration', `${duration}s`);
      button.classList.add('is-scrolling');
    }
  };

  const handleMouseLeave = (e: React.MouseEvent<HTMLButtonElement>) => {
    const button = e.currentTarget;
    button.classList.remove('is-scrolling');
    // Keep the property for the return transition
  };

  return (
    <div className="flow-imports">
      <h3 className="flow-imports__title">{t('imports.title')}</h3>
      <ul className="flow-imports__list">
        {imports.map((item) => (
          <li key={item.id}>
            <button
              type="button"
              className="flow-imports__item"
              title={item.name}
              onClick={() => handleClick(item.firstTaskId)}
              disabled={!item.firstTaskId}
              onMouseEnter={handleMouseEnter}
              onMouseLeave={handleMouseLeave}
            >
              <span>{item.name}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

export default FlowImportsTree;
