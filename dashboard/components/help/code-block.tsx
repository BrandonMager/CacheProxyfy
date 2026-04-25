export const CodeBlock = ({ children, label }: { children: string; label?: string }) => (
  <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
    {label && (
      <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800">
        <span className="text-xs font-medium text-gray-500 dark:text-gray-400">{label}</span>
      </div>
    )}
    <pre className="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 overflow-x-auto">
      <code className="text-sm font-mono text-gray-800 dark:text-gray-200 whitespace-pre">
        {children}
      </code>
    </pre>
  </div>
);
