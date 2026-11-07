import json
import unittest

from ml.evaluator import EvaluationError, evaluate


def completed(scores, **stats):
    return {"done": True, "done_reason": "stop",
            "message": {"content": json.dumps(scores)}, **stats}


class EvaluatorTests(unittest.TestCase):
    def setUp(self):
        self.dialogue = {
            "conversation_id": "synthetic", "language": "RU", "channel": "agent chat",
            "transcript": [
                {"timestamp": "00:01", "speaker": "client", "text": "Вопрос"},
                {"timestamp": "00:01", "speaker": "operator", "text": "Ответ"},
            ],
            "audio_url": "https://example.invalid/audio",
        }
        self.rubric = {"version": "test", "criteria": [
            {"criterion_id": "new-rule", "title": "Rule", "weight": 5,
             "rules": {"YES": "Action present", "NO": "Action absent"}}]}
        self.item = {"answer": "YES", "reason": "The operator answered.",
                     "evidence": {"kind": "turns", "turn_ids": [1]}}

    def test_replaced_criterion_and_duplicate_timestamps(self):
        def reply(payload):
            self.assertIn("new-rule", payload["format"]["properties"])
            content = json.loads(payload["messages"][-1]["content"])
            self.assertEqual([t["turn_id"] for t in content["transcript"]], [0, 1])
            self.assertNotIn("audio_url", content)
            return completed({"new-rule": self.item})

        result = evaluate(self.dialogue, self.rubric, chat=reply)
        self.assertEqual(result["criteria"], [{"criterion_id": "new-rule", **self.item}])

    def test_evidence_deduplication_and_reason_first(self):
        def reply(payload):
            props = list(payload["format"]["properties"]["new-rule"]["properties"])
            self.assertEqual(props, ["reason", "answer", "evidence"])
            dup_item = {"reason": "OK", "answer": "YES",
                        "evidence": {"kind": "turns", "turn_ids": [1, 1]}}
            return completed({"new-rule": dup_item})

        result = evaluate(self.dialogue, self.rubric, chat=reply)
        self.assertEqual(result["criteria"][0]["evidence"]["turn_ids"], [1])

    def test_numeric_string_conversion(self):
        numeric_rubric = {
            "version": "test",
            "criteria": [
                {"criterion_id": "04", "title": "Consultation", "weight": 20,
                 "rules": {"20": "Closed", "10": "Partial", "0": "None", "NA": "Aborted"}}
            ]
        }
        def reply(payload):
            # Model emitted "20" as string from string enum
            item = {"reason": "All good", "answer": "20", "evidence": {"kind": "turns", "turn_ids": [1]}}
            return completed({"04": item})

        result = evaluate(self.dialogue, numeric_rubric, chat=reply)
        self.assertEqual(result["criteria"][0]["answer"], 20)
        self.assertIsInstance(result["criteria"][0]["answer"], int)

    def test_invalid_answers_and_evidence_are_rejected(self):
        for changed in (
            {"answer": "NA"},
            {"answer": True},
            {"reason": " "},
            {"evidence": {"kind": "turns", "turn_ids": [7]}},
            {"evidence": {"kind": "range", "turn_ids": [1, 0]}},
        ):
            with self.subTest(changed=changed), self.assertRaises(EvaluationError):
                evaluate(self.dialogue, self.rubric,
                         chat=lambda _: completed({"new-rule": {**self.item, **changed}}))

    def test_missing_criterion_is_not_a_result(self):
        with self.assertRaises(EvaluationError):
            evaluate(self.dialogue, self.rubric, chat=lambda _: completed({}))

    def test_truncated_generation_is_not_a_result(self):
        def reply(_):
            response = completed({"new-rule": self.item})
            response["done_reason"] = "length"
            return response

        with self.assertRaises(EvaluationError):
            evaluate(self.dialogue, self.rubric, chat=reply)


if __name__ == "__main__":
    unittest.main()
