# Capacity notes — single Hetzner VPS

Rough sizing for the current production shape: **one CX23-class box**
(2 vCPU, 4 GB RAM, 40 GB local disk) running **PostGIS + Go API + Caddy**
(+ Dozzle on loopback). Not a load-test result; revisit when Graphs or
symptoms contradict this.

Last reviewed: 2026-09-22 (CX23 `parkxchange`, Nuremberg).

## What “active” means here

ParkXchange is session-ish, not an always-open chat app. Most registered
users are idle most of the day. The scarce resource is **people with the map
open** (authenticated WebSocket + occasional REST), not total accounts or
DAU.

Rule of thumb: **~200 concurrent map sessions** usually implies **many more
registered users** (thousands+ installs / accounts, depending on city and
hour). Concurrent ≪ registered ≪ install base.

## Comfortable range

| Signal | Estimate |
| --- | --- |
| Concurrent map (WS) open | **~50–150** — fine |
| Registered accounts / light DAU | Hundreds to low thousands / ~50–200 DAU — usually fine |
| Early city / friends-and-family beta | Well inside the box |

## Where service starts to degrade

| Concurrent map open | Likely symptoms |
| --- | --- |
| **~200–350** | Slower claim/announce, pool wait, RAM often high; WS fan-out cost rises if many events share one dense bbox |
| **~400–600** | Timeouts, swap/OOM risk; login bursts can spike RAM hard |
| Dense bbox + high event rate | CPU on the hub can hurt **before** disk or traffic caps |

Traffic (20 TB) and the 40 GB disk are not the early bottlenecks.

## What fails first on this design

Ordered by likelihood on a 4 GB single node:

1. **RAM** — API and Postgres compete; Docker + OS leave little headroom.
2. **CPU on WebSocket fan-out** — hub matching is **O(connections)** per spot
   event (`ARCHITECTURE.md` §3.4). Same-city rush hour with lots of churn
   stresses this.
3. **pgx pool (max 20)** — request bursts queue for DB connections; total
   user count does not by itself.
4. **argon2id (~64 MiB per password verify)** — concurrent login storms (e.g.
   after an outage) can pressure memory; rate limits help but do not remove
   the cost.

Guarantees that stay fine longer: PostGIS bbox + GiST, conditional claim
`UPDATE`, partial unique indexes. Those scale with query shape and data
size, not with “number of accounts” alone.

## When to grow the box

Upsize (e.g. **~8 GB / more vCPU**) or **split Postgres off the API host**
when you routinely see:

- **~200+** map-open clients in peak hour, or
- sustained high RAM/CPU on Hetzner **Graphs**, or
- growing latency / timeouts on claim, search, or WS.

Architectural upgrades (tile-indexed subscriptions, external bus) are
documented in `ARCHITECTURE.md` §3.4; they matter after the box is no longer
the cheap fix.

## How to validate later

- Hetzner Console → server → **Graphs** (CPU, RAM, disk, network).
- Dozzle / API logs over SSH tunnel (`README.md`).
- Optional: scripted N WebSockets + concurrent claims against staging/prod
  and watch Graphs + `ops-queries.sql`.

Update this file when hardware, compose layout, or measured limits change.
