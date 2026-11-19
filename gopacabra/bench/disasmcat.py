#!/usr/bin/env python3
"""Розкладає семпли розміченого асемблера за категоріями P-NEW-07.3.

Вхід — вивід sampler.py out.txt --disasm FUNC (один чи кілька файлів).
Категорія кожної інструкції з семплами:

  poll      опитування безпечної точки: завантаження PreemptRequested і
            перевірка за ним, тіло CooperativeSafepoint і mnClearPreempt;
  bounds    перевірка меж: cmp/test і умовний перехід на блок з викликом
            PanicIndex, PanicSlice*, AssertIndexRange, AssertSliceRange;
  nil       перевірка nil: те саме з AssertNilDeref;
  call      прямий виклик, пролог і епілог функції (push, pop, ret, call);
  indirect  непрямий виклик (call *...);
  stack     завантаження чи запис через %rsp/%rbp, крім push і pop;
  work      решта: обчислення програми.

Семпл припадає на інструкцію після тієї, що виконувалась, тож межі категорій
наближені: категорія — нижня оцінка частки, а не точна.

Використання: disasmcat.py FILE ...
"""
import collections
import re
import sys

LINE = re.compile(r"^\s*(?:([\d.]+)%\s+(\d+))?\s+([0-9a-f]+):\s+(\S+)\s*(.*?)\s* ; (.*)$")
TARGET = re.compile(r"^0x([0-9a-f]+)\b")
BOUNDS = re.compile(r"Panic(Index|Slice)|Assert(Index|Slice)Range")
NIL = re.compile(r"AssertNilDeref")
POLLFN = re.compile(r"CooperativeSafepoint|mnClearPreempt")


def parse(path):
    lines = []
    for text in open(path):
        m = LINE.match(text.rstrip("\n"))
        if m:
            count = int(m.group(2) or 0)
            lines.append((int(m.group(3), 16), m.group(4), m.group(5), m.group(6), count))
    return lines


def guard_targets(lines):
    """Адреси блоків, що закінчуються викликом паніки: адреса → категорія."""
    kinds = {}
    for i, (addr, *_) in enumerate(lines):
        for j in range(i, min(i + 8, len(lines))):
            _, op2, args2, _, _ = lines[j]
            if op2.startswith("call"):
                if BOUNDS.search(args2):
                    kinds[addr] = "bounds"
                elif NIL.search(args2):
                    kinds[addr] = "nil"
                break
            if op2.startswith(("jmp", "ret")):
                break
    return kinds


def classify(lines):
    guards = guard_targets(lines)
    cats = []
    for addr, op, args, fn, _ in lines:
        cat = "work"
        m = TARGET.match(args)
        if POLLFN.search(fn) or "PreemptRequested" in args:
            cat = "poll"
        elif op.startswith("j") and m and int(m.group(1), 16) in guards:
            cat = guards[int(m.group(1), 16)]
        elif op.startswith("call") and "*" in args:
            cat = "indirect"
        elif op.startswith(("push", "pop", "ret", "call")):
            cat = "call"
        elif re.search(r"\(%r[sb]p", args):
            cat = "stack"
        cats.append(cat)
    # Перевірка за завантаженням PreemptRequested і cmp/test перед переходом
    # на паніку належать тій самій категорії, що й сам перехід.
    for i, cat in enumerate(cats):
        if cat == "poll" and "PreemptRequested" in lines[i][2]:
            for j in (i + 1, i + 2):
                if j < len(lines) and lines[j][1].startswith(("test", "cmp", "j")):
                    cats[j] = "poll"
        if cat in ("bounds", "nil") and i > 0 and lines[i - 1][1].startswith(("cmp", "test")):
            cats[i - 1] = cat
    return cats


def main():
    order = ["work", "stack", "call", "indirect", "poll", "bounds", "nil"]
    print("| File | Samples | " + " | ".join(order) + " |")
    print("| --- | ---: | " + " | ".join("---:" for _ in order) + " |")
    for path in sys.argv[1:]:
        lines = parse(path)
        total = collections.Counter()
        for (_, _, _, _, count), cat in zip(lines, classify(lines)):
            total[cat] += count
        n = sum(total.values()) or 1
        print("| %s | %d | " % (path.rsplit("/", 1)[-1], n)
              + " | ".join("%.1f%%" % (100.0 * total[c] / n) for c in order) + " |")


if __name__ == "__main__":
    main()
