import { Activity } from "lucide-react";

type Color = "blue" | "green" | "purple" | "orange" | "red";

const iconBg: Record<Color, string> = {
  blue:   "bg-blue-50   dark:bg-blue-900/20",
  green:  "bg-green-50  dark:bg-green-900/20",
  purple: "bg-purple-50 dark:bg-purple-900/20",
  orange: "bg-orange-50 dark:bg-orange-900/20",
  red:    "bg-red-50    dark:bg-red-900/20",
};

const iconColor: Record<Color, string> = {
  blue:   "text-blue-600   dark:text-blue-400",
  green:  "text-green-600  dark:text-green-400",
  purple: "text-purple-600 dark:text-purple-400",
  orange: "text-orange-600 dark:text-orange-400",
  red:    "text-red-600    dark:text-red-400",
};

export const StatCard = ({ icon: Icon, label, value, sub, color }: {
  icon: React.ElementType;
  label: string;
  value: string;
  sub?: string;
  color: Color;
}) => {
  return (
    <div className="p-6 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm hover:shadow-md transition-shadow">
      <div className="flex items-center justify-between mb-4">
        <div className={`p-2 rounded-lg ${iconBg[color]}`}>
          <Icon className={`h-5 w-5 ${iconColor[color]}`} />
        </div>
        <Activity className="h-4 w-4 text-gray-400 dark:text-gray-500" />
      </div>
      <h3 className="font-medium text-gray-600 dark:text-gray-400 mb-1">{label}</h3>
      <p className="text-2xl font-bold text-gray-900 dark:text-gray-100">{value}</p>
      {sub && <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{sub}</p>}
    </div>
  );
};
