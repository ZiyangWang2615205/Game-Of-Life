import re
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns

# extract Gol/ line from results.csv + sec_per_op/CI then produce parsed_results.csv 
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


# load parsed_results.csv
df = pd.read_csv("parsed_results.csv")

# float
df["sec_per_op"] = pd.to_numeric(df["sec_per_op"], errors="coerce")
df = df.dropna(subset=["sec_per_op"])


# parse benchmark name
# expected: Gol/<width>x<height>x<turns>-<nodes>-<cpu> (ex: Gol/512x512x100-3-8-8)
df["bench"] = df["name"].str.replace("Gol/", "", regex=False)

pattern = re.compile(r"^(\d+)x(\d+)x(\d+)-(\d+)-(\d+)$")

# extract width, height, turns, nodes, cpu from name colunm
# 예: Gol/512x512x100-3-8-8
m = df["name"].str.extract(r"Gol/(\d+)x(\d+)x(\d+)-(\d+)-(\d+)")

m = m.dropna()

m.columns = ["width", "height", "turns", "nodes", "cpu"]

m = m.astype(int)

df = df.loc[m.index].copy()
df[["width", "height", "turns", "nodes", "cpu"]] = m


df["threads"] = df["nodes"]
df["time_sec"] = df["sec_per_op"]

sns.set(style="whitegrid")

board_sizes = df[["width", "height"]].drop_duplicates()

# time vs number of nodes

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


# Speedup / Efficiency calculation function
def compute_speedup(df_board: pd.DataFrame) -> pd.DataFrame:
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


# Speedup / Efficiency plot
for _, row in board_sizes.iterrows():
    w, h = row["width"], row["height"]
    df_board = df[(df["width"] == w) & (df["height"] == h)]

    # ---- Speedup ----
    df_speed = compute_speedup(df_board)
    if df_speed.empty:
        print(f"[warning]] {w}x{h} board has no available data to calculate speedup")
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