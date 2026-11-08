# Freedom CQA · Нуфтяники

Рабочее место для контроля качества поддержки. Приложение импортирует транскрипты, оценивает их по выбранной версии чек-листа и показывает балл, обоснования и реплики-доказательства. В реестре можно отфильтровать грубые ошибки. Из карточки можно подать апелляцию и затем увидеть пересчитанный балл.

- [DEV · 65 оценок](results/dev/results.json)
- [PROD · 10 контрольных чатов FBroker](results/prod/results.json)
- [Формат JSON 1.0 и команды сдачи](docs/submission.md)

## Запуск проекта и локальной модели

Установите **Go 1.27.1+**, **Python 3.10+** и **Ollama**. Запустите Ollama. Откройте PowerShell и выполните:

~~~powershell
git clone https://github.com/evgeniy1969lee-stack/freedom-cqa-hackathon.git
cd freedom-cqa-hackathon
ollama pull qwen3.5:2b
python -X utf8 scripts/run_local.py
~~~

После сообщения `CQA ready` откройте [http://127.0.0.1:8080/](http://127.0.0.1:8080/). В заголовке появится `qwen3.5:2b · готова`. Для проверки нажмите **Импорт JSON**, выберите [тестовый диалог](backend/testdata/example_conversation.json), откройте `example-001` и нажмите **Оценить**. Дождитесь результата и нажмите на ссылку под критерием, чтобы увидеть реплику. На ноутбуке с 8 ГБ ОЗУ приложение выполняет одну оценку за раз. Ctrl+C останавливает Go и Python.

Запуск модели отдельно, без интерфейса и Go:

~~~powershell
ollama list
python -X utf8 -m ml.evaluator examples/synthetic-ru.json
~~~

Локальный запуск обслуживает интерфейс и API по одному адресу. Go ставит задачи в очередь и сохраняет версии, оценки и апелляции в SQLite. Python передаёт транскрипт и снимок чек-листа модели **Qwen3.5:2b** через локальный Ollama. Промпт `ml2-v4` запрашивает структурированные ответы, обоснования и ссылки на реплики. Go проверяет ответы и рассчитывает балл. Для звонков приложение использует предоставленный транскрипт и открывает запись отдельно.

## Чек-лист и веса

| № | Критерий | Вес | Допустимые ответы |
| --- | --- | ---: | --- |
| 01 | Установление контакта | 10 | Да, нет |
| 02 | Выявление потребности | 15 | Да, нет, N/A |
| 03 | Идентификация | 20 | Да, нет, N/A |
| 04 | Корректная и полная консультация | 20 | 0, 5, 10, 15, 20, N/A |
| 05 | Эмпатия и клиентоориентированность | 15 | Да, нет |
| 06 | Управление диалогом, скорость и грамотность | 15 | 0, 4, 7, 10 |
| 07 | Завершение | 5 | Да, нет, N/A |
| 08 | Грубая ошибка | Отдельный флаг | Да, нет |

«Да» даёт полный вес, «нет» даёт 0. Вклад шкального критерия равен `вес × ступень / максимальная ступень`. Итог равен `100 × сумма вкладов / сумма применимых весов`, с округлением до двух знаков. N/A исключается из расчёта. Например, 7 из 10 за управление диалогом даёт 10,5 из 15 взвешенных баллов.

N/A разрешён, когда для выявления потребности уже хватало сведений, запрос не требовал идентификации, чат оборвался до формулировки запроса или оператор не успел завершить разговор. Для пропущенного действия система указывает проверенный участок. Категории грубой ошибки включают оскорбление клиента, предложение платной услуги в обход компании и раскрытие конфиденциальной информации. Полные правила и ступени хранятся в [конфигурации основного чек-листа](backend/internal/checklist/seed.json).

В JSON 1.0 отправляется исходная автоматическая оценка. Апелляция хранится в истории отдельно. Номера доказательств в файлах сдачи начинаются с 1 и относятся ко всему исходному массиву реплик. Ссылки с ключами доступа, исходные транскрипты, аудио и локальные базы хранятся вне публичного Git.

[Роли команды и план](docs/team-plan.md)

## ML-1 baseline

Run the Python verification suite from the repository root.

~~~powershell
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

## Manual startup and troubleshooting

The launcher binds to localhost. Use `--api-port`, `--ml-port`, and `--db`
for an isolated run, or `--go` if Go is installed outside `PATH`.

To start the services manually for debugging, start the Python HTTP service in
a terminal at the repository root.

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

For the organizer API, set `CQA_DEV_URL` in the terminal before
starting the launcher or backend. Use the full GET link from page 3 of the Student
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
