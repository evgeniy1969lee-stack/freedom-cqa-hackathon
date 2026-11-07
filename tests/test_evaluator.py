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
