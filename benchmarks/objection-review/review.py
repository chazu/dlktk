"""Serve the local, blind objection review."""
import argparse
import json
import os
import secrets
from html import escape
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from string import Template
from tempfile import NamedTemporaryFile
from urllib.parse import parse_qs, urlsplit

HERE = Path(__file__).resolve().parent
LABELS = {"addresses": "Addresses the objection", "persists": "Leaves the problem in place",
          "unclear": "Too little information"}
FAMILIES = {"ownership": "Ownership", "gate": "Merge gates", "contract": "Interface contracts",
            "volume": "Review volume", "practice": "Practice time"}


def read_map(name):
    path = HERE / name
    return json.loads(path.read_text()) if path.exists() else {}


def write_map(name, values):
    with NamedTemporaryFile("w", dir=HERE, delete=False) as out:
        try:
            json.dump(values, out, indent=2, ensure_ascii=False)
            out.write("\n")
            out.flush()
            os.fsync(out.fileno())
            os.replace(out.name, HERE / name)
        finally:
            Path(out.name).unlink(missing_ok=True)


class Review(BaseHTTPRequestHandler):
    def local_host(self):
        return self.headers.get("Host") == f"127.0.0.1:{self.server.server_port}"

    def do_GET(self):
        if not self.local_host() or urlsplit(self.path).path != "/":
            return self.send_error(404)
        cases = self.server.cases
        labels = read_map("reviewed.json")
        notes = read_map("review-notes.json")
        count = sum(key in labels for key in cases)
        requested = parse_qs(urlsplit(self.path).query).get("case", [None])[0]
        current = requested or next((key for key in cases if key not in labels), None)
        if current and current not in cases:
            return self.send_error(404)
        groups = []
        for family, title in FAMILIES.items():
            links = []
            for key, case in cases.items():
                if case["family"] == family:
                    active = ' aria-current="page"' if key == current else ""
                    mark = "✓" if key in labels else "○"
                    links.append(f'<a href="/?case={key}"{active}>{key}<span>{mark}</span></a>')
            groups.append(f'<h3>{title}</h3>' + "".join(links))
        if current:
            case = cases[current]
            state = case["state"]
            sections = "".join(
                f'<section><h2>{title}</h2><p>{escape(state[field])}</p></section>'
                for field, title in [("parent", "Original proposal"), ("objection", "Objection"),
                                     ("hybrid", "Hybrid proposal"), ("reply", "Reply to assess")]
            )
            options = "".join(
                f'<label><input type="radio" name="label" required value="{key}"'
                + (" checked" if labels.get(current) == key else "") + f'> {title}</label>'
                for key, title in LABELS.items()
            )
            content = f'''<small>{current}</small><h1>{FAMILIES[case["family"]]}</h1>
            {sections}<form action="/save" method="post">
            <input type="hidden" name="token" value="{self.server.token}">
            <input type="hidden" name="case" value="{current}">
            <fieldset><legend>Does the reply address the full objection?</legend>{options}</fieldset>
            <label for="note">Reason <em>(optional)</em></label>
            <textarea id="note" name="note" rows="2">{escape(notes.get(current, ""))}</textarea>
            <button>Save and continue</button></form>'''
        else:
            content = "<h1>Review complete</h1><p>All judgments are saved. Select any case to revisit it.</p>"
        html = self.server.template.substitute(content=content, nav="".join(groups),
                                                count=count, total=len(cases)).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("Content-Length", str(len(html)))
        self.end_headers()
        self.wfile.write(html)

    def do_POST(self):
        if not self.local_host() or self.path != "/save":
            return self.send_error(404)
        try:
            size = int(self.headers.get("Content-Length", 0))
            if not 0 < size <= 64 * 1024:
                return self.send_error(413)
            values = parse_qs(self.rfile.read(size).decode(), keep_blank_values=True)
            key, label, note, token = (values[k][0] for k in ("case", "label", "note", "token"))
        except (KeyError, ValueError, UnicodeError):
            return self.send_error(400)
        if not secrets.compare_digest(token.encode(), self.server.token.encode()):
            return self.send_error(403)
        if key not in self.server.cases or label not in LABELS:
            return self.send_error(400)
        labels, notes = read_map("reviewed.json"), read_map("review-notes.json")
        labels[key] = label
        if note.strip():
            notes[key] = note.strip()
        else:
            notes.pop(key, None)
        try:
            write_map("review-notes.json", notes)
            write_map("reviewed.json", labels)
        except OSError:
            return self.send_error(500, "Save failed. Go back and retry.")
        self.send_response(303)
        self.send_header("Location", "/")
        self.end_headers()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=8741)
    args = parser.parse_args()
    with HTTPServer(("127.0.0.1", args.port), Review) as server:
        cases = json.loads((HERE / "cases.json").read_text())
        cases.sort(key=lambda c: (list(FAMILIES).index(c["family"]), int(c["id"].split("-")[-1])))
        server.cases = {c["id"]: c for c in cases}
        server.token = secrets.token_urlsafe(32)
        server.template = Template((HERE / "review.html").read_text())
        print(f"http://127.0.0.1:{server.server_port}/", flush=True)
        server.serve_forever()
