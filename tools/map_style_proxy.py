#!/usr/bin/env python3
"""Proxy MapLibre demotiles so the Android emulator can load them.

The emulator on this Windows host reaches the development machine via
10.0.2.2 but has no outbound Internet. MapLibre styles and vector tiles
are therefore fetched by this process on the host and served over cleartext
HTTP, which debug builds already allow.

  python tools/map_style_proxy.py
  # then EXPO_PUBLIC_MAP_STYLE_URL=http://10.0.2.2:8090/style.json
"""

from __future__ import annotations

import json
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.parse import quote, unquote, urlsplit
from urllib.request import Request, urlopen

UPSTREAM = "https://demotiles.maplibre.org"
LISTEN_HOST = "0.0.0.0"
LISTEN_PORT = 8090
PUBLIC_BASE = f"http://10.0.2.2:{LISTEN_PORT}"
UA = "ParkXchangeMapProxy/0.1 (+https://github.com/marco/parkxchange)"

HTTPS_RE = re.compile(r"https://demotiles\.maplibre\.org")


def upstream_url(path: str) -> str:
    """Re-quote path segments so font names with spaces survive urllib."""
    parts = urlsplit(path)
    segments = [quote(unquote(seg), safe="") for seg in parts.path.split("/")]
    encoded_path = "/".join(segments)
    if parts.query:
        return f"{UPSTREAM}{encoded_path}?{parts.query}"
    return f"{UPSTREAM}{encoded_path}"


def fetch(path: str) -> tuple[bytes, str]:
    req = Request(
        upstream_url(path),
        headers={"User-Agent": UA, "Accept": "*/*", "Accept-Encoding": "identity"},
    )
    with urlopen(req, timeout=30) as resp:
        ctype = resp.headers.get("Content-Type", "application/octet-stream")
        return resp.read(), ctype


def rewrite_bytes(body: bytes) -> bytes:
    return HTTPS_RE.sub(f"{PUBLIC_BASE}/upstream", body.decode("utf-8")).encode("utf-8")


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
        # MapLibre Native on the emulator mis-reads keep-alive responses from
        # this proxy as truncated ("unexpected end of stream").
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.close_connection = True

    def do_GET(self) -> None:  # noqa: N802
        try:
            if self.path in ("/", "/style.json"):
                body, _ = fetch("/style.json")
                self._send(200, rewrite_bytes(body), "application/json")
                return
            if self.path.startswith("/upstream/"):
                upstream_path = "/" + self.path[len("/upstream/") :]
                body, ctype = fetch(upstream_path)
                if "json" in ctype or upstream_path.endswith(".json"):
                    body = rewrite_bytes(body)
                    ctype = "application/json"
                self._send(200, body, ctype)
                return
            self._send(404, b"not found", "text/plain")
        except HTTPError as exc:
            self._send(exc.code, str(exc.reason).encode(), "text/plain")
        except URLError as exc:
            self._send(502, str(exc.reason).encode(), "text/plain")
        except Exception as exc:  # noqa: BLE001 — surface any proxy failure
            self._send(500, str(exc).encode(), "text/plain")


def main() -> None:
    body, _ = fetch("/style.json")
    style = json.loads(rewrite_bytes(body))
    assert "sources" in style, "upstream style.json missing sources"
    httpd = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), Handler)
    print(
        f"demotiles proxy on http://127.0.0.1:{LISTEN_PORT}/style.json "
        f"(emulator: {PUBLIC_BASE}/style.json)",
        flush=True,
    )
    httpd.serve_forever()


if __name__ == "__main__":
    main()
