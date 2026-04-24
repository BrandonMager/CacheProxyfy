"use client"
import React, { useState, useEffect } from "react";
import { useTheme } from "next-themes";
import { usePathname } from "next/navigation";
import {
  Home,
  Package,
  ShieldAlert,
  BarChart3,
  ChevronDown,
  ChevronsRight,
  Moon,
  Sun,
  Settings,
  HelpCircle,
} from "lucide-react";

export const SidebarLayout = ({ children, title, subtitle }: {
  children: React.ReactNode;
  title: string;
  subtitle: string;
}) => {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  const isDark = mounted && theme === "dark";

  return (
    <div className="flex min-h-screen w-full bg-gray-50 dark:bg-gray-950 text-gray-900 dark:text-gray-100">
      <Sidebar />
      <main className="flex-1 bg-gray-50 dark:bg-gray-950 p-6 overflow-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-gray-900 dark:text-gray-100">{title}</h1>
            <p className="text-gray-600 dark:text-gray-400 mt-1">{subtitle}</p>
          </div>
          <button
            onClick={() => setTheme(isDark ? "light" : "dark")}
            className="flex h-10 w-10 items-center justify-center rounded-lg border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 hover:text-gray-900 dark:hover:text-gray-100 transition-colors"
          >
            {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
        </div>
        {children}
      </main>
    </div>
  );
};

const Sidebar = () => {
  const [open, setOpen] = useState(true);
  const pathname = usePathname();

  return (
    <nav
      className={`sticky top-0 h-screen shrink-0 border-r transition-all duration-300 ease-in-out ${
        open ? "w-64" : "w-16"
      } border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-2 shadow-sm`}
    >
      <TitleSection open={open} />

      <div className="space-y-1 mb-8">
        <Option Icon={Home}        title="Overview"   pathname={pathname} open={open} href="/" />
        <Option Icon={Package}     title="Packages"   pathname={pathname} open={open} href="/packages" />
        <Option Icon={ShieldAlert} title="Security"   pathname={pathname} open={open} href="/security" />
        <Option Icon={BarChart3}   title="Metrics"    pathname={pathname} open={open} href="/metrics" />
      </div>

      {open && (
        <div className="border-t border-gray-200 dark:border-gray-800 pt-4 space-y-1">
          <div className="px-3 py-2 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            System
          </div>
          <Option Icon={Settings}   title="Settings"    pathname={pathname} open={open} href="/settings" />
          <Option Icon={HelpCircle} title="Help & Docs" pathname={pathname} open={open} href="/help" />
        </div>
      )}

      <ToggleClose open={open} setOpen={setOpen} />
    </nav>
  );
};

const Option = ({ Icon, title, pathname, open, href }: {
  Icon: React.ElementType;
  title: string;
  pathname: string;
  open: boolean;
  href: string;
}) => {
  const isSelected = href === "/" ? pathname === "/" : pathname.startsWith(href);

  return (
    <a
      href={href}
      className={`relative flex h-11 w-full items-center rounded-md transition-all duration-200 ${
        isSelected
          ? "bg-blue-50 dark:bg-blue-900/50 text-blue-700 dark:text-blue-300 shadow-sm border-l-2 border-blue-500"
          : "text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 hover:text-gray-900 dark:hover:text-gray-200"
      }`}
    >
      <div className="grid h-full w-12 place-content-center">
        <Icon className="h-4 w-4" />
      </div>
      {open && (
        <span className={`text-sm font-medium transition-opacity duration-200 ${open ? "opacity-100" : "opacity-0"}`}>
          {title}
        </span>
      )}
    </a>
  );
};

const TitleSection = ({ open }: { open: boolean }) => {
  return (
    <div className="mb-6 border-b border-gray-200 dark:border-gray-800 pb-4">
      <div className="flex cursor-pointer items-center justify-between rounded-md p-2 transition-colors hover:bg-gray-50 dark:hover:bg-gray-800">
        <div className="flex items-center gap-3">
          <Logo />
          {open && (
            <div className={`transition-opacity duration-200 ${open ? "opacity-100" : "opacity-0"}`}>
              <span className="block text-sm font-semibold text-gray-900 dark:text-gray-100">CacheProxyfy</span>
              <span className="block text-xs text-gray-500 dark:text-gray-400">localhost:9090</span>
            </div>
          )}
        </div>
        {open && <ChevronDown className="h-4 w-4 text-gray-400 dark:text-gray-500" />}
      </div>
    </div>
  );
};

const Logo = () => {
  return (
    <div className="grid size-10 shrink-0 place-content-center rounded-lg bg-gradient-to-br from-blue-500 to-blue-600 shadow-sm">
      <svg width="20" height="auto" viewBox="0 0 50 39" fill="none" xmlns="http://www.w3.org/2000/svg" className="fill-white">
        <path d="M16.4992 2H37.5808L22.0816 24.9729H1L16.4992 2Z" />
        <path d="M17.4224 27.102L11.4192 36H33.5008L49 13.0271H32.7024L23.2064 27.102H17.4224Z" />
      </svg>
    </div>
  );
};

const ToggleClose = ({ open, setOpen }: { open: boolean; setOpen: (v: boolean) => void }) => {
  return (
    <button
      onClick={() => setOpen(!open)}
      className="absolute bottom-0 left-0 right-0 border-t border-gray-200 dark:border-gray-800 transition-colors hover:bg-gray-50 dark:hover:bg-gray-800"
    >
      <div className="flex items-center p-3">
        <div className="grid size-10 place-content-center">
          <ChevronsRight
            className={`h-4 w-4 transition-transform duration-300 text-gray-500 dark:text-gray-400 ${open ? "rotate-180" : ""}`}
          />
        </div>
        {open && (
          <span className={`text-sm font-medium text-gray-600 dark:text-gray-300 transition-opacity duration-200 ${open ? "opacity-100" : "opacity-0"}`}>
            Hide
          </span>
        )}
      </div>
    </button>
  );
};
