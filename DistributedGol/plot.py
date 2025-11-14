#!/usr/bin/env python3
import csv
import math
import re

import matplotlib.pyplot as plt
import pandas as pd

RESULTS_CSV = "results.csv"


def load_results(path: str) -> pd.DataFrame:
    """
    results.csv 에서 분산 GoL 벤치마크 결과를 읽어온다.

    기대하는 name 형식:
        Gol/<width>x<height>x<turns>-<run>-<workers>
      예: Gol/16x16x1-1-8
    """
    rows = []

    with open(path, newline="") as f:
        reader = csv.reader(f)

        for fields in reader:
            # goos, goarch 같은 헤더 줄 / 빈 줄 건너뛰기
            if len(fields) < 3:
                continue

            name, sec_op, ci = fields[0], fields[1], fields[2]

            # 헤더 행 건너뛰기
            if name == "name" or name.strip() == "":
                continue

            # 우리가 원하는 벤치마크 행만 사용
            if not name.startswith("Gol/"):
                continue

            m = re.match(r"Gol/(\d+)x(\d+)x(\d+)-(\d+)-(\d+)", name)
            if not m:
                # 형식이 다르면 그냥 무시 (경고만 찍고 싶으면 print)
                # print("name format not matched:", name)
                continue

            width, height, turns, run, workers = map(int, m.groups())

            try:
                sec = float(sec_op)
            except ValueError:
                # 잘못된 값이면 스킵
                continue

            try:
                ci_val = float(ci)
            except ValueError:
                ci_val = math.nan

            rows.append(
                {
                    "name": name,
                    "width": width,
                    "height": height,
                    "turns": turns,
                    "run": run,
                    "workers": workers,
                    "sec_per_op": sec,
                    "ci": ci_val,
                }
            )

    if not rows:
        raise ValueError(
            "No benchmark rows parsed. results.csv 에 Gol/.. 형식의 행이 있는지 확인해."
        )

    return pd.DataFrame(rows)


def plot_time_vs_turns(df: pd.DataFrame, out_prefix: str = "plot_dist"):
    """
    각 보드 크기(16x16, 64x64, 512x512)에 대해
    x축: 턴 수 (1, 100, 50 등이 있다면 그대로 사용)
    y축: 전체 시뮬레이션 시간 (sec/op)
    으로 그래프를 그린다.
    """
    # (width, height) 조합 목록
    boards = (
        df[["width", "height"]]
        .drop_duplicates()
        .sort_values(["width", "height"])
        .itertuples(index=False, name=None)
    )

    for width, height in boards:
        sub = df[(df["width"] == width) & (df["height"] == height)]

        turns_list = sorted(sub["turns"].unique())

        plt.figure(figsize=(8, 5))

        # 1) 개별 run 을 약간의 x축 jitter 를 주어 산점도로 표시
        for i, t in enumerate(turns_list):
            t_sub = sub[sub["turns"] == t]
            x_center = t

            # 살짝 좌우로 흔들어서 겹치지 않게
            jitter = (pd.Series(range(len(t_sub))) - len(t_sub) / 2) / len(t_sub)
            x_vals = x_center + 0.3 * jitter

            plt.scatter(
                x_vals,
                t_sub["sec_per_op"],
                alpha=0.7,
                label=f"{t} turns runs" if i == 0 else None,
            )

        # 2) 턴별 평균 시간 선으로 연결
        mean_times = [sub[sub["turns"] == t]["sec_per_op"].mean() for t in turns_list]
        plt.plot(
            turns_list,
            mean_times,
            marker="o",
            linewidth=2,
        )

        workers = int(sub["workers"].iloc[0])

        plt.xlabel("Turns")
        plt.ylabel("Time per run (seconds)")
        plt.title(
            f"Distributed Game of Life – Board {width}x{height} (workers = {workers})"
        )
        plt.xticks(turns_list)
        plt.grid(True, linestyle="--", alpha=0.3)
        plt.tight_layout()

        fname = f"{out_prefix}_{width}x{height}.png"
        plt.savefig(fname, dpi=150)
        plt.close()
        print(f"Saved: {fname}")


def main():
    df = load_results(RESULTS_CSV)

    # 간단히 무엇이 들어있는지 터미널에 확인용으로 찍어볼 수 있음
    print("Loaded rows:", len(df))
    print(df.groupby(["width", "height", "turns"]).size())

    plot_time_vs_turns(df)


if __name__ == "__main__":
    main()

# to run on linux/ubuntu
# python3 -m venv .venv
# source .venv/bin/activate
# pip install pandas matplotlib seaborn