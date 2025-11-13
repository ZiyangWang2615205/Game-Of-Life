# import pandas as pd
# import matplotlib.pyplot as plt
# import seaborn as sns



# with open("results.csv", "r") as f_in, open("parsed_results.csv", "w") as f_out:
#     # 새 CSV 헤더
#     f_out.write("name,sec_per_op,CI\n")
#     for line in f_in:
#         line = line.strip()
#         # 우리가 필요로 하는 진짜 벤치마크 결과만 필터링
#         if line.startswith("Gol/"):
#             f_out.write(line + "\n")


# df = pd.read_csv("parsed_results.csv")


# # remove "Gol/" prefix
# df["bench"] = df["name"].str.replace("Gol/", "", regex=False)


# df["config"] = df["bench"].str.extract(r"(.+)-\d+-\d+")
# df["threads"] = df["bench"].str.extract(r".+-(\d+)-\d+").astype(int)
# df["cpu"] = df["bench"].str.extract(r".+-(\d+)$").astype(int)

# df[["width", "height", "turns"]] = df["config"].str.extract(r"(\d+)x(\d+)x(\d+)")
# df[["width", "height", "turns"]] = df[["width", "height", "turns"]].astype(int)

# df["time_sec"] = df["sec_per_op"]

# # generate graphs
# sns.set(style="whitegrid")

# board_sizes = df[["width", "height"]].drop_duplicates()

# sns.set(style="whitegrid")

# board_sizes = df[["width", "height"]].drop_duplicates()

# for _, row in board_sizes.iterrows():
#     w, h = row["width"], row["height"]

#     subset = df[(df["width"] == w) & (df["height"] == h)]

#     plt.figure(figsize=(12, 6))

#     for t in sorted(subset["turns"].unique()):
#         s = subset[subset["turns"] == t].sort_values("threads")
#         plt.plot(
#             s["threads"],
#             s["time_sec"],
#             marker="o",
#             label=f"{t} turns"
#         )

#     plt.title(f"Game of Life Benchmark: {w}x{h}")
#     plt.xlabel("Worker Threads")
#     plt.ylabel("Time (seconds)")
#     plt.legend(title="Turns")
#     plt.grid(True, linestyle="--", alpha=0.5)
#     plt.tight_layout()

#     outname = f"plot_{w}x{h}_allturns_line.png"
#     plt.savefig(outname)
#     plt.close()

#     print(f"Saved: {outname}")


# print("All plots generated.")

# # to run on linux/ubuntu
# # python3 -m venv .venv
# # source .venv/bin/activate
# # pip install pandas matplotlib seaborn

import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns

# ==========================
# Load & Filter CSV
# ==========================
with open("results.csv", "r") as f_in, open("parsed_results.csv", "w") as f_out:
    f_out.write("name,sec_per_op,CI\n")
    for line in f_in:
        line = line.strip()
        if line.startswith("Gol/"):
            f_out.write(line + "\n")

df = pd.read_csv("parsed_results.csv")

# ==========================
# Parse benchmark fields
# ==========================
df["bench"] = df["name"].str.replace("Gol/", "", regex=False)
df["config"] = df["bench"].str.extract(r"(.+)-\d+-\d+")
df["threads"] = df["bench"].str.extract(r".+-(\d+)-\d+").astype(int)
df["cpu"] = df["bench"].str.extract(r".+-(\d+)$").astype(int)
df[["width", "height", "turns"]] = df["config"].str.extract(r"(\d+)x(\d+)x(\d+)")
df[["width", "height", "turns"]] = df[["width", "height", "turns"]].astype(int)
df["time_sec"] = df["sec_per_op"]

sns.set(style="whitegrid")

board_sizes = df[["width", "height"]].drop_duplicates()

# ==========================
# (1) Original line plots
# ==========================
for _, row in board_sizes.iterrows():
    w, h = row["width"], row["height"]

    subset = df[(df["width"] == w) & (df["height"] == h)]

    plt.figure(figsize=(12, 6))

    for t in sorted(subset["turns"].unique()):
        s = subset[subset["turns"] == t].sort_values("threads")
        plt.plot(
            s["threads"], s["time_sec"],
            marker="o",
            label=f"{t} turns"
        )

    plt.title(f"Game of Life Benchmark: {w}x{h}")
    plt.xlabel("Worker Threads")
    plt.ylabel("Time (seconds)")
    plt.legend(title="Turns")
    plt.grid(True, linestyle="--", alpha=0.5)
    plt.tight_layout()

    outname = f"plot_{w}x{h}.png"
    plt.savefig(outname)
    plt.close()
    print(f"Saved: {outname}")

# ==========================
# (2) Speedup & Efficiency plots
# ==========================
def compute_speedup(df_board):
    """Compute speedup per turn count."""
    speedup_data = []
    for t in sorted(df_board["turns"].unique()):
        subset = df_board[df_board["turns"] == t].sort_values("threads")
        t1 = subset[subset["threads"] == 1]["time_sec"].values[0]
        subset = subset.copy()
        subset["speedup"] = t1 / subset["time_sec"]
        speedup_data.append(subset)
    return pd.concat(speedup_data)

def compute_efficiency(df_speed):
    df = df_speed.copy()
    df["efficiency"] = df["speedup"] / df["threads"]
    return df

for _, row in board_sizes.iterrows():
    w, h = row["width"], row["height"]
    df_board = df[(df["width"] == w) & (df["height"] == h)]

    # ---- Speedup ----
    df_speed = compute_speedup(df_board)

    plt.figure(figsize=(12, 6))
    for t in sorted(df_speed["turns"].unique()):
        s = df_speed[df_speed["turns"] == t]
        plt.plot(s["threads"], s["speedup"], marker="o", label=f"{t} turns")

    plt.title(f"Speedup Plot: {w}x{h}")
    plt.xlabel("Worker Threads")
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
        s = df_eff[df_eff["turns"] == t]
        plt.plot(s["threads"], s["efficiency"], marker="o", label=f"{t} turns")

    plt.title(f"Efficiency Plot: {w}x{h}")
    plt.xlabel("Worker Threads")
    plt.ylabel("Efficiency (Speedup / Threads)")
    plt.grid(True, linestyle="--", alpha=0.5)
    plt.legend(title="Turns")
    plt.tight_layout()

    outname = f"efficiency_{w}x{h}.png"
    plt.savefig(outname)
    plt.close()
    print(f"Saved: {outname}")

print("All plots generated.")
