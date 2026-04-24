const styles: Record<string, string> = {
  npm:   "bg-red-100    text-red-700    dark:bg-red-900/30    dark:text-red-300",
  pypi:  "bg-blue-100   text-blue-700   dark:bg-blue-900/30   dark:text-blue-300",
  maven: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300",
};

export const EcosystemBadge = ({ ecosystem }: { ecosystem: string }) => {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${styles[ecosystem] ?? "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"}`}>
      {ecosystem}
    </span>
  );
};
