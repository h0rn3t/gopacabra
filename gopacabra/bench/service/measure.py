#!/usr/bin/env python3
"""Замір сервісу gopacabra/stand/rest під варіантами збірки (P11.7, Linux).

measure.py --load BIN [опції] NAME=BIN [NAME=BIN ...]

Кожен прогін — кожен варіант на свіжому процесі з порожньою базою в кожному
режимі; порядок варіантів зсувається на один з кожним прогоном (чергування).
Режим open — відкритий цикл: генератор з -rate; closed — закритий цикл,
генератор без -rate. Сервіс і генератор на окремих ядрах (taskset), тож
GOMAXPROCS кожного — кількість його ядер. Генератор вибирає RSS сервісу кожні
--sample і CPU за суміш (-pid).

Вивід — рядки генератора кожного прогону і таблиця Markdown: медіана і
мін–макс кожної метрики за варіантом і режимом. Код 1 — неочікувана відповідь,
ненульовий код сервісу чи генератора або непорожній лог сервісу.
"""
import argparse
import os
import signal
import statistics
import subprocess
import sys
import tempfile
import time
import urllib.request

METRICS = [  # ключ рядка генератора, заголовок, режими
    ("rps", "rps", ("open", "closed")),
    ("p50_ms", "p50, мс", ("open", "closed")),
    ("p99_ms", "p99, мс", ("open", "closed")),
    ("max_ms", "max, мс", ("open",)),
    ("rss_peak_kib", "RSS пік, МБ", ("open", "closed")),
    ("rss_steady_kib", "RSS усталена, МБ", ("open",)),
    ("user_s", "CPU user, с", ("open", "closed")),
    ("sys_s", "CPU sys, с", ("open", "closed")),
]


def wait_ready(url, proc):
    for _ in range(200):
        if proc.poll() is not None:
            sys.exit(f"server exited with {proc.returncode} before answering")
        try:
            with urllib.request.urlopen(url + "/healthz", timeout=1) as r:
                if r.status == 200:
                    return
        except OSError:
            pass
        time.sleep(0.05)
    sys.exit(f"{url}/healthz: no answer")


def one(args, name, binary, mode, run, port):
    """Прогін одного варіанта в одному режимі; повертає поля рядка генератора."""
    url = f"http://127.0.0.1:{port}"
    with tempfile.TemporaryDirectory() as tmp:
        log = os.path.join(args.out, f"{name}-{mode}-{run}.stderr")
        with open(log, "w") as err:
            server = subprocess.Popen(
                ["taskset", "-c", args.server_cpus, binary,
                 "-addr", f"127.0.0.1:{port}", "-db", os.path.join(tmp, "rest.sqlite")],
                stdout=err, stderr=err)
        try:
            wait_ready(url, server)
            cmd = ["taskset", "-c", args.load_cpus, args.load, "-url", url,
                   "-c", str(args.c), "-pid", str(server.pid), "-sample", args.sample,
                   "-rss-log", os.path.join(args.out, f"{name}-{mode}-{run}.rss")]
            if mode == "open":
                cmd += ["-rate", str(args.rate), "-n", str(args.n_open)]
            else:
                cmd += ["-n", str(args.n_closed)]
            load = subprocess.run(cmd, capture_output=True, text=True)
        finally:
            server.send_signal(signal.SIGINT)
            code = server.wait()
    line = load.stdout.strip()
    print(f"run={run} {name} {mode}: {line} server_exit={code}", flush=True)
    if load.returncode != 0 or code != 0 or os.path.getsize(log) > 0:
        sys.exit(f"{name} {mode} run {run}: load exit {load.returncode} {load.stderr.strip()}, "
                 f"server exit {code}, log {log}")
    return dict(kv.split("=", 1) for kv in line.split())


def fmt(key, values):
    if key.endswith("_kib"):
        values = [v / 1024 for v in values]
    digits = 0 if key == "rps" or key.endswith("_kib") else 2
    med = statistics.median(values)
    return f"{med:.{digits}f} ({min(values):.{digits}f}–{max(values):.{digits}f})"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--load", required=True, help="бінарник gopacabra/bench/service")
    ap.add_argument("--runs", type=int, default=10)
    ap.add_argument("--modes", default="open,closed")
    ap.add_argument("--rate", type=float, default=2000, help="запитів за секунду в режимі open")
    ap.add_argument("--n-open", type=int, default=60000)
    ap.add_argument("--n-closed", type=int, default=100000)
    ap.add_argument("-c", type=int, default=64, help="клієнти генератора")
    ap.add_argument("--sample", default="1s")
    ap.add_argument("--server-cpus", default="0-3")
    ap.add_argument("--load-cpus", default="4-7")
    ap.add_argument("--port", type=int, default=18090)
    ap.add_argument("--out", default=".", help="тека логів сервісу і рядів RSS")
    ap.add_argument("variants", nargs="+", metavar="NAME=BIN")
    args = ap.parse_args()
    variants = [v.split("=", 1) for v in args.variants]
    modes = args.modes.split(",")
    os.makedirs(args.out, exist_ok=True)

    results = {}  # (name, mode) -> [dict]
    for run in range(args.runs):
        k = run % len(variants)
        for name, binary in variants[k:] + variants[:k]:
            for mode in modes:
                r = one(args, name, binary, mode, run, args.port)
                results.setdefault((name, mode), []).append(r)

    for mode in modes:
        print(f"\n{mode}: медіана (мін–макс) з {args.runs} прогонів\n")
        names = [n for n, _ in variants]
        print("| Метрика | " + " | ".join(f"`{n}`" for n in names) + " |")
        print("| --- |" + " ---: |" * len(names))
        for key, title, in_modes in METRICS:
            if mode in in_modes:
                cells = [fmt(key, [float(r[key]) for r in results[(n, mode)]]) for n in names]
                print(f"| {title} | " + " | ".join(cells) + " |")


if __name__ == "__main__":
    main()
