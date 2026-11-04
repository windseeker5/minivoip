# Writes down a recording on this machine, for notes/local.go: on the graphics
# card, or slower on the processor if the card can't. Nothing leaves the machine.
# Arguments: the recording, and 2 if each of its two channels is one person.
# The last line printed is the answer.
import ctypes
import glob
import json
import sys

# The speech library asks for NVIDIA's libraries by name, and they aren't where it looks.
for folder in sys.path:
    for lib in ("nvidia/cublas/lib/libcublasLt.so.12", "nvidia/cublas/lib/libcublas.so.12", "nvidia/cudnn/lib/libcudnn.so.9"):
        for found in glob.glob(folder + "/" + lib):
            ctypes.CDLL(found, mode=ctypes.RTLD_GLOBAL)

from faster_whisper import WhisperModel, decode_audio

MODEL = "mobiuslabsgmbh/faster-whisper-large-v3-turbo"


def write_down(device, compute_type, file, channels):
    model = WhisperModel(MODEL, device=device, compute_type=compute_type, local_files_only=True)
    sounds = decode_audio(file, split_stereo=True) if channels == 2 else [file]
    lines, seconds = [], 0.0
    for channel, sound in enumerate(sounds):
        # The quiet is skipped: speech recognition makes up words in silence.
        segments, info = model.transcribe(sound, vad_filter=True, condition_on_previous_text=False)
        for s in segments:
            lines.append({"channel": channel, "start": s.start, "text": s.text.strip()})
        seconds = max(seconds, info.duration)
    return {"seconds": seconds, "lines": lines}


file, channels = sys.argv[1], int(sys.argv[2])
try:
    answer = write_down("cuda", "int8_float16", file, channels)
except RuntimeError:
    answer = write_down("cpu", "int8", file, channels)
print(json.dumps(answer))
