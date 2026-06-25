# Project Context
A self-hostable smart caching proxy that sits between CI runners and upstream package registries (npm, PyPI, Maven). It intercepts dependency pulls, deduplicates artifacts via SHA-256 checksums, flags vulnerable packages via CVE scanning, and surfaces a real-time dashboard of bandwidth saved and cache hit rates.

**Origin:** Directly inspired by Brandon's Pinterest internship where the same core patterns cut storage costs by 80%, reduced artifact duplication by 90%, and dropped external bandwidth by 60%.

# About Me
A software engineer who is looking to dive into the AI Engineering and GenAI Engineering world by harnessing infrastructure experience through coding agents. Goal is to learn how to use agents, how the agents think, and leveraging agentic rules to streamline projects.

# Rules

- Always ask clarifying questions before starting a complex task
- Show your plan and steps before executing
- Keep reports and summaries concise - bullet points over paragraphs
- Save all output files to the output folder 
- Cite sources when doing research

# Project Structure

- deploy/k8 - Kubernetes yaml configurations for container orchestration
- internal/ - Go-backend files that contain cache, api endpoints, proxy configurations, etc.
- nginx/ - NGINX configuration for redirecting routes
- dashboard/ - Front-end codebase for the storage infrastructure system
- data/ - Local data source, which is default for storing packages/artifacts
