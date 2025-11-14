import re
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns

# 1) results.csv 에서 Gol/ 라인만 추출 + sec_per_op/CI 파싱해서 parsed_results.csv 생성
with open("results.csv", "r") as f_in, open("parsed_results.csv", "w") as f_out:
    f_out.write("name,sec_per_op,CI\n")

    for line in f_in:
        line = line.strip()
        if not line.startswith("Gol/"):
            continue

        parts = [p.strip() for p in line.split(",")]

        name = parts[0]

        sec = None
        ci = ""

        for token in parts[1:]:
            if token == "":
                continue
            if sec is None:
                try:
                    sec = float(token)
                    continue
                except ValueError:
                    pass
            if sec is not None and ci == "":
                ci = token
                break

        if sec is None:
            continue

        f_out.write(f"{name},{sec},{ci}\n")

df = pd.read_csv("parsed_results.csv")
df["sec_per_op"] = pd.to_numeric(df["sec_per_op"], errors="coerce")
df = df.dropna(subset=["sec_per_op"])


# 2) parsed_results.csv 로드
df = pd.read_csv("parsed_results.csv")

# float 보장
df["sec_per_op"] = pd.to_numeric(df["sec_per_op"], errors="coerce")
df = df.dropna(subset=["sec_per_op"])


# 3) 벤치마크 이름 파싱 준비
# 기대 형식: Gol/<width>x<height>x<turns>-<nodes>-<cpu> (예: Gol/512x512x100-3-8-8)
df["bench"] = df["name"].str.replace("Gol/", "", regex=False)

pattern = re.compile(r"^(\d+)x(\d+)x(\d+)-(\d+)-(\d+)$")

# name 컬럼에서 바로 width, height, turns, nodes, cpu 추출
# 예: Gol/512x512x100-3-8-8
m = df["name"].str.extract(r"Gol/(\d+)x(\d+)x(\d+)-(\d+)-(\d+)")

# 매칭 안 된 행(NaN)은 버림
m = m.dropna()

# 열 이름 부여
m.columns = ["width", "height", "turns", "nodes", "cpu"]

# 정수형으로 변환
m = m.astype(int)

# df도 같은 index만 남기고 붙이기
df = df.loc[m.index].copy()
df[["width", "height", "turns", "nodes", "cpu"]] = m


# 기존 코드와 호환을 위해 threads = nodes 로 둠
df["threads"] = df["nodes"]
df["time_sec"] = df["sec_per_op"]

sns.set(style="whitegrid")

# 보드 크기 종류
board_sizes = df[["width", "height"]].drop_duplicates()

# ---------------------------------------------------------------------
# (1) 시간 vs 노드수 그래프
# ---------------------------------------------------------------------
for _, row in board_sizes.iterrows():
    w, h = row["width"], row["height"]

    subset = df[(df["width"] == w) & (df["height"] == h)]

    plt.figure(figsize=(12, 6))

    for t in sorted(subset["turns"].unique()):
        s = subset[subset["turns"] == t].sort_values("nodes")
        plt.plot(
            s["nodes"],
            s["time_sec"],
            marker="o",
            label=f"{t} turns",
        )

    plt.title(f"Game of Life Benchmark: {w}x{h}")
    plt.xlabel("Worker Nodes")
    plt.ylabel("Time (seconds)")
    plt.legend(title="Turns")
    plt.grid(True, linestyle="--", alpha=0.5)
    plt.tight_layout()

    outname = f"benchmark_{w}x{h}.png"
    plt.savefig(outname)
    plt.close()
    print(f"Saved: {outname}")

# ---------------------------------------------------------------------
# (2) Speedup / Efficiency 계산 함수
# ---------------------------------------------------------------------
def compute_speedup(df_board: pd.DataFrame) -> pd.DataFrame:
    """
    각 보드에 대해 turns 별로 speedup 계산.
    - nodes == 1 이 있으면 그걸 baseline (T1)으로 사용
    - 없으면 가장 작은 nodes 를 baseline 으로 사용
    """
    speedup_data = []

    for t in sorted(df_board["turns"].unique()):
        subset = df_board[df_board["turns"] == t].sort_values("nodes")
        if subset.empty:
            continue

        if (subset["nodes"] == 1).any():
            t1 = subset[subset["nodes"] == 1]["time_sec"].iloc[0]
        else:
            t1 = subset["time_sec"].iloc[0]

        subset = subset.copy()
        subset["speedup"] = t1 / subset["time_sec"]
        speedup_data.append(subset)

    if not speedup_data:
        return pd.DataFrame()

    return pd.concat(speedup_data, ignore_index=True)


def compute_efficiency(df_speed: pd.DataFrame) -> pd.DataFrame:
    df_eff = df_speed.copy()
    df_eff["efficiency"] = df_eff["speedup"] / df_eff["nodes"]
    return df_eff

# ---------------------------------------------------------------------
# (3) Speedup / Efficiency 플롯
# ---------------------------------------------------------------------
for _, row in board_sizes.iterrows():
    w, h = row["width"], row["height"]
    df_board = df[(df["width"] == w) & (df["height"] == h)]

    # ---- Speedup ----
    df_speed = compute_speedup(df_board)
    if df_speed.empty:
        print(f"[경고] {w}x{h} 보드에 대해 speedup 계산 가능한 데이터 없음. (건너뜀)")
        continue

    plt.figure(figsize=(12, 6))
    for t in sorted(df_speed["turns"].unique()):
        s = df_speed[df_speed["turns"] == t].sort_values("nodes")
        plt.plot(
            s["nodes"],
            s["speedup"],
            marker="o",
            label=f"{t} turns",
        )

    plt.title(f"Speedup Plot: {w}x{h}")
    plt.xlabel("Worker Nodes")
    plt.ylabel("Speedup (T1 / Tn)")
    plt.grid(True, linestyle="--", alpha=0.5)
    plt.legend(title="Turns")
    plt.tight_layout()

    outname = f"speedup_{w}x{h}.png"
    plt.savefig(outname)
    plt.close()
    print(f"Saved: {outname}")

    # ---- Efficiency ----
    df_eff = compute_efficiency(df_speed)

    plt.figure(figsize=(12, 6))
    for t in sorted(df_eff["turns"].unique()):
        s = df_eff[df_eff["turns"] == t].sort_values("nodes")
        plt.plot(
            s["nodes"],
            s["efficiency"],
            marker="o",
            label=f"{t} turns",
        )

    plt.title(f"Efficiency Plot: {w}x{h}")
    plt.xlabel("Worker Nodes")
    plt.ylabel("Efficiency (Speedup / Nodes)")
    plt.grid(True, linestyle="--", alpha=0.5)
    plt.legend(title="Turns")
    plt.tight_layout()

    outname = f"efficiency_{w}x{h}.png"
    plt.savefig(outname)
    plt.close()
    print(f"Saved: {outname}")

print("All plots generated.")


# to run on linux/ubuntu
# python3 -m venv .venv
# source .venv/bin/activate
# pip install pandas matplotlib seaborn