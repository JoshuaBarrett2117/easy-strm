"""更新日期：2026-10-08；维护者：Codex。仅 DBA 显式执行；不读取凭据文件。

持久 psql 会话和 pg_stat_activity 锁等待构成真实 barrier，不以 sleep 推断提交顺序。
SQL 协议演练不等同于运行 Go 导出器；文件故障注入由本地 Go mock/临时 FS 测试覆盖。
"""
import concurrent.futures
import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading
import time
import uuid


class Session:
    def __init__(self, name, output):
        self.name = name
        self.log_path = output / (name + ".log")
        self.log = self.log_path.open("w")
        self.lines = queue.Queue()
        self.lock = threading.Lock()
        command = ["psql", "-X", "-w", "-qAt", "--host", os.environ["PGHOST"], "--port", os.environ["PGPORT"], "--username", os.environ["PGUSER"], "--dbname", os.environ["PGDATABASE"], "-v", "ON_ERROR_STOP=1", "-v", "VERBOSITY=verbose"]
        self.process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.reader.start()
        self.execute("SET search_path=public; SET statement_timeout='90s'; SET lock_timeout='30s'; DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'wrong target'; END IF; END $$;")
        self.pid = int(self.execute("SELECT pg_backend_pid();")[0])

    def _read(self):
        for line in self.process.stdout:
            self.log.write(line)
            self.log.flush()
            self.lines.put(line.rstrip("\n"))
        self.lines.put(None)

    def execute(self, sql):
        with self.lock:
            marker = "PHASE02_BARRIER_" + uuid.uuid4().hex
            self.log.write("SQL> " + sql + "\n")
            self.log.flush()
            self.process.stdin.write(sql + "\n\\echo " + marker + "\n")
            self.process.stdin.flush()
            result = []
            deadline = time.monotonic() + 100
            while True:
                line = self.lines.get(timeout=max(0.01, deadline - time.monotonic()))
                if line is None:
                    self.process.wait(timeout=5)
                    raise RuntimeError(f"{self.name}: psql exit={self.process.returncode}; see {self.log_path}")
                if line == marker:
                    return result
                if line:
                    result.append(line)

    def close(self):
        if self.process.poll() is None:
            try:
                self.process.stdin.write("ROLLBACK;\n\\q\n")
                self.process.stdin.flush()
                self.process.wait(timeout=5)
            except (BrokenPipeError, subprocess.TimeoutExpired):
                self.process.terminate()
                self.process.wait(timeout=5)
        self.reader.join(timeout=5)
        self.log.close()


def quote(value):
    return "'" + str(value).replace("'", "''") + "'"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def wait_lock(control, session):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if control.execute(f"SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid={session.pid} AND wait_event_type='Lock');") == ["t"]:
            return
        time.sleep(0.02)
    raise AssertionError(f"{session.name}: no confirmed database lock barrier")


