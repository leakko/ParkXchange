# Infrastructure

Deployment is **out of scope for the MVP**. This directory is a placeholder that
records the target topology so the application code never drifts away from it.

## Target topology

- The API image built by [`services/api/Dockerfile`](../../services/api/Dockerfile)
  runs on **AWS EKS with Fargate** profiles.
- **Pulumi (TypeScript)** describes the stack. It will live here as a workspace
  package so it shares the repo's TypeScript tooling.
- **Amazon RDS for PostgreSQL** with the PostGIS extension replaces the local
  compose database.
- **PMTiles basemaps are served from S3** (optionally behind CloudFront).
  MapLibre Native reads them directly over HTTP range requests with the
  `pmtiles://https://...` URL scheme, so no tile server is needed.

## What the MVP already guarantees

The application is written so that none of the above requires a refactor:

- Every setting is read from the environment, never from a checked-in file.
- The API self-migrates on boot, so a Fargate task needs no init container.
- The map style is injected through `EXPO_PUBLIC_MAP_STYLE_URL`, so pointing the
  app at an S3-hosted PMTiles style is a configuration change.
- Real-time fan-out uses PostgreSQL `LISTEN/NOTIFY`, which works unchanged
  across multiple replicas.
