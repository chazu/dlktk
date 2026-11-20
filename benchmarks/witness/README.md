# Witness reasoning pilot

The pilot uses ten shuffled puzzles from Steging, Renooij, and Verheij. The manifest contains the published row IDs and answer keys, plus the attack edges extracted from each puzzle. Keep this file outside agent sessions. [Design](DESIGN.md) · [Published dataset](https://github.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks/tree/a10dae138180584c9fb686ff11ff62d0decac6f1/static_benchmarks)

From this directory, run `python3 corpus.py` to download and verify the pinned CSV. To work offline, pass `--csv PATH` with a copy of `static_non_linear_5_15_shuffle_True.csv`. Both paths verify the same SHA-256 digest. The check reconstructs each chain from the shuffled prose and checks its length and the supplied verdict.
