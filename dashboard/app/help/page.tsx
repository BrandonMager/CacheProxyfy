import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { HelpSection } from "@/components/help/help-section";
import { CodeBlock } from "@/components/help/code-block";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { SeverityBadge } from "@/components/ui/severity-badge";

const P = ({ children }: { children: React.ReactNode }) => (
  <p className="text-sm text-gray-700 dark:text-gray-300 leading-relaxed">{children}</p>
);

const Label = ({ children }: { children: React.ReactNode }) => (
  <p className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 mb-2">
    {children}
  </p>
);

const SEVERITY_ROWS = [
  { severity: "CRITICAL", description: "Remote code execution, authentication bypass, or data exposure at scale" },
  { severity: "HIGH",     description: "Significant impact with a known exploit or easy attack vector" },
  { severity: "MEDIUM",   description: "Moderate impact, typically requiring specific conditions" },
  { severity: "LOW",      description: "Limited impact, informational, or difficult to exploit" },
];

const OUTCOME_ROWS = [
  { outcome: "Block", color: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300",       description: "Request is rejected — the package is not served to the client" },
  { outcome: "Warn",  color: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300", description: "Request proceeds but the alert is recorded in the Security page" },
  { outcome: "Allow", color: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300",   description: "No CVEs found at or above the configured thresholds" },
];

const DASHBOARD_PAGES = [
  { name: "Overview",  href: "/",         description: "Top-level stats (hit rate, bandwidth saved, CVE alerts) and the 5 most recently cached packages." },
  { name: "Packages",  href: "/packages", description: "Full list of cached packages, filterable by ecosystem. Shows name, version, size, cache date, and last hit." },
  { name: "Security",  href: "/security", description: "All CVE alerts recorded by the proxy, filterable by severity. Shows the CVE ID, affected package, and policy outcome." },
  { name: "Metrics",   href: "/metrics",  description: "Aggregate cache performance stats for a selectable time window (24h / 7d / 30d) with a hit/miss breakdown bar." },
  { name: "Settings",  href: "/settings", description: "Read-only view of the active cacheproxyfy.yaml configuration. Secrets are never exposed." },
];

export default function HelpPage() {
  return (
    <SidebarLayout title="Help & Docs" subtitle="How CacheProxyfy works and how to configure your clients">
      <div className="space-y-6">

        {/* Overview */}
        <HelpSection title="What is CacheProxyfy?">
          <P>
            CacheProxyfy is a caching proxy for package registries. When a client requests a package,
            the proxy checks its local cache first. On a hit the artifact is served immediately — no
            upstream network round-trip. On a miss the proxy fetches from the upstream registry, stores
            the artifact, and streams it to the client.
          </P>
          <P>
            All traffic passes through a single endpoint. Clients are pointed at the proxy instead of
            the upstream registry; the proxy URL is a drop-in replacement.
          </P>
        </HelpSection>

        {/* Client configuration */}
        <HelpSection
          title="Configuring Your Client"
          description="Point each package manager at the proxy. Replace localhost:8080 with your actual proxy address."
        >
          <div className="space-y-5">
            <div>
              <Label><EcosystemBadge ecosystem="npm" /></Label>
              <div className="space-y-2">
                <CodeBlock label="Set registry globally">
                  {`npm config set registry http://localhost:8080/npm/`}
                </CodeBlock>
                <CodeBlock label="Or per-project (.npmrc)">
                  {`registry=http://localhost:8080/npm/`}
                </CodeBlock>
              </div>
            </div>

            <div>
              <Label><EcosystemBadge ecosystem="pypi" /></Label>
              <div className="space-y-2">
                <CodeBlock label="Install a package via proxy">
                  {`pip install --index-url http://localhost:8080/pypi/simple/ <package>`}
                </CodeBlock>
                <CodeBlock label="Or set globally (pip.ini / ~/.config/pip/pip.conf)">
                  {`[global]\nindex-url = http://localhost:8080/pypi/simple/`}
                </CodeBlock>
              </div>
            </div>

            <div>
              <Label><EcosystemBadge ecosystem="maven" /></Label>
              <CodeBlock label="~/.m2/settings.xml — add a mirror">
                {`<settings>\n  <mirrors>\n    <mirror>\n      <id>cacheproxyfy</id>\n      <mirrorOf>central</mirrorOf>\n      <url>http://localhost:8080/maven/</url>\n    </mirror>\n  </mirrors>\n</settings>`}
              </CodeBlock>
            </div>
          </div>
        </HelpSection>

        {/* How caching works */}
        <HelpSection title="How Caching Works">
          <P>
            Each incoming request is matched against the active ecosystems. If a matching artifact is
            found in the cache the proxy serves it directly and records a cache hit. If not, the proxy
            fetches from the upstream registry, writes the artifact to the configured storage backend,
            and records a cache miss.
          </P>
          <P>
            Artifacts are retained until the TTL expires (configured via <code className="text-xs font-mono bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded">cache.ttl_hours</code>).
            The default is 720 hours (30 days). Storage can be a local directory or an S3-compatible
            bucket — see the Settings page for the active configuration.
          </P>
        </HelpSection>

        {/* CVE scanning */}
        <HelpSection
          title="CVE Scanning"
          description="How vulnerabilities are detected and acted on"
        >
          <P>
            When CVE scanning is enabled, the proxy queries the{" "}
            <span className="font-mono text-xs bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded">OSV API</span>{" "}
            (osv.dev) for each package before serving it. If vulnerabilities are found, the configured
            severity thresholds determine the outcome.
          </P>

          <div>
            <Label>Severity levels</Label>
            <div className="rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden">
              <div className="grid grid-cols-[100px_1fr] gap-4 px-4 py-2 bg-gray-50 dark:bg-gray-800/50 border-b border-gray-100 dark:border-gray-800">
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Severity</span>
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Meaning</span>
              </div>
              <div className="divide-y divide-gray-100 dark:divide-gray-800">
                {SEVERITY_ROWS.map(({ severity, description }) => (
                  <div key={severity} className="grid grid-cols-[100px_1fr] gap-4 items-center px-4 py-3">
                    <SeverityBadge severity={severity} />
                    <span className="text-sm text-gray-700 dark:text-gray-300">{description}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div>
            <Label>Policy outcomes</Label>
            <div className="rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden">
              <div className="grid grid-cols-[80px_1fr] gap-4 px-4 py-2 bg-gray-50 dark:bg-gray-800/50 border-b border-gray-100 dark:border-gray-800">
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Outcome</span>
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Behavior</span>
              </div>
              <div className="divide-y divide-gray-100 dark:divide-gray-800">
                {OUTCOME_ROWS.map(({ outcome, color, description }) => (
                  <div key={outcome} className="grid grid-cols-[80px_1fr] gap-4 items-center px-4 py-3">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium w-fit ${color}`}>
                      {outcome}
                    </span>
                    <span className="text-sm text-gray-700 dark:text-gray-300">{description}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <P>
            Thresholds are set via <code className="text-xs font-mono bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded">security.block_severity</code> and{" "}
            <code className="text-xs font-mono bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded">security.warn_severity</code> in{" "}
            <code className="text-xs font-mono bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded">cacheproxyfy.yaml</code>.
            All alerts are visible on the Security page.
          </P>
        </HelpSection>

        {/* Dashboard pages */}
        <HelpSection title="Dashboard Pages">
          <div className="rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden">
            <div className="grid grid-cols-[110px_1fr] gap-4 px-4 py-2 bg-gray-50 dark:bg-gray-800/50 border-b border-gray-100 dark:border-gray-800">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Page</span>
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Contents</span>
            </div>
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {DASHBOARD_PAGES.map(({ name, href, description }) => (
                <div key={href} className="grid grid-cols-[110px_1fr] gap-4 items-start px-4 py-3">
                  <a
                    href={href}
                    className="text-sm font-medium text-blue-600 dark:text-blue-400 hover:underline"
                  >
                    {name}
                  </a>
                  <span className="text-sm text-gray-700 dark:text-gray-300">{description}</span>
                </div>
              ))}
            </div>
          </div>
        </HelpSection>

      </div>
    </SidebarLayout>
  );
}
