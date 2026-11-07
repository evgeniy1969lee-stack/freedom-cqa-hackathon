import json
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import threading
import unittest
from functools import partial
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from ml.evaluator import evaluate
from ml.service import make_server


class ServiceTests(unittest.TestCase):
    def setUp(self):
        self.hold = False
        self.entered = threading.Event()
        self.release = threading.Event()
        self.dialogue = {
            "conversation_id": "synthetic",
            "channel": "agent chat", "language": "KZ",
            "transcript": [
                {"turn_id": 0, "timestamp": "00:00", "speaker": "client", "text": "Question"},
                {"turn_id": 1, "timestamp": "00:00", "speaker": "operator", "text": "Answer"},
            ],
        }
        self.checklist = {
            "version": 17,
            "criteria": [
                {"criterion_id": "custom", "name": "Custom", "description": "Score the response",
                 "type": "scale", "steps": [{"value": 0, "label": "Missing"},
                                            {"value": 3, "label": "Done"}], "weight": 25},
                {"criterion_id": "05", "type": "yes_no", "description": "Confirm status",
                 "na_allowed": True, "na_rule": "No status question", "weight": 5},
            ],
            "gross_error_categories": ["Confidential disclosure"],
        }
        self.scores = {
            "custom": {"answer": 3, "reason": "Done",
                       "evidence": {"kind": "turns", "turn_ids": [1]}},
            "05": {"answer": "NA", "reason": "No status question",
                   "evidence": {"kind": "range", "turn_ids": [0, 1]}},
            "08": {"answer": "G1", "reason": "Disclosed confidential information",
                   "evidence": {"kind": "turns", "turn_ids": [1]}},
        }

        def fake_chat(payload):
            if self.hold:
                self.entered.set()
                if not self.release.wait(4):
                    raise RuntimeError("Test did not release inference")
            self.sent = json.loads(payload["messages"][-1]["content"])
            return {"done": True, "done_reason": "stop",
                    "message": {"content": json.dumps(self.scores)}}

        self.server = make_server(("127.0.0.1", 0), partial(evaluate, chat=fake_chat), lambda: True)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.thread.join)
        self.addCleanup(self.server.shutdown)

    def post(self, body):
        request = Request(f"http://127.0.0.1:{self.server.server_port}/evaluate",
                          data=json.dumps(body).encode(),
                          headers={"Content-Type": "application/json"})
        with urlopen(request, timeout=5) as response:
            return json.load(response)

    def test_backend_contract(self):
        result = self.post({"conversation": self.dialogue, "checklist": self.checklist})
        self.assertEqual(self.sent["transcript"], self.dialogue["transcript"])
        self.assertEqual(self.sent["criteria"][0]["rules"], {"0": "Missing", "3": "Done"})
        self.assertEqual(result["results"][0]["answer"], 3)
        self.assertEqual(result["results"][1]["answer"], "NA")
        self.assertEqual(result["results"][1]["evidence"], {"kind": "range", "from": 0, "to": 1})
        self.assertEqual(result["gross_error"]["category"], "Confidential disclosure")
        self.assertTrue(result["gross_error"]["flag"])
        self.assertTrue(result["prompt_version"])

    def test_health_during_inference(self):
        self.hold = True
        data = {"conversation": self.dialogue, "checklist": self.checklist}
        with ThreadPoolExecutor(max_workers=2) as pool:
            first = pool.submit(self.post, data)
            try:
                self.assertTrue(self.entered.wait(2))
                second = pool.submit(self.post, data)
                url = f"http://127.0.0.1:{self.server.server_port}/health"
                with urlopen(url, timeout=2) as response:
                    self.assertTrue(json.load(response)["ready"])
                self.assertFalse(first.done())
            finally:
                self.release.set()
            self.assertEqual(first.result()["results"], second.result()["results"])

    def test_incomplete_model_response_is_error(self):
        del self.scores["custom"]
        with self.assertRaises(HTTPError) as error:
            self.post({"conversation": self.dialogue, "checklist": self.checklist})
        self.assertEqual(error.exception.code, 502)
        self.assertIn("error", json.load(error.exception))

    def test_missing_checklist_is_bad_request(self):
        with self.assertRaises(HTTPError) as error:
            self.post({"conversation": self.dialogue})
        self.assertEqual(error.exception.code, 400)
        error.exception.close()


class SeedTests(unittest.TestCase):
    def test_official_checklist(self):
        seed = Path(__file__).resolve().parents[1] / "backend/internal/checklist/seed.json"
        checklist = json.loads(seed.read_text(encoding="utf-8-sig"))
        self.assertFalse(checklist.get("draft"))
        criteria = {item["criterion_id"]: item for item in checklist["criteria"]}
        self.assertEqual([c["weight"] for c in criteria.values()], [10, 15, 20, 20, 15, 15, 5])
        self.assertEqual({k for k,c in criteria.items() if c["na_allowed"]}, {"02","03","04","07"})
        self.assertEqual([s["value"] for s in criteria["04"]["steps"]], [0,5,10,15,20])
        self.assertEqual([s["value"] for s in criteria["06"]["steps"]], [0,4,7,10])
        self.assertEqual(len(checklist["gross_error_categories"]), 3)
