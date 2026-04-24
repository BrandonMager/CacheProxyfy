const styles: Record<string, string> = {
  CRITICAL: "bg-red-100    text-red-700    dark:bg-red-900/30    dark:text-red-300",
  HIGH:     "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300",
  MEDIUM:   "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300",
  LOW:      "bg-green-100  text-green-700  dark:bg-green-900/30  dark:text-green-300",
};

export const SeverityBadge = ({ severity }: { severity: string }) => {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${styles[severity.toUpperCase()] ?? "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"}`}>
      {severity}
    </span>
  );
};
