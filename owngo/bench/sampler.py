#!/usr/bin/env python3
"""Зводить вивід sampler.c у частки функцій: sampler.py out.txt [top].

sampler.py out.txt --disasm FUNC друкує асемблер функції FUNC (ім'я компоновки,
як у звіті) з кількістю семплів на кожній інструкції (P-NEW-07.3). Семпл
припадає на інструкцію, яку перервав сигнал, тож час довгої інструкції часто
видно на наступній.
"""
import collections
import json
import re
import subprocess
import sys


def is_exec(obj, cache={}):
    if obj not in cache:
        with open(obj, "rb") as f:
            cache[obj] = f.read(18)[16] == 2  # e_type == ET_EXEC
    return cache[obj]


def load(path):
    """Повертає адреси семплів, зведені за об'єктами, і кількість без об'єкта."""
    maps, pcs, base = [], [], {}
    for line in open(path):
        kind, rest = line.split(" ", 1)
        if kind == "map":
            f = rest.split()
            # [vdso] і подібні — не файли; їхні адреси йдуть у «no mapping».
            if len(f) < 6 or not f[5].startswith("/"):
                continue
            if int(f[2], 16) == 0:
                base.setdefault(f[5], int(f[0].split("-")[0], 16))
            if "x" not in f[1]:
                continue
            lo, hi = (int(x, 16) for x in f[0].split("-"))
            maps.append((lo, hi, int(f[2], 16), f[5]))
        else:
            pcs.append(int(rest, 16))
    by_obj = collections.defaultdict(list)
    unknown = 0
    for pc in pcs:
        for lo, hi, _, obj in maps:
            if lo <= pc < hi:
                # Адреса для llvm-symbolizer — віртуальна адреса ELF. У PIE і
                # .so перший сегмент має адресу 0, у ET_EXEC — справжню.
                by_obj[obj].append(pc - (0 if is_exec(obj) else base[obj]))
                break
        else:
            unknown += 1
    return by_obj, unknown, len(pcs)


def symbolize(obj, offs):
    uniq = sorted(set(offs))
    inp = "".join("0x%x\n" % o for o in uniq)
    out = subprocess.run(
        ["llvm-symbolizer", "--obj=" + obj, "--functions=linkage",
         "--no-inlines", "--output-style=GNU"],
        input=inp, capture_output=True, text=True).stdout.split("\n")
    return {o: out[2 * i] for i, o in enumerate(uniq)}


def outer(obj, addrs):
    """Зовнішня (фізична) функція кожної адреси: останній кадр вбудовування."""
    out = subprocess.run(
        ["llvm-symbolizer", "--obj=" + obj, "--output-style=JSON", "--functions=linkage"],
        input="".join("0x%x\n" % a for a in addrs), capture_output=True, text=True,
        check=True).stdout
    res = {}
    for line in out.splitlines():
        rec = json.loads(line)
        frame = rec["Symbol"][-1]
        res[int(rec["Address"], 16)] = (frame["FunctionName"], int(frame["StartAddress"] or "0", 16))
    return res


def disasm(path, func):
    by_obj, _, total = load(path)
    for obj, offs in by_obj.items():
        names = symbolize(obj, offs)
        mine = [o for o in offs if names[o] == func]
        if mine:
            break
    else:
        sys.exit("no samples in %s" % func)
    # FUNC може бути вбудованою: друкується вся фізична функція, у якій лежить
    # більшість її семплів, з семплами всіх вбудованих у неї функцій. У
    # бінарнику llgo під LTO локальних символів немає, тож межі беруться з
    # DWARF: початок — StartAddress, кінець — перша інструкція іншої функції.
    owners = outer(obj, sorted(set(offs)))
    host, start = collections.Counter(owners[o] for o in mine).most_common(1)[0][0]
    counts = collections.Counter(o for o in offs if owners[o][0] == host)
    n = sum(counts.values())
    print("%s: %d samples (%.1f%%) in %s: %d samples, %.1f%% of %d" % (
        func, len(mine), 100.0 * len(mine) / total, host, n, 100.0 * n / total, total))
    objdump = subprocess.run(
        ["llvm-objdump", "-d", "--no-show-raw-insn",
         "--start-address=0x%x" % start, "--stop-address=0x%x" % (max(counts) + 0x1000),
         obj], capture_output=True, text=True, check=True).stdout
    insn = re.compile(r"\s*([0-9a-f]+):\s+(.*)")
    lines = [(int(m.group(1), 16), m.group(2)) for m in map(insn.match, objdump.splitlines()) if m]
    owner = outer(obj, [a for a, _ in lines])
    inl = symbolize(obj, [a for a, _ in lines])
    for addr, text in lines:
        if owner[addr][0] != host:
            break
        c = counts.get(addr, 0)
        mark = "%5.1f%% %6d" % (100.0 * c / n, c) if c else ""
        print("%-13s  %x: %-60s ; %s" % (mark, addr, text.expandtabs(8), inl[addr]))


def main():
    if len(sys.argv) > 3 and sys.argv[2] == "--disasm":
        disasm(sys.argv[1], sys.argv[3])
        return
    path = sys.argv[1]
    top = int(sys.argv[2]) if len(sys.argv) > 2 else 15
    by_obj, unknown, total = load(path)
    funcs = collections.Counter()
    for obj, offs in by_obj.items():
        names = symbolize(obj, offs)
        short = obj.rsplit("/", 1)[-1]
        for o in offs:
            name = names[o]
            if name == "??":
                name = "?? (%s)" % short
            funcs[name] += 1
    if unknown:
        funcs["?? (no mapping)"] += unknown
    print("samples %d" % total)
    for name, n in funcs.most_common(top):
        print("%6.1f%% %7d  %s" % (100.0 * n / total, n, name))


if __name__ == "__main__":
    main()
