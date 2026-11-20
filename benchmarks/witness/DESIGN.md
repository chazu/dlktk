# Does dlktk help an agent solve witness puzzles?

Ten paired cases test whether the agent follows a chain of objections to the correct conclusion. This pilot measures reasoning on testimony. It makes no claim about creative design work.

## Cases and answer key

Use the shuffled testimony CSV published by Cor Steging, Silja Renooij, and Bart Verheij. Pin the upstream commit and CSV digest. Freeze the case IDs before inference. For each shape below, take the first matching row in file order. Each number is the length of one chain attacking the main witness. A main claim survives exactly when every chain has even length. Check this independent rule against the supplied answer. [Paper](https://arxiv.org/abs/2505.01539) · [Dataset repository](https://github.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks)

| Expected yes | Expected no |
|---|---|
| 4 | 3 |
| 8 | 9 |
| 2, 2 | 1, 2 |
| 2, 6 | 2, 5 |
| 2, 4, 6 | 2, 3, 6 |

This balances verdicts and includes both chains and branching attacks. The public puzzles could have appeared in model training.

## Two fresh sessions per puzzle

Use the same OpenRouter chat model (`openai/gpt-4.1-mini`), temperature zero, puzzle text, and 2,048 total output-token allowance per session. Start with an empty message history each time. Alternate which arm runs first. Neither arm receives the row ID, shape, gold answer, other session, or corpus access.

- **Plain:** reason from the text and answer yes or no. Give one sentence identifying the decisive testimony.
- **With dlktk:** the agent identifies the main witness and submits its own attacker-to-target pairs through a batch tool. The adapter creates a fresh temporary dlktk store. It proposes the main witness's claim, records each attack with `object`, and reads the grounded label with `show`. The agent sees that label and gives its final answer under the same response instructions. The adapter only translates supplied edges. It receives no answer key.

Allow one tool submission and one final answer in the assisted arm. Record malformed submissions, tool failures, and invalid final answers. Do not repair an answer or rerun a reasoning error. Keep transport failures distinct from incorrect verdicts. Record the model returned by the provider, usage, and the agent's graph so the run can be audited.

## Score and stopping point

Score the final yes/no against the published answer automatically. Report correct counts by arm and verdict, plus both correct, plain only, assisted only, and neither. Check submitted attacks against the prose for diagnostics. Examine each disagreement and distinguish capture errors, tool failures, and final-answer errors. Compare token usage and wall time because the assisted arm gets an extra model turn.

Stop after these ten pairs. A tie, a loss, or a ceiling result is publishable. No human labeling, model judge, or significance claim is needed. Decide whether to expand only after inspecting the paired outcomes.
