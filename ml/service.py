"""Local HTTP bridge for the Go evaluation queue."""

import argparse
from http.server import BaseHTTPRequestHandler, HTTPServer
import json

from ml.evaluator import EvaluationError, evaluate

MAX_REQUEST_BYTES = 4 * 1024 * 1024


def _rules(criterion):
    description = criterion["description"]
    if criterion["type"] == "yes_no":
        rules = {"YES": description, "NO": "Условие YES не выполнено."}
    elif criterion["type"] == "scale":
        rules = {str(step["value"]): step["label"] or f"Ступень {step['value']}."
                 for step in criterion["steps"]}
    else:
        raise ValueError("Unknown criterion type")
    if criterion.get("na_allowed"):
        rules["NA"] = criterion.get("na_rule") or "Критерий неприменим к этому диалогу."
    return rules


def _rubric(checklist):
    criteria = [
        {"criterion_id": item["criterion_id"], "title": item.get("name", item["criterion_id"]),
         "weight": item["weight"], "description": item["description"], "rules": _rules(item)}
        for item in checklist["criteria"]
    ]
    if not criteria:
        raise ValueError("Empty checklist")

    categories = checklist.get("gross_error_categories") or []
    if not isinstance(categories, list) or any(not isinstance(name, str) for name in categories):
        raise ValueError("Invalid gross error categories")
    category_by_answer = {f"G{i}": name for i, name in enumerate(categories, 1)}
    gross_id = None
    if categories:
        gross_id = "08"
        ids = {item["criterion_id"] for item in criteria}
        while gross_id in ids:
            gross_id += "_gross"
        criteria.append({
            "criterion_id": gross_id,
            "title": "Грубая ошибка. Выбери код выявленной категории либо NO.",
            "weight": 0, "flag": True,
            "rules": {"NO": "Ни одной из перечисленных ошибок нет.", **category_by_answer},
        })
    return {"version": checklist["version"], "criteria": criteria}, gross_id, category_by_answer


def _backend_response(result, gross_id, categories):
    rows = []
    gross = None
    for item in result["criteria"]:
        evidence = item["evidence"]
        if evidence["kind"] == "range":
            start, end = evidence["turn_ids"]
            evidence = {"kind": "range", "from": start, "to": end}
        if item["criterion_id"] == gross_id:
            answer = item["answer"]
            gross = {"flag": answer != "NO", "reason": item["reason"], "evidence": evidence}
            if answer != "NO":
                gross["category"] = categories[answer]
        else:
            rows.append({**item, "evidence": evidence})
    return {"model": result["model"], "prompt_version": result["prompt_version"],
            "results": rows, "gross_error": gross}


def make_server(address, run_evaluation=evaluate):
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            if self.path != "/evaluate":
                self.reply(404, {"error": "Unknown endpoint"})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= MAX_REQUEST_BYTES:
                    raise ValueError("Invalid request size")
                data = json.loads(self.rfile.read(length))
                conversation = data["conversation"]
                if not isinstance(conversation, dict) or not conversation.get("transcript"):
                    raise ValueError("Missing transcript")
                rubric, gross_id, categories = _rubric(data["checklist"])
            except (KeyError, TypeError, ValueError, AttributeError):
                self.reply(400, {"error": "Expected a conversation and a valid checklist"})
                return

            try:
                result = run_evaluation(conversation, rubric)
            except EvaluationError as exc:
                self.reply(502, {"error": str(exc)})
                return
            self.reply(200, _backend_response(result, gross_id, categories))

        def reply(self, status, value):
            body = json.dumps(value, ensure_ascii=False).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    # The development laptop runs one inference at a time.
    return HTTPServer(address, Handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=8091)
    args = parser.parse_args()
    with make_server(("127.0.0.1", args.port)) as server:
        print(f"ML evaluator listening on http://127.0.0.1:{server.server_port}/evaluate",
              flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
