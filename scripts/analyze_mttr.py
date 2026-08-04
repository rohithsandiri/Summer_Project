#!/usr/bin/env python3
"""scripts/analyze_mttr.py

Statistical analysis for scripts/06-run-mttr-benchmark.sh's output CSVs.

Per scenario: mean +/- SD, median [IQR], a bootstrap 95% CI on the mean,
a Mann-Whitney U test (manual vs. automated), a rank-biserial effect size,
and a Bonferroni-corrected significance verdict across scenarios.

Deliberately stdlib-only (no numpy/scipy) so it runs anywhere Python 3 does,
including directly against a fresh cluster with no extra setup. The
Mann-Whitney p-value uses the normal approximation with a tie correction,
which is standard practice but is itself approximate for very small n; that
caveat is printed alongside every result rather than hidden in a docstring.

Usage:
    python3 scripts/analyze_mttr.py scenarios/results-automated.csv scenarios/results-manual.csv
"""
import csv
import math
import random
import sys
from collections import defaultdict

ALPHA = 0.05


def load(path, arm):
    rows = defaultdict(list)
    try:
        with open(path, newline="") as f:
            for row in csv.DictReader(f):
                if row.get("result") != "PASS":
                    continue
                try:
                    rows[row["scenario"]].append(float(row["mttr_seconds"]))
                except (KeyError, ValueError):
                    continue
    except FileNotFoundError:
        print(f"warning: {path} not found, skipping {arm} arm", file=sys.stderr)
    return rows


def mean_sd(xs):
    n = len(xs)
    m = sum(xs) / n
    if n < 2:
        return m, 0.0
    var = sum((x - m) ** 2 for x in xs) / (n - 1)
    return m, math.sqrt(var)


def median_iqr(xs):
    ys = sorted(xs)
    n = len(ys)

    def pct(p):
        idx = p * (n - 1)
        lo, hi = int(math.floor(idx)), int(math.ceil(idx))
        if lo == hi:
            return ys[lo]
        return ys[lo] + (ys[hi] - ys[lo]) * (idx - lo)

    return pct(0.5), pct(0.25), pct(0.75)


def bootstrap_ci_mean(xs, b=10000, seed=1234):
    rng = random.Random(seed)
    n = len(xs)
    means = []
    for _ in range(b):
        sample = [xs[rng.randrange(n)] for _ in range(n)]
        means.append(sum(sample) / n)
    means.sort()
    lo = means[int(0.025 * b)]
    hi = means[int(0.975 * b)]
    return lo, hi


def mann_whitney_u(a, b):
    """Two-sided Mann-Whitney U with tie correction and normal approximation.
    Returns (U, z, p_approx, rank_biserial_effect_size)."""
    combined = [(x, "a") for x in a] + [(x, "b") for x in b]
    combined.sort(key=lambda t: t[0])

    ranks = [0.0] * len(combined)
    i = 0
    while i < len(combined):
        j = i
        while j < len(combined) and combined[j][0] == combined[i][0]:
            j += 1
        avg_rank = (i + 1 + j) / 2.0  # 1-indexed average rank for the tie block
        for k in range(i, j):
            ranks[k] = avg_rank
        i = j

    rank_sum_a = sum(r for (_, grp), r in zip(combined, ranks) if grp == "a")
    n1, n2 = len(a), len(b)
    u1 = rank_sum_a - n1 * (n1 + 1) / 2.0
    u2 = n1 * n2 - u1
    u = min(u1, u2)

    # tie correction for the normal approximation's variance term
    tie_term = 0.0
    i = 0
    while i < len(combined):
        j = i
        while j < len(combined) and combined[j][0] == combined[i][0]:
            j += 1
        t = j - i
        if t > 1:
            tie_term += (t ** 3 - t)
        i = j
    N = n1 + n2
    mean_u = n1 * n2 / 2.0
    var_u = (n1 * n2 / 12.0) * ((N + 1) - tie_term / (N * (N - 1))) if N > 1 else 0.0
    if var_u <= 0:
        z = 0.0
    else:
        # continuity correction
        z = (abs(u - mean_u) - 0.5) / math.sqrt(var_u)
    p = math.erfc(z / math.sqrt(2))  # two-sided normal-approx p-value

    r_effect = 1 - (2 * u) / (n1 * n2) if n1 * n2 > 0 else 0.0  # rank-biserial
    return u, z, p, r_effect


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)

    auto = load(sys.argv[1], "automated")
    manual = load(sys.argv[2], "manual")
    scenarios = sorted(set(auto) | set(manual))
    if not scenarios:
        print("no PASS rows found in either CSV — nothing to analyze")
        sys.exit(1)

    n_comparisons = sum(1 for s in scenarios if len(auto.get(s, [])) >= 2 and len(manual.get(s, [])) >= 2)
    bonferroni_alpha = ALPHA / max(n_comparisons, 1)

    print(f"{'Scenario':<8}{'Arm':<10}{'n':<4}{'Mean±SD':<16}{'Median [IQR]':<20}{'95% CI (mean)':<20}")
    print("-" * 78)
    for s in scenarios:
        for arm, data in (("automated", auto), ("manual", manual)):
            xs = data.get(s, [])
            if not xs:
                print(f"{s:<8}{arm:<10}{'0':<4}{'—':<16}{'—':<20}{'—':<20}")
                continue
            m, sd = mean_sd(xs)
            med, q1, q3 = median_iqr(xs)
            ci_lo, ci_hi = bootstrap_ci_mean(xs) if len(xs) >= 2 else (m, m)
            print(f"{s:<8}{arm:<10}{len(xs):<4}{f'{m:.1f}±{sd:.1f}':<16}"
                  f"{f'{med:.1f} [{q1:.1f}, {q3:.1f}]':<20}{f'[{ci_lo:.1f}, {ci_hi:.1f}]':<20}")

    print()
    print("Mann-Whitney U (manual vs. automated), Bonferroni-corrected across "
          f"{n_comparisons} scenario(s): alpha={ALPHA} -> per-test threshold={bonferroni_alpha:.4f}")
    print(f"{'Scenario':<8}{'U':<10}{'z':<8}{'p (approx)':<12}{'effect r':<10}{'Verdict':<40}")
    print("-" * 88)
    for s in scenarios:
        a, m = auto.get(s, []), manual.get(s, [])
        if len(a) < 2 or len(m) < 2:
            print(f"{s:<8}{'—':<10}{'—':<8}{'—':<12}{'—':<10}"
                  f"{'insufficient n (<2 per arm) — no test run':<40}")
            continue
        u, z, p, r = mann_whitney_u(m, a)
        verdict = "significant (reject H0)" if p < bonferroni_alpha else "not significant"
        note = "" if min(len(a), len(m)) >= 10 else "  [n<10: normal approx is weak, treat as indicative only]"
        print(f"{s:<8}{u:<10.1f}{z:<8.2f}{p:<12.4f}{r:<10.2f}{verdict + note:<40}")

    print()
    print("Caveats printed deliberately, not omitted: the normal approximation used here")
    print("for the Mann-Whitney p-value is standard but approximate at small n; treat any")
    print("scenario with fewer than ~10 reps per arm as indicative, not confirmatory.")


if __name__ == "__main__":
    main()
