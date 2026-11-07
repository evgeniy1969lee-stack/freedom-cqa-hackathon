"""Run an organizer dataset and write the CQA submission format 1.0."""

import argparse
from decimal import Decimal, ROUND_HALF_UP
import json
from pathlib import Path
import re
from tempfile import NamedTemporaryFile
import time
from urllib.error import HTTPError
from urllib.parse import quote, urlsplit
from urllib.request import ProxyHandler, Request, build_opener


ROOT = Path(__file__).resolve().parents[1]
COUNTS = {"dev": 65, "prod": 10}
CODES = dict(zip(("01", "02", "03", "04", "05", "06", "07", "08"),
                 ("contact", "discovery", "identification", "consultation",
                  "empathy", "dialogue", "closure", "critical")))
OPENER = build_opener(ProxyHandler({}))


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def require(condition, message):
    if not condition:
        raise ValueError(message)


def public_text(value):
    require(isinstance(value, str) and value.strip(), "Empty rationale or error")
    value = re.sub(r"https?://\S+|[\w.+-]+@[\w.-]+\.[a-zA-Z]{2,}", "[скрыто]", value.strip())
    return re.sub(r"(?<!\d)\+?\d(?:[ ()-]*\d){6,}(?!\d)", "[скрыто]", value)


def request(base, route, body=None):
    raw = None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = Request(base + "/api/" + route, data=raw, headers={"Content-Type": "application/json"})
    try:
        with OPENER.open(req, timeout=60) as response:
            return json.load(response)
    except HTTPError as error:
        raise ValueError(f"{route}: HTTP {error.code}") from None


def dataset_records(document, dataset):
    data = document.get("data") if isinstance(document, dict) else None
    records = data.get("records") if isinstance(data, dict) else None
    require(isinstance(document, dict) and document.get("isSuccess") is True,
            "Input must be a successful response from the organizer API")
    require(isinstance(records, list) and len(records) == COUNTS[dataset],
            f"{dataset.upper()} must contain exactly {COUNTS[dataset]} records")
    seen = set()
    for record in records:
        require(isinstance(record, dict), "Each dataset record must be an object")
        cid = record.get("conversation_id")
        require(isinstance(cid, str) and cid.strip() == cid and cid, "Missing or invalid conversation_id")
        require(cid not in seen, f"Duplicate conversation_id: {cid}")
        require(isinstance(record.get("transcript"), list) and record["transcript"], f"{cid}: empty transcript")
        seen.add(cid)
    return records


def verify_transcript(base, record, source_hash):
    cid = record["conversation_id"]
    current = request(base, "conversations/" + quote(cid, safe=""))
    require(current["source_hash"] == source_hash, f"{cid}: transcript changed during export")
    require(current["channel"] == record.get("channel", "").strip()
            and current["language"] == record.get("language", "").strip().upper(),
            f"{cid}: channel or language differs from the organizer file")
    original = record["transcript"]
    turns = current["transcript"]
    require(len(turns) == len(original), f"{cid}: transcript length differs from the organizer file")
    for i, (expected, actual) in enumerate(zip(original, turns)):
        require(actual["turn_id"] == i and actual["timestamp"] == (expected.get("timestamp") or "")
                and actual["speaker"] == expected["speaker"].strip() and actual["text"] == expected["text"],
                f"{cid}: transcript differs from the organizer file at turn {i + 1}")


def official_checklist(checklist):
    seed = read_json(ROOT / "backend/internal/checklist/seed.json")
    keys = ("criterion_id", "name", "type", "description", "weight", "na_allowed", "na_rule", "steps")

    def normalized(row):
        return tuple(row.get(key) for key in keys)

    require(not checklist.get("draft"), "Choose the official checklist, not a draft")
    actual = {r["criterion_id"]: normalized(r) for r in checklist["criteria"]}
    expected = {r["criterion_id"]: normalized(r) for r in seed["criteria"]}
    require(actual == expected and checklist["gross_error_categories"] == seed["gross_error_categories"],
            "This checklist differs from the official submission rubric")
    return {r["criterion_id"]: r for r in seed["criteria"]}


def evidence_turns(evidence, count):
    kind = evidence.get("kind")
    if kind == "turns":
        ids = evidence.get("turn_ids", [])
    elif kind == "range":
        start, end = evidence.get("from"), evidence.get("to")
        require(type(start) is int and type(end) is int and 0 <= start <= end < count,
                "Invalid evidence range")
        ids = list(range(start, end + 1))
    else:
        raise ValueError("Missing evidence")
    require(ids and all(type(i) is int and 0 <= i < count for i in ids), "Invalid evidence turn")
    return [i + 1 for i in dict.fromkeys(ids)]