def main():
    require(sys.argv[1:] == ["--synthetic-test-only", "--execute"], "DBA explicit flags required; no connection made")
    for name in ("PGHOST", "PGPORT", "PGUSER", "PGDATABASE", "DBA_PHASE02_OUTPUT", "DBA_PHASE02_TARGET_CONFIRM", "DBA_PHASE02_CONFIRM"):
        require(bool(os.environ.get(name)), "missing external variable: " + name)
    require(os.environ["PGDATABASE"] == "easy_strm_test", "refuse non-isolated database")
    require(os.environ["DBA_PHASE02_TARGET_CONFIRM"] == f"{os.environ['PGHOST']}:{os.environ['PGPORT']}/{os.environ['PGDATABASE']}", "target confirmation mismatch")
    require(os.environ["DBA_PHASE02_CONFIRM"] == "isolated-synthetic-phase02-only", "authorization missing")
    require(bool(os.environ.get("TMPDIR")), "external DBA TMPDIR required")
    output = Path(os.environ["DBA_PHASE02_OUTPUT"]) / "concurrency"
    output.mkdir(parents=True, exist_ok=False)
    sessions = []
    results = []
    def create(name):
        session = Session(name, output)
        sessions.append(session)
        return session
    control, first, second = create("control"), create("first"), create("second")
    pool = concurrent.futures.ThreadPoolExecutor(max_workers=2)
    try:
        work_a = quote(control.execute("SELECT work_key FROM t_share_media WHERE id=200000003;")[0])
        work_b = quote(control.execute("SELECT work_key FROM t_share_media WHERE id=200000005;")[0])
        control.execute(f"UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]' WHERE work_key IN ({work_a},{work_b});")
        first.execute("BEGIN; UPDATE t_share_media_file SET available=NOT available WHERE id=300000003;")
        second.execute("BEGIN; UPDATE t_share_media_file SET available=NOT available WHERE id=300000005; COMMIT;")
        require(control.execute(f"SELECT revision>acked_revision FROM t_share_export_dirty_work WHERE work_key={work_a};") == ["f"], "uncommitted producer became visible")
        control.execute(f"UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]' WHERE work_key={work_b};")
        first.execute("COMMIT;")
        require(control.execute(f"SELECT revision>acked_revision FROM t_share_export_dirty_work WHERE work_key={work_a};") == ["t"], "late commit lost")
        results.append({"case": "late-commit-not-a-watermark", "passed": True})

        for producer_outcome, expected in (("COMMIT", "0"), ("ROLLBACK", "1")):
            control.execute(f"SELECT share_export_enqueue(ARRAY[{work_a}],'concurrency-fixture'); UPDATE t_share_export_dirty_work SET pending_export_keys='[\"phase02-race-K2\"]' WHERE work_key={work_a};")
            generation = int(control.execute(f"SELECT revision FROM t_share_export_dirty_work WHERE work_key={work_a};")[0])
            first.execute("BEGIN; UPDATE t_share_media_file SET available=NOT available WHERE id=300000003;")
            statement = f"BEGIN; SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE; SELECT revision FROM t_share_export_dirty_work WHERE work_key={work_a} FOR UPDATE; WITH ack AS (UPDATE t_share_export_dirty_work SET acked_revision=revision,last_seen_run_id='synthetic-concurrent-ack',pending_export_keys='[]',updated_at=now() WHERE work_key={work_a} AND revision={generation} AND acked_revision<>revision RETURNING 1) SELECT count(*) AS ack_count FROM ack;"
            pending = pool.submit(second.execute, statement)
            wait_lock(control, second)
            first.execute(producer_outcome + ";")
            rows = pending.result(timeout=40)
            require(rows[-1] == expected, "CAS outcome incorrect: " + repr(rows))
            second.execute("ROLLBACK;" if expected == "0" else "COMMIT;")
            if expected == "0":
                require(control.execute(f"SELECT pending_export_keys @> '\"phase02-race-K2\"'::jsonb AND revision>acked_revision FROM t_share_export_dirty_work WHERE work_key={work_a};") == ["t"], "failed CAS cleared pending")
            results.append({"case": "producer-wait-" + producer_outcome.lower(), "passed": True})

        first.execute("BEGIN; SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE; SELECT share_export_enqueue(ARRAY(SELECT work_key FROM t_share_media UNION SELECT work_key FROM t_share_export_source_state UNION SELECT work_key FROM t_share_export_dirty_work ORDER BY work_key),'baseline'); UPDATE t_share_export_consumer SET config_fingerprint='synthetic-fanout-before-concurrent-config',prepared_revision=config_revision,baseline_state='building';")
        pending = pool.submit(second.execute, "BEGIN; INSERT INTO t_system_config(config_key,config_val) VALUES('movie_naming_template','synthetic-concurrent-config') ON CONFLICT(config_key) DO UPDATE SET config_val=EXCLUDED.config_val||'-next'; COMMIT;")
        wait_lock(control, second)
        first.execute("COMMIT;")
        pending.result(timeout=40)
        require(control.execute("SELECT config_revision>prepared_revision AND baseline_state='required' FROM t_share_export_consumer;") == ["t"], "late config trusted old fanout")
        previous_config = int(control.execute("SELECT config_revision FROM t_share_export_consumer;")[0])
        second.execute("UPDATE t_system_config SET config_val=config_val||'-ack-race' WHERE config_key='movie_naming_template';")
        require(control.execute(f"WITH published AS (UPDATE t_share_export_consumer SET completed_revision=config_revision,baseline_state='ready' WHERE config_revision={previous_config} AND prepared_revision=config_revision AND NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE revision>acked_revision) RETURNING 1) SELECT count(*) FROM published;") == ["0"], "stale config published ready")
        results.append({"case": "config-during-fanout-and-before-ready", "passed": True})

        revision_before = int(control.execute("SELECT config_revision FROM t_share_export_consumer;")[0])
        fingerprint_before = control.execute("SELECT config_fingerprint FROM t_share_export_consumer;")[0]
        second.execute("UPDATE t_system_config SET config_val=config_val||'-before-prepare' WHERE config_key='movie_naming_template';")
        first.execute("BEGIN; SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE;")
        require(first.execute(f"WITH prepared AS (UPDATE t_share_export_consumer SET config_fingerprint='synthetic-stale-prepare' WHERE config_revision={revision_before} RETURNING 1) SELECT count(*) FROM prepared;") == ["0"], "Prepare advanced stale fingerprint")
        first.execute("ROLLBACK;")
        require(control.execute("SELECT config_fingerprint FROM t_share_export_consumer;") == [fingerprint_before], "failed Prepare changed fingerprint")
        results.append({"case": "config-before-prepare-cas", "passed": True})

        first.execute(f"BEGIN; SELECT share_export_enqueue(ARRAY[{work_b},{work_a}],'sorted-A');")
        pending = pool.submit(second.execute, f"BEGIN; SELECT share_export_enqueue(ARRAY[{work_a},{work_b}],'sorted-B'); COMMIT;")
        wait_lock(control, second)
        first.execute("COMMIT;")
        pending.result(timeout=40)
        results.append({"case": "set-based-sorted-overlap", "passed": True})

        deadlock_a, deadlock_b = create("deadlock-a"), create("deadlock-b")
        deadlock_a.execute(f"BEGIN; SELECT revision FROM t_share_export_dirty_work WHERE work_key={work_a} FOR UPDATE;")
        deadlock_b.execute(f"BEGIN; SELECT revision FROM t_share_export_dirty_work WHERE work_key={work_b} FOR UPDATE;")
        pending_a = pool.submit(deadlock_a.execute, f"UPDATE t_share_export_dirty_work SET scope='synthetic-deadlock-A' WHERE work_key={work_b};")
        wait_lock(control, deadlock_a)
        pending_b = pool.submit(deadlock_b.execute, f"UPDATE t_share_export_dirty_work SET scope='synthetic-deadlock-B' WHERE work_key={work_a};")
        deadlocks = 0
        for session, future in ((deadlock_a, pending_a), (deadlock_b, pending_b)):
            try:
                future.result(timeout=40)
                session.execute("ROLLBACK;")
            except RuntimeError:
                require("40P01" in session.log_path.read_text(), "non-deadlock error in deadlock case")
                deadlocks += 1
        require(deadlocks == 1, "expected exactly one SQLSTATE 40P01 victim")
        results.append({"case": "cross-statement-40P01-not-eliminated-by-sorting", "passed": True})

        filesystem = Path(tempfile.mkdtemp(prefix="easy-strm-phase02-", dir=os.environ["TMPDIR"]))
        old_file, new_file = filesystem / "K2.strm", filesystem / "K3.strm"
        old_key, new_key = "phase02-fault-K2", "phase02-fault-K3"
        control.execute(f"SELECT share_export_enqueue(ARRAY[{work_a}],'synthetic-fault'); UPDATE t_share_export_dirty_work SET pending_export_keys=(SELECT jsonb_agg(key ORDER BY key) FROM (SELECT jsonb_array_elements_text(pending_export_keys) key UNION SELECT {quote(old_key)}) keys) WHERE work_key={work_a}; INSERT INTO t_strm_export_history(owner_key,output_path,reason) VALUES('share:default',{quote(old_file)},'synthetic-fault-reservation');")
        require(control.execute(f"SELECT pending_export_keys @> to_jsonb({quote(old_key)}::text) FROM t_share_export_dirty_work WHERE work_key={work_a};") == ["t"], "K2 not committed before filesystem effect")
        old_file.write_text("https://phase02.invalid/K2\n")
        control.execute(f"INSERT INTO t_strm_export_state(owner_key,export_key,output_path,last_seen_run_id) VALUES('share:default',{quote(old_key)},{quote(old_file)},'synthetic-interrupted-K2');")
        first.execute("BEGIN; UPDATE t_share_media_file SET media_id=200000005 WHERE id=300000003; COMMIT;")
        control.execute(f"UPDATE t_share_export_dirty_work SET pending_export_keys=(SELECT jsonb_agg(key ORDER BY key) FROM (SELECT jsonb_array_elements_text(pending_export_keys) key UNION SELECT {quote(new_key)}) keys) WHERE work_key={work_b}; INSERT INTO t_strm_export_history(owner_key,output_path,reason) VALUES('share:default',{quote(new_file)},'synthetic-fault-reservation');")
        require(control.execute(f"SELECT pending_export_keys @> to_jsonb({quote(new_key)}::text) FROM t_share_export_dirty_work WHERE work_key={work_b};") == ["t"], "K3 not committed before filesystem effect")
        new_file.write_text("https://phase02.invalid/K3\n")
        control.execute(f"INSERT INTO t_strm_export_state(owner_key,export_key,output_path,last_seen_run_id) VALUES('share:default',{quote(new_key)},{quote(new_file)},'synthetic-recovered-K3'); UPDATE t_share_export_source_state SET media_id=200000005,work_key={work_b},export_keys=jsonb_build_array({quote(new_key)}),last_seen_run_id='synthetic-recovered-K3' WHERE source_file_id=300000003;")
        require(control.execute(f"SELECT pending_export_keys @> to_jsonb({quote(old_key)}::text) FROM t_share_export_dirty_work WHERE work_key={work_a};") == ["t"], "A lost committed K2 after B checkpoint overwrite")
        first.execute(f"BEGIN; SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE; SELECT revision FROM t_share_export_dirty_work WHERE work_key={work_a} FOR UPDATE; UPDATE t_strm_export_state output SET state='stale' WHERE owner_key='share:default' AND export_key={quote(old_key)} AND last_seen_run_id='synthetic-interrupted-K2' AND NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE state = 'active' AND work_key<>{work_a} AND export_keys @> to_jsonb({quote(old_key)}::text)) AND NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE pending_export_keys <> '[]'::jsonb AND work_key<>{work_a} AND pending_export_keys @> to_jsonb({quote(old_key)}::text)); UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]',last_seen_run_id='synthetic-recovery-ack' WHERE work_key={work_a}; COMMIT;")
        require(control.execute(f"SELECT state FROM t_strm_export_state WHERE export_key={quote(old_key)};") == ["stale"], "K2 not recovered as stale")
        require(control.execute(f"SELECT state FROM t_strm_export_state WHERE export_key={quote(new_key)};") == ["active"], "K3 incorrectly stale")
        require(old_file.read_text() == "https://phase02.invalid/K2\n" and new_file.read_text() == "https://phase02.invalid/K3\n", "recovery deleted or rewrote witness files")
        results.append({"case": "committed-K2-fault-rebind-K3-targeted-stale", "passed": True, "filesystem_evidence": str(filesystem), "scope": "real SQL and temporary files, not Go exporter"})
    finally:
        for session in sessions:
            session.close()
        pool.shutdown(wait=True, cancel_futures=True)
        (output / "results.json").write_text(json.dumps({"checks": results, "count": len(results), "scope": "DBA SQL protocol only; not Go real-DB integration"}, indent=2) + "\n")
    print(f"PASS: {len(results)} DBA SQL protocol concurrency cases; raw session logs saved")


if __name__ == "__main__":
    main()
