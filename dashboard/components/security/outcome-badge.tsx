const styles: Record<string, string> = {
  Block: "bg-red-100    text-red-700    dark:bg-red-900/30    dark:text-red-300",
  Warn:  "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300",
  Allow: "bg-green-100  text-green-700  dark:bg-green-900/30  dark:text-green-300",
};

export const OutcomeBadge = ({ outcome }: { outcome: string }) => {
  const label = outcome.charAt(0).toUpperCase() + outcome.slice(1).toLowerCase();
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
        styles[label] ?? "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"
      }`}
    >
      {label}
    </span>
  );
};
