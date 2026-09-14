#!/usr/bin/env python3
"""Proxy remote MapLibre styles/tiles for Android emulators without Internet.

The emulator reaches the host as 10.0.2.2. This process fetches styles and
tiles on the host and rewrites absolute HTTPS URLs so subsequent MapLibre
requests stay on the proxy.

  python tools/map_style_proxy.py
  # EXPO_PUBLIC_MAP_STYLE_URL=http://10.0.2.2:8090/style.json
"""

from __future__ import annotations

import json
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.parse import quote, unquote, urlsplit
from urllib.request import Request, urlopen

LISTEN_HOST = "0.0.0.0"
LISTEN_PORT = 8090
PUBLIC_BASE = f"http://10.0.2.2:{LISTEN_PORT}"
UA = "ParkXchangeMapProxy/0.1 (+https://github.com/marco/parkxchange)"

# Prefer a street style so city zoom is useful; demotiles only go to z6.
DEFAULT_STYLE = "https://tiles.openfreemap.org/styles/liberty"

# Hosts we are willing to fetch on behalf of the emulator.
ALLOWED_HOSTS = {
    "demotiles.maplibre.org",
    "tiles.openfreemap.org",
    "assets.openfreemap.org",
}

HTTPS_HOST_RE = re.compile(
    r"https://(" + "|".join(re.escape(h) for h in sorted(ALLOWED_HOSTS)) + r")"
)


def encode_path(path: str) -> str:
    parts = urlsplit(path)
    segments = [quote(unquote(seg), safe="") for seg in parts.path.split("/")]
    encoded = "/".join(segments)
    if parts.query:
        return f"{encoded}?{parts.query}"
    return encoded


def fetch_url(url: str) -> tuple[bytes, str]:
    host = urlsplit(url).hostname or ""
    if host not in ALLOWED_HOSTS:
        raise PermissionError(f"host not allowed: {host}")
    req = Request(
        url,
        headers={"User-Agent": UA, "Accept": "*/*", "Accept-Encoding": "identity"},
    )
    with urlopen(req, timeout=60) as resp:
        ctype = resp.headers.get("Content-Type", "application/octet-stream")
        return resp.read(), ctype


def rewrite(body: bytes) -> bytes:
    text = body.decode("utf-8")
    return HTTPS_HOST_RE.sub(lambda m: f"{PUBLIC_BASE}/u/{m.group(1)}", text).encode(
        "utf-8"
    )


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _send(self, code: int, body: bytes, ctype: str) -> None:
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Cache-Control", "public, max-age=300")
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.close_connection = True

    def do_GET(self) -> None:  # noqa: N802
        try:
            if self.path in ("/", "/style.json"):
                body, ctype = fetch_url(DEFAULT_STYLE)
                if "json" in ctype or True:
                    body = rewrite(body)
                    ctype = "application/json"
                self._send(200, body, ctype)
                return
            if self.path.startswith("/u/"):
                rest = self.path[len("/u/") :]
                host, _, path = rest.partition("/")
                if host not in ALLOWED_HOSTS:
                    self._send(403, b"host not allowed", "text/plain")
                    return
                url = f"https://{host}/{encode_path('/' + path).lstrip('/')}"
                # encode_path expects a path; rebuild carefully
                raw_path = "/" + path
                url = f"https://{host}{encode_path(raw_path)}"
                body, ctype = fetch_url(url)
                if "json" in ctype or raw_path.endswith(".json"):
                    body = rewrite(body)
                    ctype = "application/json"
                self._send(200, body, ctype)
                return
            # Back-compat with the demotiles-only path shape used in Phase 9.
            if self.path.startswith("/upstream/"):
                upstream_path = "/" + self.path[len("/upstream/") :]
                body, ctype = fetch_url(f"https://demotiles.maplibre.org{encode_path(upstream_path)}")
                if "json" in ctype or upstream_path.endswith(".json"):
                    body = rewrite(body)
                    ctype = "application/json"
                self._send(200, body, ctype)
                return
            self._send(404, b"not found", "text/plain")
        except HTTPError as exc:
            self._send(exc.code, str(exc.reason).encode(), "text/plain")
        except (URLError, PermissionError) as exc:
            self._send(502, str(exc).encode(), "text/plain")
        except Exception as exc:  # noqa: BLE001
            self._send(500, str(exc).encode(), "text/plain")


def main() -> None:
    body, _ = fetch_url(DEFAULT_STYLE)
    style = json.loads(rewrite(body))
    assert "sources" in style, "upstream style missing sources"
    httpd = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), Handler)
    print(
        f"map style proxy on http://127.0.0.1:{LISTEN_PORT}/style.json "
        f"(emulator: {PUBLIC_BASE}/style.json) -> {DEFAULT_STYLE}",
        flush=True,
    )
    httpd.serve_forever()


if __name__ == "__main__":
    main()
