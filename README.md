# Freedom CQA Hackathon

Prototype for evaluating customer-support conversations against a configurable quality checklist.

Status: in development.

Team roles and implementation prompts are in [docs/team-plan.md](docs/team-plan.md).

The project must keep credentials, signed dataset links, recordings, transcripts and local databases outside the public repository.

## ML-1 baseline

The evaluator uses the locally installed Ollama model. Run these commands from
the repository root with Python 3.10+ and Ollama running.

~~~powershell
ollama list
python -X utf8 -m ml.evaluator examples/synthetic-ru.json
python -m unittest discover -s tests -v
~~~

Backend entry point: `ml.evaluator.evaluate(dialogue, rubric=None)`.
Pass one record from `data.records`, or a normalized record with unique
nonnegative integer `turn_id` values. The evaluator uses transcript order
when IDs are absent. It sends roles, text, and original timestamps to the
local model. It does not send audio URLs. A result contains the rubric
version, model, prompt version, criterion answers, evidence, and timings.

The default rubric is [ml/rubric.json](ml/rubric.json). For a new version,
pass a rubric with `version` and `criteria`. Each criterion has a
`criterion_id`, title, weight and a `rules` map from allowed answer to
description. Use YES, NO, NA or a nonnegative integer key as appropriate.
Numeric answers are returned as integers. A range carries two endpoint IDs in
transcript order. The backend computes weighted scores and handles appeals.

Run one evaluation at a time on the 8 GiB laptop. The default context is 3,072
tokens with a 768-token output cap. Overflow and incomplete generations fail
the job. For diagnosis, the CLI accepts `--num-ctx` and `--model`. Benchmark
larger context sizes on the target machine before using them in a batch.

## Run the backend with local ML

Start Ollama with the installed `qwen3.5:2b` model available. In a terminal
at the repository root, start the Python HTTP service.

~~~powershell
python -X utf8 -m ml.service --port 8091
~~~

In another terminal at the repository root, start the Go API.

~~~powershell
Set-Location backend
go run ./cmd/api -addr 127.0.0.1:8080 -ml-url http://127.0.0.1:8091/evaluate -workers 1
~~~

To import and evaluate the synthetic example from the repository root:

~~~powershell
$api = 'http://127.0.0.1:8080'
Invoke-RestMethod -Method Post "$api/api/import" -ContentType 'application/json' -InFile 'backend/testdata/example_conversation.json'
$job = Invoke-RestMethod -Method Post "$api/api/conversations/example-001/evaluate" -ContentType 'application/json' -Body '{}'
Invoke-RestMethod "$api/api/jobs/$($job.job.id)"
~~~

Once the job has completed, request its `evaluation_id` from
`GET /api/evaluations/{id}`. The Go queue sends the transcript and the
selected checklist version to Python. Go calculates the weighted total
and stores the separate gross-error flag. The service accepts one request
at a time on the loopback interface. Model failures return an error to
the queue for its normal retry handling.

Fresh databases start with the checklist from the organizer PDF. An existing
database whose latest checklist is the original draft gets the official
checklist as a new version. Earlier jobs and evaluations keep their original
snapshots. To re-evaluate a dialogue with corrected weights, enqueue it using
the new checklist version.
