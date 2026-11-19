"""Restore every saved discussion with the CI binary and compare its public views."""
import json
from pathlib import Path
import subprocess
from tempfile import TemporaryDirectory

ROOT = Path(__file__).resolve().parents[2]
EXPECTED = Path(__file__).with_name("example-outcomes.json")


def outcome(fixture, binary):
    with TemporaryDirectory() as directory:
        def read(*args):
            result = subprocess.run(
                [binary, *args, "--store", directory, "--format", "json"],
                cwd=directory, check=True, stdout=subprocess.PIPE, text=True,
            )
            return json.loads(result.stdout)

        read("import", fixture)
        # Imported transaction times start today. Inspect after all fixture review dates.
        findings = read("check", "--all", "--as-of", "4102444800")
        assert findings["discussions"] == 1 and findings["ok"]
        discussion = next(row["args"]["id"] for row in map(json.loads, fixture.read_text().splitlines())
                          if row["relation"] == "dlktk/discussion")
        decisions = []
        for issue in read("status", "-d", discussion):
            for decision in issue.get("decisions", []):
                decisions.append([issue["issue"], decision["position"]])
            if issue.get("map_decided"):
                decisions.append([issue["issue"], "map"])
        return {
            "decisions": sorted(decisions),
            "findings": sorted([f["kind"], f["issue"], f.get("node", "")] for f in findings["findings"] or []),
        }


if __name__ == "__main__":
    import sys
    binary = Path(sys.argv[1]).resolve(strict=True)
    expected = json.loads(EXPECTED.read_text())
    tracked = subprocess.check_output(["git", "ls-files", "-z", "--", "examples/*.ndjson"], cwd=ROOT)
    fixtures = [ROOT / name.decode() for name in tracked.split(b"\0") if name]
    names = {str(path.relative_to(ROOT)) for path in fixtures}
    assert names == set(expected), f"Fixture manifest differs: {names ^ set(expected)}"
    for path in fixtures:
        name = str(path.relative_to(ROOT))
        print(name, flush=True)
        actual = outcome(path, binary)
        assert actual == expected[name], f"{name}: expected {expected[name]}, got {actual}"
