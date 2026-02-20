import { useState, useEffect } from 'react';
import ReactJson from '@microlink/react-json-view';

interface JsonViewerProps {
    data: Record<string, unknown> | unknown[];
    defaultInspectDepth?: number;
    editable?: boolean;
    rootName?: string | false;
}

/**
 * Wrapper component for react-json-view with custom configuration.
 * Provides an interactive JSON viewer with expand/collapse functionality.
 */
function JsonViewer({ data, defaultInspectDepth = 1, editable = false, rootName = 'root' }: JsonViewerProps) {
    // collapsed: true (all collapsed), false (all expanded), or number (depth)
    const [collapsed, setCollapsed] = useState<boolean | number>(defaultInspectDepth);
    const [key, setKey] = useState(0);

    // Update state when defaultInspectDepth changes
    useEffect(() => {
        setCollapsed(defaultInspectDepth);
    }, [defaultInspectDepth]);

    const handleExpandAll = () => {
        setCollapsed(false);
        setKey(prev => prev + 1);
    };

    const handleCollapseAll = () => {
        setCollapsed(true);
        setKey(prev => prev + 1);
    };

    return (
        <div className="json-viewer-wrapper">
            <div className="json-viewer-controls">
                <button
                    onClick={handleExpandAll}
                    className="json-viewer-btn"
                    title="Expand All"
                    aria-label="Expand All"
                >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="7 13 12 18 17 13"></polyline>
                        <polyline points="7 6 12 11 17 6"></polyline>
                    </svg>
                </button>
                <button
                    onClick={handleCollapseAll}
                    className="json-viewer-btn"
                    title="Collapse All"
                    aria-label="Collapse All"
                >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="17 11 12 6 7 11"></polyline>
                        <polyline points="17 18 12 13 7 18"></polyline>
                    </svg>
                </button>
            </div>
            <div className="json-viewer-container">
                <ReactJson
                    key={key}
                    src={data}
                    collapsed={collapsed}
                    name={rootName === 'root' ? null : rootName}
                    enableClipboard={true}
                    displayDataTypes={false}
                    displayObjectSize={true}
                    indentWidth={2}
                    iconStyle="triangle"
                    theme="rjv-default"
                    style={{
                        fontFamily: 'var(--font-mono, "Courier New", monospace)',
                        fontSize: '13px',
                        backgroundColor: 'var(--color-bg-code, #f8f9fa)',
                        padding: '12px',
                        borderRadius: '4px',
                    }}
                    onEdit={editable ? (edit) => { console.log(edit); return true; } : false}
                    onAdd={editable ? (add) => { console.log(add); return true; } : false}
                    onDelete={editable ? (del) => { console.log(del); return true; } : false}
                />
            </div>
        </div>
    );
}

export default JsonViewer;