def result_record(cid, turn_count, evaluation, rubric):
    items = {item["criterion_id"]: item for item in evaluation["items"]}
    require(len(evaluation["items"]) == len(rubric) and items.keys() == rubric.keys(),
            f"{cid}: expected all seven scored criteria exactly once")
    criteria = []
    earned, applied = Decimal(0), Decimal(0)
    for key, criterion in rubric.items():
        item = items[key]
        answer = item["answer"]
        weight = Decimal(str(criterion["weight"]))
        if answer in ("N/A", "NA"):
            require(criterion.get("na_allowed"), f"{cid}: N/A is not allowed for {key}")
            value = None
        elif criterion["type"] == "yes_no":
            require(answer in ("YES", "NO"), f"{cid}: invalid answer for {key}")
            value = answer == "YES"
            applied += weight
            earned += weight * int(value)
        else:
            allowed = {str(step["value"]): step["value"] for step in criterion["steps"]}
            require(answer in allowed, f"{cid}: invalid scale step for {key}")
            value = allowed[answer]
            applied += weight
            earned += weight * value / max(allowed.values())
        criteria.append({"criterion_id": CODES[key], "value": value, "rationale": public_text(item["reason"]),
                         "evidence_turns": evidence_turns(item["evidence"], turn_count)})
    gross = evaluation["gross_error"]
    require(type(gross["flag"]) is bool, f"{cid}: invalid critical flag")
    criteria.append({"criterion_id": CODES["08"], "value": gross["flag"],
                     "rationale": public_text(gross["reason"]),
                     "evidence_turns": evidence_turns(gross["evidence"], turn_count)})
    require(applied > 0 and evaluation["total"] is not None, f"{cid}: missing score")
    score = (100 * earned / applied).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
    require(Decimal(str(evaluation["total"])) == score, f"{cid}: score differs from official calculation")
    return {"conversation_id": cid, "status": "ok", "criteria": criteria,
            "score": {"final_score": float(score), "critical_flag": gross["flag"]}}


def latest_jobs(jobs, hashes, version):
    latest = {}
    for job in sorted(jobs, key=lambda job: job["id"]):
        cid = job["conversation_id"]
        if job["checklist_version"] == version and cid in hashes and job["input_hash"] == hashes[cid]:
            latest[cid] = job
    return latest


def same_run(job, args):
    return job["model"] == args.model and job["prompt_version"] == args.prompt_version


def wait_for_job(base, job, deadline):
    while job["status"] in ("queued", "running"):
        if deadline is not None and time.monotonic() >= deadline:
            break
        time.sleep(3)
        job = request(base, f"jobs/{job['id']}")
    return job


def run_jobs(args, records, hashes, deadline):
    for index, record in enumerate(records, 1):
        if args.pause_file is not None and args.pause_file.exists():
            raise KeyboardInterrupt
        if deadline is not None and time.monotonic() >= deadline:
            return False
        cid = record["conversation_id"]
        jobs = request(args.api, "jobs?conversation_id=" + quote(cid, safe=""))["jobs"]
        job = latest_jobs(jobs, hashes, args.checklist_version).get(cid)
        stale = bool(job and job["status"] == "completed" and not same_run(job, args))
        if job is None or stale:
            row = request(args.api, "conversations/" + quote(cid, safe="") + "/evaluate",
                          {"checklist_version": args.checklist_version, "force_new": stale})
            job = row["job"]
        elif job["status"] == "failed" and args.retry_failed:
            job = request(args.api, f"jobs/{job['id']}/retry", {})
        job = wait_for_job(args.api, job, deadline)
        if deadline is not None and job["status"] in ("queued", "running"):
            return False
        require(job["status"] in ("completed", "failed"), f"{cid}: unexpected job state")
        if job["status"] == "completed":
            require(same_run(job, args), f"{cid}: unexpected model or prompt version")
        print(f"{index}/{len(records)} {cid} {job['status']}", flush=True)
    return True


def error_record(cid, message):
    return {"conversation_id": cid, "status": "error", "criteria": [], "score": None,
            "error": public_text(message)}


