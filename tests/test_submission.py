from pathlib import Path
from types import SimpleNamespace
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

from scripts import submit_results as submission


class SubmissionTests(unittest.TestCase):
    def setUp(self):
        seed = submission.read_json(submission.ROOT / "backend/internal/checklist/seed.json")
        self.rubric = submission.official_checklist(seed)
        values = ("YES", "N/A", "N/A", "15", "YES", "7", "YES")
        self.evaluation = {
            "conversation_id": "sample", "checklist_version": 1, "input_hash": "hash",
            "model": "qwen3.5:2b", "prompt_version": "ml2-v4",
            "items": [
                {"criterion_id": key, "answer": value, "reason": "Короткое объяснение.",
                 "evidence": {"kind": "range", "from": 3, "to": 5}}
                for key, value in zip(self.rubric, values)
            ],
            "total": 85.38, "final_total": 100,
            "gross_error": {"flag": False, "reason": "Грубых ошибок нет.",
                            "evidence": {"kind": "turns", "turn_ids": [0, 3]}},
        }

    def test_matches_appendix_types_and_original_score(self):
        row = submission.result_record("sample", 9, self.evaluation, self.rubric)
        self.assertEqual(row["score"], {"final_score": 85.38, "critical_flag": False})
        self.assertEqual([c["criterion_id"] for c in row["criteria"]], list(submission.CODES.values()))
        self.assertEqual([c["value"] for c in row["criteria"]], [True, None, None, 15, True, 7, True, False])
        self.assertIs(type(row["criteria"][3]["value"]), int)
        self.assertEqual(row["criteria"][0]["evidence_turns"], [4, 5, 6])
        self.assertEqual(row["criteria"][-1]["evidence_turns"], [1, 4])

    def test_rejects_malformed_evidence_and_wrong_score(self):
        with self.assertRaisesRegex(ValueError, "Invalid evidence turn"):
            submission.evidence_turns({"kind": "turns", "turn_ids": [9]}, 9)
        self.evaluation["total"] = 1
        with self.assertRaisesRegex(ValueError, "score differs"):
            submission.result_record("sample", 9, self.evaluation, self.rubric)

    def test_rejects_duplicate_ids_even_at_correct_count(self):
        rows = [{"conversation_id": "sample", "transcript": [{}]}] * 10
        with self.assertRaisesRegex(ValueError, "Duplicate conversation_id"):
            submission.dataset_records({"isSuccess": True, "data": {"records": rows}}, "prod")

    def test_deadline_keeps_ids_and_does_not_invent_scores(self):
        args = SimpleNamespace(api="local", dataset="prod", checklist_version=1, team_name="Team",
                               model="qwen3.5:2b", prompt_version="ml2-v4")
        records = [{"conversation_id": "sample", "transcript": [{}] * 9},
                   {"conversation_id": "unfinished", "transcript": [{}]}]
        hashes = {"sample": "hash", "unfinished": "hash2"}
        completed = {"conversation_id": "sample", "status": "completed", "input_hash": "hash",
                     "checklist_version": 1, "model": args.model, "prompt_version": args.prompt_version,
                     "id": 1, "evaluation_id": 1}
        stale = {**completed, "conversation_id": "unfinished", "input_hash": "hash2",
                 "prompt_version": "ml2-v3", "id": 2}
        stale_retry = {**completed, "prompt_version": "ml2-v3", "id": 3}
        with TemporaryDirectory() as temp, patch.object(submission, "ROOT", Path(temp)), \
                patch.object(submission, "verify_transcript"), \
                patch.object(submission, "request", side_effect=lambda _, route: {"jobs": [completed, stale, stale_retry]}
                             if route == "jobs" else self.evaluation):
            with self.assertRaisesRegex(ValueError, "no terminal result"):
                submission.export_results(args, records, hashes, self.rubric, False)
            submission.export_results(args, records, hashes, self.rubric, True)
            result = submission.read_json(Path(temp) / "results/prod/results.json")
        self.assertEqual(result["schema_version"], "1.0")
        self.assertEqual(result["model"], {"mode": "local", "name": "qwen3.5:2b"})
        self.assertEqual([r["conversation_id"] for r in result["results"]], ["sample", "unfinished"])
        error = result["results"][1]
        self.assertEqual((error["status"], error["criteria"], error["score"]), ("error", [], None))
        self.assertEqual(result["results"][0]["status"], "ok")

    def test_replaces_in_flight_job_that_completed_with_an_old_prompt(self):
        args = SimpleNamespace(api="local", checklist_version=1, model="qwen3.5:2b",
                               prompt_version="ml2-v4", pause_file=None, retry_failed=False)
        running = {"id": 1, "conversation_id": "sample", "input_hash": "hash",
                   "checklist_version": 1, "status": "running"}
        stale = {**running, "status": "completed", "model": args.model, "prompt_version": "ml2-v3"}
        current = {**stale, "id": 2, "prompt_version": args.prompt_version}
        with patch.object(submission, "request", side_effect=[{"jobs": [running]}, {"job": current}]) as request, \
                patch.object(submission, "wait_for_job", side_effect=[stale, current]):
            self.assertTrue(submission.run_jobs(args, [{"conversation_id": "sample"}], {"sample": "hash"}, None))
        request.assert_called_with("local", "conversations/sample/evaluate",
                                   {"checklist_version": 1, "force_new": True})


if __name__ == "__main__":
    unittest.main()
