# CQA prompt evaluation

This suite compares the current ML-1 prompt with two experimental variants.
All three use the same local Qwen 3.5 2B model, default rubric and evaluator.
Only synthetic dialogues are stored here. The expected labels cover all eight
criteria on RU and selected criteria on KZ and EN. KZ and EN were not used to
write the first candidate prompt. The compact variant was tried after the
first run, so its results are exploratory.

Run from the repository root with local Ollama running. The commands use
one evaluation at a time and write results outside the repository.

~~~powershell
$env:PROMPTFOO_DISABLE_TELEMETRY = '1'
$env:PROMPTFOO_DISABLE_UPDATE = '1'
$cli = Join-Path $env:USERPROFILE '.codex/tools/hackathon/node_modules/.bin/promptfoo.cmd'
& $cli eval -c ml/evals/promptfooconfig.yaml -j 1 --no-cache --no-share -o ../cqa-prompt-eval.json
~~~

The command exits nonzero when a response differs from the expected labels
or inference fails. Read criterion matches and timeouts separately. A valid
turn ID does not establish that the referenced turn supports the answer.

## Local pilot on 7 November 2026

Qwen 3.5 2B, Ollama on CPU, 3,072 context tokens and one request at a time.
The timings include model load when it occurred.

| Variant | RU known example | KZ challenge | EN challenge |
|---|---|---|---|
| ML-1 baseline | 5/8, 166 s | Timeout | Timeout |
| Clarified prompt | Timeout | 2/5, 121 s | Timeout |
| Compact JSON | 6/8, 126 s | Timeout | 2/4, 98 s |

The compact variant returned 479 tokens on RU compared with 582 for the
baseline. The baseline had about 22 seconds of model loading. The compact
run used an already loaded model, so the wall times do not isolate the effect
of the shorter response.

The compact variant fixed the RU closing answer but still missed the greeting
and consultation. On EN it missed late identity verification and the offer of
a private paid consultation. These two errors block promotion. The deployed
evaluator keeps the baseline prompt. The pilot covers three synthetic cases
and does not establish expected accuracy on DEV or the organizers' control
set.

Next, use two independent annotations of the same DEV chats, settle differences
against the official rubric, and repeat the comparison. Reserve separate
dialogues for the final check. Align the backend seed checklist and HTTP
contract before the first end-to-end evaluation.