def export_results(args, records, hashes, rubric, allow_unfinished):
    latest = latest_jobs(request(args.api, "jobs")["jobs"], hashes, args.checklist_version)
    missing = [r["conversation_id"] for r in records
               if latest.get(r["conversation_id"], {}).get("status") not in ("completed", "failed")]
    require(not missing or allow_unfinished, f"{len(missing)} jobs still have no terminal result")
    rows = []
    for record in records:
        cid = record["conversation_id"]
        verify_transcript(args.api, record, hashes[cid])
        job = latest.get(cid)
        if job is None or job["status"] in ("queued", "running"):
            rows.append(error_record(cid, "Оценка не завершилась к моменту выгрузки."))
            continue
        if job["status"] == "failed":
            rows.append(error_record(cid, job["error"] or "Не удалось получить ответ модели."))
            continue
        require(same_run(job, args), f"{cid}: run the selected model and prompt before exporting")
        evaluation = request(args.api, f"evaluations/{job['evaluation_id']}")
        require(evaluation["input_hash"] == hashes[cid] and evaluation["conversation_id"] == cid
                and evaluation["checklist_version"] == args.checklist_version and same_run(evaluation, args),
                f"{cid}: evaluation does not match the selected dataset and run")
        rows.append(result_record(cid, len(record["transcript"]), evaluation, rubric))

    output = ROOT / "results" / args.dataset / "results.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    payload = {"schema_version": "1.0", "team_name": args.team_name.strip(),
               "model": {"mode": "local", "name": args.model}, "results": rows}
    content = json.dumps(payload, ensure_ascii=False, indent=2, allow_nan=False) + "\n"
    with NamedTemporaryFile(mode="w", encoding="utf-8", dir=output.parent, delete=False) as temporary:
        temporary.write(content)
    Path(temporary.name).replace(output)
    print(f"{output} · {len(rows)} records · {sum(row['status'] == 'error' for row in rows)} errors", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dataset", choices=COUNTS)
    parser.add_argument("input", type=Path, help="Saved organizer API response")
    parser.add_argument("--team-name")
    parser.add_argument("--api", default="http://127.0.0.1:18088")
    parser.add_argument("--checklist-version", type=int, required=True)
    parser.add_argument("--model", default="qwen3.5:2b")
    parser.add_argument("--prompt-version", default="ml2-v4")
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--process-only", action="store_true", help="Run jobs without writing a submission file")
    modes.add_argument("--export-only", action="store_true", help="Write an already processed dataset")
    parser.add_argument("--retry-failed", action="store_true")
    parser.add_argument("--pause-file", type=Path, help="Pause after the active job if this file is created")
    parser.add_argument("--deadline-minutes", type=int, help="Export actual results before the submission deadline")
    parser.add_argument("--allow-unfinished", action="store_true", help="Export unfinished jobs as errors")
    args = parser.parse_args()
    args.api = args.api.rstrip("/")
    try:
        require(args.process_only or args.team_name and args.team_name.strip(), "Pass the registered --team-name")
        require(args.checklist_version > 0, "Pin the official --checklist-version")
        endpoint = urlsplit(args.api)
        require(endpoint.scheme == "http" and endpoint.hostname in ("localhost", "127.0.0.1", "::1"),
                "Use the local Go API to keep the organizer dataset on this machine")
        require(args.deadline_minutes is None or args.deadline_minutes > 0, "Deadline must be positive")
        deadline = None if args.deadline_minutes is None else time.monotonic() + 60 * args.deadline_minutes
        doc = read_json(args.input)
        records = dataset_records(doc, args.dataset)
        rubric = official_checklist(request(args.api, f"checklists/{args.checklist_version}"))
        if not args.export_only:
            health = request(args.api, "health")
            require(health["ml_ready"] and health["ml_model"] == args.model, "The requested local model is not ready")
            request(args.api, "import", doc)
        summaries = request(args.api, "conversations")
        selected = {r["conversation_id"] for r in records}
        hashes = {r["conversation_id"]: r["source_hash"] for r in summaries if r["conversation_id"] in selected}
        require(hashes.keys() == selected, "Imported dataset IDs do not match")
        finished = args.export_only or run_jobs(args, records, hashes, deadline)
        if not finished:
            print("Time limit reached. The already queued job can finish.", flush=True)
        if not args.process_only:
            export_results(args, records, hashes, rubric, args.allow_unfinished or not finished)
    except (KeyError, TypeError, ValueError, OSError) as error:
        parser.exit(1, f"Submission error: {error}\n")
    except KeyboardInterrupt:
        parser.exit(130, "\nPaused. The already queued job can finish. Run the same command to resume.\n")


if __name__ == "__main__":
    main()
