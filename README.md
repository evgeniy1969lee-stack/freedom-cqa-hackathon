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

Run one evaluation at a time on the 8 GiB laptop. The evaluator uses a
3,072-token context for short inputs and 12,288 tokens when the serialized
transcript and checklist exceed 7,500 characters. It also retries a short
input with the larger window if Ollama rejects it for context size. The
output cap is 768 tokens. The service allows up to nine minutes per
Ollama request with the larger context on CPU. The CLI accepts
`--num-ctx` and `--model` overrides for diagnosis.

## Run the backend with local ML

Install Go 1.27.1 or later, Python 3.10 or later, and Ollama. With Ollama
running, install the local model once with `ollama pull qwen3.5:2b`.
In a terminal at the repository root, start the Python HTTP service.

~~~powershell
python -X utf8 -m ml.service --port 8091
~~~

In another terminal at the repository root, start the Go API.

~~~powershell
Set-Location backend
go run ./cmd/api -addr 127.0.0.1:8080 -ml-url http://127.0.0.1:8091/evaluate -workers 1
~~~

Open `http://127.0.0.1:8080/`. Go serves the frontend and API from one
origin. The header shows when the Python bridge responds and the selected
model is installed in Ollama. The test stub has its own status and
results banner.

For the organizer API, set `CQA_DEV_URL` in the Go terminal before
starting the backend. Use the full GET link from page 3 of the Student
Guide. Open **Загрузить DEV** in the app and enter the group ID issued
for the dataset. The server sends the GET request. The **Импорт JSON**
action also accepts a saved organizer response.

To check the browser flow, import `backend/testdata/example_conversation.json`
with **Импорт JSON**, open the imported dialogue, and click **Оценить**.
Wait for the job to finish and click a criterion's evidence link to see the
referenced turns. The browser creates the job and then polls its status.
The backend sends the transcript and selected checklist to the bridge.
Ollama returns the criterion answers, and Go calculates and stores the score.

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
at a time on the loopback interface and responds to health probes during
inference. Model failures return an error to
the queue for its normal retry handling.

Fresh databases start with the checklist and sections from the organizer PDF.
The original draft and the stock checklist before sections were added each
get a new official version when they are the latest checklist. Earlier jobs
and evaluations keep their original snapshots. To apply the current checklist
to a dialogue, enqueue it using the new version.

Reimporting a changed transcript keeps earlier evaluations in history.
Historical results are labelled and the UI hides their evidence links to
the current text. Default exports include current transcript versions.
Use `GET /api/export?all=true` for history and filter on `input_hash` or
`historical` to distinguish revisions. JSON preserves text values as
stored, while CSV prefixes formula-like text for spreadsheet use.
