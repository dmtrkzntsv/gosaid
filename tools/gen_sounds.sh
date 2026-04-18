#!/usr/bin/env bash
# Generates the three feedback cues embedded in the gosaid binary.
# Run once from the repo root; commit the resulting WAVs.
#
# Format: 16-bit PCM, 22050 Hz, mono. Short (~150-250ms).

set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p assets/sounds

# start.wav — ascending blip (E5 -> A5, quick rise)
ffmpeg -y -f lavfi -i "sine=frequency=660:duration=0.08,volume=0.35" \
              -f lavfi -i "sine=frequency=880:duration=0.08,volume=0.35" \
       -filter_complex "[0:a][1:a]concat=n=2:v=0:a=1,afade=t=out:st=0.13:d=0.03" \
       -ar 22050 -ac 1 -sample_fmt s16 \
       assets/sounds/start.wav

# stop.wav — descending blip (A5 -> E5)
ffmpeg -y -f lavfi -i "sine=frequency=880:duration=0.08,volume=0.35" \
              -f lavfi -i "sine=frequency=660:duration=0.08,volume=0.35" \
       -filter_complex "[0:a][1:a]concat=n=2:v=0:a=1,afade=t=out:st=0.13:d=0.03" \
       -ar 22050 -ac 1 -sample_fmt s16 \
       assets/sounds/stop.wav

# error.wav — low buzz (two short low tones)
ffmpeg -y -f lavfi -i "sine=frequency=200:duration=0.12,volume=0.4" \
              -f lavfi -i "anullsrc=r=22050:cl=mono:d=0.05" \
              -f lavfi -i "sine=frequency=200:duration=0.12,volume=0.4" \
       -filter_complex "[0:a][1:a][2:a]concat=n=3:v=0:a=1,afade=t=out:st=0.25:d=0.04" \
       -ar 22050 -ac 1 -sample_fmt s16 \
       assets/sounds/error.wav

mkdir -p internal/audio/sounds
cp assets/sounds/*.wav internal/audio/sounds/

echo "generated:"
ls -la assets/sounds/ internal/audio/sounds/
